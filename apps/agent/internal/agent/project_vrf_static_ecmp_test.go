package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// F-vrf-static-ecmp through the whole agent (projection → scheduler → descriptors on the coretest model → Retrieve):
// weighted ECMP, blackhole, a next hop in another VRF, source VRF select and a viaFrr route the agent must not program.
const ecmpDoc = `{
  "vrfs": {"red": {"id": 2001, "sourceSelect": [
      {"prefix": "10.2.50.0/24", "interface": "loop202"},
      {"prefix": "2001:db8:2:50::/64", "interface": "loop202"}]},
    "blue": {"id": 2002}},
  "interfaces": {"loop201": {"vrf": "red", "ipv4": ["10.2.1.1/24"]}, "loop202": {"ipv4": ["10.2.2.1/24"]}},
  "routing": {"static": [
    {"prefix": "0.0.0.0/0", "vrf": "red", "distance": 1, "blackhole": false,
     "nextHops": [{"address": "10.2.1.2", "weight": 3}, {"address": "10.2.1.3", "weight": 1}]},
    {"prefix": "10.2.60.0/24", "vrf": "red", "distance": 1, "blackhole": false,
     "nextHops": [{"address": "10.2.2.2", "weight": 1, "vrf": "default"}]},
    {"prefix": "10.2.70.0/24", "vrf": "blue", "distance": 1, "blackhole": true},
    {"prefix": "10.2.80.0/24", "vrf": "default", "distance": 1, "blackhole": false, "viaFrr": true,
     "nextHops": [{"address": "10.2.2.2", "weight": 1}]}
  ]}
}`

// fixedIdentity replaces the D-080 identity source for the fake VPP (its PID is not a real process).
func fixedIdentity(t *testing.T) {
	t.Helper()
	prev := dfkit.IdentitySource
	dfkit.IdentitySource = func(context.Context, vpp.Client) (bootid.Identity, error) {
		return bootid.Identity{BootID: "fake", PID: 1, StartTime: 1}, nil
	}
	t.Cleanup(func() { dfkit.IdentitySource = prev })
}

func newEcmpSvc(t *testing.T) (*Service, *coretest.VPP) {
	t.Helper()
	t.Setenv(subsystems.EnvTableBase, "2000") // svs tables from the slot range (2900–2999)
	fixedIdentity(t)
	v := coretest.New().InstallVrfStaticEcmp()
	return newSvc(t, v, t.TempDir()), v
}

