package tapv2_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func TestTapOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()
	d := tapv2.New(c, owner)
	name := vpptest.Name(t, "tap40")
	// host addresses are inside the slot's 10.<slot>.0.0/16 and a fd00:<slot>:: ULA: never management
	desired := &tapv2.Tap{Name: name, Id: vpptest.LoopbackInstance(t, 40), HostIfName: name, HostMtu: 1400,
		HostIp4Prefix: "10." + string(rune('0'+vpptest.Slot(t))) + ".40.1/24", HostIp6Prefix: "fd00:2:40::1/64", RxRingSize: 512, TxRingSize: 256, Gso: true}
	if vpptest.Slot(t) > 9 {
		desired.HostIp4Prefix = ""
	}
	key := string(d.KeyOf(desired))
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			_ = d.Delete(ctx, desired, meta)
		}
	})
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := ifacetest.Find(t, kvs, key, func(kv scheduler.KV) string { return string(kv.Key) })
	if !proto.Equal(got.Value, desired) || got.Meta != meta {
		t.Fatalf("Retrieve = %v %+v, want %v %+v", got.Value, got.Meta, desired, meta)
	}
	t.Logf("tapv2.tap: Retrieve == desired: %s %v (sw_if_index %+v)", key, got.Value, meta)
	ifacetest.Hold(t)
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	deleted = true
	kvs, _ = d.Retrieve(ctx)
	for _, kv := range kvs {
		if string(kv.Key) == key {
			t.Fatal("tap still retrieved after Delete")
		}
	}
}
