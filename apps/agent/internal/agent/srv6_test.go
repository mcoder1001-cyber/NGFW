package agent

// F-srv6: routing.srv6 through the agent (projection → DF-6 sr descriptors → coretest's VPP sr model →
// Retrieve), the restart simulation, the delete order that leaves no SR FIB entry in a deleted table
// (V15/V22a), validation failures with pointers, the D-071 globals split and the Srv6State RPC.

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	srapi "ngfw/agent/binapi/sr"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// newSrv6Svc is newSvc with the D-071 globals flag.
func newSrv6Svc(t *testing.T, v *coretest.VPP, dir string, globalsOwner bool) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globalsOwner})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc
}

// srv6Doc is an L3VPN-style SRv6 document in canonical form (every default spelled out, steering in
// Retrieve order) — Retrieve must return routing.srv6 exactly.
const srv6Doc = `{
  "vrfs": {"cust": {"id": 7001}},
  "interfaces": {"loop701": {"ipv6": ["2001:db8:7::1/64"]}, "loop702": {}},
  "routing": {"srv6": {
    "localSids": {
      "fd00:7:ff::1": {"behavior": "end", "psp": true, "vrf": "default"},
      "fd00:7:ff::2": {"behavior": "end.x", "psp": false, "vrf": "default", "interface": "loop701", "nextHop": "2001:db8:7::2"},
      "fd00:7:ff::a": {"behavior": "end.dt4", "psp": false, "vrf": "default", "lookupVrf": "cust"},
      "fd00:7:ff::b": {"behavior": "end.dt6", "psp": false, "vrf": "cust", "lookupVrf": "cust"},
      "fd00:7:ff::c": {"behavior": "end.dx2", "psp": false, "vrf": "default", "interface": "loop702"}
    },
    "policies": {
      "fd00:7:bb::1": {"type": "default", "encap": true, "vrf": "default", "encapSource": "fd00:7::1",
        "sidLists": [{"sids": ["fd00:7:ee::1", "fd00:7:ee::a"], "weight": 1}, {"sids": ["fd00:7:ee::2"], "weight": 3}]},
      "fd00:7:bb::2": {"type": "spray", "encap": false, "vrf": "cust", "sidLists": [{"sids": ["fd00:7:ee::3"], "weight": 1}]}
    },
    "steering": [
      {"type": "l3", "prefix": "10.7.100.0/24", "vrf": "cust", "bsid": "fd00:7:bb::1"},
      {"type": "l3", "prefix": "fd00:7:100::/48", "vrf": "cust", "bsid": "fd00:7:bb::2"},
      {"type": "l2", "interface": "loop702", "bsid": "fd00:7:bb::1"}
    ]
  }}
}`

func retrieveSrv6(t *testing.T, s *Service) *vrxv1.Srv6Config {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetRouting().GetSrv6()
}

