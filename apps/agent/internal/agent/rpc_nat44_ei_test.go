package agent

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-nat44-ei-64-66-nptv6: NAT44-EI, NAT64, NAT66 and NPTv6 through the whole service on the fake VPP (slot owner w7,
// not the globals owner: the plugins are test fixtures, their enables are requirements, D-071): commit → Retrieve ==
// canonical desired (NPTv6 write-only, never retrieved) → re-apply empty → loss + resync recreates everything, the
// npt66 binding exactly once → rollback removes every object, the npt66 binding before its interface (D-095c);
// the EI and NAT64 session variants and the EI kill over real gRPC.

const eiPart = `,
  "nat": {
    "mode": "ei",
    "inside": ["host-w7l0"],
    "outside": ["host-w7w0"],
    "forwarding": false,
    "staticMappingOnly": false,
    "connectionTracking": false,
    "timeouts": {"udp": 300, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60},
    "pools": [{"name": "out", "range": "10.7.2.100-10.7.2.103", "twiceNat": false}],
    "staticMappings": [{"name": "web", "protocol": "tcp", "local": {"ip": "10.7.1.2", "port": 80}, "external": {"ip": "10.7.2.110", "port": 8080}}],
    "nat64": {
      "enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"],
      "prefixes": [{"prefix": "fd00:7:64::/96", "vrf": "cust"}],
      "pools": [{"range": "10.7.64.1-10.7.64.2"}],
      "staticBibs": [{"protocol": "tcp", "inside": {"ip": "fd00:7::80", "port": 80}, "outside": {"ip": "10.7.64.1", "port": 8080}}],
      "timeouts": {"udp": 300, "tcpEstablished": 7440, "tcpTransitory": 240, "icmp": 60}
    },
    "nat66": {"enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"],
      "staticMappings": [{"local": "fd00:7:1::66", "external": "fd00:7:2::66"}]},
    "nptv6": {"bindings": [{"interface": "host-w7w0", "internal": "fd00:7:10::/48", "external": "fd00:7:20::/48"}]}
  }`

const canonicalEI = `{
  "mode": "ei",
  "inside": ["host-w7l0"],
  "outside": ["host-w7w0"],
  "pools": [{"range": "10.7.2.100-10.7.2.103", "twiceNat": false}],
  "staticMappings": [{"name": "web", "protocol": "tcp", "local": {"ip": "10.7.1.2", "port": 80}, "external": {"ip": "10.7.2.110", "port": 8080}, "twiceNat": false, "selfTwiceNat": false, "out2inOnly": false}],
  "nat64": {"enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"],
    "prefixes": [{"prefix": "fd00:7:64::/96", "vrf": "cust"}], "pools": [{"range": "10.7.64.1-10.7.64.2"}],
    "staticBibs": [{"protocol": "tcp", "inside": {"ip": "fd00:7::80", "port": 80}, "outside": {"ip": "10.7.64.1", "port": 8080}}]},
  "nat66": {"enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"],
    "staticMappings": [{"local": "fd00:7:1::66", "external": "fd00:7:2::66"}]}
}`

func fixtures(v *coretest.VPP) {
	v.Nat44EI().NatEnable()
	v.Nat64().NatEnable()
	v.Nat66().NatEnable()
}

func npt66State(v *coretest.VPP) (bindings, features, adds int) {
	n := v.NPT66()
	b, _ := n.Binding("host-w7w0")
	n.Lock()
	defer n.Unlock()
	return len(n.Bindings), n.FeatureEnables[b.SwIfIndex], n.Adds
}

func TestNatEI6466DomainOnFake(t *testing.T) {
	v := coretest.New()
	fixtures(v)
	s := newSvc(t, v, t.TempDir())
	withNat := doc(t, strings.Replace(natIfDoc, "%s", eiPart, 1))
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "e1", DesiredState: withNat})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got, want := natNat(t, s), doc(t, `{"nat":`+canonicalEI+`}`).GetNat(); !proto.Equal(got, want) {
		t.Fatalf("Retrieve nat != canonical desired:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if b, f, _ := npt66State(v); b != 1 || f != 1 {
		t.Fatalf("npt66 after commit: %d bindings, features %d", b, f)
	}
	// idempotent: an empty plan (the write-only enables and the binding were applied by this process)
	if resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "e2", DesiredState: withNat}); len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: withNat})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run %v %v", err, rep)
	}

	// loss behind the agent's back (the npt66 binding and the NAT objects first, D-095c) + resync → recreated
	if !v.NPT66().DeleteBinding("host-w7w0") {
		t.Fatal("no binding to lose")
	}
	ei, n64, n66 := v.Nat44EI(), v.Nat64(), v.Nat66()
	ei.Lock()
	ei.Statics, ei.Addrs, ei.Features = nil, nil, map[uint32]nat44_ei.Nat44EiConfigFlags{}
	ei.Unlock()
	n64.Lock()
	n64.Prefixes, n64.Pool, n64.BIBs, n64.Ifaces = nil, nil, nil, map[uint32]nat_types.NatConfigFlags{}
	n64.Unlock()
	n66.Lock()
	n66.Mappings, n66.Ifaces = nil, map[uint32]nat_types.NatConfigFlags{}
	n66.Unlock()
	s.Resync(context.Background())
	if got, want := natNat(t, s), doc(t, `{"nat":`+canonicalEI+`}`).GetNat(); !proto.Equal(got, want) {
		t.Fatalf("after resync:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	// the write-only binding is re-applied on every resync: exactly one binding, features enabled once
	for i := 0; i < 2; i++ {
		s.Resync(context.Background())
	}
	if b, f, adds := npt66State(v); b != 1 || f != 1 || adds < 4 {
		t.Fatalf("npt66 after loss + 3 resyncs: %d bindings, features %d, %d adds", b, f, adds)
	}

	// rollback to a document without nat and without the interfaces: every object leaves VPP (Retrieve and the
	// models), the npt66 binding before its interface; the plugins stay enabled (a slot never disables, D-071)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "e3", DesiredState: doc(t, `{"vrfs": {"cust": {"id": 7001}}, "interfaces": {}, "nat": {}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := natNat(t, s); proto.Size(got) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if st := v.NPT66().Stale(); len(st) != 0 || v.NPT66().Count() != 0 {
		t.Fatalf("npt66 after rollback: %d bindings, stale %v", v.NPT66().Count(), st)
	}
	if !ei.Empty() || !n64.Empty() || !n66.Empty() || !ei.Enabled || !n64.Enabled || !n66.Enabled {
		t.Fatalf("after rollback: empty %v %v %v enabled %v %v %v", ei.Empty(), n64.Empty(), n66.Empty(), ei.Enabled, n64.Enabled, n66.Enabled)
	}
}

