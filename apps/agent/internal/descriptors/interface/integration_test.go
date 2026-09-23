package iface_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func has(kvs []scheduler.KV, key string) bool {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return true
		}
	}
	return false
}

// TestAttributesOnHost: one create → Retrieve shows it → delete → Retrieve shows nothing per
// interface.* object type, on the shared host VPP, with slot-prefixed objects only.
func TestAttributesOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()
	_, loopKey := ifacetest.Loopback(t, c, owner, 1)

	// a tap gives us an interface with rx queues (rx-mode / rx-placement / promisc)
	td := tapv2.New(c, owner)
	tapName := vpptest.Name(t, "tap1")
	tap := &tapv2.Tap{Name: tapName, Id: vpptest.LoopbackInstance(t, 1), HostIfName: tapName, RxRingSize: 256, TxRingSize: 256}
	tapMeta, err := td.Create(ctx, tap)
	if err != nil {
		t.Fatalf("tap create: %v", err)
	}
	t.Cleanup(func() {
		if err := td.Delete(context.Background(), tap, tapMeta); err != nil {
			t.Errorf("cleanup tap: %v", err)
		}
	})
	tapKey := string(td.KeyOf(tap))

	roundTrip := func(t *testing.T, d scheduler.Descriptor, desired proto.Message, deletable bool) {
		t.Helper()
		key := string(d.KeyOf(desired))
		meta, err := d.Create(ctx, desired)
		if err != nil {
			t.Fatalf("%s Create: %v", d.Name(), err)
		}
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			t.Fatalf("%s Retrieve: %v", d.Name(), err)
		}
		got := ifacetest.Find(t, kvs, key, keyOf)
		if !proto.Equal(got.Value, desired) {
			t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), key, got.Value, desired)
		}
		if got.Meta != meta {
			t.Fatalf("%s meta = %+v, want %+v", d.Name(), got.Meta, meta)
		}
		t.Logf("%s: Retrieve == desired: %s %v", d.Name(), key, got.Value)
		ifacetest.Hold(t)
		if err := d.Delete(ctx, desired, meta); err != nil {
			t.Fatalf("%s Delete: %v", d.Name(), err)
		}
		kvs, err = d.Retrieve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if deletable && has(kvs, key) {
			t.Fatalf("%s: %s still retrieved after Delete", d.Name(), key)
		}
		for _, kv := range kvs {
			if string(kv.Key) == key {
				t.Logf("%s: after Delete VPP reports %v (attribute without an unset)", d.Name(), kv.Value)
			}
		}
	}

	t.Run("admin-state", func(t *testing.T) {
		roundTrip(t, iface.NewAdminState(c, owner), &iface.AdminState{Interface: loopKey}, true)
	})
	t.Run("mtu", func(t *testing.T) {
		d := iface.NewMtu(c, owner)
		kvs, _ := d.Retrieve(ctx)
		for _, kv := range kvs {
			if string(kv.Key) == string(d.KeyOf(&iface.Mtu{Interface: loopKey})) {
				t.Logf("fresh loopback MTU as VPP reports it: %v", kv.Value)
			}
		}
		roundTrip(t, d, &iface.Mtu{Interface: loopKey, Mtu: 1500, Ip4: 1400}, false)
	})
	t.Run("mac-address", func(t *testing.T) {
		roundTrip(t, iface.NewMacAddress(c, owner), &iface.MacAddress{Interface: loopKey, Mac: "02:02:00:00:c9:01"}, false)
	})
	t.Run("promisc", func(t *testing.T) {
		roundTrip(t, iface.NewPromisc(c, owner), &iface.Promisc{Interface: tapKey}, true)
	})
	t.Run("rx-mode", func(t *testing.T) {
		roundTrip(t, iface.NewRxMode(c, owner), &iface.RxMode{Interface: tapKey, Mode: iface.RxModeKind_RX_MODE_KIND_INTERRUPT}, true)
	})
	t.Run("rx-placement", func(t *testing.T) {
		if n := ifacetest.Workers(t, c); n == 0 {
			t.Skip("skip: no workers on host (startup.conf cpu { } runs the main core only; sw_interface_set_rx_placement needs a worker)")
		}
		roundTrip(t, iface.NewRxPlacement(c, owner), &iface.RxPlacement{Interface: tapKey, Queue: 0, Worker: 0}, true)
	})
	t.Run("subinterface", func(t *testing.T) {
		roundTrip(t, iface.NewSubinterface(c, owner), &iface.Subinterface{Parent: loopKey, SubId: 100, OuterVlan: 100, ExactMatch: true}, true)
	})
}
