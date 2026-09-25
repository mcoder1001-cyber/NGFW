package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// F-acl: the acl domain end to end on the fake VPP — projection (objects from the request, families,
// disabled / inactive-schedule rules, ICMP per family, MACIP any → v4 + v6), Apply → Retrieve ==
// desired, AclState (paging, sequence filter, counters mapped back to configuration rules,
// bindings with another owner's ACL kept first, D-066), a restart after the ACL was deleted behind
// the agent's back, rollback to an empty domain, DryRun findings, and the D-071 globals flag.

const aclJSON = `{
  "interfaces": {"loop701": {"ipv4": ["10.7.1.1/24"]}},
  "objects": {
    "addresses": {
      "web1": {"type": "host", "address": "192.0.2.10"},
      "web2": {"type": "host", "address": "192.0.2.11"},
      "web6": {"type": "host", "address": "2001:db8::10"}
    },
    "addressGroups": {"web-servers": {"members": ["web1", "web2", "web6"]}},
    "services": {"https": {"protocol": "tcp", "destinationPorts": ["443"]}},
    "schedules": {
      "always": {"type": "once", "start": "2020-01-01T00:00:00Z", "end": "2099-01-01T00:00:00Z"},
      "past":   {"type": "once", "start": "2020-01-01T00:00:00Z", "end": "2020-01-02T00:00:00Z"}
    },
    "zones": {"lan": {"interfaces": ["loop701"]}}
  },
  "acl": {
    "lists": {
      "web-in": {"description": "web", "tags": [], "rules": [
        {"sequence": 30, "action": "reflect", "enabled": true, "ipVersion": "any", "source": {"kind": "prefix", "prefix": "10.7.1.0/24"}, "log": false},
        {"sequence": 10, "action": "permit", "enabled": true, "ipVersion": "any", "destination": {"kind": "object", "name": "web-servers"}, "service": {"kind": "object", "name": "https"}, "schedule": "always", "log": true},
        {"sequence": 20, "action": "deny", "enabled": false, "ipVersion": "any"},
        {"sequence": 40, "action": "permit", "enabled": true, "ipVersion": "any", "service": {"kind": "inline", "spec": {"protocol": "icmp", "type": 8}}},
        {"sequence": 50, "action": "permit", "enabled": true, "ipVersion": "any", "schedule": "past"},
        {"sequence": 60, "action": "deny", "enabled": true, "ipVersion": "ipv6"}
      ]}
    },
    "macip": {"l2": {"tags": [], "rules": [{"sequence": 10, "action": "permit", "sourceMac": "02:00:00:00:00:01", "sourceMacMask": "ff:ff:ff:ff:ff:ff"}]}},
    "attachments": [{"list": "web-in", "target": {"kind": "zone", "zone": "lan"}, "direction": "in", "sequence": 1, "enabled": true}],
    "macipAttachments": [{"list": "l2", "interface": "loop701", "enabled": true}]
  }
}`

