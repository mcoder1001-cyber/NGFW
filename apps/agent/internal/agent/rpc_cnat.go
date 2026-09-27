package agent

// F-det44-map-dslite-cnat: CNAT read-only session state (CnatSessions) and the purge action
// (ActionRequest.cnat_session_purge, server.go's case under this task's anchor). The CNAT session table is a VPP
// global with no owner tag: every agent may read it, only the globals owner (D-071) may purge it.

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	cnatapi "ngfw/agent/binapi/cnat"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/natcommon"
)

// cnatSessionCap bounds one cnat_session_dump walk (the dump has no cursor): past it the response is truncated.
var cnatSessionCap = 200_000

// CnatSessions pages the CNAT session table in VPP's dump order.
func (s *Service) CnatSessions(ctx context.Context, req *vrxv1.CnatSessionsRequest) (*vrxv1.CnatSessionsResponse, error) {
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	limit, err := cgnatLimit(req.GetLimit())
	if err != nil {
		return nil, err
	}
	st, err := cnatapi.NewServiceClient(s.vpp).CnatSessionDump(ctx, &cnatapi.CnatSessionDump{})
	if err != nil {
		return nil, natErr("cnat_session_dump", err)
	}
	resp := &vrxv1.CnatSessionsResponse{Owner: s.owner}
	off := int(req.GetOffset())
	n := 0
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, natErr("cnat_session_dump", err)
		}
		if n >= cnatSessionCap {
			resp.Truncated = true
			continue // drain the stream; the page and the total stop at the cap
		}
		if n >= off && n < off+limit {
			t := d.Session.Tuple
			row := &vrxv1.CnatSession{
				DstAddress: t.Addr[0].String(), SrcAddress: t.Addr[1].String(),
				Protocol: natcommon.ProtoName(uint8(t.IPProto)), TranslationIndex: d.Session.TsIndex, Flags: d.Session.Flags,
			}
			if len(t.Port) == 2 {
				row.DstPort, row.SrcPort = uint32(t.Port[0]), uint32(t.Port[1])
			}
			resp.Sessions = append(resp.Sessions, row)
		}
		n++
	}
	resp.TotalSessions = uint64(n) //nolint:gosec // n ≥ 0
	if end := off + limit; end < n {
		next := uint32(end) //nolint:gosec // bounded by the cap
		resp.NextOffset = &next
	}
	resp.RetrievedAt = timestamppb.New(s.now())
	return resp, nil
}

// cnatSessionPurge purges the CNAT session table (globals owner only).
func (s *Service) cnatSessionPurge(ctx context.Context, send func(*vrxv1.ActionOutput) error) error {
	if !captureGlobalsOwner(s.owner) {
		return status.Errorf(codes.PermissionDenied, "the CNAT session table is a VPP global: only the globals owner may purge it (D-071; owner %q)", s.owner)
	}
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	if _, err := cnatapi.NewServiceClient(s.vpp).CnatSessionPurge(ctx, &cnatapi.CnatSessionPurge{}); err != nil {
		return natErr("cnat_session_purge", err)
	}
	s.log.Info("cnat session purge")
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: "purged the CNAT session table"}}})
}

func (g *server) CnatSessions(ctx context.Context, req *vrxv1.CnatSessionsRequest) (*vrxv1.CnatSessionsResponse, error) {
	return g.svc.CnatSessions(ctx, req)
}

// cnatSessionPurge adapts the Action stream (server.go's case under the F-det44-map-dslite-cnat anchor).
func (g *server) cnatSessionPurge(_ *vrxv1.ActionRequest, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	return g.svc.cnatSessionPurge(stream.Context(), stream.Send)
}
