package gso_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	gsoapi "ngfw/agent/binapi/gso"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/vpp/vpptest"
)

// Host test (VRX_INTEGRATION=1, shared lab lock): gso.interface on one of this slot's tagged loopbacks
// (loop<slot>60). VPP stacks the feature on every enable, so the proof that a re-applied Create is idempotent
// is that ONE Delete after three Creates leaves the feature off (feature_is_enabled), and Retrieve reports the
// object exactly while it is on. No packets are sent.
func TestGSOOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "gso", &gsoapi.FeatureGsoEnableDisable{}, &gsoapi.FeatureGsoEnableDisableReply{})
	c := h.Client()
	name, idx := h.Loopback(t, 60)
	d := gso.New(c, h.Owner, dfkit.NewMemoryBootStore())
	desired := gso.Interface{Interface: name}.Proto()
	t.Cleanup(func() { // D-095c: the feature goes before the loopback (registered after it: runs first)
		_, _ = gsoapi.NewServiceClient(c).FeatureGsoEnableDisable(ctx, &gsoapi.FeatureGsoEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx)})
	})

	var meta any
	for i := 0; i < 3; i++ { // write-only style re-applies: must not stack
		m, err := d.Create(ctx, desired)
		if err != nil {
			t.Fatalf("create #%d: %v", i+1, err)
		}
		meta = m
	}
	on, err := gso.IsEnabled(ctx, c, idx)
	if err != nil || !on {
		t.Fatalf("feature_is_enabled after create: %v %v", on, err)
	}
	got := retrieve(t, d)
	if len(got) != 1 || !proto.Equal(got[gso.Key(name)], desired) {
		t.Fatalf("Retrieve != desired: %v", got)
	}
	t.Logf("Retrieve == desired: %s on %s (sw_if_index %d)", gso.Key(name), name, idx)
	dfkittest.HoldForEvidence(t, "gso on "+name)

	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if on, err := gso.IsEnabled(ctx, c, idx); err != nil || on {
		t.Fatalf("one delete after three creates left GSO on (stacked): %v %v", on, err)
	}
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("Retrieve after delete: %v", got)
	}
	t.Logf("after Delete: feature_is_enabled(ip4-output, gso-ip4, %d) = false, Retrieve empty", idx)

	// restart simulation: enabled, then disabled behind the agent's back → Create restores it once
	if _, err = d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if _, err := gsoapi.NewServiceClient(c).FeatureGsoEnableDisable(ctx, &gsoapi.FeatureGsoEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
		t.Fatal(err)
	}
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("lost GSO still reported: %v", got)
	}
	if meta, err = d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if got := retrieve(t, d); len(got) != 1 {
		t.Fatalf("not restored: %v", got)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if on, _ := gso.IsEnabled(ctx, c, idx); on {
		t.Fatal("restored GSO stacked twice")
	}
}

// V21 check (no packets): VPP clears every vnet feature arc of an interface on its deletion
// (vnet_feature_add_del_sw_interface), so a GSO enable left on a deleted interface is NOT inherited by the
// next interface that reuses the sw_if_index — unlike span / lldp bookkeeping (docs/vpp-code-track.md V-new).
func TestGSONotInheritedOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "gso", &gsoapi.FeatureGsoEnableDisable{}, &gsoapi.FeatureGsoEnableDisableReply{})
	c := h.Client()
	svc := interfaces.NewServiceClient(c)
	inst := vpptest.LoopbackInstance(t, 66)
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatal(err)
	}
	idx := rep.SwIfIndex
	if _, err := gsoapi.NewServiceClient(c).FeatureGsoEnableDisable(ctx, &gsoapi.FeatureGsoEnableDisable{SwIfIndex: idx, EnableDisable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: idx}); err != nil {
		t.Fatal(err)
	}
	name, idx2 := h.Loopback(t, 67) // tagged, deleted in Cleanup
	if interface_types.InterfaceIndex(idx2) != idx {
		t.Skipf("sw_if_index %d was not reused (%s got %d): nothing to check", idx, name, idx2)
	}
	on, err := gso.IsEnabled(ctx, c, idx2)
	if err != nil || on {
		t.Fatalf("GSO inherited by %s on the reused sw_if_index %d: %v %v", name, idx2, on, err)
	}
	t.Logf("GSO enabled on loop%d (sw_if_index %d), loopback deleted without disabling; %s reused the index: gso-ip4 off (feature arcs are cleared on delete)", inst, idx, name)
}
