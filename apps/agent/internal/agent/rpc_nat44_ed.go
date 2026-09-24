package agent

// F-nat44-ed-sessions: the NatSessions / NatSummary RPCs and the NatSessionKillAction case of Action
// (docs/contracts/proto.md §11). Read-only except the kill; the logic lives in internal/actions/nat44-ed-sessions,
// this file translates vrx.v1 ↔ the pure functions and reads VPP through DF-3's nat44ed helpers. The helpers are
// built per call (nat44ed.New): they read state only, so they need neither the claim store nor the globals flag.

import (
	"context"
	"errors"
	"net/netip"
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	natsessions "ngfw/agent/internal/actions/nat44-ed-sessions"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// natScanCap bounds the sessions one filtered NatSessions or one NatSummary looks at (tests lower it).
var natScanCap = natsessions.DefaultScanCap

func (g *server) NatSessions(ctx context.Context, req *vrxv1.NatSessionsRequest) (*vrxv1.NatSessionsResponse, error) {
	return g.svc.NatSessions(ctx, req)
}

func (g *server) NatSummary(ctx context.Context, req *vrxv1.NatSummaryRequest) (*vrxv1.NatSummaryResponse, error) {
	return g.svc.NatSummary(ctx, req)
}

// natErr maps a helper error to a gRPC status.
func natErr(what string, err error) error {
	switch {
	case errors.Is(err, natsessions.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

func (s *Service) natReady(owner string) error {
	if err := s.checkOwner(owner); err != nil {
		return err
	}
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	return nil
}

// natFilter parses the request filter (INVALID_ARGUMENT on a bad field).
func (s *Service) natFilter(f *vrxv1.NatSessionFilter) (natsessions.Filter, error) {
	var out natsessions.Filter
	var err error
	if f == nil {
		return out, nil
	}
	if f.InsideAddress != nil {
		if out.Inside, err = natsessions.ParseIPv4("filter.inside_address", f.GetInsideAddress()); err != nil {
			return out, err
		}
	}
	if f.OutsideAddress != nil {
		if out.Outside, err = natsessions.ParseIPv4("filter.outside_address", f.GetOutsideAddress()); err != nil {
			return out, err
		}
	}
	if f.ExternalAddress != nil {
		if out.External, err = natsessions.ParseIPv4("filter.external_address", f.GetExternalAddress()); err != nil {
			return out, err
		}
	}
	if f.Port != nil {
		p := f.GetPort()
		out.Port = &p
	}
	if f.Protocol != nil {
		if out.Protocol, err = natsessions.ParseProtocol(f.GetProtocol()); err != nil {
			return out, err
		}
	}
	if f.Vrf != nil {
		id, err := natsessions.ParseVRF(f.GetVrf(), s.resolveVRF)
		if err != nil {
			return out, err
		}
		out.VRF = &id
	}
	return out, nil
}

// NatSessions implements the NatSessions RPC: one bounded page of this owner's sessions.
func (s *Service) NatSessions(ctx context.Context, req *vrxv1.NatSessionsRequest) (*vrxv1.NatSessionsResponse, error) {
	// wave-A: F-nat44-ei-64-66-nptv6 — variant dispatch (EI, NAT64: rpc_nat44_ei.go); unset / ED below
	if natVariantOf(req.GetVariant()) {
		return s.natSessionsVariant(ctx, req)
	}
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	switch {
	case limit == 0:
		limit = natsessions.DefaultLimit
	case limit > natsessions.MaxLimit:
		return nil, status.Errorf(codes.InvalidArgument, "limit %d > %d", limit, natsessions.MaxLimit)
	}
	f, err := s.natFilter(req.GetFilter())
	if err != nil {
		return nil, natErr("filter", err)
	}
	page, err := natsessions.List(ctx, nat44ed.New(s.vpp, s.owner), natcommon.ScopeFor(s.owner), f, int(req.GetOffset()), limit, natScanCap)
	if err != nil {
		return nil, natErr("nat sessions", err)
	}
	resp := &vrxv1.NatSessionsResponse{
		NextOffset: page.Next, TotalUsers: page.TotalUsers, TotalSessions: page.TotalSessions, Truncated: page.Truncated,
		Owner: s.owner, RetrievedAt: timestamppb.New(s.now()),
	}
	for _, r := range page.Rows {
		resp.Sessions = append(resp.Sessions, &vrxv1.NatSession{
			InsideAddress: r.Inside.IP, InsidePort: r.Inside.Port, OutsideAddress: r.Outside.IP, OutsidePort: r.Outside.Port,
			ExternalAddress: r.ExtHost.IP, ExternalPort: r.ExtHost.Port, ExternalNatAddress: r.ExtHostNAT.IP, ExternalNatPort: r.ExtHostNAT.Port,
			Protocol: r.Protocol, Vrf: s.tableName(r.VRF), TableId: r.VRF, Static: r.Static, TwiceNat: r.TwiceNAT, TimedOut: r.TimedOut,
			IdleSeconds: r.IdleSeconds, Bytes: r.TotalBytes, Packets: uint64(r.TotalPkts),
		})
	}
	return resp, nil
}

// natPools lists this owner's pools from its own Retrieve (the persisted claims decide what is owned), with the
// current addresses of interface pools from the live interface table.
func (s *Service) natPools(ctx context.Context) ([]natsessions.Pool, error) {
	r, err := s.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"nat"}, Owner: s.owner})
	if err != nil {
		return nil, err
	}
	var tbl ifTable
	var out []natsessions.Pool
	for _, p := range r.GetDesiredState().GetNat().GetPools() {
		if p.Interface == nil {
			first, last, err := desired.ParseNatRange(p.GetRange())
			if err != nil {
				continue
			}
			vrf := p.GetVrf()
			if vrf == "" {
				vrf = "default"
			}
			out = append(out, natsessions.Pool{First: first, Last: last, VRF: vrf, TwiceNAT: p.GetTwiceNat()})
			continue
		}
		if tbl == nil {
			if tbl, err = s.interfaceTable(ctx); err != nil {
				return nil, err
			}
		}
		pool := natsessions.Pool{Interface: p.GetInterface(), TwiceNAT: p.GetTwiceNat()}
		if st, ok := tbl.State(p.GetInterface()); ok {
			for _, a := range st.GetIpv4() {
				if pfx, err := netip.ParsePrefix(a); err == nil {
					pool.Addrs = append(pool.Addrs, pfx.Addr())
				}
			}
			sort.Slice(pool.Addrs, func(a, b int) bool { return pool.Addrs[a].Less(pool.Addrs[b]) })
			if len(pool.Addrs) > 0 {
				pool.First, pool.Last = pool.Addrs[0], pool.Addrs[len(pool.Addrs)-1]
			}
		}
		out = append(out, pool)
	}
	return out, nil
}

// NatSummary implements the NatSummary RPC.
func (s *Service) NatSummary(ctx context.Context, req *vrxv1.NatSummaryRequest) (*vrxv1.NatSummaryResponse, error) {
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	enabled, limit, err := natsessions.RunningConfig(ctx, s.vpp)
	if err != nil {
		return nil, natErr("nat summary", err)
	}
	resp := &vrxv1.NatSummaryResponse{Enabled: enabled, SessionLimit: limit, Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), SessionsByProtocol: map[string]uint64{}}
	if !enabled {
		return resp, nil // nothing to count; the plugin holds no pool either
	}
	pools, err := s.natPools(ctx)
	if err != nil {
		return nil, natErr("nat pools", err)
	}
	sum, err := natsessions.Summarize(ctx, nat44ed.New(s.vpp, s.owner), natcommon.ScopeFor(s.owner), pools, natScanCap)
	if err != nil {
		return nil, natErr("nat summary", err)
	}
	resp.TotalUsers, resp.TotalSessions, resp.StaticSessions, resp.Truncated = sum.TotalUsers, sum.TotalSessions, sum.StaticSessions, sum.Truncated
	resp.SessionsByProtocol = sum.ByProtocol
	for i, p := range pools {
		u := &vrxv1.NatPoolUsage{Interface: p.Interface, Vrf: p.VRF, TwiceNat: p.TwiceNAT, Addresses: p.Size(), Sessions: sum.PoolSessions[i]}
		if p.First.IsValid() {
			u.FirstAddress, u.LastAddress = p.First.String(), p.Last.String()
		}
		resp.Pools = append(resp.Pools, u)
	}
	return resp, nil
}

