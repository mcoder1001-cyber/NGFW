package arp

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func find(kvs []scheduler.KV, k scheduler.Key) *scheduler.KV {
	for i := range kvs {
		if kvs[i].Key == k {
			return &kvs[i]
		}
	}
	return nil
}

func TestProxyRangeOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	slot := df2test.Slot(t)
	table := vpptest.TableBase(t) + 1
	df2test.VRF(t, c, table, false, "parp")
	d := NewRange(c, &df2.IDRange{Lo: vpptest.TableBase(t), Hi: vpptest.TableBase(t) + 999})
	desired := &ProxyRange{TableId: table, Low: fmt.Sprintf("10.%d.2.1", slot), High: fmt.Sprintf("10.%d.2.9", slot)}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Delete(df2test.Ctx(t), desired, nil) })
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, d.KeyOf(desired))
	if kv == nil || !proto.Equal(kv.Value, desired) {
		t.Fatalf("Retrieve = %+v, want %+v", actual, desired)
	}
	t.Logf("Retrieve: %s", kv.Key)
	df2test.Hold(t)
	if err := d.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); find(actual, d.KeyOf(desired)) != nil {
		t.Fatalf("still retrieved after Delete: %+v", actual)
	}
}

func TestProxyInterfaceOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	loop, _ := df2test.Loopback(t, c, 1)
	d := NewInterface(c, owner)
	desired := &ProxyInterface{Interface: loop}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Delete(df2test.Ctx(t), desired, meta) })
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, d.KeyOf(desired))
	if kv == nil || !proto.Equal(kv.Value, desired) || kv.Meta != meta {
		t.Fatalf("Retrieve = %+v, want %+v meta %+v", actual, desired, meta)
	}
	t.Logf("Retrieve: %s (meta %+v)", kv.Key, kv.Meta)
	df2test.Hold(t)
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); find(actual, d.KeyOf(desired)) != nil {
		t.Fatalf("still retrieved after Delete: %+v", actual)
	}
}
