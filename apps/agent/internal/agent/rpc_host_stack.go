package agent

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/hoststack"
	"ngfw/agent/internal/vpp"
)

// HostStackState implements the F-host-stack RPC (docs/contracts/proto.md § F-host-stack): a
// read-only snapshot — session layer probe, this owner's rules, the namespaces it applied.
func (g *server) HostStackState(ctx context.Context, req *vrxv1.HostStackStateRequest) (*vrxv1.HostStackStateResponse, error) {
	return g.svc.HostStackState(ctx, req)
}

// HostStackState is the Service side of the RPC.
func (s *Service) HostStackState(ctx context.Context, req *vrxv1.HostStackStateRequest) (*vrxv1.HostStackStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	snap, err := hoststack.State(ctx, s.vpp, s.owner)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "host stack state: %v", err)
	}
	out := &vrxv1.HostStackStateResponse{SessionEnabled: snap.SessionEnabled, SessionDetail: snap.SessionDetail,
		Namespaces: snap.Namespaces, RuleCountTotal: snap.RuleCountTotal, RetrievedAt: timestamppb.New(s.now())}
	for _, r := range snap.Rules {
		out.Rules = append(out.Rules, &vrxv1.HostStackRuleState{Tag: r.Tag, Scope: r.Scope, Transport: r.Transport,
			Local: r.Local, LocalPort: r.LocalPort, Remote: r.Remote, RemotePort: r.RemotePort, Action: r.Action,
			AppnsIndexes: snap.AppnsIndexes[r.Tag]})
	}
	return out, nil
}