// natSessionKill runs a NatSessionKillAction: validation errors are INVALID_ARGUMENT before any output; the stream
// is exactly one `done` (exit 0 deleted, 1 no such session, 2 VPP error).
func (s *Service) natSessionKill(ctx context.Context, a *vrxv1.NatSessionKillAction, send func(*vrxv1.ActionOutput) error) error {
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	k, err := natsessions.ParseKill(natcommon.ScopeFor(s.owner), a.GetProtocol(), a.GetInsideAddress(), a.GetInsidePort(),
		a.GetExternalAddress(), a.GetExternalPort(), a.GetVrf(), s.resolveVRF)
	if err != nil {
		return natErr("nat session kill", err)
	}
	code, summary := k.Do(ctx, nat44ed.New(s.vpp, s.owner))
	s.log.Info("nat session kill", "protocol", k.Protocol, "inside", k.Inside.String(), "external", k.External.String(), "table", k.Table, "exit_code", code, "summary", summary)
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: summary, ExitCode: int32(code), Stats: k.Stats()}}}) //nolint:gosec // 0–2
}

// natSessionKillStream adapts the Action stream (server.go's case under the F-nat44-ed-sessions anchor).
func (g *server) natSessionKill(req *vrxv1.ActionRequest, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	return g.svc.natSessionKill(stream.Context(), req.GetNatSessionKill(), stream.Send)
}
