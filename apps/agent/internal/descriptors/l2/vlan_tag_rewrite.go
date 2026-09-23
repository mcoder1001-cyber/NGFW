package l2

import (
	"context"
	"errors"
	"fmt"
	"io"

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
	o := obj.(*VlanTagRewrite)
	deps := []scheduler.Dependency{{Key: scheduler.Key(o.GetInterface())}}
	// leaving/re-joining a bridge or an xconnect clears the rewrite in VPP (l2_input.c
	// set_int_l2_mode): depend on the L2 membership so it is re-created with it (review M5)
	if o.GetBridgeDomain() != 0 {
		deps = append(deps, scheduler.Dependency{Key: MemberKey(o.GetBridgeDomain(), o.GetInterface())})
	}
	if o.GetXconnect() {
		deps = append(deps, scheduler.Dependency{Key: XconnectKey(o.GetInterface())})
	}
	return deps
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*VlanTagRewriteDescriptor) Normalize(obj proto.Message) proto.Message {
	return iface.NormalizeRefs(obj, "interface")
}

// l2Mode returns idx's bridge domain (0 = none; only owned bridge domains count) and whether it
// is the rx side of a cross-connect.
func (d *VlanTagRewriteDescriptor) l2Mode(ctx context.Context) (map[uint32]uint32, map[uint32]bool, error) {
	bds, err := d.bridgeDomains(ctx)
	if err != nil {
		return nil, nil, err
	}
	bdOf := map[uint32]uint32{}
	for _, bd := range bds {
		for _, sw := range bd.SwIfDetails {
			bdOf[uint32(sw.SwIfIndex)] = bd.BdID
		}
	}
	stream, err := d.svc().L2XconnectDump(ctx, &l2api.L2XconnectDump{})
	if err != nil {
		return nil, nil, fmt.Errorf("l2_xconnect_dump: %w", err)
	}
	xc := map[uint32]bool{}
	for {
		x, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("l2_xconnect_dump: %w", err)
		}
		xc[uint32(x.RxSwIfIndex)] = true
	}
	return bdOf, xc, nil
}

// checkMode rejects a desired L2 mode that differs from the interface's actual one (the object
// would never compare equal to Retrieve).
func (d *VlanTagRewriteDescriptor) checkMode(ctx context.Context, idx uint32, o *VlanTagRewrite) error {
	bdOf, xc, err := d.l2Mode(ctx)
	if err != nil {
		return err
	}
	if bdOf[idx] != o.GetBridgeDomain() || xc[idx] != o.GetXconnect() {
		return fmt.Errorf("l2.vlan-tag-rewrite: %s is in bridge domain %d / xconnect %v, desired says %d / %v",
			o.GetInterface(), bdOf[idx], xc[idx], o.GetBridgeDomain(), o.GetXconnect())
	}
	return nil
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
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := t.Index(o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.checkMode(ctx, idx, o); err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, o); err != nil {
		return nil, err
	}
	return iface.Meta{SwIfIndex: idx}, t.ClaimIfUntagged(idx, VlanTagRewriteName)
}

// Update implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return nil, err
	}
	o, n := oldObj.(*VlanTagRewrite), newObj.(*VlanTagRewrite)
	if o.GetInterface() != n.GetInterface() || o.GetBridgeDomain() != n.GetBridgeDomain() || o.GetXconnect() != n.GetXconnect() {
		return nil, scheduler.ErrRecreate
	}
	if n.GetOp() == VtrOp_VTR_OP_DISABLED {
		return nil, fmt.Errorf("l2.vlan-tag-rewrite: op disabled is the absence of the object")
	}
	return m, d.set(ctx, m.SwIfIndex, n)
}

// Delete implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	if err := d.set(ctx, m.SwIfIndex, &VlanTagRewrite{Op: VtrOp_VTR_OP_DISABLED}); err != nil {
		return err
	}
	if o, ok := obj.(*VlanTagRewrite); ok {
		_ = iface.ReleaseRef(d.owner, o.GetInterface(), VlanTagRewriteName)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *VlanTagRewriteDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	bdOf, xc, err := d.l2Mode(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.Indexes() {
		key, ok := t.OwnedRef(idx, VlanTagRewriteName)
		if !ok {
			continue
		}
		row, _ := t.Details(idx)
		if row.VtrOp == 0 || row.VtrOp > uint32(VtrOp_VTR_OP_TRANSLATE_2_2) {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(VlanTagRewriteName, key.ID()),
			Value: &VlanTagRewrite{Interface: string(key), Op: VtrOp(row.VtrOp), PushDot1Q: row.VtrPushDot1q != 0, Tag1: row.VtrTag1, Tag2: row.VtrTag2, BridgeDomain: bdOf[idx], Xconnect: xc[idx]}, //nolint:gosec // range-checked
			Meta:  iface.Meta{SwIfIndex: idx},
		})
	}
	return out, nil
}