func TestNatEIAndNat64SessionsOverGRPC(t *testing.T) {
	v := coretest.New()
	fixtures(v)
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "k1", DesiredState: doc(t, strings.Replace(natIfDoc, "%s", eiPart, 1))}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	ei := v.Nat44EI()
	for u := 0; u < 21; u++ { // 2 100 EI sessions of w7's users, 1 in VRF cust, 1 of another slot's
		for i := 0; i < 100; i++ {
			ei.AddEISession(0, 6, "10.7.1."+strconv.Itoa(10+u), uint16(10000+i), "10.7.2.100", uint16(20000+u*100+i), "10.7.2.2", 80, false) //nolint:gosec // test data
		}
	}
	ei.AddEISession(7001, 17, "10.7.1.10", 5353, "10.7.2.101", 6000, "10.7.2.3", 53, false)
	ei.AddEISession(0, 6, "10.9.1.5", 1, "10.9.2.100", 2, "10.9.2.2", 3, false)
	n64 := v.Nat64()
	for i := 0; i < 5; i++ {
		n64.AddSession(0, 6, "fd00:7::"+strconv.Itoa(10+i), uint16(40000+i), "10.7.64.1", uint16(1024+i), "fd00:7:64::a07:202", "10.7.2.2", 80) //nolint:gosec // test data
	}
	n64.AddSession(0, 17, "fd00:7::10", 5353, "10.7.64.2", 2000, "fd00:7:64::a07:203", "10.7.2.3", 53)
	n64.AddSession(9001, 6, "fd00:9::1", 1, "10.9.64.1", 2, "fd00:9:64::a09:202", "10.9.2.2", 80) // another slot's
	c := natServer(t, s)
	ctx := context.Background()
	eiV, n64V := vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_EI.Enum(), vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_NAT64.Enum()

	// EI: the ED pager semantics (bounded, owner-scoped, filters), the variant echoed
	r, err := c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Variant: eiV, Limit: 100, Offset: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetSessions()) != 100 || r.GetTotalSessions() != 2101 || r.GetTotalUsers() != 22 || r.GetNextOffset() != 300 || r.GetVariant() != *eiV {
		t.Fatalf("EI page: %d sessions total %d users %d next %d variant %v", len(r.GetSessions()), r.GetTotalSessions(), r.GetTotalUsers(), r.GetNextOffset(), r.GetVariant())
	}
	first := r.GetSessions()[0]
	if first.GetInsideAddress() != "10.7.1.12" || first.GetExternalNatAddress() != first.GetExternalAddress() || first.GetTwiceNat() || first.GetVrf() != "default" {
		t.Fatalf("EI session %v", first)
	}
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Variant: eiV, Filter: &vrxv1.NatSessionFilter{Vrf: proto.String("cust")}})
	if err != nil || r.GetTotalSessions() != 1 || r.GetSessions()[0].GetProtocol() != "udp" {
		t.Fatalf("EI vrf filter %v %v", r, err)
	}
	// an unset variant is still NAT44-ED (nat44-ed is off in this fake: no session, no variant echoed)
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{})
	if err != nil || r.GetTotalSessions() != 0 || r.Variant != nil {
		t.Fatalf("ED page %v %v", r, err)
	}

	// NAT64: owner-scoped, paged, protocol filter only; the field mapping of proto.md §11
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Variant: n64V, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetSessions()) != 4 || r.GetTotalSessions() != 6 || r.GetTotalUsers() != 5 || r.GetNextOffset() != 4 || r.GetVariant() != *n64V {
		t.Fatalf("NAT64 page %s", protojson.Format(r))
	}
	s0 := r.GetSessions()[0]
	if s0.GetInsideAddress() != "fd00:7::10" || s0.GetOutsideAddress() != "10.7.64.1" || s0.GetExternalAddress() != "10.7.2.2" ||
		s0.GetExternalNatAddress() != "fd00:7:64::a07:202" || s0.GetExternalPort() != 80 || s0.GetProtocol() != "tcp" {
		t.Fatalf("NAT64 session %v", s0)
	}
	r, err = c.NatSessions(ctx, &vrxv1.NatSessionsRequest{Variant: n64V, Filter: &vrxv1.NatSessionFilter{Protocol: proto.String("udp")}})
	if err != nil || r.GetTotalSessions() != 1 || r.GetSessions()[0].GetOutsidePort() != 2000 {
		t.Fatalf("NAT64 protocol filter %v %v", r, err)
	}
	for name, req := range map[string]*vrxv1.NatSessionsRequest{
		"nat64 inside filter": {Variant: n64V, Filter: &vrxv1.NatSessionFilter{InsideAddress: proto.String("10.7.1.1")}},
		"nat64 limit":         {Variant: n64V, Limit: 1001},
		"ei owner":            {Variant: eiV, Owner: "w3"},
		"ei vrf":              {Variant: eiV, Filter: &vrxv1.NatSessionFilter{Vrf: proto.String("nope")}},
	} {
		if _, err := c.NatSessions(ctx, req); grpcCode(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}

	// EI kill by the inside endpoint (no external endpoint needed): exit 0, then 1; foreign / NAT64: INVALID_ARGUMENT
	k := &vrxv1.NatSessionKillAction{Variant: eiV, Protocol: "tcp", InsideAddress: "10.7.1.10", InsidePort: 10005}
	out, err := kill(t, c, k)
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 0 || out[0].GetDone().GetStats()["variant"] != "ei" {
		t.Fatalf("EI kill: %v %v", out, err)
	}
	if ei.SessionCount() != 2102-1 {
		t.Fatalf("EI sessions after kill: %d", ei.SessionCount())
	}
	out, err = kill(t, c, k)
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 1 || !strings.Contains(out[0].GetDone().GetSummary(), "no such NAT44-EI session") {
		t.Fatalf("second EI kill: %v %v", out, err)
	}
	for name, a := range map[string]*vrxv1.NatSessionKillAction{
		"foreign": {Variant: eiV, Protocol: "tcp", InsideAddress: "10.9.1.5", InsidePort: 1},
		"nat64":   {Variant: n64V, Protocol: "tcp", InsideAddress: "fd00:7::10", InsidePort: 40000},
		"proto":   {Variant: eiV, Protocol: "xx", InsideAddress: "10.7.1.10", InsidePort: 1},
	} {
		if _, err := kill(t, c, a); grpcCode(err) != codes.InvalidArgument {
			t.Errorf("%s kill: %v", name, err)
		}
	}
}

