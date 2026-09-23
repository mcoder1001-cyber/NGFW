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

// VlanTagRewriteDescriptor implements l2.vlan-tag-rewrite (l2_interface_vlan_tag_rewrite). The
// op codes are VPP's l2_vtr_op_t (src/vnet/l2/l2_vtr.h); "disabled" (0) is the absence of the
// object and what Delete sets. Retrieve decodes sw_interface_details.vtr_*.
type VlanTagRewriteDescriptor struct{ base }

// NewVlanTagRewrite returns the descriptor for owner.
func NewVlanTagRewrite(c vpp.Client, owner string) *VlanTagRewriteDescriptor {
	return &VlanTagRewriteDescriptor{base{c, owner}}
}

// Name implements scheduler.Descriptor.
func (*VlanTagRewriteDescriptor) Name() string { return VlanTagRewriteName }

// KeyOf implements scheduler.Descriptor.
func (*VlanTagRewriteDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(VlanTagRewriteName, iface.RefID(obj.(*VlanTagRewrite).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
func (*VlanTagRewriteDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: scheduler.Key(obj.(*VlanTagRewrite).GetInterface())}}
}

func (d *VlanTagRewriteDescriptor) set(ctx context.Context, idx uint32, o *VlanTagRewrite) error {
	if o.GetTag1() > 4095 || o.GetTag2() > 4095 {
		return fmt.Errorf("l2.vlan-tag-rewrite: vlan tag out of range")
	}
	var push uint32
	if o.GetPushDot1Q() {
		push = 1
	}
	_, err := d.svc().L2InterfaceVlanTagRewrite(ctx, &l2api.L2InterfaceVlanTagRewrite{
		SwIfIndex: interface_types.InterfaceIndex(idx), VtrOp: uint32(o.GetOp()), PushDot1q: push, Tag1: o.GetTag1(), Tag2: o.GetTag2(), //nolint:gosec // enum values 0-8
	})
	if err != nil {
		return fmt.Errorf("l2_interface_vlan_tag_rewrite: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*VlanTagRewrite)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetOp() == VtrOp_VTR_OP_DISABLED || o.GetOp() > VtrOp_VTR_OP_TRANSLATE_2_2 {
		return nil, fmt.Errorf("l2.vlan-tag-rewrite: op %v is not a rewrite operation", o.GetOp())
	}
	idx, err := iface.Resolve(ctx, d.client, d.owner, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return iface.Meta{SwIfIndex: idx}, d.set(ctx, idx, o)
}

// Update implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return nil, err
	}
	o, n := oldObj.(*VlanTagRewrite), newObj.(*VlanTagRewrite)
	if o.GetInterface() != n.GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	if n.GetOp() == VtrOp_VTR_OP_DISABLED {
		return nil, fmt.Errorf("l2.vlan-tag-rewrite: op disabled is the absence of the object")
	}
	return m, d.set(ctx, m.SwIfIndex, n)
}

// Delete implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	return d.set(ctx, m.SwIfIndex, &VlanTagRewrite{Op: VtrOp_VTR_OP_DISABLED})
}

// Retrieve implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.Indexes() {
		key, ok := t.KeyFor(idx)
		if !ok {
			continue
		}
		row, _ := t.Details(idx)
		if row.VtrOp == 0 || row.VtrOp > uint32(VtrOp_VTR_OP_TRANSLATE_2_2) {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(VlanTagRewriteName, key.ID()),
			Value: &VlanTagRewrite{Interface: string(key), Op: VtrOp(row.VtrOp), PushDot1Q: row.VtrPushDot1q != 0, Tag1: row.VtrTag1, Tag2: row.VtrTag2}, //nolint:gosec // range-checked
			Meta:  iface.Meta{SwIfIndex: idx},
		})
	}
	return out, nil
}
