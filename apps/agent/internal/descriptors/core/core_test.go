package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

type rig struct {
	vpp   *coretest.VPP
	owned *ownertable.Memory
	s     *scheduler.Scheduler
	reg   *scheduler.MapRegistry
}

func newRig(t *testing.T, owner string, v *coretest.VPP) *rig {
	t.Helper()
	if v == nil {
		v = coretest.New()
	}
	r := &rig{vpp: v, owned: ownertable.NewMemory(), reg: scheduler.NewRegistry()}
	core.Register(r.reg, core.Env{Client: v, Owner: owner, Owned: r.owned})
	r.s = scheduler.New(r.reg, nil)
	r.s.VerifyRetries = 0
	return r
}

func (r *rig) desc(name string) scheduler.Descriptor {
	d, _ := r.reg.Get(name)
	return d
}

func sample() []scheduler.KV {
	return []scheduler.KV{
		{Key: "vrf/7001", Value: &core.Table{Id: 7001, Vrf: "red"}},
		{Key: "interface.loopback/loop701", Value: &core.Loopback{Name: "loop701", Instance: 701}},
		{Key: "interface.loopback/loop702", Value: &core.Loopback{Name: "loop702", Instance: 702}},
		{Key: "interface-ip.table/loop701", Value: &core.InterfaceTable{Interface: "loop701", TableId: 7001}},
		{Key: "interface-ip/loop701/10.7.1.1/24", Value: &core.InterfaceAddress{Interface: "loop701", Prefix: "10.7.1.1/24"}},
		{Key: "interface-ip/loop701/2001:db8:7::1/64", Value: &core.InterfaceAddress{Interface: "loop701", Prefix: "2001:db8:7::1/64"}},
		{Key: "interface-ip/loop702/10.7.2.1/24", Value: &core.InterfaceAddress{Interface: "loop702", Prefix: "10.7.2.1/24"}},
		{Key: "ip.route/7001/10.7.100.0/24", Value: &core.Route{TableId: 7001, Prefix: "10.7.100.0/24", Preference: 1,
			Paths: []*core.RoutePath{{Address: "10.7.1.253", Interface: "loop701", Weight: 2}, {Address: "10.7.1.254", Weight: 1}}}},
		{Key: "ip.route/0/10.7.200.0/24", Value: &core.Route{TableId: 0, Prefix: "10.7.200.0/24"}},
		{Key: "ip.route/7001/2001:db8:70::/48", Value: &core.Route{TableId: 7001, Prefix: "2001:db8:70::/48", Paths: []*core.RoutePath{{Address: "2001:db8:7::fe", Weight: 1}}}},
	}
}

func TestKeysAndDependencies(t *testing.T) {
	r := newRig(t, "w7", nil)
	for _, kv := range sample() {
		d, ok := r.reg.ForKey(kv.Key)
		if !ok {
			t.Fatalf("no descriptor for %s", kv.Key)
		}
		if got := d.KeyOf(kv.Value); got != kv.Key {
			t.Errorf("KeyOf = %s, want %s", got, kv.Key)
		}
	}
	lb := r.desc(core.LoopbackName).(scheduler.KeyProvider)
	if k := lb.ProvidedKeys(&core.Loopback{Name: "loop701"}); len(k) != 1 || k[0] != "interface/loop701" {
		t.Fatalf("loopback alias %v", k)
	}
	deps := r.desc(core.InterfaceAddrName).Dependencies(&core.InterfaceAddress{Interface: "loop701", Prefix: "10.7.1.1/24"})
	if len(deps) != 2 || deps[0] != (scheduler.Dependency{Key: "interface/loop701"}) || deps[1] != (scheduler.Dependency{Key: "interface-ip.table/loop701", Optional: true}) {
		t.Fatalf("interface-ip deps %v", deps)
	}
	deps = r.desc(core.InterfaceTableName).Dependencies(&core.InterfaceTable{Interface: "loop701", TableId: 7001})
	if len(deps) != 2 || deps[0].Key != "interface/loop701" || deps[1].Key != "vrf/7001" || deps[1].Optional {
		t.Fatalf("interface-ip.table deps %v", deps)
	}
	deps = r.desc(core.RouteName).Dependencies(&core.Route{TableId: 7001, Paths: []*core.RoutePath{{Interface: "loop701"}, {Interface: "loop701"}}})
	if len(deps) != 2 || deps[0] != (scheduler.Dependency{Key: "vrf/7001"}) || deps[1] != (scheduler.Dependency{Key: "interface/loop701", Optional: true}) {
		t.Fatalf("route deps %v", deps)
	}
	if deps = r.desc(core.RouteName).Dependencies(&core.Route{TableId: 0}); len(deps) != 0 {
		t.Fatalf("table-0 route deps %v", deps)
	}
}

