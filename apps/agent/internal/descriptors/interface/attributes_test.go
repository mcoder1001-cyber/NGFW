package iface_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

var ctx = context.Background()

func retrieve(t *testing.T, d scheduler.Descriptor) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	return kvs
}

func mustCreate(t *testing.T, d scheduler.Descriptor, obj proto.Message) any {
	t.Helper()
	meta, err := d.Create(ctx, obj)
	if err != nil {
		t.Fatalf("%s Create: %v", d.Name(), err)
	}
	return meta
}

// assertOnly checks Retrieve returns exactly the given desired objects (by key), each proto-equal
// and with the expected Meta.
func assertOnly(t *testing.T, d scheduler.Descriptor, want map[scheduler.Key]proto.Message, metas map[scheduler.Key]any) {
	t.Helper()
	kvs := retrieve(t, d)
	if len(kvs) != len(want) {
		t.Fatalf("%s Retrieve = %d objects %+v, want %d", d.Name(), len(kvs), kvs, len(want))
	}
	for _, kv := range kvs {
		w, ok := want[kv.Key]
		if !ok {
			t.Fatalf("%s Retrieve unexpected key %s", d.Name(), kv.Key)
		}
		if !proto.Equal(kv.Value, w) {
			t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), kv.Key, kv.Value, w)
		}
		if m, ok := metas[kv.Key]; ok && kv.Meta != m {
			t.Fatalf("%s Retrieve %s meta = %+v, want %+v", d.Name(), kv.Key, kv.Meta, m)
		}
	}
}

