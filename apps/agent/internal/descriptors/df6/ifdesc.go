package df6

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
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
	Del func(ctx context.Context, c vpp.Client, ifs *Interfaces, obj T, idx interface_types.InterfaceIndex) error
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
// when tagging fails so nothing untagged is left on a shared VPP). When an interface tagged
// "<owner>:<id>" already exists (a resync of a write-only type such as ipip.sixrd, or a
// re-apply after an agent restart) it is adopted as Meta instead of adding a duplicate
// (D-076, review H3).
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
	if idx, ok := ifs.IndexByTag(id); ok {
		return IfMeta{SwIfIndex: idx}, nil
	}
	idx, err := d.spec.Add(ctx, d.client, ifs, t)
	if err != nil {
		return nil, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	rollback := func() error {
		if d.spec.Del == nil {
			return nil
		}
		return d.spec.Del(ctx, d.client, ifs, t, idx)
	}
	// D-095 / VPP V19/V21: clear what the previous holder of this sw_if_index left behind before
	// the interface is tagged and reported created
	if _, err := ifsanitize.Sanitize(ctx, d.client, uint32(idx), scheduler.Join(d.spec.Name, id).String()); err != nil {
		if rerr := rollback(); rerr != nil {
			return nil, fmt.Errorf("%s: %w (rollback failed: %v)", d.spec.Name, err, rerr)
		}
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	if err := TagOrRollback(ctx, d.client, d.owner, scheduler.Join(d.spec.Name, id), idx, rollback); err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// verify re-establishes the object's interface immediately before a destructive call
// (D-071: deletes by index re-verify identity): the interface must carry "<owner>:<id>" and,
// when VPP has a dump for the type, a record of this type at that index must decode to the
// same id. Meta (possibly unknown after an agent restart) is only a hint: the tag decides.
// ok=false: the object no longer exists (nothing to delete).
func (d *IfDescriptor[T, D]) verify(ctx context.Context, t T, _ any) (*Interfaces, uint32, T, bool, error) {
	var zero T
	id, err := d.spec.ID(t)
	if err != nil {
		return nil, 0, zero, false, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, 0, zero, false, err
	}
	idx, ok := ifs.IndexByTag(id)
	if !ok {
		return ifs, 0, zero, false, nil
	}
	// A recorded Meta index that differs is stale (VPP restart / index reuse): the tagged
	// interface is the object.
	recs, err := d.spec.Dump(ctx, d.client)
	if errors.Is(err, ErrRetrieveUnsupported) {
		return ifs, idx, t, true, nil // no dump (6rd): the tag is the identity
	}
	if err != nil {
		return nil, 0, zero, false, PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	for _, r := range recs {
		obj, i, ok := d.spec.Decode(r, ifs)
		if !ok || i != idx {
			continue
		}
		if got, err := d.spec.ID(obj); err == nil && got == id {
			return ifs, idx, obj, true, nil // delete what VPP actually has (its key fields)
		}
		return nil, 0, zero, false, fmt.Errorf("%s: %w: sw_if_index %d is tagged %q but holds another object", d.spec.Name, ErrNotOurs, idx, id)
	}
	return ifs, 0, zero, false, nil
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
	if d.spec.Update == nil {
		return nil, scheduler.ErrRecreate
	}
	_, idx, _, ok, err := d.verify(ctx, o, meta)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, scheduler.ErrRecreate
	}
	handled, err := d.spec.Update(ctx, d.client, o, n, interface_types.InterfaceIndex(idx))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	if !handled {
		return nil, scheduler.ErrRecreate
	}
	return IfMeta{SwIfIndex: idx}, nil
}

// Delete implements scheduler.Descriptor. The object is located by its owner tag (so it is
// deletable after an agent restart, when Meta is unknown) and its identity re-verified; an
// object VPP no longer has is already deleted and nothing is sent (V8: some handlers do not
// survive a failed delete).
func (d *IfDescriptor[T, D]) Delete(ctx context.Context, obj proto.Message, meta any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	if meta != nil {
		if _, ok := meta.(IfMeta); !ok {
			return fmt.Errorf("%s: %w %T", d.spec.Name, ErrBadMeta, meta)
		}
	}
	if d.spec.Del == nil {
		return fmt.Errorf("%s: %w", d.spec.Name, ErrNoDelete)
	}
	ifs, idx, actual, ok, err := d.verify(ctx, t, meta)
	if err != nil || !ok {
		return err
	}
	if err := d.spec.Del(ctx, d.client, ifs, actual, interface_types.InterfaceIndex(idx)); err != nil {
		return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
	}
	return nil
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
