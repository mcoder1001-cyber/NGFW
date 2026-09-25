package agent

// F-mpls-srmpls RPC (wave-A-hotspots A4 pattern: own file): MplsState, the read-only MPLS state of
// this owner (docs/contracts/proto.md "F-mpls-srmpls: MplsState"). view "fib" pages one MPLS FIB in
// the agent (mpls_route_dump read once; only the page crosses the gRPC boundary); view "tunnels" lists
// the MPLS tunnels (mpls_tunnel_dump). Nothing here mutates VPP.

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/mpls"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// Page limits of MplsState view "fib".
const (
	mplsDefaultLimit = 100
	mplsMaxLimit     = 1000
	// mplsMaxWindow bounds offset + limit: the lister keeps that many entries while it streams the
	// table, so memory is bounded by the window, not by the table.
	mplsMaxWindow = 100_000
	// mplsWalkWait is how long a request waits for the MPLS FIB walk in progress (D-132).
	mplsWalkWait = 3 * time.Second
	// mplsWalkTimeout bounds one walk once it started (it is not cut short by the caller, so the next
	// walk never queues behind a half-read one in VPP).
	mplsWalkTimeout = 2 * time.Minute
)

// mplsWalkSem admits one MPLS FIB walk per agent (D-132): VPP's mpls_route_dump walks the table under
// the worker barrier; concurrent readers would only queue in VPP and stretch the stall.
var mplsWalkSem = make(chan struct{}, 1)

// errMplsBusy → UNAVAILABLE.
var errMplsBusy = errors.New("an MPLS FIB read is in progress (the agent runs one mpls_route_dump walk at a time); retry shortly")

func acquireMplsWalk(ctx context.Context) error {
	t := time.NewTimer(mplsWalkWait)
	defer t.Stop()
	select {
	case mplsWalkSem <- struct{}{}:
		return nil
	case <-t.C:
		return errMplsBusy
	case <-ctx.Done():
		return fmt.Errorf("%w (%v)", errMplsBusy, ctx.Err())
	}
}

// MplsState implements vrx.v1.Dataplane/MplsState.
func (g *server) MplsState(ctx context.Context, req *vrxv1.MplsStateRequest) (*vrxv1.MplsStateResponse, error) {
	return g.svc.mplsState(ctx, req)
}

