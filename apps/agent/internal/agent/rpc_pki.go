package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ngfw/agent/internal/subsystems"

	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// PkiFileState reports only owner-authorized public file facts; materializer errors are sanitized.
func (g *server) PkiFileState(_ context.Context, req *ngfwv1.PkiFileStateRequest) (*ngfwv1.PkiFileStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	rt := subsystems.PKIRuntimeFor(g.svc.owner)
	if rt.Enabled() {
		files, err := rt.State()
		if err != nil {
			return nil, status.Error(codes.Unavailable, "PKI file state is unavailable")
		}
		return &ngfwv1.PkiFileStateResponse{Owner: g.svc.owner, Root: rt.Root(), RetrievedAt: timestamppb.New(g.svc.now()), Files: files}, nil
	}
	return &ngfwv1.PkiFileStateResponse{
		Owner:       g.svc.owner,
		RetrievedAt: timestamppb.New(g.svc.now()),
		Unavailable: "PKI file materializer is not wired in this agent build",
	}, nil
}
