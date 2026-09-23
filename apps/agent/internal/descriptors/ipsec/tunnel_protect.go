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
// place; a different interface or next hop is a recreate.
type TunnelProtect struct{ cfg Config }

// TunnelProtectMeta is the runtime handle of a protection.
type TunnelProtectMeta struct{ SwIfIndex uint32 }

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
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.Index(o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.update(ctx, idx, o); err != nil {
		return nil, err
	}
	return TunnelProtectMeta{SwIfIndex: uint32(idx)}, nil
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

// Delete implements scheduler.Descriptor.
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
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecTunnelProtectDel(ctx, &ipsec.IpsecTunnelProtectDel{
		SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), Nh: nh,
	}); err != nil {
		return fmt.Errorf("ipsec_tunnel_protect_del (%s): %w", o.GetInterface(), err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: protections on interfaces tagged by this owner.
func (d *TunnelProtect) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecTunnelProtectDump(ctx, &ipsec.IpsecTunnelProtectDump{
		SwIfIndex: interface_types.InterfaceIndex(noInterface),
	})
	if err != nil {
		return nil, fmt.Errorf("ipsec_tunnel_protect_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_tunnel_protect_dump: %w", err)
		}
		idx := uint32(det.Tun.SwIfIndex)
		if _, owned := tbl.Owned(idx, d.cfg.Owner); !owned {
			continue
		}
		v := &vpnpb.IpsecTunnelProtect{Interface: tbl.Name(idx), SaOut: det.Tun.SaOut, SaIn: slices.Clone(det.Tun.SaIn)}
		if !vpn.IsUnspecified(det.Tun.Nh) {
			v.Nh = vpn.AddressString(det.Tun.Nh)
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: TunnelProtectMeta{SwIfIndex: idx}})
	}
	sortKVs(out)
	return out, nil
}

// nextHop encodes the p2mp next hop; "" (p2p) is the unspecified IPv4 address.
func nextHop(s string) (ip_types.Address, error) {
	if s == "" {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4}, nil
	}
	return vpn.ParseAddress(s)
}
