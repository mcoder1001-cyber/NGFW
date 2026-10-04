package det44_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	det44d "ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeDet struct {
	*fake.Client
	enabled  bool
	timeouts det44.Det44GetTimeoutsReply
	ifaces   map[uint32]*det44.Det44InterfaceDetails
	maps     []*det44.Det44MapDetails
	// arc is the ip4-unicast feature arc per sw_if_index (det44 nodes only, duplicates kept) as VPP
	// 26.06 builds it: det44_interface_add_del puts one node on the arc for an add AND for a delete
	// (enable=1 on both paths, src/plugins/nat/det44/det44.c); fixedVPP models the corrected delete.
	arc      map[uint32][]string
	fixedVPP bool
}

const (
	nodeIn  = "det44-in2out"
	nodeOut = "det44-out2in"
)

func (f *fakeDet) count(idx uint32, node string) int {
	n := 0
	for _, x := range f.arc[idx] {
		if x == node {
			n++
		}
	}
	return n
}

func (f *fakeDet) removeOne(idx uint32, node string) {
	for i, x := range f.arc[idx] {
		if x == node {
			f.arc[idx] = append(f.arc[idx][:i:i], f.arc[idx][i+1:]...)
			return
		}
	}
}

// rawDet44 is a det44_interface_add_del_feature behind the agent's back (the simulated loss).
func (f *fakeDet) rawDet44(t *testing.T, idx uint32, inside, add bool) {
	t.Helper()
	svc := det44.NewServiceClient(f)
	if _, err := svc.Det44InterfaceAddDelFeature(context.Background(), &det44.Det44InterfaceAddDelFeature{IsAdd: add, IsInside: inside, SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Fatalf("raw det44 add=%v: %v", add, err)
	}
}

func newFakeDet() *fakeDet {
	f := &fakeDet{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), ifaces: map[uint32]*det44.Det44InterfaceDetails{},
		timeouts: det44.Det44GetTimeoutsReply{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}, arc: map[uint32][]string{}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop901", Tag: "w9:loop901"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 4, InterfaceName: "GigabitEthernet0/8/0"}) // untagged: owned by claim only
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		switch {
		case uint32(r.SwIfIndex) > 4:
			return []api.Message{&feature.FeatureIsEnabledReply{Retval: int32(api.INVALID_SW_IF_INDEX)}}, nil
		case r.ArcName != "ip4-unicast" || (r.FeatureName != nodeIn && r.FeatureName != nodeOut):
			return nil, errors.New("fake: unexpected arc/feature " + r.ArcName + "/" + r.FeatureName)
		}
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: f.count(uint32(r.SwIfIndex), r.FeatureName) > 0}}, nil
	})
	f.On("feature_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureEnableDisable)
		if r.ArcName != "ip4-unicast" || (r.FeatureName != nodeIn && r.FeatureName != nodeOut) {
			return nil, errors.New("fake: unexpected arc/feature " + r.ArcName + "/" + r.FeatureName)
		}
		if r.Enable {
			f.arc[uint32(r.SwIfIndex)] = append(f.arc[uint32(r.SwIfIndex)], r.FeatureName)
		} else {
			f.removeOne(uint32(r.SwIfIndex), r.FeatureName) // VPP ignores the disable of a node that is not on the arc
		}
		return []api.Message{&feature.FeatureEnableDisableReply{}}, nil
	})
	f.On("det44_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44PluginEnableDisable)
		rep := &det44.Det44PluginEnableDisableReply{}
		if r.Enable == f.enabled {
			rep.Retval = 1
		}
		f.enabled = r.Enable
		return []api.Message{rep}, nil
	})
	f.On("det44_get_timeouts", func(api.Message) ([]api.Message, error) {
		t := f.timeouts
		return []api.Message{&t}, nil
	})
	f.On("det44_set_timeouts", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44SetTimeouts)
		f.timeouts = det44.Det44GetTimeoutsReply{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return []api.Message{&det44.Det44SetTimeoutsReply{}}, nil
	})
	f.On("det44_interface_add_del_feature", func(req api.Message) ([]api.Message, error) { // det44_interface_add_del (26.06)
		r := req.(*det44.Det44InterfaceAddDelFeature)
		idx := uint32(r.SwIfIndex)
		node := nodeOut
		if r.IsInside {
			node = nodeIn
		}
		_, held := f.ifaces[idx]
		if held == r.IsAdd { // "already enabled" / "not enabled on this interface"
			return []api.Message{&det44.Det44InterfaceAddDelFeatureReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		switch {
		case r.IsAdd:
			f.ifaces[idx] = &det44.Det44InterfaceDetails{SwIfIndex: r.SwIfIndex, IsInside: r.IsInside, IsOutside: !r.IsInside}
			f.arc[idx] = append(f.arc[idx], node)
		case f.fixedVPP:
			delete(f.ifaces, idx)
			f.removeOne(idx, node)
		default:
			delete(f.ifaces, idx)
			f.arc[idx] = append(f.arc[idx], node) // the 26.06 delete enables the node once more
		}
		return []api.Message{&det44.Det44InterfaceAddDelFeatureReply{}}, nil
	})
	f.On("det44_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if d, ok := f.ifaces[idx]; ok {
				out = append(out, d)
			}
		}
		return out, nil
	})
	f.On("det44_add_del_map", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44AddDelMap)
		if r.IsAdd {
			f.maps = append(f.maps, &det44.Det44MapDetails{InAddr: r.InAddr, InPlen: r.InPlen, OutAddr: r.OutAddr, OutPlen: r.OutPlen, SharingRatio: 4, PortsPerHost: 1000})
		} else {
			for i, m := range f.maps {
				if m.InAddr == r.InAddr && m.InPlen == r.InPlen {
					f.maps = append(f.maps[:i], f.maps[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&det44.Det44AddDelMapReply{}}, nil
	})
	f.On("det44_map_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.maps))
		for _, m := range f.maps {
			out = append(out, m)
		}
		return out, nil
	})
	f.Reply("det44_session_dump", &det44.Det44SessionDetails{InPort: 1000, OutPort: 2000, ExtAddr: [4]uint8{8, 8, 8, 8}, ExtPort: 53, State: 1, Expire: 30})
	f.Reply("det44_close_session_in", &det44.Det44CloseSessionInReply{})
	f.Reply("det44_close_session_out", &det44.Det44CloseSessionOutReply{})
	return f
}