// D-095c: an npt66 binding is removed before the interface it sits on (VPP keeps a binding whose interface is gone;
// the model reports it as stale). The loopback is created and deleted by the agent.
func TestNptv6BindingDeletedBeforeItsInterface(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	withBinding := doc(t, `{"interfaces": {"loop703": {"enabled": true, "ipv6": ["fd00:7:30::1/64"]}},
	  "nat": {"nptv6": {"bindings": [{"interface": "loop703", "internal": "fd00:7:10::/48", "external": "fd00:7:20::/48"}]}}}`)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "b1", DesiredState: withBinding}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if b, ok := v.NPT66().Binding("loop703"); !ok || b.External.String() != "fd00:7:20::/48" {
		t.Fatalf("binding %v %+v", ok, b)
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "b2", DesiredState: doc(t, `{"nat": {}}`), Subsystems: []string{"interfaces", "nat"}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	bi, ii := -1, -1
	var order []string
	for i, r := range resp.GetResults() {
		k := r.GetKey()
		order = append(order, k)
		switch {
		case strings.HasPrefix(k, "npt66.binding/loop703/"):
			bi = i
		case strings.Contains(k, "loop703") && ii < 0:
			ii = i
		}
	}
	t.Logf("delete order: %v", order)
	if bi < 0 || ii < 0 || ii < bi {
		t.Fatalf("binding at %d, first interface object at %d: %v", bi, ii, order)
	}
	if _, ok := v.InterfaceByName("loop703"); ok {
		t.Fatal("loop703 not deleted")
	}
	if st := v.NPT66().Stale(); len(st) != 0 || v.NPT66().Count() != 0 {
		t.Fatalf("npt66 after delete: %d bindings, stale %v", v.NPT66().Count(), st)
	}
}
