package iface

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
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
//   - Retrieve: every VPP interface except local0. The name is the owner-tag id for this owner's
//     interfaces (their stable name) and VPP's interface name for everything else; creator is the
//     full creator key for our interfaces of a known device class, empty otherwise.
//
// Name resolution (Create without creator): this owner's tag id first, then VPP's interface name.
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

func vppName(d *ifapi.SwInterfaceDetails) string { return strings.TrimRight(d.InterfaceName, "\x00") }

// find resolves the alias against one dump.
func (d *AliasDescriptor) find(t *Table, o *InterfaceAlias) (uint32, error) {
	if o.GetName() == "" {
		return 0, fmt.Errorf("%w: empty interface name", ErrEmptyValue)
	}
	if c := o.GetCreator(); c != "" {
		k, err := ParseRef(c)
		if err != nil {
			return 0, err
		}
		if k.ID() != o.GetName() {
			return 0, fmt.Errorf("%w: alias %q names creator %q with another id", ErrBadRef, o.GetName(), c)
		}
		return t.Index(c)
	}
	for _, idx := range t.order {
		if id, ok := t.OwnedID(idx); ok && id == o.GetName() {
			return idx, nil
		}
	}
	for _, idx := range t.order {
		if n := vppName(t.byIndex[idx]); n == o.GetName() && n != "local0" {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("%w: interface %q", ErrNotFound, o.GetName())
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

// Retrieve implements scheduler.Descriptor: one alias per VPP interface except local0 (see the
// type documentation for naming). On a name clash this owner's interface wins.
func (d *AliasDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []scheduler.KV
	add := func(idx uint32, v *InterfaceAlias) {
		if v.GetName() == "" || seen[v.GetName()] {
			return
		}
		seen[v.GetName()] = true
		out = append(out, scheduler.KV{Key: AliasKey(v.GetName()), Value: v, Meta: Meta{idx}})
	}
	for _, idx := range t.order { // ours first: stable name + creator
		if id, ok := t.OwnedID(idx); ok {
			v := &InterfaceAlias{Name: id}
			if k, ok := t.KeyFor(idx); ok {
				v.Creator = string(k)
			}
			add(idx, v)
		}
	}
	for _, idx := range t.order {
		if _, ok := t.OwnedID(idx); ok {
			continue
		}
		if n := vppName(t.byIndex[idx]); n != "local0" {
			add(idx, &InterfaceAlias{Name: n})
		}
	}
	return out, nil
}
