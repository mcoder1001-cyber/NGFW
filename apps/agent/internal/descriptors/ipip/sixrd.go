package ipip

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	ipipapi "ngfw/agent/binapi/ipip"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SixrdName is the descriptor name; keys are "ipip.sixrd/<name>".
const SixrdName = "ipip.sixrd"

// SixrdDescriptor manages 6rd tunnels (ipip_6rd_add_tunnel / ipip_6rd_del_tunnel). VPP has no
// dump exposing the 6rd parameters (ipip_tunnel_dump lists the interface but not the
// prefixes), so Retrieve returns df6.ErrRetrieveUnsupported: the descriptor is write-only.
type SixrdDescriptor = df6.IfDescriptor[*Tunnel6Rd, *ipipapi.IpipTunnelDetails]

// NewSixrd returns the descriptor for the given owner.
func NewSixrd(c vpp.Client, owner string) *SixrdDescriptor {
	return df6.NewIfDescriptor(sixrdSpec, c, owner)
}

var sixrdSpec = df6.IfSpec[*Tunnel6Rd, *ipipapi.IpipTunnelDetails]{
	Name:   SixrdName,
	Plugin: Plugin,
	ID: func(t *Tunnel6Rd) (string, error) {
		if t.GetName() == "" {
			return "", fmt.Errorf("%w: name is mandatory", df6.ErrBadValue)
		}
		if t.GetIp6Prefix() == "" || t.GetIp4Prefix() == "" || t.GetIp4Src() == "" {
			return "", fmt.Errorf("%w: ip6_prefix, ip4_prefix and ip4_src are mandatory", df6.ErrBadValue)
		}
		if t.GetTcTos() > 255 {
			return "", fmt.Errorf("%w: tc_tos %d > 255", df6.ErrBadValue, t.GetTcTos())
		}
		return t.GetName(), nil
	},
	Deps: func(t *Tunnel6Rd) []scheduler.Dependency {
		deps := df6.VRFDeps(t.GetIp6TableId())
		if t.GetIp4TableId() != t.GetIp6TableId() {
			deps = append(deps, df6.VRFDeps(t.GetIp4TableId())...)
		}
		return deps
	},
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, t *Tunnel6Rd) (interface_types.InterfaceIndex, error) {
		p6, err := df6.IP6PrefixOf(t.GetIp6Prefix())
		if err != nil {
			return 0, err
		}
		p4, err := df6.IP4PrefixOf(t.GetIp4Prefix())
		if err != nil {
			return 0, err
		}
		src, err := df6.IP4Of(t.GetIp4Src())
		if err != nil {
			return 0, err
		}
		rep, err := ipipapi.NewServiceClient(c).Ipip6rdAddTunnel(ctx, &ipipapi.Ipip6rdAddTunnel{
			IP6TableID:    t.GetIp6TableId(),
			IP4TableID:    t.GetIp4TableId(),
			IP6Prefix:     p6,
			IP4Prefix:     p4,
			IP4Src:        src,
			SecurityCheck: t.GetSecurityCheck(),
			TcTos:         uint8(t.GetTcTos()), //nolint:gosec // validated ≤ 255
		})
		if err != nil {
			return 0, fmt.Errorf("ipip_6rd_add_tunnel: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, _ *Tunnel6Rd, idx interface_types.InterfaceIndex) error {
		if _, err := ipipapi.NewServiceClient(c).Ipip6rdDelTunnel(ctx, &ipipapi.Ipip6rdDelTunnel{SwIfIndex: idx}); err != nil {
			return fmt.Errorf("ipip_6rd_del_tunnel: %w", err)
		}
		return nil
	},
	Dump: func(context.Context, vpp.Client) ([]*ipipapi.IpipTunnelDetails, error) {
		return nil, df6.ErrRetrieveUnsupported
	},
	Decode: func(*ipipapi.IpipTunnelDetails, *df6.Interfaces) (*Tunnel6Rd, uint32, bool) { return nil, 0, false },
}
