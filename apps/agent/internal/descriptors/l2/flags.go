package l2

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ErrEqualsBridgeDefault is returned by l2.flags Create/Update when the requested flags equal the
// member default (memberDefault: everything on, learning off for the BVI): such an object would
// vanish from Retrieve, so it must be removed from the desired state instead.
var ErrEqualsBridgeDefault = errors.New("l2.flags: flags equal the bridge member defaults; remove the object instead")

// FlagsDescriptor implements l2.flags with l2_interface_feat_flags_set / _get: the per-interface
// learn/forward/flood/uu-flood/arp-term/arp-ufwd mask of a bridge member (ANDed with the bridge
// domain's flags by VPP). The object exists only while the mask differs from memberDefault.
type FlagsDescriptor struct{ base }

// NewFlags returns the descriptor for owner.
func NewFlags(c vpp.Client, owner string) *FlagsDescriptor { return &FlagsDescriptor{base{c, owner}} }

// Name implements scheduler.Descriptor.
func (*FlagsDescriptor) Name() string { return FlagsName }

// KeyOf implements scheduler.Descriptor.
func (*FlagsDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(FlagsName, iface.RefID(obj.(*Flags).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
func (*FlagsDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: scheduler.Key(obj.(*Flags).GetInterface())}}
}

func featOf(o *Flags) l2api.L2IntfFeatFlags {
	var f l2api.L2IntfFeatFlags
	if o.GetLearn() {
		f |= l2api.L2_INTF_FEAT_LEARN
	}
	if o.GetForward() {
		f |= l2api.L2_INTF_FEAT_FWD
	}
	if o.GetFlood() {
		f |= l2api.L2_INTF_FEAT_FLOOD
	}
	if o.GetUuFlood() {
		f |= l2api.L2_INTF_FEAT_UU_FLOOD
	}
	if o.GetArpTerm() {
		f |= l2api.L2_INTF_FEAT_ARP_TERM
	}
	if o.GetArpUfwd() {
		f |= l2api.L2_INTF_FEAT_ARP_UFWD
	}
	return f
}

func decodeFeat(ref string, f l2api.L2IntfFeatFlags) *Flags {
	return &Flags{
		Interface: ref,
		Learn:     f&l2api.L2_INTF_FEAT_LEARN != 0,
		Forward:   f&l2api.L2_INTF_FEAT_FWD != 0,
		Flood:     f&l2api.L2_INTF_FEAT_FLOOD != 0,
		UuFlood:   f&l2api.L2_INTF_FEAT_UU_FLOOD != 0,
		ArpTerm:   f&l2api.L2_INTF_FEAT_ARP_TERM != 0,
		ArpUfwd:   f&l2api.L2_INTF_FEAT_ARP_UFWD != 0,
	}
}

// memberDefault is what a member's interface feature bitmap is without an l2.flags object. VPP
// 26.06 sets it when the interface joins the bridge, independent of the bridge domain's own
// flags (l2_input.c set_int_l2_mode: FWD|UU_FLOOD|UU_FWD|FLOOD|LEARN|ARP_UFWD|ARP_TERM, and for a
// BVI LEARN cleared again — "no use since l2fib entry is static"); the data path ANDs it with
// the bridge domain's bitmap, so the per-interface flags are a mask. Verified on the host with
// l2_interface_feat_flags_get (docs/agent/descriptors/l2.md).
func memberDefault(bd *l2api.BridgeDomainDetails, idx uint32) l2api.L2IntfFeatFlags {
	f := l2api.L2_INTF_FEAT_LEARN | l2api.L2_INTF_FEAT_FWD | l2api.L2_INTF_FEAT_FLOOD | l2api.L2_INTF_FEAT_UU_FLOOD |
		l2api.L2_INTF_FEAT_ARP_TERM | l2api.L2_INTF_FEAT_ARP_UFWD
	if uint32(bd.BviSwIfIndex) == idx {
		f &^= l2api.L2_INTF_FEAT_LEARN
	}
	return f
}

// bridgeOf returns the owned bridge domain idx is a member of. It scans the full owned dump: on
// VPP 26.06 bridge_domain_dump's sw_if_index filter returned nothing for a member (host test),
// and a member of another owner's bridge is not ours to override anyway.
func (d *FlagsDescriptor) bridgeOf(ctx context.Context, idx uint32) (*l2api.BridgeDomainDetails, error) {
	bds, err := d.bridgeDomains(ctx)
	if err != nil {
		return nil, err
	}
	for _, bd := range bds {
		for _, sw := range bd.SwIfDetails {
			if uint32(sw.SwIfIndex) == idx {
				return bd, nil
			}
		}
	}
	return nil, fmt.Errorf("l2.flags: interface %d is not a member of an owned bridge domain", idx)
}

func (d *FlagsDescriptor) apply(ctx context.Context, idx uint32, want l2api.L2IntfFeatFlags) error {
	cur, err := d.svc().L2InterfaceFeatFlagsGet(ctx, &l2api.L2InterfaceFeatFlagsGet{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return fmt.Errorf("l2_interface_feat_flags_get: %w", err)
	}
	if set := want &^ cur.Flags; set != 0 {
		if _, err := d.svc().L2InterfaceFeatFlagsSet(ctx, &l2api.L2InterfaceFeatFlagsSet{SwIfIndex: interface_types.InterfaceIndex(idx), IsSet: true, Flags: set}); err != nil {
			return fmt.Errorf("l2_interface_feat_flags_set: %w", err)
		}
	}
	if clr := cur.Flags &^ want; clr != 0 {
		if _, err := d.svc().L2InterfaceFeatFlagsSet(ctx, &l2api.L2InterfaceFeatFlagsSet{SwIfIndex: interface_types.InterfaceIndex(idx), IsSet: false, Flags: clr}); err != nil {
			return fmt.Errorf("l2_interface_feat_flags_set: %w", err)
		}
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *FlagsDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Flags)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := iface.Resolve(ctx, d.client, d.owner, o.GetInterface())
	if err != nil {
		return nil, err
	}
	bd, err := d.bridgeOf(ctx, idx)
	if err != nil {
		return nil, err
	}
	if featOf(o) == memberDefault(bd, idx) {
		return nil, ErrEqualsBridgeDefault
	}
	return iface.Meta{SwIfIndex: idx}, d.apply(ctx, idx, featOf(o))
}

// Update implements scheduler.Descriptor.
func (d *FlagsDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return nil, err
	}
	o, n := oldObj.(*Flags), newObj.(*Flags)
	if o.GetInterface() != n.GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	bd, err := d.bridgeOf(ctx, m.SwIfIndex)
	if err != nil {
		return nil, err
	}
	if featOf(n) == memberDefault(bd, m.SwIfIndex) {
		return nil, ErrEqualsBridgeDefault
	}
	return m, d.apply(ctx, m.SwIfIndex, featOf(n))
}

// Delete restores the member default (memberDefault) on the interface (no-op if it left the bridge).
func (d *FlagsDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	bd, err := d.bridgeOf(ctx, m.SwIfIndex)
	if err != nil {
		return nil //nolint:nilerr // not a bridge member any more: nothing to restore
	}
	return d.apply(ctx, m.SwIfIndex, memberDefault(bd, m.SwIfIndex))
}

// Retrieve implements scheduler.Descriptor.
func (d *FlagsDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
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
			key, ok := t.KeyFor(idx)
			if !ok {
				continue
			}
			cur, err := d.svc().L2InterfaceFeatFlagsGet(ctx, &l2api.L2InterfaceFeatFlagsGet{SwIfIndex: interface_types.InterfaceIndex(idx)})
			if err != nil {
				return nil, fmt.Errorf("l2_interface_feat_flags_get: %w", err)
			}
			if cur.Flags == memberDefault(bd, idx) {
				continue
			}
			out = append(out, scheduler.KV{Key: scheduler.Join(FlagsName, key.ID()), Value: decodeFeat(string(key), cur.Flags), Meta: iface.Meta{SwIfIndex: idx}})
		}
	}
	return out, nil
}