func TestApplyRetrieveIdempotent(t *testing.T) {
	r := newRig(t, "w7", nil)
	ctx := context.Background()
	res := r.s.Apply(ctx, sample(), nil)
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Created != len(sample()) {
		t.Fatalf("apply %s %+v %v", res.Outcome, res.Summary, res.Err)
	}
	// Exact requests.
	cl := r.vpp.CallsNamed("create_loopback_instance")
	if len(cl) != 2 || !cl[0].(*interfaces.CreateLoopbackInstance).IsSpecified || cl[0].(*interfaces.CreateLoopbackInstance).UserInstance != 701 {
		t.Fatalf("create_loopback_instance %+v", cl)
	}
	tags := r.vpp.CallsNamed("sw_interface_tag_add_del")
	if len(tags) != 2 || tags[0].(*interfaces.SwInterfaceTagAddDel).Tag != "w7:loop701" {
		t.Fatalf("tags %+v", tags)
	}
	tbl := r.vpp.CallsNamed("ip_table_add_del")
	if len(tbl) != 2 || tbl[0].(*ip.IPTableAddDel).Table.Name != "w7:red" || tbl[0].(*ip.IPTableAddDel).Table.IsIP6 || !tbl[1].(*ip.IPTableAddDel).Table.IsIP6 {
		t.Fatalf("tables %+v", tbl)
	}
	st := r.vpp.CallsNamed("sw_interface_set_table")
	if len(st) != 2 || st[0].(*interfaces.SwInterfaceSetTable).VrfID != 7001 {
		t.Fatalf("set_table %+v", st)
	}
	if !r.owned.Has("ip.route/7001/10.7.100.0/24") || !r.owned.Has("ip.route/0/10.7.200.0/24") {
		t.Fatalf("owner table %v", r.owned.Keys(""))
	}
	// Retrieve equals desired.
	actual, err := r.s.Retrieve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[scheduler.Key]proto.Message{}
	for _, kv := range sample() {
		want[kv.Key] = kv.Value
	}
	if len(actual) != len(want) {
		t.Fatalf("retrieve %d objects: %v", len(actual), actual)
	}
	for _, kv := range actual {
		if !proto.Equal(kv.Value, want[kv.Key]) {
			t.Errorf("%s: got %v want %v", kv.Key, kv.Value, want[kv.Key])
		}
	}
	// Idempotent.
	r.vpp.Reset()
	res = r.s.Apply(ctx, sample(), nil)
	if res.Outcome != scheduler.OutcomeApplied || !res.Plan.Empty() {
		t.Fatalf("second apply %s %+v", res.Outcome, res.Plan.Ops)
	}
	for _, c := range r.vpp.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("idempotent apply sent %s", n)
		}
	}
}

