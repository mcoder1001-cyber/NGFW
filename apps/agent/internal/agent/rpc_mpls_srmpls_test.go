package agent

// F-mpls-srmpls agent tests on the fake VPP (coretest, with the MPLS/SR-MPLS model): projection of
// routing.mpls, apply → Retrieve == canonical, rollback (label routes before their tables, steering
// before its policy), the D-071 table-0 requirement, the agent-restart simulation of the write-only
// SR policy, and the MplsState RPC.

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	mplsapi "ngfw/agent/binapi/mpls"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/mpls"
	srmpls "ngfw/agent/internal/descriptors/sr_mpls"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp/bootid"
)

const mplsOwner = "w5"

// fakeBootProc gives the fake VPP (vpe_pid 4242, see mplsModel) a complete D-080 boot identity: a
// fake proc tree with a boot_id and /proc/4242/stat, so the table-0 route records and SR-MPLS claims can be written.
func fakeBootProc(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := bootid.WriteFakeProc(root, "f-mpls-srmpls-boot", map[int]uint64{4242: 777}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bootid.SetProcRoot(root))
}

// newMplsSvc is newSvc for owner w5 with the slot's id range (5000–5999) and a D-071 role.
func newMplsSvc(t *testing.T, v *coretest.VPP, dir string, globalsOwner bool) (*Service, *scheduler.MapRegistry) {
	t.Helper()
	owned, err := ownertable.Open(dir, mplsOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: mplsOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs,
		GlobalsOwner: globalsOwner, IDs: subsystems.IDScope{Range: &subsystems.IDRange{Lo: 5000, Hi: 5999}}})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: mplsOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc, reg
}

const mplsDoc = `{
  "vrfs": {"red": {"id": 5010}},
  "interfaces": {"loop5001": {"ipv4": ["10.5.1.1/24"]}},
  "routing": {"mpls": {
    "interfaces": ["loop5001"],
    "tables": {"5001": {}},
    "labelRoutes": [
      {"table": 5001, "label": 50016, "eos": true, "paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50017, 50018], "weight": 1}]},
      {"table": 0, "label": 50020, "eos": false, "paths": [{"interface": "t1", "outLabels": [50021], "weight": 2}]},
      {"table": 0, "label": 50030, "eos": true, "payload": "ip6", "paths": [{"vrf": "red", "weight": 1}]}
    ],
    "ipBindings": [{"label": 50040, "vrf": "red", "prefix": "10.5.40.0/24"}],
    "tunnels": {"t1": {"paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50050], "weight": 1}], "l2Only": false}},
    "sr": {
      "policies": {"50100": {"segmentLists": [{"labels": [50103], "weight": 2}, {"labels": [50101, 50102], "weight": 1}], "spray": false}},
      "steering": [{"vrf": "default", "prefix": "10.5.60.0/24", "bsid": 50100, "vpnLabel": 50061}]
    }
  }}
}`

// mplsCanonical is Retrieve's routing.mpls for mplsDoc: no ipBindings, no sr (write-only), table 0
// implicit, label routes by table and label, payload only where it differs from the default.
const mplsCanonical = `{
  "interfaces": ["loop5001"],
  "tables": {"5001": {}},
  "labelRoutes": [
    {"table": 0, "label": 50020, "eos": false, "paths": [{"interface": "t1", "outLabels": [50021], "weight": 2}]},
    {"table": 0, "label": 50030, "eos": true, "payload": "ip6", "paths": [{"vrf": "red", "weight": 1}]},
    {"table": 5001, "label": 50016, "eos": true, "paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50017, 50018], "weight": 1}]}
  ],
  "tunnels": {"t1": {"paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50050], "weight": 1}], "l2Only": false}}
}`

func mplsModel(t *testing.T) *coretest.VPP {
	t.Helper()
	fakeBootProc(t)
	v := coretest.New()
	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4242}) // the fake VPP's main PID (D-080)
	v.UseSRFibSource()
	return v
}

func retrievedMpls(t *testing.T, s *Service) *vrxv1.MplsConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState().GetRouting().GetMpls()
}