func TestVrfStaticEcmpApplyRetrieve(t *testing.T) {
	s, v := newEcmpSvc(t)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "e1", DesiredState: doc(t, ecmpDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	// D-072: the viaFrr route is not programmed by the agent
	if v.HasRoute(0, "10.2.80.0/24") {
		t.Fatal("viaFrr route programmed in VPP")
	}
	// weighted ECMP and the next-hop table reached VPP
	if !v.HasRoute(2001, "0.0.0.0/0") || !v.HasRoute(2001, "10.2.60.0/24") || !v.HasRoute(2002, "10.2.70.0/24") {
		t.Fatalf("routes missing:\n%s", v.Snapshot())
	}
	// source VRF select: one svs table for loop202 in the slot's svs range, both families enabled
	st := v.Svs()
	if len(st.Enabled) != 2 || len(st.Routes) != 2 {
		t.Fatalf("svs state enabled %v routes %v", st.Enabled, st.Routes)
	}
	for k, e := range st.Routes {
		if e.SourceTable != 2001 {
			t.Fatalf("svs route %v selects %d", k, e.SourceTable)
		}
	}
	for _, table := range st.Enabled {
		if table < 2900 || table > 2999 {
			t.Fatalf("svs table %d outside the slot's svs range", table)
		}
	}

	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"vrfs", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	want := doc(t, ecmpDoc)
	want.Interfaces = nil
	// the viaFrr route is FRR's (not in the agent's Retrieve); canonical order is (VRF, prefix)
	st3 := want.Routing.Static
	want.Routing.Static = []*vrxv1.StaticRoute{st3[2], st3[0], st3[1]}
	gotDS := got.GetDesiredState()
	if !proto.Equal(gotDS.GetVrfs()["red"], want.GetVrfs()["red"]) || !proto.Equal(gotDS.GetRouting(), want.GetRouting()) {
		t.Fatalf("retrieve\n got %s\nwant %s", protojson.Format(gotDS), protojson.Format(want))
	}

	// the same document again: empty plan
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "e2", DesiredState: doc(t, ecmpDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := resp.GetSummary().GetCreated() + resp.GetSummary().GetUpdated() + resp.GetSummary().GetDeleted(); n != 0 {
		t.Fatalf("second apply changed %d objects: %v", n, resp.GetResults())
	}

	// DryRun names the viaFrr route as FRR's (drift ignores it)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: doc(t, ecmpDoc)})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, is := range rep.GetErrors() {
		if is.GetPointer() == "/routing/static/3" && is.GetRule() == "agent.unsupported-field" && is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
			found = true
		}
	}
	if !found {
		t.Fatalf("no viaFrr notice in %v", rep.GetErrors())
	}

	// rollback to nothing: routes, svs entries, enablements and tables all gone (V15 order)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "e3", DesiredState: doc(t, `{}`), Subsystems: []string{"interfaces", "vrfs", "routing"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if v.RouteCount() != 0 || len(st.Routes) != 0 || len(st.Enabled) != 0 {
		t.Fatalf("leftovers:\n%s\nsvs %v %v", v.Snapshot(), st.Routes, st.Enabled)
	}
	for id := uint32(2000); id < 3000; id++ {
		if v.HasTable(id, false) || v.HasTable(id, true) {
			t.Fatalf("table %d left", id)
		}
	}
}

func TestVrfStaticEcmpNextHopVrfValidation(t *testing.T) {
	s, _ := newEcmpSvc(t)
	for _, tc := range []struct{ js, pointer string }{
		{`{"vrfs": {"red": {"id": 2001}}, "routing": {"static": [{"prefix": "10.2.60.0/24", "vrf": "red", "nextHops": [{"address": "10.2.2.2", "vrf": "nope"}]}]}}`, "/routing/static/0/nextHops/0/vrf"},
		{`{"vrfs": {"red": {"id": 2001}}, "routing": {"static": [{"prefix": "10.2.60.0/24", "vrf": "red", "nextHops": [{"address": "10.2.2.2", "interface": "loop201", "vrf": "default"}]}]}}`, "/routing/static/0/nextHops/0/vrf"},
		{`{"vrfs": {"red": {"id": 2001, "sourceSelect": [{"prefix": "0.0.0.0/0", "interface": "loop201"}]}}}`, "/vrfs/red/sourceSelect/0/prefix"},
	} {
		resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "v-" + tc.pointer + strings.Repeat("x", len(tc.js)%7), DesiredState: doc(t, tc.js)})
		if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Fatalf("%s: applied", tc.js)
		}
		ok := false
		for _, is := range resp.GetValidation().GetErrors() {
			if is.GetPointer() == tc.pointer && is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%s: no error at %s: %v", tc.js, tc.pointer, resp.GetValidation().GetErrors())
		}
	}
}

func TestVrfStaticEcmpSelectorRegisteredOnce(t *testing.T) {
	subsystems.RegisterStaticSelector()
	subsystems.RegisterStaticSelector() // guarded by sync.Once: a second call must not panic
	if !subsystems.ViaFrr(0, &vrxv1.StaticRoute{ViaFrr: proto.Bool(true)}, nil) || subsystems.ViaFrr(0, &vrxv1.StaticRoute{}, nil) {
		t.Fatal("selector does not read viaFrr")
	}
}

