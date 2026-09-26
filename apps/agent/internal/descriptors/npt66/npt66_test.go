package npt66_test

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/descriptors/npt66"
	"ngfw/agent/internal/scheduler"
)

func rig(t *testing.T) (*coretest.VPP, *npt66.Plugin) {
	t.Helper()
	v := coretest.New()
	v.AddInterface("host-w4w0", "af_packet", "w4:host-w4w0")
	v.AddInterface("host-w4l0", "af_packet", "w4:host-w4l0")
	v.AddInterface("host-w9w0", "af_packet", "w9:host-w9w0") // another slot's
	return v, npt66.New(v, "w4")
}

func spec(ifName, in, ex string) *npt66.BindingSpec {
	return &npt66.BindingSpec{Interface: ifName, Internal: in, External: ex}
}

func TestBindingKeyAndWriteOnly(t *testing.T) {
	_, p := rig(t)
	obj := natcommon.MustEncode(spec("host-w4w0", "fd00:4:1::/48", "fd00:4:2::/48"))
	if got := p.Binding.KeyOf(obj); got != "npt66.binding/host-w4w0/fd00:4:1::/48" {
		t.Fatalf("key %s", got)
	}
	// host bits are masked, so the key is the network
	if got := p.Binding.KeyOf(natcommon.MustEncode(spec("host-w4w0", "fd00:4:1::5/48", "fd00:4:2::/48"))); got != "npt66.binding/host-w4w0/fd00:4:1::/48" {
		t.Fatalf("masked key %s", got)
	}
	deps := p.Binding.Dependencies(obj)
	if len(deps) != 1 || deps[0].Key != "interface/host-w4w0" || deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}
	nattest.AssertWriteOnly(t, p.Binding) // no dump in VPP 26.06 (D-063)
}

// The reconciler re-applies a write-only object on every resync (D-063). VPP's add overwrites the interface's binding
// and enables the features only for a new binding, so three re-applies leave exactly one binding and one feature
// enable (D-076 needs no applied-once record here); the fake models the duplicate add.
func TestBindingResyncIsIdempotent(t *testing.T) {
	v, p := rig(t)
	ctx := context.Background()
	obj := natcommon.MustEncode(spec("host-w4w0", "fd00:4:1::/48", "fd00:4:2::/48"))
	var meta any
	for i := 0; i < 3; i++ {
		m, err := p.Binding.Create(ctx, obj)
		if err != nil {
			t.Fatalf("create #%d: %v", i, err)
		}
		meta = m
	}
	n := v.NPT66()
	b, ok := n.Binding("host-w4w0")
	n.Lock()
	adds, enables, count := n.Adds, n.FeatureEnables[b.SwIfIndex], len(n.Bindings)
	n.Unlock()
	if !ok || count != 1 || adds != 3 || enables != 1 {
		t.Fatalf("after 3 adds: binding %v %+v, %d bindings, %d adds, feature enabled %d times", ok, b, count, adds, enables)
	}
	if b.Internal != netip.MustParsePrefix("fd00:4:1::/48") || b.External != netip.MustParsePrefix("fd00:4:2::/48") {
		t.Fatalf("binding %+v", b)
	}
	// another external prefix: in-place Update (same key), still one binding, features untouched
	obj2 := natcommon.MustEncode(spec("host-w4w0", "fd00:4:1::/48", "fd00:4:3::/48"))
	if _, err := p.Binding.Update(ctx, obj, obj2, meta); err != nil {
		t.Fatal(err)
	}
	b, _ = n.Binding("host-w4w0")
	n.Lock()
	enables, count = n.FeatureEnables[b.SwIfIndex], len(n.Bindings)
	n.Unlock()
	if count != 1 || enables != 1 || b.External != netip.MustParsePrefix("fd00:4:3::/48") {
		t.Fatalf("after update: %d bindings, features %d, %+v", count, enables, b)
	}
	// Delete removes it and disables the features; a second Delete (already gone) is not an error
	if err := p.Binding.Delete(ctx, obj2, meta); err != nil {
		t.Fatal(err)
	}
	if err := p.Binding.Delete(ctx, obj2, meta); err != nil {
		t.Fatalf("delete of a gone binding: %v", err)
	}
	n.Lock()
	enables, count = n.FeatureEnables[b.SwIfIndex], len(n.Bindings)
	n.Unlock()
	if count != 0 || enables != 0 {
		t.Fatalf("after delete: %d bindings, features %d", count, enables)
	}
}

// ifStub provides the interface/<name> keys the binding depends on (the core alias in the agent).
type ifSpec struct {
	Name string `json:"name"`
}

