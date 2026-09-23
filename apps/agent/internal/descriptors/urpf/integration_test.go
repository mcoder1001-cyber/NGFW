package urpf

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

func TestURPFOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	table := vpptest.TableBase(t) + 2
	loop, idx := df2test.Loopback(t, c, 2)
	df2test.VRF(t, c, table, false, "urpf")
	df2test.AddAddress(t, c, idx, fmt.Sprintf("10.%d.3.1/24", slot))
	d := New(c, owner)
	rx := &Interface{Interface: loop, Af: df2.AddressFamily_IPV4, Direction: Interface_RX, Mode: Interface_STRICT}
	tx := &Interface{Interface: loop, Af: df2.AddressFamily_IPV4, Direction: Interface_TX, Mode: Interface_LOOSE, TableId: table}
	metas := map[*Interface]any{}
	for _, u := range []*Interface{rx, tx} {
		meta, err := d.Create(ctx, u)
		if err != nil {
			t.Fatalf("Create %s: %v", d.KeyOf(u), err)
		}
		metas[u] = meta
	}
	t.Cleanup(func() {
		for u, m := range metas {
			_ = d.Delete(df2test.Ctx(t), u, m)
		}
	})
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []*Interface{rx, tx} {
		kv := find(actual, d.KeyOf(want))
		if kv == nil {
			t.Fatalf("Retrieve does not show %s: %+v", d.KeyOf(want), actual)
		}
		t.Logf("Retrieve[%s] = %+v", kv.Key, kv.Value)
		if !proto.Equal(kv.Value, want) || kv.Meta != metas[want] {
			t.Fatalf("Retrieve[%s] = %+v, want %+v", kv.Key, kv.Value, want)
		}
	}
	df2test.Hold(t)
	for u, m := range metas {
		if err := d.Delete(ctx, u, m); err != nil {
			t.Fatalf("Delete %s: %v", d.KeyOf(u), err)
		}
		delete(metas, u)
	}
	if actual, _ = d.Retrieve(ctx); find(actual, d.KeyOf(rx)) != nil || find(actual, d.KeyOf(tx)) != nil {
		t.Fatalf("still retrieved after Delete: %+v", actual)
	}
}
