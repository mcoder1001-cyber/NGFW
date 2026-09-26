package agent

// F-neighbors-ra RPCs (docs/contracts/proto.md §11): ListNeighbors and the arp_flush action. The logic lives in
// internal/actions/neighbors-ra; this file only adapts it to gRPC.

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	neighborsra "ngfw/agent/internal/actions/neighbors-ra"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// neighborsError maps an action/lister error onto a gRPC status.
func neighborsError(what string, err error) error {
	switch {
	case errors.Is(err, neighborsra.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

// ListNeighbors implements the ListNeighbors RPC: the live ARP/ND table of every interface this agent can name.
func (g *server) ListNeighbors(ctx context.Context, req *vrxv1.ListNeighborsRequest) (*vrxv1.ListNeighborsResponse, error) {
	s := g.svc
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	page, err := neighborsra.List(ctx, s.vpp, s.owner, neighborsra.Query{
		VRF: req.GetVrf(), Interface: req.GetInterface(), Family: req.GetFamily(), State: req.GetState(),
		Search: req.GetSearch(), Sort: req.GetSort(), Descending: req.GetDescending(),
		Offset: req.GetOffset(), Limit: req.GetLimit(),
	}, s.tableName)
	if err != nil {
		return nil, neighborsError("list neighbours", err)
	}
	resp := &vrxv1.ListNeighborsResponse{Owner: s.owner, Total: uint32(page.Total), RetrievedAt: timestamppb.New(s.now())} //nolint:gosec // G115: a neighbour table is far below 2^32 rows
	for _, e := range page.Entries {
		resp.Neighbors = append(resp.Neighbors, &vrxv1.NeighborEntry{
			Interface: e.Interface, Ip: e.IP.String(), Mac: e.MAC, Family: e.Family, State: e.State(),
			NoFibEntry: e.NoFibEntry, AgeSec: e.Age, Vrf: e.VRF, TableId: e.TableID,
		})
	}
	return resp, nil
}

// configuredInterfaces lists the (sub-)interfaces of the stored `interfaces` document, sorted: what an arp_flush
// without an interface flushes.
func (s *Service) configuredInterfaces() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for name, itf := range s.storedIfs {
		out = append(out, name)
		for id := range itf.GetSubinterfaces() {
			out = append(out, desired.SubName(name, id))
		}
	}
	sort.Strings(out)
	return out
}

// arpFlush runs the arp_flush action (ActionRequest 4): delete learned ARP/ND entries, static ones stay.
func (g *server) arpFlush(req *vrxv1.ArpFlushAction, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	s := g.svc
	fam, err := neighborsra.ParseFamily(req.GetFamily())
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	ctx := stream.Context()
	// L1: one flush at a time with Apply/Resync. VPP's delete takes whatever entry has the key, so a flush interleaved
	// with a commit that turns a learned entry static could delete the new static neighbour.
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	configured := s.configuredInterfaces()
	var names []string
	if name := req.GetInterface(); name != "" {
		// M1: a named flush acts only on an interface of this agent's configuration or one it created (own tag) — never
		// on another workload's untagged interface, which the resolver would otherwise accept by its VPP name.
		ok := slices.Contains(configured, name)
		if !ok {
			owned, err := neighborsra.Owned(ctx, s.vpp, s.owner, name)
			if err != nil {
				return neighborsError("arp flush", err)
			}
			ok = owned
		}
		if !ok {
			return status.Errorf(codes.InvalidArgument, "interface %q is not an interface of this configuration (configured, or created by owner %q): refusing to flush it", name, s.owner)
		}
		names = []string{name}
	} else {
		all, err := neighborsra.Nameable(ctx, s.vpp, s.owner, "")
		if err != nil {
			return neighborsError("arp flush", err)
		}
		nameable := map[string]bool{}
		for _, it := range all {
			nameable[it.Name] = true
		}
		for _, n := range configured {
			if nameable[n] {
				names = append(names, n)
			}
		}
	}
	var sendErr error
	line := func(l string) {
		if sendErr == nil {
			sendErr = stream.Send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: l}})
		}
	}
	res, err := neighborsra.Flush(ctx, s.vpp, s.owner, names, fam, line)
	if err != nil {
		if errors.Is(err, neighborsra.ErrInvalid) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		g.log.Warn("arp flush failed", "interface", req.GetInterface(), "deleted", res.Deleted, "err", err)
		return stream.Send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{
			Summary: err.Error(), ExitCode: 1,
			Stats: map[string]string{"deleted": strconv.Itoa(res.Deleted), "interfaces": strconv.Itoa(res.Interfaces)},
		}}})
	}
	if sendErr != nil {
		return sendErr
	}
	g.log.Info("arp flush", "interface", req.GetInterface(), "family", req.GetFamily(), "deleted", res.Deleted, "interfaces", res.Interfaces)
	return stream.Send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{
		Summary: "deleted " + strconv.Itoa(res.Deleted) + " learned entries on " + strconv.Itoa(res.Interfaces) + " interfaces",
		Stats:   map[string]string{"deleted": strconv.Itoa(res.Deleted), "interfaces": strconv.Itoa(res.Interfaces)},
	}}})
}
