package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// TunnelProtect protects a tunnel interface (ipip from DF-6, or ipsec.itf) with SAs
// (ipsec_tunnel_protect_update / _del; dump ipsec_tunnel_protect_dump). The SAs can be swapped in
// place; a different interface or next hop is a recreate. The tunnel interface is named by its
// logical name (D-069) and must be one of ours (tagged): a protection belongs to the owner of its
// tunnel interface, and tunnel interfaces are always created by some owner.
type TunnelProtect struct{ cfg Config }

// TunnelProtectMeta is the runtime handle of a protection: the tunnel's index and logical name
// (re-verified before a delete).
type TunnelProtectMeta struct {
	SwIfIndex uint32
	Interface string
}

// NewTunnelProtect returns the descriptor.
func NewTunnelProtect(cfg Config) *TunnelProtect { return &TunnelProtect{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*TunnelProtect) Name() string { return TunnelProtectName }

// KeyOf implements scheduler.Descriptor: ipsec.tunnel-protect/<interface>[/<nh>].
func (*TunnelProtect) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecTunnelProtect)
	if o.GetNh() == "" {
		return scheduler.Join(TunnelProtectName, o.GetInterface())
	}
	return scheduler.Join(TunnelProtectName, o.GetInterface(), canon(o.GetNh()))
}

// Dependencies implements scheduler.Descriptor: the tunnel interface, sa_out and every sa_in.
func (*TunnelProtect) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.IpsecTunnelProtect)
	deps := []scheduler.Dependency{
		{Key: vpn.InterfaceKey(o.GetInterface())},
		{Key: scheduler.Join(SaName, vpn.Uint(o.GetSaOut()))},
	}
	for _, in := range o.GetSaIn() {
		deps = append(deps, scheduler.Dependency{Key: scheduler.Join(SaName, vpn.Uint(in))})
	}
	return deps
}

// Create implements scheduler.Descriptor.
func (d *TunnelProtect) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecTunnelProtect)
	if !ok {
		return nil, typeErr(TunnelProtectName, obj)
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.ResolveOwn(o.GetInterface())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", TunnelProtectName, err)
	}
	if err := d.update(ctx, idx, o); err != nil {
		return nil, err
	}
	return TunnelProtectMeta{SwIfIndex: uint32(idx), Interface: o.GetInterface()}, nil
}

// Update implements scheduler.Descriptor: same interface and next hop → swap SAs in place.
func (d *TunnelProtect) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, ok := oldObj.(*vpnpb.IpsecTunnelProtect)
	n, ok2 := newObj.(*vpnpb.IpsecTunnelProtect)
	if !ok || !ok2 {
		return nil, typeErr(TunnelProtectName, newObj)
	}
	m, ok := meta.(TunnelProtectMeta)
	if !ok {
		return nil, metaErr(TunnelProtectName, meta)
	}
	if o.GetInterface() != n.GetInterface() || canon(o.GetNh()) != canon(n.GetNh()) {
		return nil, scheduler.ErrRecreate
	}
	if ok, err := vpn.OwnedAt(ctx, d.cfg.Client, d.cfg.Owner, m.SwIfIndex, n.GetInterface()); err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("%s: %w: %s", TunnelProtectName, vpn.ErrNoInterface, n.GetInterface())
		}
		return nil, err
	}
	if err := d.update(ctx, interface_types.InterfaceIndex(m.SwIfIndex), n); err != nil {
		return nil, err
	}
	return m, nil
}

func (d *TunnelProtect) update(ctx context.Context, idx interface_types.InterfaceIndex, o *vpnpb.IpsecTunnelProtect) error {
	if len(o.GetSaIn()) == 0 || len(o.GetSaIn()) > 255 {
		return fmt.Errorf("ipsec: tunnel-protect %s needs 1–255 sa_in", o.GetInterface())
	}
	nh, err := nextHop(o.GetNh())
	if err != nil {
		return err
	}
	tp := ipsec.IpsecTunnelProtect{
		SwIfIndex: idx, Nh: nh, SaOut: o.GetSaOut(),
		NSaIn: uint8(len(o.GetSaIn())), SaIn: slices.Clone(o.GetSaIn()), //nolint:gosec // checked
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecTunnelProtectUpdate(ctx, &ipsec.IpsecTunnelProtectUpdate{Tunnel: tp}); err != nil {
		return fmt.Errorf("ipsec_tunnel_protect_update (%s): %w", o.GetInterface(), err)
	}
	return nil
}

// Delete implements scheduler.Descriptor. Right before the delete it re-verifies that the index
// still is our tunnel interface and that the protection still exists (D-071, D-074).
func (d *TunnelProtect) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.IpsecTunnelProtect)
	if !ok {
		return typeErr(TunnelProtectName, obj)
	}
	m, ok := meta.(TunnelProtectMeta)
	if !ok {
		return metaErr(TunnelProtectName, meta)
	}
	nh, err := nextHop(o.GetNh())
	if err != nil {
		return err
	}
	present, err := vpn.OwnedAt(ctx, d.cfg.Client, d.cfg.Owner, m.SwIfIndex, o.GetInterface())
	if err != nil || !present {
		return err // gone with its interface, or the index is no longer ours
	}
	cur, err := d.dump(ctx, m.SwIfIndex)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(cur, func(tp ipsec.IpsecTunnelProtect) bool { return canonNh(tp.Nh) == canon(o.GetNh()) }) {
		return nil
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecTunnelProtectDel(ctx, &ipsec.IpsecTunnelProtectDel{
		SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), Nh: nh,
	}); err != nil {
		return fmt.Errorf("ipsec_tunnel_protect_del (%s): %w", o.GetInterface(), err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: protections on interfaces tagged by this owner.
func (d *TunnelProtect) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	all, err := d.dump(ctx, noInterface)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, tp := range all {
		idx := uint32(tp.SwIfIndex)
		name, owned := tbl.Owned(idx)
		if !owned {
			continue
		}
		v := &vpnpb.IpsecTunnelProtect{Interface: name, SaOut: tp.SaOut, SaIn: slices.Clone(tp.SaIn), Nh: canonNh(tp.Nh)}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: TunnelProtectMeta{SwIfIndex: idx, Interface: name}})
	}
	return sortKVs(out), nil
}

// dump runs ipsec_tunnel_protect_dump for sw_if_index (~0 = all).
func (d *TunnelProtect) dump(ctx context.Context, swIfIndex uint32) ([]ipsec.IpsecTunnelProtect, error) {
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecTunnelProtectDump(ctx, &ipsec.IpsecTunnelProtectDump{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	})
	if err != nil {
		return nil, fmt.Errorf("ipsec_tunnel_protect_dump: %w", err)
	}
	var out []ipsec.IpsecTunnelProtect
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_tunnel_protect_dump: %w", err)
		}
		if swIfIndex == noInterface || uint32(det.Tun.SwIfIndex) == swIfIndex {
			out = append(out, det.Tun)
		}
	}
}

// canonNh is the desired-state text of a dumped next hop ("" for p2p / unspecified).
func canonNh(a ip_types.Address) string {
	if vpn.IsUnspecified(a) {
		return ""
	}
	return vpn.AddressString(a)
}

// nextHop encodes the p2mp next hop; "" (p2p) is the unspecified IPv4 address.
func nextHop(s string) (ip_types.Address, error) {
	if s == "" {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4}, nil
	}
	return vpn.ParseAddress(s)
}
