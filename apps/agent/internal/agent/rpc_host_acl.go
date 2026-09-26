package agent

// HostAclState (F-host-acl-nftables, docs/contracts/proto.md §11): the host firewall table as the kernel
// holds it (`nft -j list table`), annotated with what the agent rendered, with per-rule counters —
// read-only runtime state, never part of Retrieve (§5). No VPP round trip: works while VPP is down.

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/nftables"
)

//nolint:revive // the generated gRPC method name (vrx.v1.Dataplane/HostAclState)
func (g *server) HostAclState(ctx context.Context, req *vrxv1.HostAclStateRequest) (*vrxv1.HostAclStateResponse, error) {
	return g.svc.HostACLState(ctx, req)
}

// HostACLState implements the HostAclState RPC.
func (s *Service) HostACLState(ctx context.Context, req *vrxv1.HostAclStateRequest) (*vrxv1.HostAclStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	rt := nftables.RuntimeFor(s.st.dir, s.owner) // st.dir is fixed at construction
	if rt == nil {
		return nil, status.Error(codes.Unimplemented, "the host firewall (acl.host) is not wired in this agent")
	}
	resp, err := rt.State(ctx, s.now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "host firewall state: %v", err)
	}
	return resp, nil
}
