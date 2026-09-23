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

// MacipBindingMeta is the runtime handle of an acl.macip-interface-binding object.
type MacipBindingMeta struct {
	SwIfIndex uint32
	ACLIndex  uint32
}

// KeyMacipBinding is the key of the MACIP binding on ifName: "acl.macip-interface-binding/<ifName>".
func KeyMacipBinding(ifName string) scheduler.Key {
	return scheduler.Join(NameMacipInterfaceBinding, ifName)
}

// MacipBindingDescriptor manages acl.macip-interface-binding objects with
// macip_acl_interface_add_del (one MACIP ACL per interface, inbound; adding another replaces
// it, acl.c macip_acl_interface_add_acl) and macip_acl_interface_list_dump. Ownership: the
// bound MACIP ACL's tag.
type MacipBindingDescriptor struct {
	client vpp.Client
	owner  string
	opts   options
}

var _ scheduler.Descriptor = (*MacipBindingDescriptor)(nil)

// NewMacipBinding returns the acl.macip-interface-binding descriptor for one owner.
func NewMacipBinding(client vpp.Client, owner string, opts ...Option) *MacipBindingDescriptor {
	return &MacipBindingDescriptor{client: client, owner: owner, opts: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*MacipBindingDescriptor) Name() string { return NameMacipInterfaceBinding }

// KeyOf implements scheduler.Descriptor: acl.macip-interface-binding/<interface>.
func (*MacipBindingDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	b, _ := MacipBindingFromProto(obj)
	return KeyMacipBinding(b.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface (optional) and the MACIP ACL
// (mandatory: VPP refuses macip_acl_del while bound).
func (d *MacipBindingDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	b, _ := MacipBindingFromProto(obj)
	return []scheduler.Dependency{d.opts.interfaceDependency(b.Interface), {Key: KeyMacipACL(b.ACL)}}
}

func (d *MacipBindingDescriptor) resolveACL(ctx context.Context, name string) (uint32, error) {
	owned, err := dumpOwnedMacipACLs(ctx, d.client, d.owner)
	if err != nil {
		return 0, err
	}
	idx, ok := macipIndexByName(owned)[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q (owner %q)", ErrNoMacipACL, name, d.owner)
	}
	return idx, nil
}

func (d *MacipBindingDescriptor) addDel(ctx context.Context, isAdd bool, swIfIndex, aclIndex uint32) error {
	req := &acl.MacipACLInterfaceAddDel{IsAdd: isAdd, SwIfIndex: interface_types.InterfaceIndex(swIfIndex), ACLIndex: aclIndex}
	if _, err := acl.NewServiceClient(d.client).MacipACLInterfaceAddDel(ctx, req); err != nil {
		return fmt.Errorf("macip_acl_interface_add_del(add=%v, sw_if_index=%d, acl=%d): %w", isAdd, swIfIndex, aclIndex, err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *MacipBindingDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	b, err := MacipBindingFromProto(obj)
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
	aclIndex, err := d.resolveACL(ctx, b.ACL)
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, true, swIfIndex, aclIndex); err != nil {
		return nil, err
	}
	return MacipBindingMeta{SwIfIndex: swIfIndex, ACLIndex: aclIndex}, nil
}

// Update implements scheduler.Descriptor: binding a different MACIP ACL is one add (VPP
// unapplies the previous one itself). A different interface is ErrRecreate.
func (d *MacipBindingDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldB, err := MacipBindingFromProto(oldObj)
	if err != nil {
		return nil, err
	}
	newB, err := MacipBindingFromProto(newObj)
	if err != nil {
		return nil, err
	}
	if oldB.Interface != newB.Interface {
		return nil, scheduler.ErrRecreate
	}
	if err := newB.Validate(); err != nil {
		return nil, err
	}
	m, ok := meta.(MacipBindingMeta)
	if !ok {
		return nil, fmt.Errorf("acl.macip-interface-binding: unexpected meta %T", meta)
	}
	aclIndex, err := d.resolveACL(ctx, newB.ACL)
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, true, m.SwIfIndex, aclIndex); err != nil {
		return nil, err
	}
	return MacipBindingMeta{SwIfIndex: m.SwIfIndex, ACLIndex: aclIndex}, nil
}

// Delete implements scheduler.Descriptor: macip_acl_interface_add_del with is_add = false.
func (d *MacipBindingDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(MacipBindingMeta)
	if !ok {
		return fmt.Errorf("acl.macip-interface-binding: unexpected meta %T", meta)
	}
	return d.addDel(ctx, false, m.SwIfIndex, m.ACLIndex)
}

// Retrieve implements scheduler.Descriptor: macip_acl_interface_list_dump for all interfaces
// (VPP reports only interfaces with a MACIP ACL applied), keeping bindings whose ACL is ours.
func (d *MacipBindingDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := dumpOwnedMacipACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	names := macipNameByIndex(owned)
	ifaces, err := dumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	stream, err := acl.NewServiceClient(d.client).MacipACLInterfaceListDump(ctx, &acl.MacipACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(allInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("macip_acl_interface_list_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("macip_acl_interface_list_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		if len(det.Acls) == 0 {
			continue
		}
		iface, known := ifaces.byIndex[uint32(det.SwIfIndex)]
		if !known {
			continue
		}
		aclIndex := det.Acls[0] // VPP holds exactly one MACIP ACL per interface
		name, ours := names[aclIndex]
		if !ours {
			continue
		}
		b := MacipBinding{Interface: iface.Name, ACL: name}
		out = append(out, scheduler.KV{Key: KeyMacipBinding(b.Interface), Value: b.Proto(), Meta: MacipBindingMeta{SwIfIndex: uint32(det.SwIfIndex), ACLIndex: aclIndex}})
	}
	return out, nil
}