func TestSvsRangeFromSlot(t *testing.T) {
	t.Setenv(subsystems.EnvTableBase, "2000")
	if r := subsystems.SvsRange(); r != (svs.Range{Lo: 2900, Hi: 2999}) {
		t.Fatalf("slot range %v", r)
	}
	t.Setenv(subsystems.EnvTableBase, "bogus")
	if r := subsystems.SvsRange(); r != (svs.Range{}) {
		t.Fatalf("malformed base → %v", r)
	}
	if err := os.Unsetenv(subsystems.EnvTableBase); err != nil {
		t.Fatal(err)
	}
	if r := subsystems.SvsRange(); r != svs.DefaultRange {
		t.Fatalf("product range %v", r)
	}
}

// actionStream is a minimal grpc.ServerStreamingServer[vrxv1.ActionOutput].
type actionStream struct {
	grpc.ServerStream
	ctx context.Context
	out []*vrxv1.ActionOutput
}

func (a *actionStream) Context() context.Context         { return a.ctx }
func (a *actionStream) Send(o *vrxv1.ActionOutput) error { a.out = append(a.out, o); return nil }

func TestVrfStaticEcmpRPCs(t *testing.T) {
	s, v := newEcmpSvc(t)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, ecmpDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	g := &server{svc: s}
	ctx := context.Background()

	page, err := g.ListRoutes(ctx, &vrxv1.ListRoutesRequest{Vrf: "red", Family: "ipv4", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.GetTableId() != 2001 || page.GetVrf() != "red" || page.GetOwner() != testOwner {
		t.Fatalf("page header %+v", page)
	}
	var def *vrxv1.ListRoutesEntry
	for _, r := range page.GetRoutes() {
		if r.GetPrefix() == "0.0.0.0/0" && r.GetSource() == "API" {
			def = r
		}
	}
	if def == nil || len(def.GetPaths()) != 2 || def.GetPaths()[0].GetWeight() != 3 {
		t.Fatalf("ECMP default route in red: %v", page.GetRoutes())
	}
	for _, tc := range []struct {
		req  *vrxv1.ListRoutesRequest
		code codes.Code
	}{
		{&vrxv1.ListRoutesRequest{Vrf: "nope"}, codes.NotFound},
		{&vrxv1.ListRoutesRequest{Vrf: "red", Limit: 5000}, codes.InvalidArgument},
		{&vrxv1.ListRoutesRequest{Owner: "someone-else"}, codes.InvalidArgument},
	} {
		if _, err := g.ListRoutes(ctx, tc.req); status.Code(err) != tc.code {
			t.Errorf("%v: %v, want %v", tc.req, err, tc.code)
		}
	}

	st := &actionStream{ctx: ctx}
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Ping{Ping: &vrxv1.PingAction{Target: "10.2.2.2", Count: 3, IntervalMs: 100}}}, st); err != nil {
		t.Fatal(err)
	}
	if len(st.out) != 2 || st.out[1].GetDone() == nil || st.out[1].GetDone().GetStats()["received"] != "3" {
		t.Fatalf("ping output %v", st.out)
	}
	if len(v.Svs().Pings) != 1 {
		t.Fatalf("pings sent %v", v.Svs().Pings)
	}
	// review M1: a VPP with worker threads → FAILED_PRECONDITION (the API answers 409), nothing sent to the ping plugin
	v.Svs().Workers = 2
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Ping{Ping: &vrxv1.PingAction{Target: "10.2.2.2"}}}, &actionStream{ctx: ctx}); status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "2 worker thread(s)") {
		t.Fatalf("ping with workers: %v", err)
	}
	if len(v.Svs().Pings) != 1 {
		t.Fatalf("ping with workers reached VPP: %v", v.Svs().Pings)
	}
	v.Svs().Workers = 0
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Ping{Ping: &vrxv1.PingAction{Target: "10.2.2.2", Vrf: "red"}}}, &actionStream{ctx: ctx}); status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "VRF") {
		t.Fatalf("ping in red: %v", err)
	}
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Traceroute{Traceroute: &vrxv1.TracerouteAction{Target: "10.2.2.2"}}}, &actionStream{ctx: ctx}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("traceroute: %v", err)
	}
	if err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_Capture{Capture: &vrxv1.CaptureAction{Interface: "loop201"}}}, &actionStream{ctx: ctx}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("capture: %v", err)
	}
}