func mustEqualSrv6(t *testing.T, got, want *vrxv1.Srv6Config) {
	t.Helper()
	if !proto.Equal(got, want) {
		t.Fatalf("routing.srv6:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
}

func srCalls(v *coretest.VPP) []string {
	var out []string
	for _, c := range v.Calls() {
		switch m := c.(type) {
		case *srapi.SrSteeringAddDel:
			out = append(out, map[bool]string{true: "steer-del", false: "steer-add"}[m.IsDel])
		case *srapi.SrPolicyAddV2:
			out = append(out, "policy-add")
		case *srapi.SrPolicyDel:
			out = append(out, "policy-del")
		case *srapi.SrLocalsidAddDel:
			out = append(out, map[bool]string{true: "sid-del", false: "sid-add"}[m.IsDel])
		case *ip.IPTableAddDel:
			if !m.IsAdd {
				out = append(out, "table-del")
			}
		}
	}
	return out
}

func noSRDamage(t *testing.T, v *coretest.VPP) {
	t.Helper()
	if c := v.SR().Crashes(); len(c) > 0 {
		t.Fatalf("calls that crash VPP 26.06: %v", c)
	}
	if l := v.SR().Leaks(); len(l) > 0 {
		t.Fatalf("leaked SR FIB entries (V15): %v", l)
	}
}

// TestSrv6ApplyRetrieveRollback: after commit Retrieve() == desired; an idempotent re-apply writes
// nothing; the rollback deletes steering → policies → SIDs before the VRF, with no leaked SR entry.
func TestSrv6ApplyRetrieveRollback(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	want := doc(t, srv6Doc)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: want})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), "sr.") && (r.GetSubsystem() != "routing" || !strings.HasPrefix(r.GetPointer(), "/routing/srv6/")) {
			t.Fatalf("result %v", r)
		}
	}
	if sids, pols, st := v.SR().Counts(); sids != 5 || pols != 2 || st != 3 {
		t.Fatalf("vpp has %d sids, %d policies, %d steering", sids, pols, st)
	}
	mustEqualSrv6(t, retrieveSrv6(t, s), want.GetRouting().GetSrv6())
	t.Logf("apply: %s; Retrieve routing.srv6 == desired", protojson.MarshalOptions{}.Format(resp.GetSummary()))
	noSRDamage(t, v)

	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, srv6Doc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("idempotent apply changed %v", resp.GetResults())
	}
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("idempotent apply sent %s", n)
		}
	}

	// Rollback to a document without SRv6 and without the VRF (routing and vrfs authoritative, empty).
	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, `{"interfaces": {"loop701": {"ipv6": ["2001:db8:7::1/64"]}, "loop702": {}}}`), Subsystems: []string{"interfaces", "vrfs", "routing"}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	calls := strings.Join(srCalls(v), " ")
	t.Logf("rollback VPP calls: %s", calls)
	lastSteer, firstPolicy := strings.LastIndex(calls, "steer-del"), strings.Index(calls, "policy-del")
	lastSR := max(strings.LastIndex(calls, "policy-del"), strings.LastIndex(calls, "sid-del"))
	if lastSteer < 0 || firstPolicy < lastSteer || strings.Index(calls, "table-del") < lastSR {
		t.Fatalf("delete order (steering → policies/SIDs → VRF): %s", calls)
	}
	if sids, pols, st := v.SR().Counts(); sids+pols+st != 0 {
		t.Fatalf("left in vpp: %d sids, %d policies, %d steering", sids, pols, st)
	}
	if got := retrieveSrv6(t, s); got != nil {
		t.Fatalf("retrieve after rollback: %s", protojson.Format(got))
	}
	if v.HasTable(7001, true) || v.HasTable(7001, false) {
		t.Fatal("VRF table not deleted")
	}
	noSRDamage(t, v)
}

// TestSrv6RestartSimulation: an agent restart re-adds nothing it claimed (claims persisted in
// claims-df6-<owner>.json); SR objects lost behind the agent's back come back on the next resync.
func TestSrv6RestartSimulation(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	want := doc(t, srv6Doc)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: want}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()

	v.Reset()
	s2 := newSvc(t, v, dir)
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if c := srCalls(v); len(c) != 0 {
		t.Fatalf("restart re-added claimed objects: %v", c)
	}
	mustEqualSrv6(t, retrieveSrv6(t, s2), want.GetRouting().GetSrv6())
	s2.Close()

	// Simulated loss while the agent is down (steering first, as VPP would need).
	v.SR().DeleteAll()
	v.Reset()
	s3 := newSvc(t, v, dir)
	resp = s3.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var created int
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), "sr.") && r.GetOp() == vrxv1.ApplyOperation_APPLY_OPERATION_CREATE {
			created++
		}
	}
	if created != 10 {
		t.Fatalf("resync re-created %d SR objects, want 10: %v", created, resp.GetResults())
	}
	mustEqualSrv6(t, retrieveSrv6(t, s3), want.GetRouting().GetSrv6())
	noSRDamage(t, v)
}

// TestSrv6NeverTakesOverForeignSid: a SID another owner holds fails the transaction (ErrNotOurs),
// the rollback removes what was created, the foreign SID stays.
func TestSrv6NeverTakesOverForeignSid(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	v.SR().AddForeignLocalSid("fd00:7:ff::c")
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, srv6Doc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	var failed bool
	for _, r := range resp.GetResults() {
		if r.GetKey() == "sr.localsid/fd00:7:ff::c" && r.GetCode() == vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_FAILED {
			failed = r.GetPointer() == "/routing/srv6/localSids/fd00:7:ff::c" && strings.Contains(r.GetMessage(), "not ours")
		}
	}
	if !failed {
		t.Fatalf("results %v", resp.GetResults())
	}
	if sids, pols, st := v.SR().Counts(); sids != 1 || pols+st != 0 {
		t.Fatalf("after rollback: %d sids (want only the foreign one), %d policies, %d steering", sids, pols, st)
	}
	if got := retrieveSrv6(t, s); got != nil {
		t.Fatalf("foreign SID reported: %s", protojson.Format(got))
	}
	noSRDamage(t, v)
}

