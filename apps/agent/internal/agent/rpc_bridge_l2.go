package agent

// F-bridge-l2 live-state RPCs (docs/contracts/proto.md §11 "F-bridge-l2"): read-only, built from dumps
// only, like InterfaceState (P08). BridgeDomainState lists this agent's bridge domains (bd_tag
// "<owner>:<id>[/<name>]") with members, BVI, flags and L2 FIB counts; BridgeDomainMacs pages through one
// bridge domain's L2 FIB (≤ 1000 entries per message).

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	l2api "ngfw/agent/binapi/l2"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// Page size bounds of BridgeDomainMacs.
const (
	bridgeMacsDefaultLimit = 100
	bridgeMacsMaxLimit     = 1000
)

// BridgeDomainState implements the BridgeDomainState RPC.
func (g *server) BridgeDomainState(ctx context.Context, req *vrxv1.BridgeDomainStateRequest) (*vrxv1.BridgeDomainStateResponse, error) {
	return g.svc.BridgeDomainState(ctx, req)
}

// BridgeDomainMacs implements the BridgeDomainMacs RPC.
func (g *server) BridgeDomainMacs(ctx context.Context, req *vrxv1.BridgeDomainMacsRequest) (*vrxv1.BridgeDomainMacsResponse, error) {
	return g.svc.BridgeDomainMacs(ctx, req)
}

func grpcVPPError(err error, what string) error {
	if errors.Is(err, vpp.ErrDisconnected) {
		return status.Error(codes.Unavailable, err.Error())
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

// ifName is the logical name of idx (our tag id, VPP's name when untagged; another owner's
// interface in our bridge shows VPP's name).
func ifName(t *iface.Table, idx uint32) string {
	if n, ok := t.Logical(idx); ok {
		return n
	}
	if d, ok := t.Details(idx); ok {
		return strings.TrimRight(d.InterfaceName, "\x00")
	}
	return ""
}

func (s *Service) fibOf(ctx context.Context, bd uint32) ([]*l2api.L2FibTableDetails, error) {
	stream, err := l2api.NewServiceClient(s.vpp).L2FibTableDump(ctx, &l2api.L2FibTableDump{BdID: bd})
	if err != nil {
		return nil, err
	}
	var out []*l2api.L2FibTableDetails
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if e.BdID == bd {
			out = append(out, e)
		}
	}
}

// fibCounts streams one bridge domain's L2 FIB and counts static/filter/BVI vs learned entries without buffering it.
func (s *Service) fibCounts(ctx context.Context, bd uint32) (static, learned uint32, err error) {
	stream, err := l2api.NewServiceClient(s.vpp).L2FibTableDump(ctx, &l2api.L2FibTableDump{BdID: bd})
	if err != nil {
		return 0, 0, err
	}
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return static, learned, nil
		}
		if err != nil {
			return 0, 0, err
		}
		switch {
		case e.BdID != bd:
		case e.StaticMac || e.FilterMac || e.BviMac:
			static++
		default:
			learned++
		}
	}
}

