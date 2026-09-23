package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// KeyedSpec describes an object type that is not an interface and carries no owner tag (LISP
// locator sets, mappings, …): VPP identifies it by its own key fields, List reads every
// object of the type back, and Owns attributes objects to this agent on a shared VPP.
type KeyedSpec[T proto.Message] struct {
	Name   string
	Plugin string
	// Canon validates obj and returns its canonical copy (the form List returns).
	Canon func(obj T) (T, error)
	// ID is the object id of a canonical object (the key's id part).
	ID func(obj T) string
	// Deps lists the dependencies of a canonical object (nil = none).
	Deps func(obj T) []scheduler.Dependency
	// Add / Del send the create / delete messages for a canonical object.
	Add func(ctx context.Context, c vpp.Client, obj T) error
	Del func(ctx context.Context, c vpp.Client, obj T) error
	// List returns every object of the type VPP has, canonical.
	List func(ctx context.Context, c vpp.Client) ([]T, error)
	// Owns filters List for Retrieve (nil = everything is ours).
	Owns func(obj T) bool
	// WriteOnly: List cannot read everything the diff needs, so Retrieve returns
	// ErrRetrieveUnsupported; List still serves Delete's (and Present's) existence check by ID.
	WriteOnly bool
	// Update changes what can change in place (nil or false → scheduler.ErrRecreate).
	Update func(ctx context.Context, c vpp.Client, oldObj, newObj T) (bool, error)
}

// KeyedDescriptor is the scheduler.Descriptor built from a KeyedSpec.
type KeyedDescriptor[T proto.Message] struct {
	spec   KeyedSpec[T]
	client vpp.Client
}

// NewKeyedDescriptor returns the descriptor for spec.
func NewKeyedDescriptor[T proto.Message](spec KeyedSpec[T], c vpp.Client) *KeyedDescriptor[T] {
	return &KeyedDescriptor[T]{spec: spec, client: c}
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

// Create implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	if err := d.spec.Add(ctx, d.client, t); err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
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
	handled, err := d.spec.Update(ctx, d.client, o, n)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	if !handled {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor: an object VPP no longer has is already deleted
// (never send a delete VPP would have to reject — some handlers do not survive it).
func (d *KeyedDescriptor[T]) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	all, err := d.spec.List(ctx, d.client)
	if err != nil {
		return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	id := d.spec.ID(t)
	for _, a := range all {
		if d.spec.ID(a) == id {
			if err := d.spec.Del(ctx, d.client, t); err != nil {
				return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
			}
			return nil
		}
	}
	return nil
}

// Present reports whether VPP has the object of obj's id.
func (d *KeyedDescriptor[T]) Present(ctx context.Context, obj proto.Message) (bool, error) {
	t, err := d.cast(obj)
	if err != nil {
		return false, err
	}
	all, err := d.spec.List(ctx, d.client)
	if err != nil {
		return false, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	id := d.spec.ID(t)
	for _, a := range all {
		if d.spec.ID(a) == id {
			return true, nil
		}
	}
	return false, nil
}

// Retrieve implements scheduler.Descriptor.
func (d *KeyedDescriptor[T]) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.spec.WriteOnly {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
	}
	all, err := d.spec.List(ctx, d.client)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	out := make([]scheduler.KV, 0, len(all))
	for _, a := range all {
		if d.spec.Owns != nil && !d.spec.Owns(a) {
			continue
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(d.spec.Name, d.spec.ID(a)), Value: a})
	}
	return out, nil
}
