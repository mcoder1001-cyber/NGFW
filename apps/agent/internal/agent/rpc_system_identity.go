package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/subsystems"
)

// SystemIdentityState observes installed files without changing host identity,
// privileges, or the pending systemd-resolved restart boundary.
func (g *server) SystemIdentityState(_ context.Context, req *vrxv1.SystemIdentityStateRequest) (*vrxv1.SystemIdentityStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	d := subsystems.SystemIdentityOf(g.svc.owner)
	if d == nil {
		return nil, status.Error(codes.Unavailable, "system identity is not wired")
	}
	st := d.State("/proc", "/run/systemd/resolve/resolv.conf")
	return &vrxv1.SystemIdentityStateResponse{
		Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now()), Hostname: st.Hostname, Timezone: st.Timezone,
		UptimeSeconds: st.UptimeSeconds, KernelHostname: st.KernelHostname, ConfiguredNameServers: st.ConfiguredNameServers,
		ConfiguredSearchDomains: st.ConfiguredSearchDomains, ResolverStatus: st.ResolverStatus, ObservedNameServers: st.ObservedNameServers, Errors: st.Errors,
	}, nil
}
