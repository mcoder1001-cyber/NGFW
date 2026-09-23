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
// bridge domain's own flags: such an object would vanish from Retrieve, so it must be removed
// from the desired state instead.
var ErrEqualsBridgeDefault = errors.New("l2.flags: flags equal the bridge-domain defaults; remove the object instead")

// FlagsDescriptor implements l2.flags with l2_interface_feat_flags_set / _get. A bridge member
// inherits the bridge domain's learn/forward/flood/uu-flood/arp-term/arp-ufwd features; this
// object overrides them per interface and exists only while they differ from the bridge's.
type FlagsDescriptor struct{ base }

// NewFlags returns the descriptor for owner.
func NewFlags(c vpp.Client, owner string) *FlagsDescriptor { return &FlagsDescriptor{base{c, owner}} }

func (*FlagsDescriptor) Name() string { return FlagsName }

func (*FlagsDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(FlagsName, iface.RefID(obj.(*Flags).GetInterface()))
}

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

// bdFeat is the bridge domain's flags expressed as interface feature flags.
func bdFeat(bd *l2api.BridgeDomainDetails) l2api.L2IntfFeatFlags {
	return featOf(&Flags{Learn: bd.Learn, Forward: bd.Forward, Flood: bd.Flood, UuFlood: bd.UuFlood, ArpTerm: bd.ArpTerm, ArpUfwd: bd.ArpUfwd})
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
	if clear := cur.Flags &^ want; clear != 0 {
		if _, err := d.svc().L2InterfaceFeatFlagsSet(ctx, &l2api.L2InterfaceFeatFlagsSet{SwIfIndex: interface_types.InterfaceIndex(idx), IsSet: false, Flags: clear}); err != nil {
			return fmt.Errorf("l2_interface_feat_flags_set: %w", err)
		}
	}
	return nil
}

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
	if featOf(o) == bdFeat(bd) {
		return nil, ErrEqualsBridgeDefault
	}
	return iface.Meta{SwIfIndex: idx}, d.apply(ctx, idx, featOf(o))
}

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
	if featOf(n) == bdFeat(bd) {
		return nil, ErrEqualsBridgeDefault
	}
	return m, d.apply(ctx, m.SwIfIndex, featOf(n))
}

// Delete restores the bridge domain's flags on the interface (no-op if it left the bridge).
func (d *FlagsDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	bd, err := d.bridgeOf(ctx, m.SwIfIndex)
	if err != nil {
		return nil //nolint:nilerr // not a bridge member any more: nothing to restore
	}
	return d.apply(ctx, m.SwIfIndex, bdFeat(bd))
}

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
			if cur.Flags == bdFeat(bd) {
				continue
			}
			out = append(out, scheduler.KV{Key: scheduler.Join(FlagsName, key.ID()), Value: decodeFeat(string(key), cur.Flags), Meta: iface.Meta{SwIfIndex: idx}})
		}
	}
	return out, nil
}
