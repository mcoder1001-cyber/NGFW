package vxlan_gpe

import (
	"context"
	"fmt"
	"net/netip"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	gpeapi "ngfw/agent/binapi/vxlan_gpe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "vxlan-gpe.tunnel/<name>".
const TunnelName = "vxlan-gpe.tunnel"

// Plugin is the VPP plugin providing the messages.
const Plugin = "vxlan-gpe"

// DefaultPort is the VXLAN-GPE UDP port VPP uses when 0 is configured.
const DefaultPort = 4790

// TunnelDescriptor manages VXLAN-GPE tunnel interfaces.
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *gpeapi.VxlanGpeTunnelV2Details]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

func validate(t *Tunnel) (local, remote netip.Addr, err error) {
	if t.GetName() == "" {
		return local, remote, fmt.Errorf("%w: name is mandatory", df6.ErrBadValue)
	}
	if t.GetVni() > 0xffffff {
		return local, remote, fmt.Errorf("%w: vni %d > 2^24-1", df6.ErrBadValue, t.GetVni())
	}
	if t.GetLocalPort() > 0xffff || t.GetRemotePort() > 0xffff {
		return local, remote, fmt.Errorf("%w: port > 65535", df6.ErrBadValue)
	}
	if t.GetProtocol() < Protocol_IP4 || t.GetProtocol() > Protocol_NSH {
		return local, remote, fmt.Errorf("%w: protocol must be IP4, IP6, ETHERNET or NSH", df6.ErrBadValue)
	}
	if local, err = df6.ParseAddr(t.GetLocal()); err != nil {
		return local, remote, err
	}
	if remote, err = df6.ParseAddr(t.GetRemote()); err != nil {
		return local, remote, err
	}
	if local.Is4() != remote.Is4() {
		return local, remote, fmt.Errorf("%w: local and remote must be the same address family", df6.ErrBadValue)
	}
	if remote.IsMulticast() != (t.GetMcastInterface() != "") {
		return local, remote, fmt.Errorf("%w: mcast_interface is mandatory iff remote is multicast", df6.ErrBadValue)
	}
	if !decapsIP(t.GetProtocol()) && t.GetDecapVrfId() != 0 {
		return local, remote, fmt.Errorf("%w: decap_vrf_id only applies to IP4/IP6 payloads", df6.ErrBadValue)
	}
	return local, remote, nil
}

// decapsIP reports whether the payload protocol is routed after decapsulation (VPP keeps —
// and reports — a decap FIB only for IP4/IP6 payloads).
func decapsIP(p Protocol) bool {
	return p == Protocol_IP4 || p == Protocol_IP6
}

func encode(t *Tunnel, ifs *df6.Interfaces, isAdd bool) (*gpeapi.VxlanGpeAddDelTunnelV2, error) {
	local, remote, err := validate(t)
	if err != nil {
		return nil, err
	}
	mcast, err := ifs.OptionalIndex(t.GetMcastInterface())
	if err != nil {
		return nil, err
	}
	return &gpeapi.VxlanGpeAddDelTunnelV2{
		Local:          df6.ToAddress(local),
		Remote:         df6.ToAddress(remote),
		LocalPort:      uint16(t.GetLocalPort()),  //nolint:gosec // validated
		RemotePort:     uint16(t.GetRemotePort()), //nolint:gosec // validated
		McastSwIfIndex: mcast,
		EncapVrfID:     t.GetEncapVrfId(),
		DecapVrfID:     t.GetDecapVrfId(),
		Protocol:       ip_types.IPProto(t.GetProtocol()), //nolint:gosec // 1..4
		Vni:            t.GetVni(),
		IsAdd:          isAdd,
	}, nil
}

var tunnelSpec = df6.IfSpec[*Tunnel, *gpeapi.VxlanGpeTunnelV2Details]{
	Name:   TunnelName,
	Plugin: Plugin,
	ID: func(t *Tunnel) (string, error) {
		if _, _, err := validate(t); err != nil {
			return "", err
		}
		return t.GetName(), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency {
		deps := df6.VRFDeps(t.GetEncapVrfId())
		if t.GetDecapVrfId() != t.GetEncapVrfId() {
			deps = append(deps, df6.VRFDeps(t.GetDecapVrfId())...)
		}
		return append(deps, df6.InterfaceDeps(t.GetMcastInterface())...)
	},
	Add: func(ctx context.Context, c vpp.Client, ifs *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		req, err := encode(t, ifs, true)
		if err != nil {
			return 0, err
		}
		rep, err := gpeapi.NewServiceClient(c).VxlanGpeAddDelTunnelV2(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("vxlan_gpe_add_del_tunnel_v2: %w", err)
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
		if _, err := gpeapi.NewServiceClient(c).VxlanGpeAddDelTunnelV2(ctx, req); err != nil {
			return fmt.Errorf("vxlan_gpe_add_del_tunnel_v2 (del): %w", err)
		}
		return nil
	},
	Dump: func(ctx context.Context, c vpp.Client) ([]*gpeapi.VxlanGpeTunnelV2Details, error) {
		stream, err := gpeapi.NewServiceClient(c).VxlanGpeTunnelV2Dump(ctx, &gpeapi.VxlanGpeTunnelV2Dump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
		if err != nil {
			return nil, fmt.Errorf("vxlan_gpe_tunnel_v2_dump: %w", err)
		}
		return df6.Collect(stream.Recv)
	},
	Decode: func(d *gpeapi.VxlanGpeTunnelV2Details, ifs *df6.Interfaces) (*Tunnel, uint32, bool) {
		id, _ := ifs.OwnedID(uint32(d.SwIfIndex))
		t := &Tunnel{
			Name:           id,
			Local:          df6.AddressString(d.Local),
			Remote:         df6.AddressString(d.Remote),
			LocalPort:      canonicalPort(d.LocalPort),
			RemotePort:     canonicalPort(d.RemotePort),
			McastInterface: ifs.NameOrEmpty(uint32(d.McastSwIfIndex)),
			Vni:            d.Vni,
			Protocol:       Protocol(d.Protocol),
			EncapVrfId:     d.EncapVrfID,
			DecapVrfId:     d.DecapVrfID,
		}
		if !decapsIP(t.Protocol) {
			t.DecapVrfId = 0
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