// The projection emits one object per leaf, under the key the descriptor itself computes, with
// canonical values and the pointers the API maps problems to.
func TestMplsProjection(t *testing.T) {
	v := coretest.New()
	_, reg := newMplsSvc(t, v, t.TempDir(), false)
	ds := doc(t, mplsDoc)
	pj := project(ds, []string{"interfaces", "vrfs", "routing"}, nil, nil)
	for _, is := range pj.issues {
		t.Errorf("issue %+v", is)
	}
	want := map[scheduler.Key]string{
		"mpls-table/0":                           "/routing/mpls",
		"mpls-table/5001":                        "/routing/mpls/tables/5001",
		"mpls-interface/loop5001":                "/routing/mpls/interfaces/0",
		"mpls-route/5001/50016/eos":              "/routing/mpls/labelRoutes/0/label",
		"mpls-route/0/50020/neos":                "/routing/mpls/labelRoutes/1/label",
		"mpls-route/0/50030/eos":                 "/routing/mpls/labelRoutes/2/label",
		"mpls-ip-bind/0/50040/5010/10.5.40.0/24": "/routing/mpls/ipBindings/0/label",
		"mpls-tunnel/t1":                         "/routing/mpls/tunnels/t1",
		"sr-mpls.policy/50100":                   "/routing/mpls/sr/policies/50100",
		"sr-mpls.steering/0/10.5.60.0/24":        "/routing/mpls/sr/steering/0/prefix",
	}
	got := map[scheduler.Key]string{}
	for _, kv := range pj.kvs {
		if !strings.HasPrefix(string(kv.Key), "mpls") && !strings.HasPrefix(string(kv.Key), "sr-mpls") {
			continue
		}
		got[kv.Key] = pj.pointers[kv.Key]
		d, ok := reg.ForKey(kv.Key)
		if !ok {
			t.Fatalf("no descriptor for %s", kv.Key)
		}
		if k := d.KeyOf(kv.Value); k != kv.Key {
			t.Errorf("builder key %s ≠ descriptor key %s", kv.Key, k)
		}
		switch val := kv.Value.(type) {
		case *srmpls.Policy:
			if l := val.GetSegmentLists(); len(l) != 2 || l[0].GetLabels()[0] != 50101 {
				t.Errorf("segment lists not ascending (D-074): %v", l)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("objects:\n got %v\nwant %v", got, want)
	}
	for k, p := range want {
		if got[k] != p {
			t.Errorf("%s: pointer %q, want %q", k, got[k], p)
		}
	}
	// a document without anything that needs MPLS table 0 does not declare it
	ds = doc(t, `{"routing": {"mpls": {"tables": {"5001": {}}, "labelRoutes": [{"table": 5001, "label": 50016, "eos": true, "paths": [{"interface": "t1", "weight": 1}]}],
	  "tunnels": {"t1": {"paths": [{"nextHop": "10.5.1.2", "weight": 1}]}}}}}`)
	for _, kv := range project(ds, []string{"routing"}, nil, nil).kvs {
		if kv.Key == "mpls-table/0" {
			t.Fatal("mpls-table/0 declared without a need")
		}
	}
	// an unknown VRF is a projection error at its pointer
	ds = doc(t, `{"routing": {"mpls": {"ipBindings": [{"label": 50040, "vrf": "blue", "prefix": "10.5.40.0/24"}]}}}`)
	pj = project(ds, []string{"routing"}, nil, nil)
	if len(pj.issues) != 1 || pj.issues[0].pointer != "/routing/mpls/ipBindings/0/vrf" {
		t.Fatalf("issues %+v", pj.issues)
	}
}

// Apply → VPP has every object → Retrieve == canonical; the same document again plans nothing; the
// removal deletes label routes before their table and steering before its policy (V15/V22a), and a
// non-owner never touches MPLS table 0.
func TestMplsApplyRetrieveRollback(t *testing.T) {
	v := mplsModel(t)
	v.AddMplsTable(0, "vrx:0") // the globals owner's table 0 (D-071)
	s, _ := newMplsSvc(t, v, t.TempDir(), false)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "m1", DesiredState: doc(t, mplsDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	if got := v.MplsLabels(5001); strings.Join(got, ",") != "50016/eos" {
		t.Fatalf("table 5001 labels %v", got)
	}
	if got := v.MplsLabels(0); strings.Join(got, ",") != "50020/neos,50030/eos,50040/eos,50100/eos" {
		t.Fatalf("table 0 labels %v (route, route, binding, BSID)", got)
	}
	if n := v.MplsTableNames(); n[5001] != "w5:5001" || n[0] != "vrx:0" {
		t.Fatalf("tables %v", n)
	}
	if tn := v.MplsTunnelsByTag(); tn["w5:t1"] == 0 {
		t.Fatalf("tunnels %v", tn)
	}
	if p := v.SRPolicies()[50100]; len(p) != 2 || p[0].Labels[0] != 50101 {
		t.Fatalf("policy %v (segment lists in ascending order, D-074)", p)
	}
	if !v.HasRoute(0, "10.5.60.0/24") || v.MplsBindings()["5010|10.5.40.0/24"] != 50040 {
		t.Fatal("steering route or binding missing")
	}
	if i, _ := v.InterfaceByName("loop5001"); v.MplsEnabled(i.Index) != 1 {
		t.Fatal("MPLS not enabled on loop5001")
	}

	if got, want := retrievedMpls(t, s), mplsFromJSON(t, mplsCanonical); !proto.Equal(got, want) {
		t.Fatalf("retrieve:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}

	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "m2", DesiredState: doc(t, mplsDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, c := range v.Calls() {
		switch n := c.GetMessageName(); n {
		case "mpls_table_add_del", "mpls_route_add_del", "sw_interface_set_mpls_enable", "mpls_tunnel_add_del",
			"sr_mpls_policy_add", "sr_mpls_policy_mod", "sr_mpls_policy_del", "sr_mpls_steering_add_del":
			t.Fatalf("the same document again sent %s", n)
		}
	}

	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "m3", DesiredState: doc(t, `{"vrfs": {"red": {"id": 5010}}, "interfaces": {"loop5001": {"ipv4": ["10.5.1.1/24"]}}, "routing": {}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	order := mplsCallOrder(v)
	for _, pair := range [][2]string{
		{"route-del 5001", "table-del 5001"},
		{"steering-del", "policy-del 50100"},
		{"route-del 0", "tunnel-del"}, // the label route through the tunnel goes before the tunnel
	} {
		if !before(order, pair[0], pair[1]) {
			t.Errorf("%s must come before %s: %v", pair[0], pair[1], order)
		}
	}
	if got := v.MplsLabels(0); len(got) != 0 {
		t.Fatalf("table 0 still has %v", got)
	}
	if n := v.MplsTableNames(); len(n) != 1 || n[0] != "vrx:0" {
		t.Fatalf("tables after rollback %v (table 0 stays: not ours)", n)
	}
	if len(v.SRPolicies()) != 0 || v.HasRoute(0, "10.5.60.0/24") || len(v.MplsBindings()) != 0 || len(v.MplsTunnelsByTag()) != 0 {
		t.Fatal("SR policy, steering, binding or tunnel left behind")
	}
	if m := retrievedMpls(t, s); m != nil {
		t.Fatalf("retrieve after rollback: %s", protojson.Format(m))
	}
}

func mplsFromJSON(t *testing.T, js string) *vrxv1.MplsConfig {
	t.Helper()
	m := &vrxv1.MplsConfig{}
	if err := protojson.Unmarshal([]byte(js), m); err != nil {
		t.Fatal(err)
	}
	return m
}

// mplsCallOrder names the deleting MPLS / SR-MPLS calls in order.
func mplsCallOrder(v *coretest.VPP) []string {
	var out []string
	for _, c := range v.Calls() {
		switch m := c.(type) {
		case *mplsapi.MplsRouteAddDel:
			if !m.MrIsAdd {
				out = append(out, "route-del "+u32s(m.MrRoute.MrTableID))
			}
		case *mplsapi.MplsTableAddDel:
			if !m.MtIsAdd {
				out = append(out, "table-del "+u32s(m.MtTable.MtTableID))
			}
		case *mplsapi.MplsTunnelAddDel:
			if !m.MtIsAdd {
				out = append(out, "tunnel-del")
			}
		case *srmplsapi.SrMplsPolicyDel:
			out = append(out, "policy-del "+u32s(m.Bsid))
		case *srmplsapi.SrMplsSteeringAddDel:
			if m.IsDel {
				out = append(out, "steering-del")
			}
		}
	}
	return out
}

func u32s(v uint32) string { return itoa(v) }

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for ; v > 0; v /= 10 {
		b = append([]byte{byte('0' + v%10)}, b...)
	}
	return string(b)
}

func before(order []string, a, b string) bool {
	ia, ib := -1, -1
	for i, o := range order {
		if o == a && ia < 0 {
			ia = i
		}
		if o == b {
			ib = i
		}
	}
	return ia >= 0 && ib >= 0 && ia < ib
}

// D-071: an agent that is not the globals owner requires MPLS table 0 — a clear, failed transaction
// while VPP has none (nothing written); a configuration that needs no table 0 applies anyway.
func TestMplsTableZeroRequired(t *testing.T) {
	v := mplsModel(t)
	s, _ := newMplsSvc(t, v, t.TempDir(), false)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "z1", DesiredState: doc(t, `{"interfaces": {"loop5001": {}}, "routing": {"mpls": {"interfaces": ["loop5001"]}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	if !strings.Contains(protojson.Format(resp), "only the globals owner creates it (D-071)") {
		t.Fatalf("the failure must name the globals-owner rule: %s", protojson.Format(resp))
	}
	if len(v.MplsTableNames()) != 0 {
		t.Fatal("a non-owner created an MPLS table")
	}
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "z2", DesiredState: doc(t, `{"interfaces": {"loop5001": {"ipv4": ["10.5.1.1/24"]}}, "routing": {"mpls": {
	  "tables": {"5001": {}},
	  "labelRoutes": [{"table": 5001, "label": 50016, "eos": true, "paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50017], "weight": 1}]}],
	  "tunnels": {"t1": {"paths": [{"nextHop": "10.5.1.2", "interface": "loop5001", "outLabels": [50050], "weight": 1}], "l2Only": false}}}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := v.MplsTableNames(); len(n) != 1 || n[5001] != "w5:5001" {
		t.Fatalf("tables %v", n)
	}
	// the globals owner creates table 0 itself
	g := mplsModel(t)
	gs, _ := newMplsSvc(t, g, t.TempDir(), true)
	mustStatus(t, apply(t, gs, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, `{"interfaces": {"loop5001": {}}, "routing": {"mpls": {"interfaces": ["loop5001"]}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := g.MplsTableNames(); n[0] != "w5:0" {
		t.Fatalf("globals owner's table 0: %v", n)
	}
}

// Restart safety without restarting VPP: the agent stops, label routes and the SR policy are lost
// behind its back, a new agent on the same state dir resyncs and re-creates them — the write-only
// policy re-applied once for this VPP instance (D-076/D-080), the label routes from their dump.
func TestMplsRestartSimulation(t *testing.T) {
	v := mplsModel(t)
	v.AddMplsTable(0, "vrx:0")
	dir := t.TempDir()
	s, _ := newMplsSvc(t, v, dir, false)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, mplsDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()
	if !v.DeleteMplsRoute(5001, 50016, true) || !v.DeleteMplsRoute(0, 50030, true) || !v.DeleteSRPolicy(50100) {
		t.Fatal("simulated loss")
	}
	s2, _ := newMplsSvc(t, v, dir, false)
	start := time.Now()
	resp := s2.Resync(context.Background())
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if time.Since(start) > 30*time.Second {
		t.Fatal("resync took more than 30 s")
	}
	if got := v.MplsLabels(5001); strings.Join(got, ",") != "50016/eos" {
		t.Fatalf("table 5001 after resync %v", got)
	}
	if got := v.MplsLabels(0); strings.Join(got, ",") != "50020/neos,50030/eos,50040/eos,50100/eos" {
		t.Fatalf("table 0 after resync %v", got)
	}
	if p := v.SRPolicies()[50100]; len(p) != 2 {
		t.Fatalf("SR policy not re-created: %v", p)
	}
	// a second resync on the same VPP instance re-applies nothing of the write-only policy
	v.Reset()
	mustStatus(t, s2.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := len(v.CallsNamed("sr_mpls_policy_add")); n != 0 {
		t.Fatalf("policy re-added %d times on the same VPP instance", n)
	}
}

// MplsState: FIB pages of readable tables only, label filter, tunnels of this owner and untagged
// ones, request validation, and one walk at a time (D-132).
func TestMplsStateRPC(t *testing.T) {
	v := mplsModel(t)
	v.AddMplsTable(0, "vrx:0")
	v.AddMplsTable(3001, "w3:3001") // another owner's table
	s, _ := newMplsSvc(t, v, t.TempDir(), false)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "s1", DesiredState: doc(t, mplsDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	g := &server{svc: s}
	ctx := context.Background()

	r, err := g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "fib", TableId: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetTables()) != 2 || r.GetTables()[0].GetTableId() != 0 || r.GetTables()[1].GetTableId() != 5001 {
		t.Fatalf("readable tables %v", r.GetTables())
	}
	var labels []string
	for _, e := range r.GetEntries() {
		labels = append(labels, itoa(e.GetLabel())+map[bool]string{true: "/eos", false: "/neos"}[e.GetEos()])
	}
	if strings.Join(labels, ",") != "0/eos,1/neos,2/eos,50020/neos,50030/eos,50040/eos,50100/eos" || r.GetTotal() != 7 {
		t.Fatalf("table 0: %v total %d", labels, r.GetTotal())
	}
	e := r.GetEntries()[3] // 50020 via the tunnel
	if p := e.GetPaths()[0]; p.GetInterface() != "t1" || len(p.GetOutLabels()) != 1 || p.GetOutLabels()[0] != 50021 || p.GetWeight() != 2 {
		t.Fatalf("50020 path %v", p)
	}
	if r.GetEntries()[4].GetPayload() != "ip6" || r.GetEntries()[4].GetPaths()[0].GetTableId() != 5010 {
		t.Fatalf("50030 %v", r.GetEntries()[4])
	}

	r, err = g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "fib", TableId: 0, Offset: 2, Limit: 2})
	if err != nil || len(r.GetEntries()) != 2 || r.GetEntries()[0].GetLabel() != 2 || r.GetEntries()[1].GetLabel() != 50020 || r.GetTotal() != 7 {
		t.Fatalf("page 2×2: %v %v", r.GetEntries(), err)
	}
	r, err = g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "fib", TableId: 5001, Label: 50016})
	if err != nil || len(r.GetEntries()) != 1 || r.GetTotal() != 1 || r.GetEntries()[0].GetPaths()[0].GetNextHop() != "10.5.1.2" {
		t.Fatalf("label filter: %v %v", r, err)
	}
	for _, bad := range []struct {
		req  *vrxv1.MplsStateRequest
		code codes.Code
	}{
		{&vrxv1.MplsStateRequest{View: "fib", TableId: 3001}, codes.NotFound},
		{&vrxv1.MplsStateRequest{View: "fib", TableId: 5002}, codes.NotFound},
		{&vrxv1.MplsStateRequest{View: "bogus"}, codes.InvalidArgument},
		{&vrxv1.MplsStateRequest{View: "fib", Limit: 1001}, codes.InvalidArgument},
		{&vrxv1.MplsStateRequest{View: "fib", Offset: 99_950, Limit: 100}, codes.InvalidArgument},
		{&vrxv1.MplsStateRequest{View: "fib", Owner: "w3"}, codes.InvalidArgument},
	} {
		if _, err := g.MplsState(ctx, bad.req); grpcCode(err) != bad.code {
			t.Errorf("%v: %v, want %s", bad.req, err, bad.code)
		}
	}

	// tunnels: ours by name, an untagged one by VPP name, another owner's never
	v.AddMplsRoute(mplsapi.MplsRoute{}) // noise in table 0's key space must not matter
	addTunnel(t, v, "")
	addTunnel(t, v, "w3:theirs")
	r, err = g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "tunnels"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tn := range r.GetTunnels() {
		names = append(names, tn.GetName()+map[bool]string{true: "(ours)", false: ""}[tn.GetOwned()])
	}
	if strings.Join(names, ",") != "mpls-tunnel1,t1(ours)" {
		t.Fatalf("tunnels %v", names)
	}

	// one MPLS FIB walk at a time: a second reader waits, then gets UNAVAILABLE
	mplsWalkSem <- struct{}{}
	start := time.Now()
	_, err = g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "fib"})
	<-mplsWalkSem
	if grpcCode(err) != codes.Unavailable || time.Since(start) < mplsWalkWait {
		t.Fatalf("busy walk: %v after %s", err, time.Since(start))
	}
	v.SetConnected(false)
	if _, err := g.MplsState(ctx, &vrxv1.MplsStateRequest{View: "tunnels"}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("disconnected: %v", err)
	}
}

// addTunnel creates a tunnel behind the agent's back (tag "" = untagged).
func addTunnel(t *testing.T, v *coretest.VPP, tag string) {
	t.Helper()
	rep := &mplsapi.MplsTunnelAddDelReply{}
	if err := v.Invoke(context.Background(), &mplsapi.MplsTunnelAddDel{MtIsAdd: true, MtTunnel: mplsapi.MplsTunnel{MtSwIfIndex: interface_types.InterfaceIndex(^uint32(0)), MtTag: tag}}, rep); err != nil || rep.Retval != 0 {
		t.Fatal(err, rep.Retval)
	}
}

var _ = mpls.NameTable // the descriptor package this file exercises through the agent
