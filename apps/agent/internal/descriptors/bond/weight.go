package bond

import (
	"context"
	"errors"
	"fmt"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// WeightName is the descriptor of a bond member's weight (F-bonding).
const WeightName = "bond.member-weight"

// Weight is the desired weight of one bond membership (sw_interface_set_bond_weight). Bond and Interface are
// interface references like Member's (alias "interface/<name>" canonical, creator keys accepted). Weight 0 is VPP's
// value for "never set" (member_if_t is zeroed by bond_add_member), so a desired weight is 1 or more.
//
// The model is a dfkit spec (D-055 stand-in codec, D-077): the value is the canonical structpb of this struct.
type Weight struct {
	Bond      string `json:"bond"`
	Interface string `json:"interface"`
	Weight    uint32 `json:"weight"`
}

// Proto returns the canonical desired value.
func (w Weight) Proto() proto.Message { return dfkit.Encode(w) }

// WeightFromProto decodes a desired or retrieved value.
func WeightFromProto(m proto.Message) (Weight, error) {
	var w Weight
	err := dfkit.Decode(m, &w)
	return w, err
}

// WeightKey is "bond.member-weight/<bond>/<member>" (logical names).
func WeightKey(bond, member string) scheduler.Key {
	return scheduler.Join(WeightName, iface.RefID(bond), iface.RefID(member))
}

// WeightMeta is the member's sw_if_index at Create.
type WeightMeta struct{ SwIfIndex uint32 }

// WeightDescriptor implements bond.member-weight: VPP keeps the weight in the membership (member_if_t), so the
// object depends on its bond.member and dies with it; sw_member_interface_dump reports it (a real Retrieve, D-063).
// VPP accepts a weight only on active-backup bonds ("Weight valid for active-backup only").
type WeightDescriptor struct{ base }

// NewWeight returns the descriptor for owner.
func NewWeight(c vpp.Client, owner string) *WeightDescriptor {
	return &WeightDescriptor{base{c, owner}}
}

// Name implements scheduler.Descriptor.
func (*WeightDescriptor) Name() string { return WeightName }

func weightOf(obj proto.Message) Weight {
	w, _ := WeightFromProto(obj) // an undecodable value fails in Create; its key is then "bond.member-weight//"
	return w
}

// KeyOf implements scheduler.Descriptor.
func (*WeightDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	w := weightOf(obj)
	return WeightKey(w.Bond, w.Interface)
}

// Dependencies implements scheduler.Descriptor: the membership (which carries the weight in VPP).
func (*WeightDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	w := weightOf(obj)
	return []scheduler.Dependency{{Key: scheduler.Join(MemberName, iface.RefID(w.Bond), iface.RefID(w.Interface))}}
}

// Normalize implements scheduler.Normalizer: references in canonical alias form.
func (*WeightDescriptor) Normalize(obj proto.Message) proto.Message {
	w, err := WeightFromProto(obj)
	if err != nil {
		return obj
	}
	w.Bond, w.Interface = iface.CanonicalRef(w.Bond), iface.CanonicalRef(w.Interface)
	return w.Proto()
}

// member resolves the membership: the member's sw_if_index, when it is a member of that bond right now.
func (d *WeightDescriptor) member(ctx context.Context, w Weight) (uint32, bool, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return 0, false, err
	}
	bond, err := t.Index(w.Bond)
	if err != nil {
		return 0, false, err
	}
	member, err := t.Index(w.Interface)
	if err != nil {
		return 0, false, err
	}
	members, err := dfkit.Drain(d.memberStream(ctx, bond))
	if err != nil {
		return 0, false, fmt.Errorf("sw_member_interface_dump: %w", err)
	}
	for _, m := range members {
		if uint32(m.SwIfIndex) == member {
			return member, true, nil
		}
	}
	return member, false, nil
}

// memberStream opens sw_member_interface_dump of one bond (a failed open is returned by the first Recv).
func (d *WeightDescriptor) memberStream(ctx context.Context, bond uint32) (interface{ Close() error }, func() (*bondapi.SwMemberInterfaceDetails, error)) {
	stream, err := d.svc().SwMemberInterfaceDump(ctx, &bondapi.SwMemberInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(bond)})
	if err != nil {
		return nopCloser{}, func() (*bondapi.SwMemberInterfaceDetails, error) { return nil, err }
	}
	return stream, stream.Recv
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func (d *WeightDescriptor) set(ctx context.Context, member, weight uint32) error {
	_, err := d.svc().SwInterfaceSetBondWeight(ctx, &bondapi.SwInterfaceSetBondWeight{SwIfIndex: interface_types.InterfaceIndex(member), Weight: weight})
	if err != nil {
		return fmt.Errorf("sw_interface_set_bond_weight: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent: it sets the value).
func (d *WeightDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	w, err := WeightFromProto(obj)
	if err != nil {
		return nil, err
	}
	if w.Weight == 0 {
		return nil, dfkit.Specf("bond member weight must be 1 or more (0 is VPP's unset value)")
	}
	member, ok, err := d.member(ctx, w)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bond: %s is not a member of %s", w.Interface, w.Bond)
	}
	if err := d.set(ctx, member, w.Weight); err != nil {
		return nil, err
	}
	return WeightMeta{SwIfIndex: member}, nil
}

// Update implements scheduler.Descriptor: the weight changes in place (VPP re-sorts the active set).
func (d *WeightDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to 0 while the membership exists (re-resolved by name right before,
// D-071); a membership that is gone took its weight with it.
func (d *WeightDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	w, err := WeightFromProto(obj)
	if err != nil {
		return err
	}
	member, ok, err := d.member(ctx, w)
	switch {
	case errors.Is(err, iface.ErrNotFound):
		return nil // bond or member gone
	case err != nil:
		return err
	case !ok:
		return nil
	}
	if err := d.set(ctx, member, 0); err != nil && !dfkit.IsVPPError(err, api.INVALID_INTERFACE) {
		return err // INVALID_INTERFACE: detached between the dump and the set
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: the non-zero weights of the memberships Member.Retrieve reports (owned
// bonds, owned or claimed members).
func (d *WeightDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
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
		members, err := dfkit.Drain(d.memberStream(ctx, uint32(bd.SwIfIndex)))
		if err != nil {
			return nil, fmt.Errorf("sw_member_interface_dump: %w", err)
		}
		for _, m := range members {
			if m.Weight == 0 {
				continue
			}
			mk, ok := t.OwnedRef(uint32(m.SwIfIndex), MemberName)
			if !ok {
				continue
			}
			w := Weight{Bond: iface.CanonicalRef(string(keys[i])), Interface: string(mk), Weight: m.Weight}
			out = append(out, scheduler.KV{Key: WeightKey(w.Bond, w.Interface), Value: w.Proto(), Meta: WeightMeta{SwIfIndex: uint32(m.SwIfIndex)}})
		}
	}
	return dfkit.Dedupe(out), nil
}
