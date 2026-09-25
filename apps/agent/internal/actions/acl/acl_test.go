package acl

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/acl_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
)

func info(seq, first, count uint32, st vrxv1.AclRuleStatus) RuleInfo {
	return RuleInfo{Sequence: seq, First: first, Count: count, Status: st}
}

const (
	applied  = vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED
	disabled = vrxv1.AclRuleStatus_ACL_RULE_STATUS_DISABLED
)

// Counters of VPP rules map back to configuration rules through the expansion blocks; paging,
// the sequence filter and hits-only work on configuration rules.
func TestRulePageMapsCounters(t *testing.T) {
	exp := &Expansion{Rules: []RuleInfo{
		info(10, 0, 2, applied), info(20, 2, 0, disabled), info(30, 2, 3, applied), info(40, 5, 1, applied),
	}}
	c := []descacl.RuleCounter{{Packets: 1, Bytes: 10}, {Packets: 2, Bytes: 20}, {}, {}, {Packets: 4, Bytes: 40}, {}, {Packets: 99}} // + VPP's spare slot
	page, total := RulePage(exp, c, nil, 0, 10)
	if total != 4 || len(page) != 4 || page[0].GetPackets() != 3 || page[0].GetBytes() != 30 || page[2].GetPackets() != 4 || page[3].GetPackets() != 0 {
		t.Fatalf("page %v total %d", page, total)
	}
	page, total = RulePage(exp, c, &vrxv1.AclStateFilter{HitsOnly: true}, 1, 1)
	if total != 2 || len(page) != 1 || page[0].GetSequence() != 30 {
		t.Fatalf("hits only %v %d", page, total)
	}
	page, total = RulePage(exp, nil, &vrxv1.AclStateFilter{Sequences: []uint32{40, 20}}, 0, 10)
	if total != 2 || page[0].GetSequence() != 20 || page[1].GetSequence() != 40 || page[1].GetPackets() != 0 {
		t.Fatalf("filter %v", page)
	}
	if _, total := RulePage(exp, nil, &vrxv1.AclStateFilter{HitsOnly: true}, 0, 10); total != 0 {
		t.Fatal("hits only without counters must select nothing")
	}
	if p, b := Sum(c, 6); p != 7 || b != 70 {
		t.Fatalf("sum %d %d (the spare slot must not count)", p, b)
	}
}

// The record keeps the newest expansions per list and evicts old ones over the rule budget, never a
// list's newest.
func TestRecordBoundsAndLookup(t *testing.T) {
	r := NewRecord(10)
	mk := func(name, fp string, n int) *Expansion {
		return &Expansion{Name: name, Fingerprint: fp, Rules: make([]RuleInfo, n)}
	}
	r.PutACL(mk("a", "1", 6))
	r.PutACL(mk("a", "2", 6)) // 12 > 10: a/1 evicted
	if _, ok := r.ACL("a", "1"); ok {
		t.Fatal("old record kept over budget")
	}
	r.PutACL(mk("b", "1", 8)) // newest records are never evicted
	if _, ok := r.ACL("a", "2"); !ok {
		t.Fatal("newest a evicted")
	}
	if _, ok := r.ACL("b", "1"); !ok {
		t.Fatal("newest b evicted")
	}
	r.PutACL(mk("b", "1", 8)) // same fingerprint replaces
	if len(r.acls["b"]) != 1 {
		t.Fatalf("duplicate fingerprint kept: %d", len(r.acls["b"]))
	}
	a := &Attachments{Fingerprint: BindingsFingerprint([]descacl.InterfaceBinding{{Interface: "i2", Input: []string{"x"}}, {Interface: "i1", Output: []string{"y"}}}, nil)}
	r.PutAttachments(a)
	if got, ok := r.Attachments(BindingsFingerprint([]descacl.InterfaceBinding{{Interface: "i1", Output: []string{"y"}}, {Interface: "i2", Input: []string{"x"}}}, nil)); !ok || got != a {
		t.Fatal("binding fingerprint must not depend on order")
	}
	if Fingerprint([]descacl.Rule{{Action: "permit", Src: "10.0.0.0/8", Dst: "0.0.0.0/0"}}) == Fingerprint([]descacl.Rule{{Action: "permit", Src: "10.0.0.0/80", Dst: ".0.0.0/0"}}) {
		t.Fatal("fingerprint fields must be length-delimited")
	}
}

