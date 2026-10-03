package desired

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/scheduler"
)

func gbList(dir string, ifs []string, entries ...string) *ngfwv1.GlobalBlockingList {
	return &ngfwv1.GlobalBlockingList{
		Enabled: proto.Bool(true), Source: &ngfwv1.GlobalBlockingSource{Kind: proto.String("upload")}, AllInterfaces: proto.Bool(false),
		Interfaces: ifs, Direction: proto.String(dir), ProtectHost: proto.Bool(true), Log: proto.Bool(false), Entries: entries,
	}
}

func gbDoc(lists map[string]*ngfwv1.GlobalBlockingList, att ...*ngfwv1.AclAttachment) *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{
		Interfaces: map[string]*ngfwv1.Interface{"loop1": {}, "loop2": {}, "loop3": {}},
		Acl: &ngfwv1.AclConfig{
			Lists:          map[string]*ngfwv1.AclList{"a": {Rules: []*ngfwv1.AclRule{rule(10, "permit", nil)}}},
			Attachments:    att,
			GlobalBlocking: &ngfwv1.GlobalBlocking{Lists: lists},
		},
	}
}

func attachIn(list, ifn string) *ngfwv1.AclAttachment {
	return &ngfwv1.AclAttachment{List: proto.String(list), Target: &ngfwv1.AttachmentTarget{Kind: proto.String("interface"), Interface: proto.String(ifn)}, Direction: proto.String("in"), Sequence: proto.Uint32(10), Enabled: proto.Bool(true)}
}

