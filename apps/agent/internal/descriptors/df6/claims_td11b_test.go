package df6_test

// TD-11b: DF-6 claim hygiene where it touches df6 (re-review N7) and the product agent's
// persistence guard (review 3.2).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// pairClaims is a df6.ClaimStore (id, holder) that can refuse to record.
type pairClaims struct {
	mu   sync.Mutex
	m    map[[2]string]bool
	fail error
}

func newPairClaims() *pairClaims { return &pairClaims{m: map[[2]string]bool{}} }

func (c *pairClaims) Claim(id, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.m[[2]string{id, h}] = true
	return nil
}

func (c *pairClaims) Release(id, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{id, h})
	return nil
}

func (c *pairClaims) Claimed(id, h string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[[2]string{id, h}]
}

type persistedPairs struct{ *pairClaims }

func (persistedPairs) Persistent() bool { return true }

// ifaceBound is a persisted store that binds every claim to an interface's sw_if_index
// (subsystems.IfaceClaims' shape): wrong for keyed ids, which are not interface names.
type ifaceBound struct{ persistedPairs }

func (ifaceBound) BindsInterfaceIndex() bool { return true }

// keyedObjs is the VPP side of a keyed test type: ids of existing objects, the adds seen.
type keyedObjs struct {
	mu      sync.Mutex
	ids     map[string]bool
	adds    int
	failAdd error
}

func keyedSpec(o *keyedObjs) df6.KeyedSpec[*wrapperspb.StringValue] {
	return df6.KeyedSpec[*wrapperspb.StringValue]{
		Name: "test.keyed", Plugin: "test",
		Canon: func(v *wrapperspb.StringValue) (*wrapperspb.StringValue, error) { return v, nil },
		ID:    func(v *wrapperspb.StringValue) string { return v.GetValue() },
		Add: func(_ context.Context, _ vpp.Client, v *wrapperspb.StringValue) error {
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.failAdd != nil {
				return o.failAdd
			}
			o.adds++
			o.ids[v.GetValue()] = true
			return nil
		},
		Del: func(_ context.Context, _ vpp.Client, v *wrapperspb.StringValue) error {
			o.mu.Lock()
			defer o.mu.Unlock()
			delete(o.ids, v.GetValue())
			return nil
		},
		List: func(context.Context, vpp.Client) ([]*wrapperspb.StringValue, error) {
			o.mu.Lock()
			defer o.mu.Unlock()
			var out []*wrapperspb.StringValue
			for id := range o.ids {
				out = append(out, wrapperspb.String(id))
			}
			return out, nil
		},
	}
}

// TestKeyedClaimsBeforeAdd (N7 "keyed crash window"): the claim precedes the add. A claim that
// cannot be recorded fails the Create before the add — with the old order the object stayed in
// VPP unclaimed and every later Create failed with ErrNotOurs. A failed add releases the claim.
func TestKeyedClaimsBeforeAdd(t *testing.T) {
	ctx := context.Background()
	f := df6test.NewFakeVPP()
	f.SetBoot(811)
	o := &keyedObjs{ids: map[string]bool{}}
	c := newPairClaims()
	c.fail = errors.New("claim store: flush: read-only file system")
	d := df6.NewKeyedDescriptor(keyedSpec(o), f, "w8k", df6.WithClaims(c))
	if _, err := d.Create(ctx, wrapperspb.String("sid-1")); !errors.Is(err, c.fail) {
		t.Fatalf("Create = %v, want the claim error", err)
	}
	if o.adds != 0 || len(o.ids) != 0 {
		t.Fatalf("added although the claim failed: adds=%d ids=%v (unclaimed object: ErrNotOurs forever)", o.adds, o.ids)
	}
	c.fail = nil
	o.failAdd = errors.New("vpp: add refused")
	if _, err := d.Create(ctx, wrapperspb.String("sid-1")); err == nil {
		t.Fatal("Create succeeded although the add failed")
	}
	if d.Claimed(ctx, wrapperspb.String("sid-1")) {
		t.Fatal("a failed add left its claim")
	}
	o.failAdd = nil
	if _, err := d.Create(ctx, wrapperspb.String("sid-1")); err != nil || o.adds != 1 || !d.Claimed(ctx, wrapperspb.String("sid-1")) {
		t.Fatalf("Create: %v adds=%d", err, o.adds)
	}
	// a foreign object with another id: never claimed by a Create that fails on it
	o.ids["foreign"] = true
	if _, err := d.Create(ctx, wrapperspb.String("foreign")); !errors.Is(err, df6.ErrNotOurs) || d.Claimed(ctx, wrapperspb.String("foreign")) {
		t.Fatalf("foreign object: %v", err)
	}
}