// newACLSvc is newSvc with the D-071 globals flag, the resync hook and the wiring closed at the end.
func newACLSvc(t *testing.T, v *coretest.VPP, dir string, globals bool) (*Service, *aclstate.Runtime) {
	t.Helper()
	t.Setenv(subsystems.EnvDNSServers, "127.0.0.1:9")
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	var svc *Service
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globals,
		Resync: func() {
			if svc != nil {
				svc.Resync(context.Background())
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err = NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	rt := aclstate.RuntimeFor(dir, testOwner)
	if rt == nil {
		t.Fatal("acl runtime not registered")
	}
	rt.SetStats(v.ACL().Stats)
	t.Cleanup(func() {
		svc.Close()
		w.Close()
		if o := objects.RuntimeFor(dir, testOwner); o != nil {
			o.Close()
		}
	})
	return svc, rt
}

func retrieveACL(t *testing.T, s *Service) *vrxv1.AclConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"acl"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetAcl()
}

// ownedACL returns the index of this owner's ACL called name in the fake VPP.
func ownedACL(t *testing.T, v *coretest.VPP, name string) uint32 {
	t.Helper()
	for idx, tag := range v.ACL().ACLs() {
		if tag == testOwner+":"+name {
			return idx
		}
	}
	t.Fatalf("no ACL %s in VPP: %v", name, v.ACL().ACLs())
	return 0
}

func loopIndex(t *testing.T, v *coretest.VPP, name string) uint32 {
	t.Helper()
	i, ok := v.InterfaceByName(name)
	if !ok {
		t.Fatalf("no interface %s", name)
	}
	return i.Index
}

func TestACLDomainOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s, rt := newACLSvc(t, v, dir, false)
	if !strings.Contains(strings.Join(s.Health().GetSubsystems(), ","), "acl") {
		t.Fatalf("acl not in Health.subsystems: %v", s.Health().GetSubsystems())
	}
	want := doc(t, aclJSON)

	// DryRun first: the log flag is reported (VPP cannot log per rule); nothing is applied
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: want})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %v", err, rep)
	}
	var logWarn bool
	for _, e := range rep.GetErrors() {
		if e.GetRule() == "acl.log-unsupported" && e.GetPointer() == "/acl/lists/web-in/rules/1/log" {
			logWarn = true
		}
	}
	if !logWarn {
		t.Fatalf("no acl.log-unsupported warning: %v", rep.GetErrors())
	}
	if len(v.ACL().ACLs()) != 0 {
		t.Fatal("dry run applied something")
	}

	// a foreign owner's ACL is already on the interface once it exists (D-066): created with the loopback
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", DesiredState: want})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	ptrs := map[string]string{}
	for _, r := range resp.GetResults() {
		ptrs[r.GetKey()] = r.GetPointer()
		if strings.HasPrefix(r.GetKey(), "acl.") && r.GetSubsystem() != "acl" {
			t.Fatalf("subsystem of %s = %q", r.GetKey(), r.GetSubsystem())
		}
	}
	for k, p := range map[string]string{
		"acl.acl/web-in":                      "/acl/lists/web-in",
		"acl.macip-acl/l2":                    "/acl/macip/l2",
		"acl.interface-binding/loop701":       "/acl/attachments/0",
		"acl.macip-interface-binding/loop701": "/acl/macipAttachments/0",
	} {
		if ptrs[k] != p {
			t.Fatalf("result %s pointer %q, want %q (all: %v)", k, ptrs[k], p, ptrs)
		}
	}
	if _, ok := ptrs[string(descacl.KeyStatsEnable)]; ok {
		t.Fatal("a slot agent (not the globals owner) projected acl.stats-enable (D-071)")
	}

	// the expansion in VPP, in sequence order:
	//   10 permit any → 192.0.2.10/31 tcp/443 (v4) and any → 2001:db8::10/128 tcp/443 (v6)
	//   20 disabled, 30 reflect 10.7.1.0/24 → any (v4 only: the prefix pins the family)
	//   40 ICMP echo: v4 only (icmp has no v6 rule), 50 schedule in the past: omitted, 60 deny v6 any
	idx := ownedACL(t, v, "web-in")
	rules := v.ACL().Rules(idx)
	if len(rules) != 5 {
		t.Fatalf("VPP rules %d, want 5: %+v", len(rules), rules)
	}
	if rules[0].DstPrefix.Len != 31 || rules[0].Proto != 6 || rules[0].DstportOrIcmpcodeFirst != 443 || rules[1].DstPrefix.Len != 128 ||
		rules[2].IsPermit != acl_types.ACL_ACTION_API_PERMIT_REFLECT || rules[3].Proto != 1 || rules[3].SrcportOrIcmptypeFirst != 8 ||
		rules[4].IsPermit != acl_types.ACL_ACTION_API_DENY || rules[4].SrcPrefix.Address.Af != 1 {
		t.Fatalf("VPP rules: %+v", rules)
	}

	// Retrieve == desired (the acl domain as the API sent it), and a second Apply changes nothing
	if got := retrieveACL(t, s); !proto.Equal(got, want.GetAcl()) {
		t.Fatalf("Retrieve != desired:\n%s\nwant\n%s", protojson.Format(got), protojson.Format(want.GetAcl()))
	}
	again := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a2", DesiredState: want})
	if sm := again.GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("second apply not empty: %v", sm)
	}

	// AclState: counters off in VPP → unavailable with a reason; per-rule mapping known
	st, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{List: "web-in", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if st.GetCountersAvailable() || !strings.Contains(st.GetCountersReason(), "D-071") || st.GetTotal() != 6 || len(st.GetRules()) != 3 ||
		len(st.GetLists()) != 1 || !st.GetLists()[0].GetMappingKnown() || st.GetLists()[0].GetVppRules() != 5 || st.GetLists()[0].GetConfigRules() != 6 {
		t.Fatalf("AclState: %s", protojson.Format(st))
	}
	r := st.GetRules()
	if r[0].GetSequence() != 10 || r[0].GetVppRules() != 2 || r[0].GetStatus() != vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED ||
		r[1].GetSequence() != 20 || r[1].GetStatus() != vrxv1.AclRuleStatus_ACL_RULE_STATUS_DISABLED ||
		r[2].GetSequence() != 30 || r[2].GetFirstVppRule() != 2 {
		t.Fatalf("rule page: %s", protojson.Format(st))
	}
	// counters on (as the globals owner would switch them): hits of VPP rules 0, 1 and 4 map to sequences 10 and 60
	v.ACL().SetCountersEnabled(true)
	rt.ForgetCountersFlag()
	v.ACL().SetHits(idx, 0, 3, 300)
	v.ACL().SetHits(idx, 1, 2, 200)
	v.ACL().SetHits(idx, 4, 7, 700)
	st, err = s.ACLState(context.Background(), &vrxv1.AclStateRequest{List: "web-in", Offset: 1, Limit: 10, Filter: &vrxv1.AclStateFilter{Sequences: []uint32{10, 50, 60}}})
	if err != nil {
		t.Fatal(err)
	}
	if !st.GetCountersAvailable() || st.GetTotal() != 3 || len(st.GetRules()) != 2 || st.GetRules()[0].GetSequence() != 50 ||
		st.GetRules()[0].GetStatus() != vrxv1.AclRuleStatus_ACL_RULE_STATUS_SCHEDULE_INACTIVE || st.GetRules()[1].GetPackets() != 7 ||
		st.GetLists()[0].GetPackets() != 12 || st.GetLists()[0].GetBytes() != 1200 {
		t.Fatalf("counters page: %s", protojson.Format(st))
	}
	st, _ = s.ACLState(context.Background(), &vrxv1.AclStateRequest{List: "web-in", Filter: &vrxv1.AclStateFilter{HitsOnly: true}})
	if st.GetTotal() != 2 || st.GetRules()[0].GetSequence() != 10 || st.GetRules()[0].GetPackets() != 5 || st.GetRules()[0].GetBytes() != 500 {
		t.Fatalf("hits only: %s", protojson.Format(st))
	}
	if _, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{List: "nope"}); grpcCode(err) != codes.NotFound {
		t.Fatalf("unknown list: %v", err)
	}
	if _, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{Limit: 1001}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("limit: %v", err)
	}
	if _, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{Owner: "w9"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("owner: %v", err)
	}

	// another owner's ACL on the same interface: planted first in the input list; our next Apply
	// (a changed rule) keeps it first and ours after it (D-066)
	lo := loopIndex(t, v, "loop701")
	foreign := v.ACL().AddACL("w9:foreign", acl_types.ACLRule{IsPermit: acl_types.ACL_ACTION_API_PERMIT})
	v.ACL().Bind(lo, 2, foreign, idx)
	changed := proto.Clone(want).(*vrxv1.DesiredState)
	changed.Acl.Lists["web-in"].Rules[5].Action = proto.String("permit")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a3", DesiredState: changed}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n, acls := v.ACL().Binding(lo); n != 2 || len(acls) != 2 || acls[0] != foreign || acls[1] != idx {
		t.Fatalf("binding after update: n_input %d %v (foreign %d, ours %d)", n, acls, foreign, idx)
	}
	st, err = s.ACLState(context.Background(), &vrxv1.AclStateRequest{IncludeInterfaces: true})
	if err != nil || len(st.GetInterfaces()) != 1 {
		t.Fatalf("interfaces: %v %v", err, st)
	}
	in := st.GetInterfaces()[0]
	if in.GetInterface() != "loop701" || len(in.GetInput()) != 2 || !in.GetInput()[0].GetForeign() || in.GetInput()[0].GetTag() != "w9:foreign" ||
		in.GetInput()[1].GetName() != "web-in" || in.GetMacip().GetName() != "l2" || len(st.GetMacipLists()) != 1 || st.GetMacipLists()[0].GetVppRules() != 2 {
		t.Fatalf("interfaces: %s", protojson.Format(st))
	}

	// restart safety: the agent stops, our ACL is unbound and deleted behind its back (the foreign one
	// stays), a new agent on the same state dir resyncs and rebuilds it — foreign still first
	s.Close()
	v.ACL().Bind(lo, 1, foreign)
	if !v.ACL().DeleteACL(idx) {
		t.Fatal("delete behind the back")
	}
	s2, _ := newACLSvc(t, v, dir, false)
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync after loss: %v", r)
	}
	idx2 := ownedACL(t, v, "web-in")
	if n, acls := v.ACL().Binding(lo); n != 2 || len(acls) != 2 || acls[0] != foreign || acls[1] != idx2 {
		t.Fatalf("binding after resync: %d %v", n, acls)
	}
	if got := retrieveACL(t, s2); !proto.Equal(got, changed.GetAcl()) {
		t.Fatalf("Retrieve after restart:\n%s", protojson.Format(got))
	}

	// rollback to "no ACLs": ours are unbound and deleted, the foreign ACL and its binding stay
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "a4", DesiredState: doc(t, `{"interfaces": {"loop701": {"ipv4": ["10.7.1.1/24"]}}, "objects": {}, "acl": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := retrieveACL(t, s2); got != nil {
		t.Fatalf("acl left after rollback: %s", protojson.Format(got))
	}
	if left := v.ACL().ACLs(); len(left) != 1 || left[foreign] != "w9:foreign" {
		t.Fatalf("VPP ACLs after rollback: %v", left)
	}
	if n, acls := v.ACL().Binding(lo); n != 1 || len(acls) != 1 || acls[0] != foreign {
		t.Fatalf("binding after rollback: %d %v", n, acls)
	}
	if _, ok := v.ACL().MacipBinding(lo); ok {
		t.Fatal("MACIP binding left")
	}
}

// DryRun findings of the projection: an object reference in a transaction without objects, an
// expansion above the per-rule cap (sources × destinations), unknown schedule.
func TestACLDryRunFindings(t *testing.T) {
	s, _ := newACLSvc(t, coretest.New(), t.TempDir(), false)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{Subsystems: []string{"acl"}, DesiredState: doc(t, `{"acl": {"lists": {"x": {"rules": [
	  {"sequence": 1, "action": "permit", "source": {"kind": "object", "name": "web"}}]}}}}`)})
	if err != nil || rep.GetOk() || !hasIssue(rep, "/acl/lists/x/rules/0/source/name", "acl.objects-required") {
		t.Fatalf("objects-required: %v %v", err, rep)
	}
	// 101 × 100 non-adjacent hosts: 10 100 VPP rules for one configuration rule
	var a, b []string
	addrs := map[string]*vrxv1.AddressObject{}
	for i := 0; i < 101; i++ {
		n := "a" + itoa3(i)
		addrs[n] = &vrxv1.AddressObject{Type: proto.String("host"), Address: proto.String("10.1." + strconv.Itoa(i) + ".1")}
		a = append(a, n)
		if i < 100 {
			m := "b" + itoa3(i)
			addrs[m] = &vrxv1.AddressObject{Type: proto.String("host"), Address: proto.String("10.2." + strconv.Itoa(i) + ".1")}
			b = append(b, m)
		}
	}
	ds := &vrxv1.DesiredState{
		Objects: &vrxv1.ObjectsConfig{Addresses: addrs, AddressGroups: map[string]*vrxv1.AddressGroup{"ga": {Members: a}, "gb": {Members: b}}},
		Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"big": {Rules: []*vrxv1.AclRule{
			{Sequence: proto.Uint32(1), Action: proto.String("permit"), Source: &vrxv1.AddressMatch{Kind: proto.String("object"), Name: proto.String("ga")}, Destination: &vrxv1.AddressMatch{Kind: proto.String("object"), Name: proto.String("gb")}},
			{Sequence: proto.Uint32(2), Action: proto.String("permit"), Schedule: proto.String("missing")},
		}}}},
	}
	rep, err = s.DryRun(context.Background(), &vrxv1.DryRunRequest{Subsystems: []string{"objects", "acl"}, DesiredState: ds})
	if err != nil || rep.GetOk() || !hasIssue(rep, "/acl/lists/big/rules/0", "acl.expansion-limit") || !hasIssue(rep, "/acl/lists/big/rules/1/schedule", "acl.rule-references") {
		t.Fatalf("expansion limit / schedule: %v %v", err, rep)
	}
}

// The D-071 globals owner projects acl.stats-enable (the counters switch) with the ACLs.
func TestACLStatsEnableGlobalsOwner(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), true)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, `{"acl": {"lists": {"x": {"rules": [{"sequence": 1, "action": "deny", "ipVersion": "ipv4"}]}}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var got bool
	for _, r := range resp.GetResults() {
		if r.GetKey() == string(descacl.KeyStatsEnable) && r.GetPointer() == "/acl" {
			got = true
		}
	}
	if !got || !v.ACL().CountersEnabled() {
		t.Fatalf("stats-enable not applied (results %v)", resp.GetResults())
	}
	st, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{})
	if err != nil || !st.GetCountersAvailable() {
		t.Fatalf("counters: %v %v", err, st)
	}
}

func hasIssue(rep *vrxv1.ValidationReport, pointer, rule string) bool {
	for _, e := range rep.GetErrors() {
		if e.GetPointer() == pointer && e.GetRule() == rule {
			return true
		}
	}
	return false
}

func itoa3(i int) string { return fmt.Sprintf("%03d", i) }
