package agent

// F-det44-map-dslite-cnat: DET44 read-only state (Det44Sessions, Det44Lookup) and the session close action
// (ActionRequest.det44_session_close, server.go's case under this task's anchor). Everything goes through the det44
// binary API (det44_forward / det44_reverse / det44_session_dump / det44_close_session_in|out); nothing mutates
// configuration. A slot agent answers only for inside users in its own address scope (natcommon.Scope).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/det44"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/natcommon"
)

// Page limits of the DET44 / CNAT session RPCs (proto: 0 = 100, > 1000 INVALID_ARGUMENT).
const (
	cgnatDefaultLimit = 100
	cgnatMaxLimit     = 1000
)

var det44States = []string{"unknown", "udp-active", "tcp-syn-sent", "tcp-established", "tcp-fin-wait", "tcp-close-wait",
	"tcp-closing", "tcp-last-ack", "tcp-closed", "icmp-active"}

// det44StateName is VPP's det44 session state name (foreach_det44_session_state), else the number.
func det44StateName(s uint8) string {
	if int(s) < len(det44States) {
		return det44States[s]
	}
	return strconv.Itoa(int(s))
}

func cgnatLimit(l uint32) (int, error) {
	switch {
	case l == 0:
		return cgnatDefaultLimit, nil
	case l > cgnatMaxLimit:
		return 0, status.Errorf(codes.InvalidArgument, "limit %d > %d", l, cgnatMaxLimit)
	}
	return int(l), nil
}

func parseIP4(what, s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return netip.Addr{}, status.Errorf(codes.InvalidArgument, "%s: %q is not an IPv4 address", what, s)
	}
	return a, nil
}

func port16(what string, p uint32) (uint16, error) {
	if p > 65535 {
		return 0, status.Errorf(codes.InvalidArgument, "%s: %d is not a port", what, p)
	}
	return uint16(p), nil
}

// det44Err maps a det44 API error: NO_SUCH_ENTRY → NOT_FOUND, INVALID_VALUE → INVALID_ARGUMENT.
func det44Err(what string, err error) error {
	if rv, ok := natcommon.Retval(err); ok {
		switch {
		case natcommon.IsNoSuchEntry(err):
			return status.Errorf(codes.NotFound, "%s: no DET44 mapping or session (retval %d)", what, int32(rv))
		case int32(rv) == -7: // VNET_API_ERROR_INVALID_VALUE
			return status.Errorf(codes.InvalidArgument, "%s: invalid value (retval %d)", what, int32(rv))
		}
	}
	return natErr(what, err)
}

func (s *Service) det44Owned(in netip.Addr) error {
	if !natcommon.ScopeFor(s.owner).OwnsAddr(in) {
		return status.Errorf(codes.PermissionDenied, "inside address %s is outside this agent's address scope (owner %q)", in, s.owner)
	}
	return nil
}

// Det44Sessions pages one inside user's DET44 sessions (det44_forward for the port block, then det44_session_dump).
func (s *Service) Det44Sessions(ctx context.Context, req *vrxv1.Det44SessionsRequest) (*vrxv1.Det44SessionsResponse, error) {
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	limit, err := cgnatLimit(req.GetLimit())
	if err != nil {
		return nil, err
	}
	user, err := parseIP4("user", req.GetUser())
	if err != nil {
		return nil, err
	}
	if err := s.det44Owned(user); err != nil {
		return nil, err
	}
	c := det44.NewServiceClient(s.vpp)
	fw, err := c.Det44Forward(ctx, &det44.Det44Forward{InAddr: user.As4()})
	if err != nil {
		return nil, det44Err("det44_forward", err)
	}
	st, err := c.Det44SessionDump(ctx, &det44.Det44SessionDump{UserAddr: user.As4()})
	if err != nil {
		return nil, det44Err("det44_session_dump", err)
	}
	var all []*det44.Det44SessionDetails
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, det44Err("det44_session_dump", err)
		}
		all = append(all, d)
	}
	resp := &vrxv1.Det44SessionsResponse{
		TotalSessions: uint64(len(all)), OutsideAddress: netip.AddrFrom4(fw.OutAddr).String(),
		PortLo: uint32(fw.OutPortLo), PortHi: uint32(fw.OutPortHi), Owner: s.owner, RetrievedAt: timestamppb.New(s.now()),
	}
	off := int(req.GetOffset())
	for i := off; i < len(all) && i < off+limit; i++ {
		d := all[i]
		resp.Sessions = append(resp.Sessions, &vrxv1.Det44Session{
			InsidePort: uint32(d.InPort), OutsidePort: uint32(d.OutPort), ExternalAddress: netip.AddrFrom4(d.ExtAddr).String(),
			ExternalPort: uint32(d.ExtPort), State: det44StateName(d.State), Expire: d.Expire,
		})
	}
	if end := off + limit; end < len(all) {
		n := uint32(end) //nolint:gosec // bounded by the dump size
		resp.NextOffset = &n
	}
	return resp, nil
}

