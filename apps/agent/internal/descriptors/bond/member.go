package bond

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// MemberDescriptor implements bond.member (bond_add_member / bond_detach_member).
type MemberDescriptor struct{ base }

// NewMember returns the descriptor for owner.
func NewMember(c vpp.Client, owner string) *MemberDescriptor {
	return &MemberDescriptor{base{c, owner}}
}

// MemberMeta holds the member and bond indexes.
type MemberMeta struct{ SwIfIndex, Bond uint32 }

// Name implements scheduler.Descriptor.
func (*MemberDescriptor) Name() string { return MemberName }

// KeyOf implements scheduler.Descriptor.
func (*MemberDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*Member)
	return scheduler.Join(MemberName, iface.RefID(o.GetBond()), iface.RefID(o.GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
func (*MemberDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Member)
	return []scheduler.Dependency{{Key: scheduler.Key(o.GetBond())}, {Key: scheduler.Key(o.GetInterface())}}
}

// Create implements scheduler.Descriptor.
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
	if err := ethernetMember(t, member, o.GetInterface()); err != nil {
		return nil, err
	}
	_, err = d.svc().BondAddMember(ctx, &bondapi.BondAddMember{
		SwIfIndex: interface_types.InterfaceIndex(member), BondSwIfIndex: interface_types.InterfaceIndex(bond),
		IsPassive: o.GetPassive(), IsLongTimeout: o.GetLongTimeout(),
	})
	if err != nil {
		return nil, fmt.Errorf("bond_add_member: %w", err)
	}
	// a physical (untagged) member is ours only through the claim (review H2)
	return MemberMeta{SwIfIndex: member, Bond: bond}, t.ClaimIfUntagged(member, MemberName)
}

// ErrNotEthernet is returned (wrapped) by Create for a member that is not an Ethernet NIC.
var ErrNotEthernet = errors.New("bond: a member must be an Ethernet NIC")

// ethernetMember refuses, before bond_add_member, what VPP 26.06 does not check (review F4): bond_add_member rejects only
// a bond and then copies the member's hardware address (vnet/bonding/cli.c, memcpy of hw_address), which a tunnel or other
// L3 interface does not have — a likely SIGSEGV of the shared VPP. A member must be a hardware interface (not a
// sub-interface), not a loopback or bond, and carry an L2 address (VPP reports one only for Ethernet hardware).
func ethernetMember(t *iface.Table, idx uint32, ref string) error {
	d, ok := t.Details(idx)
	if !ok {
		return fmt.Errorf("%w: %s is not in the interface table", ErrNotEthernet, ref)
	}
	dev := strings.TrimRight(d.InterfaceDevType, "\x00")
	switch {
	case d.Type == interface_types.IF_API_TYPE_SUB || d.SupSwIfIndex != uint32(d.SwIfIndex):
		return fmt.Errorf("%w: %s is a sub-interface", ErrNotEthernet, ref)
	case dev == "Loopback" || dev == "bond":
		return fmt.Errorf("%w: %s is a %s interface", ErrNotEthernet, ref, dev)
	case d.L2Address == [6]uint8{}:
		return fmt.Errorf("%w: %s has no L2 address (device class %q, not Ethernet)", ErrNotEthernet, ref, dev)
	}
	return nil
}

// Update implements scheduler.Descriptor: passive / long-timeout are LACP negotiation parameters fixed at attach → recreate.
func (*MemberDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *MemberDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(MemberMeta)
	if !ok {
		return fmt.Errorf("bond: unexpected meta %T", meta)
	}
	if _, err := d.svc().BondDetachMember(ctx, &bondapi.BondDetachMember{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("bond_detach_member: %w", err)
	}
	if o, ok := obj.(*Member); ok {
		_ = iface.ReleaseRef(d.owner, o.GetInterface(), MemberName)
	}
	return nil
}

// CheckPersistent declares for the product agent's ownership guard (TD-11b, dfkit/persist protocol): a membership of an
// untagged NIC is ours only through the claim recorded in the owner's iface.ClaimStore, which must survive an agent
// restart (the product wiring installs subsystems.IfaceClaims, D-075); the in-memory default is for tests.
func (d *MemberDescriptor) CheckPersistent() error {
	return persist.Require("bond.member of owner "+d.owner+": claims on untagged member NICs (install a persisted store with iface.SetClaimStore)", iface.Claims(d.owner))
}

// Normalize implements scheduler.Normalizer: bond and member references in canonical alias form.
func (*MemberDescriptor) Normalize(obj proto.Message) proto.Message {
	return iface.NormalizeRefs(obj, "bond", "interface")
}

// Retrieve implements scheduler.Descriptor.
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
			mk, ok := t.OwnedRef(uint32(m.SwIfIndex), MemberName)
			if !ok {
				continue
			}
			out = append(out, scheduler.KV{
				Key:   scheduler.Join(MemberName, keys[i].ID(), mk.ID()),
				Value: &Member{Bond: iface.CanonicalRef(string(keys[i])), Interface: string(mk), Passive: m.IsPassive, LongTimeout: m.IsLongTimeout},
				Meta:  MemberMeta{SwIfIndex: uint32(m.SwIfIndex), Bond: uint32(bd.SwIfIndex)},
			})
		}
	}
	return out, nil
}
