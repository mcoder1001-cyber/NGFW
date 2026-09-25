package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// newLispSvc is newSvc with the D-071 globals flag.
func newLispSvc(t *testing.T, v *coretest.VPP, dir string, globals bool) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globals})
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

// lispDoc is the full example (packages/proto/test/fixtures/lisp-full.json, on the w11 slot numbers)
// with loopbacks as the RLOC interface (the unit-test model has no af_packet netdevs to spare).
const lispDoc = `{
  "vrfs": {"overlay": {"id": 1100}},
  "interfaces": {
    "loop1101": {"ipv4": ["10.11.1.1/24"]},
    "loop1100": {"vrf": "overlay", "ipv4": ["10.11.100.1/24"]}
  },
  "tunnels": {"lisp": {
    "enabled": true, "gpe": true,
    "locatorSets": {"w11-rloc": {"locators": [{"interface": "loop1101", "priority": 1, "weight": 1}]}},
    "localEids": [{"vni": 1100, "eid": "10.11.100.0/24", "locatorSet": "w11-rloc"}],
    "eidTables": {"1100": {"vrf": "overlay"}},
    "remoteMappings": [{"vni": 1100, "eid": "10.11.200.0/24", "rlocs": [{"address": "10.11.1.2", "priority": 1, "weight": 1}], "action": "no-action"}],
    "adjacencies": [{"vni": 1100, "reid": "10.11.200.0/24", "leid": "10.11.100.0/24"}],
    "gpeEntries": [{"vni": 1101, "vrf": "overlay", "reid": "10.11.201.0/24", "leid": "10.11.101.0/24",
      "pairs": [{"local": "10.11.1.1", "remote": "10.11.1.3", "weight": 1}], "action": "no-action"}],
    "mapResolvers": ["10.11.1.254"],
    "mapServers": ["10.11.1.253"],
    "pitr": "w11-rloc"
  }}
}`

// lispRetrieved is what Retrieve reports for lispDoc: everything but the write-only GPE entry (V13).
const lispRetrieved = `{
  "enabled": true, "gpe": true,
  "locatorSets": {"w11-rloc": {"locators": [{"interface": "loop1101", "priority": 1, "weight": 1}]}},
  "localEids": [{"vni": 1100, "eid": "10.11.100.0/24", "locatorSet": "w11-rloc"}],
  "eidTables": {"1100": {"vrf": "overlay"}},
  "remoteMappings": [{"vni": 1100, "eid": "10.11.200.0/24", "rlocs": [{"address": "10.11.1.2", "priority": 1, "weight": 1}], "action": "no-action"}],
  "adjacencies": [{"vni": 1100, "reid": "10.11.200.0/24", "leid": "10.11.100.0/24"}],
  "mapResolvers": ["10.11.1.254"],
  "mapServers": ["10.11.1.253"],
  "pitr": "w11-rloc"
}`

func lispRetrieve(t *testing.T, s *Service) *vrxv1.LispConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"tunnels"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetTunnels().GetLisp()
}

func changes(r *vrxv1.ApplyResponse) uint32 {
	s := r.GetSummary()
	return s.GetCreated() + s.GetUpdated() + s.GetDeleted() + s.GetFailed()
}

func sent(v *coretest.VPP, name string) int {
	n := 0
	for _, c := range v.Calls() {
		if c.GetMessageName() == name {
			n++
		}
	}
	return n
}

