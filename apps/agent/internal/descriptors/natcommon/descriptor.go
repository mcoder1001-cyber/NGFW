package natcommon

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
)

// Item is one retrieved object: its typed spec and the Meta Create would have returned.
// NeedsClaim marks an object whose ownership is not proven by a tag or the slot range
// (untagged object, untagged interface): the generic Descriptor reports it only when this
// owner's ClaimStore holds its key (D-071 claim rule).
type Item[T any] struct {
	Spec       T
	Meta       any
	NeedsClaim bool
}

// Ops are the five operations of one object type, written against the typed spec T. Update
// may be nil: every change then asks the scheduler to recreate (scheduler.ErrRecreate).
// Deps may be nil.
type Ops[T any] struct {
	// Name is the descriptor name, "<plugin>.<object>" (scheduler.ValidName).
	Name string
	// ID returns the stable object id (the part after "<name>/" in the key).
	ID func(spec T) string
	// Deps lists the dependencies of spec.
	Deps func(spec T) []scheduler.Dependency
	// Create creates spec and returns its Meta.
	Create func(ctx context.Context, spec T) (any, error)
	// Update changes oldSpec into newSpec in place; nil means "always recreate".
	Update func(ctx context.Context, oldSpec, newSpec T, meta any) (any, error)
	// Delete removes spec with the Meta from Create/Retrieve.
	Delete func(ctx context.Context, spec T, meta any) error
	// Retrieve dumps every owned object of this type.
	Retrieve func(ctx context.Context) ([]Item[T], error)
	// Claims is the claim store (nil: in-memory). Create claims the key, Delete releases it.
	Claims ClaimStore
	// Global marks a VPP-global singleton (built by Global); no claims are recorded.
	Global bool
}

// Descriptor adapts Ops[T] to scheduler.Descriptor: it decodes the *structpb.Struct
// carrier into T, routes to the typed closure and re-encodes on Retrieve.
type Descriptor[T any] struct {
	ops Ops[T]
}

var _ scheduler.Descriptor = (*Descriptor[struct{}])(nil)

// New builds a Descriptor from ops. It panics on an invalid name or missing mandatory
// closure — descriptors are constructed once at start-up, so that is a programming error.
func New[T any](ops Ops[T]) *Descriptor[T] {
	switch {
	case !scheduler.ValidName(ops.Name):
		panic(fmt.Sprintf("natcommon: invalid descriptor name %q", ops.Name))
	case ops.ID == nil || ops.Create == nil || ops.Delete == nil || ops.Retrieve == nil:
		panic(fmt.Sprintf("natcommon: descriptor %q: ID, Create, Delete and Retrieve are mandatory", ops.Name))
	}
	if ops.Claims == nil {
		ops.Claims = NewMemoryClaimStore()
	}
	return &Descriptor[T]{ops: ops}
}

// Name implements scheduler.Descriptor.
func (d *Descriptor[T]) Name() string { return d.ops.Name }

// Key returns the key of a typed spec.
func (d *Descriptor[T]) Key(spec T) scheduler.Key {
	return scheduler.Join(d.ops.Name, d.ops.ID(spec))
}

// Spec decodes the carrier into T (exposed for tests and for consumers that page state).
func (d *Descriptor[T]) Spec(obj proto.Message) (T, error) { return Decode[T](obj) }

// KeyOf implements scheduler.Descriptor. An undecodable object gets a key that can never
// match a real one, so the scheduler reports it instead of silently merging it.
func (d *Descriptor[T]) KeyOf(obj proto.Message) scheduler.Key {
	spec, err := Decode[T](obj)
	if err != nil {
		return scheduler.Join(d.ops.Name, "invalid", err.Error())
	}
	return d.Key(spec)
}

// Dependencies implements scheduler.Descriptor.
func (d *Descriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	if d.ops.Deps == nil {
		return nil
	}
	spec, err := Decode[T](obj)
	if err != nil {
		return nil
	}
	return d.ops.Deps(spec)
}

// Create implements scheduler.Descriptor. The key is claimed BEFORE the VPP call (TD-11b, review
// 3.3): a claim that cannot be recorded fails the Create with nothing written — claiming after the
// call left an object in VPP that no rollback undid and, untagged, Retrieve never reported. When the
// call fails, a claim this Create made is released again, unless the call returned a
// scheduler.PartialCreate error (it changed VPP; with or without Meta): then the claim stays for the
// scheduler's rollback Delete, which proves ownership with it and releases it (scheduler.
// IsPartialCreate is the predicate both sides use). A claim that existed before the Create is never
// released here.
func (d *Descriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	spec, err := Decode[T](obj)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.ops.Name, err)
	}
	if d.ops.Global {
		return d.ops.Create(ctx, spec)
	}
	key := string(d.Key(spec))
	had := d.ops.Claims.Claimed(key)
	if err := d.ops.Claims.Claim(key); err != nil {
		return nil, fmt.Errorf("%s: claim: %w", d.ops.Name, err)
	}
	meta, err := d.ops.Create(ctx, spec)
	if err != nil && !had && !scheduler.IsPartialCreate(err) { // a partial Create keeps the claim: the rollback deletes it
		if rerr := d.ops.Claims.Release(key); rerr != nil {
			err = errors.Join(err, fmt.Errorf("%s: release claim: %w", d.ops.Name, rerr))
		}
	}
	return meta, err
}

