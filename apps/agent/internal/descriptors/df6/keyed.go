package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// KeyedSpec describes an object type that is not an interface and carries no owner tag (SR
// SIDs / policies / steering, SR-MPLS, LISP): VPP identifies it by its own key fields, List
// reads every object of the type back, and ownership is a ClaimStore record (D-071, see
// claims.go) — never an address or id range.
type KeyedSpec[T proto.Message] struct {
	Name   string
	Plugin string
	// Canon validates obj and returns its canonical copy (the form List returns).
	Canon func(obj T) (T, error)
	// ID is the object id of a canonical object (the key's id part and the claim id).
	ID func(obj T) string
	// Deps lists the dependencies of a canonical object (nil = none).
	Deps func(obj T) []scheduler.Dependency
	// Add / Del send the create / delete messages for a canonical object. Del receives the
	// object as VPP has it (from List) unless the type is WriteOnly.
	Add func(ctx context.Context, c vpp.Client, obj T) error
	Del func(ctx context.Context, c vpp.Client, obj T) error
	// List returns every object of the type VPP has, canonical (partial objects allowed for
	// write-only types: the id fields must be right).
	List func(ctx context.Context, c vpp.Client) ([]T, error)
	// Identity (optional) compares the key fields beyond the id of a desired/recorded object
	// and the object VPP has (e.g. the BSID a steering entry points at). A mismatch means VPP
	// holds a different object under the same id: never deleted, never taken over.
	Identity func(want, have T) bool
	// WriteOnly: List cannot read everything the diff needs, so Retrieve returns
	// ErrRetrieveUnsupported; List still serves the existence / identity checks.
	WriteOnly bool
	// Update changes what can change in place (nil or false → scheduler.ErrRecreate).
	Update func(ctx context.Context, c vpp.Client, oldObj, newObj T) (bool, error)
}

// KeyedDescriptor is the scheduler.Descriptor built from a KeyedSpec.
//
//   - Create: VPP has the id and we claimed it (same identity) → no-op, i.e. an idempotent
//     re-apply of write-only types (D-076) and after an agent restart; VPP has it unclaimed →
//     ErrNotOurs (never take over, review M3); absent → Add, then Claim.
//   - Delete: unclaimed → no-op (never touch foreign objects); claimed and present with the
//     same identity → Del, then Release; claimed but gone → Release only.
//   - Retrieve: the claimed ids VPP has (a claimed id VPP lost is absent → recreated).
//
// Claims are per VPP instance (D-080, review N3): the holder is "<name>@vpp-<BootID>", so after a
// VPP or host restart every claim has expired — an object someone else created at a formerly
// claimed id is never adopted, updated or deleted; ours are re-established only by our own
// successful Create (VPP lost them anyway).
type KeyedDescriptor[T proto.Message] struct {
	spec   KeyedSpec[T]
	client vpp.Client
	claims ClaimStore
}

// NewKeyedDescriptor returns the descriptor for spec (claims: Options.Claims of owner).
func NewKeyedDescriptor[T proto.Message](spec KeyedSpec[T], c vpp.Client, owner string, opts ...Option) *KeyedDescriptor[T] {
	return &KeyedDescriptor[T]{spec: spec, client: c, claims: BuildOptions(owner, opts).Claims}
}

// Name implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Name() string { return d.spec.Name }

func (d *KeyedDescriptor[T]) cast(obj proto.Message) (T, error) {
	t, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	c, err := d.spec.Canon(t)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	return c, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) KeyOf(obj proto.Message) scheduler.Key {
	t, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	return scheduler.Join(d.spec.Name, d.spec.ID(t))
}

// Dependencies implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, err := d.cast(obj)
	if err != nil || d.spec.Deps == nil {
		return nil
	}
	return d.spec.Deps(t)
}

