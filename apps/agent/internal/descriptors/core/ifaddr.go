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
// dependency: ordering + recreate cascade) and Update always recreates. An untagged interface is
// bound through the claim path (Env.Claims, holder InterfaceTableName, TD-11c).
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
func (d *InterfaceTableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asIfTable(obj)
	return []scheduler.Dependency{{Key: d.ifRef(v.GetInterface())}, {Key: VRFKey(v.GetTableId())}}
}

// setTable binds one family of sw_if_index idx to table (VPP refuses a family that has an address:
// ADDRESS_FOUND_FOR_INTERFACE).
func (d *InterfaceTableDescriptor) setTable(ctx context.Context, idx uint32, ifName string, v6 bool, table uint32) error {
	_, err := interfaces.NewServiceClient(d.Client).SwInterfaceSetTable(ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: v6, VrfID: table})
	if err != nil {
		return fmt.Errorf("sw_interface_set_table %s ipv6=%v table %d: %w", ifName, v6, table, err)
	}
	return nil
}

// bind sets the IPv4 table to t4, then the IPv6 table to t6, and reports whether IPv4 was set.
func (d *InterfaceTableDescriptor) bind(ctx context.Context, idx uint32, ifName string, t4, t6 uint32) (v4set bool, err error) {
	if err := d.setTable(ctx, idx, ifName, false, t4); err != nil {
		return false, err
	}
	return true, d.setTable(ctx, idx, ifName, true, t6)
}

// table4 reads the IPv4 table of sw_if_index idx.
func (d *InterfaceTableDescriptor) table4(ctx context.Context, idx uint32, ifName string) (uint32, error) {
	rep, err := interfaces.NewServiceClient(d.Client).SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return 0, fmt.Errorf("sw_interface_get_table %s: %w", ifName, err)
	}
	return rep.VrfID, nil
}

// Create implements scheduler.Descriptor. On an untagged interface the claim is recorded first; when
// the binding then fails, a family already changed gets its previous table back and the claim is
// released (it is kept only when that restore fails too, so Retrieve still sees the leftover and the
// next resync removes it).
func (d *InterfaceTableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*InterfaceTable)
	if !ok || v.GetTableId() == 0 {
		return nil, fmt.Errorf("%w %T (table id must be > 0)", ErrBadValue, obj)
	}
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	in, err := d.target(t, v.GetInterface())
	if err != nil {
		return nil, err
	}
	var p4 uint32
	if in.Untagged {
		if p4, err = d.table4(ctx, in.Index, v.GetInterface()); err != nil {
			return nil, err
		}
		if err := d.claim(in, InterfaceTableName); err != nil {
			return nil, err
		}
	}
	if v4set, err := d.bind(ctx, in.Index, v.GetInterface(), v.GetTableId(), v.GetTableId()); err != nil {
		if in.Untagged {
			if v4set { // IPv4 was bound, IPv6 failed: put IPv4 back (IPv6 is unchanged)
				if rerr := d.setTable(context.WithoutCancel(ctx), in.Index, v.GetInterface(), false, p4); rerr != nil {
					return nil, fmt.Errorf("%w (restoring IPv4 table %d: %v; claim kept)", err, p4, rerr)
				}
			}
			if rerr := d.release(in.VPPName, InterfaceTableName); rerr != nil {
				return nil, errors.Join(err, rerr)
			}
		}
		return nil, err
	}
	return IfMeta{SwIfIndex: in.Index}, nil
}

