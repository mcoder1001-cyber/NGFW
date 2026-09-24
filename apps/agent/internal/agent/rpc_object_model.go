package agent

// FqdnObjectState (F-object-model, docs/contracts/proto.md §11): the FQDN resolver's state of the
// FQDN address objects this agent holds — read-only runtime state, never part of Retrieve (§5).
// The objects runtime is the one subsystems registered for this agent's state dir and owner.

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/objects"
)

func (g *server) FqdnObjectState(_ context.Context, req *vrxv1.FqdnObjectStateRequest) (*vrxv1.FqdnObjectStateResponse, error) {
	return g.svc.FqdnObjectState(req)
}

// FqdnObjectState implements the RPC (no VPP round trip; works while VPP is disconnected).
func (s *Service) FqdnObjectState(req *vrxv1.FqdnObjectStateRequest) (*vrxv1.FqdnObjectStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	rt := objects.RuntimeFor(s.st.dir, s.owner) // st.dir is fixed at construction
	if rt == nil {
		return nil, status.Error(codes.Unavailable, "the objects domain is not running in this agent")
	}
	resp := &vrxv1.FqdnObjectStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, st := range rt.FQDNStates(req.GetNames()...) {
		o := &vrxv1.FqdnObjectState{Name: st.Name, Fqdn: st.FQDN, Error: st.Err, Failures: uint32(max(st.Failures, 0))} //nolint:gosec // a small non-negative counter
		for _, a := range st.Addresses {
			o.Addresses = append(o.Addresses, a.String())
		}
		if !st.LastResolved.IsZero() {
			o.LastResolved = timestamppb.New(st.LastResolved)
		}
		if !st.NextRefresh.IsZero() {
			o.NextRefresh = timestamppb.New(st.NextRefresh)
		}
		resp.Objects = append(resp.Objects, o)
	}
	return resp, nil
}
