package iface

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SubinterfaceDescriptor implements interface.subinterface with create_subif / delete_subif.
// create_vlan_subif (the 802.1q exact-match shortcut) is not used: create_subif expresses the
// same thing with SUB_IF_API_FLAG_ONE_TAG|EXACT_MATCH and covers dot1ad, QinQ, default and
// untagged as well, so there is one code path and one Retrieve decoding.
type SubinterfaceDescriptor struct{ base }

// NewSubinterface returns the descriptor for owner.
func NewSubinterface(c vpp.Client, owner string) *SubinterfaceDescriptor {
	return &SubinterfaceDescriptor{base{c, owner}}
}

// SubinterfaceID is the object id: "<parent id>.<sub_id>", VPP's own naming of sub-interfaces.
func SubinterfaceID(o *Subinterface) string {
	return RefID(o.GetParent()) + "." + strconv.FormatUint(uint64(o.GetSubId()), 10)
}

// Name implements scheduler.Descriptor.
func (*SubinterfaceDescriptor) Name() string { return SubinterfaceName }

// KeyOf implements scheduler.Descriptor.
func (*SubinterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(SubinterfaceName, SubinterfaceID(obj.(*Subinterface)))
}

// Dependencies implements scheduler.Descriptor.
func (*SubinterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*Subinterface).GetParent())
}

// SubifFlags encodes the model into sub_if_flags. The tag count is derived: untagged → no
// tags, default → none (matches whatever else is unmatched), inner vlan/any → two tags, else one.
func SubifFlags(o *Subinterface) interface_types.SubIfFlags {
	var f interface_types.SubIfFlags
	switch {
	case o.GetUntagged():
		f |= interface_types.SUB_IF_API_FLAG_NO_TAGS
	case o.GetDefaultSubif():
	case o.GetInnerVlan() != 0 || o.GetInnerVlanAny():
		f |= interface_types.SUB_IF_API_FLAG_TWO_TAGS
	default:
		f |= interface_types.SUB_IF_API_FLAG_ONE_TAG
	}
	if o.GetDot1Ad() {
		f |= interface_types.SUB_IF_API_FLAG_DOT1AD
	}
	if o.GetExactMatch() {
		f |= interface_types.SUB_IF_API_FLAG_EXACT_MATCH
	}
	if o.GetDefaultSubif() {
		f |= interface_types.SUB_IF_API_FLAG_DEFAULT
	}
	if o.GetOuterVlanAny() {
		f |= interface_types.SUB_IF_API_FLAG_OUTER_VLAN_ID_ANY
	}
	if o.GetInnerVlanAny() {
		f |= interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY
	}
	return f
}

// DecodeSubif is the inverse of SubifFlags applied to a dump row.
func DecodeSubif(parent scheduler.Key, d *ifapi.SwInterfaceDetails) *Subinterface {
	f := d.SubIfFlags
	o := &Subinterface{
		Parent:       string(parent),
		SubId:        d.SubID,
		Dot1Ad:       f&interface_types.SUB_IF_API_FLAG_DOT1AD != 0,
		ExactMatch:   f&interface_types.SUB_IF_API_FLAG_EXACT_MATCH != 0,
		DefaultSubif: f&interface_types.SUB_IF_API_FLAG_DEFAULT != 0,
		Untagged:     f&interface_types.SUB_IF_API_FLAG_NO_TAGS != 0,
		OuterVlanAny: f&interface_types.SUB_IF_API_FLAG_OUTER_VLAN_ID_ANY != 0,
		InnerVlanAny: f&interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY != 0,
	}
	if d.SubNumberOfTags >= 1 && !o.OuterVlanAny {
		o.OuterVlan = uint32(d.SubOuterVlanID)
	}
	if d.SubNumberOfTags >= 2 && !o.InnerVlanAny {
		o.InnerVlan = uint32(d.SubInnerVlanID)
	}
	return o
}

// Create implements scheduler.Descriptor.
func (d *SubinterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Subinterface)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetOuterVlan() > 4095 || o.GetInnerVlan() > 4095 {
		return nil, fmt.Errorf("iface: vlan id out of range in %s", SubinterfaceID(o))
	}
	parent, err := d.resolve(ctx, o.GetParent())
	if err != nil {
		return nil, err
	}
	rep, err := d.svc().CreateSubif(ctx, &ifapi.CreateSubif{
		SwIfIndex:   interface_types.InterfaceIndex(parent),
		SubID:       o.GetSubId(),
		SubIfFlags:  SubifFlags(o),
		OuterVlanID: uint16(o.GetOuterVlan()), //nolint:gosec // range-checked above
		InnerVlanID: uint16(o.GetInnerVlan()), //nolint:gosec // range-checked above
	})
	if err != nil {
		return nil, fmt.Errorf("create_subif: %w", err)
	}
	idx := uint32(rep.SwIfIndex)
	if err := Tag(ctx, d.client, d.owner, SubinterfaceID(o), idx); err != nil {
		// an untagged sub-interface is invisible to Retrieve and would block every retry with
		// "sub-interface already exists": remove it (review M3)
		if _, derr := d.svc().DeleteSubif(ctx, &ifapi.DeleteSubif{SwIfIndex: rep.SwIfIndex}); derr != nil {
			return nil, fmt.Errorf("%w (and delete_subif of the untagged orphan %d: %v)", err, idx, derr)
		}
		return nil, err
	}
	return Meta{idx}, nil
}

// Update implements scheduler.Descriptor: every field of a sub-interface is part of its classification → recreate.
func (d *SubinterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *SubinterfaceDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	if _, err := d.svc().DeleteSubif(ctx, &ifapi.DeleteSubif{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("delete_subif: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *SubinterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok || key.Descriptor() != SubinterfaceName {
			continue
		}
		row := t.byIndex[idx]
		parent, ok := t.Ref(row.SupSwIfIndex)
		if !ok {
			continue // parent tagged by another owner: not something we can express
		}
		out = append(out, scheduler.KV{Key: key, Value: DecodeSubif(parent, row), Meta: Meta{idx}})
	}
	return out, nil
}

// Normalize implements scheduler.Normalizer: the parent reference in canonical alias form.
func (*SubinterfaceDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "parent")
}
