package agent

// Srv6State (F-srv6, docs/contracts/proto.md §11): the live, read-only SRv6 objects of this owner — the
// local SIDs, policies and steering entries its own Creates claimed (DF-6 ClaimStore), as VPP has them,
// local SIDs with their good/bad traffic counters (sr_localsids_with_packet_stats_dump). Never another
// owner's objects, never the write-only globals. Never mutates. D-132: one SR walk in flight per agent;
// a second caller waits at most srv6WalkWait and then gets UNAVAILABLE (the UI's Refresh retries).

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// srv6WalkWait bounds the wait for the SR walk slot (D-132: 3 s).
var srv6WalkWait = 3 * time.Second

// srv6Walk is the SR walk slot (one walk in flight per agent process).
var srv6Walk = make(chan struct{}, 1)

// Srv6State implements the Srv6State RPC.
func (g *server) Srv6State(ctx context.Context, req *vrxv1.Srv6StateRequest) (*vrxv1.Srv6StateResponse, error) {
	return g.svc.Srv6State(ctx, req)
}

// Srv6State builds the response.
func (s *Service) Srv6State(ctx context.Context, req *vrxv1.Srv6StateRequest) (*vrxv1.Srv6StateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	t := time.NewTimer(srv6WalkWait)
	defer t.Stop()
	select {
	case srv6Walk <- struct{}{}:
		defer func() { <-srv6Walk }()
	case <-t.C:
		return nil, status.Error(codes.Unavailable, "another SRv6 state walk is in flight; one at a time (D-132) — retry")
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	snap, err := subsystems.Srv6State(ctx, s.owner)
	switch {
	case errors.Is(err, subsystems.ErrSrv6NotWired):
		return nil, status.Error(codes.Unimplemented, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return nil, status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, df6.ErrPluginNotLoaded):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	case err != nil:
		return nil, status.Errorf(codes.Internal, "srv6 state: %v", err)
	}
	resp := &vrxv1.Srv6StateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, l := range snap.LocalSids {
		resp.LocalSids = append(resp.LocalSids, &vrxv1.Srv6StateLocalSid{
			Sid: l.GetSid(), Behavior: desired.Srv6BehaviorName(l.GetBehavior()), Psp: l.GetEndPsp(), FibTable: l.GetFibTable(),
			Interface: l.GetInterface(), NextHop: l.GetNextHop(), LookupTable: l.GetLookupTable(),
			GoodPackets: l.GoodPackets, GoodBytes: l.GoodBytes, BadPackets: l.BadPackets, BadBytes: l.BadBytes,
		})
	}
	for _, p := range snap.Policies {
		out := &vrxv1.Srv6StatePolicy{Bsid: p.GetBsid(), Type: desired.Srv6PolicyTypeName(p.GetType()), Encap: p.GetEncap(), FibTable: p.GetFibTable(), EncapSource: p.GetEncapSrc()}
		for _, l := range p.GetSidLists() {
			out.SidLists = append(out.SidLists, &vrxv1.Srv6StateSidList{Sids: append([]string(nil), l.GetSids()...), Weight: l.GetWeight()})
		}
		resp.Policies = append(resp.Policies, out)
	}
	for _, st := range snap.Steering {
		out := &vrxv1.Srv6StateSteering{Bsid: st.GetBsid(), Prefix: st.GetPrefix(), FibTable: st.GetTableId(), Interface: st.GetInterface()}
		switch st.GetTrafficType() {
		case sr.SteerType_L2:
			out.TrafficType = "l2"
		case sr.SteerType_IPV4:
			out.TrafficType = "ipv4"
		default:
			out.TrafficType = "ipv6"
		}
		resp.Steering = append(resp.Steering, out)
	}
	return resp, nil
}
