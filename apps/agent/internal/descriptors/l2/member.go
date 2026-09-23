package l2

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// MemberDescriptor implements l2.bridge-domain-member (sw_interface_set_l2_bridge; enable=false
// returns the interface to L3 mode, which is VPP's "remove from bridge").
type MemberDescriptor struct{ base }

// NewMember returns the descriptor for owner.
func NewMember(c vpp.Client, owner string) *MemberDescriptor {
	return &MemberDescriptor{base{c, owner}}
}

// Name implements scheduler.Descriptor.
func (*MemberDescriptor) Name() string { return MemberName }

// KeyOf implements scheduler.Descriptor.
func (*MemberDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*BridgeDomainMember)
	return scheduler.Join(MemberName, bdID(o.GetBridgeDomain()), iface.RefID(o.GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
func (*MemberDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*BridgeDomainMember)
	return []scheduler.Dependency{{Key: BridgeDomainKey(o.GetBridgeDomain())}, {Key: scheduler.Key(o.GetInterface())}}
}

func portType(p PortType) (l2api.L2PortType, error) {
	switch p {
	case PortType_PORT_TYPE_NORMAL:
		return l2api.L2_API_PORT_TYPE_NORMAL, nil
	case PortType_PORT_TYPE_BVI:
		return l2api.L2_API_PORT_TYPE_BVI, nil
	case PortType_PORT_TYPE_UU_FWD:
		return l2api.L2_API_PORT_TYPE_UU_FWD, nil
	}
	return 0, fmt.Errorf("l2: unknown port type %v", p)
}

// Create implements scheduler.Descriptor.
func (d *MemberDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*BridgeDomainMember)
	if !ok {
		return nil, ErrEmptyValue
	}
	pt, err := portType(o.GetPortType())
	if err != nil {
		return nil, err
	}
	if o.GetShg() > 255 {
		return nil, fmt.Errorf("l2: shg %d out of range", o.GetShg())
	}
	idx, err := iface.Resolve(ctx, d.client, d.owner, o.GetInterface())
	if err != nil {
		return nil, err
	}
	_, err = d.svc().SwInterfaceSetL2Bridge(ctx, &l2api.SwInterfaceSetL2Bridge{
		RxSwIfIndex: interface_types.InterfaceIndex(idx), BdID: o.GetBridgeDomain(), PortType: pt, Shg: uint8(o.GetShg()), Enable: true, //nolint:gosec // range-checked
	})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_set_l2_bridge: %w", err)
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: shg / port type / bridge changes go through leave + join (recreate).
func (*MemberDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *MemberDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	o, ok := obj.(*BridgeDomainMember)
	if !ok {
		return ErrEmptyValue
	}
	_, err = d.svc().SwInterfaceSetL2Bridge(ctx, &l2api.SwInterfaceSetL2Bridge{
		RxSwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), BdID: o.GetBridgeDomain(), Enable: false,
	})
	if err != nil {
		return fmt.Errorf("sw_interface_set_l2_bridge (disable): %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *MemberDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	bds, err := d.bridgeDomains(ctx)
	if err != nil {
		return nil, err
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, bd := range bds {
		for _, sw := range bd.SwIfDetails {
			idx := uint32(sw.SwIfIndex)
			key, ok := t.Ref(idx) // ours or untagged (a physical NIC in our bridge); never another owner's
			if !ok {
				continue
			}
			pt := PortType_PORT_TYPE_NORMAL
			switch {
			case uint32(bd.BviSwIfIndex) == idx:
				pt = PortType_PORT_TYPE_BVI
			case uint32(bd.UuFwdSwIfIndex) == idx:
				pt = PortType_PORT_TYPE_UU_FWD
			}
			out = append(out, scheduler.KV{
				Key:   scheduler.Join(MemberName, bdID(bd.BdID), key.ID()),
				Value: &BridgeDomainMember{BridgeDomain: bd.BdID, Interface: string(key), PortType: pt, Shg: uint32(sw.Shg)},
				Meta:  iface.Meta{SwIfIndex: idx},
			})
		}
	}
	return out, nil
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*MemberDescriptor) Normalize(obj proto.Message) proto.Message {
	return iface.NormalizeRefs(obj, "interface")
}

// MemberKey is the key of interface ref's membership in bridge domain bd.
func MemberKey(bd uint32, ref string) scheduler.Key {
	return scheduler.Join(MemberName, bdID(bd), iface.RefID(ref))
}
