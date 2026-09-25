package desired

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/scheduler"
)

// recSink records what a builder emits.
type recSink struct {
	kvs    []scheduler.KV
	ptrs   map[scheduler.Key]string
	issues []string
}

func newRecSink() *recSink { return &recSink{ptrs: map[scheduler.Key]string{}} }

func (s *recSink) Add(k scheduler.Key, v proto.Message, pointer string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.ptrs[k] = pointer
}
func (s *recSink) Errorf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "E "+pointer+" "+rule+" "+fmt.Sprintf(format, a...))
}
func (s *recSink) Warnf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "W "+pointer+" "+rule+" "+fmt.Sprintf(format, a...))
}

func (s *recSink) value(t *testing.T, k scheduler.Key) proto.Message {
	t.Helper()
	for _, kv := range s.kvs {
		if kv.Key == k {
			return kv.Value
		}
	}
	t.Fatalf("no %s in %v", k, s.kvs)
	return nil
}

func withEnv(t *testing.T, e ACLEnv) {
	t.Helper()
	prev := aclEnv.Load()
	if e.Record == nil {
		e.Record = aclstate.NewRecord(0)
	}
	SetACLEnv(e)
	t.Cleanup(func() { aclEnv.Store(prev) })
}

func rule(seq uint32, action string, mut func(r *vrxv1.AclRule)) *vrxv1.AclRule {
	r := &vrxv1.AclRule{Sequence: proto.Uint32(seq), Action: proto.String(action), Enabled: proto.Bool(true), IpVersion: proto.String("any")}
	if mut != nil {
		mut(r)
	}
	return r
}

func prefix(p string) *vrxv1.AddressMatch {
	return &vrxv1.AddressMatch{Kind: proto.String("prefix"), Prefix: proto.String(p)}
}

func object(n string) *vrxv1.AddressMatch {
	return &vrxv1.AddressMatch{Kind: proto.String("object"), Name: proto.String(n)}
}

// Attachments: a zone expands to its interfaces; per interface and direction the lists are ordered by
// attachment sequence (a zone attachment and an interface attachment interleave); disabled
// attachments are skipped; MACIP attachments bind one list per interface.
func TestACLBindingsOrderAndZones(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	ds := &vrxv1.DesiredState{
		Objects: &vrxv1.ObjectsConfig{Zones: map[string]*vrxv1.Zone{"lan": {Interfaces: []string{"loop3001", "loop3002"}}}},
		Acl: &vrxv1.AclConfig{
			Lists: map[string]*vrxv1.AclList{"a": {}, "b": {}, "c": {}},
			Macip: map[string]*vrxv1.MacipList{"m": {Rules: []*vrxv1.MacipRule{{Sequence: proto.Uint32(1), Action: proto.String("deny"), SourceMac: proto.String("02-00-00-AA-BB-CC"), SourcePrefix: proto.String("10.3.1.7/24")}}}},
			Attachments: []*vrxv1.AclAttachment{
				{List: proto.String("b"), Target: &vrxv1.AttachmentTarget{Kind: proto.String("interface"), Interface: proto.String("loop3001")}, Direction: proto.String("in"), Sequence: proto.Uint32(20)},
				{List: proto.String("a"), Target: &vrxv1.AttachmentTarget{Kind: proto.String("zone"), Zone: proto.String("lan")}, Direction: proto.String("in"), Sequence: proto.Uint32(10)},
				{List: proto.String("c"), Target: &vrxv1.AttachmentTarget{Kind: proto.String("zone"), Zone: proto.String("lan")}, Direction: proto.String("out"), Sequence: proto.Uint32(5)},
				{List: proto.String("c"), Target: &vrxv1.AttachmentTarget{Kind: proto.String("interface"), Interface: proto.String("loop3001")}, Direction: proto.String("in"), Sequence: proto.Uint32(1), Enabled: proto.Bool(false)},
			},
			MacipAttachments: []*vrxv1.MacipAttachment{{List: proto.String("m"), Interface: proto.String("loop3002")}},
		},
	}
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true, "objects": true})
	if len(s.issues) != 0 {
		t.Fatalf("issues: %v", s.issues)
	}
	b1, _ := descacl.InterfaceBindingFromProto(s.value(t, descacl.KeyInterfaceBinding("loop3001")))
	b2, _ := descacl.InterfaceBindingFromProto(s.value(t, descacl.KeyInterfaceBinding("loop3002")))
	if strings.Join(b1.Input, ",") != "a,b" || strings.Join(b1.Output, ",") != "c" || strings.Join(b2.Input, ",") != "a" || strings.Join(b2.Output, ",") != "c" {
		t.Fatalf("bindings %+v %+v", b1, b2)
	}
	m, _ := descacl.MacipACLFromProto(s.value(t, descacl.KeyMacipACL("m")))
	if len(m.Rules) != 1 || m.Rules[0].SrcMac != "02:00:00:aa:bb:cc" || m.Rules[0].SrcMacMask != "ff:ff:ff:ff:ff:ff" || m.Rules[0].SrcPrefix != "10.3.1.0/24" {
		t.Fatalf("macip canonical form: %+v", m)
	}
	if mb, _ := descacl.MacipBindingFromProto(s.value(t, descacl.KeyMacipBinding("loop3002"))); mb.ACL != "m" {
		t.Fatalf("macip binding %+v", mb)
	}
	// assemble from exactly these objects → the configuration (recorded), including the disabled attachment
	got := AssembleACL(s.kvs)
	if !proto.Equal(got, ds.GetAcl()) {
		t.Fatalf("assemble:\n%v\nwant\n%v", got, ds.GetAcl())
	}
}