// The tracker follows Create/Update/Delete/Retrieve of the wrapped DF-4 descriptors on the fake VPP,
// and State/Interfaces read counters and bindings (another owner's ACL included) without dumping rules.
func TestTrackerAndStateOnFake(t *testing.T) {
	v := coretest.New()
	lo := v.AddInterface("loop3001", "loopback", "w3:loop3001")
	rec := NewRecord(0)
	rt := Open(Config{StateDir: t.TempDir(), Owner: "w3", Client: v, Stats: v.ACL().Stats, Record: rec, Now: time.Now})
	t.Cleanup(rt.Close)
	d := rt.Tracker().WrapACL(descacl.NewACL(v, "w3"))
	ctx := context.Background()
	a := descacl.ACL{Name: "web", Rules: []descacl.Rule{
		{Action: descacl.ActionPermit, Src: descacl.AnyV4, Dst: "192.0.2.0/24", SrcPortLast: 65535, DstPortLast: 65535},
		{Action: descacl.ActionDeny, Src: descacl.AnyV4, Dst: descacl.AnyV4, SrcPortLast: 65535, DstPortLast: 65535},
	}}
	meta, err := d.Create(ctx, a.Proto())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := rt.Tracker().ACL("web")
	if !ok || got.Index != meta.(descacl.Meta).ACLIndex || got.VPPRules != 2 || got.Fingerprint != Fingerprint(a.Rules) {
		t.Fatalf("tracked %+v", got)
	}
	// State without a recorded expansion: the summary is there, the mapping is not
	rec.PutACL(&Expansion{Name: "web", Fingerprint: Fingerprint(a.Rules), VPPRules: 2, Rules: []RuleInfo{info(5, 0, 1, applied), info(7, 1, 1, applied)}})
	v.ACL().SetCountersEnabled(true)
	v.ACL().SetHits(got.Index, 1, 9, 900)
	foreign := v.ACL().AddACL("w9:other", acl_types.ACLRule{})
	v.ACL().Bind(lo, 1, foreign, got.Index)
	st, err := rt.State(ctx, &vrxv1.AclStateRequest{List: "web", IncludeInterfaces: true})
	if err != nil {
		t.Fatal(err)
	}
	if !st.GetCountersAvailable() || st.GetTotal() != 2 || st.GetRules()[1].GetPackets() != 9 || st.GetLists()[0].GetPackets() != 9 {
		t.Fatalf("state %v", st)
	}
	ifs := st.GetInterfaces()
	if len(ifs) != 1 || ifs[0].GetInterface() != "loop3001" || len(ifs[0].GetInput()) != 1 || !ifs[0].GetInput()[0].GetForeign() ||
		ifs[0].GetInput()[0].GetTag() != "w9:other" || len(ifs[0].GetOutput()) != 1 || ifs[0].GetOutput()[0].GetName() != "web" {
		t.Fatalf("interfaces %v", ifs)
	}
	// Update keeps the index and changes the fingerprint; a Retrieve replaces the whole view; Delete forgets
	b := a
	b.Rules = a.Rules[:1]
	if _, err := d.Update(ctx, a.Proto(), b.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	if got2, _ := rt.Tracker().ACL("web"); got2.Index != got.Index || got2.VPPRules != 1 {
		t.Fatalf("after update %+v", got2)
	}
	if st, _ := rt.State(ctx, &vrxv1.AclStateRequest{List: "web"}); st.GetLists()[0].GetMappingKnown() || len(st.GetRules()) != 0 {
		t.Fatalf("mapping must be unknown for content no projection produced: %v", st)
	}
	v.ACL().Bind(lo, 1, foreign)
	if err := d.Delete(ctx, b.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.Tracker().ACL("web"); ok {
		t.Fatal("deleted ACL still tracked")
	}
	v.ACL().AddACL("w3:planted", acl_types.ACLRule{IsPermit: acl_types.ACL_ACTION_API_PERMIT})
	if _, err := d.Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	if acls := rt.Tracker().ACLs(); len(acls) != 1 || acls[0].Name != "planted" {
		t.Fatalf("after retrieve %+v", acls)
	}
	// counters off → unavailable with a reason, no numbers; a broken stats segment is a reason, not an error
	v.ACL().SetCountersEnabled(false)
	rt.ForgetCountersFlag()
	if st, err := rt.State(ctx, &vrxv1.AclStateRequest{}); err != nil || st.GetCountersAvailable() || st.GetCountersReason() == "" {
		t.Fatalf("counters off: %v %v", st, err)
	}
	if _, err := rt.State(ctx, &vrxv1.AclStateRequest{List: "nope"}); err == nil {
		t.Fatal("unknown list must fail")
	}
	if ok, err := ReadCountersFlag(ctx, v); err != nil || ok {
		t.Fatalf("flag %v %v", ok, err)
	}
}
