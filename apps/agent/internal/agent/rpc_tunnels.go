package agent

// TunnelState (S-tunnels-contract, T1): the live tunnel interfaces of this owner from one
// sw_interface_dump — every interface whose owner tag is ours and whose device class belongs to a
// tunnel descriptor (gre, ipip incl. 6RD, vxlan, vxlan-gpe, gtpu, l2tpv3, pppoe), named from
// tunnels.meta (the instance-keyed kinds and PPPoE) or the tag id itself (the kinds VPP names).
// Read-only; never another owner's interfaces.

import (
	"context"
	"errors"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/interface_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/descriptors/gtpu"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/descriptors/l2tp"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/descriptors/vxlan_gpe"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// tunnelKindOf maps a tunnel descriptor to the `tunnels.<kind>` key it is projected from.
var tunnelKindOf = map[string]string{
	gre.TunnelName:       "gre",
	ipip.TunnelName:      "ipip",
	ipip.SixrdName:       "ipip",
	vxlan.TunnelName:     "vxlan",
	vxlan_gpe.TunnelName: "vxlanGpe",
	gtpu.TunnelName:      "gtpu",
	l2tp.TunnelName:      "l2tpv3",
	pppoe.SessionName:    "pppoe",
}

// TunnelState implements ngfw.v1.Dataplane.
func (g *server) TunnelState(ctx context.Context, req *ngfwv1.TunnelStateRequest) (*ngfwv1.TunnelStateResponse, error) {
	return g.svc.TunnelState(ctx, req)
}

// TunnelState lists this owner's tunnel interfaces (see rpc_tunnels.go).
func (s *Service) TunnelState(ctx context.Context, req *ngfwv1.TunnelStateRequest) (*ngfwv1.TunnelStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	names := map[string]string{}
	if d, ok := s.sched.Registry().Get(desired.TunnelMetaName); ok {
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "tunnels.meta: %v", err)
		}
		for _, kv := range kvs {
			if m, err := desired.DecodeTunnelMeta(kv.Value); err == nil {
				names[m.ID] = m.Name
			}
		}
	}
	tunnels, err := tunnelState(ctx, s.vpp, s.owner, names)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "tunnel state: %v", err)
	}
	return &ngfwv1.TunnelStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), Tunnels: tunnels}, nil
}

func tunnelState(ctx context.Context, c vpp.Client, owner string, names map[string]string) ([]*ngfwv1.TunnelStateTunnel, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	var out []*ngfwv1.TunnelStateTunnel
	for _, idx := range t.Indexes() {
		key, ok := t.KeyFor(idx)
		if !ok {
			continue
		}
		kind, ok := tunnelKindOf[scheduler.Key(key).Descriptor()]
		if !ok {
			continue
		}
		d, _ := t.Details(idx)
		id := scheduler.Key(key).ID()
		name := names[id]
		if name == "" {
			name = id
		}
		out = append(out, &ngfwv1.TunnelStateTunnel{
			Name: name, Kind: kind, Interface: t.VPPName(idx), SwIfIndex: idx,
			AdminUp: d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
			LinkUp:  d.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
			Mtu:     d.Mtu[0], DeviceClass: strings.TrimRight(d.InterfaceDevType, "\x00"),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetKind() != out[j].GetKind() {
			return out[i].GetKind() < out[j].GetKind()
		}
		return out[i].GetName() < out[j].GetName()
	})
	return out, nil
}
