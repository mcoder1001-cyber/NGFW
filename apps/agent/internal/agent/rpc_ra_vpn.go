package agent

import (
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"regexp"
	"sort"
)

var raProfileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (g *server) RemoteAccessCapabilities(ctx context.Context, req *ngfwv1.RemoteAccessCapabilitiesRequest) (*ngfwv1.RemoteAccessCapabilitiesResponse, error) {
	return g.svc.RemoteAccessCapabilities(ctx, req)
}
func (s *Service) RemoteAccessCapabilities(ctx context.Context, req *ngfwv1.RemoteAccessCapabilitiesRequest) (*ngfwv1.RemoteAccessCapabilitiesResponse, error) {
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, status.Error(codes.PermissionDenied, "remote-access owner mismatch")
	}
	response := &ngfwv1.RemoteAccessCapabilitiesResponse{Engine: "strongswan-ra", EditableDisabledDrafts: true, SupportedAuth: []string{"eap-mschapv2", "eap-tls", "eap-radius", "pubkey"}}
	rt := subsystems.RARuntimeFor(s.owner)
	if rt == nil || rt.Ready(ctx) != nil {
		response.Reason = "engine-not-ready"
		return response, nil
	}
	response.Operational = true
	return response, nil
}
func (g *server) RemoteAccessSessions(ctx context.Context, req *ngfwv1.RemoteAccessSessionsRequest) (*ngfwv1.RemoteAccessSessionsResponse, error) {
	return g.svc.RemoteAccessSessions(ctx, req)
}
func (s *Service) RemoteAccessSessions(ctx context.Context, req *ngfwv1.RemoteAccessSessionsRequest) (*ngfwv1.RemoteAccessSessionsResponse, error) {
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, status.Error(codes.PermissionDenied, "remote-access owner mismatch")
	}
	if !raProfileName.MatchString(req.GetProfile()) || req.GetLimit() > 100 || (req.GetCursor() != "" && !ravpn.ValidInstance(req.GetCursor())) {
		return nil, status.Error(codes.InvalidArgument, "invalid remote-access page")
	}
	rt := subsystems.RARuntimeFor(s.owner)
	if rt == nil {
		return nil, status.Error(codes.Unavailable, "remote-access engine unavailable")
	}
	rows, e := rt.Sessions(ctx, req.GetProfile())
	if e != nil {
		return nil, status.Error(codes.FailedPrecondition, "verified remote-access observations unavailable")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	start := 0
	if req.GetCursor() != "" {
		found := false
		for i, row := range rows {
			if row.ID == req.GetCursor() {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, status.Error(codes.FailedPrecondition, "remote-access page changed")
		}
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	end := min(len(rows), start+limit)
	response := &ngfwv1.RemoteAccessSessionsResponse{}
	for _, row := range rows[start:end] {
		response.Sessions = append(response.Sessions, &ngfwv1.RemoteAccessSession{Id: row.ID, Profile: row.Profile, Identity: row.Identity, Addresses: row.Addresses, EstablishedSeconds: row.EstablishedSeconds, BytesIn: row.BytesIn, BytesOut: row.BytesOut})
	}
	if end < len(rows) {
		response.NextCursor = rows[end-1].ID
	}
	return response, nil
}
func (g *server) RemoteAccessDisconnect(ctx context.Context, req *ngfwv1.RemoteAccessDisconnectRequest) (*ngfwv1.RemoteAccessDisconnectResponse, error) {
	return g.svc.RemoteAccessDisconnect(ctx, req)
}
func (s *Service) RemoteAccessDisconnect(ctx context.Context, req *ngfwv1.RemoteAccessDisconnectRequest) (*ngfwv1.RemoteAccessDisconnectResponse, error) {
	if e := s.checkOwner(req.GetOwner()); e != nil {
		return nil, status.Error(codes.PermissionDenied, "remote-access owner mismatch")
	}
	if !raProfileName.MatchString(req.GetProfile()) || !ravpn.ValidInstance(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid remote-access session")
	}
	rt := subsystems.RARuntimeFor(s.owner)
	if rt == nil {
		return nil, status.Error(codes.Unavailable, "remote-access engine unavailable")
	}
	if rt.Disconnect(ctx, req.GetProfile(), req.GetId()) != nil {
		return nil, status.Error(codes.FailedPrecondition, "owned session removal was not observed")
	}
	return &ngfwv1.RemoteAccessDisconnectResponse{Disconnected: true}, nil
}

// quiesceRA runs complete read-only planning before any changed engine stops.
func (s *Service) quiesceRA(ctx context.Context, kvs []scheduler.KV, domains []string) ([]ravpn.EngineSpec, *scheduler.TxnResult) {
	managed := false
	for _, domain := range domains {
		if domain == "vpn" {
			managed = true
		}
	}
	if !managed {
		return nil, nil
	}
	runtime := subsystems.RARuntimeFor(s.owner)
	if runtime == nil {
		return nil, nil
	}
	var wanted []ravpn.EngineSpec
	for _, kv := range kvs {
		if kv.Key.Descriptor() != ravpn.EngineName {
			continue
		}
		value, _ := kv.Value.(*structpb.Struct)
		spec, err := ravpn.DecodeEngine(value)
		if err != nil {
			return nil, &scheduler.TxnResult{Outcome: scheduler.OutcomeFailed, Err: ravpn.ErrEngine}
		}
		wanted = append(wanted, spec)
	}
	changed, err := runtime.ChangeRequired(ctx, wanted)
	if err != nil {
		return nil, &scheduler.TxnResult{Outcome: scheduler.OutcomeFailed, Err: ravpn.ErrEngine}
	}
	if !changed {
		return nil, nil
	}
	plan, err := s.sched.Plan(ctx, kvs, scopeOf(domains))
	if err != nil || plan == nil || len(plan.Issues) > 0 {
		return nil, &scheduler.TxnResult{Outcome: scheduler.OutcomeFailed, Plan: plan, Err: ravpn.ErrEngine}
	}
	previous, err := runtime.QuiesceChanged(ctx, wanted)
	if err != nil {
		return previous, &scheduler.TxnResult{Outcome: scheduler.OutcomeDegraded, Plan: plan, Err: ravpn.ErrEngine}
	}
	return previous, nil
}
func (s *Service) restoreRA(ctx context.Context, previous []ravpn.EngineSpec, result *scheduler.TxnResult) {
	if len(previous) == 0 || result.Outcome == scheduler.OutcomeApplied {
		return
	}
	runtime := subsystems.RARuntimeFor(s.owner)
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scheduler.DefaultRollbackTimeout)
	defer cancel()
	if runtime == nil || runtime.Restore(restoreCtx, previous) != nil {
		result.Outcome = scheduler.OutcomeDegraded
		result.Err = errors.Join(result.Err, ravpn.ErrEngine)
	}
}
