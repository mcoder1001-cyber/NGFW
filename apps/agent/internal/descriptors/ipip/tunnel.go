package ipip

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	ipipapi "ngfw/agent/binapi/ipip"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "ipip.tunnel/ipip<instance>".
const TunnelName = "ipip.tunnel"

// Plugin is the VPP component providing the messages (ipip is built into vnet).
const Plugin = "ipip"

// TunnelDescriptor manages IPIP tunnel interfaces (ipip_add_tunnel / ipip_del_tunnel /
// ipip_tunnel_dump).
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *ipipapi.IpipTunnelDetails]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

// InterfaceName is the VPP interface name of a tunnel with the given instance.
func InterfaceName(instance uint32) string { return fmt.Sprintf("ipip%d", instance) }

var tunnelSpec = df6.IfSpec[*Tunnel, *ipipapi.IpipTunnelDetails]{
	Name:   TunnelName,
	Plugin: Plugin,
	ID: func(t *Tunnel) (string, error) {
		if t.GetInstance() == df6.NoInterface {
			return "", fmt.Errorf("%w: instance is mandatory", df6.ErrBadValue)
		}
		if t.GetSrc() == "" {
			return "", fmt.Errorf("%w: src is mandatory", df6.ErrBadValue)
		}
		if t.GetMode() == TunnelMode_P2P && t.GetDst() == "" {
			return "", fmt.Errorf("%w: dst is mandatory for p2p tunnels", df6.ErrBadValue)
		}
		if t.GetMode() == TunnelMode_MP && t.GetDst() != "" {
			return "", fmt.Errorf("%w: dst must be empty for mp tunnels", df6.ErrBadValue)
		}
		if t.GetDscp() > 63 {
			return "", fmt.Errorf("%w: dscp %d > 63", df6.ErrBadValue, t.GetDscp())
		}
		return InterfaceName(t.GetInstance()), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency { return df6.VRFDeps(t.GetTableId()) },
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		src, err := df6.AddressOf(t.GetSrc())
		if err != nil {
			return 0, err
		}
		dst, err := df6.AddressOf(t.GetDst())
		if err != nil {
			return 0, err
		}
		if t.GetMode() == TunnelMode_MP {
			dst = ip_types.Address{Af: src.Af}
		}
		rep, err := ipipapi.NewServiceClient(c).IpipAddTunnel(ctx, &ipipapi.IpipAddTunnel{Tunnel: ipipapi.IpipTunnel{
			Instance:  t.GetInstance(),
			Src:       src,
			Dst:       dst,
			SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface),
			TableID:   t.GetTableId(),
			Flags:     tunnel_types.TunnelEncapDecapFlags(t.GetFlags()), //nolint:gosec // 8-bit mask, validated by VPP
			Mode:      tunnel_types.TunnelMode(t.GetMode()),
			Dscp:      ip_types.IPDscp(t.GetDscp()), //nolint:gosec // validated ≤ 63
		}})
		if err != nil {
			return 0, fmt.Errorf("ipip_add_tunnel: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, _ *Tunnel, idx interface_types.InterfaceIndex) error {
		if _, err := ipipapi.NewServiceClient(c).IpipDelTunnel(ctx, &ipipapi.IpipDelTunnel{SwIfIndex: idx}); err != nil {
			return fmt.Errorf("ipip_del_tunnel: %w", err)
		}
		return nil
	},
	Dump: dump,
	Decode: func(d *ipipapi.IpipTunnelDetails, _ *df6.Interfaces) (*Tunnel, uint32, bool) {
		t := &Tunnel{
			Instance: d.Tunnel.Instance,
			Src:      df6.AddressString(d.Tunnel.Src),
			Dst:      df6.AddressString(d.Tunnel.Dst),
			TableId:  d.Tunnel.TableID,
			Flags:    uint32(d.Tunnel.Flags),
			Mode:     TunnelMode(d.Tunnel.Mode),
			Dscp:     uint32(d.Tunnel.Dscp),
		}
		if t.Mode == TunnelMode_MP {
			t.Dst = ""
		}
		return t, uint32(d.Tunnel.SwIfIndex), true
	},
}

func dump(ctx context.Context, c vpp.Client) ([]*ipipapi.IpipTunnelDetails, error) {
	stream, err := ipipapi.NewServiceClient(c).IpipTunnelDump(ctx, &ipipapi.IpipTunnelDump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
	if err != nil {
		return nil, fmt.Errorf("ipip_tunnel_dump: %w", err)
	}
	return df6.Collect(stream.Recv)
}