// Retrieve of something no projection produced (someone changed the ACL in VPP) is reconstructed
// from the VPP rules — so the API's drift check shows the difference.
func TestAssembleACLReconstructsUnknownContent(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	v := descacl.ACL{Name: "x", Rules: []descacl.Rule{
		{Action: descacl.ActionPermit, Src: "10.0.0.0/8", Dst: descacl.AnyV4, Proto: 6, SrcPortLast: 65535, DstPortFirst: 80, DstPortLast: 81},
		{Action: descacl.ActionDeny, Src: descacl.AnyV6, Dst: descacl.AnyV6, Proto: 58, SrcPortFirst: 128, SrcPortLast: 128, DstPortLast: 255},
	}}.Proto()
	b := descacl.InterfaceBinding{Interface: "loop1", Input: []string{"x"}}.Proto()
	got := AssembleACL([]scheduler.KV{{Key: descacl.KeyACL("x"), Value: v}, {Key: descacl.KeyInterfaceBinding("loop1"), Value: b}})
	r := got.GetLists()["x"].GetRules()
	if len(r) != 2 || r[0].GetSequence() != 10 || r[0].GetSource().GetPrefix() != "10.0.0.0/8" || r[0].GetDestination().GetKind() != "any" ||
		r[0].GetService().GetSpec().GetProtocol() != "tcp" || strings.Join(r[0].GetService().GetSpec().GetDestinationPorts(), ",") != "80-81" ||
		r[1].GetIpVersion() != "ipv6" || r[1].GetService().GetSpec().GetProtocol() != "icmp6" || r[1].GetService().GetSpec().GetType() != 128 {
		t.Fatalf("reconstructed: %v", got)
	}
	if a := got.GetAttachments(); len(a) != 1 || a[0].GetTarget().GetInterface() != "loop1" || a[0].GetDirection() != "in" {
		t.Fatalf("attachments: %v", a)
	}
	if AssembleACL(nil) != nil {
		t.Fatal("empty retrieve must assemble to nil")
	}
}

// Name length: the VPP tag "<owner>:<name>" is at most 63 bytes.
func TestACLNameTooLongForTag(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	name := strings.Repeat("n", 61)
	s := newRecSink()
	ACL(s, &vrxv1.DesiredState{Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{name: {}}}}, map[string]bool{"acl": true})
	if len(s.issues) != 1 || !strings.Contains(s.issues[0], "acl.name") {
		t.Fatalf("issues %v", s.issues)
	}
}

