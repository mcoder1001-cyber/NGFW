package agent

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// PkiFileState implements the existing read-only contract without implying that
// this build materializes PKI files. P11 owns secret transport and strongSwan
// materializer wiring; until those exist, no filesystem or secret store is read.
func (g *server) PkiFileState(_ context.Context, req *ngfwv1.PkiFileStateRequest) (*ngfwv1.PkiFileStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	return &ngfwv1.PkiFileStateResponse{
		Owner:       g.svc.owner,
		RetrievedAt: timestamppb.New(g.svc.now()),
		Unavailable: "PKI file materializer is not wired in this agent build",
	}, nil
}
