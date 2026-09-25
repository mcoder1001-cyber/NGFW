package agent

// P12 RPC (wave-A-hotspots A4): RoutingState, the routing-daemon state RPC (docs/contracts/proto.md §11 P12). The work is
// in internal/subsystems/frr.go (FRR reads) and the DF-8 lcp descriptor (pairs); this file maps it onto gRPC.

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// RoutingState implements vrx.v1.Dataplane/RoutingState.
func (g *server) RoutingState(ctx context.Context, req *vrxv1.RoutingStateRequest) (*vrxv1.RoutingStateResponse, error) {
	s := g.svc
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	vrf := req.GetRibVrf()
	if vrf != "" && vrf != "default" {
		if _, ok := s.resolveVRF(vrf); !ok {
			return nil, status.Errorf(codes.NotFound, "VRF %q is not in the agent's configuration", vrf)
		}
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	rt := subsystems.FRRRuntime(s.owner)
	if rt == nil {
		return nil, status.Error(codes.Unavailable, "routing subsystem is not wired")
	}
	st, err := rt.State(ctx, req.GetReaders(), req.GetRibPrefixes(), vrf)
	if err != nil {
		switch {
		case errors.Is(err, subsystems.ErrState):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, subsystems.ErrStateBusy):
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "routing state: %v", err)
	}
	resp := &vrxv1.RoutingStateResponse{
		Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), FrrRunning: st.Running, FrrVersion: st.Version,
		Error: st.Err, RibCounts: st.RIBCounts, Readers: st.Readers, Rib: st.RIB,
	}
	for _, in := range st.BGP {
		bi := &vrxv1.BgpInstanceState{Vrf: in.VRF, Asn: in.ASN, RouterId: in.RouterID}
		for _, n := range in.Neighbors {
			bn := &vrxv1.BgpNeighborState{
				Address: n.Address, RemoteAs: n.RemoteAS, State: n.State, UptimeSec: n.UptimeSec,
				PrefixesReceived: n.PrefixesReceived, PrefixesSent: n.PrefixesSent, Flaps: n.Flaps, Established: n.Established,
				Description: n.Description, MessagesReceived: n.MessagesReceived, MessagesSent: n.MessagesSent,
			}
			for _, a := range n.AFIs {
				bn.Afis = append(bn.Afis, &vrxv1.BgpNeighborAfiState{Afi: a.AFI, PrefixesReceived: a.PrefixesReceived, PrefixesSent: a.PrefixesSent})
			}
			bi.Neighbors = append(bi.Neighbors, bn)
		}
		resp.Bgp = append(resp.Bgp, bi)
	}
	pairs, err := st.LcpPairs, st.PairsErr // read inside the serialised walk (review M3)
	switch {
	case errors.Is(err, vpp.ErrDisconnected):
		return nil, status.Error(codes.Unavailable, err.Error())
	case err != nil:
		if resp.Error != "" {
			resp.Error += "; "
		}
		resp.Error += "lcp pairs: " + err.Error()
	}
	resp.LcpPairs = pairs
	return resp, nil
}
