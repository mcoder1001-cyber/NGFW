package agent

// F-unbound-chrony-syslog: the read-only DNS state RPC and the dns_lookup action (wave-A-hotspots A4: state RPCs in
// rpc_<slug>.go, one Action case in server.go).

import (
	"context"
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	ucsaction "ngfw/agent/internal/actions/unbound-chrony-syslog"
	"ngfw/agent/internal/subsystems"
)

// hostServices returns this agent's host-service descriptors after the owner check.
func (g *server) hostServices(owner string) (*subsystems.HostServices, error) {
	if err := g.svc.checkOwner(owner); err != nil {
		return nil, err
	}
	hs := subsystems.HostServicesOf(g.svc.owner)
	if hs == nil {
		return nil, status.Error(codes.Unavailable, "host services (unbound, chrony, rsyslog) are not wired in this agent")
	}
	return hs, nil
}

// storedServices returns a copy of the stored (applied) desired services and management domains.
func (g *server) storedServices(ctx context.Context) (*vrxv1.ServicesConfig, *vrxv1.ManagementConfig, error) {
	if err := g.svc.lock(ctx); err != nil {
		return nil, nil, err
	}
	defer g.svc.unlock()
	ds := g.svc.st.desired
	svc, _ := proto.Clone(ds.GetServices()).(*vrxv1.ServicesConfig)
	mgmt, _ := proto.Clone(ds.GetManagement()).(*vrxv1.ManagementConfig)
	return svc, mgmt, nil
}

// DnsState implements the DnsState RPC (the name is fixed by the generated DataplaneServer interface).
func (g *server) DnsState(ctx context.Context, req *vrxv1.DnsStateRequest) (*vrxv1.DnsStateResponse, error) { //nolint:revive // generated interface name
	hs, err := g.hostServices(req.GetOwner())
	if err != nil {
		return nil, err
	}
	r := hs.Unbound.Renderer()
	resp := &vrxv1.DnsStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now()), ConfigPath: r.Paths().Conf()}
	st, err := r.State(ctx)
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Running, resp.Status, resp.Stats = st.Running, st.Status, st.Stats
	for _, z := range st.Forwards {
		resp.Forwards = append(resp.Forwards, &vrxv1.DnsZoneState{Zone: z.Zone, Kind: z.Kind, Flags: z.Flags, Addresses: z.Addrs})
	}
	for _, z := range st.Stubs {
		resp.Stubs = append(resp.Stubs, &vrxv1.DnsZoneState{Zone: z.Zone, Kind: z.Kind, Flags: z.Flags, Addresses: z.Addrs})
	}
	for _, z := range st.LocalZones {
		resp.LocalZones = append(resp.LocalZones, &vrxv1.DnsLocalZoneState{Zone: z.Zone, Type: z.Type})
	}
	resp.LocalData, resp.LocalDataTruncated = st.LocalData, st.LocalDataTruncated
	for _, p := range hs.Unbound.Pending(ctx) {
		resp.PendingActions = append(resp.PendingActions, &vrxv1.ServiceDaemonAction{Daemon: p.Daemon, Unit: p.Unit, Action: p.Action, Reason: p.Reason})
	}
	svc, _, err := g.storedServices(ctx)
	if err != nil {
		return nil, err
	}
	vc := svc.GetDns().GetVppCache()
	resp.VppCache = &vrxv1.DnsVppCacheState{Configured: vc.GetEnabled(), AppliedByThisAgent: vc.GetEnabled() && hs.GlobalsOwner, Upstreams: append([]string(nil), vc.GetUpstreams()...)}
	sort.Strings(resp.VppCache.Upstreams)
	return resp, nil
}

// dnsLookup runs the dns_lookup action (ActionRequest 7) — only where this agent enabled VPP's DNS cache itself: the
// globals owner whose applied configuration enables it with an upstream (VPP 26.06 crashes on dns_resolve_name
// otherwise; ucsaction.Lookup).
func (g *server) dnsLookup(a *vrxv1.DnsLookupAction, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	ctx := stream.Context()
	hs, err := g.hostServices("")
	if err != nil {
		return err
	}
	svc, _, err := g.storedServices(ctx)
	if err != nil {
		return err
	}
	vc := svc.GetDns().GetVppCache()
	ready := hs.GlobalsOwner && vc.GetEnabled() && len(vc.GetUpstreams()) > 0
	g.log.Info("action dns_lookup", "name", a.GetName(), "timeout_ms", a.GetTimeoutMs(), "vpp_cache_ready", ready)
	return ucsaction.Lookup(ctx, g.svc.vpp, a, ready, stream.Send)
}
