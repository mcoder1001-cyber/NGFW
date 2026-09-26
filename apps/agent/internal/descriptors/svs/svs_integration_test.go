package svs_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	svsapi "ngfw/agent/binapi/svs"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestVrfStaticEcmpOnHost is the F-vrf-static-ecmp host check (VRX_INTEGRATION=1, shared lab lock, slot-prefixed objects
// only): weighted ECMP, blackhole, next hops resolved in another VRF and source VRF select on the real VPP. After Apply
// Retrieve == desired and `vppctl show …` reflects it; a simulated loss is recreated; the rollback removes routes before
// tables and leaves no stray entry (V15), proved by re-creating the table id and dumping it.
func TestVrfStaticEcmpOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %s → %s", restarts0, n)
		}
	})
	owner := vpptest.Prefix(t) + "s" // own owner: the core and agent host tests of this slot clean up by theirs
	slot := vpptest.Slot(t)
	base := vpptest.TableBase(t)
	red, blue := base+21, base+22
	l1 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 21))
	l2 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 22))
	i1, _ := core.LoopbackInstance(l1)
	i2, _ := core.LoopbackInstance(l2)
	ids, err := svs.Allocate([]string{l2}, svs.RangeIn(base, base+999), func(id uint32) bool { return id == red || id == blue })
	if err != nil {
		t.Fatal(err)
	}
	st := ids[l2]
	c := dialVPP(t)
	ctx := context.Background()
	// the owner table and the BootStore are the agent's persisted stores: shared by every reconciler of this test
	boot, owned := dfkit.NewMemoryBootStore(), ownertable.NewMemory()
	newSched := func() *scheduler.Scheduler {
		reg := scheduler.NewRegistry()
		core.Register(reg, core.Env{Client: c, Owner: owner, Owned: owned, IfRef: core.DirectInterfaceRef})
		svs.Register(reg, svs.Env{Client: c, Owner: owner, Boot: boot, IfRef: core.DirectInterfaceRef, IfTableRef: core.InterfaceTableKey})
		return scheduler.New(reg, nil)
	}
	cleanup := func() {
		flushOwnedTables(t, c, owner) // API routes in tables named "<owner>:…" first (V15), then everything else
		if r := newSched().Apply(context.Background(), nil, nil); r.Outcome != scheduler.OutcomeApplied {
			t.Errorf("cleanup: %s %v", r.Outcome, r.Err)
		}
	}
	cleanup() // leftovers of an aborted earlier run
	t.Cleanup(cleanup)
	s := newSched()

	p := func(f string, a ...any) string { return fmt.Sprintf(f, a...) }
	a1, a2, a6 := p("10.%d.221.1/24", slot), p("10.%d.222.1/24", slot), p("2001:db8:%d:222::1/64", slot)
	ecmp, leak, leak6, bh := "0.0.0.0/0", p("10.%d.60.0/24", slot), p("2001:db8:%d:60::/64", slot), p("10.%d.70.0/24", slot)
	src4, src6 := p("10.%d.50.0/24", slot), p("2001:db8:%d:50::/64", slot)
	desired := []scheduler.KV{
		{Key: core.VRFKey(red), Value: &core.Table{Id: red, Vrf: "red"}},
		{Key: core.VRFKey(blue), Value: &core.Table{Id: blue, Vrf: "blue"}},
		{Key: core.LoopbackKey(l1), Value: &core.Loopback{Name: l1, Instance: i1}},
		{Key: core.LoopbackKey(l2), Value: &core.Loopback{Name: l2, Instance: i2}},
		{Key: core.InterfaceTableKey(l1), Value: &core.InterfaceTable{Interface: l1, TableId: red}},
		{Key: core.InterfaceAddrKey(l1, a1), Value: &core.InterfaceAddress{Interface: l1, Prefix: a1}},
		{Key: core.InterfaceAddrKey(l2, a2), Value: &core.InterfaceAddress{Interface: l2, Prefix: a2}},
		{Key: core.InterfaceAddrKey(l2, a6), Value: &core.InterfaceAddress{Interface: l2, Prefix: a6}},
		// weighted ECMP default route in red (3:1)
		{Key: core.RouteKey(red, ecmp), Value: &core.Route{TableId: red, Prefix: ecmp, Preference: 1, Paths: []*core.RoutePath{
			{Address: p("10.%d.221.2", slot), Weight: 3}, {Address: p("10.%d.221.3", slot), Weight: 1}}}},
		// next hops in another VRF (default) from red, IPv4 and IPv6
		{Key: core.RouteKey(red, leak), Value: &core.Route{TableId: red, Prefix: leak, Paths: []*core.RoutePath{
			{Address: p("10.%d.222.2", slot), Weight: 1, NextHopTable: proto.Uint32(0)}}}},
		{Key: core.RouteKey(red, leak6), Value: &core.Route{TableId: red, Prefix: leak6, Paths: []*core.RoutePath{
			{Address: p("2001:db8:%d:222::2", slot), Weight: 1, NextHopTable: proto.Uint32(0)}}}},
		// blackhole in blue
		{Key: core.RouteKey(blue, bh), Value: &core.Route{TableId: blue, Prefix: bh}},
		// source VRF select: sources src4/src6 arriving on l2 (default VRF) are routed in red
		{Key: svs.TableKey(st), Value: &svs.Table{Id: st}},
		{Key: svs.InterfaceKey(l2), Value: &svs.Interface{Interface: l2, TableId: st}},
		{Key: svs.RouteKey(st, src4), Value: &svs.Route{TableId: st, Prefix: src4, SourceTableId: red}},
		{Key: svs.RouteKey(st, src6), Value: &svs.Route{TableId: st, Prefix: src6, SourceTableId: red}},
	}
	start := time.Now()
	r := s.Apply(ctx, desired, nil)
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %s %v %+v", r.Outcome, r.Err, r.Results)
	}
	t.Logf("apply: %+v in %v", r.Summary, time.Since(start))
	checkRetrieve(t, s, desired)
	t.Logf("vppctl show ip fib table %d %s:\n%s", red, ecmp, vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(red), ecmp))
	out := vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(red), ecmp)
	if !strings.Contains(out, "weight=3") || !strings.Contains(out, "weight=1") {
		t.Errorf("show ip fib: both weighted paths expected")
	}
	t.Logf("vppctl show ip fib table %d %s:\n%s", red, leak, vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(red), leak))
	t.Logf("vppctl show ip6 fib table %d %s:\n%s", red, leak6, vppctl(t, "show", "ip6", "fib", "table", fmt.Sprint(red), leak6))
	t.Logf("vppctl show ip fib table %d %s:\n%s", blue, bh, vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(blue), bh))
	t.Logf("vppctl show svs:\n%s", vppctl(t, "show", "svs"))
	t.Logf("vppctl show ip fib table %d %s:\n%s", st, src4, vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(st), src4))
	if svsOut := vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(st), src4); !strings.Contains(svsOut, "svs") {
		t.Errorf("svs entry not in table %d", st)
	}
	if r := s.Apply(ctx, desired, nil); r.Outcome != scheduler.OutcomeApplied || !r.Plan.Empty() {
		t.Fatalf("second apply not empty: %s %+v", r.Outcome, r.Plan.Ops)
	}

	// simulated loss behind the reconciler's back (binapi, our objects only), then a resync recreates them
	idx := swIfIndex(t, c, l2)
	for _, v6 := range []bool{false, true} {
		if _, err := svsapi.NewServiceClient(c).SvsEnableDisable(ctx, &svsapi.SvsEnableDisable{IsEnable: false, Af: afOf(v6), TableID: st, SwIfIndex: idx}); err != nil {
			t.Fatal(err)
		}
	}
	for _, pfx := range []string{src4, src6} {
		pp, _ := ip_types.ParsePrefix(pfx)
		if _, err := svsapi.NewServiceClient(c).SvsRouteAddDel(ctx, &svsapi.SvsRouteAddDel{IsAdd: false, Prefix: pp, TableID: st}); err != nil {
			t.Fatal(err)
		}
	}
	for _, v6 := range []bool{false, true} {
		if _, err := ip.NewServiceClient(c).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: st, IsIP6: v6}}); err != nil {
			t.Fatal(err)
		}
	}
	lost, err := s.Retrieve(ctx, scheduler.Only(svs.Names()...))
	if err != nil || len(lost) != 0 {
		t.Fatalf("after the loss retrieve = %v %v", lost, err)
	}
	start = time.Now()
	s2 := newSched() // a fresh reconciler (agent restart); the BootStore persists
	r = s2.ApplyWith(ctx, desired, nil, scheduler.ApplyOptions{Resync: true})
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("resync: %s %v", r.Outcome, r.Err)
	}
	t.Logf("resync after the simulated loss: %+v in %v", r.Summary, time.Since(start))
	checkRetrieve(t, s2, desired)

	// rollback to nothing: routes before tables (V15)
	r = s2.Apply(ctx, nil, nil)
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("rollback: %s %v", r.Outcome, r.Err)
	}
	if left, err := s2.Retrieve(ctx, nil); err != nil || len(left) != 0 {
		t.Fatalf("after rollback retrieve = %v %v", left, err)
	}
	t.Logf("after rollback, vppctl show ip fib table %d:\n%s", red, vppctl(t, "show", "ip", "fib", "table", fmt.Sprint(red)))
	t.Logf("after rollback, vppctl show svs:\n%s", vppctl(t, "show", "svs"))
	// V15: a table re-created with the same ids holds only VPP's default entries (no leaked /32, drop or svs entry)
	for _, id := range []uint32{red, blue, st} {
		n, entries := probeTable(t, c, id, owner+":v15probe")
		t.Logf("V15 probe: table %d re-created holds %d entries: %s", id, n, entries)
		if n != 5 {
			t.Errorf("V15: table %d re-created with %d entries (want VPP's 5 defaults): %s", id, n, entries)
		}
	}
	// and no recursive-resolution /32 of our next hops is left in table 0
	if out := vppctl(t, "show", "ip", "fib", "table", "0", p("10.%d.222.2/32", slot)); strings.Contains(out, "recursive-resolution") {
		t.Errorf("stray recursive-resolution entry in table 0:\n%s", out)
	}
}

