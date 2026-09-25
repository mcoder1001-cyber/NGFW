package acl

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// BindingMeta is the runtime handle of an acl.interface-binding or acl.etype-whitelist object:
// the sw_if_index of the interface the lists are applied to.
type BindingMeta struct {
	SwIfIndex uint32
}

// KeyInterfaceBinding is the key of the ACL binding on interface ifName: "acl.interface-binding/<ifName>".
func KeyInterfaceBinding(ifName string) scheduler.Key {
	return scheduler.Join(NameInterfaceBinding, ifName)
}

// InterfaceBindingDescriptor manages acl.interface-binding objects with
// acl_interface_set_acl_list (this owner's whole in+out list is the unit of desired state;
// other owners' ACLs on the same interface are preserved, see setList) and acl_interface_list_dump.
type InterfaceBindingDescriptor struct {
	client vpp.Client
	owner  string
	opts   options
}

var _ scheduler.Descriptor = (*InterfaceBindingDescriptor)(nil)

// NewInterfaceBinding returns the acl.interface-binding descriptor for one owner.
func NewInterfaceBinding(client vpp.Client, owner string, opts ...Option) *InterfaceBindingDescriptor {
	return &InterfaceBindingDescriptor{client: client, owner: owner, opts: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*InterfaceBindingDescriptor) Name() string { return NameInterfaceBinding }

// KeyOf implements scheduler.Descriptor: acl.interface-binding/<interface>.
func (*InterfaceBindingDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	b, _ := InterfaceBindingFromProto(obj)
	return KeyInterfaceBinding(b.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface (mandatory, review 3.4; see WithInterfaceKey)
// and every acl.acl the lists name (mandatory: an ACL is created before it is bound and unbound
// before it is deleted — VPP refuses acl_del while bound).
func (d *InterfaceBindingDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	b, _ := InterfaceBindingFromProto(obj)
	deps := []scheduler.Dependency{d.opts.boundInterfaceDependency(b.Interface)}
	seen := map[string]struct{}{}
	for _, name := range append(append([]string{}, b.Input...), b.Output...) {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		deps = append(deps, scheduler.Dependency{Key: KeyACL(name)})
	}
	return deps
}

// setList sends acl_interface_set_acl_list for swIfIndex with the desired lists, resolving ACL
// names to this owner's indexes. ACLs of other owners (or untagged ones) already applied to the
// interface are preserved: they stay first in their direction, in their current order, and this
// owner's ACLs follow in desired order (another owner's semantics never change because of us;
// see docs/agent/descriptors/acl.md "Ownership"). An empty b (Delete) removes only our entries.
func (d *InterfaceBindingDescriptor) setList(ctx context.Context, swIfIndex uint32, b InterfaceBinding) error {
	if err := b.validateLists(); err != nil {
		return err
	}
	owned, err := dumpOwnedACLs(ctx, d.client, d.owner)
	if err != nil {
		return err
	}
	byName := aclIndexByName(owned)
	ours := aclNameByIndex(owned)
	curIn, curOut, err := d.currentList(ctx, swIfIndex)
	if err != nil {
		return err
	}
	foreign := func(list []uint32) []uint32 {
		var out []uint32
		for _, idx := range list {
			if _, mine := ours[idx]; !mine {
				out = append(out, idx)
			}
		}
		return out
	}
	resolve := func(names []string) ([]uint32, error) {
		out := make([]uint32, 0, len(names))
		for _, name := range names {
			idx, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("%w: %q (owner %q) while binding to %q", ErrNoACL, name, d.owner, b.Interface)
			}
			out = append(out, idx)
		}
		return out, nil
	}
	in, err := resolve(b.Input)
	if err != nil {
		return err
	}
	out, err := resolve(b.Output)
	if err != nil {
		return err
	}
	in = append(foreign(curIn), in...)
	out = append(foreign(curOut), out...)
	if len(in)+len(out) > maxInterfaceList {
		return fmt.Errorf("%w: %d ACLs on %q including other owners' ACLs, VPP takes at most %d", ErrSpec, len(in)+len(out), b.Interface, maxInterfaceList)
	}
	req := &acl.ACLInterfaceSetACLList{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		NInput:    uint8(len(in)), //nolint:gosec // bounded by maxInterfaceList above
		Acls:      append(in, out...),
	}
	if _, err := acl.NewServiceClient(d.client).ACLInterfaceSetACLList(ctx, req); err != nil {
		return fmt.Errorf("acl_interface_set_acl_list %q: %w", b.Interface, err)
	}
	return nil
}

// currentList reads the input and output ACL index lists applied to swIfIndex.
func (d *InterfaceBindingDescriptor) currentList(ctx context.Context, swIfIndex uint32) (in, out []uint32, err error) {
	stream, err := acl.NewServiceClient(d.client).ACLInterfaceListDump(ctx, &acl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(swIfIndex)})
	if err != nil {
		return nil, nil, fmt.Errorf("acl_interface_list_dump(%d): %w", swIfIndex, err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, nil, fmt.Errorf("acl_interface_list_dump(%d): %w", swIfIndex, err)
	}
	for _, det := range details {
		if uint32(det.SwIfIndex) != swIfIndex || int(det.NInput) > len(det.Acls) {
			continue
		}
		in = append(in, det.Acls[:det.NInput]...)
		out = append(out, det.Acls[det.NInput:]...)
	}
	return in, out, nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceBindingDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	b, err := InterfaceBindingFromProto(obj)
	if err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	ifaces, err := dumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	swIfIndex, err := ifaces.indexShared(b.Interface)
	if err != nil {
		return nil, err
	}
	if err := d.setList(ctx, swIfIndex, b); err != nil {
		return nil, err
	}
	return BindingMeta{SwIfIndex: swIfIndex}, nil
}

// Update implements scheduler.Descriptor: the new lists replace the old ones in one
// acl_interface_set_acl_list (a reorder is an update, never a recreate). A different interface is
// a different object (ErrRecreate).
func (d *InterfaceBindingDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldB, err := InterfaceBindingFromProto(oldObj)
	if err != nil {
		return nil, err
	}
	newB, err := InterfaceBindingFromProto(newObj)
	if err != nil {
		return nil, err
	}
	if oldB.Interface != newB.Interface {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(BindingMeta)
	if !ok {
		return nil, fmt.Errorf("acl.interface-binding: unexpected meta %T", meta)
	}
	if err := d.setList(ctx, m.SwIfIndex, newB); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: removes this owner's ACLs from the interface (an empty
// list, n_input = 0, count = 0, when no other owner has ACLs there).
func (d *InterfaceBindingDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	b, err := InterfaceBindingFromProto(obj)
	if err != nil {
		return err
	}
	m, ok := meta.(BindingMeta)
	if !ok {
		return fmt.Errorf("acl.interface-binding: unexpected meta %T", meta)
	}
	return d.setList(ctx, m.SwIfIndex, InterfaceBinding{Interface: b.Interface})
}

// Retrieve implements scheduler.Descriptor: acl_interface_list_dump for all interfaces. Bindings
// have no tag of their own: the binding reports the entries whose ACL carries this owner's tag
// (in VPP order, with the n_input split); entries of other owners are skipped (setList preserves
// them), and an interface with none of our ACLs has no binding of ours.
func (d *InterfaceBindingDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := dumpOwnedACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	names := aclNameByIndex(owned)
	ifaces, err := dumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := acl.NewServiceClient(d.client).ACLInterfaceListDump(ctx, &acl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(allInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("acl_interface_list_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_interface_list_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		if len(det.Acls) == 0 || int(det.NInput) > len(det.Acls) {
			continue
		}
		iface, known := ifaces.byIndex[uint32(det.SwIfIndex)]
		if !known {
			continue
		}
		b := InterfaceBinding{Interface: iface.Name, Input: []string{}, Output: []string{}}
		for i, idx := range det.Acls {
			name, mine := names[idx]
			if !mine {
				continue // another owner's ACL: not part of our binding
			}
			if i < int(det.NInput) {
				b.Input = append(b.Input, name)
			} else {
				b.Output = append(b.Output, name)
			}
		}
		if len(b.Input)+len(b.Output) == 0 {
			continue
		}
		out = append(out, scheduler.KV{Key: KeyInterfaceBinding(b.Interface), Value: b.Proto(), Meta: BindingMeta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}
