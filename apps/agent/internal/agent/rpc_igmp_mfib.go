package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/scheduler"
	"strconv"
)

func (g *server) MulticastState(ctx context.Context, req *ngfwv1.MulticastStateRequest) (*ngfwv1.MulticastStateResponse, error) {
	return g.svc.MulticastState(ctx, req)
}

// MulticastState reads actual VPP membership and owned static mFIB entries. PIM belongs to F-pim-frrsync.
func (s *Service) MulticastState(ctx context.Context, req *ngfwv1.MulticastStateRequest) (*ngfwv1.MulticastStateResponse, error) {
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, e
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if e := s.lock(ctx); e != nil {
		return nil, e
	}
	defer s.unlock()
	groups, e := igmp.DumpGroups(ctx, s.vpp, s.owner)
	if e != nil {
		return nil, status.Errorf(codes.Internal, "IGMP state: %v", e)
	}
	kvs, e := s.sched.Retrieve(ctx, scheduler.Only(mfib.Name))
	if e != nil {
		return nil, status.Errorf(codes.Internal, "mFIB state: %v", e)
	}
	out := &ngfwv1.MulticastStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, g := range groups {
		out.Groups = append(out.Groups, &ngfwv1.IgmpGroupState{Interface: g.Interface, Group: g.Group, Sources: g.Sources})
	}
	s.mu.Lock()
	names := map[uint32]string{0: "default"}
	for name, id := range s.vrfIDs {
		names[id] = name
	}
	s.mu.Unlock()
	for _, kv := range kvs {
		r, e := df7.Decode[mfib.Route](kv.Value)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "mFIB decode: %v", e)
		}
		name := names[r.Table]
		if name == "" {
			name = strconv.FormatUint(uint64(r.Table), 10)
		}
		v := &ngfwv1.MrouteState{Vrf: name, Group: r.Group, Source: r.Source}
		for _, p := range r.Paths {
			if p.Flags == "accept" {
				v.Accept = p.Interface
			} else {
				v.Forward = append(v.Forward, p.Interface)
			}
		}
		out.Mroutes = append(out.Mroutes, v)
	}
	return out, nil
}
