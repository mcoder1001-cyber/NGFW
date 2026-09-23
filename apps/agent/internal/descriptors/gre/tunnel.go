package gre

import (
	"context"
	"fmt"

	greapi "ngfw/agent/binapi/gre"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "gre.tunnel/gre<instance>".
const TunnelName = "gre.tunnel"

// Plugin is the VPP plugin providing the messages.
const Plugin = "gre"

// TunnelDescriptor manages GRE tunnel interfaces (gre_tunnel_add_del_v2 / gre_tunnel_v2_dump).
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *greapi.GreTunnelV2Details]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

// InterfaceName is the VPP interface name of a tunnel with the given instance.
func InterfaceName(instance uint32) string { return fmt.Sprintf("gre%d", instance) }

var tunnelSpec = df6.IfSpec[*Tunnel, *greapi.GreTunnelV2Details]{
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
		if t.GetSessionId() > 1023 {
			return "", fmt.Errorf("%w: session_id %d > 1023", df6.ErrBadValue, t.GetSessionId())
		}
		if t.GetType() != TunnelType_ERSPAN && t.GetSessionId() != 0 {
			return "", fmt.Errorf("%w: session_id only for ERSPAN tunnels", df6.ErrBadValue)
		}
		return InterfaceName(t.GetInstance()), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency { return df6.VRFDeps(t.GetOuterTableId()) },
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		req, err := encode(t)
		if err != nil {
			return 0, err
		}
		rep, err := greapi.NewServiceClient(c).GreTunnelAddDelV2(ctx, &greapi.GreTunnelAddDelV2{IsAdd: true, Tunnel: req})
		if err != nil {
			return 0, fmt.Errorf("gre_tunnel_add_del_v2: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, t *Tunnel, idx interface_types.InterfaceIndex) error {
		req, err := encode(t)
		if err != nil {
			return err
		}
		req.SwIfIndex = idx
		if _, err := greapi.NewServiceClient(c).GreTunnelAddDelV2(ctx, &greapi.GreTunnelAddDelV2{IsAdd: false, Tunnel: req}); err != nil {
			return fmt.Errorf("gre_tunnel_add_del_v2 (del): %w", err)
		}
		return nil
	},
	Dump: func(ctx context.Context, c vpp.Client) ([]*greapi.GreTunnelV2Details, error) {
		stream, err := greapi.NewServiceClient(c).GreTunnelV2Dump(ctx, &greapi.GreTunnelV2Dump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
		if err != nil {
			return nil, fmt.Errorf("gre_tunnel_v2_dump: %w", err)
		}
		return df6.Collect(stream.Recv)
	},
	Decode: func(d *greapi.GreTunnelV2Details, _ *df6.Interfaces) (*Tunnel, uint32, bool) {
		t := &Tunnel{
			Instance:     d.Tunnel.Instance,
			Type:         TunnelType(d.Tunnel.Type),
			Mode:         TunnelMode(d.Tunnel.Mode),
			Src:          df6.AddressString(d.Tunnel.Src),
			Dst:          df6.AddressString(d.Tunnel.Dst),
			OuterTableId: d.Tunnel.OuterTableID,
			SessionId:    uint32(d.Tunnel.SessionID),
			Flags:        uint32(d.Tunnel.Flags),
			Key:          d.Tunnel.Key,
		}
		if t.Mode == TunnelMode_MP {
			t.Dst = ""
		}
		return t, uint32(d.Tunnel.SwIfIndex), true
	},
}

// encode converts the desired tunnel to the binapi struct (without sw_if_index).
func encode(t *Tunnel) (greapi.GreTunnelV2, error) {
	src, err := df6.AddressOf(t.GetSrc())
	if err != nil {
		return greapi.GreTunnelV2{}, err
	}
	dst, err := df6.AddressOf(t.GetDst())
	if err != nil {
		return greapi.GreTunnelV2{}, err
	}
	if t.GetMode() == TunnelMode_MP {
		// VPP wants an all-zero destination of the source's family for multipoint tunnels.
		dst = src
		dst.Un = src.Un
		for i := range dst.Un.XXX_UnionData {
			dst.Un.XXX_UnionData[i] = 0
		}
	}
	return greapi.GreTunnelV2{
		Type:         greapi.GreTunnelType(t.GetType()),
		Mode:         tunnel_types.TunnelMode(t.GetMode()),
		Flags:        tunnel_types.TunnelEncapDecapFlags(t.GetFlags()), //nolint:gosec // 8-bit mask, validated by VPP
		SessionID:    uint16(t.GetSessionId()),                         //nolint:gosec // validated ≤ 1023
		Instance:     t.GetInstance(),
		OuterTableID: t.GetOuterTableId(),
		SwIfIndex:    interface_types.InterfaceIndex(df6.NoInterface),
		Src:          src,
		Dst:          dst,
		Key:          t.GetKey(),
	}, nil
}
