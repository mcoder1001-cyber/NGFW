package bond

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// MemberDescriptor implements bond.member (bond_add_member / bond_detach_member).
type MemberDescriptor struct{ base }

// NewMember returns the descriptor for owner.
func NewMember(c vpp.Client, owner string) *MemberDescriptor { return &MemberDescriptor{base{c, owner}} }

// MemberMeta holds the member and bond indexes.
type MemberMeta struct{ SwIfIndex, Bond uint32 }

func (*MemberDescriptor) Name() string { return MemberName }

func (*MemberDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*Member)
	return scheduler.Join(MemberName, iface.RefID(o.GetBond()), iface.RefID(o.GetInterface()))
}

func (*MemberDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Member)
	return []scheduler.Dependency{{Key: scheduler.Key(o.GetBond())}, {Key: scheduler.Key(o.GetInterface())}}
}

func (d *MemberDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Member)
	if !ok {
		return nil, ErrEmptyValue
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	bond, err := t.Index(o.GetBond())
	if err != nil {
		return nil, err
	}
	member, err := t.Index(o.GetInterface())
	if err != nil {
		return nil, err
	}
	_, err = d.svc().BondAddMember(ctx, &bondapi.BondAddMember{
		SwIfIndex: interface_types.InterfaceIndex(member), BondSwIfIndex: interface_types.InterfaceIndex(bond),
		IsPassive: o.GetPassive(), IsLongTimeout: o.GetLongTimeout(),
	})
	if err != nil {
		return nil, fmt.Errorf("bond_add_member: %w", err)
	}
	return MemberMeta{SwIfIndex: member, Bond: bond}, nil
}

// Update: passive / long-timeout are LACP negotiation parameters fixed at attach → recreate.
func (*MemberDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

func (d *MemberDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(MemberMeta)
	if !ok {
		return fmt.Errorf("bond: unexpected meta %T", meta)
	}
	if _, err := d.svc().BondDetachMember(ctx, &bondapi.BondDetachMember{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("bond_detach_member: %w", err)
	}
	return nil
}

func (d *MemberDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	bonds, keys, err := d.bonds(ctx, t)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for i, bd := range bonds {
		stream, err := d.svc().SwMemberInterfaceDump(ctx, &bondapi.SwMemberInterfaceDump{SwIfIndex: bd.SwIfIndex})
		if err != nil {
			return nil, fmt.Errorf("sw_member_interface_dump: %w", err)
		}
		for {
			m, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("sw_member_interface_dump: %w", err)
			}
			mk, ok := t.KeyFor(uint32(m.SwIfIndex))
			if !ok {
				continue
			}
			out = append(out, scheduler.KV{
				Key:   scheduler.Join(MemberName, keys[i].ID(), mk.ID()),
				Value: &Member{Bond: string(keys[i]), Interface: string(mk), Passive: m.IsPassive, LongTimeout: m.IsLongTimeout},
				Meta:  MemberMeta{SwIfIndex: uint32(m.SwIfIndex), Bond: uint32(bd.SwIfIndex)},
			})
		}
	}
	return out, nil
}
