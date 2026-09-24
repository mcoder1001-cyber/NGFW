package agent

// F-vrf-static-ecmp RPCs (wave-A-hotspots A4): ListRoutes (the FIB browser's state RPC, docs/contracts/proto.md §11) and
// the ping / traceroute cases of Action. The work is in internal/actions/vrf-static-ecmp; this file maps it onto gRPC.

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	vse "ngfw/agent/internal/actions/vrf-static-ecmp"
	"ngfw/agent/internal/vpp"
)

// ListRoutes implements vrx.v1.Dataplane/ListRoutes: one page of one VRF's live FIB, paged and filtered here.
func (g *server) ListRoutes(ctx context.Context, req *vrxv1.ListRoutesRequest) (*vrxv1.ListRoutesResponse, error) {
	s := g.svc
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	vrf, table := req.GetVrf(), uint32(0)
	if vrf == "" {
		vrf = "default"
	}
	if vrf != "default" {
		id, ok := s.resolveVRF(vrf)
		if !ok {
			return nil, status.Errorf(codes.NotFound, "VRF %q is not in the agent's configuration", vrf)
		}
		table = id
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	page, err := vse.ListRoutes(ctx, s.vpp, s.owner, vse.Query{
		Table: table, Family: req.GetFamily(), Prefix: req.GetPrefix(), Source: req.GetSource(),
		Offset: req.GetOffset(), Limit: req.GetLimit(),
	})
	if err != nil {
		return nil, actionStatus(err)
	}
	return &vrxv1.ListRoutesResponse{Routes: page.Routes, Total: page.Total, Owner: s.owner, Vrf: vrf, TableId: table, RetrievedAt: timestamppb.New(s.now())}, nil
}

// actionPing runs a ping through VPP's ping plugin (default table only; vse.ValidatePing).
func (g *server) actionPing(req *vrxv1.PingAction, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	plan, err := vse.ValidatePing(req)
	if err != nil {
		return actionStatus(err)
	}
	if !g.svc.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if g.log != nil {
		g.log.Info("action ping", "target", plan.Target.String(), "count", plan.Count, "interval", plan.Interval.String())
	}
	return actionStatus(vse.Ping(stream.Context(), g.svc.vpp, plan, stream.Send))
}

// actionTraceroute answers UNIMPLEMENTED (no VPP API, no Linux path before P12).
func (g *server) actionTraceroute(req *vrxv1.TracerouteAction) error {
	return actionStatus(vse.Traceroute(req))
}

// actionStatus maps the action/lister errors onto gRPC codes.
func actionStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, vse.ErrInvalid), errors.Is(err, vse.ErrBadRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, vse.ErrUnimplemented):
		return status.Error(codes.Unimplemented, err.Error())
	case errors.Is(err, vse.ErrBusy), errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
