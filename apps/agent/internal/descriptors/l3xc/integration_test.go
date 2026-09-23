package l3xc_test

import (
	"context"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/l3xc"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func TestL3xcOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	ctx := context.Background()
	_, loopKey := ifacetest.Loopback(t, c, owner, 30)
	td := tapv2.New(c, owner)
	name := vpptest.Name(t, "tap30")
	tap := &tapv2.Tap{Name: name, Id: vpptest.LoopbackInstance(t, 30), HostIfName: name, RxRingSize: 256, TxRingSize: 256}
	tapMeta, err := td.Create(ctx, tap)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = td.Delete(ctx, tap, tapMeta) })

	d := l3xc.New(c, owner)
	slot := strconv.Itoa(vpptest.Slot(t))
	desired := &l3xc.L3Xc{Interface: string(td.KeyOf(tap)), Paths: []*l3xc.Path{{NextHop: "10." + slot + ".30.254", Interface: loopKey, Weight: 1}}}
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
	check := func(want *l3xc.L3Xc) {
		t.Helper()
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := ifacetest.Find(t, kvs, key, keyOf)
		if !proto.Equal(got.Value, want) || got.Meta != meta {
			t.Fatalf("Retrieve = %v %+v, want %v %+v", got.Value, got.Meta, want, meta)
		}
		t.Logf("l3xc.l3xc: Retrieve == desired: %s %v", key, got.Value)
	}
	check(desired)
	updated := &l3xc.L3Xc{Interface: desired.Interface, Paths: []*l3xc.Path{{NextHop: "10." + slot + ".30.253", Interface: loopKey, Weight: 1}, {NextHop: "10." + slot + ".30.254", Interface: loopKey, Weight: 1}}}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatalf("Update: %v", err)
	}
	check(updated)
	ifacetest.Hold(t)
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	deleted = true
	kvs, _ := d.Retrieve(ctx)
	for _, kv := range kvs {
		if string(kv.Key) == key {
			t.Fatal("l3xc still retrieved after Delete")
		}
	}
}
