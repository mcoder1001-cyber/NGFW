package iface_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// TestPhysicalNIC (review H2): an untagged interface (a DPDK NIC, "ens224") is configured through
// its alias reference "interface/ens224": the attribute objects are ours through the ClaimStore,
// survive a fresh descriptor (agent restart with a persisted store), are released on Delete, and a
// sub-interface on it is a normal tagged object. Another owner's interface is refused.
func TestPhysicalNIC(t *testing.T) {
	iface.SetClaimStore(owner, nil)
	w := newWorld()
	nic := w.v.Add("ens224", "dpdk", "")
	const nicRef = "interface/ens224"

	ad := iface.NewAdminState(w.v, owner)
	if kvs := retrieve(t, ad); len(kvs) != 0 {
		t.Fatalf("unclaimed NIC is not ours: %+v", kvs)
	}
	up := &iface.AdminState{Interface: nicRef}
	meta := mustCreate(t, ad, up)
	if meta != (iface.Meta{SwIfIndex: nic}) {
		t.Fatalf("meta = %+v", meta)
	}
	assertOnly(t, ad, map[scheduler.Key]proto.Message{"interface.admin-state/ens224": up}, map[scheduler.Key]any{"interface.admin-state/ens224": meta})
	// a fresh descriptor (restart; P05 persists the store via SetClaimStore) still sees it
	assertOnly(t, iface.NewAdminState(w.v, owner), map[scheduler.Key]proto.Message{"interface.admin-state/ens224": up}, nil)

	md := iface.NewMtu(w.v, owner)
	mtu := &iface.Mtu{Interface: nicRef, Mtu: 1500}
	mm := mustCreate(t, md, mtu)
	assertOnly(t, md, map[scheduler.Key]proto.Message{"interface.mtu/ens224": mtu}, nil)

	rd := iface.NewRxMode(w.v, owner)
	rx := &iface.RxMode{Interface: nicRef, Mode: iface.RxModeKind_RX_MODE_KIND_ADAPTIVE}
	mustCreate(t, rd, rx)
	assertOnly(t, rd, map[scheduler.Key]proto.Message{"interface.rx-mode/ens224": rx}, nil)

	// sub-interface (VLAN 100) on the NIC: a tagged object of ours, parent in alias form
	sd := iface.NewSubinterface(w.v, owner)
	sub := &iface.Subinterface{Parent: nicRef, SubId: 100, OuterVlan: 100, ExactMatch: true}
	if k := sd.KeyOf(sub); k != "interface.subinterface/ens224.100" {
		t.Fatalf("KeyOf = %s", k)
	}
	mustCreate(t, sd, sub)
	assertOnly(t, sd, map[scheduler.Key]proto.Message{"interface.subinterface/ens224.100": sub}, nil)
	// and its alias: logical name ens224.100 with its creator
	assertAlias := false
	for _, kv := range retrieve(t, iface.NewAlias(w.v, owner)) {
		if kv.Key == "interface/ens224.100" {
			assertAlias = proto.Equal(kv.Value, &iface.InterfaceAlias{Name: "ens224.100", Creator: "interface.subinterface/ens224.100"})
		}
	}
	if !assertAlias {
		t.Fatal("alias of the NIC's sub-interface missing")
	}

	// Delete: the setting goes back to default, the claim is released, the NIC stays
	if err := ad.Delete(ctx, up, meta); err != nil {
		t.Fatal(err)
	}
	if err := md.Delete(ctx, mtu, mm); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, ad, map[scheduler.Key]proto.Message{}, nil)
	assertOnly(t, md, map[scheduler.Key]proto.Message{}, nil)
	if got, ok := w.v.Get(nic); !ok || got.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0 {
		t.Fatalf("NIC after Delete: %+v %v", got, ok)
	}
	if iface.Claims(owner).Claimed("ens224", iface.AdminStateName) {
		t.Fatal("claim not released")
	}

	// another owner's interface (loop300 is tagged w3) is refused by every descriptor
	for _, d := range []scheduler.Descriptor{ad, md, sd} {
		var obj proto.Message
		switch d.(type) {
		case *iface.AdminStateDescriptor:
			obj = &iface.AdminState{Interface: "interface/loop300"}
		case *iface.MtuDescriptor:
			obj = &iface.Mtu{Interface: "interface/loop300", Mtu: 1400}
		default:
			obj = &iface.Subinterface{Parent: "interface/loop300", SubId: 5, OuterVlan: 5}
		}
		if _, err := d.Create(ctx, obj); !errors.Is(err, iface.ErrForeignInterface) {
			t.Fatalf("%s on a foreign interface: %v", d.Name(), err)
		}
	}
}
