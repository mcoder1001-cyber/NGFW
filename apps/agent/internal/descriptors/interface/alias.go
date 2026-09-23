package iface

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// AliasName is the descriptor name of the generic interface reference (D-065): every consumer
// outside DF-1 depends on AliasKey(name) = "interface/<name>" and never needs to know which
// descriptor (if any) created the interface.
const AliasName = "interface"

// AliasKey is the key consumers depend on: "interface/<name>".
func AliasKey(name string) scheduler.Key { return scheduler.Join(AliasName, name) }

// AliasDescriptor implements the "interface" alias (D-065). It creates nothing in VPP:
//
//   - Dependencies: the creator key when set (mandatory), nothing for physical/pre-existing
//     interfaces (DPDK NICs bound by the port-group mapping, interfaces made outside the scheduler);
//   - Create/Update: verify the interface exists (one sw_interface_dump) and return its sw_if_index
//     as Meta; fail with ErrNotFound otherwise;
//   - Delete: no-op — deleting an alias never touches VPP, so a foreign interface can never be
//     removed through it;
//   - Retrieve: every interface with a logical name for this owner (ours + untagged, never another
//     owner's, never local0); observe-only for the reconciler (DeleteOnAbsence() == false).
//
// Names are logical names (D-069, names.go): our tag id, else an untagged interface's VPP name.
type AliasDescriptor struct{ base }

// NewAlias returns the descriptor for owner.
func NewAlias(c vpp.Client, owner string) *AliasDescriptor { return &AliasDescriptor{base{c, owner}} }

// Name implements scheduler.Descriptor.
func (*AliasDescriptor) Name() string { return AliasName }

// KeyOf implements scheduler.Descriptor.
func (*AliasDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return AliasKey(obj.(*InterfaceAlias).GetName())
}

// Dependencies implements scheduler.Descriptor: the creator key when present.
func (*AliasDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	if c := obj.(*InterfaceAlias).GetCreator(); c != "" {
		return []scheduler.Dependency{{Key: scheduler.Key(c)}}
	}
	return nil
}

// find resolves the alias against one dump: by creator key (owner tag) when set, else by logical
// name (IndexByName: our tag id, else an untagged interface's VPP name; another owner's
// interface is refused with ErrForeignInterface — review M1).
func (d *AliasDescriptor) find(t *Table, o *InterfaceAlias) (uint32, error) {
	if o.GetName() == "" {
		return 0, fmt.Errorf("%w: empty interface name", ErrEmptyValue)
	}
	if c := o.GetCreator(); c != "" {
		k, err := ParseRef(c)
		if err != nil {
			return 0, err
		}
		if k.ID() != o.GetName() || k.Descriptor() == AliasName {
			return 0, fmt.Errorf("%w: alias %q names creator %q", ErrBadRef, o.GetName(), c)
		}
		return t.Index(c)
	}
	return t.IndexByName(o.GetName())
}

func (d *AliasDescriptor) verify(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*InterfaceAlias)
	if !ok || o == nil {
		return nil, ErrEmptyValue
	}
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := d.find(t, o)
	if err != nil {
		return nil, err
	}
	return Meta{idx}, nil
}

// Create implements scheduler.Descriptor: verifies the interface exists, creates nothing.
func (d *AliasDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return d.verify(ctx, obj)
}

// Update implements scheduler.Descriptor: the creator is metadata; re-verify and return the Meta.
func (d *AliasDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.verify(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op, never touches VPP (D-065).
func (*AliasDescriptor) Delete(context.Context, proto.Message, any) error { return nil }

// DeleteOnAbsence implements P05's scheduler.AbsenceDeleter (review H1): the alias is
// observe-only — retrieved aliases that are not desired (physical NICs, interfaces other
// descriptors or agents manage) are never planned for Delete and never fail verification.
func (*AliasDescriptor) DeleteOnAbsence() bool { return false }

// Retrieve implements scheduler.Descriptor: one alias per interface that has a logical name for
// this owner — our tagged interfaces (name = tag id, creator = creator key when the device class
// is known) and untagged ones (name = VPP name, no creator). Interfaces tagged by another owner and
// local0 are never reported (review H1).
func (d *AliasDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []scheduler.KV
	idxs := make([]uint32, 0, len(t.order)) // ours first: on a name clash our interface wins
	for _, idx := range t.order {
		if _, ours := t.OwnedID(idx); ours {
			idxs = append(idxs, idx)
		}
	}
	for _, idx := range t.order {
		if _, ours := t.OwnedID(idx); !ours {
			idxs = append(idxs, idx)
		}
	}
	for _, idx := range idxs {
		name, ok := t.Logical(idx)
		if !ok || seen[name] {
			continue // another owner's interface / local0, or an untagged duplicate of our name
		}
		seen[name] = true
		v := &InterfaceAlias{Name: name}
		if _, ours := t.OwnedID(idx); ours {
			if k, ok := t.KeyFor(idx); ok {
				v.Creator = string(k)
			}
		}
		out = append(out, scheduler.KV{Key: AliasKey(name), Value: v, Meta: Meta{idx}})
	}
	return out, nil
}
