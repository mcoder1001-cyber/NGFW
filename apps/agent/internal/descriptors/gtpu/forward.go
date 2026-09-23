package gtpu

import (
	"context"
	"fmt"

	gtpuapi "ngfw/agent/binapi/gtpu"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ForwardName is the descriptor name; keys are "gtpu.forward/<name>".
const ForwardName = "gtpu.forward"

// ForwardingTypeMask is every gtpu_forwarding_type bit.
const ForwardingTypeMask = uint32(gtpuapi.GTPU_API_FORWARDING_BAD_HEADER | gtpuapi.GTPU_API_FORWARDING_UNKNOWN_TEID | gtpuapi.GTPU_API_FORWARDING_UNKNOWN_TYPE)

// ForwardDescriptor manages GTP-U forwarding entries (gtpu_add_del_forward), which VPP
// represents as tunnel interfaces flagged is_forwarding in gtpu_tunnel_v2_dump.
type ForwardDescriptor = df6.IfDescriptor[*Forward, *gtpuapi.GtpuTunnelV2Details]

// NewForward returns the descriptor for the given owner.
func NewForward(c vpp.Client, owner string) *ForwardDescriptor {
	return df6.NewIfDescriptor(forwardSpec, c, owner)
}

func encodeForward(f *Forward, isAdd bool) (*gtpuapi.GtpuAddDelForward, error) {
	dst, err := df6.ParseAddr(f.GetDst())
	if err != nil {
		return nil, err
	}
	return &gtpuapi.GtpuAddDelForward{
		IsAdd:          isAdd,
		DstAddress:     df6.ToAddress(dst),
		ForwardingType: gtpuapi.GtpuForwardingType(f.GetForwardingType()), //nolint:gosec // validated mask
		EncapVrfID:     f.GetEncapVrfId(),
		DecapNextIndex: gtpuapi.GtpuDecapNextType(f.GetDecapNext()), //nolint:gosec // proto enum 0–3
	}, nil
}

var forwardSpec = df6.IfSpec[*Forward, *gtpuapi.GtpuTunnelV2Details]{
	Name:   ForwardName,
	Plugin: Plugin,
	ID: func(f *Forward) (string, error) {
		if f.GetName() == "" {
			return "", fmt.Errorf("%w: name is mandatory", df6.ErrBadValue)
		}
		if f.GetForwardingType() == 0 || f.GetForwardingType()&^ForwardingTypeMask != 0 {
			return "", fmt.Errorf("%w: forwarding_type must be a non-empty mask of 1|2|4", df6.ErrBadValue)
		}
		if _, err := df6.ParseAddr(f.GetDst()); err != nil {
			return "", err
		}
		return f.GetName(), nil
	},
	Deps: func(f *Forward) []scheduler.Dependency { return df6.VRFDeps(f.GetEncapVrfId()) },
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, f *Forward) (interface_types.InterfaceIndex, error) {
		req, err := encodeForward(f, true)
		if err != nil {
			return 0, err
		}
		rep, err := gtpuapi.NewServiceClient(c).GtpuAddDelForward(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("gtpu_add_del_forward: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, f *Forward, _ interface_types.InterfaceIndex) error {
		req, err := encodeForward(f, false)
		if err != nil {
			return err
		}
		if _, err := gtpuapi.NewServiceClient(c).GtpuAddDelForward(ctx, req); err != nil {
			return fmt.Errorf("gtpu_add_del_forward (del): %w", err)
		}
		return nil
	},
	Dump: dump,
	Decode: func(d *gtpuapi.GtpuTunnelV2Details, ifs *df6.Interfaces) (*Forward, uint32, bool) {
		if !d.IsForwarding {
			return nil, 0, false
		}
		id, _ := ifs.OwnedID(uint32(d.SwIfIndex))
		// VPP stores a forwarding entry as a tunnel whose *src* is the forwarding destination
		// (dst holds an internal placeholder, 127.0.0.128 for IPv4).
		return &Forward{
			Name:           id,
			Dst:            df6.AddressString(d.SrcAddress),
			ForwardingType: uint32(d.ForwardingType),
			EncapVrfId:     d.EncapVrfID,
			DecapNext:      DecapNext(d.DecapNextIndex),
		}, uint32(d.SwIfIndex), true
	},
}