func (s *Service) mplsState(ctx context.Context, req *vrxv1.MplsStateRequest) (*vrxv1.MplsStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	view := req.GetView()
	if view != "fib" && view != "tunnels" {
		return nil, status.Errorf(codes.InvalidArgument, "view %q: \"fib\" or \"tunnels\"", view)
	}
	limit := req.GetLimit()
	if limit == 0 {
		limit = mplsDefaultLimit
	}
	if view == "fib" {
		if limit > mplsMaxLimit {
			return nil, status.Errorf(codes.InvalidArgument, "limit %d > %d", limit, mplsMaxLimit)
		}
		if uint64(req.GetOffset())+uint64(limit) > mplsMaxWindow {
			return nil, status.Errorf(codes.InvalidArgument, "offset + limit %d > %d: filter by label", uint64(req.GetOffset())+uint64(limit), mplsMaxWindow)
		}
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	tables, err := mplsReadableTables(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, mplsStatus(err)
	}
	ifs, err := iface.Dump(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, mplsStatus(err)
	}
	resp := &vrxv1.MplsStateResponse{Owner: s.owner, View: view, Tables: tables}
	switch view {
	case "fib":
		readable := false
		for _, t := range tables {
			readable = readable || t.GetTableId() == req.GetTableId()
		}
		if !readable {
			return nil, status.Errorf(codes.NotFound, "MPLS table %d does not exist or is not this owner's", req.GetTableId())
		}
		entries, total, err := listMplsFib(ctx, s.vpp, ifs, req.GetTableId(), req.GetLabel(), req.GetOffset(), limit)
		if err != nil {
			return nil, mplsStatus(err)
		}
		resp.TableId, resp.Entries, resp.Total = req.GetTableId(), entries, total
	case "tunnels":
		tunnels, err := listMplsTunnels(ctx, s.vpp, ifs, s.owner)
		if err != nil {
			return nil, mplsStatus(err)
		}
		resp.Tunnels = tunnels
	}
	resp.RetrievedAt = timestamppb.New(s.now())
	return resp, nil
}

// mplsStatus maps lister errors onto gRPC codes.
func mplsStatus(err error) error {
	switch {
	case errors.Is(err, errMplsBusy), errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	return status.Errorf(codes.Internal, "mpls state: %v", err)
}

// mplsReadableTables lists the MPLS tables owner may read: table 0 when it exists (VPP-global) and
// the tables named "<owner>:<id>", ascending.
func mplsReadableTables(ctx context.Context, c vpp.Client, owner string) ([]*vrxv1.MplsStateTable, error) {
	stream, err := mpls.NewServiceClient(c).MplsTableDump(ctx, &mpls.MplsTableDump{})
	if err != nil {
		return nil, fmt.Errorf("mpls_table_dump: %w", err)
	}
	var out []*vrxv1.MplsStateTable
	for {
		d, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("mpls_table_dump: %w", err)
		}
		id, name := d.MtTable.MtTableID, strings.TrimRight(d.MtTable.MtName, "\x00")
		tagged, ok := vpp.ParseOwnerTag(name, owner)
		if id == 0 || (ok && tagged == fmt.Sprint(id)) {
			out = append(out, &vrxv1.MplsStateTable{TableId: id, Name: name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetTableId() < out[j].GetTableId() })
	return out, nil
}

// mplsEntry is one kept FIB entry (the window holds these, never the decoded binapi routes).
type mplsEntry struct {
	label uint32
	eos   bool
	e     *vrxv1.MplsStateFibEntry
}

func mplsLess(a, b mplsEntry) bool {
	if a.label != b.label {
		return a.label < b.label
	}
	return a.eos && !b.eos
}

// mplsWindow is a max-heap of the smallest n entries seen so far.
type mplsWindow []mplsEntry

func (w mplsWindow) Len() int           { return len(w) }
func (w mplsWindow) Less(i, j int) bool { return mplsLess(w[j], w[i]) }
func (w mplsWindow) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w *mplsWindow) Push(x any)        { *w = append(*w, x.(mplsEntry)) }
func (w *mplsWindow) Pop() any {
	old := *w
	x := old[len(old)-1]
	*w = old[:len(old)-1]
	return x
}

// listMplsFib reads MPLS table `table` once (serialised per agent), keeps the entries of `label` (0 =
// all) and only the smallest offset+limit of them, and returns the page and the number of matches.
func listMplsFib(ctx context.Context, c vpp.Client, ifs *iface.Table, table, label, offset, limit uint32) ([]*vrxv1.MplsStateFibEntry, uint32, error) {
	if err := acquireMplsWalk(ctx); err != nil {
		return nil, 0, err
	}
	defer func() { <-mplsWalkSem }()
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mplsWalkTimeout)
	defer cancel()
	stream, err := mpls.NewServiceClient(c).MplsRouteDump(wctx, &mpls.MplsRouteDump{Table: mpls.MplsTable{MtTableID: table}})
	if err != nil {
		return nil, 0, fmt.Errorf("mpls_route_dump: %w", err)
	}
	window := int(offset + limit)
	w := &mplsWindow{}
	var total uint32
	for {
		d, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, 0, fmt.Errorf("mpls_route_dump: %w", err)
		}
		r := d.MrRoute
		if label != 0 && r.MrLabel != label {
			continue
		}
		total++
		e := mplsEntry{label: r.MrLabel, eos: r.MrEos != 0}
		if w.Len() == window && !mplsLess(e, (*w)[0]) {
			continue
		}
		e.e = &vrxv1.MplsStateFibEntry{Label: r.MrLabel, Eos: r.MrEos != 0, Paths: mplsStatePaths(r.MrPaths, ifs)}
		if e.eos {
			e.e.Payload = mplsPayloadName(r.MrEosProto)
		}
		heap.Push(w, e)
		if w.Len() > window {
			heap.Pop(w)
		}
	}
	kept := append(mplsWindow(nil), *w...)
	sort.Slice(kept, func(i, j int) bool { return mplsLess(kept[i], kept[j]) })
	var page []*vrxv1.MplsStateFibEntry
	for i := int(offset); i < len(kept); i++ {
		page = append(page, kept[i].e)
	}
	return page, total, nil
}