// TestFileClaimStoreWriteFailure (N7 "stale in-memory claim"): a claim whose file write fails is not
// kept in memory — otherwise the retry reports success and the claim is lost on restart.
func TestFileClaimStoreWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.json")
	s, err := df6.OpenFileClaimStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("a", "h"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil { // the next write cannot create its temp file
		t.Fatal(err)
	}
	if err := s.Claim("b", "h"); err == nil {
		t.Fatal("Claim succeeded although the write failed")
	}
	if s.Claimed("b", "h") {
		t.Fatal("claim kept in memory although it was never written")
	}
	if err := s.Release("a", "h"); err == nil {
		t.Fatal("Release succeeded although the write failed")
	}
	if !s.Claimed("a", "h") {
		t.Fatal("release applied in memory although it was never written")
	}
	if !persist.Is(s) {
		t.Fatal("FileClaimStore survives an agent restart")
	}
}

// TestCheckPersistent (review 3.2 and N7 "claims split across two stores"): keyed and bypass
// descriptors fail the product agent's guard on an in-memory store and on the interface claim store
// (it binds claims to interface indexes, keyed ids are not interfaces); bypass also needs the
// owner's DF-1 store persisted (its per-interface claims live there).
func TestCheckPersistent(t *testing.T) {
	const owner = "w8p"
	f := df6test.NewFakeVPP()
	o := &keyedObjs{ids: map[string]bool{}}
	iface.SetClaimStore(owner, nil)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	keyed := func(opts ...df6.Option) error {
		return persist.Check(df6.NewKeyedDescriptor(keyedSpec(o), f, owner, opts...))
	}
	bypass := func(opts ...df6.Option) error {
		return persist.Check(df6.NewBypassDescriptor(df6.BypassSpec[*wrapperspb.StringValue]{Name: "test.bypass", Plugin: "test"}, f, owner, opts...))
	}
	if err := keyed(); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("keyed, default (in-memory iface store): %v", err)
	}
	if err := keyed(df6.WithClaims(newPairClaims())); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("keyed, in-memory store: %v", err)
	}
	if err := keyed(df6.WithClaims(ifaceBound{persistedPairs{newPairClaims()}})); !errors.Is(err, df6.ErrClaimStoreKind) {
		t.Fatalf("keyed, interface-bound store: %v", err)
	}
	if err := keyed(df6.WithClaims(persistedPairs{newPairClaims()})); err != nil {
		t.Fatalf("keyed, persisted id store: %v", err)
	}
	if err := bypass(df6.WithClaims(persistedPairs{newPairClaims()})); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("bypass with the in-memory DF-1 store: %v", err)
	}
	iface.SetClaimStore(owner, ifaceBound{persistedPairs{newPairClaims()}})
	if err := keyed(); !errors.Is(err, df6.ErrClaimStoreKind) {
		t.Fatalf("keyed, default = the persisted interface store: %v", err)
	}
	if err := bypass(df6.WithClaims(persistedPairs{newPairClaims()})); err != nil {
		t.Fatalf("bypass, both stores persisted: %v", err)
	}
}

// TestBypassPartialCreateReturnsMeta (review 3.3): when one address family was enabled and the
// other failed, Create returns its Meta with the error, so the scheduler journals it and the
// rollback's Delete disables the family that was enabled (before: nil Meta, ip4 bypass left on).
func TestBypassPartialCreateReturnsMeta(t *testing.T) {
	ctx := context.Background()
	f := newCountingFake()
	const owner = "w8b"
	iface.SetClaimStore(owner, nil)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	f.SetBoot(812)
	idx := f.AddInterface("loop1181", owner+":loop1181")
	f.failV6Enable = true
	d := vxlan.NewBypass(f, owner)
	obj := &vxlan.Bypass{Interface: "loop1181", Ipv4: true, Ipv6: true}
	meta, err := d.Create(ctx, obj)
	if err == nil {
		t.Fatal("Create succeeded although ip6 failed")
	}
	if meta == nil || !errors.Is(err, scheduler.ErrPartialCreate) {
		t.Fatalf("ip4 bypass is enabled (%d) but Create returned %v, %v: the rollback cannot undo it", f.count[[2]uint32{idx, 0}], meta, err)
	}
	if err := d.Delete(ctx, obj, meta); err != nil {
		t.Fatal(err)
	}
	if n := f.count[[2]uint32{idx, 0}]; n != 0 {
		t.Fatalf("ip4 bypass still enabled after the rollback Delete: %d", n)
	}
}
