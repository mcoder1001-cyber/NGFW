package agent

// WireguardState (F-wireguard, docs/contracts/proto.md §11): the live, read-only WireGuard state of
// this owner's wg<N> interfaces and their peers — DF-5's dumps (show_private_key=false, the v1 peer
// dump without preshared keys), the interfaces' stats-segment counters and the handshake times the
// peer-event watcher observed. Never mutates; one walk at a time (D-132).

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// wireguardStateMu serialises the WireGuard state walks (D-132: never two walks of a VPP table at once).
var wireguardStateMu sync.Mutex

// WireguardState implements the WireguardState RPC.
func (g *server) WireguardState(ctx context.Context, req *vrxv1.WireguardStateRequest) (*vrxv1.WireguardStateResponse, error) {
	return g.svc.WireguardState(ctx, req, g.stats)
}

// WireguardState builds the response; stats (nil: no counters) is the stats-segment reader.
func (s *Service) WireguardState(ctx context.Context, req *vrxv1.WireguardStateRequest, stats statsSource) (*vrxv1.WireguardStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	wireguardStateMu.Lock()
	defer wireguardStateMu.Unlock()
	st, err := wireguard.DumpState(ctx, s.vpp, s.owner, req.GetInterfaces()...)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "wireguard state: %v", err)
	}
	type ctr struct{ rxp, rxb, txp, txb uint64 }
	counters := map[uint32]ctr{}
	if stats != nil && len(st) > 0 {
		if snap, err := stats.InterfaceStats(); err == nil {
			for _, c := range snap {
				counters[c.InterfaceIndex] = ctr{c.Rx.Packets, c.Rx.Bytes, c.Tx.Packets, c.Tx.Bytes}
			}
		} else {
			s.log.Debug("wireguard state: stats segment unavailable, counters are 0", "err", err)
		}
	}
	obs := subsystems.WireguardObserverFor(s.owner)
	resp := &vrxv1.WireguardStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), EventsActive: obs != nil && obs.Active()}
	for _, i := range st {
		c := counters[i.SwIfIndex]
		out := &vrxv1.WireguardInterfaceState{
			Name: i.Name, Instance: i.Instance, SwIfIndex: i.SwIfIndex, PublicKey: i.PublicKey, ListenPort: i.Port,
			ListenAddress: i.SrcIP, AdminUp: i.AdminUp, LinkUp: i.LinkUp,
			RxPackets: c.rxp, RxBytes: c.rxb, TxPackets: c.txp, TxBytes: c.txb,
		}
		for _, p := range i.Peers {
			ps := &vrxv1.WireguardPeerState{
				PublicKey: p.PublicKey, PeerIndex: p.PeerIndex, Established: p.Established, Dead: p.Dead,
				Endpoint: p.Endpoint, EndpointPort: p.EndpointPort, PersistentKeepaliveSec: p.PersistentKeepalive,
				AllowedIps: p.AllowedIps,
			}
			if obs != nil {
				if t, ok := obs.LastEstablished(i.Name, p.PublicKey); ok {
					ps.LastHandshake = timestamppb.New(t)
				}
			}
			out.Peers = append(out.Peers, ps)
		}
		resp.Interfaces = append(resp.Interfaces, out)
	}
	return resp, nil
}