// BridgeDomainState implements the RPC.
func (s *Service) BridgeDomainState(ctx context.Context, req *vrxv1.BridgeDomainStateRequest) (*vrxv1.BridgeDomainStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	bds, err := l2.OwnedBridgeDomains(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, grpcVPPError(err, "bridge-domain state")
	}
	t, err := iface.Dump(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, grpcVPPError(err, "bridge-domain state")
	}
	want := map[uint32]bool{}
	for _, id := range req.GetIds() {
		want[id] = true
	}
	resp := &vrxv1.BridgeDomainStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	for _, bd := range bds {
		if len(want) > 0 && !want[bd.BdID] {
			continue
		}
		name, _ := l2.ParseBDTag(bd.BdTag, s.owner, bd.BdID)
		st := &vrxv1.BridgeDomainStatus{
			Id: bd.BdID, Name: name, Flood: bd.Flood, UuFlood: bd.UuFlood, Forward: bd.Forward, Learn: bd.Learn,
			ArpTerm: bd.ArpTerm, ArpUfwd: bd.ArpUfwd, MacAgeMin: uint32(bd.MacAge),
		}
		bvi, uu := uint32(bd.BviSwIfIndex), uint32(bd.UuFwdSwIfIndex)
		for _, sw := range bd.SwIfDetails {
			idx := uint32(sw.SwIfIndex)
			m := &vrxv1.BridgeDomainMember{Interface: ifName(t, idx), SwIfIndex: idx, PortType: "normal", Shg: uint32(sw.Shg)}
			switch idx {
			case bvi:
				m.PortType = "bvi"
				st.Bvi = m.Interface
			case uu:
				m.PortType = "uu-fwd"
				st.UuFwd = m.Interface
			}
			if d, ok := t.Details(idx); ok && d.VtrOp != 0 {
				m.TagRewrite = desired.TagRewriteName(l2.VtrOp(d.VtrOp)) //nolint:gosec // VPP op codes 0–8
			}
			st.Members = append(st.Members, m)
		}
		sort.Slice(st.Members, func(i, j int) bool { return st.Members[i].GetInterface() < st.Members[j].GetInterface() })
		// counted while streaming: the list only needs the two counters, never the table (review #5)
		if st.StaticMacs, st.LearnedMacs, err = s.fibCounts(ctx, bd.BdID); err != nil {
			return nil, grpcVPPError(err, "l2_fib_table_dump")
		}
		resp.BridgeDomains = append(resp.BridgeDomains, st)
	}
	return resp, nil
}

// BridgeDomainMacs implements the RPC: one page of an owned bridge domain's L2 FIB, ordered by MAC.
func (s *Service) BridgeDomainMacs(ctx context.Context, req *vrxv1.BridgeDomainMacsRequest) (*vrxv1.BridgeDomainMacsResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	limit := req.GetLimit()
	switch {
	case limit == 0:
		limit = bridgeMacsDefaultLimit
	case limit > bridgeMacsMaxLimit:
		return nil, status.Errorf(codes.InvalidArgument, "limit %d exceeds %d", limit, bridgeMacsMaxLimit)
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	bds, err := l2.OwnedBridgeDomains(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, grpcVPPError(err, "bridge-domain macs")
	}
	owned := false
	for _, bd := range bds {
		owned = owned || bd.BdID == req.GetBdId()
	}
	if !owned {
		return nil, status.Errorf(codes.NotFound, "bridge domain %d is not this agent's", req.GetBdId())
	}
	fib, err := s.fibOf(ctx, req.GetBdId())
	if err != nil {
		return nil, grpcVPPError(err, "l2_fib_table_dump")
	}
	sort.Slice(fib, func(i, j int) bool { return bytes.Compare(fib[i].Mac[:], fib[j].Mac[:]) < 0 })
	resp := &vrxv1.BridgeDomainMacsResponse{Total: uint32(len(fib)), Owner: s.owner, RetrievedAt: timestamppb.New(s.now())} //nolint:gosec // an L2 FIB fits in 32 bits
	start := int(req.GetOffset())
	if start >= len(fib) {
		return resp, nil
	}
	end := start + int(limit)
	if end > len(fib) {
		end = len(fib)
	}
	var t *iface.Table
	if end > start {
		if t, err = iface.Dump(ctx, s.vpp, s.owner); err != nil {
			return nil, grpcVPPError(err, "bridge-domain macs")
		}
	}
	for _, e := range fib[start:end] {
		m := &vrxv1.BridgeDomainMac{Mac: iface.FormatMAC(e.Mac), Static: e.StaticMac && !e.FilterMac, Filter: e.FilterMac, Bvi: e.BviMac}
		if idx := uint32(e.SwIfIndex); idx != iface.AllInterfaces && !e.FilterMac {
			m.SwIfIndex = idx
			m.Interface = ifName(t, idx)
		}
		resp.Macs = append(resp.Macs, m)
	}
	return resp, nil
}
