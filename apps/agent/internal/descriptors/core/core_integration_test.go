package core_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// dialVPP connects to the host VPP (VRX_VPP_API_SOCKET, default /run/vpp/api.sock).
func dialVPP(t *testing.T) *vpp.Conn {
	t.Helper()
	path := os.Getenv("VRX_VPP_API_SOCKET")
	if path == "" {
		path = "/run/vpp/api.sock"
	}
	c := vpp.Dial(path, vpp.ConnOptions{})
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.WaitConnected(ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

func newScheduler(t *testing.T, c vpp.Client, owner string) *scheduler.Scheduler {
	t.Helper()
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: c, Owner: owner, Owned: ownertable.NewMemory()})
	return scheduler.New(reg, nil)
}

// cleanupSlot deletes, via binapi, every object of this test's owners: loopbacks tagged
// "<owner>:*" and tables named "<owner>:*" (routes go with their table). Runs in t.Cleanup.
func cleanupSlot(t *testing.T, c vpp.Client, owners ...string) {
	t.Helper()
	ctx := context.Background()
	for _, owner := range owners {
		// Scheduler with an empty desired state deletes everything the owner has (except routes in
		// table 0, which this test never creates).
		s := newScheduler(t, c, owner)
		if r := s.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied {
			t.Errorf("cleanup %s: %s %v", owner, r.Outcome, r.Err)
		}
	}
}

func TestCoreOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	// Own owner and index/table range: the agent package's host tests run in parallel with this
	// one under owner <prefix> and clean up by owner.
	owner := vpptest.Prefix(t) + "c"
	c := dialVPP(t)
	t.Cleanup(func() { cleanupSlot(t, c, owner) })
	ctx := context.Background()
	slot := vpptest.Slot(t)
	table := vpptest.TableBase(t) + 11
	l1 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 11))
	l2 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 12))
	i1, _ := core.LoopbackInstance(l1)
	i2, _ := core.LoopbackInstance(l2)
	a1 := fmt.Sprintf("10.%d.11.1/24", slot)
	a2 := fmt.Sprintf("10.%d.12.1/24", slot)
	a6 := fmt.Sprintf("2001:db8:%d:11::1/64", slot)
	nh := fmt.Sprintf("10.%d.11.254", slot)
	dst := fmt.Sprintf("10.%d.110.0/24", slot)
	blackhole := fmt.Sprintf("10.%d.210.0/24", slot)

	desired := []scheduler.KV{
		{Key: core.VRFKey(table), Value: &core.Table{Id: table, Vrf: owner + "-red"}},
		{Key: core.LoopbackKey(l1), Value: &core.Loopback{Name: l1, Instance: i1}},
		{Key: core.LoopbackKey(l2), Value: &core.Loopback{Name: l2, Instance: i2}},
		{Key: core.InterfaceTableKey(l1), Value: &core.InterfaceTable{Interface: l1, TableId: table}},
		{Key: core.InterfaceTableKey(l2), Value: &core.InterfaceTable{Interface: l2, TableId: table}},
		{Key: core.InterfaceAddrKey(l1, a1), Value: &core.InterfaceAddress{Interface: l1, Prefix: a1}},
		{Key: core.InterfaceAddrKey(l1, a6), Value: &core.InterfaceAddress{Interface: l1, Prefix: a6}},
		{Key: core.InterfaceAddrKey(l2, a2), Value: &core.InterfaceAddress{Interface: l2, Prefix: a2}},
		{Key: core.RouteKey(table, dst), Value: &core.Route{TableId: table, Prefix: dst, Preference: 1,
			Paths: []*core.RoutePath{{Address: nh, Weight: 1}, {Address: fmt.Sprintf("10.%d.12.254", slot), Interface: l2, Weight: 3}}}},
		{Key: core.RouteKey(table, blackhole), Value: &core.Route{TableId: table, Prefix: blackhole}},
	}
	s := newScheduler(t, c, owner)
	r := s.Apply(ctx, desired, nil)
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %s %v %+v", r.Outcome, r.Err, r.Results)
	}
	t.Logf("apply: %+v in %v", r.Summary, r.Duration)

	actual, err := s.Retrieve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(desired) {
		t.Fatalf("retrieve returned %d objects, want %d: %v", len(actual), len(desired), actual)
	}
	byKey := map[scheduler.Key]proto.Message{}
	for _, kv := range actual {
		byKey[kv.Key] = kv.Value
	}
	for _, kv := range desired {
		if !proto.Equal(byKey[kv.Key], kv.Value) {
			t.Errorf("%s: retrieved %v, want %v", kv.Key, byKey[kv.Key], kv.Value)
		}
	}

	// Idempotent: second apply plans nothing.
	if r := s.Apply(ctx, desired, nil); r.Outcome != scheduler.OutcomeApplied || !r.Plan.Empty() {
		t.Fatalf("second apply not empty: %s %+v", r.Outcome, r.Plan.Ops)
	}

	// Update in place: route path set changes; VRF binding recreate cascades to the addresses.
	desired[8].Value = &core.Route{TableId: table, Prefix: dst, Preference: 5, Paths: []*core.RoutePath{{Address: nh, Weight: 1}}}
	r = s.Apply(ctx, desired, nil)
	if r.Outcome != scheduler.OutcomeApplied || r.Summary.Updated != 1 {
		t.Fatalf("route update: %s %+v %v", r.Outcome, r.Summary, r.Err)
	}

	// Failure → rollback: a VPP-refused address (overlaps loop1's subnet on loop2) after a
	// successful create; and, separately, a route failing after an address was added.
	for i, extra := range [][]scheduler.KV{
		{
			{Key: core.InterfaceAddrKey(l1, fmt.Sprintf("10.%d.13.1/24", slot)), Value: &core.InterfaceAddress{Interface: l1, Prefix: fmt.Sprintf("10.%d.13.1/24", slot)}},
			{Key: core.InterfaceAddrKey(l2, fmt.Sprintf("10.%d.11.2/24", slot)), Value: &core.InterfaceAddress{Interface: l2, Prefix: fmt.Sprintf("10.%d.11.2/24", slot)}},
		},
		{
			{Key: core.InterfaceAddrKey(l1, fmt.Sprintf("10.%d.13.1/24", slot)), Value: &core.InterfaceAddress{Interface: l1, Prefix: fmt.Sprintf("10.%d.13.1/24", slot)}},
			{Key: core.RouteKey(table, fmt.Sprintf("10.%d.111.0/24", slot)), Value: &core.Route{TableId: table, Prefix: fmt.Sprintf("10.%d.111.0/24", slot), Paths: []*core.RoutePath{{Address: nh, Interface: owner + "-nosuch", Weight: 1}}}},
		},
	} {
		bad := append(append([]scheduler.KV(nil), desired...), extra...)
		r = s.Apply(ctx, bad, nil)
		if r.Outcome != scheduler.OutcomeRolledBack || r.Summary.Reverted != 1 {
			t.Fatalf("case %d: outcome %s summary %+v err %v results %+v", i, r.Outcome, r.Summary, r.Err, r.Results)
		}
		t.Logf("rollback case %d: err=%v results=%+v", i, r.Err, r.Results)
	}
	if p, err := s.Plan(ctx, desired, nil); err != nil || !p.Empty() {
		t.Fatalf("after rollback the state is not the previous one: %v %+v", err, p)
	}

	// Another owner on the same VPP sees nothing of ours and its empty apply deletes nothing.
	other := owner + "x"
	so := newScheduler(t, c, other)
	if kvs, err := so.Retrieve(ctx, nil); err != nil || len(kvs) != 0 {
		t.Fatalf("owner %s sees %v (err %v)", other, kvs, err)
	}
	if r := so.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied || !r.Plan.Empty() {
		t.Fatalf("owner %s empty apply: %s %+v", other, r.Outcome, r.Plan.Ops)
	}
	if p, _ := s.Plan(ctx, desired, nil); !p.Empty() {
		t.Fatalf("our objects changed by the other owner: %+v", p.Ops)
	}

	// Simulated loss: delete a loopback and one VPP table family behind the scheduler's back.
	var idx uint32
	for _, kv := range actual {
		if kv.Key == core.LoopbackKey(l2) {
			idx = kv.Meta.(core.IfMeta).SwIfIndex
		}
	}
	if _, err := interfaces.NewServiceClient(c).DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Plan(ctx, desired, nil)
	if err != nil || p.Empty() {
		t.Fatalf("loss not detected: %v %+v", err, p)
	}
	if r := s.Apply(ctx, desired, nil); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("repair: %s %v", r.Outcome, r.Err)
	}
	if p, _ := s.Plan(ctx, desired, nil); !p.Empty() {
		t.Fatalf("not converged after repair: %+v", p.Ops)
	}

	// Delete everything.
	if r := s.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied || r.Summary.Deleted != len(desired) {
		t.Fatalf("delete all: %s %+v %v", r.Outcome, r.Summary, r.Err)
	}
	if kvs, _ := s.Retrieve(ctx, nil); len(kvs) != 0 {
		t.Fatalf("left over: %v", kvs)
	}
	stream, err := ip.NewServiceClient(c).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		if d.Table.TableID == table {
			t.Fatalf("table %d still exists: %+v", table, d.Table)
		}
	}
}

