package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	hastatesync "ngfw/agent/internal/actions/ha-state-sync"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/hasync"
)

// One Service per agent process, matching the existing capture sidecar pattern.
var haSyncRuntimes sync.Map

func (s *Service) haSyncRuntime() *hastatesync.Runtime {
	value, _ := haSyncRuntimes.LoadOrStore(s, &hastatesync.Runtime{Client: s.vpp, GlobalsOwner: s.captureConfig.GlobalsOwner})
	return value.(*hastatesync.Runtime)
}
func (g *server) HaSyncState(ctx context.Context, req *ngfwv1.HaSyncStateRequest) (*ngfwv1.HaSyncStateResponse, error) {
	return g.svc.haSyncState(ctx, req)
}
func (s *Service) haSyncState(ctx context.Context, req *ngfwv1.HaSyncStateRequest) (*ngfwv1.HaSyncStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	config := proto.Clone(s.st.desired).(*ngfwv1.DesiredState)
	s.unlock()
	out := &ngfwv1.HaSyncStateResponse{Owner: s.owner, ActionsAllowed: s.captureConfig.GlobalsOwner, RetrievedAt: timestamppb.New(s.now())}
	cluster := config.GetHa().GetCluster()
	flags := cluster.GetStateSync()
	enabled := cluster.GetEnabled()
	mode := config.GetNat().GetMode()
	out.Kinds = []*ngfwv1.HaSyncKindState{
		{Kind: "nat44-ei", Supported: true, Configured: enabled && flags.GetNat() && mode == "ei", Reason: "native unauthenticated UDP endpoint sync; remote continuity requires a two-node test"},
		{Kind: "nat44-ed", Configured: enabled && flags.GetNat() && mode != "ei", Reason: "not supported by VPP (V2); failover loses sessions"},
		{Kind: "acl", Configured: enabled && flags.GetAcl(), Reason: "not supported by VPP (V2); reflexive sessions are not preserved"},
		{Kind: "ipsec", Configured: enabled && flags.GetIpsec(), Reason: "no native SA sequence/replay sync API; native IKEv2 must re-key on failover"},
	}
	var l hasync.Listener
	var f hasync.Failover
	if s.vpp == nil || !s.vpp.Connected() {
		out.ObservationError = "VPP binary API is not connected"
	} else {
		var err error
		l, err = hasync.ReadListener(ctx, s.vpp)
		if err == nil {
			f, err = hasync.ReadFailover(ctx, s.vpp)
		}
		if err != nil {
			out.ObservationError = hasync.ExplainUnavailable(err)
		} else {
			if l.Port != 0 {
				out.Listener = &ngfwv1.HaNatListener{Address: proto.String(l.Address), Port: proto.Uint32(uint32(l.Port)), PathMtu: proto.Uint32(l.PathMtu)}
			}
			if f.Port != 0 {
				out.Failover = &ngfwv1.HaNatFailover{Address: proto.String(f.Address), Port: proto.Uint32(uint32(f.Port)), SessionRefreshSec: proto.Uint32(f.SessionRefreshSec)}
			}
			expectedL, expectedF := flags.GetNatListener(), flags.GetNatFailover()
			out.Kinds[0].Active = out.Kinds[0].Configured && l.Port != 0 && f.Port != 0 && expectedL != nil && expectedF != nil &&
				l.Address == expectedL.GetAddress() && uint32(l.Port) == expectedL.GetPort() && l.PathMtu == expectedL.GetPathMtu() &&
				f.Address == expectedF.GetAddress() && uint32(f.Port) == expectedF.GetPort() && f.SessionRefreshSec == expectedF.GetSessionRefreshSec() &&
				(cluster.GetVrf() == "" || cluster.GetVrf() == "default")
		}
	}
	observation := s.haSyncRuntime().Observation()
	out.ResyncCount = observation.Completed
	if !observation.CompletedAt.IsZero() {
		out.LastResync = timestamppb.New(observation.CompletedAt)
		out.LastMissedCount = proto.Uint32(observation.MissedCount)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return out, nil
}

func (g *server) haSyncAction(req *ngfwv1.HaSyncAction, stream grpc.ServerStreamingServer[ngfwv1.ActionOutput]) error {
	return g.svc.haSyncAction(stream.Context(), req, stream.Send)
}
func (s *Service) haSyncAction(ctx context.Context, req *ngfwv1.HaSyncAction, send func(*ngfwv1.ActionOutput) error) error {
	// The startup-resolved role is mandatory; owner names/environment cannot escalate a slot.
	if !s.captureConfig.GlobalsOwner {
		return status.Error(codes.PermissionDenied, dfkit.ErrNotGlobalsOwner.Error())
	}
	if req.GetOp() != ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC && req.GetOp() != ngfwv1.HaSyncOp_HA_SYNC_OP_FLUSH {
		return status.Error(codes.InvalidArgument, "unknown HA sync operation")
	}
	var observed hastatesync.Observation
	err := s.exclusive(ctx, func(ctx context.Context) error {
		cluster := s.st.desired.GetHa().GetCluster()
		if !cluster.GetEnabled() || !cluster.GetStateSync().GetNat() || s.st.desired.GetNat().GetMode() != "ei" {
			return status.Error(codes.FailedPrecondition, "NAT44-EI HA must be enabled in running state")
		}
		flags := cluster.GetStateSync()
		if cluster.GetVrf() != "" && cluster.GetVrf() != "default" {
			return status.Error(codes.FailedPrecondition, "native NAT HA has no sync VRF binding")
		}
		liveL, err := hasync.ReadListener(ctx, s.vpp)
		if err != nil {
			return err
		}
		liveF, err := hasync.ReadFailover(ctx, s.vpp)
		if err != nil {
			return err
		}
		wantL, wantF := flags.GetNatListener(), flags.GetNatFailover()
		if wantL == nil || wantF == nil || liveL.Port == 0 || liveF.Port == 0 ||
			liveL.Address != wantL.GetAddress() || uint32(liveL.Port) != wantL.GetPort() || liveL.PathMtu != wantL.GetPathMtu() ||
			liveF.Address != wantF.GetAddress() || uint32(liveF.Port) != wantF.GetPort() || liveF.SessionRefreshSec != wantF.GetSessionRefreshSec() {
			return status.Error(codes.FailedPrecondition, "native HA endpoints differ from running configuration")
		}
		observed, err = s.haSyncRuntime().Run(ctx, req.GetOp())
		return err
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return status.FromContextError(err).Err()
		}
		if _, ok := status.FromError(err); ok {
			return err
		}
		if errors.Is(err, dfkit.ErrSpec) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	summary := "flushed queued NAT HA updates (sessions retained)"
	if req.GetOp() == ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC {
		summary = fmt.Sprintf("NAT44-EI resync completed; %d unacknowledged messages", observed.MissedCount)
	}
	return send(&ngfwv1.ActionOutput{Output: &ngfwv1.ActionOutput_Done{Done: &ngfwv1.ActionDone{Summary: summary}}})
}