// Det44Lookup is the deterministic mapping in either direction (CGNAT logging): forward inside → outside address
// and port block, reverse outside address + port → inside address.
func (s *Service) Det44Lookup(ctx context.Context, req *vrxv1.Det44LookupRequest) (*vrxv1.Det44LookupResponse, error) {
	if err := s.natReady(req.GetOwner()); err != nil {
		return nil, err
	}
	c := det44.NewServiceClient(s.vpp)
	switch {
	case req.InsideAddress != nil && req.OutsideAddress == nil:
		in, err := parseIP4("inside_address", req.GetInsideAddress())
		if err != nil {
			return nil, err
		}
		if err := s.det44Owned(in); err != nil {
			return nil, err
		}
		fw, err := c.Det44Forward(ctx, &det44.Det44Forward{InAddr: in.As4()})
		if err != nil {
			return nil, det44Err("det44_forward", err)
		}
		return &vrxv1.Det44LookupResponse{InsideAddress: in.String(), OutsideAddress: netip.AddrFrom4(fw.OutAddr).String(),
			PortLo: uint32(fw.OutPortLo), PortHi: uint32(fw.OutPortHi)}, nil
	case req.OutsideAddress != nil && req.InsideAddress == nil:
		out, err := parseIP4("outside_address", req.GetOutsideAddress())
		if err != nil {
			return nil, err
		}
		if req.OutsidePort == nil {
			return nil, status.Error(codes.InvalidArgument, "outside_port is required with outside_address")
		}
		p, err := port16("outside_port", req.GetOutsidePort())
		if err != nil {
			return nil, err
		}
		rv, err := c.Det44Reverse(ctx, &det44.Det44Reverse{OutAddr: out.As4(), OutPort: p})
		if err != nil {
			return nil, det44Err("det44_reverse", err)
		}
		in := netip.AddrFrom4(rv.InAddr)
		if err := s.det44Owned(in); err != nil {
			return nil, err
		}
		return &vrxv1.Det44LookupResponse{InsideAddress: in.String(), OutsideAddress: out.String()}, nil
	}
	return nil, status.Error(codes.InvalidArgument, "set exactly one of inside_address or outside_address")
}

// det44SessionClose closes one session; exit_code 0 closed, 1 no such session.
func (s *Service) det44SessionClose(ctx context.Context, a *vrxv1.Det44SessionCloseAction, send func(*vrxv1.ActionOutput) error) error {
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	addr, err := parseIP4("address", a.GetAddress())
	if err != nil {
		return err
	}
	ext, err := parseIP4("external_address", a.GetExternalAddress())
	if err != nil {
		return err
	}
	p, err := port16("port", a.GetPort())
	if err != nil {
		return err
	}
	ep, err := port16("external_port", a.GetExternalPort())
	if err != nil {
		return err
	}
	c := det44.NewServiceClient(s.vpp)
	switch a.GetDirection() {
	case "in":
		if err := s.det44Owned(addr); err != nil {
			return err
		}
		_, err = c.Det44CloseSessionIn(ctx, &det44.Det44CloseSessionIn{InAddr: addr.As4(), InPort: p, ExtAddr: ext.As4(), ExtPort: ep})
	case "out":
		rv, rerr := c.Det44Reverse(ctx, &det44.Det44Reverse{OutAddr: addr.As4(), OutPort: p})
		if rerr != nil {
			// no mapping (or an invalid port): NotFound without echoing the endpoint, so the answer leaks nothing
			// about which outside endpoints exist
			return status.Error(codes.NotFound, "no DET44 session")
		}
		if oerr := s.det44Owned(netip.AddrFrom4(rv.InAddr)); oerr != nil {
			return oerr
		}
		_, err = c.Det44CloseSessionOut(ctx, &det44.Det44CloseSessionOut{OutAddr: addr.As4(), OutPort: p, ExtAddr: ext.As4(), ExtPort: ep})
	default:
		return status.Errorf(codes.InvalidArgument, "direction %q is not in or out", a.GetDirection())
	}
	code, summary := 0, fmt.Sprintf("closed DET44 session %s %s:%d ↔ %s:%d", a.GetDirection(), addr, p, ext, ep)
	switch {
	case err == nil:
	case natcommon.IsNoSuchEntry(err):
		code, summary = 1, fmt.Sprintf("no DET44 session %s %s:%d ↔ %s:%d", a.GetDirection(), addr, p, ext, ep)
	default:
		return det44Err("det44 session close", err)
	}
	s.log.Info("det44 session close", "direction", a.GetDirection(), "address", addr.String(), "port", p, "external", ext.String(), "external_port", ep, "exit_code", code)
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: summary, ExitCode: int32(code)}}}) //nolint:gosec // 0–1
}

func (g *server) Det44Sessions(ctx context.Context, req *vrxv1.Det44SessionsRequest) (*vrxv1.Det44SessionsResponse, error) {
	return g.svc.Det44Sessions(ctx, req)
}

func (g *server) Det44Lookup(ctx context.Context, req *vrxv1.Det44LookupRequest) (*vrxv1.Det44LookupResponse, error) {
	return g.svc.Det44Lookup(ctx, req)
}

// det44SessionClose adapts the Action stream (server.go's case under the F-det44-map-dslite-cnat anchor).
func (g *server) det44SessionClose(req *vrxv1.ActionRequest, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	return g.svc.det44SessionClose(stream.Context(), req.GetDet44SessionClose(), stream.Send)
}
