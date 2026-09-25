package agent

// LispState (F-lisp): the live LISP / LISP-GPE state read from the VPP dumps. Read-only; VPP-wide
// (LISP objects carry no owner tag), like `show lisp …`. A VPP without the lisp plugin answers
// enabled: false with empty lists.

import (
	"context"
	"errors"
	"sort"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/ethernet_types"
	lispapi "ngfw/agent/binapi/lisp"
	gpeapi "ngfw/agent/binapi/lisp_gpe"
	"ngfw/agent/binapi/lisp_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// LispState implements vrx.v1.Dataplane.
func (g *server) LispState(ctx context.Context, req *vrxv1.LispStateRequest) (*vrxv1.LispStateResponse, error) {
	return g.svc.LispState(ctx, req)
}

// LispState dumps the LISP state (see rpc_lisp.go).
func (s *Service) LispState(ctx context.Context, req *vrxv1.LispStateRequest) (*vrxv1.LispStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	resp, err := lispState(ctx, s.vpp, s.owner)
	switch {
	case err == nil:
	case errors.Is(err, vpp.ErrDisconnected):
		return nil, status.Error(codes.Unavailable, err.Error())
	case errors.Is(df6.PluginError("lisp", err), df6.ErrPluginNotLoaded):
		resp = &vrxv1.LispStateResponse{}
	default:
		return nil, status.Errorf(codes.Internal, "lisp state: %v", err)
	}
	resp.Owner = s.owner
	resp.RetrievedAt = timestamppb.New(s.now())
	return resp, nil
}

func lispEID(e lisp_types.Eid) string {
	switch e.Type {
	case lisp_types.EID_TYPE_API_PREFIX:
		return df6.PrefixString(e.Address.GetPrefix())
	case lisp_types.EID_TYPE_API_MAC:
		m := e.Address.GetMac()
		if m == (ethernet_types.MacAddress{}) {
			return ""
		}
		return df6.MACString(m)
	}
	return ""
}

var lispActionNames = []string{"no-action", "natively-forward", "send-map-request", "drop"}

func lispState(ctx context.Context, c vpp.Client, owner string) (*vrxv1.LispStateResponse, error) {
	svc := lispapi.NewServiceClient(c)
	st, err := svc.ShowLispStatus(ctx, &lispapi.ShowLispStatus{})
	if err != nil {
		return nil, err
	}
	out := &vrxv1.LispStateResponse{Enabled: st.IsLispEnabled, GpeEnabled: st.IsGpeEnabled}
	if st.IsGpeEnabled {
		vr, err := gpeapi.NewServiceClient(c).GpeFwdEntryVnisGet(ctx, &gpeapi.GpeFwdEntryVnisGet{})
		if err != nil {
			return nil, err
		}
		out.GpeVnis = append(out.GpeVnis, vr.Vnis...)
		sort.Slice(out.GpeVnis, func(a, b int) bool { return out.GpeVnis[a] < out.GpeVnis[b] })
	}
	if !st.IsLispEnabled {
		return out, nil
	}
	if p, err := svc.ShowLispPitr(ctx, &lispapi.ShowLispPitr{}); err != nil {
		return nil, err
	} else if p.IsEnabled {
		out.Pitr = p.LocatorSetName
	}
	names := map[uint32]string{}
	if ifs, err := df6.DumpInterfaces(ctx, c, owner); err == nil {
		t := ifs.Table()
		for _, idx := range t.Indexes() {
			if n, ok := t.Logical(idx); ok {
				names[idx] = n
			} else {
				names[idx] = t.VPPName(idx)
			}
		}
	}
	locators := func(idx uint32) ([]*lispapi.LispLocatorDetails, error) {
		st, err := svc.LispLocatorDump(ctx, &lispapi.LispLocatorDump{LsIndex: idx, IsIndexSet: 1})
		if err != nil {
			return nil, err
		}
		return df6.Collect(st.Recv)
	}
	ls, err := svc.LispLocatorSetDump(ctx, &lispapi.LispLocatorSetDump{Filter: lispapi.LISP_LOCATOR_SET_FILTER_API_LOCAL})
	if err != nil {
		return nil, err
	}
	sets, err := df6.Collect(ls.Recv)
	if err != nil {
		return nil, err
	}
	setName := map[uint32]string{}
	for _, d := range sets {
		setName[d.LsIndex] = d.LsName
		locs, err := locators(d.LsIndex)
		if err != nil {
			return nil, err
		}
		s := &vrxv1.LispStateLocatorSet{Name: d.LsName}
		for _, l := range locs {
			s.Locators = append(s.Locators, &vrxv1.LispStateLocator{Interface: names[uint32(l.SwIfIndex)], SwIfIndex: uint32(l.SwIfIndex), Priority: uint32(l.Priority), Weight: uint32(l.Weight)})
		}
		out.LocatorSets = append(out.LocatorSets, s)
	}
	sort.Slice(out.LocatorSets, func(a, b int) bool { return out.LocatorSets[a].GetName() < out.LocatorSets[b].GetName() })

	et, err := svc.LispEidTableDump(ctx, &lispapi.LispEidTableDump{Filter: lispapi.LISP_LOCATOR_SET_FILTER_API_ALL})
	if err != nil {
		return nil, err
	}
	recs, err := df6.Collect(et.Recv)
	if err != nil {
		return nil, err
	}
	vnis := map[uint32]bool{}
	for _, r := range recs {
		if r.IsSrcDst {
			continue
		}
		m := &vrxv1.LispStateMapping{Vni: r.Vni, Eid: lispEID(r.Seid), Local: r.IsLocal, Ttl: r.TTL, Authoritative: r.Authoritative != 0}
		if int(r.Action) < len(lispActionNames) {
			m.Action = lispActionNames[r.Action]
		}
		if r.IsLocal {
			m.LocatorSet = setName[r.LocatorSetIndex]
		} else if r.LocatorSetIndex != ^uint32(0) {
			locs, err := locators(r.LocatorSetIndex)
			if err != nil {
				return nil, err
			}
			for _, l := range locs {
				m.Rlocs = append(m.Rlocs, df6.AddressString(l.IPAddress))
			}
		}
		vnis[r.Vni] = true
		out.Mappings = append(out.Mappings, m)
	}
	sort.Slice(out.Mappings, func(a, b int) bool {
		x, y := out.Mappings[a], out.Mappings[b]
		if x.GetVni() != y.GetVni() {
			return x.GetVni() < y.GetVni()
		}
		if x.GetLocal() != y.GetLocal() {
			return x.GetLocal()
		}
		return x.GetEid() < y.GetEid()
	})
	vl := make([]uint32, 0, len(vnis))
	for v := range vnis {
		vl = append(vl, v)
	}
	sort.Slice(vl, func(a, b int) bool { return vl[a] < vl[b] })
	for _, v := range vl {
		rep, err := svc.LispAdjacenciesGet(ctx, &lispapi.LispAdjacenciesGet{Vni: v})
		if err != nil {
			return nil, err
		}
		for _, a := range rep.Adjacencies {
			out.Adjacencies = append(out.Adjacencies, &vrxv1.LispStateAdjacency{Vni: v, Reid: lispEID(a.Reid), Leid: lispEID(a.Leid)})
		}
	}
	for _, l2 := range []bool{false, true} {
		st, err := svc.LispEidTableMapDump(ctx, &lispapi.LispEidTableMapDump{IsL2: l2})
		if err != nil {
			return nil, err
		}
		maps, err := df6.Collect(st.Recv)
		if err != nil {
			return nil, err
		}
		for _, m := range maps {
			if m.Vni == 0 {
				continue // the implicit default instance
			}
			out.EidTables = append(out.EidTables, &vrxv1.LispStateEidTable{Vni: m.Vni, DpTable: m.DpTable, IsL2: l2})
		}
	}
	sort.Slice(out.EidTables, func(a, b int) bool { return out.EidTables[a].GetVni() < out.EidTables[b].GetVni() })
	mr, err := svc.LispMapResolverDump(ctx, &lispapi.LispMapResolverDump{})
	if err != nil {
		return nil, err
	}
	rs, err := df6.Collect(mr.Recv)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		out.MapResolvers = append(out.MapResolvers, df6.AddressString(r.IPAddress))
	}
	ms, err := svc.LispMapServerDump(ctx, &lispapi.LispMapServerDump{})
	if err != nil {
		return nil, err
	}
	ss, err := df6.Collect(ms.Recv)
	if err != nil {
		return nil, err
	}
	for _, r := range ss {
		out.MapServers = append(out.MapServers, df6.AddressString(r.IPAddress))
	}
	sort.Strings(out.MapResolvers)
	sort.Strings(out.MapServers)
	return out, nil
}
