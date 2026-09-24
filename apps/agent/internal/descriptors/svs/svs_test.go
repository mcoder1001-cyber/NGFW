package svs_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// rig is core + svs on the coretest model, loopback references by creator key (no DF-1 alias here).
type rig struct {
	v    *coretest.VPP
	s    *scheduler.Scheduler
	reg  *scheduler.MapRegistry
	boot dfkit.BootStore
	pid  int
}

func newRig(t *testing.T, owner string, v *coretest.VPP, boot dfkit.BootStore) *rig {
	t.Helper()
	if v == nil {
		v = coretest.New().InstallVrfStaticEcmp()
	}
	if boot == nil {
		boot = dfkit.NewMemoryBootStore()
	}
	r := &rig{v: v, reg: scheduler.NewRegistry(), boot: boot, pid: 100}
	core.Register(r.reg, core.Env{Client: v, Owner: owner, Owned: ownertable.NewMemory(), IfRef: core.DirectInterfaceRef})
	svs.Register(r.reg, svs.Env{
		Client: v, Owner: owner, Boot: boot, IfRef: core.DirectInterfaceRef, IfTableRef: core.InterfaceTableKey,
		Identity: func(context.Context, vpp.Client) (bootid.Identity, error) {
			return bootid.Identity{BootID: "fake", PID: r.pid, StartTime: 1}, nil
		},
	})
	r.s = scheduler.New(r.reg, nil)
	r.s.VerifyRetries = 0
	return r
}

func (r *rig) apply(t *testing.T, desired []scheduler.KV) *scheduler.TxnResult {
	t.Helper()
	res := r.s.Apply(context.Background(), desired, nil)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %v %v", res.Outcome, res.Err)
	}
	return res
}

func (r *rig) retrieve(t *testing.T, names ...string) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := r.s.Retrieve(context.Background(), scheduler.Only(names...))
	if err != nil {
		t.Fatal(err)
	}
	out := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		out[kv.Key] = kv.Value
	}
	return out
}

// desired: VRF red (2001) on loop201, source 10.2.50.0/24 and 2001:db8:2:50::/64 arriving on loop201 → red.
func desired(source uint32) []scheduler.KV {
	return []scheduler.KV{
		{Key: "vrf/2001", Value: &core.Table{Id: 2001, Vrf: "red"}},
		{Key: "interface.loopback/loop201", Value: &core.Loopback{Name: "loop201", Instance: 201}},
		{Key: svs.TableKey(2999), Value: &svs.Table{Id: 2999}},
		{Key: svs.InterfaceKey("loop201"), Value: &svs.Interface{Interface: "loop201", TableId: 2999}},
		{Key: svs.RouteKey(2999, "10.2.50.0/24"), Value: &svs.Route{TableId: 2999, Prefix: "10.2.50.0/24", SourceTableId: source}},
		{Key: svs.RouteKey(2999, "2001:db8:2:50::/64"), Value: &svs.Route{TableId: 2999, Prefix: "2001:db8:2:50::/64", SourceTableId: source}},
	}
}

var svsNames = []string{svs.TableName, svs.InterfaceName, svs.RouteName}

func checkEqual(t *testing.T, got map[scheduler.Key]proto.Message, want []scheduler.KV) {
	t.Helper()
	n := 0
	for _, kv := range want {
		d := kv.Key.Descriptor()
		if d != svs.TableName && d != svs.InterfaceName && d != svs.RouteName {
			continue
		}
		n++
		if !proto.Equal(got[kv.Key], kv.Value) {
			t.Errorf("%s: retrieved %v, want %v", kv.Key, got[kv.Key], kv.Value)
		}
	}
	if len(got) != n {
		t.Errorf("retrieved %d svs objects, want %d: %v", len(got), n, got)
	}
}

