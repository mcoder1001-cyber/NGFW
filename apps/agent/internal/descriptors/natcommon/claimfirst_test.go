package natcommon_test

// TD-11b (review 3.3): the generic Descriptor claims the key BEFORE the VPP call and releases a
// claim it made when the call fails. With the old order (Create, then Claim) a claim failure left
// the object in VPP with (meta, err): unjournaled, and — for untagged objects — invisible to
// Retrieve, so the next Create hit "already exists" forever.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// recClaims is a ClaimStore that can refuse to record.
type recClaims struct {
	mu       sync.Mutex
	m        map[string]bool
	failWith error
}

func newRecClaims() *recClaims { return &recClaims{m: map[string]bool{}} }

func (c *recClaims) Claim(k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failWith != nil {
		return c.failWith
	}
	c.m[k] = true
	return nil
}

func (c *recClaims) Release(k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

func (c *recClaims) Claimed(k string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[k]
}

// vppObjs models the objects of one type in VPP.
type vppObjs struct {
	mu      sync.Mutex
	objs    map[string]bool
	writes  int
	failAdd error // the add fails before writing
	failPst error // the add writes, then fails (partial)
	nilMeta bool  // the partial failure comes with a nil Meta (key-addressed objects)
}

func claimDescriptor(v *vppObjs, claims natcommon.ClaimStore) *natcommon.Descriptor[claimSpec] {
	return natcommon.New(natcommon.Ops[claimSpec]{
		Name:   "test.claimed",
		ID:     func(s claimSpec) string { return s.ID },
		Claims: claims,
		Create: func(_ context.Context, s claimSpec) (any, error) {
			v.mu.Lock()
			defer v.mu.Unlock()
			if v.failAdd != nil {
				return nil, v.failAdd
			}
			v.objs[s.ID] = true
			v.writes++
			if v.failPst != nil {
				if v.nilMeta {
					return nil, scheduler.PartialCreate(v.failPst) // key-addressed: no Meta, still partial
				}
				return s.ID, scheduler.PartialCreate(v.failPst) // VPP was written: the Meta says what to delete
			}
			return s.ID, nil
		},
		Delete: func(_ context.Context, s claimSpec, _ any) error {
			v.mu.Lock()
			defer v.mu.Unlock()
			delete(v.objs, s.ID)
			return nil
		},
		Retrieve: func(context.Context) ([]natcommon.Item[claimSpec], error) {
			v.mu.Lock()
			defer v.mu.Unlock()
			var out []natcommon.Item[claimSpec]
			for id := range v.objs {
				out = append(out, natcommon.Item[claimSpec]{Spec: claimSpec{ID: id}, Meta: id, NeedsClaim: true})
			}
			return out, nil
		},
	})
}

// A claim that cannot be recorded fails the Create before anything is written to VPP.
func TestGenericCreateClaimsBeforeVPP(t *testing.T) {
	refused := errors.New("claim store: flush: no space left on device")
	c := newRecClaims()
	c.failWith = refused
	v := &vppObjs{objs: map[string]bool{}}
	d := claimDescriptor(v, c)
	meta, err := d.Create(context.Background(), natcommon.MustEncode(&claimSpec{ID: "a"}))
	if !errors.Is(err, refused) {
		t.Fatalf("Create = %v, %v; want the claim error", meta, err)
	}
	if v.writes != 0 || len(v.objs) != 0 {
		t.Fatalf("VPP written although the claim failed: writes=%d objs=%v (invisible, unjournaled object)", v.writes, v.objs)
	}
	if meta != nil {
		t.Fatalf("nothing was written, so no Meta: %v", meta)
	}
}

// A VPP call that fails before writing releases the claim the Create made; a claim that existed
// before stays.
func TestGenericCreateFailureReleasesNewClaim(t *testing.T) {
	c := newRecClaims()
	v := &vppObjs{objs: map[string]bool{}, failAdd: errors.New("VNET_API_ERROR_VALUE_EXIST")}
	d := claimDescriptor(v, c)
	obj := natcommon.MustEncode(&claimSpec{ID: "a"})
	if _, err := d.Create(context.Background(), obj); err == nil {
		t.Fatal("Create succeeded although VPP refused")
	}
	if c.Claimed("test.claimed/a") {
		t.Fatal("a failed Create left its claim (a foreign object with that key would be adopted)")
	}
	_ = c.Claim("test.claimed/a")
	if _, err := d.Create(context.Background(), obj); err == nil {
		t.Fatal("Create succeeded although VPP refused")
	}
	if !c.Claimed("test.claimed/a") {
		t.Fatal("a claim that existed before the Create was released")
	}
}

// A Create that wrote VPP and then failed returns its Meta with a scheduler.PartialCreate error and
// KEEPS the claim, so
// the scheduler's rollback Delete (which journals it, TD-11b) proves ownership and releases it.
func TestGenericPartialCreateKeepsClaimForRollback(t *testing.T) {
	c := newRecClaims()
	v := &vppObjs{objs: map[string]bool{}, failPst: errors.New("tag readback failed")}
	d := claimDescriptor(v, c)
	obj := natcommon.MustEncode(&claimSpec{ID: "a"})
	meta, err := d.Create(context.Background(), obj)
	if err == nil || meta != "a" {
		t.Fatalf("Create = %v, %v; want the Meta of the written object with the error", meta, err)
	}
	if !c.Claimed("test.claimed/a") {
		t.Fatal("the partial object's claim was dropped: Retrieve cannot see it, the rollback cannot prove it is ours")
	}
	if kvs, _ := d.Retrieve(context.Background()); len(kvs) != 1 {
		t.Fatalf("partial object invisible to Retrieve: %v", kvs)
	}
	if err := d.Delete(context.Background(), obj, meta); err != nil {
		t.Fatal(err)
	}
	if len(v.objs) != 0 || c.Claimed("test.claimed/a") {
		t.Fatalf("after the rollback Delete: objs=%v claimed=%v", v.objs, c.Claimed("test.claimed/a"))
	}
}

// Global singletons never claim (unchanged).
func TestGlobalCreateDoesNotClaim(t *testing.T) {
	c := newRecClaims()
	c.failWith = errors.New("must not be called")
	g := natcommon.Global(natcommon.BuildConfig([]natcommon.Option{natcommon.WithGlobalsOwner(true), natcommon.WithClaims(c)}), natcommon.GlobalOps[gspec]{
		Name: "test.global2", ID: "global",
		Read: func(context.Context) (natcommon.GlobalState[gspec], error) {
			return natcommon.GlobalState[gspec]{Observable: true}, nil
		},
		Set:    func(context.Context, gspec) error { return nil },
		Reset:  func(context.Context, gspec) error { return nil },
		Absent: func(v gspec) bool { return v.V == 0 },
	})
	if _, err := g.Create(context.Background(), natcommon.MustEncode(&gspec{V: 1})); err != nil {
		t.Fatal(err)
	}
}

// persistedClaims is recClaims that survives an agent restart (the subsystems.KeyedClaims shape).
type persistedClaims struct{ *recClaims }

func (persistedClaims) Persistent() bool { return true }

// TestCheckPersistent (TD-11b, review 3.2): a claiming descriptor with the in-memory default store
// fails the product agent's guard; a persisted store passes; global singletons record no claims.
func TestCheckPersistent(t *testing.T) {
	v := &vppObjs{objs: map[string]bool{}}
	if err := persist.Check(claimDescriptor(v, nil)); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory default: %v", err)
	}
	if err := persist.Check(claimDescriptor(v, newRecClaims())); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("a store without Persistent(): %v", err)
	}
	if err := persist.Check(claimDescriptor(v, persistedClaims{newRecClaims()})); err != nil {
		t.Fatal(err)
	}
	g := natcommon.Global(natcommon.BuildConfig(nil), natcommon.GlobalOps[gspec]{
		Name: "test.global3", ID: "global",
		Read: func(context.Context) (natcommon.GlobalState[gspec], error) {
			return natcommon.GlobalState[gspec]{}, nil
		},
	})
	if err := persist.Check(g); err != nil {
		t.Fatalf("global: %v", err)
	}
}

// Review M2: a partial Create with a nil Meta keeps the claim too (the marker alone decides, the
// same predicate as the scheduler's journal: scheduler.IsPartialCreate); before fix round 1 the claim
// was released and the object left in VPP unclaimed.
func TestGenericPartialCreateNilMetaKeepsClaim(t *testing.T) {
	c := newRecClaims()
	v := &vppObjs{objs: map[string]bool{}, failPst: errors.New("tag readback failed"), nilMeta: true}
	d := claimDescriptor(v, c)
	obj := natcommon.MustEncode(&claimSpec{ID: "a"})
	meta, err := d.Create(context.Background(), obj)
	if !scheduler.IsPartialCreate(err) || meta != nil {
		t.Fatalf("Create = %v, %v", meta, err)
	}
	if !c.Claimed("test.claimed/a") {
		t.Fatal("claim released although VPP was written (partial with nil Meta)")
	}
	if err := d.Delete(context.Background(), obj, nil); err != nil || len(v.objs) != 0 || c.Claimed("test.claimed/a") {
		t.Fatalf("rollback Delete: %v objs=%v", err, v.objs)
	}
}
