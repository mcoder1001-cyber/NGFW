package gso_test

import (
	"context"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/feature"
	gsoapi "ngfw/agent/binapi/gso"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/scheduler"
)

const owner = "w7"

var ctx = context.Background()

// fakeGSO models VPP's GSO feature: every enable stacks the feature (vnet_config_add_feature has no
// duplicate check), a disable removes one instance, and feature_is_enabled("ip4-output", "gso-ip4")
// answers true for an index the arc never reached (VNET_API_ERROR_INVALID_SW_IF_INDEX cast to bool).
type fakeGSO struct {
	*dfkittest.FakeVPP
	mu      sync.Mutex
	count   map[uint32]int
	reached map[uint32]bool
	enables int
	arcs    []string
}

func newFake() *fakeGSO {
	f := &fakeGSO{
		FakeVPP: dfkittest.NewFake(
			dfkittest.Iface{Index: 1, Name: "loop7001", Tag: "w7:loop7001"},
			dfkittest.Iface{Index: 2, Name: "loop7002", Tag: "w7:loop7002"},
			dfkittest.Iface{Index: 3, Name: "loop9", Tag: "w3:loop9"},
			dfkittest.Iface{Index: 4, Name: "eth0"}, // untagged: ours only through a claim
		),
		count: map[uint32]int{}, reached: map[uint32]bool{},
	}
	f.On("feature_gso_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*gsoapi.FeatureGsoEnableDisable)
		f.mu.Lock()
		defer f.mu.Unlock()
		idx := uint32(r.SwIfIndex)
		f.reached[idx] = true
		if r.EnableDisable {
			f.count[idx]++
			f.enables++
		} else if f.count[idx] > 0 {
			f.count[idx]--
		}
		return []api.Message{&gsoapi.FeatureGsoEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(m api.Message) ([]api.Message, error) {
		r := m.(*feature.FeatureIsEnabled)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.arcs = append(f.arcs, r.ArcName+"/"+r.FeatureName)
		idx := uint32(r.SwIfIndex)
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: f.count[idx] > 0 || !f.reached[idx]}}, nil
	})
	return f
}

func retrieve(t *testing.T, d scheduler.Descriptor) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		out[kv.Key] = kv.Value
	}
	return out
}

func TestGSOAppliedOnce(t *testing.T) {
	f := newFake()
	store := dfkit.NewMemoryBootStore()
	d := gso.New(f, owner, store)
	desired := gso.Interface{Interface: "loop7001"}.Proto()
	if d.KeyOf(desired) != "gso.interface/loop7001" || d.Dependencies(desired)[0].Key != "interface/loop7001" {
		t.Fatal("key / deps")
	}
	// never-reached index: feature_is_enabled says "on" but there is no record → not reported
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("quirk leaked into Retrieve: %v", got)
	}
	if _, err := d.Create(ctx, desired); err != nil || f.count[1] != 1 {
		t.Fatalf("create: %v count %d", err, f.count[1])
	}
	if f.arcs[0] != "ip4-output/gso-ip4" {
		t.Fatalf("read-back arc %v", f.arcs)
	}
	// every resync re-runs Create: the feature must not stack (D-076)
	for i := 0; i < 3; i++ {
		if _, err := d.Create(ctx, desired); err != nil {
			t.Fatal(err)
		}
	}
	if f.count[1] != 1 || f.enables != 1 {
		t.Fatalf("stacked: count %d, enables %d", f.count[1], f.enables)
	}
	if got := retrieve(t, d); len(got) != 1 || !proto.Equal(got["gso.interface/loop7001"], desired) {
		t.Fatalf("retrieve = %v", got)
	}
	// disabled behind the agent's back: Retrieve no longer reports it, Create restores it once
	f.count[1] = 0
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("after loss: %v", got)
	}
	if _, err := d.Create(ctx, desired); err != nil || f.count[1] != 1 {
		t.Fatalf("restore: %v count %d", err, f.count[1])
	}
	// a VPP restart expires the record: enabled once on the new instance
	f.RestartVPP()
	f.count, f.reached = map[uint32]int{}, map[uint32]bool{}
	if got := retrieve(t, d); len(got) != 0 {
		t.Fatalf("stale record reported after a VPP restart: %v", got)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || f.count[1] != 1 {
		t.Fatalf("after restart: %v count %d", err, f.count[1])
	}
	// a stale enable without a record (lost state dir, or V21 inherited from a deleted interface): exactly one
	f.count[2], f.reached[2] = 2, true
	if _, err := d.Create(ctx, gso.Interface{Interface: "loop7002"}.Proto()); err != nil || f.count[2] != 1 {
		t.Fatalf("lost record: %v count %d", err, f.count[2])
	}
	if err := d.Delete(ctx, desired, meta); err != nil || f.count[1] != 0 {
		t.Fatalf("delete: %v count %d", err, f.count[1])
	}
	if _, ok := store.Get("gso.interface/loop7001"); ok {
		t.Fatal("record survived Delete")
	}
	// Delete with a stale handle (index reused by another interface) never touches it
	f.count[1] = 1
	if err := d.Delete(ctx, desired, gso.Meta{SwIfIndex: 99, Name: "loop7001"}); err != nil || f.count[1] != 1 {
		t.Fatalf("stale meta: %v count %d", err, f.count[1])
	}
	// another owner's interface is never resolved
	if _, err := d.Create(ctx, gso.Interface{Interface: "loop9"}.Proto()); err == nil {
		t.Fatal("foreign interface accepted")
	}
	if _, err := d.Create(ctx, dfkit.Encode(gso.Interface{})); err == nil {
		t.Fatal("empty interface accepted")
	}
}