func ifStub(names ...string) *natcommon.Descriptor[ifSpec] {
	return natcommon.New(natcommon.Ops[ifSpec]{
		Name:   "interface",
		ID:     func(s ifSpec) string { return s.Name },
		Create: func(context.Context, ifSpec) (any, error) { return nil, nil },
		Delete: func(context.Context, ifSpec, any) error { return nil },
		Retrieve: func(context.Context) ([]natcommon.Item[ifSpec], error) {
			var out []natcommon.Item[ifSpec]
			for _, n := range names {
				out = append(out, natcommon.Item[ifSpec]{Spec: ifSpec{Name: n}})
			}
			return out, nil
		},
	})
}

// The same through the reconciler: an apply, three resyncs (agent start / VPP reconnect), one lost binding
// (simulated loss behind the agent's back) that the next resync puts back, and the removal from the desired state.
func TestBindingThroughReconciler(t *testing.T) {
	v, p := rig(t)
	reg := scheduler.NewRegistry()
	reg.Register(ifStub("host-w4w0", "host-w4l0"))
	reg.Register(p.Binding)
	s := scheduler.New(reg, nil)
	ctx := context.Background()
	desired := []scheduler.KV{{Key: "npt66.binding/host-w4w0/fd00:4:1::/48", Value: natcommon.MustEncode(spec("host-w4w0", "fd00:4:1::/48", "fd00:4:2::/48"))}}
	scope := scheduler.Only(npt66.NameBinding)
	if r := s.Apply(ctx, desired, scope); r.Err != nil {
		t.Fatalf("apply: %v", r.Err)
	}
	n := v.NPT66()
	for i := 0; i < 3; i++ {
		if r := s.ApplyWith(ctx, desired, scope, scheduler.ApplyOptions{Resync: true}); r.Err != nil {
			t.Fatalf("resync %d: %v", i, r.Err)
		}
	}
	b, _ := n.Binding("host-w4w0")
	n.Lock()
	adds, enables, count := n.Adds, n.FeatureEnables[b.SwIfIndex], len(n.Bindings)
	n.Unlock()
	if count != 1 || enables != 1 || adds != 4 {
		t.Fatalf("apply + 3 resyncs: %d bindings, features %d, %d adds", count, enables, adds)
	}
	// a plain apply of the same state sends nothing (the process remembers what it applied)
	if r := s.Apply(ctx, desired, scope); r.Err != nil || len(r.Plan.Ops) != 0 {
		t.Fatalf("idempotent apply planned %+v (%v)", r.Plan.Ops, r.Err)
	}
	// simulated loss: the next resync re-adds it
	if !n.DeleteBinding("host-w4w0") || n.Count() != 0 {
		t.Fatal("simulated loss failed")
	}
	if r := s.ApplyWith(ctx, desired, scope, scheduler.ApplyOptions{Resync: true}); r.Err != nil || n.Count() != 1 {
		t.Fatalf("resync after loss: %v, %d bindings", r.Err, n.Count())
	}
	// removed from the desired state → deleted
	if r := s.Apply(ctx, nil, scope); r.Err != nil || n.Count() != 0 {
		t.Fatalf("removal: %v, %d bindings", r.Err, n.Count())
	}
}

func TestBindingValidationAndOwnership(t *testing.T) {
	v, p := rig(t)
	ctx := context.Background()
	for _, bad := range []*npt66.BindingSpec{
		spec("host-w4w0", "fd00:4:1::/80", "fd00:4:2::/80"), // > /64: VPP INVALID_VALUE, refused before sending
		spec("host-w4w0", "fd00:4:1::/48", "fd00:4:2::/56"), // lengths differ
		spec("host-w4w0", "10.4.1.0/24", "10.4.2.0/24"),     // IPv4
	} {
		if _, err := p.Binding.Create(ctx, natcommon.MustEncode(bad)); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, err := p.Binding.Create(ctx, natcommon.MustEncode(spec("host-w9w0", "fd00:4:1::/48", "fd00:4:2::/48"))); !errors.Is(err, natcommon.ErrForeignInterface) {
		t.Fatalf("another owner's interface: %v", err)
	}
	if _, err := p.Binding.Create(ctx, natcommon.MustEncode(spec("host-nope", "fd00:4:1::/48", "fd00:4:2::/48"))); err == nil || !strings.Contains(err.Error(), "no such interface") {
		t.Fatalf("missing interface: %v", err)
	}
	n := v.NPT66()
	n.Lock()
	adds := n.Adds
	n.Unlock()
	if adds != 0 || n.Count() != 0 {
		t.Fatalf("invalid bindings reached VPP: %d adds, %d bindings", adds, n.Count())
	}
}
