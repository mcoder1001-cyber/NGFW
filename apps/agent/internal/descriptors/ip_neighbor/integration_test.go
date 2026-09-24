package ipneighbor

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func ours(t *testing.T, kvs []scheduler.KV, prefix string) []scheduler.KV {
	t.Helper()
	var out []scheduler.KV
	for _, kv := range kvs {
		if n, ok := kv.Value.(*Neighbor); ok && len(n.GetInterface()) > 4 && n.GetInterface()[:4] == "loop" {
			out = append(out, kv)
		}
	}
	_ = prefix
	return out
}

// TestNeighborOnHost: create → Retrieve shows it → delete → Retrieve shows nothing of ours.
func TestNeighborOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	loop, _ := df2test.Loopback(t, c, 0)
	loopIdx := loop
	_ = loopIdx
	df2test.AddAddress(t, c, swIfIndexOf(t, c, owner, loop), sprintf("10.%d.1.1/24", slot))
	df2test.AddAddress(t, c, swIfIndexOf(t, c, owner, loop), sprintf("2001:db8:%d:1::1/64", slot))

	d := NewNeighbor(c, owner)
	v4 := &Neighbor{Interface: loop, IpAddress: sprintf("10.%d.1.10", slot), MacAddress: sprintf("02:00:00:%02x:00:10", slot)}
	v6 := &Neighbor{Interface: loop, IpAddress: sprintf("2001:db8:%d:1::10", slot), MacAddress: sprintf("02:00:00:%02x:00:11", slot), NoFibEntry: true}
	metas := map[*Neighbor]any{}
	for _, n := range []*Neighbor{v4, v6} {
		meta, err := d.Create(ctx, n)
		if err != nil {
			t.Fatalf("Create %s: %v", d.KeyOf(n), err)
		}
		metas[n] = meta
	}
	t.Cleanup(func() {
		for n, meta := range metas {
			_ = d.Delete(df2test.Ctx(t), n, meta)
		}
	})
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []*Neighbor{v4, v6} {
		found := false
		for _, kv := range actual {
			if kv.Key == d.KeyOf(want) {
				found = true
				if !proto.Equal(kv.Value, want) || kv.Meta != metas[want] {
					t.Fatalf("Retrieve[%s] = %+v (meta %+v), want %+v (meta %+v)", kv.Key, kv.Value, kv.Meta, want, metas[want])
				}
			}
		}
		if !found {
			t.Fatalf("Retrieve does not show %s: %+v", d.KeyOf(want), actual)
		}
	}
	t.Logf("Retrieve shows %d neighbour(s) of owner %s: %v", len(actual), owner, keys(actual))
	df2test.Hold(t)
	for n, meta := range metas {
		if err := d.Delete(ctx, n, meta); err != nil {
			t.Fatalf("Delete %s: %v", d.KeyOf(n), err)
		}
		delete(metas, n)
	}
	actual, err = d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range actual {
		if kv.Key == d.KeyOf(v4) || kv.Key == d.KeyOf(v6) {
			t.Fatalf("still retrieved after Delete: %+v", kv)
		}
	}
	t.Logf("after Delete: %d neighbour(s) of owner %s", len(ours(t, actual, owner)), owner)
}

// TestConfigOnHost sets the IPv4 neighbour limit, reads it back and restores the previous
// value in Cleanup (the object is global).
func TestConfigOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	d := NewConfig(c)
	before, err := d.Retrieve(ctx)
	if err != nil || len(before) != 2 {
		t.Fatalf("Retrieve = %+v, %v", before, err)
	}
	var prev *Config
	for _, kv := range before {
		if kv.Key == "ip-neighbor.config/ipv4" {
			prev = kv.Value.(*Config)
		}
	}
	if prev == nil {
		t.Fatalf("no ipv4 config in %+v", before)
	}
	t.Logf("previous ipv4 config: %+v", prev)
	t.Cleanup(func() {
		if _, err := d.Create(df2test.Ctx(t), prev); err != nil {
			t.Errorf("restore previous config: %v", err)
		}
	})
	desired := &Config{Af: df2.AddressFamily_IPV4, MaxNumber: prev.GetMaxNumber() + 1, MaxAge: prev.GetMaxAge(), Recycle: prev.GetRecycle()}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	after, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range after {
		if kv.Key == d.KeyOf(desired) && !proto.Equal(kv.Value, desired) {
			t.Fatalf("Retrieve = %+v, want %+v", kv.Value, desired)
		}
	}
	t.Logf("ipv4 config after set: %+v", desired)
}
