package vxlan

import (
	"context"
	"fmt"
	"net/netip"

	"ngfw/agent/binapi/interface_types"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "vxlan.tunnel/vxlan_tunnel<instance>".
const TunnelName = "vxlan.tunnel"

// Plugin is the VPP plugin providing the messages.
const Plugin = "vxlan"

// DefaultPort is the VXLAN UDP port VPP uses when 0 is configured.
const DefaultPort = 4789

// TunnelDescriptor manages VXLAN tunnel interfaces.
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *vxlanapi.VxlanTunnelV2Details]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

// InterfaceName is the VPP interface name of a tunnel with the given instance.
func InterfaceName(instance uint32) string { return fmt.Sprintf("vxlan_tunnel%d", instance) }

func validate(t *Tunnel) (src, dst netip.Addr, err error) {
	if t.GetInstance() == df6.NoInterface {
		return src, dst, fmt.Errorf("%w: instance is mandatory", df6.ErrBadValue)
	}
	if t.GetVni() > 0xffffff {
		return src, dst, fmt.Errorf("%w: vni %d > 2^24-1", df6.ErrBadValue, t.GetVni())
	}
	if t.GetSrcPort() > 0xffff || t.GetDstPort() > 0xffff {
		return src, dst, fmt.Errorf("%w: port > 65535", df6.ErrBadValue)
	}
	if src, err = df6.ParseAddr(t.GetSrc()); err != nil {
		return src, dst, err
	}
	if dst, err = df6.ParseAddr(t.GetDst()); err != nil {
		return src, dst, err
	}
	if src.Is4() != dst.Is4() {
		return src, dst, fmt.Errorf("%w: src and dst must be the same address family", df6.ErrBadValue)
	}
	if dst.IsMulticast() != (t.GetMcastInterface() != "") {
		return src, dst, fmt.Errorf("%w: mcast_interface is mandatory iff dst is multicast", df6.ErrBadValue)
	}
	return src, dst, nil
}

func encode(t *Tunnel, ifs *df6.Interfaces, isAdd bool) (*vxlanapi.VxlanAddDelTunnelV3, error) {
	src, dst, err := validate(t)
	if err != nil {
		return nil, err
	}
	mcast, err := ifs.OptionalIndex(t.GetMcastInterface())
	if err != nil {
		return nil, err
	}
	return &vxlanapi.VxlanAddDelTunnelV3{
		IsAdd:          isAdd,
		Instance:       t.GetInstance(),
		SrcAddress:     df6.ToAddress(src),
		DstAddress:     df6.ToAddress(dst),
		SrcPort:        uint16(t.GetSrcPort()), //nolint:gosec // validated
		DstPort:        uint16(t.GetDstPort()), //nolint:gosec // validated
		McastSwIfIndex: mcast,
		EncapVrfID:     t.GetEncapVrfId(),
		DecapNextIndex: df6.NoInterface,
		Vni:            t.GetVni(),
		IsL3:           t.GetIsL3(),
	}, nil
}

var tunnelSpec = df6.IfSpec[*Tunnel, *vxlanapi.VxlanTunnelV2Details]{
	Name:   TunnelName,
	Plugin: Plugin,
	ID: func(t *Tunnel) (string, error) {
		if _, _, err := validate(t); err != nil {
			return "", err
		}
		return InterfaceName(t.GetInstance()), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency {
		return append(df6.VRFDeps(t.GetEncapVrfId()), df6.InterfaceDeps(t.GetMcastInterface())...)
	},
	Add: func(ctx context.Context, c vpp.Client, ifs *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		req, err := encode(t, ifs, true)
		if err != nil {
			return 0, err
		}
		rep, err := vxlanapi.NewServiceClient(c).VxlanAddDelTunnelV3(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("vxlan_add_del_tunnel_v3: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, t *Tunnel, _ interface_types.InterfaceIndex) error {
		ifs, err := df6.DumpInterfaces(ctx, c, "")
		if err != nil {
			return err
		}
		req, err := encode(t, ifs, false)
		if err != nil {
			return err
		}
		if _, err := vxlanapi.NewServiceClient(c).VxlanAddDelTunnelV3(ctx, req); err != nil {
			return fmt.Errorf("vxlan_add_del_tunnel_v3 (del): %w", err)
		}
		return nil
	},
	Dump: func(ctx context.Context, c vpp.Client) ([]*vxlanapi.VxlanTunnelV2Details, error) {
		stream, err := vxlanapi.NewServiceClient(c).VxlanTunnelV2Dump(ctx, &vxlanapi.VxlanTunnelV2Dump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
		if err != nil {
			return nil, fmt.Errorf("vxlan_tunnel_v2_dump: %w", err)
		}
		return df6.Collect(stream.Recv)
	},
	Decode: func(d *vxlanapi.VxlanTunnelV2Details, ifs *df6.Interfaces) (*Tunnel, uint32, bool) {
		t := &Tunnel{
			Instance:       d.Instance,
			Src:            df6.AddressString(d.SrcAddress),
			Dst:            df6.AddressString(d.DstAddress),
			McastInterface: ifs.NameOrEmpty(uint32(d.McastSwIfIndex)),
			Vni:            d.Vni,
			EncapVrfId:     d.EncapVrfID,
			SrcPort:        canonicalPort(d.SrcPort),
			DstPort:        canonicalPort(d.DstPort),
		}
		// The v2 dump does not report is_l3; an L2 tunnel is an ethernet interface with a MAC,
		// an L3 tunnel has none.
		if det, ok := ifs.Details(uint32(d.SwIfIndex)); ok {
			t.IsL3 = det.L2Address == [6]byte{}
		}
		return t, uint32(d.SwIfIndex), true
	},
}

func canonicalPort(p uint16) uint32 {
	if p == DefaultPort {
		return 0
	}
	return uint32(p)
}