func TestAdminState(t *testing.T) {
	w := newWorld()
	d := iface.NewAdminState(w.v, owner)
	desired := &iface.AdminState{Interface: loopKey}
	if k := d.KeyOf(desired); k != "interface.admin-state/loop201" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != loopKey || deps[0].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	// the other owner's interface is up: never ours
	w.v.Ifs[w.other].Flags = interface_types.IF_STATUS_API_FLAG_ADMIN_UP
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("nothing of ours is up yet: %+v", kvs)
	}
	meta := mustCreate(t, d, desired)
	calls := w.v.CallsNamed("sw_interface_set_flags")
	if len(calls) != 1 || calls[0].(*ifapi.SwInterfaceSetFlags).Flags != interface_types.IF_STATUS_API_FLAG_ADMIN_UP || uint32(calls[0].(*ifapi.SwInterfaceSetFlags).SwIfIndex) != w.loop {
		t.Fatalf("set_flags calls: %+v", calls)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.admin-state/loop201": desired}, map[scheduler.Key]any{"interface.admin-state/loop201": meta})
	if m, err := d.Update(ctx, desired, desired, meta); err != nil || m != meta {
		t.Fatalf("Update: %v %+v", err, m)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.v.Get(w.loop); got.Flags != 0 {
		t.Fatalf("flags after Delete = %v", got.Flags)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if got, _ := w.v.Get(w.other); got.Flags == 0 {
		t.Fatal("the other owner's interface was touched")
	}
	// errors: bad reference, unknown interface, disconnected
	if _, err := d.Create(ctx, &iface.AdminState{Interface: "loop201"}); !errors.Is(err, iface.ErrBadRef) {
		t.Fatalf("bad ref: %v", err)
	}
	if _, err := d.Create(ctx, &iface.AdminState{Interface: "interface.loopback/loop299"}); !errors.Is(err, iface.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	w.v.SetConnected(false)
	if _, err := d.Create(ctx, desired); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
	if err := d.Delete(ctx, desired, "not a meta"); !errors.Is(err, iface.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestMtu(t *testing.T) {
	w := newWorld()
	d := iface.NewMtu(w.v, owner)
	// VPP gives hardware interfaces an L3 MTU at creation, so both of ours are visible already
	kvs := retrieve(t, d)
	if len(kvs) != 2 {
		t.Fatalf("initial Retrieve = %+v", kvs)
	}
	desired := &iface.Mtu{Interface: loopKey, Mtu: 1500, Ip4: 1400}
	meta := mustCreate(t, d, desired)
	req := w.v.CallsNamed("sw_interface_set_mtu")[0].(*ifapi.SwInterfaceSetMtu)
	if uint32(req.SwIfIndex) != w.loop || len(req.Mtu) != 4 || req.Mtu[0] != 1500 || req.Mtu[1] != 1400 || req.Mtu[2] != 0 || req.Mtu[3] != 0 {
		t.Fatalf("set_mtu = %+v", req)
	}
	tapMtu := &iface.Mtu{Interface: tapKey, Mtu: 9000}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.mtu/loop201": desired, "interface.mtu/w2-tap0": tapMtu}, map[scheduler.Key]any{"interface.mtu/loop201": meta})
	updated := &iface.Mtu{Interface: loopKey, Mtu: 9000}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.mtu/loop201": updated, "interface.mtu/w2-tap0": tapMtu}, nil)
	if _, err := d.Update(ctx, desired, &iface.Mtu{Interface: tapKey, Mtu: 1}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("interface change must recreate: %v", err)
	}
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.mtu/w2-tap0": tapMtu}, nil)
}

func TestMacAddress(t *testing.T) {
	w := newWorld()
	d := iface.NewMacAddress(w.v, owner)
	sub := w.v.Add("loop201.10", "Loopback", "w2:loop201.10")
	w.v.Ifs[sub].Type = interface_types.IF_API_TYPE_SUB
	w.v.Ifs[sub].SupSwIfIndex = w.loop
	w.v.Ifs[sub].SubID = 10
	if kvs := retrieve(t, d); len(kvs) != 2 { // loop201 and w2-tap0, never the sub-interface
		t.Fatalf("initial Retrieve = %+v", kvs)
	}
	desired := &iface.MacAddress{Interface: loopKey, Mac: "02:AA:bb:cc:dd:01"}
	meta := mustCreate(t, d, desired)
	if got, _ := w.v.Get(w.loop); iface.FormatMAC(got.L2Address) != "02:aa:bb:cc:dd:01" {
		t.Fatalf("mac = %v", got.L2Address)
	}
	kvs := retrieve(t, d)
	for _, kv := range kvs {
		if kv.Key == "interface.mac-address/loop201" && kv.Value.(*iface.MacAddress).GetMac() != "02:aa:bb:cc:dd:01" {
			t.Fatalf("Retrieve mac not canonical lower-case: %v", kv.Value)
		}
	}
	if _, err := d.Update(ctx, desired, &iface.MacAddress{Interface: loopKey, Mac: "02:aa:bb:cc:dd:02"}, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, &iface.MacAddress{Interface: loopKey, Mac: "nope"}); err == nil {
		t.Fatal("invalid mac accepted")
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if len(w.v.CallsNamed("sw_interface_set_mac_address")) != 2 {
		t.Fatal("Delete must not touch the MAC")
	}
}

func TestPromisc(t *testing.T) {
	w := newWorld()
	d := iface.NewPromisc(w.v, owner)
	desired := &iface.Promisc{Interface: tapKey}
	meta := mustCreate(t, d, desired)
	if !w.v.Promisc[w.tap] {
		t.Fatal("promisc not set")
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.promisc/w2-tap0": desired}, map[scheduler.Key]any{"interface.promisc/w2-tap0": meta})
	// a fresh descriptor (agent restart) cannot read the state back: it re-applies idempotently
	fresh := iface.NewPromisc(w.v, owner)
	if kvs := retrieve(t, fresh); len(kvs) != 0 {
		t.Fatalf("fresh descriptor Retrieve = %+v", kvs)
	}
	mustCreate(t, fresh, desired)
	if err := fresh.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if w.v.Promisc[w.tap] {
		t.Fatal("promisc still on")
	}
	if kvs := retrieve(t, fresh); len(kvs) != 0 {
		t.Fatalf("after Delete = %+v", kvs)
	}
}

func TestRxMode(t *testing.T) {
	w := newWorld()
	d := iface.NewRxMode(w.v, owner)
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("polling everywhere means no objects: %+v", kvs)
	}
	desired := &iface.RxMode{Interface: tapKey, Mode: iface.RxModeKind_RX_MODE_KIND_INTERRUPT}
	meta := mustCreate(t, d, desired)
	req := w.v.CallsNamed("sw_interface_set_rx_mode")[0].(*ifapi.SwInterfaceSetRxMode)
	if req.QueueIDValid || req.Mode != interface_types.RX_MODE_API_INTERRUPT {
		t.Fatalf("set_rx_mode = %+v", req)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.rx-mode/w2-tap0": desired}, map[scheduler.Key]any{"interface.rx-mode/w2-tap0": meta})
	adaptive := &iface.RxMode{Interface: tapKey, Mode: iface.RxModeKind_RX_MODE_KIND_ADAPTIVE}
	if _, err := d.Update(ctx, desired, adaptive, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.rx-mode/w2-tap0": adaptive}, nil)
	if err := d.Delete(ctx, adaptive, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 || w.v.Queues[w.tap][0].Mode != interface_types.RX_MODE_API_POLLING {
		t.Fatalf("after Delete: %+v %+v", kvs, w.v.Queues[w.tap])
	}
	if _, err := d.Create(ctx, &iface.RxMode{Interface: loopKey, Mode: iface.RxModeKind_RX_MODE_KIND_INTERRUPT}); err == nil {
		t.Fatal("loopbacks have no rx queues; VPP's error must surface")
	}
	if _, err := d.Create(ctx, &iface.RxMode{Interface: tapKey}); err == nil {
		t.Fatal("unspecified mode accepted")
	}
}

func TestRxPlacement(t *testing.T) {
	w := newWorld()
	d := iface.NewRxPlacement(w.v, owner)
	desired := &iface.RxPlacement{Interface: tapKey, Queue: 0, Worker: 1}
	if k := d.KeyOf(desired); k != "interface.rx-placement/w2-tap0/0" {
		t.Fatalf("KeyOf = %s", k)
	}
	if _, err := d.Create(ctx, desired); err == nil {
		t.Fatal("no workers configured: VPP's error must surface (this is the skip: no workers case)")
	}
	w.v.Workers = 2
	meta := mustCreate(t, d, desired)
	if w.v.Queues[w.tap][0].Thread != 2 {
		t.Fatalf("worker 1 is thread 2, got %d", w.v.Queues[w.tap][0].Thread)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.rx-placement/w2-tap0/0": desired}, map[scheduler.Key]any{"interface.rx-placement/w2-tap0/0": meta})
	moved := &iface.RxPlacement{Interface: tapKey, Queue: 0, Worker: 0}
	if _, err := d.Update(ctx, desired, moved, meta); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.rx-placement/w2-tap0/0": moved}, nil)
	if _, err := d.Update(ctx, moved, &iface.RxPlacement{Interface: tapKey, Queue: 1, Worker: 0}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("queue change: %v", err)
	}
	if err := d.Delete(ctx, moved, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 || w.v.Queues[w.tap][0].Thread != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
}