func binding(t *testing.T, s *aclRecSink, ifn string) descacl.InterfaceBinding {
	t.Helper()
	b, err := descacl.InterfaceBindingFromProto(s.value(t, descacl.KeyInterfaceBinding(ifn)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A block list is put before the user's ACLs; a direction without a user ACL gets the pass ACL after it
// (VPP denies what no ACL of a bound direction matches); inbound drops by source, outbound by
// destination; the retrieved objects assemble back to the configuration.
func TestGlobalBlockingProjection(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	ds := gbDoc(map[string]*ngfwv1.GlobalBlockingList{
		"bad": gbList("both", []string{"loop1", "loop2"}, "192.0.2.7/32", "2001:db8::/48"),
	}, attachIn("a", "loop1"))
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	if len(s.issues) != 0 {
		t.Fatalf("issues: %v", s.issues)
	}
	b1, b2 := binding(t, s, "loop1"), binding(t, s, "loop2")
	if got := strings.Join(b1.Input, ","); got != "_gb.bad.i00,a" {
		t.Fatalf("loop1 in: %s", got)
	}
	if got := strings.Join(b1.Output, ","); got != "_gb.bad.o00,_gb.pass" {
		t.Fatalf("loop1 out: %s", got)
	}
	if got := strings.Join(b2.Input, ",") + "|" + strings.Join(b2.Output, ","); got != "_gb.bad.i00,_gb.pass|_gb.bad.o00,_gb.pass" {
		t.Fatalf("loop2: %s", got)
	}
	if _, ok := s.ptrs[descacl.KeyInterfaceBinding("loop3")]; ok {
		t.Fatal("loop3 is not selected")
	}
	in, _ := descacl.FromProto(s.value(t, descacl.KeyACL("_gb.bad.i00")))
	out, _ := descacl.FromProto(s.value(t, descacl.KeyACL("_gb.bad.o00")))
	if len(in.Rules) != 2 || in.Rules[0].Src != "192.0.2.7/32" || in.Rules[0].Dst != descacl.AnyV4 || in.Rules[0].Action != descacl.ActionDeny ||
		in.Rules[1].Src != "2001:db8::/48" || in.Rules[1].Dst != descacl.AnyV6 {
		t.Fatalf("inbound rules: %+v", in.Rules)
	}
	if out.Rules[0].Dst != "192.0.2.7/32" || out.Rules[0].Src != descacl.AnyV4 {
		t.Fatalf("outbound rules: %+v", out.Rules)
	}
	for _, r := range append(in.Rules, out.Rules...) {
		if err := (descacl.ACL{Name: "x", Rules: []descacl.Rule{r}}).Validate(); err != nil {
			t.Fatalf("rule %+v: %v", r, err)
		}
	}
	got := AssembleACL(s.kvs)
	if !proto.Equal(got, ds.GetAcl()) {
		t.Fatalf("assemble:\n%v\nwant\n%v", got, ds.GetAcl())
	}
}

// Someone removes an entry from a block-list ACL in VPP: the assembled lists are reconstructed from
// VPP (the drift view shows it); user lists and attachments are still attributed to the configuration.
func TestGlobalBlockingDriftReconstructs(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	ds := gbDoc(map[string]*ngfwv1.GlobalBlockingList{
		"bad": gbList("inbound", []string{"loop1"}, "192.0.2.7/32", "198.51.100.0/24"),
	}, attachIn("a", "loop1"))
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	var kvs []scheduler.KV
	for _, kv := range s.kvs {
		if kv.Key == descacl.KeyACL("_gb.bad.i00") {
			a, _ := descacl.FromProto(kv.Value)
			a.Rules = a.Rules[:1]
			kv.Value = a.Proto()
		}
		kvs = append(kvs, kv)
	}
	got := AssembleACL(kvs)
	l := got.GetGlobalBlocking().GetLists()["bad"]
	if strings.Join(l.GetEntries(), ",") != "192.0.2.7/32" || l.GetDirection() != "inbound" || strings.Join(l.GetInterfaces(), ",") != "loop1" {
		t.Fatalf("reconstructed: %v", l)
	}
	if !proto.Equal(got.GetLists()["a"], ds.GetAcl().GetLists()["a"]) || len(got.GetAttachments()) != 1 || got.GetAttachments()[0].GetSequence() != 10 {
		t.Fatalf("user part must stay attributed: %v", got)
	}
}

// allInterfaces selects every interface of the document; a disabled or empty list emits nothing but is
// still reported back as configured.
func TestGlobalBlockingAllInterfacesAndDisabled(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	all := gbList("outbound", nil, "203.0.113.9/32")
	all.AllInterfaces = proto.Bool(true)
	off := gbList("both", []string{"loop1"}, "198.51.100.1/32")
	off.Enabled = proto.Bool(false)
	ds := gbDoc(map[string]*ngfwv1.GlobalBlockingList{"all": all, "off": off, "empty": gbList("both", []string{"loop2"})})
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	if len(s.issues) != 0 {
		t.Fatalf("issues: %v", s.issues)
	}
	for _, ifn := range []string{"loop1", "loop2", "loop3"} {
		b := binding(t, s, ifn)
		if len(b.Input) != 0 || strings.Join(b.Output, ",") != "_gb.all.o00,_gb.pass" {
			t.Fatalf("%s: %+v", ifn, b)
		}
	}
	for _, kv := range s.kvs {
		if strings.HasPrefix(string(kv.Key), string(descacl.KeyACL("_gb.off"))) || strings.HasPrefix(string(kv.Key), string(descacl.KeyACL("_gb.empty"))) {
			t.Fatalf("disabled/empty list emitted %s", kv.Key)
		}
	}
	if got := AssembleACL(s.kvs); !proto.Equal(got.GetGlobalBlocking(), ds.GetAcl().GetGlobalBlocking()) {
		t.Fatalf("assemble: %v", got.GetGlobalBlocking())
	}
}

func manyEntries(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("10.%d.%d.%d/32", i>>16&255, i>>8&255, i&255)
	}
	return out
}

// Buckets are stable: changing one entry of a 10 000-entry list changes one bucket ACL; 200 000 entries
// project quickly into at most 64 buckets per direction and fit one interface (≤ 255 ACLs).
func TestGlobalBlockingBucketsAndScale(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	project := func(entries []string) map[scheduler.Key]proto.Message {
		s := newRecSink()
		ACL(s, gbDoc(map[string]*ngfwv1.GlobalBlockingList{"big": gbList("both", []string{"loop1"}, entries...)}), map[string]bool{"acl": true})
		if len(s.issues) != 0 {
			t.Fatalf("issues: %v", s.issues)
		}
		m := map[scheduler.Key]proto.Message{}
		for _, kv := range s.kvs {
			m[kv.Key] = kv.Value
		}
		return m
	}
	e := manyEntries(10_000)
	a := project(e)
	e2 := append([]string(nil), e...)
	e2[1234] = "192.0.2.1/32"
	b := project(e2)
	changed, buckets := 0, 0
	for k, v := range a {
		if strings.HasPrefix(string(k), string(descacl.KeyACL("_gb.big.i"))) {
			buckets++
			if !proto.Equal(v, b[k]) {
				changed++
			}
		}
	}
	if buckets != 4 || changed > 2 || changed == 0 {
		t.Fatalf("buckets %d, changed %d (want 4 buckets, the old and new entry's buckets changed)", buckets, changed)
	}

	start := time.Now()
	big := project(manyEntries(200_000))
	t.Logf("200 000 entries projected in %s", time.Since(start))
	bnd, _ := descacl.InterfaceBindingFromProto(big[descacl.KeyInterfaceBinding("loop1")])
	if len(bnd.Input) != 65 || len(bnd.Output) != 65 { // 64 buckets + pass
		t.Fatalf("binding sizes %d/%d", len(bnd.Input), len(bnd.Output))
	}
}

// Limits: a name that does not fit the VPP tag, and an interface taking more than 255 ACLs.
func TestGlobalBlockingLimits(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	s := newRecSink()
	ACL(s, gbDoc(map[string]*ngfwv1.GlobalBlockingList{strings.Repeat("n", 53): gbList("both", []string{"loop1"}, "192.0.2.7/32")}), map[string]bool{"acl": true})
	if len(s.issues) != 1 || !strings.Contains(s.issues[0], "acl.name") {
		t.Fatalf("issues %v", s.issues)
	}
	lists := map[string]*ngfwv1.GlobalBlockingList{}
	for i := range 128 { // 128 lists × 2 directions + the pass ACL
		lists[fmt.Sprintf("l%d", i)] = gbList("both", []string{"loop1"}, "192.0.2.7/32")
	}
	s = newRecSink()
	ACL(s, gbDoc(lists), map[string]bool{"acl": true})
	if len(s.issues) != 1 || !strings.Contains(s.issues[0], "acl.global-blocking-limit") {
		t.Fatalf("issues %v", s.issues)
	}
}

func TestParseGlobalBlockingACLName(t *testing.T) {
	for in, want := range map[string]string{"_gb.a.b.i07": "a.b/i", "_gb.x.o63": "x/o", "_gb.pass": "", "_gb.x.q01": "", "a.i00": ""} {
		l, d, ok := parseGlobalBlockingACLName(in)
		got := ""
		if ok {
			got = l + "/" + string(d)
		}
		if got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}
