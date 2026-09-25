package df6_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/vxlan"
)

// countingFake models a feature toggle whose enable does NOT deduplicate (gtpu, vxlan-gpe,
// l2tp decap, pppoe cp: vnet_feature_enable_disable stacks the node on every enable).
type countingFake struct {
	*df6test.FakeVPP
	count map[[2]uint32]int // {sw_if_index, is_ipv6} → times enabled
	// bm is vxlan's bypass bitmap, which VPP does not clear when an interface is deleted
	bm map[[2]uint32]bool
	// failV6Enable makes every ip6 enable fail (retval -9), for partial Creates (TD-11b)
	failV6Enable bool
}

func newCountingFake() *countingFake {
	f := &countingFake{FakeVPP: df6test.NewFakeVPP(), count: map[[2]uint32]int{}, bm: map[[2]uint32]bool{}}
	f.On("sw_interface_set_vxlan_bypass", func(req api.Message) ([]api.Message, error) {
		r := req.(*vxlanapi.SwInterfaceSetVxlanBypass)
		if f.failV6Enable && r.IsIPv6 && r.Enable {
			return []api.Message{&vxlanapi.SwInterfaceSetVxlanBypassReply{Retval: -9}}, nil
		}
		bk := [2]uint32{uint32(r.SwIfIndex), 0}
		if r.IsIPv6 {
			bk[1] = 1
		}
		if f.bm[bk] == r.Enable { // vnet_int_vxlan_bypass_mode: bitmap guard
			return []api.Message{&vxlanapi.SwInterfaceSetVxlanBypassReply{}}, nil
		}
		f.bm[bk] = r.Enable
		{
			arc, node := "ip4-unicast", "ip4-vxlan-bypass"
			if r.IsIPv6 {
				arc, node = "ip6-unicast", "ip6-vxlan-bypass"
			}
			f.SetFeature(arc, node, uint32(r.SwIfIndex), r.Enable)
		}
		k := [2]uint32{uint32(r.SwIfIndex), 0}
		if r.IsIPv6 {
			k[1] = 1
		}
		if r.Enable {
			f.count[k]++
		} else if f.count[k] > 0 {
			f.count[k]--
		}
		return []api.Message{&vxlanapi.SwInterfaceSetVxlanBypassReply{}}, nil
	})
	return f
}

// TestBypassIdempotentAcrossResyncs (review H3, D-076): P05 re-applies write-only objects on
// every resync; the feature must be enabled exactly once per VPP boot.
func TestBypassIdempotentAcrossResyncs(t *testing.T) {
	ctx := context.Background()
	f := newCountingFake()
	owner := "w11bp"
	iface.SetClaimStore(owner, nil)
	ours := f.AddInterface("loop1170", owner+":loop1170")
	phys := f.AddInterface("ens224", "")
	f.AddInterface("w3-tap1", "w3:w3-tap1")
	d := vxlan.NewBypass(f, owner)
	obj := &vxlan.Bypass{Interface: "loop1170", Ipv4: true, Ipv6: true}

	f.SetBoot(500)
	var meta any
	for i := 0; i < 3; i++ { // agent start + two resyncs
		m, err := d.Create(ctx, obj)
		if err != nil {
			t.Fatal(err)
		}
		meta = m
	}
	if f.count[[2]uint32{ours, 0}] != 1 || f.count[[2]uint32{ours, 1}] != 1 {
		t.Fatalf("after 3 applies: %v, want exactly one instance per family", f.count)
	}

	// Agent restart: fresh descriptor, same (persisted) claim store → still no re-add.
	d2 := vxlan.NewBypass(f, owner)
	if _, err := d2.Create(ctx, obj); err != nil {
		t.Fatal(err)
	}
	if f.count[[2]uint32{ours, 0}] != 1 {
		t.Fatalf("after agent restart: %v", f.count)
	}

	// VPP restart: features are gone, a new boot id → enabled exactly once again.
	f.count = map[[2]uint32]int{}
	f.ClearFeatures(ours)
	f.SetBoot(501)
	for i := 0; i < 2; i++ {
		if _, err := d2.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	if f.count[[2]uint32{ours, 0}] != 1 || f.count[[2]uint32{ours, 1}] != 1 {
		t.Fatalf("after VPP restart + 2 resyncs: %v", f.count)
	}

	// Update: turning ipv6 off disables it once.
	if _, err := d2.Update(ctx, obj, &vxlan.Bypass{Interface: "loop1170", Ipv4: true}, meta); err != nil {
		t.Fatal(err)
	}
	if f.count[[2]uint32{ours, 1}] != 0 || f.count[[2]uint32{ours, 0}] != 1 {
		t.Fatalf("after update: %v", f.count)
	}

	// Delete with a stale Meta index (VPP restart reused indexes) is refused, not misdirected.
	if err := d2.Delete(ctx, obj, df6.IfMeta{SwIfIndex: phys}); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("delete with stale meta = %v, want ErrNotOurs", err)
	}
	// Delete after an agent restart (no Meta) resolves by logical name.
	if err := d2.Delete(ctx, &vxlan.Bypass{Interface: "loop1170", Ipv4: true}, nil); err != nil {
		t.Fatal(err)
	}
	if f.count[[2]uint32{ours, 0}] != 0 {
		t.Fatalf("after delete: %v", f.count)
	}
	// A second delete sends nothing.
	before := len(f.CallsNamed("sw_interface_set_vxlan_bypass"))
	if err := d2.Delete(ctx, obj, nil); err != nil {
		t.Fatal(err)
	}
	if len(f.CallsNamed("sw_interface_set_vxlan_bypass")) != before {
		t.Fatal("second delete reached VPP")
	}

	// Foreign-tagged interfaces are refused (DF-1 resolver), VPP names of ours are not names.
	if _, err := d2.Create(ctx, &vxlan.Bypass{Interface: "w3-tap1", Ipv4: true}); !errors.Is(err, iface.ErrForeignInterface) {
		t.Fatalf("foreign = %v", err)
	}
	// Untagged (physical) interfaces are claimed; Delete of an unclaimed one never sends.
	if _, err := d2.Create(ctx, &vxlan.Bypass{Interface: "ens224", Ipv4: true}); err != nil {
		t.Fatal(err)
	}
	if !iface.Claims(owner).Claimed("ens224", vxlan.BypassName) {
		t.Fatal("untagged interface not claimed")
	}
	if err := d2.Delete(ctx, &vxlan.Bypass{Interface: "ens224", Ipv4: true}, nil); err != nil || f.count[[2]uint32{phys, 0}] != 0 {
		t.Fatalf("delete on claimed physical: %v %v", err, f.count)
	}
	other := vxlan.NewBypass(f, "w12bp")
	iface.SetClaimStore("w12bp", nil)
	if err := other.Delete(ctx, &vxlan.Bypass{Interface: "ens224", Ipv4: true}, nil); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("delete on unclaimed physical = %v, want ErrNotOurs", err)
	}
}