// CheckPersistent is the product agent's guard (dfkit/persist, TD-11b review 3.2): a descriptor that
// records claims needs a claim store that survives an agent restart — the product wiring passes
// natcommon.WithClaims(Wiring.KeyedClaims("nat")); the in-memory default is for tests. Global
// singletons record no claims.
func (d *Descriptor[T]) CheckPersistent() error {
	if d.ops.Global {
		return nil
	}
	return persist.Require("natcommon "+d.ops.Name+": claims (pass natcommon.WithClaims(Wiring.KeyedClaims(\"nat\")))", d.ops.Claims)
}

// Update implements scheduler.Descriptor.
func (d *Descriptor[T]) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.ops.Update == nil {
		return nil, scheduler.ErrRecreate
	}
	oldSpec, err := Decode[T](oldObj)
	if err != nil {
		return nil, fmt.Errorf("%s: old object: %w", d.ops.Name, err)
	}
	newSpec, err := Decode[T](newObj)
	if err != nil {
		return nil, fmt.Errorf("%s: new object: %w", d.ops.Name, err)
	}
	if d.ops.ID(oldSpec) != d.ops.ID(newSpec) {
		return nil, scheduler.ErrRecreate
	}
	return d.ops.Update(ctx, oldSpec, newSpec, meta)
}

// Delete implements scheduler.Descriptor.
func (d *Descriptor[T]) Delete(ctx context.Context, obj proto.Message, meta any) error {
	spec, err := Decode[T](obj)
	if err != nil {
		return fmt.Errorf("%s: %w", d.ops.Name, err)
	}
	if err := d.ops.Delete(ctx, spec, meta); err != nil {
		return err
	}
	if !d.ops.Global {
		return d.ops.Claims.Release(string(d.Key(spec)))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *Descriptor[T]) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	items, err := d.ops.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: retrieve: %w", d.ops.Name, err)
	}
	out := make([]scheduler.KV, 0, len(items))
	seen := map[scheduler.Key]bool{}
	for i := range items {
		spec := items[i].Spec
		val, err := Encode(&spec)
		if err != nil {
			return nil, fmt.Errorf("%s: retrieve: %w", d.ops.Name, err)
		}
		key := d.Key(spec)
		if items[i].NeedsClaim && !d.ops.Claims.Claimed(string(key)) {
			continue
		}
		if seen[key] {
			return nil, fmt.Errorf("%s: retrieve: %w: %s", d.ops.Name, ErrDuplicateKey, key)
		}
		seen[key] = true
		out = append(out, scheduler.KV{Key: key, Value: val, Meta: items[i].Meta})
	}
	return out, nil
}

// DedupeTagged collapses the details VPP sends for ONE tagged object and separates different
// objects that share a tag (re-review N3). key returns the object's name (from its tag), an
// identity string (what makes two details the same object, e.g. local ip/port/proto/vrf of a
// static mapping) and whether the detail is the interface-bound (to-resolve) record.
//   - same name, same identity: one object (the resolved twin of an interface-bound mapping,
//     or the per-VRF details of one identity mapping) — the interface-bound record wins;
//   - same name, different identity: a genuinely different object (failed rollback, manual or
//     second instance) — reported as "<name>#<n>" (n = 1, 2, … in dump order, D-066 pattern)
//     so the scheduler deletes it instead of hiding it.
//
// rename sets the name of an extra.
func DedupeTagged[T any](items []Item[T], key func(Item[T]) (name, identity string, ifBound bool), rename func(*T, string)) []Item[T] {
	seen := map[string]map[string]int{} // name → identity → position in out
	var out []Item[T]
	for _, it := range items {
		name, id, ifBound := key(it)
		byID := seen[name]
		if byID == nil {
			byID = map[string]int{}
			seen[name] = byID
		}
		if i, ok := byID[id]; ok {
			if _, _, curIf := key(out[i]); ifBound && !curIf {
				curName, _, _ := key(out[i]) // keep the name (base or "#n") the slot already has
				rename(&it.Spec, curName)
				out[i] = it
			}
			continue
		}
		if extra := len(byID); extra > 0 {
			rename(&it.Spec, fmt.Sprintf("%s#%d", name, extra))
		}
		byID[id] = len(out)
		out = append(out, it)
	}
	return out
}

// BaseName strips the "#<n>" suffix of a duplicate extra (the VPP tag is the base name).
func BaseName(name string) string {
	if i := strings.IndexByte(name, '#'); i >= 0 {
		return name[:i]
	}
	return name
}