var owner = natcommon.WithGlobalsOwner(true)

func TestDet44(t *testing.T) {
	f := newFakeDet()
	p := det44d.New(f, "w9", owner)
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	det44d.Register(reg, f, "w9", owner)
	if reg.Len() != 4 {
		t.Fatalf("registered %d", reg.Len())
	}
	en := natcommon.MustEncode(&det44d.EnableSpec{InsideVRF: 9001, OutsideVRF: 9002})
	nattest.AssertWriteOnly(t, p.Enable)
	for i := 0; i < 2; i++ { // re-apply on every resync is idempotent (retval 1 tolerated)
		if _, err := p.Enable.Create(ctx, en); err != nil || !f.enabled {
			t.Fatalf("enable #%d: %v", i, err)
		}
	}
	req := f.CallsNamed("det44_plugin_enable_disable")[0].(*det44.Det44PluginEnableDisable)
	if req.InsideVrf != 9001 || req.OutsideVrf != 9002 || !req.Enable {
		t.Fatalf("enable request %+v", req)
	}
	if deps := p.Enable.Dependencies(en); len(deps) != 2 || deps[0].Key != "vrf/9001" || deps[1].Key != "vrf/9002" {
		t.Fatalf("deps %+v", deps)
	}
	f.maps = append(f.maps, &det44.Det44MapDetails{InAddr: [4]uint8{10, 3, 0, 0}, InPlen: 24, OutAddr: [4]uint8{10, 3, 1, 0}, OutPlen: 30}) // w3's map
	// VRF change needs a disable (crashes VPP 26.06) → explicit error, no VPP call
	before := len(f.CallsNamed("det44_plugin_enable_disable"))
	if _, err := p.Enable.Update(ctx, en, natcommon.MustEncode(&det44d.EnableSpec{InsideVRF: 9003}), nil); !errors.Is(err, det44d.ErrVRFChangeUnsafe) {
		t.Fatalf("vrf change: %v", err)
	}
	if len(f.CallsNamed("det44_plugin_enable_disable")) != before {
		t.Fatal("update must not touch VPP")
	}
	// finding 6: the write-only enable never gets an Update; a Create with other VRFs than
	// this process enabled with is refused instead of reported as applied
	if _, err := p.Enable.Create(ctx, natcommon.MustEncode(&det44d.EnableSpec{InsideVRF: 9003})); !errors.Is(err, det44d.ErrVRFChangeUnsafe) {
		t.Fatalf("vrf change via create: %v", err)
	}
	// non-owner (D-071): requires only (evidence: a det44 map exists), never sends
	w3 := det44d.New(f, "w3")
	before = len(f.CallsNamed("det44_plugin_enable_disable"))
	if _, err := w3.Enable.Create(ctx, en); err != nil || w3.Enable.Delete(ctx, en, nil) != nil || len(f.CallsNamed("det44_plugin_enable_disable")) != before {
		t.Fatalf("non-owner enable must not touch VPP: %v", err)
	}
	if _, err := w3.Timeouts.Create(ctx, natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: 1, TCPEstablished: 2, TCPTransitory: 3, ICMP: 4})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("non-owner timeouts: %v", err)
	}

	tmo := natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: 10, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60})
	if len(nattest.Keys(t, p.Timeouts)) != 0 || nattest.Apply(t, p.Timeouts, tmo) != 1 || nattest.Apply(t, p.Timeouts, tmo) != 0 || f.timeouts.UDP != 10 {
		t.Fatal("timeouts")
	}
	if nattest.Apply(t, p.Timeouts) != 1 || f.timeouts.UDP != 300 {
		t.Fatal("timeouts delete → defaults")
	}

	f.ifaces[3] = &det44.Det44InterfaceDetails{SwIfIndex: 3, IsInside: true}
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	if nattest.Apply(t, p.Interface, in, out) != 2 || nattest.Apply(t, p.Interface, in, out) != 0 {
		t.Fatal("interfaces")
	}
	if !f.ifaces[1].IsInside || !f.ifaces[2].IsOutside {
		t.Fatalf("interface flags %+v %+v", f.ifaces[1], f.ifaces[2])
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 2 || keys[0] != "det44.interface/loop900/inside" || keys[1] != "det44.interface/loop901/outside" {
		t.Fatalf("keys %v", keys)
	}
	if _, err := p.Interface.Create(ctx, natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "x"})); err == nil {
		t.Fatal("bad side")
	}

	m := natcommon.MustEncode(&det44d.MapSpec{Inside: "10.9.10.7/24", Outside: "10.9.11.0/30"})
	if nattest.Apply(t, p.Map, m) != 1 || nattest.Apply(t, p.Map, m) != 0 {
		t.Fatal("map")
	}
	if keys := nattest.Keys(t, p.Map); len(keys) != 1 || keys[0] != "det44.map/10.9.10.0/24/10.9.11.0/30" {
		t.Fatalf("map keys %v (masked, foreign filtered)", keys)
	}
	r := f.CallsNamed("det44_add_del_map")[0].(*det44.Det44AddDelMap)
	if r.InPlen != 24 || r.OutPlen != 30 || r.InAddr != [4]uint8{10, 9, 10, 0} {
		t.Fatalf("map request %+v", r)
	}
	sess, err := p.Sessions(ctx, "10.9.10.1", 0, 0)
	if err != nil || len(sess) != 1 || sess[0].ExternalPort != 53 {
		t.Fatalf("sessions %+v %v", sess, err)
	}
	if err := p.CloseSessionIn(ctx, "10.9.10.1", 1000, "8.8.8.8", 53); err != nil {
		t.Fatal(err)
	}
	if err := p.CloseSessionOut(ctx, "10.9.11.1", 2000, "8.8.8.8", 53); err != nil {
		t.Fatal(err)
	}
	if nattest.Apply(t, p.Map) != 1 || nattest.Apply(t, p.Interface) != 2 || len(f.maps) != 1 || f.ifaces[3] == nil {
		t.Fatal("delete leftovers, foreign kept")
	}
	f.maps = nil
	delete(f.ifaces, 3)
	// Delete releases the singleton in the agent only: det44_plugin_enable_disable(disable)
	// crashes VPP 26.06 once an interface was removed, so it is never sent.
	if err := p.Enable.Delete(ctx, en, nil); err != nil || !f.enabled || len(f.CallsNamed("det44_plugin_enable_disable")) != before {
		t.Fatalf("release: %v enabled=%v", err, f.enabled)
	}
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatalf("re-create after release (already enabled tolerated): %v", err)
	}
}

