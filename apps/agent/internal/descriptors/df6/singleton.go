package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SingletonID is the object id of every global singleton ("<name>/global").
const SingletonID = "global"

// SingletonSpec describes a VPP-global setting (SR encap source, LISP enable, l2tpv3 lookup
// key, …): one object per VPP, key "<name>/global". On a shared VPP a singleton belongs to
// everyone; tests read it first and only touch it when nobody else has (docs/lab rules), and
// Delete restores VPP's default rather than "removing" anything.
type SingletonSpec[T proto.Message] struct {
	Name   string
	Plugin string
	// Validate checks obj (nil = no validation).
	Validate func(obj T) error
	// Set applies obj.
	Set func(ctx context.Context, c vpp.Client, obj T) error
	// Unset restores the default (nil = Delete is a documented no-op).
	Unset func(ctx context.Context, c vpp.Client, obj T) error
	// Get reads the current value; present=false means "at default / not configured", which
	// Retrieve reports as no object. nil Get = write-only (Retrieve → ErrRetrieveUnsupported).
	Get func(ctx context.Context, c vpp.Client) (obj T, present bool, err error)
	// Deps lists dependencies of obj (nil = none).
	Deps func(obj T) []scheduler.Dependency
	// KeepOnAbsence: the reconciler must not delete the global just because desired state
	// does not mention it (plugin enable switches, review H1); Delete still works when asked.
	KeepOnAbsence bool
	// SafeToUnset (optional) is the cross-owner emptiness check run before Unset; false →
	// Delete is a no-op (D-071: disable only when nothing of any owner uses the plugin).
	SafeToUnset func(ctx context.Context, c vpp.Client) (bool, error)
	// Change (optional) replaces old by new in place; nil = Set(new).
	Change func(ctx context.Context, c vpp.Client, oldObj, newObj T) error
	// Equal compares a retrieved value with a required one (require variant; nil =
	// proto.Equal).
	Equal func(have, want T) bool
}

// SingletonDescriptor is the scheduler.Descriptor built from a SingletonSpec.
type SingletonDescriptor[T proto.Message] struct {
	spec   SingletonSpec[T]
	client vpp.Client
}

// NewSingletonDescriptor returns the descriptor for spec.
func NewSingletonDescriptor[T proto.Message](spec SingletonSpec[T], c vpp.Client) *SingletonDescriptor[T] {
	return &SingletonDescriptor[T]{spec: spec, client: c}
}

func (d *SingletonDescriptor[T]) cast(obj proto.Message) (T, error) {
	t, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	if d.spec.Validate != nil {
		if err := d.spec.Validate(t); err != nil {
			return zero(t), fmt.Errorf("%s: %w", d.spec.Name, err)
		}
	}
	return t, nil
}

func zero[T any](T) (z T) { return z }

// Name implements scheduler.Descriptor.
func (d *SingletonDescriptor[T]) Name() string { return d.spec.Name }

// Key is the singleton's key.
func (d *SingletonDescriptor[T]) Key() scheduler.Key { return scheduler.Join(d.spec.Name, SingletonID) }

// KeyOf implements scheduler.Descriptor: always "<name>/global".
func (d *SingletonDescriptor[T]) KeyOf(proto.Message) scheduler.Key { return d.Key() }

// Dependencies implements scheduler.Descriptor.
func (d *SingletonDescriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, ok := obj.(T)
	if !ok || d.spec.Deps == nil {
		return nil
	}
	return d.spec.Deps(t)
}

// Create implements scheduler.Descriptor.
func (d *SingletonDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	if err := d.spec.Set(ctx, d.client, t); err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: set the new value in place (Change when given).
func (d *SingletonDescriptor[T]) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	if d.spec.Change == nil {
		return d.Create(ctx, newObj)
	}
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	if err := d.spec.Change(ctx, d.client, o, n); err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	return nil, nil
}

// Delete implements scheduler.Descriptor: restore the default (or nothing, when VPP has no
// way to; the doc table says which).
func (d *SingletonDescriptor[T]) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	if d.spec.Unset == nil {
		return nil
	}
	if d.spec.SafeToUnset != nil {
		ok, err := d.spec.SafeToUnset(ctx, d.client)
		if err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
		if !ok {
			return nil // still in use by some owner: leave it on
		}
	}
	if err := d.spec.Unset(ctx, d.client, t); err != nil {
		return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *SingletonDescriptor[T]) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.spec.Get == nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
	}
	t, present, err := d.spec.Get(ctx, d.client)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	if !present {
		return nil, nil
	}
	return []scheduler.KV{{Key: d.Key(), Value: t}}, nil
}

// DeleteOnAbsence implements P05's scheduler.AbsenceDeleter.
func (d *SingletonDescriptor[T]) DeleteOnAbsence() bool { return !d.spec.KeepOnAbsence }

// RequireDescriptor is the non-globals-owner variant of a singleton (D-071): Create/Update
// only check that the global already has the required value (fail with ErrNotGlobalsOwner
// otherwise, and always for write-only globals that cannot be read), Delete is a no-op,
// Retrieve reports nothing and the reconciler never deletes it on absence.
type RequireDescriptor[T proto.Message] struct {
	spec   SingletonSpec[T]
	client vpp.Client
}

// NewRequireDescriptor returns the require variant of spec.
func NewRequireDescriptor[T proto.Message](spec SingletonSpec[T], c vpp.Client) *RequireDescriptor[T] {
	return &RequireDescriptor[T]{spec: spec, client: c}
}

// Name implements scheduler.Descriptor.
func (d *RequireDescriptor[T]) Name() string { return d.spec.Name }

// KeyOf implements scheduler.Descriptor.
func (d *RequireDescriptor[T]) KeyOf(proto.Message) scheduler.Key {
	return scheduler.Join(d.spec.Name, SingletonID)
}

// Dependencies implements scheduler.Descriptor.
func (d *RequireDescriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, ok := obj.(T)
	if !ok || d.spec.Deps == nil {
		return nil
	}
	return d.spec.Deps(t)
}

// Create implements scheduler.Descriptor: check, never set.
func (d *RequireDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	want, ok := obj.(T)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	if d.spec.Get == nil {
		return nil, fmt.Errorf("%s: %w (write-only global; configure it on the globals owner)", d.spec.Name, ErrNotGlobalsOwner)
	}
	have, present, err := d.spec.Get(ctx, d.client)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	eq := d.spec.Equal
	if eq == nil {
		eq = func(a, b T) bool { return proto.Equal(a, b) }
	}
	if !present || !eq(have, want) {
		return nil, fmt.Errorf("%s: %w: required %v, vpp has %v (present=%v)", d.spec.Name, ErrNotGlobalsOwner, want, have, present)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *RequireDescriptor[T]) Update(ctx context.Context, _, newObj proto.Message, meta any) (any, error) {
	if _, err := d.Create(ctx, newObj); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor: a non-owner never resets a global.
func (d *RequireDescriptor[T]) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: a non-owner does not report globals.
func (d *RequireDescriptor[T]) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
}

// DeleteOnAbsence implements P05's scheduler.AbsenceDeleter.
func (d *RequireDescriptor[T]) DeleteOnAbsence() bool { return false }

// Global returns the globals-owner setter or the require variant of spec (D-071).
func Global[T proto.Message](spec SingletonSpec[T], c vpp.Client, o Options) scheduler.Descriptor {
	if o.GlobalsOwner {
		return NewSingletonDescriptor(spec, c)
	}
	return NewRequireDescriptor(spec, c)
}
