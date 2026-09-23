package gtpu

import (
	"context"
	"fmt"
	"net/netip"

	"google.golang.org/protobuf/proto"
	gtpuapi "ngfw/agent/binapi/gtpu"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelName is the descriptor name; keys are "gtpu.tunnel/<name>".
const TunnelName = "gtpu.tunnel"

// Plugin is the VPP plugin providing the messages.
const Plugin = "gtpu"

// TunnelDescriptor manages GTP-U tunnel interfaces.
type TunnelDescriptor = df6.IfDescriptor[*Tunnel, *gtpuapi.GtpuTunnelV2Details]

// NewTunnel returns the descriptor for the given owner.
func NewTunnel(c vpp.Client, owner string) *TunnelDescriptor {
	return df6.NewIfDescriptor(tunnelSpec, c, owner)
}

func validate(t *Tunnel) (src, dst netip.Addr, err error) {
	if t.GetName() == "" {
		return src, dst, fmt.Errorf("%w: name is mandatory", df6.ErrBadValue)
	}
	if t.GetQfi() > 63 {
		return src, dst, fmt.Errorf("%w: qfi %d > 63", df6.ErrBadValue, t.GetQfi())
	}
	if !t.GetPduExtension() && t.GetQfi() != 0 {
		return src, dst, fmt.Errorf("%w: qfi needs pdu_extension", df6.ErrBadValue)
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

func encode(t *Tunnel, ifs *df6.Interfaces, isAdd bool) (*gtpuapi.GtpuAddDelTunnelV2, error) {
	src, dst, err := validate(t)
	if err != nil {
		return nil, err
	}
	mcast, err := ifs.OptionalIndex(t.GetMcastInterface())
	if err != nil {
		return nil, err
	}
	return &gtpuapi.GtpuAddDelTunnelV2{
		IsAdd:          isAdd,
		SrcAddress:     df6.ToAddress(src),
		DstAddress:     df6.ToAddress(dst),
		McastSwIfIndex: mcast,
		EncapVrfID:     t.GetEncapVrfId(),
		DecapNextIndex: gtpuapi.GtpuDecapNextType(t.GetDecapNext()),
		Teid:           t.GetTeid(),
		Tteid:          effectiveTteid(t),
		PduExtension:   t.GetPduExtension(),
		Qfi:            uint8(t.GetQfi()), //nolint:gosec // validated ≤ 63
	}, nil
}

var tunnelSpec = df6.IfSpec[*Tunnel, *gtpuapi.GtpuTunnelV2Details]{
	Name:   TunnelName,
	Plugin: Plugin,
	ID: func(t *Tunnel) (string, error) {
		if _, _, err := validate(t); err != nil {
			return "", err
		}
		return t.GetName(), nil
	},
	Deps: func(t *Tunnel) []scheduler.Dependency {
		return append(df6.VRFDeps(t.GetEncapVrfId()), df6.InterfaceDeps(t.GetMcastInterface())...)
	},
	Add: func(ctx context.Context, c vpp.Client, ifs *df6.Interfaces, t *Tunnel) (interface_types.InterfaceIndex, error) {
		req, err := encode(t, ifs, true)
		if err != nil {
			return 0, err
		}
		rep, err := gtpuapi.NewServiceClient(c).GtpuAddDelTunnelV2(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("gtpu_add_del_tunnel_v2: %w", err)
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
		if _, err := gtpuapi.NewServiceClient(c).GtpuAddDelTunnelV2(ctx, req); err != nil {
			return fmt.Errorf("gtpu_add_del_tunnel_v2 (del): %w", err)
		}
		return nil
	},
	Dump: dump,
	Decode: func(d *gtpuapi.GtpuTunnelV2Details, ifs *df6.Interfaces) (*Tunnel, uint32, bool) {
		if d.IsForwarding {
			return nil, 0, false // gtpu.forward's records
		}
		id, _ := ifs.OwnedID(uint32(d.SwIfIndex))
		return &Tunnel{
			Name:           id,
			Src:            df6.AddressString(d.SrcAddress),
			Dst:            df6.AddressString(d.DstAddress),
			McastInterface: ifs.NameOrEmpty(uint32(d.McastSwIfIndex)),
			EncapVrfId:     d.EncapVrfID,
			DecapNext:      DecapNext(d.DecapNextIndex),
			Teid:           d.Teid,
			Tteid:          d.Tteid,
			PduExtension:   d.PduExtension,
			Qfi:            uint32(d.Qfi),
		}
		if t.Tteid == t.Teid {
			t.Tteid = 0 // VPP uses teid when no tteid is given; 0 is the canonical form
		}
		return t, uint32(d.SwIfIndex), true
	},
	// Only tteid changes in place (gtpu_tunnel_update_tteid).
	Update: func(ctx context.Context, c vpp.Client, o, n *Tunnel, _ interface_types.InterfaceIndex) (bool, error) {
		probe := proto.Clone(n).(*Tunnel)
		probe.Tteid = o.GetTteid()
		if !proto.Equal(probe, o) {
			return false, nil
		}
		dst, err := df6.AddressOf(n.GetDst())
		if err != nil {
			return false, err
		}
		if _, err := gtpuapi.NewServiceClient(c).GtpuTunnelUpdateTteid(ctx, &gtpuapi.GtpuTunnelUpdateTteid{DstAddress: dst, EncapVrfID: n.GetEncapVrfId(), Teid: n.GetTeid(), Tteid: effectiveTteid(n)}); err != nil {
			return false, fmt.Errorf("gtpu_tunnel_update_tteid: %w", err)
		}
		return true, nil
	},
}

// effectiveTteid is the transmit TEID VPP ends up with: tteid, or teid when tteid is 0.
func effectiveTteid(t *Tunnel) uint32 {
	if t.GetTteid() == 0 {
		return t.GetTeid()
	}
	return t.GetTteid()
}

func dump(ctx context.Context, c vpp.Client) ([]*gtpuapi.GtpuTunnelV2Details, error) {
	stream, err := gtpuapi.NewServiceClient(c).GtpuTunnelV2Dump(ctx, &gtpuapi.GtpuTunnelV2Dump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
	if err != nil {
		return nil, fmt.Errorf("gtpu_tunnel_v2_dump: %w", err)
	}
	return df6.Collect(stream.Recv)
}