// find returns what VPP has under t's id.
func (d *KeyedDescriptor[T]) find(ctx context.Context, t T) (T, bool, error) {
	var zero T
	all, err := d.spec.List(ctx, d.client)
	if err != nil {
		return zero, false, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	id := d.spec.ID(t)
	for _, a := range all {
		if d.spec.ID(a) == id {
			return a, true, nil
		}
	}
	return zero, false, nil
}

// holder is the claim holder for the running VPP instance.
func (d *KeyedDescriptor[T]) holder(ctx context.Context) (string, error) {
	boot, err := BootID(ctx, d.client)
	if err != nil {
		return "", err
	}
	return KeyedHolder(d.spec.Name, boot), nil
}

// KeyedHolder is the claim holder of keyed descriptor name on VPP instance boot.
func KeyedHolder(name, boot string) string { return BootHolder(name, boot) }

// ClaimedNow reports whether id of descriptor name is claimed by this owner on the running VPP.
func ClaimedNow(ctx context.Context, c vpp.Client, claims ClaimStore, name, id string) (bool, error) {
	boot, err := BootID(ctx, c)
	if err != nil {
		return false, err
	}
	return claims.Claimed(id, KeyedHolder(name, boot)), nil
}

func (d *KeyedDescriptor[T]) same(want, have T) bool {
	return d.spec.Identity == nil || d.spec.Identity(want, have)
}

// Create implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	id := d.spec.ID(t)
	holder, err := d.holder(ctx)
	if err != nil {
		return nil, err
	}
	have, present, err := d.find(ctx, t)
	if err != nil {
		return nil, err
	}
	if present {
		if d.claims.Claimed(id, holder) && d.same(t, have) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w: %s", d.spec.Name, ErrNotOurs, id)
	}
	if err := d.spec.Add(ctx, d.client, t); err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	if err := d.claims.Claim(id, holder); err != nil {
		return nil, fmt.Errorf("%s: claim %s: %w", d.spec.Name, id, err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.spec.Update == nil {
		return nil, scheduler.ErrRecreate
	}
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	holder, err := d.holder(ctx)
	if err != nil {
		return nil, err
	}
	if !d.claims.Claimed(d.spec.ID(o), holder) {
		return nil, fmt.Errorf("%s: %w: %s", d.spec.Name, ErrNotOurs, d.spec.ID(o))
	}
	handled, err := d.spec.Update(ctx, d.client, o, n)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	if !handled {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	id := d.spec.ID(t)
	holder, err := d.holder(ctx)
	if err != nil {
		return err
	}
	if !d.claims.Claimed(id, holder) {
		return nil // not ours on this VPP instance: never touched (D-071, D-080)
	}
	have, present, err := d.find(ctx, t)
	if err != nil {
		return err
	}
	if present {
		if !d.same(t, have) {
			return fmt.Errorf("%s: %w: %s now holds another object", d.spec.Name, ErrNotOurs, id)
		}
		target := have
		if d.spec.WriteOnly {
			target = t // List returns partial objects for write-only types
		}
		if err := d.spec.Del(ctx, d.client, target); err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
	}
	return d.claims.Release(id, holder)
}

// Present reports whether VPP has an object under obj's id (any owner).
func (d *KeyedDescriptor[T]) Present(ctx context.Context, obj proto.Message) (bool, error) {
	t, err := d.cast(obj)
	if err != nil {
		return false, err
	}
	_, ok, err := d.find(ctx, t)
	return ok, err
}

// Claimed reports whether obj's id is claimed by this owner on the running VPP.
func (d *KeyedDescriptor[T]) Claimed(ctx context.Context, obj proto.Message) bool {
	t, err := d.cast(obj)
	if err != nil {
		return false
	}
	h, err := d.holder(ctx)
	return err == nil && d.claims.Claimed(d.spec.ID(t), h)
}

// Retrieve implements scheduler.Descriptor: the claimed objects VPP has; an id VPP lists twice
// (review L3) is reported once.
func (d *KeyedDescriptor[T]) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.spec.WriteOnly {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
	}
	holder, err := d.holder(ctx)
	if err != nil {
		return nil, err
	}
	all, err := d.spec.List(ctx, d.client)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	seen := map[string]bool{}
	out := make([]scheduler.KV, 0, len(all))
	for _, a := range all {
		id := d.spec.ID(a)
		if seen[id] || !d.claims.Claimed(id, holder) {
			continue
		}
		seen[id] = true
		out = append(out, scheduler.KV{Key: scheduler.Join(d.spec.Name, id), Value: a})
	}
	return out, nil
}
