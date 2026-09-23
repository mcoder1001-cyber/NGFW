package core

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/scheduler"
)

// ---- interface-ip.table ----------------------------------------------------------------------

// InterfaceTableDescriptor binds an owned interface to a FIB table (IPv4 and IPv6). VPP refuses to
// rebind an interface that has addresses, so the address objects depend on this one (optional
// dependency: ordering + recreate cascade) and Update always recreates.
type InterfaceTableDescriptor struct{ Env }

var _ scheduler.Descriptor = (*InterfaceTableDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*InterfaceTableDescriptor) Name() string { return InterfaceTableName }

func asIfTable(obj proto.Message) *InterfaceTable {
	v, _ := obj.(*InterfaceTable)
	if v == nil {
		return &InterfaceTable{}
	}
	return v
}

// KeyOf implements scheduler.Descriptor.
func (*InterfaceTableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return InterfaceTableKey(asIfTable(obj).GetInterface())
}

// Dependencies implements scheduler.Descriptor: the interface and the VRF table.
func (*InterfaceTableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asIfTable(obj)
	return []scheduler.Dependency{{Key: InterfaceKey(v.GetInterface())}, {Key: VRFKey(v.GetTableId())}}
}

func (d *InterfaceTableDescriptor) set(ctx context.Context, ifName string, meta any, table uint32) (IfMeta, error) {
	m, ok := meta.(IfMeta)
	if !ok {
		t, err := dumpInterfaces(ctx, d.Client, d.Owner)
		if err != nil {
			return IfMeta{}, err
		}
		in, err := t.owned(ifName)
		if err != nil {
			return IfMeta{}, err
		}
		m = IfMeta{SwIfIndex: in.Index}
	}
	svc := interfaces.NewServiceClient(d.Client)
	for _, v6 := range []bool{false, true} {
		if _, err := svc.SwInterfaceSetTable(ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), IsIPv6: v6, VrfID: table}); err != nil {
			return IfMeta{}, fmt.Errorf("sw_interface_set_table %s ipv6=%v table %d: %w", ifName, v6, table, err)
		}
	}
	return m, nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceTableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*InterfaceTable)
	if !ok || v.GetTableId() == 0 {
		return nil, fmt.Errorf("%w %T (table id must be > 0)", ErrBadValue, obj)
	}
	return d.set(ctx, v.GetInterface(), nil, v.GetTableId())
}

// Update implements scheduler.Descriptor: always recreate (addresses must be removed first).
func (*InterfaceTableDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: back to table 0.
func (d *InterfaceTableDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	_, err := d.set(ctx, asIfTable(obj).GetInterface(), meta, 0)
	return err
}

// Retrieve implements scheduler.Descriptor: owned interfaces whose IPv4 table is not 0.
func (d *InterfaceTableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	svc := interfaces.NewServiceClient(d.Client)
	var out []scheduler.KV
	for _, in := range t.all {
		if in.ID == "" {
			continue
		}
		rep, err := svc.SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(in.Index)})
		if err != nil {
			return nil, fmt.Errorf("sw_interface_get_table %s: %w", in.ID, err)
		}
		if rep.VrfID == 0 {
			continue
		}
		out = append(out, scheduler.KV{Key: InterfaceTableKey(in.ID), Value: &InterfaceTable{Interface: in.ID, TableId: rep.VrfID}, Meta: IfMeta{SwIfIndex: in.Index}})
	}
	return out, nil
}

// ---- interface-ip ----------------------------------------------------------------------------

// InterfaceAddrDescriptor manages one address on an owned interface.
type InterfaceAddrDescriptor struct{ Env }

var _ scheduler.Descriptor = (*InterfaceAddrDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*InterfaceAddrDescriptor) Name() string { return InterfaceAddrName }

func asIfAddr(obj proto.Message) *InterfaceAddress {
	v, _ := obj.(*InterfaceAddress)
	if v == nil {
		return &InterfaceAddress{}
	}
	return v
}

// KeyOf implements scheduler.Descriptor.
func (*InterfaceAddrDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	v := asIfAddr(obj)
	return InterfaceAddrKey(v.GetInterface(), v.GetPrefix())
}

// Dependencies implements scheduler.Descriptor: the interface; the table binding (optional —
// ordering only, and it makes a rebind recreate the addresses).
func (*InterfaceAddrDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asIfAddr(obj)
	return []scheduler.Dependency{
		{Key: InterfaceKey(v.GetInterface())},
		{Key: InterfaceTableKey(v.GetInterface()), Optional: true},
	}
}

func (d *InterfaceAddrDescriptor) addDel(ctx context.Context, v *InterfaceAddress, meta any, add bool) (IfMeta, error) {
	pfx, err := ip_types.ParseAddressWithPrefix(v.GetPrefix())
	if err != nil {
		return IfMeta{}, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	m, ok := meta.(IfMeta)
	if !ok {
		t, err := dumpInterfaces(ctx, d.Client, d.Owner)
		if err != nil {
			return IfMeta{}, err
		}
		in, err := t.owned(v.GetInterface())
		if err != nil {
			return IfMeta{}, err
		}
		m = IfMeta{SwIfIndex: in.Index}
	}
	_, err = interfaces.NewServiceClient(d.Client).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{
		SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), IsAdd: add, Prefix: pfx,
	})
	if err != nil {
		return IfMeta{}, fmt.Errorf("sw_interface_add_del_address %s %s add=%v: %w", v.GetInterface(), v.GetPrefix(), add, err)
	}
	return m, nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceAddrDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*InterfaceAddress)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	return d.addDel(ctx, v, nil, true)
}

// Update implements scheduler.Descriptor: the value equals the key; nothing to update.
func (*InterfaceAddrDescriptor) Update(_ context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if !proto.Equal(oldObj, newObj) {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
func (d *InterfaceAddrDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	_, err := d.addDel(ctx, asIfAddr(obj), meta, false)
	return err
}

// Retrieve implements scheduler.Descriptor: every address of every owned interface.
func (d *InterfaceAddrDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	svc := ip.NewServiceClient(d.Client)
	var out []scheduler.KV
	for _, in := range t.all {
		if in.ID == "" {
			continue
		}
		for _, v6 := range []bool{false, true} {
			stream, err := svc.IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(in.Index), IsIPv6: v6})
			if err != nil {
				return nil, fmt.Errorf("ip_address_dump %s: %w", in.ID, err)
			}
			for {
				a, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("ip_address_dump %s: %w", in.ID, err)
				}
				p, err := CanonAddrPrefix(a.Prefix.String())
				if err != nil {
					return nil, err
				}
				out = append(out, scheduler.KV{
					Key:   InterfaceAddrKey(in.ID, p),
					Value: &InterfaceAddress{Interface: in.ID, Prefix: p},
					Meta:  IfMeta{SwIfIndex: in.Index},
				})
			}
		}
	}
	return out, nil
}