func checkRetrieve(t *testing.T, s *scheduler.Scheduler, desired []scheduler.KV) {
	t.Helper()
	actual, err := s.Retrieve(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[scheduler.Key]proto.Message{}
	for _, kv := range actual {
		byKey[kv.Key] = kv.Value
	}
	if len(actual) != len(desired) {
		t.Errorf("retrieve returned %d objects, want %d: %v", len(actual), len(desired), actual)
	}
	for _, kv := range desired {
		if !proto.Equal(byKey[kv.Key], kv.Value) {
			t.Errorf("%s: retrieved %v, want %v", kv.Key, byKey[kv.Key], kv.Value)
		}
	}
	if !t.Failed() {
		t.Logf("Retrieve == desired (%d objects)", len(desired))
	}
}

// probeTable creates table id (IPv4) under a probe name, returns its entries and deletes it again.
func probeTable(t *testing.T, c vpp.Client, id uint32, name string) (int, string) {
	t.Helper()
	ctx := context.Background()
	cl := ip.NewServiceClient(c)
	if _, err := cl.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: id, Name: name}}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = cl.IPTableAddDel(context.Background(), &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: id}})
	}()
	stream, err := cl.IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Table: ip.IPTable{TableID: id}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for {
		d, err := stream.Recv()
		if err != nil {
			break
		}
		names = append(names, fmt.Sprintf("%s(src %d)", d.Route.Prefix, d.Route.Src))
	}
	return len(names), strings.Join(names, " ")
}