func TestLispFullExampleApplyRetrieveResyncRollback(t *testing.T) {
	v := coretest.New()
	l := v.InstallLisp()
	dir := t.TempDir()
	s := newLispSvc(t, v, dir, true)

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "l1", DesiredState: doc(t, lispDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var keys []string
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), "lisp") {
			keys = append(keys, r.GetKey())
		}
	}
	t.Logf("plan (lisp objects, apply order): %s", strings.Join(keys, " → "))
	if len(keys) != 12 {
		t.Fatalf("want 12 lisp objects (enable, gpe, set, locator, eid-table map, local eid, remote mapping, adjacency, gpe entry, resolver, server, pitr), got %d: %v", len(keys), keys)
	}
	if want := mustLisp(t, lispRetrieved); !proto.Equal(lispRetrieve(t, s), want) {
		t.Fatalf("retrieve:\n got %s\nwant %s", protojson.Format(lispRetrieve(t, s)), protojson.Format(want))
	}

	// Second plan empty.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "l2", DesiredState: doc(t, lispDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if changes(resp) != 0 {
		t.Fatalf("second apply changed something: %v", resp.GetSummary())
	}

	// Agent restart (same VPP boot): the resync converges without re-adding the write-only GPE entry (D-076).
	s.Close()
	before := sent(v, "gpe_add_del_fwd_entry")
	calls := len(v.Calls())
	start := time.Now()
	s2 := newLispSvc(t, v, dir, true)
	resp = s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	t.Logf("restart resync: %s in %v, summary %v", resp.GetStatus(), time.Since(start), resp.GetSummary())
	// The write-only entry is re-asserted from the claim of this VPP boot (counted as created, nothing sent).
	mutating := 0
	for _, c := range v.Calls()[calls:] {
		n := c.GetMessageName()
		if strings.HasPrefix(n, "lisp_add_del") || strings.HasPrefix(n, "gpe_add_del") || strings.HasSuffix(n, "_enable_disable") || n == "lisp_pitr_set_locator_set" || n == "lisp_eid_table_add_del_map" {
			mutating++
		}
	}
	if resp.GetSummary().GetUpdated()+resp.GetSummary().GetDeleted()+resp.GetSummary().GetFailed() != 0 || resp.GetSummary().GetCreated() > 1 || mutating != 0 || sent(v, "gpe_add_del_fwd_entry") != before {
		t.Fatalf("restart resync changed something / re-added the GPE entry: %v (gpe adds %d → %d, lisp writes %d)", resp.GetSummary(), before, sent(v, "gpe_add_del_fwd_entry"), mutating)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("config back after %v (> 30 s)", time.Since(start))
	}
	if !proto.Equal(lispRetrieve(t, s2), mustLisp(t, lispRetrieved)) {
		t.Fatalf("retrieve after restart: %s", protojson.Format(lispRetrieve(t, s2)))
	}

	// Rollback: an empty lisp removes adjacencies → mappings → EIDs → sets → EID-table maps.
	resp = apply(t, s2, &vrxv1.ApplyRequest{TxnId: "l3", DesiredState: doc(t, `{"vrfs": {"overlay": {"id": 1100}},
	  "interfaces": {"loop1101": {"ipv4": ["10.11.1.1/24"]}, "loop1100": {"vrf": "overlay", "ipv4": ["10.11.100.1/24"]}},
	  "tunnels": {}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	pos := map[string]int{}
	var dels []string
	for i, r := range resp.GetResults() {
		if r.GetOp() == vrxv1.ApplyOperation_APPLY_OPERATION_DELETE {
			d := strings.SplitN(r.GetKey(), "/", 2)[0]
			if _, seen := pos[d]; !seen {
				pos[d] = i
			}
			dels = append(dels, r.GetKey())
		}
	}
	t.Logf("rollback deletes: %s", strings.Join(dels, " → "))
	order := []string{"lisp.adjacency", "lisp.remote-mapping", "lisp.local-eid", "lisp.locator", "lisp.locator-set"}
	for i := 1; i < len(order); i++ {
		if pos[order[i-1]] > pos[order[i]] {
			t.Fatalf("delete order: %s after %s (%v)", order[i-1], order[i], dels)
		}
	}
	if pos["lisp.local-eid"] > pos["lisp.eid-table-map"] {
		t.Fatalf("eid-table map deleted before the local EID: %v", dels)
	}
	if got := lispRetrieve(t, s2); got != nil && (len(got.GetLocatorSets()) > 0 || len(got.GetLocalEids()) > 0 || len(got.GetRemoteMappings()) > 0 || len(got.GetAdjacencies()) > 0 || len(got.GetEidTables()) > 0) {
		t.Fatalf("objects left after rollback: %s", protojson.Format(got))
	}
	sets, mappings, adjs, maps, fwd := l.State()
	t.Logf("after rollback: local sets %d, mappings %d, adjacencies %d, eid-table maps %d, gpe entries %d; leaked remote locator sets %d (V14); LISP still on %v (globals kept on absence, D-071)", sets, mappings, adjs, maps, fwd, l.RemoteSets(), l.Enabled)
	if sets+mappings+adjs+maps+fwd != 0 {
		t.Fatalf("VPP still has LISP objects: sets %d mappings %d adjacencies %d maps %d fwd %d", sets, mappings, adjs, maps, fwd)
	}
	if l.RemoteSets() != 1 {
		t.Fatalf("the model should show the V14 leak of the remote mapping's locator set")
	}
}

// A VPP restart loses the write-only GPE entry: the resync on reconnect re-applies it once.
func TestLispGpeEntryReappliedAfterVPPRestart(t *testing.T) {
	v := coretest.New()
	l := v.InstallLisp()
	s := newLispSvc(t, v, t.TempDir(), true)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, lispDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	l.ForgetGpe()
	resp := s.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, _, _, _, fwd := l.State(); fwd != 1 {
		t.Fatalf("GPE entry not re-applied by the resync: %d entries, summary %v", fwd, resp.GetSummary())
	}
}

// An agent that is not the globals owner requires the switches: with LISP off it fails clearly.
func TestLispRequiresGlobalsOnSlotAgents(t *testing.T) {
	v := coretest.New()
	l := v.InstallLisp()
	s := newLispSvc(t, v, t.TempDir(), false)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, lispDoc)})
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatal("applied with LISP off on a non-globals-owner agent")
	}
	if !strings.Contains(protojson.Format(resp), "globals owner") {
		t.Fatalf("no clear globals-owner error: %s", protojson.Format(resp))
	}
	if l.Enabled || sent(v, "lisp_enable_disable") != 0 {
		t.Fatal("a slot agent switched LISP")
	}
	// With LISP (and GPE, PITR) set by the globals owner the same document applies.
	l.Enabled, l.GpeOn = true, true
	pitr := apply(t, newLispSvc(t, v, t.TempDir(), true), &vrxv1.ApplyRequest{TxnId: "r0", DesiredState: doc(t, `{"interfaces": {"loop1101": {"ipv4": ["10.11.1.1/24"]}},
	  "tunnels": {"lisp": {"enabled": true, "gpe": true, "locatorSets": {"w11-rloc": {}}, "pitr": "w11-rloc"}}}`)})
	mustStatus(t, pitr, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
}

func TestLispProjectionWarnsForTunnelKindsNotWired(t *testing.T) {
	p := project(doc(t, `{"tunnels": {"gre": {"g": {"src": "10.0.0.1", "dst": "10.0.0.2"}}}}`), []string{"tunnels"}, nil, nil)
	if len(p.issues) != 1 || p.issues[0].rule != "agent.unsupported-field" || p.issues[0].pointer != "/tunnels/gre" {
		t.Fatalf("issues %+v", p.issues)
	}
	p = project(doc(t, `{"tunnels": {"lisp": {"enabled": true, "localEids": [{"vni": 1, "eid": "10.1.2.0/24", "locatorSet": "x"}]}}}`), []string{"tunnels"}, nil, nil)
	var warned bool
	for _, is := range p.issues {
		warned = warned || is.rule == "tunnels.lisp-gpe-implied"
	}
	if !warned || len(p.kvs) != 2 {
		t.Fatalf("issues %+v kvs %v", p.issues, p.kvs)
	}
	if k := p.kvs[1].Key; k != "lisp.local-eid/1/10.1.2.0/24" {
		t.Fatalf("EID not canonicalised in the key: %s", k)
	}
	// host bits are rejected, not masked (TD-16b, D-149)
	p = project(doc(t, `{"tunnels": {"lisp": {"enabled": true, "localEids": [{"vni": 1, "eid": "10.1.2.3/24", "locatorSet": "x"}]}}}`), []string{"tunnels"}, nil, nil)
	var rejected bool
	for _, is := range p.issues {
		rejected = rejected || (is.rule == "tunnels.lisp-eid-canonical" && is.pointer == "/tunnels/lisp/localEids/0/eid")
	}
	if !rejected {
		t.Fatalf("host-bit EID accepted: issues %+v", p.issues)
	}
}

func TestLispStateRPC(t *testing.T) {
	v := coretest.New()
	v.InstallLisp()
	s := newLispSvc(t, v, t.TempDir(), true)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "s1", DesiredState: doc(t, lispDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	st, err := s.LispState(context.Background(), &vrxv1.LispStateRequest{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LispState: %s", protojson.Format(st))
	if !st.GetEnabled() || !st.GetGpeEnabled() || st.GetPitr() != "w11-rloc" || len(st.GetLocatorSets()) != 1 ||
		st.GetLocatorSets()[0].GetLocators()[0].GetInterface() != "loop1101" || len(st.GetMappings()) != 2 ||
		len(st.GetAdjacencies()) != 1 || len(st.GetEidTables()) != 1 || len(st.GetGpeVnis()) != 1 ||
		st.GetMapResolvers()[0] != "10.11.1.254" || st.GetMapServers()[0] != "10.11.1.253" {
		t.Fatalf("state %s", protojson.Format(st))
	}
	if st.GetMappings()[0].GetLocal() != true || st.GetMappings()[1].GetRlocs()[0] != "10.11.1.2" {
		t.Fatalf("mappings %v", st.GetMappings())
	}
	if _, err := s.LispState(context.Background(), &vrxv1.LispStateRequest{Owner: "other"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}
}

func mustLisp(t *testing.T, js string) *vrxv1.LispConfig {
	t.Helper()
	l := &vrxv1.LispConfig{}
	if err := protojson.Unmarshal([]byte(js), l); err != nil {
		t.Fatal(err)
	}
	return l
}
