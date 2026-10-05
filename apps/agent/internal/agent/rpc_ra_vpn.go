package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	ravpn "ngfw/agent/internal/ra_vpn"
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
		return nil, e
	}
	response := &ngfwv1.RemoteAccessCapabilitiesResponse{Engine: "strongswan-ra", EditableDisabledDrafts: true, SupportedAuth: []string{"eap-mschapv2", "eap-tls", "eap-radius", "pubkey"}}
	rt := subsystems.RARuntimeFor(s.owner)
	if rt == nil || rt.Ready(ctx) != nil {
		response.Reason = "independent engine installation or owned transport is unavailable"
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
		return nil, e
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
		return nil, e
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
