package core_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/scheduler"
)

// F-vrf-static-ecmp: weighted ECMP, blackhole and a next hop resolved in another VRF (RoutePath.next_hop_table).
func ecmpDesired() []scheduler.KV {
	return []scheduler.KV{
		{Key: "vrf/2001", Value: &core.Table{Id: 2001, Vrf: "red"}},
		{Key: "vrf/2002", Value: &core.Table{Id: 2002, Vrf: "blue"}},
		// weighted ECMP default route in red: 3:1
		{Key: "ip.route/2001/0.0.0.0/0", Value: &core.Route{TableId: 2001, Prefix: "0.0.0.0/0", Preference: 1, Paths: []*core.RoutePath{
			{Address: "10.2.1.2", Weight: 3}, {Address: "10.2.1.3", Weight: 1}}}},
		// next hops resolved in default (table 0) and in blue, both from red
		{Key: "ip.route/2001/10.2.60.0/24", Value: &core.Route{TableId: 2001, Prefix: "10.2.60.0/24", Paths: []*core.RoutePath{
			{Address: "10.2.2.2", Weight: 1, NextHopTable: proto.Uint32(0)}, {Address: "10.2.9.9", Weight: 1, NextHopTable: proto.Uint32(2002)}}}},
		{Key: "ip.route/2002/10.2.70.0/24", Value: &core.Route{TableId: 2002, Prefix: "10.2.70.0/24"}}, // blackhole
		{Key: "ip.route/2001/2001:db8:2:60::/64", Value: &core.Route{TableId: 2001, Prefix: "2001:db8:2:60::/64", Paths: []*core.RoutePath{
			{Address: "2001:db8:2::2", Weight: 1, NextHopTable: proto.Uint32(0)}}}},
	}
}

func TestECMPAndNextHopTableRoundTrip(t *testing.T) {
	r := newRig(t, "w2", nil)
	res := r.s.Apply(context.Background(), ecmpDesired(), nil)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply %v %v", res.Outcome, res.Err)
	}
	// what VPP was told: weights and per-path resolution tables
	got := r.vpp.Routes
	paths := func(table uint32, prefix string) []fib_types.FibPath {
		for k, v := range got {
			if v.TableID == table && v.Prefix.String() == prefix {
				_ = k
				return v.Paths
			}
		}
		t.Fatalf("route %d %s not in VPP", table, prefix)
		return nil
	}
	ecmp := paths(2001, "0.0.0.0/0")
	if len(ecmp) != 2 || ecmp[0].Weight != 3 || ecmp[1].Weight != 1 || ecmp[0].TableID != 2001 {
		t.Fatalf("ECMP paths %+v", ecmp)
	}
	leak := paths(2001, "10.2.60.0/24")
	if len(leak) != 2 || leak[0].TableID != 0 || leak[1].TableID != 2002 {
		t.Fatalf("next-hop table paths %+v", leak)
	}
	if bh := paths(2002, "10.2.70.0/24"); len(bh) != 1 || bh[0].Type != fib_types.FIB_API_PATH_TYPE_DROP {
		t.Fatalf("blackhole %+v", bh)
	}
	// Retrieve == desired, and a second Apply is an empty plan
	kvs, err := r.s.Retrieve(context.Background(), scheduler.Only(core.VRFName, core.RouteName))
	if err != nil {
		t.Fatal(err)
	}
	want := map[scheduler.Key]proto.Message{}
	for _, kv := range ecmpDesired() {
		want[kv.Key] = kv.Value
	}
	if len(kvs) != len(want) {
		t.Fatalf("retrieved %d objects, want %d: %v", len(kvs), len(want), kvs)
	}
	for _, kv := range kvs {
		if !proto.Equal(kv.Value, want[kv.Key]) {
			t.Errorf("%s: retrieved %v, want %v", kv.Key, kv.Value, want[kv.Key])
		}
	}
	if res := r.s.Apply(context.Background(), ecmpDesired(), nil); len(res.Plan.Ops) != 0 {
		t.Fatalf("second apply planned %v", res.Plan.Ops)
	}
	// rollback of everything: routes before tables (V15), nothing left
	if res := r.s.Apply(context.Background(), nil, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("delete %v %v", res.Outcome, res.Err)
	}
	if r.vpp.RouteCount() != 0 || r.vpp.HasTable(2001, false) || r.vpp.HasTable(2002, false) {
		t.Fatalf("leftovers:\n%s", r.vpp.Snapshot())
	}
}

func TestNextHopTableDependenciesAndValidation(t *testing.T) {
	r := newRig(t, "w2", nil)
	d := r.desc(core.RouteName)
	deps := d.Dependencies(&core.Route{TableId: 2001, Prefix: "10.2.60.0/24", Paths: []*core.RoutePath{
		{Address: "10.2.2.2", NextHopTable: proto.Uint32(0)}, {Address: "10.2.9.9", NextHopTable: proto.Uint32(2002)},
		{Address: "10.2.9.8", NextHopTable: proto.Uint32(2002)}}})
	if len(deps) != 2 || deps[0].Key != "vrf/2001" || deps[1].Key != "vrf/2002" || deps[1].Optional {
		t.Fatalf("deps %v", deps)
	}
	// a next-hop table with an egress interface is refused before anything is sent
	_, err := d.Create(context.Background(), &core.Route{TableId: 0, Prefix: "10.2.61.0/24", Paths: []*core.RoutePath{
		{Address: "10.2.2.2", Interface: "loop201", NextHopTable: proto.Uint32(2002)}}})
	if !errors.Is(err, core.ErrBadValue) {
		t.Fatalf("next-hop table + interface: %v", err)
	}
	if len(r.vpp.CallsNamed("ip_route_add_del")) != 0 {
		t.Fatal("sent a route with an invalid path")
	}
}

func TestSortPathsNextHopTableTieBreak(t *testing.T) {
	p := []*core.RoutePath{
		{Address: "10.2.2.2", Weight: 1, NextHopTable: proto.Uint32(2002)},
		{Address: "10.2.2.2", Weight: 1, NextHopTable: proto.Uint32(0)},
		{Address: "10.2.2.2", Weight: 1},
	}
	core.SortPaths(p)
	if p[0].NextHopTable != nil || p[1].GetNextHopTable() != 0 || p[2].GetNextHopTable() != 2002 {
		t.Fatalf("order %v", p)
	}
}
