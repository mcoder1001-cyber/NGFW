package agent

// F-nat44-ei-64-66-nptv6: the NAT44-EI and NAT64 variants of the NatSessions RPC and the NAT44-EI variant of the
// NatSessionKillAction (docs/contracts/proto.md §11 "NAT session variants"). F-nat44-ed-sessions' handlers dispatch
// here on NatSessionVariant (one hunk each in rpc_nat44_ed.go and server.go); the logic lives in
// internal/actions/nat44-ei-64-66-nptv6. The DF-3 helpers are built per call: they only read state (or delete one
// session), so they need neither the claim store nor the globals flag.

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	binnat64 "ngfw/agent/binapi/nat64"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	natsessions "ngfw/agent/internal/actions/nat44-ed-sessions"
	natvariants "ngfw/agent/internal/actions/nat44-ei-64-66-nptv6"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/vpp"
)

// natVariantOf reports whether a variant is served by this file: EI, NAT64 and any value this build does not know
// (answered INVALID_ARGUMENT here, review L3); unset and ED stay F-nat44-ed-sessions'.
func natVariantOf(v vrxv1.NatSessionVariant) bool {
	return v != vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_UNSPECIFIED && v != vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_ED
}

// natSessionsVariant serves NatSessions for the EI and NAT64 tables. The walk takes F-nat44-ed-sessions' per-agent walk
// slot (s.natWalk, D-132: one VPP walk at a time in the agent, ED's and these together; a caller whose deadline passes
// while another walk runs gives up).
//
// Cost note (review M3, follow-up): a NAT64 page is one nat64_st_dump of the whole session table (DF-3's
// nat64.Sessions materialises every owner's rows; only offset+limit rows are kept) plus, when the page has rows, one
// nat64_bib_dump of all protocols for the port correction. Streaming the ST dump and dumping the BIB only for the
// page's protocols is the follow-up.
func (s *Service) natSessionsVariant(ctx context.Context, req *vrxv1.NatSessionsRequest) (*vrxv1.NatSessionsResponse, error) {
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	switch req.GetVariant() {
	case vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_EI, vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_NAT64:
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown NAT session variant %d", req.GetVariant())
	}
	release, err := s.natWalk(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	limit := int(req.GetLimit())
	switch {
	case limit == 0:
		limit = natsessions.DefaultLimit
	case limit > natsessions.MaxLimit:
		return nil, status.Errorf(codes.InvalidArgument, "limit %d > %d", limit, natsessions.MaxLimit)
	}
	if req.GetVariant() == vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_NAT64 {
		return s.natSessionsNat64(ctx, req, limit)
	}
	f, err := s.natFilter(req.GetFilter())
	if err != nil {
		return nil, natErr("filter", err)
	}
	page, err := natvariants.ListEI(ctx, nat44ei.New(s.vpp, s.owner), natcommon.ScopeFor(s.owner), f, int(req.GetOffset()), limit, natCaps)
	if err != nil {
		return nil, natErr("nat44-ei sessions", err)
	}
	resp := &vrxv1.NatSessionsResponse{
		NextOffset: page.Next, TotalUsers: page.TotalUsers, TotalSessions: page.TotalSessions, Truncated: page.Truncated,
		Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), Variant: vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_EI.Enum(),
	}
	for _, r := range page.Rows {
		resp.Sessions = append(resp.Sessions, &vrxv1.NatSession{
			InsideAddress: r.Inside.IP, InsidePort: r.Inside.Port, OutsideAddress: r.Outside.IP, OutsidePort: r.Outside.Port,
			ExternalAddress: r.ExtHost.IP, ExternalPort: r.ExtHost.Port, ExternalNatAddress: r.ExtHostNAT.IP, ExternalNatPort: r.ExtHostNAT.Port,
			Protocol: r.Protocol, Vrf: s.tableName(r.VRF), TableId: r.VRF, Static: r.Static,
			IdleSeconds: r.IdleSeconds, Bytes: r.TotalBytes, Packets: uint64(r.TotalPkts),
		})
	}
	return resp, nil
}

