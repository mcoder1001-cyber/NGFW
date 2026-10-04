package agent

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/subsystems"
)

// MplsLdpState reports the source's last bounded observation and scheduler status.
func (g *server) MplsLdpState(ctx context.Context, req *ngfwv1.MplsLdpStateRequest) (*ngfwv1.MplsLdpStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if !g.svc.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	out, err := subsystems.MplsLdpState(ctx, g.svc.owner)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if out == nil {
		return nil, status.Error(codes.Unavailable, "LDP runtime unavailable")
	}
	out.Owner = g.svc.owner
	out.RetrievedAt = timestamppb.New(g.svc.now())
	return out, nil
}