// The list cap: more than 100 000 VPP rules is an error at the list; exactly 100 000 is fine, and the
// projection of a 100 000-rule list is fast (the time is logged for the task's evidence).
func TestACLListLimitAndProjectionTime(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	build := func(n int) *vrxv1.DesiredState {
		l := &vrxv1.AclList{}
		for i := range uint32(n) { //nolint:gosec // n ≤ 100 000
			l.Rules = append(l.Rules, rule(i+1, "permit", func(r *vrxv1.AclRule) {
				r.Source = prefix(fmt.Sprintf("10.%d.%d.0/24", i/256%256, i%256))
				r.Destination = prefix(fmt.Sprintf("172.%d.%d.%d/32", 16+i/65536, i/256%256, i%256))
			}))
		}
		return &vrxv1.DesiredState{Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"big": l}}}
	}
	ds := build(100_000)
	start := time.Now()
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	took := time.Since(start)
	if len(s.issues) != 0 {
		t.Fatalf("issues %v", s.issues[:1])
	}
	a, _ := descacl.FromProto(s.value(t, descacl.KeyACL("big")))
	if len(a.Rules) != 100_000 {
		t.Fatalf("rules %d", len(a.Rules))
	}
	t.Logf("projection of a 100 000-rule list (100 000 VPP rules): %v", took)

	ds.Acl.Lists["big"].Rules = append(ds.Acl.Lists["big"].Rules, rule(200_000, "deny", func(r *vrxv1.AclRule) { r.IpVersion = proto.String("ipv4") }))
	s = newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	if len(s.kvs) != 0 || len(s.issues) != 1 || !strings.Contains(s.issues[0], "E /acl/lists/big acl.list-limit") {
		t.Fatalf("over the cap: kvs %d issues %v", len(s.kvs), s.issues)
	}
}

// An FQDN object without addresses renders nothing (status EMPTY) and warns; ipVersion pins the family.
func TestACLFQDNUnresolvedAndFamilies(t *testing.T) {
	withEnv(t, ACLEnv{Owner: "w3"})
	ds := &vrxv1.DesiredState{
		Objects: &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{
			"cdn":   {Type: proto.String("fqdn"), Fqdn: proto.String("cdn.w3.test")},
			"dual":  {Type: proto.String("host"), Address: proto.String("192.0.2.1")},
			"dual6": {Type: proto.String("host"), Address: proto.String("2001:db8::1")},
		}, AddressGroups: map[string]*vrxv1.AddressGroup{"both": {Members: []string{"dual", "dual6"}}}},
		Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"l": {Rules: []*vrxv1.AclRule{
			rule(1, "permit", func(r *vrxv1.AclRule) { r.Destination = object("cdn") }),
			rule(2, "permit", func(r *vrxv1.AclRule) { r.Destination = object("both"); r.IpVersion = proto.String("ipv6") }),
		}}}},
	}
	rec := aclstate.NewRecord(0)
	withEnv(t, ACLEnv{Owner: "w3", Record: rec})
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true, "objects": true})
	if len(s.issues) != 1 || !strings.Contains(s.issues[0], "W /acl/lists/l/rules/0 acl.fqdn-unresolved") {
		t.Fatalf("issues %v", s.issues)
	}
	a, _ := descacl.FromProto(s.value(t, descacl.KeyACL("l")))
	if len(a.Rules) != 1 || a.Rules[0].Dst != "2001:db8::1/128" || a.Rules[0].Src != descacl.AnyV6 {
		t.Fatalf("rules %+v", a.Rules)
	}
	exp, ok := rec.ACL("l", aclstate.Fingerprint(a.Rules))
	if !ok || exp.Rules[0].Status != vrxv1.AclRuleStatus_ACL_RULE_STATUS_EMPTY || strings.Join(exp.Rules[0].FQDN, ",") != "cdn" || exp.Rules[1].Count != 1 {
		t.Fatalf("record %+v", exp)
	}
}