func TestOtherOwnerInvisible(t *testing.T) {
	v := coretest.New()
	a := newRig(t, "w7", v)
	b := newRig(t, "w7x", v)
	ctx := context.Background()
	if res := a.s.Apply(ctx, sample(), nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	before := v.Snapshot()
	if kvs, err := b.s.Retrieve(ctx, nil); err != nil || len(kvs) != 0 {
		t.Fatalf("w7x sees %v %v", kvs, err)
	}
	if res := b.s.Apply(ctx, nil, nil); res.Outcome != scheduler.OutcomeApplied || !res.Plan.Empty() {
		t.Fatalf("w7x empty apply %s %+v", res.Outcome, res.Plan.Ops)
	}
	// b creates its own VRF with another id and a loopback; a's Retrieve is unchanged.
	if res := b.s.Apply(ctx, []scheduler.KV{
		{Key: "vrf/7500", Value: &core.Table{Id: 7500, Vrf: "red"}},
		{Key: "interface.loopback/loop750", Value: &core.Loopback{Name: "loop750", Instance: 750}},
	}, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	if p, _ := a.s.Plan(ctx, sample(), nil); !p.Empty() {
		t.Fatalf("a's plan after b's apply: %+v", p.Ops)
	}
	if res := b.s.Apply(ctx, nil, nil); res.Outcome != scheduler.OutcomeApplied || res.Summary.Deleted != 2 {
		t.Fatalf("b cleanup %s %+v", res.Outcome, res.Summary)
	}
	if v.Snapshot() != before {
		t.Fatalf("a's objects changed:\n%s\n%s", before, v.Snapshot())
	}
}

func TestUpdatesAndRecreate(t *testing.T) {
	r := newRig(t, "w7", nil)
	ctx := context.Background()
	if res := r.s.Apply(ctx, sample(), nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	// Route path change → in-place update.
	d := sample()
	d[7].Value = &core.Route{TableId: 7001, Prefix: "10.7.100.0/24", Paths: []*core.RoutePath{{Address: "10.7.1.1", Weight: 1}}}
	res := r.s.Apply(ctx, d, nil)
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Updated != 1 || res.Results[0].Op != scheduler.OpUpdate {
		t.Fatalf("route update %s %+v %v", res.Outcome, res.Results, res.Err)
	}
	// VRF rename → recreate cascades: interface-ip.table, its addresses and routes in the table.
	d[0].Value = &core.Table{Id: 7001, Vrf: "blue"}
	res = r.s.Apply(ctx, d, nil)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("vrf rename %s %v %+v", res.Outcome, res.Err, res.Results)
	}
	keys := map[scheduler.Key]string{}
	for _, x := range res.Results {
		keys[x.Key] = x.Op
	}
	for _, k := range []scheduler.Key{"vrf/7001", "interface-ip.table/loop701", "interface-ip/loop701/10.7.1.1/24", "ip.route/7001/10.7.100.0/24", "ip.route/7001/2001:db8:70::/48"} {
		if keys[k] != scheduler.OpRecreate {
			t.Errorf("%s not recreated: %v", k, keys)
		}
	}
	if p, _ := r.s.Plan(ctx, d, nil); !p.Empty() {
		t.Fatalf("not converged: %+v", p.Ops)
	}
	// Move loop702 into the VRF: new binding, address recreated around it.
	d = append(d, scheduler.KV{Key: "interface-ip.table/loop702", Value: &core.InterfaceTable{Interface: "loop702", TableId: 7001}})
	res = r.s.Apply(ctx, d, nil)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("bind loop702: %s %v %+v", res.Outcome, res.Err, res.Results)
	}
	if i, _ := r.vpp.InterfaceByName("loop702"); i.Table4 != 7001 || !i.Addrs["10.7.2.1/24"] {
		t.Fatalf("loop702 %+v", i)
	}
}

func TestRepairAfterLoss(t *testing.T) {
	r := newRig(t, "w7", nil)
	ctx := context.Background()
	if res := r.s.Apply(ctx, sample(), nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	r.vpp.DeleteInterface("loop701")
	r.vpp.DeleteTable(7001, true)
	p, err := r.s.Plan(ctx, sample(), nil)
	if err != nil || p.Empty() {
		t.Fatalf("loss not detected %v %+v", err, p)
	}
	res := r.s.Apply(ctx, sample(), nil)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("repair %s %v %+v", res.Outcome, res.Err, res.Results)
	}
	if !r.vpp.HasTable(7001, true) || !r.vpp.HasRoute(7001, "10.7.100.0/24") {
		t.Fatal("not repaired")
	}
	if p, _ := r.s.Plan(ctx, sample(), nil); !p.Empty() {
		t.Fatalf("not converged %+v", p.Ops)
	}
}

func TestDeleteAll(t *testing.T) {
	r := newRig(t, "w7", nil)
	ctx := context.Background()
	if res := r.s.Apply(ctx, sample(), nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	res := r.s.Apply(ctx, nil, nil)
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Deleted != len(sample()) {
		t.Fatalf("delete %s %+v %v", res.Outcome, res.Summary, res.Err)
	}
	if r.vpp.RouteCount() != 0 || r.vpp.HasTable(7001, false) || len(r.owned.Keys("")) != 0 {
		t.Fatalf("left over: %s owned %v", r.vpp.Snapshot(), r.owned.Keys(""))
	}
	if _, ok := r.vpp.InterfaceByName("loop701"); ok {
		t.Fatal("loop701 still there")
	}
}

func TestErrors(t *testing.T) {
	r := newRig(t, "w7", nil)
	ctx := context.Background()
	lb := r.desc(core.LoopbackName)
	// VPP retval → error; instance already in use (foreign loop701).
	r.vpp.AddInterface("loop701", "Loopback", "w3:loop701")
	if _, err := lb.Create(ctx, &core.Loopback{Name: "loop701", Instance: 701}); err == nil || !strings.Contains(err.Error(), "create_loopback_instance") {
		t.Fatalf("instance in use: %v", err)
	}
	if _, err := lb.Create(ctx, &core.Loopback{Name: "loop1", Instance: 2}); err == nil {
		t.Fatal("name/instance mismatch accepted")
	}
	// Not owned interface → interface-ip refuses.
	if _, err := r.desc(core.InterfaceAddrName).Create(ctx, &core.InterfaceAddress{Interface: "loop701", Prefix: "10.7.1.1/24"}); !errors.Is(err, core.ErrNotOwned) {
		t.Fatalf("foreign interface: %v", err)
	}
	// Route into a table that does not exist → VPP error, owner table entry rolled back.
	if _, err := r.desc(core.RouteName).Create(ctx, &core.Route{TableId: 7999, Prefix: "10.7.0.0/16"}); err == nil || r.owned.Has("ip.route/7999/10.7.0.0/16") {
		t.Fatalf("route in missing table: %v owned=%v", err, r.owned.Keys(""))
	}
	// Mixed families.
	if _, err := r.desc(core.RouteName).Create(ctx, &core.Route{Prefix: "10.7.0.0/16", Paths: []*core.RoutePath{{Address: "2001:db8::1"}}}); err == nil {
		t.Fatal("mixed family accepted")
	}
	// Table 0 is never managed.
	if _, err := r.desc(core.VRFName).Create(ctx, &core.Table{Id: 0, Vrf: "default"}); err == nil {
		t.Fatal("table 0 accepted")
	}
	// Disconnected.
	r.vpp.SetConnected(false)
	if _, err := r.s.Retrieve(ctx, nil); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected retrieve: %v", err)
	}
	res := r.s.Apply(ctx, sample(), nil)
	if res.Outcome != scheduler.OutcomeFailed || !errors.Is(res.Err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected apply %s %v", res.Outcome, res.Err)
	}
}

func TestCanon(t *testing.T) {
	if p, _ := core.CanonAddrPrefix("2001:DB8:0:0::1/64"); p != "2001:db8::1/64" {
		t.Fatal(p)
	}
	if p, _ := core.CanonNetPrefix("10.7.1.1/24"); p != "10.7.1.0/24" {
		t.Fatal(p)
	}
	if _, err := core.CanonNetPrefix("10.7.1.1"); err == nil {
		t.Fatal("no length accepted")
	}
	if a, _ := core.CanonAddr("::ffff:10.0.0.1"); a != "10.0.0.1" {
		t.Fatal(a)
	}
	if _, ok := core.LoopbackInstance("loop"); ok {
		t.Fatal("loop without number")
	}
	if n, ok := core.LoopbackInstance("loop701"); !ok || n != 701 {
		t.Fatal(n)
	}
}