// TestDet44InterfaceArcRepair (F-det44-cnat-fix): VPP 26.06's det44 delete puts one MORE det44 node
// on the ip4-unicast arc instead of removing one. The descriptor keeps the arc exact: one node per
// desired det44 interface, none after its delete, leftovers of deletes behind its back reported and
// removed by the reconciler, and Create never enables twice.
func TestDet44InterfaceArcRepair(t *testing.T) {
	f := newFakeDet()
	f.enabled = true
	p := det44d.New(f, "w9", owner)
	ctx := context.Background()
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	arcIs := func(what string, lanIn, lanOut, wanIn, wanOut int) {
		t.Helper()
		if got := [4]int{f.count(1, nodeIn), f.count(1, nodeOut), f.count(2, nodeIn), f.count(2, nodeOut)}; got != [4]int{lanIn, lanOut, wanIn, wanOut} {
			t.Fatalf("%s: arc (loop900 in/out, loop901 in/out) = %v, want %v (arcs %v)", what, got, [4]int{lanIn, lanOut, wanIn, wanOut}, f.arc)
		}
	}
	disables := func() int {
		n := 0
		for _, m := range f.CallsNamed("feature_enable_disable") {
			if m.(*feature.FeatureEnableDisable).Enable {
				t.Fatalf("the descriptor never enables a feature node itself: %+v", m)
			}
			n++
		}
		return n
	}

	// create → exactly one node per interface; re-apply changes nothing
	if nattest.Apply(t, p.Interface, in, out) != 2 || nattest.Apply(t, p.Interface, in, out) != 0 {
		t.Fatal("create / re-apply")
	}
	arcIs("after create", 1, 0, 0, 1)
	// Create on an interface det44 already holds on that side: no add is sent (never enables twice)
	adds := len(f.CallsNamed("det44_interface_add_del_feature"))
	if meta, err := p.Interface.Create(ctx, in); err != nil || meta.(det44d.IfMeta).SwIfIndex != 1 {
		t.Fatalf("idempotent create: %v %+v", err, meta)
	}
	if len(f.CallsNamed("det44_interface_add_del_feature")) != adds {
		t.Fatal("Create re-sent det44_interface_add_del_feature for an interface det44 already holds")
	}
	arcIs("after a second create", 1, 0, 0, 1)

	// delete (the document drops det44): the 26.06 delete leaves 2 nodes, the repair removes both
	if nattest.Apply(t, p.Interface) != 2 {
		t.Fatal("delete")
	}
	arcIs("after delete", 0, 0, 0, 0)
	if n := disables(); n != 4 {
		t.Fatalf("feature_enable_disable enable=0 calls = %d, want 4 (2 per interface)", n)
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 0 {
		t.Fatalf("Retrieve after delete: %v", keys)
	}

	// loss behind the agent's back (raw delete, as the restart test does): det44 forgets the
	// interfaces, 2 nodes stay on each arc → Retrieve reports leftovers, never the desired keys
	if nattest.Apply(t, p.Interface, in, out) != 2 {
		t.Fatal("re-create")
	}
	f.rawDet44(t, 1, true, false)
	f.rawDet44(t, 2, false, false)
	arcIs("after the raw delete", 2, 0, 0, 2)
	keys := nattest.Keys(t, p.Interface)
	if len(keys) != 2 || keys[0] != "det44.interface/loop900#leftover/inside" || keys[1] != "det44.interface/loop901#leftover/outside" {
		t.Fatalf("leftover keys %v", keys)
	}
	if deps := p.Interface.Dependencies(natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900" + det44d.LeftoverSuffix, Side: "inside"})); len(deps) != 2 || deps[1].Key != "interface/loop900" {
		t.Fatalf("a leftover depends on its real interface: %+v", deps)
	}
	// the resync: desired back + leftovers deleted → exactly one node each again
	if n := nattest.Apply(t, p.Interface, in, out); n != 4 {
		t.Fatalf("resync ops = %d, want 2 creates + 2 leftover deletes", n)
	}
	arcIs("after the resync", 1, 0, 0, 1)
	if keys := nattest.Keys(t, p.Interface); len(keys) != 2 || keys[0] != "det44.interface/loop900/inside" || keys[1] != "det44.interface/loop901/outside" {
		t.Fatalf("keys after the resync %v", keys)
	}

	// leftovers nobody wants: removed by the plain delete of the reconciler
	f.rawDet44(t, 1, true, false)
	f.rawDet44(t, 2, false, false)
	if nattest.Apply(t, p.Interface) != 2 {
		t.Fatal("leftover delete")
	}
	arcIs("after the leftover delete", 0, 0, 0, 0)
	if len(f.ifaces) != 0 {
		t.Fatalf("det44 pool %v", f.ifaces)
	}

	// leftovers of BOTH sides on one interface read like a VPP without det44 nodes (feature_is_enabled
	// casts "no such feature" to true): not trusted, nothing reported or repaired; the next Create
	// on it does not trust them either (documented limit: only outside churn produces this)
	f.arc[1] = []string{nodeIn, nodeIn, nodeOut, nodeOut}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 0 {
		t.Fatalf("both-sides leftovers reported: %v", keys)
	}
	sent := len(f.CallsNamed("feature_enable_disable"))
	if nattest.Apply(t, p.Interface, in) != 1 || len(f.CallsNamed("feature_enable_disable")) != sent {
		t.Fatalf("create on an interface with both-sides leftovers sent %d disables", len(f.CallsNamed("feature_enable_disable"))-sent)
	}
	arcIs("after a create over both-sides leftovers", 3, 2, 0, 0)
	f.arc[1] = []string{nodeIn}

	// a rollback re-creating a leftover sends nothing and keeps no claim of the leftover key
	left := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901" + det44d.LeftoverSuffix, Side: "outside"})
	before := len(f.Calls())
	if _, err := p.Interface.Create(ctx, left); err != nil || len(f.Calls()) != before {
		t.Fatalf("leftover re-create: %v (calls %d → %d)", err, before, len(f.Calls()))
	}
	if nattest.Apply(t, p.Interface) != 1 {
		t.Fatal("final delete")
	}
	arcIs("final", 0, 0, 0, 0)

	// foreign (w3-tagged) interfaces are never looked at; untagged ones only where this owner holds
	// the claim of the det44 interface object (D-071)
	f.arc[3] = []string{nodeIn, nodeIn}
	f.arc[4] = []string{nodeOut}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 0 {
		t.Fatalf("foreign / unclaimed leftovers reported: %v", keys)
	}
	nic := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "GigabitEthernet0/8/0", Side: "outside"})
	if nattest.Apply(t, p.Interface, nic) != 1 || f.count(4, nodeOut) != 1 {
		t.Fatalf("create on the untagged NIC (claimed by the create): arc %v", f.arc[4])
	}
	f.rawDet44(t, 4, false, false) // loss behind the agent's back: claim kept, 2 nodes, no det44 entry
	if keys := nattest.Keys(t, p.Interface); len(keys) != 1 || keys[0] != "det44.interface/GigabitEthernet0/8/0#leftover/outside" {
		t.Fatalf("claimed untagged leftover keys %v", keys)
	}
	if nattest.Apply(t, p.Interface) != 1 || f.count(4, nodeOut) != 0 || f.count(3, nodeIn) != 2 {
		t.Fatalf("untagged leftover repair: arcs %v (the foreign w3 arc must stay untouched)", f.arc)
	}

	// an interface that vanished meanwhile has no arc left: the repair is done, not an error
	if err := p.Interface.Delete(ctx, natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop999" + det44d.LeftoverSuffix, Side: "inside"}), det44d.IfMeta{SwIfIndex: 9}); err != nil {
		t.Fatalf("leftover delete on a gone interface: %v", err)
	}
}

