package vxlan_test

import (
	"context"
	"fmt"
	"testing"

	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestBypassInterfaceRecreatedOnHost (fix round 2, N1 regression mirroring the reviewer's
// probe): the interface under a write-only bypass is lost and recreated (new sw_if_index) on
// the SAME VPP boot; the re-apply must enable the feature on the new interface.
func TestBypassInterfaceRecreatedOnHost(t *testing.T) {
	h := df6test.Connect(t)
	inst := vpptest.LoopbackInstance(t, 30)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(h.Client)
	mkInst := func(inst uint32) interface_types.InterfaceIndex {
		name := fmt.Sprintf("loop%d", inst)
		rep, err := svc.CreateLoopbackInstance(h.Ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		tag, _ := vpp.OwnerTag(h.Owner, name)
		if _, err := svc.SwInterfaceTagAddDel(h.Ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
			t.Fatal(err)
		}
		return rep.SwIfIndex
	}
	mk := func() interface_types.InterfaceIndex { return mkInst(inst) }
	del := func(idx interface_types.InterfaceIndex) {
		_, _ = svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: idx})
	}
	enabled := func(idx interface_types.InterfaceIndex) bool {
		rep, err := feature.NewServiceClient(h.Client).FeatureIsEnabled(h.Ctx, &feature.FeatureIsEnabled{ArcName: "ip4-unicast", FeatureName: "ip4-vxlan-bypass", SwIfIndex: idx})
		if err != nil {
			t.Fatalf("feature_is_enabled: %v", err)
		}
		return rep.IsEnabled
	}
	// a filler loopback makes sure the recreated interface gets a different sw_if_index
	// leftovers of an interrupted run (ours by tag only)
	if ifs, err := df6.DumpInterfaces(h.Ctx, h.Client, h.Owner); err == nil {
		for _, n := range []string{name, fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 31)), fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 32))} {
			if idx, ok := ifs.IndexByTag(n); ok {
				del(interface_types.InterfaceIndex(idx))
			}
		}
	}
	filler := mkInst(vpptest.LoopbackInstance(t, 31))
	t.Cleanup(func() { del(filler) })
	first := mk()
	del(filler)
	cur := first
	t.Cleanup(func() { del(cur) })

	d := vxlan.NewBypass(h.Client, h.Owner)
	obj := &vxlan.Bypass{Interface: name, Ipv4: true}
	t.Cleanup(func() { _ = d.Delete(context.Background(), obj, nil) })
	if _, err := d.Create(h.Ctx, obj); err != nil {
		t.Fatal(err)
	}
	if !enabled(first) {
		t.Fatal("bypass not enabled on the first interface")
	}
	prev := first
	for round, forceNew := range []bool{false, true} {
		del(prev) // lost behind the agent's back, VPP keeps running
		if forceNew {
			f2 := mkInst(vpptest.LoopbackInstance(t, 32)) // occupies the freed index
			t.Cleanup(func() { del(f2) })
		}
		cur = mk()
		t.Logf("round %d: %s old sw_if_index %d, recreated as %d (same VPP boot, index reused=%v)", round, name, prev, cur, cur == prev)
		for i := 0; i < 2; i++ { // two resyncs
			if _, err := d.Create(h.Ctx, obj); err != nil {
				t.Fatal(err)
			}
		}
		if !enabled(cur) {
			t.Fatalf("round %d: feature not re-enabled on the recreated interface (review N1)", round)
		}
		t.Logf("round %d: ip4-unicast/ip4-vxlan-bypass enabled on sw_if_index %d: true", round, cur)
		prev = cur
	}
	if err := d.Delete(h.Ctx, obj, nil); err != nil {
		t.Fatal(err)
	}
	if enabled(cur) {
		t.Fatal("feature still enabled after delete")
	}
}