// TestSrv6Validation: the agent's own gate (a document that bypassed the API) — encap without a
// source, a non-canonical SID, missing behaviour fields, a steering entry to an unknown policy.
func TestSrv6Validation(t *testing.T) {
	cases := map[string]struct{ doc, pointer, rule string }{
		"encap without source (D-074)": {`{"routing": {"srv6": {"policies": {"fd00:7:bb::1": {"sidLists": [{"sids": ["fd00:7:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:7:bb::1/encapSource", "routing.srv6-encap-source"},
		"insert with source": {`{"routing": {"srv6": {"policies": {"fd00:7:bb::1": {"encap": false, "encapSource": "fd00:7::1", "sidLists": [{"sids": ["fd00:7:ee::1"]}]}}}}}`,
			"/routing/srv6/policies/fd00:7:bb::1/encapSource", "routing.srv6-encap-source"},
		"non-canonical SID": {`{"routing": {"srv6": {"localSids": {"FD00:7:ff::1": {"behavior": "end"}}}}}`,
			"/routing/srv6/localSids/FD00:7:ff::1", "routing.srv6-canonical"},
		"end.x without next hop": {`{"interfaces": {"loop701": {}}, "routing": {"srv6": {"localSids": {"fd00:7:ff::2": {"behavior": "end.x", "interface": "loop701"}}}}}`,
			"/routing/srv6/localSids/fd00:7:ff::2/nextHop", "routing.srv6-behavior-fields"},
		"end.ad proxy": {`{"routing": {"srv6": {"localSids": {"fd00:7:ff::3": {"behavior": "end.ad"}}}}}`,
			"/routing/srv6/localSids/fd00:7:ff::3/behavior", "routing.srv6"},
		"missing VRF": {`{"routing": {"srv6": {"localSids": {"fd00:7:ff::4": {"behavior": "end.dt4", "lookupVrf": "nope"}}}}}`,
			"/routing/srv6/localSids/fd00:7:ff::4/lookupVrf", "routing.srv6-vrf-exists"},
		"steering without policy": {`{"routing": {"srv6": {"steering": [{"type": "l3", "prefix": "10.7.0.0/16", "bsid": "fd00:7:bb::9"}]}}}`,
			"/routing/srv6/steering/0/bsid", "routing.srv6-steering-bsid"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			v := coretest.New()
			s := newSvc(t, v, t.TempDir())
			resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, c.doc)})
			mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
			var hit bool
			for _, e := range resp.GetValidation().GetErrors() {
				hit = hit || (e.GetPointer() == c.pointer && e.GetRule() == c.rule)
			}
			if !hit {
				t.Fatalf("validation %v", resp.GetValidation().GetErrors())
			}
			if sids, pols, st := v.SR().Counts(); sids+pols+st != 0 {
				t.Fatal("a failed validation touched VPP")
			}
		})
	}
}

// TestSrv6GlobalsNonOwner (D-071): a slot agent never sets the VPP-wide encap source / hop limit; the
// write-only require variant fails the transaction, the globals stay untouched.
func TestSrv6GlobalsNonOwner(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"routing": {"srv6": {"encapSource": "fd00:7::1", "encapHopLimit": 32}}}`)})
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || !strings.Contains(protojson.Format(resp), "globals owner") {
		t.Fatalf("non-owner applied a global: %s", protojson.Format(resp))
	}
	if src, hl := v.SR().Globals(); src != "::" || hl != 64 {
		t.Fatalf("globals changed: %s %d", src, hl)
	}
}

