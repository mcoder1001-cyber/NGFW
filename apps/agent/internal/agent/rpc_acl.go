package agent

// AclState (F-acl, docs/contracts/proto.md §11): runtime state of this owner's ACLs — per-list VPP
// index, rule count and hit counters; a page of one list's configuration rules with the VPP rules
// each expanded to and their counters; the ACLs bound per interface (other owners' included,
// D-066). Read-only, never part of Retrieve (§5). The acl runtime is the one subsystems registered
// for this agent's state dir and owner (internal/actions/acl).

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	"ngfw/agent/internal/vpp"
)

func (g *server) AclState(ctx context.Context, req *vrxv1.AclStateRequest) (*vrxv1.AclStateResponse, error) { //nolint:revive // the generated gRPC method name
	return g.svc.ACLState(ctx, req)
}

// ACLState implements the AclState RPC.
func (s *Service) ACLState(ctx context.Context, req *vrxv1.AclStateRequest) (*vrxv1.AclStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	rt := aclstate.RuntimeFor(s.st.dir, s.owner) // st.dir is fixed at construction
	if rt == nil {
		return nil, status.Error(codes.Unimplemented, "the acl domain is not running in this agent")
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	resp, err := rt.State(ctx, req)
	switch {
	case err == nil:
		return resp, nil
	case errors.Is(err, aclstate.ErrInvalid):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, aclstate.ErrNotFound):
		return nil, status.Error(codes.NotFound, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return nil, status.Error(codes.Unavailable, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "acl state: %v", err)
	}
}
