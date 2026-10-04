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

	"google.golang.org/protobuf/proto"
	ifapi "ngfw/agent/binapi/interface"

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
	return g.svc.tunnelStateWithStats(ctx, req, g.stats)
}

// TunnelState lists this owner's tunnel interfaces (see rpc_tunnels.go).
func (s *Service) TunnelState(ctx context.Context, req *ngfwv1.TunnelStateRequest) (*ngfwv1.TunnelStateResponse, error) {
	return s.tunnelStateWithStats(ctx, req, nil)
}

func (s *Service) tunnelStateWithStats(ctx context.Context, req *ngfwv1.TunnelStateRequest, stats statsSource) (*ngfwv1.TunnelStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	s.tunnelStateOnce.Do(func() { s.tunnelStateGate = make(chan struct{}, 1) })
	gate := s.tunnelStateGate
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
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
	s.enrichTunnelState(ctx, tunnels, stats, names)
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

// enrichTunnelState only joins live descriptor/FIB/stats readback to already owner-filtered interfaces.
// Never substitutes candidate or agent metadata for a missing VPP getter.
func (s *Service) enrichTunnelState(ctx context.Context, tunnels []*ngfwv1.TunnelStateTunnel, stats statsSource, names map[string]string) {
	objects := map[scheduler.Key]proto.Message{}
	pppoeByName := map[string]*pppoe.Session{}
	queried := map[string]bool{}
	failed := map[string]bool{}
	for _, t := range tunnels {
		descriptor := ""
		for d, kind := range tunnelKindOf {
			if kind == t.Kind && d != ipip.SixrdName {
				descriptor = d
				break
			}
		}
		if t.DeviceClass == "ip6ip-6rd" {
			descriptor = ipip.SixrdName
		}
		if descriptor != ipip.SixrdName && !queried[descriptor] {
			queried[descriptor] = true
			if d, ok := s.sched.Registry().Get(descriptor); ok {
				kvs, err := d.Retrieve(ctx)
				if err != nil {
					failed[descriptor] = true
				} else {
					for _, kv := range kvs {
						objects[kv.Key] = kv.Value
						if p, ok := kv.Value.(*pppoe.Session); ok {
							if id, err := pppoe.SessionID(p.GetClientMac(), p.GetSessionId()); err == nil {
								name := names[id]
								if name == "" {
									name = id
								}
								pppoeByName[name] = p
							}
						}
					}
				}
			} else {
				failed[descriptor] = true
			}
		}
		// Instance kinds use engine names as ids; named kinds use the config name; PPPoE uses MAC/session id.
		var value proto.Message
		for _, id := range []string{t.Interface, t.Name} {
			if v := objects[scheduler.Join(descriptor, id)]; v != nil {
				value = v
				break
			}
		}
		if descriptor == pppoe.SessionName {
			value = pppoeByName[t.Name]
		}
		if value != nil {
			fillTunnelEndpoints(t, value)
		} else {
			note := "Tunnel parameters unavailable from VPP readback"
			if descriptor == ipip.SixrdName {
				note = "6RD parameters have no VPP getter; configuration is not live readback"
			} else if failed[descriptor] {
				note = "Tunnel descriptor readback unavailable"
			}
			t.Notes = append(t.Notes, note)
		}
		for _, v6 := range []bool{false, true} {
			table, err := ifapi.NewServiceClient(s.vpp).SwInterfaceGetTable(ctx, &ifapi.SwInterfaceGetTable{SwIfIndex: interface_types.InterfaceIndex(t.SwIfIndex), IsIPv6: v6})
			if err == nil {
				if v6 {
					t.Ipv6TableId = proto.Uint32(table.VrfID)
				} else {
					t.Ipv4TableId = proto.Uint32(table.VrfID)
				}
			} else {
				if v6 {
					t.Notes = append(t.Notes, "Interface IPv6 FIB unavailable")
				} else {
					t.Notes = append(t.Notes, "Interface IPv4 FIB unavailable")
				}
			}
		}
	}
	if stats == nil {
		for _, t := range tunnels {
			t.Notes = append(t.Notes, "Interface counters unavailable")
		}
		return
	}
	snapshot, err := stats.InterfaceStats()
	if err != nil {
		for _, t := range tunnels {
			t.Notes = append(t.Notes, "Interface counters unavailable")
		}
		return
	}
	counters := map[uint32]*ngfwv1.InterfaceCounters{}
	for _, c := range snapshot {
		counters[c.InterfaceIndex] = &ngfwv1.InterfaceCounters{Name: c.InterfaceName, SwIfIndex: c.InterfaceIndex, RxPackets: c.Rx.Packets, RxBytes: c.Rx.Bytes, TxPackets: c.Tx.Packets, TxBytes: c.Tx.Bytes, Drops: c.Drops, Errors: c.RxErrors + c.TxErrors, Punts: c.Punts, RxMisses: c.RxNoBuf + c.RxMiss}
	}
	for _, t := range tunnels {
		if c := counters[t.SwIfIndex]; c != nil && (c.Name == "" || c.Name == t.Interface) {
			t.Counters = c
			t.Counters.Name = t.Interface
		}
		if t.Counters == nil {
			t.Notes = append(t.Notes, "Interface counters unavailable")
		}
	}
}

func fillTunnelEndpoints(t *ngfwv1.TunnelStateTunnel, value proto.Message) {
	switch v := value.(type) {
	case *gre.Tunnel:
		t.Src = proto.String(v.GetSrc())
		t.Dst = proto.String(v.GetDst())
		t.UnderlayTableId = proto.Uint32(v.GetOuterTableId())
	case *ipip.Tunnel:
		t.Src = proto.String(v.GetSrc())
		t.Dst = proto.String(v.GetDst())
		t.UnderlayTableId = proto.Uint32(v.GetTableId())
	case *vxlan.Tunnel:
		t.Src = proto.String(v.GetSrc())
		t.Dst = proto.String(v.GetDst())
		t.UnderlayTableId = proto.Uint32(v.GetEncapVrfId())
	case *vxlan_gpe.Tunnel:
		t.Src = proto.String(v.GetLocal())
		t.Dst = proto.String(v.GetRemote())
		t.UnderlayTableId = proto.Uint32(v.GetEncapVrfId())
	case *gtpu.Tunnel:
		t.Src = proto.String(v.GetSrc())
		t.Dst = proto.String(v.GetDst())
		t.UnderlayTableId = proto.Uint32(v.GetEncapVrfId())
	case *l2tp.Tunnel:
		t.Src = proto.String(v.GetOurAddress())
		t.Dst = proto.String(v.GetClientAddress())
		t.Notes = append(t.Notes, "L2TPv3 underlay FIB is not reported by VPP")
	case *pppoe.Session:
		t.Dst = proto.String(v.GetClientIp())
		t.Notes = append(t.Notes, "PPPoE session has a client endpoint, not an outer IP tunnel source")
	}
}