// TestDet44InterfaceArcRepairFixedVPP: on a VPP whose det44 delete disables the node, the repair
// finds nothing to do (no feature_enable_disable at all) and the arcs stay exact.
func TestDet44InterfaceArcRepairFixedVPP(t *testing.T) {
	f := newFakeDet()
	f.enabled, f.fixedVPP = true, true
	p := det44d.New(f, "w9", owner)
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	for i := 0; i < 3; i++ {
		if nattest.Apply(t, p.Interface, in, out) != 2 || f.count(1, nodeIn) != 1 || f.count(2, nodeOut) != 1 {
			t.Fatalf("round %d create: arcs %v", i, f.arc)
		}
		if nattest.Apply(t, p.Interface) != 2 || len(f.arc[1]) != 0 || len(f.arc[2]) != 0 {
			t.Fatalf("round %d delete: arcs %v", i, f.arc)
		}
	}
	if n := len(f.CallsNamed("feature_enable_disable")); n != 0 {
		t.Fatalf("feature_enable_disable sent %d times on a fixed VPP", n)
	}
}

// TestDet44InterfaceUntrustedArc: a VPP whose feature_is_enabled says "enabled" for every node (no
// det44 nodes registered: VPP casts "no such feature" to true; the agent's in-memory test VPP
// answers the same for every node it does not model) and that has no feature_enable_disable at all:
// Create, Retrieve and Delete work as before the arc repair and never try to repair anything.
func TestDet44InterfaceUntrustedArc(t *testing.T) {
	f := newFakeDet()
	f.enabled = true
	f.On("feature_is_enabled", func(api.Message) ([]api.Message, error) {
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: true}}, nil
	})
	f.Fail("feature_enable_disable", errors.New("fake: no feature_enable_disable on this VPP"))
	p := det44d.New(f, "w9", owner)
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	if nattest.Apply(t, p.Interface, in, out) != 2 || nattest.Apply(t, p.Interface, in, out) != 0 {
		t.Fatal("create / re-apply")
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 2 || keys[0] != "det44.interface/loop900/inside" || keys[1] != "det44.interface/loop901/outside" {
		t.Fatalf("keys %v (no leftover may be reported)", keys)
	}
	if nattest.Apply(t, p.Interface) != 2 || len(f.ifaces) != 0 {
		t.Fatalf("delete: det44 pool %v", f.ifaces)
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 0 {
		t.Fatalf("keys after delete %v", keys)
	}
	if n := len(f.CallsNamed("feature_enable_disable")); n != 0 {
		t.Fatalf("feature_enable_disable sent %d times on an untrusted arc", n)
	}
}