// TestSrv6GlobalsOwner: the globals owner sets both globals, a policy that inherits the global source
// comes back without an encapSource of its own, DryRun notes the write-only leaves (drift skips them),
// and removing them restores VPP's defaults.
func TestSrv6GlobalsOwner(t *testing.T) {
	v := coretest.New()
	s := newSrv6Svc(t, v, t.TempDir(), true)
	const d = `{"routing": {"srv6": {"encapSource": "fd00:7::1", "encapHopLimit": 32,
	  "policies": {
	    "fd00:7:bb::1": {"type": "default", "encap": true, "vrf": "default", "sidLists": [{"sids": ["fd00:7:ee::1"], "weight": 1}]},
	    "fd00:7:bb::2": {"type": "default", "encap": true, "vrf": "default", "encapSource": "fd00:7::2", "sidLists": [{"sids": ["fd00:7:ee::2"], "weight": 1}]}}}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, d)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if src, hl := v.SR().Globals(); src != "fd00:7::1" || hl != 32 {
		t.Fatalf("globals %s %d", src, hl)
	}
	want := proto.Clone(doc(t, d).GetRouting().GetSrv6()).(*vrxv1.Srv6Config)
	want.EncapSource, want.EncapHopLimit = nil, nil // write-only: never retrieved
	mustEqualSrv6(t, retrieveSrv6(t, s), want)

	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, d)})
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string]bool{}
	for _, e := range rep.GetErrors() {
		if e.GetRule() == "agent.unsupported-field" && e.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
			notes[e.GetPointer()] = true
		}
	}
	if !rep.GetOk() || !notes["/routing/srv6/encapSource"] || !notes["/routing/srv6/encapHopLimit"] {
		t.Fatalf("dry run %v", rep.GetErrors())
	}

	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, `{}`), Subsystems: []string{"routing"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if src, hl := v.SR().Globals(); src != "::" || hl != 64 {
		t.Fatalf("globals not reset: %s %d", src, hl)
	}
	noSRDamage(t, v)
}

// TestSrv6State: the claimed objects with their counters, sorted; another owner's SID is not
// reported; owner mismatch and a busy walk are refused (D-132).
func TestSrv6State(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, srv6Doc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.SR().AddForeignLocalSid("fd00:3:ff::1")
	v.SR().SetCounters("fd00:7:ff::a", coretest.SRCounters{GoodPackets: 10, GoodBytes: 1000, BadPackets: 1, BadBytes: 64})
	ctx := context.Background()
	st, err := s.Srv6State(ctx, &vrxv1.Srv6StateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var sids []string
	for _, l := range st.GetLocalSids() {
		sids = append(sids, l.GetSid())
	}
	if strings.Join(sids, ",") != "fd00:7:ff::1,fd00:7:ff::2,fd00:7:ff::a,fd00:7:ff::b,fd00:7:ff::c" {
		t.Fatalf("local sids %v", sids)
	}
	dt4 := st.GetLocalSids()[2]
	if dt4.GetBehavior() != "end.dt4" || dt4.GetLookupTable() != 7001 || dt4.GetGoodPackets() != 10 || dt4.GetGoodBytes() != 1000 || dt4.GetBadPackets() != 1 || dt4.GetBadBytes() != 64 {
		t.Fatalf("end.dt4 %v", dt4)
	}
	if x := st.GetLocalSids()[1]; x.GetInterface() != "loop701" || x.GetNextHop() != "2001:db8:7::2" {
		t.Fatalf("end.x %v", x)
	}
	if len(st.GetPolicies()) != 2 || st.GetPolicies()[0].GetEncapSource() != "fd00:7::1" || st.GetPolicies()[1].GetType() != "spray" || st.GetPolicies()[1].GetFibTable() != 7001 {
		t.Fatalf("policies %v", st.GetPolicies())
	}
	var steer []string
	for _, x := range st.GetSteering() {
		steer = append(steer, x.GetTrafficType()+":"+x.GetPrefix()+x.GetInterface())
	}
	if strings.Join(steer, ",") != "ipv4:10.7.100.0/24,ipv6:fd00:7:100::/48,l2:loop702" {
		t.Fatalf("steering %v", steer)
	}
	if st.GetOwner() != testOwner || st.GetRetrievedAt() == nil {
		t.Fatalf("meta %v", st)
	}
	if _, err := s.Srv6State(ctx, &vrxv1.Srv6StateRequest{Owner: "w3"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("owner mismatch: %v", err)
	}
	// D-132: a second walk while one is in flight gets UNAVAILABLE after the bounded wait.
	prev := srv6WalkWait
	srv6WalkWait = 20 * time.Millisecond
	t.Cleanup(func() { srv6WalkWait = prev })
	srv6Walk <- struct{}{}
	_, err = s.Srv6State(ctx, &vrxv1.Srv6StateRequest{})
	<-srv6Walk
	if grpcCode(err) != codes.Unavailable {
		t.Fatalf("busy walk: %v", err)
	}
	v.SetConnected(false)
	if _, err := s.Srv6State(ctx, &vrxv1.Srv6StateRequest{}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected: %v", err)
	}
}