func TestApplyRetrieveIdempotentAndDelete(t *testing.T) {
	r := newRig(t, "w2", nil, nil)
	r.apply(t, desired(2001))
	st := r.v.Svs()
	for _, p := range []string{"10.2.50.0/24", "2001:db8:2:50::/64"} {
		if e := routeEntry(r.v, 2999, p); e == nil || e.SourceTable != 2001 || e.Refs != 1 {
			t.Fatalf("svs route %s = %+v", p, e)
		}
	}
	if len(st.Enabled) != 2 {
		t.Fatalf("svs enabled %v, want IPv4 + IPv6 on loop201", st.Enabled)
	}
	if !r.v.HasTable(2999, false) || !r.v.HasTable(2999, true) {
		t.Fatal("svs table 2999 missing")
	}
	checkEqual(t, r.retrieve(t, svsNames...), desired(2001))
	// the VRF descriptor does not mistake the svs table ("w2:svs:2999") for a VRF
	if vrfs := r.retrieve(t, core.VRFName); len(vrfs) != 1 || vrfs["vrf/2001"] == nil {
		t.Fatalf("VRF retrieve %v", vrfs)
	}

	// idempotent: the same desired state again is an empty plan and nothing is added twice (D-076)
	res := r.apply(t, desired(2001))
	if len(res.Plan.Ops) != 0 {
		t.Fatalf("second apply planned %v", res.Plan.Ops)
	}
	for k, n := range st.SvsEnables {
		if n != 1 {
			t.Fatalf("svs enabled %d times on %v", n, k)
		}
	}

	// another selected table: delete + add (VPP keeps the first DPO of a repeated add)
	r.apply(t, desired(0))
	if e := routeEntry(r.v, 2999, "10.2.50.0/24"); e == nil || e.SourceTable != 0 || e.Refs != 1 {
		t.Fatalf("after source change %+v", e)
	}
	checkEqual(t, r.retrieve(t, svsNames...), desired(0))

	// delete everything: entries before tables (V15), nothing left
	r.apply(t, nil)
	if len(st.Routes) != 0 || len(st.Enabled) != 0 || r.v.HasTable(2999, false) || r.v.HasTable(2999, true) || r.v.HasTable(2001, false) {
		t.Fatalf("leftovers: routes %v enabled %v\n%s", st.Routes, st.Enabled, r.v.Snapshot())
	}
	for _, kv := range desired(2001) {
		if _, ok := r.boot.Get(string(kv.Key)); ok {
			t.Fatalf("record %s left", kv.Key)
		}
	}
}

func routeEntry(v *coretest.VPP, table uint32, prefix string) *coretest.SvsEntry {
	if e, ok := v.SvsRoute(table, prefix); ok {
		return &e
	}
	return nil
}

func TestRecordLostOrNewVPPReprogramsTheEntry(t *testing.T) {
	r := newRig(t, "w2", nil, nil)
	r.apply(t, desired(2001))

	// a new VPP boot identity: the records no longer vouch for the selected table
	r.pid = 101
	got := r.retrieve(t, svs.RouteName)
	for k, v := range got {
		if v.(*svs.Route).GetSourceTableId() != svs.UnknownTable {
			t.Fatalf("%s reported %v with a stale record", k, v)
		}
	}
	r.apply(t, desired(2001)) // re-programs (delete + add), records the new identity
	checkEqual(t, r.retrieve(t, svsNames...), desired(2001))
	if e := routeEntry(r.v, 2999, "10.2.50.0/24"); e == nil || e.Refs != 1 || e.SourceTable != 2001 {
		t.Fatalf("after re-program %+v", e)
	}

	// an agent whose store was lost (fresh BootStore): same convergence
	r2 := newRig(t, "w2", r.v, dfkit.NewMemoryBootStore())
	r2.apply(t, desired(2001))
	checkEqual(t, r2.retrieve(t, svsNames...), desired(2001))
	if e := routeEntry(r.v, 2999, "10.2.50.0/24"); e == nil || e.Refs != 1 {
		t.Fatalf("after store loss %+v", e)
	}
	r2.apply(t, nil)
}

