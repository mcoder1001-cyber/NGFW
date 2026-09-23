package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// IfSpec describes one object type that is a VPP interface (tunnel, session): how to build
// its id, its dependencies, the add/del messages, the dump and its decoding. IfDescriptor
// turns it into a scheduler.Descriptor with owner tagging and owner-filtered Retrieve, so the
// eight tunnel-like descriptors of DF-6 share one implementation of the ownership rules.
type IfSpec[T proto.Message, D any] struct {
	// Name is the descriptor name ("gre.tunnel").
	Name string
	// Plugin is the VPP plugin name for ErrPluginNotLoaded.
	Plugin string
	// ID validates obj and returns its stable id (the key's id part and the tag id).
	ID func(obj T) (string, error)
	// Deps lists the dependencies of obj (may be nil).
	Deps func(obj T) []scheduler.Dependency
	// Add creates the interface and returns its sw_if_index. ifs is a fresh snapshot for
	// resolving interface references.
	Add func(ctx context.Context, c vpp.Client, ifs *Interfaces, obj T) (interface_types.InterfaceIndex, error)
	// Del deletes the interface created for obj. nil → ErrNoDelete.
	Del func(ctx context.Context, c vpp.Client, obj T, idx interface_types.InterfaceIndex) error
	// Dump returns every object of the type VPP has.
	Dump func(ctx context.Context, c vpp.Client) ([]D, error)
	// Decode turns one dump record into the canonical desired object and its sw_if_index;
	// ok=false skips the record (a record of another object type sharing the dump).
	Decode func(d D, ifs *Interfaces) (obj T, idx uint32, ok bool)
	// Update changes what can change in place and reports whether it handled the whole
	// difference; false (or nil Update) → scheduler.ErrRecreate.
	Update func(ctx context.Context, c vpp.Client, oldObj, newObj T, idx interface_types.InterfaceIndex) (bool, error)
}

// IfDescriptor is the scheduler.Descriptor built from an IfSpec.
type IfDescriptor[T proto.Message, D any] struct {
	spec   IfSpec[T, D]
	client vpp.Client
	owner  string
}

// NewIfDescriptor returns the descriptor for spec, talking to c, owning objects as owner.
func NewIfDescriptor[T proto.Message, D any](spec IfSpec[T, D], c vpp.Client, owner string) *IfDescriptor[T, D] {
	return &IfDescriptor[T, D]{spec: spec, client: c, owner: owner}
}

// Name implements scheduler.Descriptor.
func (d *IfDescriptor[T, D]) Name() string { return d.spec.Name }

// Owner returns the owner the descriptor was built with.
func (d *IfDescriptor[T, D]) Owner() string { return d.owner }

func (d *IfDescriptor[T, D]) cast(obj proto.Message) (T, error) {
	t, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	return t, nil
}

// KeyOf implements scheduler.Descriptor: Join(Name, ID(obj)). An invalid object still gets a
// deterministic key so the transaction can report the validation error against it.
func (d *IfDescriptor[T, D]) KeyOf(obj proto.Message) scheduler.Key {
	t, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	id, err := d.spec.ID(t)
	if err != nil {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	return scheduler.Join(d.spec.Name, id)
}

// Dependencies implements scheduler.Descriptor.
func (d *IfDescriptor[T, D]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, err := d.cast(obj)
	if err != nil || d.spec.Deps == nil {
		return nil
	}
	return d.spec.Deps(t)
}

// Create implements scheduler.Descriptor: add, then stamp the owner tag (rolling the add back
// when tagging fails so nothing untagged is left on a shared VPP).
func (d *IfDescriptor[T, D]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	id, err := d.spec.ID(t)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := d.spec.Add(ctx, d.client, ifs, t)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	rollback := func() error {
		if d.spec.Del == nil {
			return nil
		}
		return d.spec.Del(ctx, d.client, t, idx)
	}
	if err := TagOrRollback(ctx, d.client, d.owner, scheduler.Join(d.spec.Name, id), idx, rollback); err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor.
func (d *IfDescriptor[T, D]) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	m, err := IfMetaOf(d.spec.Name, meta)
	if err != nil {
		return nil, err
	}
	if d.spec.Update == nil {
		return nil, scheduler.ErrRecreate
	}
	handled, err := d.spec.Update(ctx, d.client, o, n, interface_types.InterfaceIndex(m.SwIfIndex))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	if !handled {
		return nil, scheduler.ErrRecreate
	}
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (d *IfDescriptor[T, D]) Delete(ctx context.Context, obj proto.Message, meta any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	m, err := IfMetaOf(d.spec.Name, meta)
	if err != nil {
		return err
	}
	if d.spec.Del == nil {
		return fmt.Errorf("%s: %w", d.spec.Name, ErrNoDelete)
	}
	// Never send a delete VPP would reject: some 26.06 handlers do not survive a failed
	// add/del (V8, gtpu). An object whose interface is gone is already deleted.
	present, err := d.present(ctx, m.SwIfIndex)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if err := d.spec.Del(ctx, d.client, t, interface_types.InterfaceIndex(m.SwIfIndex)); err != nil {
		return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	return nil
}

// present reports whether sw_if_index idx still exists and carries this owner's tag (every
// object of an IfDescriptor is a tagged interface).
func (d *IfDescriptor[T, D]) present(ctx context.Context, idx uint32) (bool, error) {
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return false, err
	}
	return ifs.Owned(idx), nil
}

// Retrieve implements scheduler.Descriptor: dump, keep the records whose interface carries
// this owner's tag with the id the object itself produces (so two descriptors sharing a dump,
// like ipip.tunnel and ipip.6rd, never claim each other's interfaces), decode canonically.
func (d *IfDescriptor[T, D]) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := d.spec.Dump(ctx, d.client)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, r := range recs {
		obj, idx, ok := d.spec.Decode(r, ifs)
		if !ok {
			continue
		}
		tagID, owned := ifs.OwnedID(idx)
		if !owned {
			continue
		}
		id, err := d.spec.ID(obj)
		if err != nil || id != tagID {
			continue
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(d.spec.Name, id), Value: obj, Meta: IfMeta{SwIfIndex: idx}})
	}
	return out, nil
}