// listMplsTunnels lists this owner's MPLS tunnels (tag "<owner>:<name>") and untagged ones; another
// owner's are never listed.
func listMplsTunnels(ctx context.Context, c vpp.Client, ifs *iface.Table, owner string) ([]*vrxv1.MplsStateTunnel, error) {
	stream, err := mpls.NewServiceClient(c).MplsTunnelDump(ctx, &mpls.MplsTunnelDump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("mpls_tunnel_dump: %w", err)
	}
	var out []*vrxv1.MplsStateTunnel
	for {
		d, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("mpls_tunnel_dump: %w", err)
		}
		t := d.MtTunnel
		sw := uint32(t.MtSwIfIndex)
		tag := strings.TrimRight(t.MtTag, "\x00")
		vppName := ifs.VPPName(sw)
		st := &vrxv1.MplsStateTunnel{Interface: vppName, SwIfIndex: sw, TunnelIndex: t.MtTunnelIndex, L2Only: t.MtL2Only,
			Multicast: t.MtIsMulticast, Paths: mplsStatePaths(t.MtPaths, ifs)}
		switch name, ours := vpp.ParseOwnerTag(tag, owner); {
		case ours:
			st.Name, st.Owned = name, true
		case tag == "":
			st.Name = vppName
		default:
			continue // another owner's tunnel
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetName() < out[j].GetName() })
	return out, nil
}

// mplsStatePaths converts FIB paths (interface names: logical, else VPP's).
func mplsStatePaths(paths []fib_types.FibPath, ifs *iface.Table) []*vrxv1.MplsStatePath {
	out := make([]*vrxv1.MplsStatePath, 0, len(paths))
	for _, p := range paths {
		sp := &vrxv1.MplsStatePath{
			Type:       enumName(p.Type.String(), "FIB_API_PATH_TYPE_"),
			Proto:      enumName(p.Proto.String(), "FIB_API_PATH_NH_PROTO_"),
			TableId:    p.TableID,
			Weight:     uint32(p.Weight),
			Preference: uint32(p.Preference),
		}
		if p.SwIfIndex != iface.AllInterfaces {
			if n, ok := ifs.Logical(p.SwIfIndex); ok {
				sp.Interface = n
			} else if n := ifs.VPPName(p.SwIfIndex); n != "" {
				sp.Interface = n
			} else {
				sp.Interface = fmt.Sprintf("#%d", p.SwIfIndex)
			}
		}
		var a netip.Addr
		switch p.Proto {
		case fib_types.FIB_API_PATH_NH_PROTO_IP4:
			a = netip.AddrFrom4(p.Nh.Address.GetIP4())
		case fib_types.FIB_API_PATH_NH_PROTO_IP6:
			a = netip.AddrFrom16(p.Nh.Address.GetIP6())
		}
		if a.IsValid() && !a.IsUnspecified() {
			sp.NextHop = a.String()
		}
		n := int(p.NLabels)
		if n > len(p.LabelStack) {
			n = len(p.LabelStack)
		}
		for i := 0; i < n; i++ {
			sp.OutLabels = append(sp.OutLabels, p.LabelStack[i].Label)
		}
		out = append(out, sp)
	}
	return out
}

// enumName turns a binapi enum name ("FIB_API_PATH_TYPE_ICMP_UNREACH") into the API spelling
// ("icmp-unreach").
func enumName(s, prefix string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, prefix)), "_", "-")
}

// mplsPayloadName names an EOS payload (dpo_proto_t).
func mplsPayloadName(p uint8) string {
	switch p {
	case 0:
		return "ip4"
	case 1:
		return "ip6"
	case 2:
		return "mpls"
	case 3:
		return "ethernet"
	}
	return fmt.Sprintf("#%d", p)
}