// natSessionsNat64 pages the NAT64 session table; only filter.protocol applies (the other filter fields are IPv4
// NAT44 fields).
func (s *Service) natSessionsNat64(ctx context.Context, req *vrxv1.NatSessionsRequest, limit int) (*vrxv1.NatSessionsResponse, error) {
	f := req.GetFilter()
	if f == nil {
		f = &vrxv1.NatSessionFilter{}
	}
	if f.InsideAddress != nil || f.OutsideAddress != nil || f.ExternalAddress != nil || f.Port != nil || f.Vrf != nil {
		return nil, status.Error(codes.InvalidArgument, "NAT64 sessions accept only filter.protocol")
	}
	proto := ""
	if f.Protocol != nil {
		p, err := natsessions.ParseProtocol(f.GetProtocol())
		if err != nil {
			return nil, natErr("filter", err)
		}
		proto = p
	}
	page, err := natvariants.ListNat64(ctx, nat64.New(s.vpp, s.owner), nat64BIB{s.vpp}, natcommon.ScopeFor(s.owner), proto, int(req.GetOffset()), limit, natCaps.ScanCap)
	if err != nil {
		return nil, natErr("nat64 sessions", err)
	}
	resp := &vrxv1.NatSessionsResponse{
		NextOffset: page.Next, TotalUsers: page.TotalUsers, TotalSessions: page.TotalSessions, Truncated: page.Truncated,
		Owner: s.owner, RetrievedAt: timestamppb.New(s.now()), Variant: vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_NAT64.Enum(),
	}
	for _, r := range page.Rows {
		resp.Sessions = append(resp.Sessions, &vrxv1.NatSession{
			InsideAddress: r.InsideLocal, InsidePort: r.InsidePort, OutsideAddress: r.OutsideLocal, OutsidePort: r.OutsidePort,
			ExternalAddress: r.OutsideRemote, ExternalPort: r.RemotePort, ExternalNatAddress: r.InsideRemote, ExternalNatPort: r.RemotePort,
			Protocol: r.Protocol, Vrf: s.tableName(r.VRF), TableId: r.VRF,
		})
	}
	return resp, nil
}

// nat64BIB reads every BIB entry (nat64_bib_dump, all protocols) for the NAT64 port correction (natvariants.BIBSource).
type nat64BIB struct{ c vpp.Client }

func (b nat64BIB) InsidePorts(ctx context.Context) (map[natvariants.BIBKey]uint32, error) {
	stream, err := binnat64.NewServiceClient(b.c).Nat64BibDump(ctx, &binnat64.Nat64BibDump{Proto: 255})
	if err != nil {
		return nil, fmt.Errorf("nat64_bib_dump: %w", err)
	}
	out := map[natvariants.BIBKey]uint32{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("nat64_bib_dump: %w", err)
		}
		out[natvariants.BIBKey{Protocol: natcommon.ProtoName(d.Proto), Outside: natcommon.IP4String(d.OAddr), Port: uint32(d.OPort)}] = uint32(d.IPort)
	}
}

// natSessionKillVariant runs the EI kill (NAT64 has no session delete in VPP 26.06: INVALID_ARGUMENT). Same stream
// contract as the ED kill: validation errors before any output, then exactly one `done`.
func (s *Service) natSessionKillVariant(ctx context.Context, a *vrxv1.NatSessionKillAction, send func(*vrxv1.ActionOutput) error) error {
	switch a.GetVariant() {
	case vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_EI:
	case vrxv1.NatSessionVariant_NAT_SESSION_VARIANT_NAT64:
		return status.Error(codes.InvalidArgument, "NAT64 sessions cannot be deleted: VPP 26.06 has no NAT64 session delete")
	default:
		return status.Errorf(codes.InvalidArgument, "unknown NAT session variant %d", a.GetVariant())
	}
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	k, err := natvariants.ParseKillEI(natcommon.ScopeFor(s.owner), a.GetProtocol(), a.GetInsideAddress(), a.GetInsidePort(),
		a.GetExternalAddress(), a.GetExternalPort(), a.GetVrf(), s.resolveVRF)
	if err != nil {
		return natErr("nat44-ei session kill", err)
	}
	code, summary := k.Do(ctx, nat44ei.New(s.vpp, s.owner))
	s.log.Info("nat session kill", "variant", "ei", "protocol", k.Protocol, "inside", k.Inside.String(), "table", k.Table, "exit_code", code, "summary", summary)
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: summary, ExitCode: int32(code), Stats: k.Stats()}}}) //nolint:gosec // 0–2
}

// natSessionKillVariantStream adapts the Action stream (the variant dispatch in server.go's nat_session_kill case).
func (g *server) natSessionKillVariantStream(req *vrxv1.ActionRequest, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	return g.svc.natSessionKillVariant(stream.Context(), req.GetNatSessionKill(), stream.Send)
}