// TestClaimRulesOnHost (review H1/H2, D-071): two owners collide on a table id and on a table-0
// prefix; the second owner must fail without touching the first owner's objects.
func TestClaimRulesOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	p := vpptest.Prefix(t)
	ownerB, ownerA := p+"rb", p+"ra"
	c := dialVPP(t)
	ctx := context.Background()
	slot := vpptest.Slot(t)
	table := vpptest.TableBase(t) + 50
	blue := fmt.Sprintf("10.%d.250.0/24", slot)
	shared := fmt.Sprintf("10.%d.99.0/24", slot)
	sb := newScheduler(t, c, ownerB)
	sa := newScheduler(t, c, ownerA)
	// Clean up through the same schedulers: their owner tables hold the route claims (a fresh
	// scheduler would not own — and therefore never delete — the table-0 route).
	t.Cleanup(func() {
		for _, s := range []*scheduler.Scheduler{sa, sb} {
			if r := s.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied {
				t.Errorf("cleanup: %s %v", r.Outcome, r.Err)
			}
		}
	})
	theirs := []scheduler.KV{
		{Key: core.VRFKey(table), Value: &core.Table{Id: table, Vrf: "blue"}},
		{Key: core.RouteKey(table, blue), Value: &core.Route{TableId: table, Prefix: blue}},
		{Key: core.RouteKey(0, shared), Value: &core.Route{TableId: 0, Prefix: shared}},
	}
	if r := sb.Apply(ctx, theirs, nil); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("owner %s: %s %v", ownerB, r.Outcome, r.Err)
	}
	// H1: same table id under another name.
	r := sa.Apply(ctx, []scheduler.KV{{Key: core.VRFKey(table), Value: &core.Table{Id: table, Vrf: "red"}}}, nil)
	if r.Outcome == scheduler.OutcomeApplied || !errors.Is(r.Err, core.ErrTableConflict) {
		t.Fatalf("id collision: %s %v", r.Outcome, r.Err)
	}
	t.Logf("id collision: %s: %v", r.Outcome, r.Err)
	// H2: same prefix in the shared table 0.
	r = sa.Apply(ctx, []scheduler.KV{{Key: core.RouteKey(0, shared), Value: &core.Route{TableId: 0, Prefix: shared, Paths: []*core.RoutePath{{Address: fmt.Sprintf("10.%d.1.1", slot), Weight: 1}}}}}, nil)
	if r.Outcome == scheduler.OutcomeApplied || !errors.Is(r.Err, core.ErrRouteConflict) {
		t.Fatalf("prefix collision: %s %v", r.Outcome, r.Err)
	}
	t.Logf("prefix collision: %s: %v", r.Outcome, r.Err)
	if r := sa.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied || !r.Plan.Empty() {
		t.Fatalf("owner %s empty apply: %s %+v", ownerA, r.Outcome, r.Plan.Ops)
	}
	// Owner B still has everything, unchanged.
	if p, err := sb.Plan(ctx, theirs, nil); err != nil || !p.Empty() {
		t.Fatalf("owner %s lost objects: %v %+v", ownerB, err, p)
	}
	// A default route in our own VRF overrides VPP's default-drop entry and is removed cleanly.
	own := vpptest.TableBase(t) + 52
	def := []scheduler.KV{
		{Key: core.VRFKey(own), Value: &core.Table{Id: own, Vrf: "def"}},
		{Key: core.RouteKey(own, "0.0.0.0/0"), Value: &core.Route{TableId: own, Prefix: "0.0.0.0/0", Paths: []*core.RoutePath{{Address: fmt.Sprintf("10.%d.1.1", slot), Weight: 1}}}},
	}
	if r := sa.Apply(ctx, def, nil); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("default route: %s %v", r.Outcome, r.Err)
	}
	if r := sa.Apply(ctx, nil, nil); r.Outcome != scheduler.OutcomeApplied || r.Summary.Deleted != 2 {
		t.Fatalf("default route delete: %s %+v %v", r.Outcome, r.Summary, r.Err)
	}
}
