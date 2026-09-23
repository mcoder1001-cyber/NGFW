package l2tp

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2tpapi "ngfw/agent/binapi/l2tp"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "l2tp.tunnel/<name>".
const TunnelName = "l2tp.tunnel"

// Plugin is the VPP plugin providing the messages.
const Plugin = "l2tp"

// TunnelDescriptor manages L2TPv3 tunnel interfaces. VPP offers no delete: Delete returns
// df6.ErrNoDelete.
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *l2tpapi.SwIfL2tpv3TunnelDetails]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

var tunnelSpec = df6.IfSpec[*Tunnel, *l2tpapi.SwIfL2tpv3TunnelDetails]{
	Name:   TunnelName,
	Plugin: Plugin,
	ID: func(t *Tunnel) (string, error) {
		if t.GetName() == "" {
			return "", fmt.Errorf("%w: name is mandatory", df6.ErrBadValue)
		}
		if _, err := df6.ParseAddr6(t.GetClientAddress()); err != nil {
			return "", err
		}
		if _, err := df6.ParseAddr6(t.GetOurAddress()); err != nil {
			return "", err
		}
		if t.GetEncapVrfId() != 0 {
			return "", fmt.Errorf("%w: encap_vrf_id is not reported by sw_if_l2tpv3_tunnel_dump; only 0 is supported", df6.ErrBadValue)
		}
		return t.GetName(), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency { return df6.VRFDeps(t.GetEncapVrfId()) },
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		client, err := df6.AddressOf(t.GetClientAddress())
		if err != nil {
			return 0, err
		}
		our, err := df6.AddressOf(t.GetOurAddress())
		if err != nil {
			return 0, err
		}
		rep, err := l2tpapi.NewServiceClient(c).L2tpv3CreateTunnel(ctx, &l2tpapi.L2tpv3CreateTunnel{
			ClientAddress:     client,
			OurAddress:        our,
			LocalSessionID:    t.GetLocalSessionId(),
			RemoteSessionID:   t.GetRemoteSessionId(),
			LocalCookie:       t.GetLocalCookie(),
			RemoteCookie:      t.GetRemoteCookie(),
			L2SublayerPresent: t.GetL2SublayerPresent(),
			EncapVrfID:        t.GetEncapVrfId(),
		})
		if err != nil {
			return 0, fmt.Errorf("l2tpv3_create_tunnel: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: nil, // no l2tpv3 delete message in VPP 26.06 → df6.ErrNoDelete
	Dump: func(ctx context.Context, c vpp.Client) ([]*l2tpapi.SwIfL2tpv3TunnelDetails, error) {
		stream, err := l2tpapi.NewServiceClient(c).SwIfL2tpv3TunnelDump(ctx, &l2tpapi.SwIfL2tpv3TunnelDump{})
		if err != nil {
			return nil, fmt.Errorf("sw_if_l2tpv3_tunnel_dump: %w", err)
		}
		return df6.Collect(stream.Recv)
	},
	Decode: func(d *l2tpapi.SwIfL2tpv3TunnelDetails, ifs *df6.Interfaces) (*Tunnel, uint32, bool) {
		id, _ := ifs.OwnedID(uint32(d.SwIfIndex))
		t := &Tunnel{
			Name:              id,
			ClientAddress:     df6.AddressString(d.ClientAddress),
			OurAddress:        df6.AddressString(d.OurAddress),
			LocalSessionId:    d.LocalSessionID,
			RemoteSessionId:   d.RemoteSessionID,
			RemoteCookie:      d.RemoteCookie,
			L2SublayerPresent: d.L2SublayerPresent,
		}
		if len(d.LocalCookie) > 0 {
			t.LocalCookie = d.LocalCookie[0] // [0] = current, [1] = previous
		}
		return t, uint32(d.SwIfIndex), true
	},
	// Cookies change in place (l2tpv3_set_tunnel_cookies); anything else would need a
	// recreate, which VPP cannot do (no delete) — the scheduler surfaces ErrNoDelete then.
	Update: func(ctx context.Context, c vpp.Client, o, n *Tunnel, idx interface_types.InterfaceIndex) (bool, error) {
		probe := proto.Clone(n).(*Tunnel)
		probe.LocalCookie, probe.RemoteCookie = o.GetLocalCookie(), o.GetRemoteCookie()
		if !proto.Equal(probe, o) {
			return false, nil
		}
		if _, err := l2tpapi.NewServiceClient(c).L2tpv3SetTunnelCookies(ctx, &l2tpapi.L2tpv3SetTunnelCookies{SwIfIndex: idx, NewLocalCookie: n.GetLocalCookie(), NewRemoteCookie: n.GetRemoteCookie()}); err != nil {
			return false, fmt.Errorf("l2tpv3_set_tunnel_cookies: %w", err)
		}
		return true, nil
	},
}