func afOf(v6 bool) ip_types.AddressFamily {
	if v6 {
		return ip_types.ADDRESS_IP6
	}
	return ip_types.ADDRESS_IP4
}

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("vppctl", args...).CombinedOutput() //nolint:gosec // fixed test arguments
	if err != nil {
		t.Fatalf("vppctl %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func nRestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts", "--value").Output()
	if err != nil {
		t.Fatalf("systemctl show vpp: %v", err)
	}
	return strings.TrimSpace(string(out))
}

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

func swIfIndex(t *testing.T, c vpp.Client, name string) interface_types.InterfaceIndex {
	t.Helper()
	tbl, err := iface.Dump(context.Background(), c, vpptest.Prefix(t)+"s")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tbl.IndexByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return interface_types.InterfaceIndex(idx)
}

// flushOwnedTables deletes, via binapi, every API-source route in the FIB tables named "<owner>:…" (both families):
// what a crashed run can leave behind; never table 0, never another owner's table.
func flushOwnedTables(t *testing.T, c vpp.Client, owner string) {
	t.Helper()
	ctx := context.Background()
	cl := ip.NewServiceClient(c)
	tables, err := cl.IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	type fam struct {
		id uint32
		v6 bool
	}
	var mine []fam
	for {
		d, err := tables.Recv()
		if err != nil {
			break
		}
		if d.Table.TableID != 0 && strings.HasPrefix(strings.TrimRight(d.Table.Name, "\x00"), owner+":") {
			mine = append(mine, fam{d.Table.TableID, d.Table.IsIP6})
		}
	}
	for _, f := range mine {
		stream, err := cl.IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Src: 8, Table: ip.IPTable{TableID: f.id, IsIP6: f.v6}})
		if err != nil {
			t.Fatal(err)
		}
		var routes []ip.IPRouteV2
		for {
			d, err := stream.Recv()
			if err != nil {
				break
			}
			routes = append(routes, d.Route)
		}
		for _, r := range routes {
			if _, err := cl.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: false, Route: ip.IPRoute{TableID: f.id, Prefix: r.Prefix}}); err != nil {
				t.Logf("flush %d %s: %v", f.id, r.Prefix, err)
			}
		}
	}
}
