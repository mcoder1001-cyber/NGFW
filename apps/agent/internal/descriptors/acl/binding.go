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
// acl_interface_set_acl_list (the whole in+out list is the unit of desired state; an empty list
// unbinds) and acl_interface_list_dump.
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

// Dependencies implements scheduler.Descriptor: the interface (optional; see WithInterfaceKey)
// and every acl.acl the lists name (mandatory: an ACL is created before it is bound and unbound
// before it is deleted — VPP refuses acl_del while bound).
func (d *InterfaceBindingDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	b, _ := InterfaceBindingFromProto(obj)
	deps := []scheduler.Dependency{d.opts.interfaceDependency(b.Interface)}
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
// names to this owner's indexes.
func (d *InterfaceBindingDescriptor) setList(ctx context.Context, swIfIndex uint32, b InterfaceBinding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	var byName map[string]uint32
	if len(b.Input)+len(b.Output) > 0 {
		owned, err := dumpOwnedACLs(ctx, d.client, d.owner)
		if err != nil {
			return err
		}
		byName = aclIndexByName(owned)
	}
	acls := make([]uint32, 0, len(b.Input)+len(b.Output))
	for _, name := range append(append([]string{}, b.Input...), b.Output...) {
		idx, ok := byName[name]
		if !ok {
			return fmt.Errorf("%w: %q (owner %q) while binding to %q", ErrNoACL, name, d.owner, b.Interface)
		}
		acls = append(acls, idx)
	}
	req := &acl.ACLInterfaceSetACLList{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		NInput:    uint8(len(b.Input)), //nolint:gosec // Validate bounds the lists at 255
		Acls:      acls,
	}
	if _, err := acl.NewServiceClient(d.client).ACLInterfaceSetACLList(ctx, req); err != nil {
		return fmt.Errorf("acl_interface_set_acl_list %q: %w", b.Interface, err)
	}
	return nil
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
	ifaces, err := dumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	swIfIndex, err := ifaces.index(b.Interface)
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

// Delete implements scheduler.Descriptor: an empty list (n_input = 0, count = 0) unbinds.
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

// Retrieve implements scheduler.Descriptor: acl_interface_list_dump for all interfaces; a
// binding is ours when it is non-empty and every ACL in it carries this owner's tag (bindings
// have no tag of their own). Lists are decoded in VPP order with the n_input split.
func (d *InterfaceBindingDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := dumpOwnedACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	names := aclNameByIndex(owned)
	ifaces, err := dumpInterfaces(ctx, d.client)
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
		ours := true
		for i, idx := range det.Acls {
			name, owned := names[idx]
			if !owned {
				ours = false
				break
			}
			if i < int(det.NInput) {
				b.Input = append(b.Input, name)
			} else {
				b.Output = append(b.Output, name)
			}
		}
		if !ours {
			continue
		}
		out = append(out, scheduler.KV{Key: KeyInterfaceBinding(b.Interface), Value: b.Proto(), Meta: BindingMeta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}