func TestForeignTableAndInterfaceConflicts(t *testing.T) {
	v := coretest.New().InstallVrfStaticEcmp()
	v.SetTableName(2999, false, "w3:svs:2999")
	r := newRig(t, "w2", v, nil)
	res := r.s.Apply(context.Background(), desired(2001), nil)
	if res.Outcome == scheduler.OutcomeApplied || !errors.Is(res.Err, svs.ErrTableConflict) {
		t.Fatalf("foreign table: %v %v", res.Outcome, res.Err)
	}
	if n, ok := tableName(t, v, 2999, false); !ok || n != "w3:svs:2999" {
		t.Fatalf("foreign table touched: %q %v", n, ok)
	}
	// nothing of ours is left behind by the rolled-back transaction
	if len(v.Svs().Routes) != 0 || len(v.Svs().Enabled) != 0 || v.HasTable(2999, true) {
		t.Fatalf("rollback left %v %v\n%s", v.Svs().Routes, v.Svs().Enabled, v.Snapshot())
	}
	if w3 := r.retrieve(t, svsNames...); len(w3) != 0 {
		t.Fatalf("w2 retrieves w3's table: %v", w3)
	}
}

func tableName(t *testing.T, v *coretest.VPP, id uint32, v6 bool) (string, bool) {
	t.Helper()
	stream, err := ip.NewServiceClient(v).IPTableDump(context.Background(), &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := stream.Recv()
		if err != nil {
			return "", false
		}
		if d.Table.TableID == id && d.Table.IsIP6 == v6 {
			return d.Table.Name, true
		}
	}
}

func TestInterfaceRebindReenablesSvs(t *testing.T) {
	r := newRig(t, "w2", nil, nil)
	base := desired(2001)
	r.apply(t, base)
	// bind loop201 to red: the reconciler re-creates svs.interface around the new binding (optional dependency)
	withBind := append(append([]scheduler.KV(nil), base...), scheduler.KV{Key: "interface-ip.table/loop201", Value: &core.InterfaceTable{Interface: "loop201", TableId: 2001}})
	res := r.apply(t, withBind)
	recreated := false
	for _, op := range res.Results {
		if op.Key == svs.InterfaceKey("loop201") {
			recreated = true
		}
	}
	if !recreated {
		t.Fatalf("svs.interface not re-created around the VRF binding: %+v", res.Results)
	}
	for k, n := range r.v.Svs().SvsEnables {
		if n != 1 {
			t.Fatalf("svs enabled %d times on %v", n, k)
		}
	}
	r.apply(t, nil)
}

func TestAllocate(t *testing.T) {
	rng := svs.Range{Lo: 2900, Hi: 2999}
	a, err := svs.Allocate([]string{"wan", "lan", "lan.100"}, rng, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := svs.Allocate([]string{"lan.100", "wan", "lan"}, rng, nil)
	seen := map[uint32]bool{}
	for n, id := range a {
		if id < rng.Lo || id > rng.Hi || seen[id] || b[n] != id {
			t.Fatalf("allocation %v / %v", a, b)
		}
		seen[id] = true
	}
	// stable when another interface comes or goes (no collision in this set)
	c, _ := svs.Allocate([]string{"lan", "wan"}, rng, nil)
	if c["lan"] != a["lan"] || c["wan"] != a["wan"] {
		t.Fatalf("not stable: %v vs %v", c, a)
	}
	// declared VRF ids are skipped
	d, _ := svs.Allocate([]string{"lan"}, rng, func(id uint32) bool { return id == a["lan"] })
	if d["lan"] == a["lan"] {
		t.Fatal("taken id reused")
	}
	if _, err := svs.Allocate([]string{"a", "b", "c"}, svs.Range{Lo: 10, Hi: 11}, nil); !errors.Is(err, svs.ErrRangeFull) {
		t.Fatalf("full range: %v", err)
	}
	if r := svs.RangeIn(2000, 2999); r != (svs.Range{Lo: 2900, Hi: 2999}) {
		t.Fatalf("RangeIn %v", r)
	}
	ids := make([]uint32, 0, len(a))
	for _, id := range a {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	t.Logf("allocation in %v: %v", rng, a)
}

func TestCanonPrefixRefusesDefault(t *testing.T) {
	if _, err := svs.CanonPrefix("0.0.0.0/0"); err == nil {
		t.Fatal("/0 accepted")
	}
	if p, err := svs.CanonPrefix("10.2.50.7/24"); err != nil || p != "10.2.50.0/24" {
		t.Fatalf("%q %v", p, err)
	}
}