// Update implements scheduler.Descriptor: always recreate (addresses must be removed first).
func (*InterfaceTableDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: back to table 0.
// The interface is re-resolved by tag, an untagged one by our claim on its current sw_if_index
// (identity re-verified, D-071/D-080): a binding we do not hold is never touched; a vanished
// interface has no binding left (its claim is released).
func (d *InterfaceTableDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	name := asIfTable(obj).GetInterface()
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return err
	}
	in, err := d.target(t, name)
	if errors.Is(err, ErrNotOwned) {
		if _, exists := t.byName[name]; !exists {
			return d.release(name, InterfaceTableName) // the interface vanished: nothing left to unbind
		}
		return nil
	}
	if err != nil {
		return err
	}
	if _, ours := d.logical(in, InterfaceTableName); !ours {
		return nil
	}
	if _, err := d.bind(ctx, in.Index, name, 0, 0); err != nil {
		return err
	}
	if in.Untagged {
		return d.release(in.VPPName, InterfaceTableName)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: our interfaces (tagged, or untagged and claimed) whose
// IPv4 table is not 0.
func (d *InterfaceTableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	svc := interfaces.NewServiceClient(d.Client)
	var out []scheduler.KV
	for _, in := range t.all {
		name, ours := d.logical(in, InterfaceTableName)
		if !ours {
			continue
		}
		rep, err := svc.SwInterfaceGetTable(ctx, &interfaces.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(in.Index)})
		if err != nil {
			return nil, fmt.Errorf("sw_interface_get_table %s: %w", name, err)
		}
		if rep.VrfID == 0 {
			continue
		}
		out = append(out, scheduler.KV{Key: InterfaceTableKey(name), Value: &InterfaceTable{Interface: name, TableId: rep.VrfID}, Meta: IfMeta{SwIfIndex: in.Index}})
	}
	return out, nil
}

// ---- interface-ip ----------------------------------------------------------------------------

// InterfaceAddrDescriptor manages one address on an owned interface; on an untagged interface
// through the claim path (Env.Claims, holder AddrHolder(prefix), TD-11c).
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
func (d *InterfaceAddrDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asIfAddr(obj)
	return []scheduler.Dependency{
		{Key: d.ifRef(v.GetInterface())},
		{Key: InterfaceTableKey(v.GetInterface()), Optional: true},
	}
}

func (d *InterfaceAddrDescriptor) addDel(ctx context.Context, v *InterfaceAddress, idx uint32, add bool) error {
	pfx, err := ip_types.ParseAddressWithPrefix(v.GetPrefix())
	if err != nil {
		return fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	_, err = interfaces.NewServiceClient(d.Client).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{
		SwIfIndex: interface_types.InterfaceIndex(idx), IsAdd: add, Prefix: pfx,
	})
	if err != nil {
		return fmt.Errorf("sw_interface_add_del_address %s %s add=%v: %w", v.GetInterface(), v.GetPrefix(), add, err)
	}
	return nil
}

// Create implements scheduler.Descriptor. On an untagged interface the address's claim is recorded
// first and released again when VPP refuses the address (e.g. it exists already: never adopted).
func (d *InterfaceAddrDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*InterfaceAddress)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	if _, err := ip_types.ParseAddressWithPrefix(v.GetPrefix()); err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	in, err := d.target(t, v.GetInterface())
	if err != nil {
		return nil, err
	}
	holder := AddrHolder(v.GetPrefix())
	if err := d.claim(in, holder); err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, v, in.Index, true); err != nil {
		if in.Untagged {
			if rerr := d.release(in.VPPName, holder); rerr != nil {
				return nil, errors.Join(err, rerr)
			}
		}
		return nil, err
	}
	return IfMeta{SwIfIndex: in.Index}, nil
}

// Update implements scheduler.Descriptor: the value equals the key; nothing to update.
func (*InterfaceAddrDescriptor) Update(_ context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if !proto.Equal(oldObj, newObj) {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
// The interface is re-resolved by tag, an untagged one by our claim on its current sw_if_index
// (identity re-verified, D-071/D-080): an address we do not hold is never removed; a vanished
// interface has no address left (its claim is released).
func (d *InterfaceAddrDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v := asIfAddr(obj)
	holder := AddrHolder(v.GetPrefix())
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return err
	}
	in, err := d.target(t, v.GetInterface())
	if errors.Is(err, ErrNotOwned) {
		if _, exists := t.byName[v.GetInterface()]; !exists {
			return d.release(v.GetInterface(), holder) // the interface vanished: no address left
		}
		return nil
	}
	if err != nil {
		return err
	}
	if _, ours := d.logical(in, holder); !ours {
		return nil
	}
	if err := d.addDel(ctx, v, in.Index, false); err != nil {
		return err
	}
	if in.Untagged {
		return d.release(in.VPPName, holder)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: every address of every tagged interface of ours, and the
// claimed addresses of untagged interfaces.
func (d *InterfaceAddrDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	svc := ip.NewServiceClient(d.Client)
	var out []scheduler.KV
	for _, in := range t.all {
		if in.ID == "" && (!in.Untagged || d.Claims == nil) {
			continue
		}
		for _, v6 := range []bool{false, true} {
			stream, err := svc.IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(in.Index), IsIPv6: v6})
			if err != nil {
				return nil, fmt.Errorf("ip_address_dump %s: %w", in.VPPName, err)
			}
			for {
				a, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("ip_address_dump %s: %w", in.VPPName, err)
				}
				p, err := CanonAddrPrefix(a.Prefix.String())
				if err != nil {
					return nil, err
				}
				name, ours := d.logical(in, AddrHolder(p))
				if !ours {
					continue
				}
				out = append(out, scheduler.KV{
					Key:   InterfaceAddrKey(name, p),
					Value: &InterfaceAddress{Interface: name, Prefix: p},
					Meta:  IfMeta{SwIfIndex: in.Index},
				})
			}
		}
	}
	return out, nil
}
