package iface

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// UnnumberedName identifies the address-borrowing relationship.
const UnnumberedName = "interface.unnumbered"

// UnnumberedDescriptor retrieves authoritative VPP relationships, not a process cache.
// Physical interface claims are persisted by the same identity-bound store as other attributes.
type UnnumberedDescriptor struct{ base }

// NewUnnumbered constructs the descriptor.
func NewUnnumbered(c vpp.Client, owner string) *UnnumberedDescriptor {
	return &UnnumberedDescriptor{base{c, owner}}
}
func (*UnnumberedDescriptor) Name() string { return UnnumberedName }
func (*UnnumberedDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(UnnumberedName, RefID(obj.(*Unnumbered).Interface))
}
func (*UnnumberedDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface", "donor")
}
func (*UnnumberedDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Unnumbered)
	return []scheduler.Dependency{{Key: scheduler.Key(o.Interface)}, {Key: scheduler.Key(o.Donor)},
		{Key: scheduler.Join("interface-ip.table", RefID(o.Interface)), Optional: true},
		{Key: scheduler.Join("interface-ip.table", RefID(o.Donor)), Optional: true}}
}

func (d *UnnumberedDescriptor) relationships(ctx context.Context) (map[uint32]uint32, error) {
	stream, err := ip.NewServiceClient(d.client).IPUnnumberedDump(ctx, &ip.IPUnnumberedDump{SwIfIndex: interface_types.InterfaceIndex(AllInterfaces)})
	if err != nil {
		return nil, err
	}
	out := map[uint32]uint32{}
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out[uint32(row.SwIfIndex)] = uint32(row.IPSwIfIndex)
	}
}
func (d *UnnumberedDescriptor) set(ctx context.Context, borrower, donor uint32, add bool) error {
	_, err := d.svc().SwInterfaceSetUnnumbered(ctx, &ifapi.SwInterfaceSetUnnumbered{SwIfIndex: interface_types.InterfaceIndex(donor), UnnumberedSwIfIndex: interface_types.InterfaceIndex(borrower), IsAdd: add})
	return err
}
func (d *UnnumberedDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Unnumbered)
	if !ok {
		return nil, ErrEmptyValue
	}
	t, b, err := d.lookup(ctx, o.Interface)
	if err != nil {
		return nil, err
	}
	donor, err := t.Index(o.Donor)
	if err != nil {
		return nil, err
	}
	if b == donor {
		return nil, fmt.Errorf("unnumbered: cannot borrow from self")
	}
	// VPP shares both families; neither may cross VRFs, including the default table.
	for _, v6 := range []bool{false, true} {
		bt, e := d.svc().SwInterfaceGetTable(ctx, &ifapi.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(b), IsIPv6: v6})
		if e != nil {
			return nil, e
		}
		dt, e := d.svc().SwInterfaceGetTable(ctx, &ifapi.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(donor), IsIPv6: v6})
		if e != nil {
			return nil, e
		}
		if bt.VrfID != dt.VrfID {
			return nil, fmt.Errorf("unnumbered: donor and borrower must share IPv4 and IPv6 VRFs")
		}
	}
	rel, err := d.relationships(ctx)
	if err != nil {
		return nil, err
	}
	if _, exists := rel[b]; exists && !t.Owns(b, UnnumberedName) {
		return nil, fmt.Errorf("unnumbered: refusing existing unclaimed relationship")
	}
	for _, liveDonor := range rel {
		if liveDonor == b {
			return nil, fmt.Errorf("unnumbered: borrower is already a live donor")
		}
	}
	if _, nested := rel[donor]; nested {
		return nil, fmt.Errorf("unnumbered: donor must be numbered")
	}
	undo, err := d.claimFirst(ctx, t, b, UnnumberedName)
	if err != nil {
		return nil, err
	}
	if err = d.set(ctx, b, donor, true); err != nil {
		undo()
		return nil, err
	}
	return Meta{b}, nil
}
func (d *UnnumberedDescriptor) Update(_ context.Context, _, _ proto.Message, _ any) (any, error) {
	return nil, scheduler.ErrRecreate
}
func (d *UnnumberedDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*Unnumbered)
	if !ok {
		return ErrEmptyValue
	}
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return err
	}
	b, err := t.Index(o.Interface)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if b != m.SwIfIndex || !t.Owns(b, UnnumberedName) {
		return fmt.Errorf("unnumbered: borrower identity or ownership changed")
	}
	rel, err := d.relationships(ctx)
	if err != nil {
		return err
	}
	donor, exists := rel[b]
	if exists {
		ref, ok := t.Ref(donor)
		if !ok || string(ref) != CanonicalRef(o.Donor) {
			return fmt.Errorf("unnumbered: live donor changed; refusing deletion")
		}
		if err = d.set(ctx, b, donor, false); err != nil {
			return err
		}
	}
	d.release(obj, UnnumberedName)
	return nil
}
func (d *UnnumberedDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	rel, err := d.relationships(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, b := range t.order {
		donor, exists := rel[b]
		if !exists {
			continue
		}
		ref, owned := t.OwnedRef(b, UnnumberedName)
		if !owned {
			continue
		}
		dr, ok := t.Ref(donor)
		if !ok {
			return nil, fmt.Errorf("unnumbered: owned relationship has inaccessible donor")
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(UnnumberedName, ref.ID()), Value: &Unnumbered{Interface: string(ref), Donor: string(dr)}, Meta: Meta{b}})
	}
	return out, nil
}
