package agent

// F-qos-flat: the QosPolicerState and QosPolicerReset RPCs (docs/contracts/proto.md §11). Read-only status and an
// action — never part of a transaction: the policers come from DF-7's policer walk (policer.States: policer_dump_v2)
// and their counters from the stats segment (/net/policer/conform|exceed|violate, combined counters indexed by the
// pool index), the reset is policer_reset through policer.ResetIndex (the pool index looked up by the owner-tagged
// name right before the call). One policer walk at a time per agent (D-132): a call that cannot start within
// qosWalkWait fails with UNAVAILABLE. Service is A5 core, so this feature's per-Service state lives here.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/adapter/statsclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/vpp"
)

// qosWalkWait bounds how long a call waits for the walk slot (D-132); tests lower it.
var qosWalkWait = 3 * time.Second

// qosPolicerName is a policer name without the owner prefix: an objectName, or "shaper:<objectName>".
var qosPolicerName = regexp.MustCompile(`^(shaper:)?[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// Stats segment directories of the policer counters (VPP policer plugin, plugin.c), by colour.
var qosCounterNames = map[string]int{"/net/policer/conform": 0, "/net/policer/exceed": 1, "/net/policer/violate": 2}

// qosCounter is packets/bytes of one colour.
type qosCounter struct{ packets, bytes uint64 }

// policerCounterSource reads the policer counters: pool index → conform, exceed, violate.
type policerCounterSource interface {
	PolicerCounters() (map[uint32][3]qosCounter, error)
}

// qosCountersOf returns the counter source of a server (tests replace it): the stats segment the agent's
// interface counters come from (VRX_AGENT_VPP_STATS_SOCKET), nil when the server has none.
var qosCountersOf = func(g *server) policerCounterSource {
	if r, ok := g.stats.(*statsReader); ok && r.path != "" {
		return segmentPolicerCounters{path: r.path}
	}
	return nil
}

// segmentPolicerCounters reads the stats segment with a connection of its own per call (the interface counter
// reader's connection is a core.StatsConnection, which has no generic dump).
type segmentPolicerCounters struct{ path string }

// PolicerCounters implements policerCounterSource.
func (p segmentPolicerCounters) PolicerCounters() (map[uint32][3]qosCounter, error) {
	sc := statsclient.NewStatsClient(p.path, statsclient.SetSocketRetryTimeout(time.Second))
	if err := sc.Connect(); err != nil {
		return nil, fmt.Errorf("stats segment %s: %w", p.path, err)
	}
	defer func() { _ = sc.Disconnect() }()
	entries, err := sc.DumpStats(`^/net/policer/(conform|exceed|violate)$`)
	if err != nil {
		return nil, fmt.Errorf("stats segment: %w", err)
	}
	return sumPolicerCounters(entries), nil
}

// sumPolicerCounters sums the per-thread combined counters of the three policer directories by pool index.
func sumPolicerCounters(entries []adapter.StatEntry) map[uint32][3]qosCounter {
	out := map[uint32][3]qosCounter{}
	for _, e := range entries {
		col, ok := qosCounterNames[string(e.Name)]
		if !ok {
			continue
		}
		data, ok := e.Data.(adapter.CombinedCounterStat)
		if !ok {
			continue
		}
		for _, perThread := range data {
			for idx, c := range perThread {
				v := out[uint32(idx)] //nolint:gosec // G115: a pool index
				v[col].packets += c.Packets()
				v[col].bytes += c.Bytes()
				out[uint32(idx)] = v //nolint:gosec // G115: a pool index
			}
		}
	}
	return out
}

// qosState is this feature's per-Service state.
type qosState struct {
	walk chan struct{} // one policer walk at a time (D-132)
}

var qosStates sync.Map // *Service → *qosState

func (s *Service) qosState() *qosState {
	st, _ := qosStates.LoadOrStore(s, &qosState{walk: make(chan struct{}, 1)})
	return st.(*qosState)
}

// qosWalk takes the walk slot (waiting at most qosWalkWait) and returns its release.
func (s *Service) qosWalk(ctx context.Context) (func(), error) {
	w := s.qosState().walk
	wait, cancel := context.WithTimeout(ctx, qosWalkWait)
	defer cancel()
	select {
	case w <- struct{}{}:
		return func() { <-w }, nil
	case <-wait.Done():
		if err := ctx.Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
		return nil, status.Errorf(codes.Unavailable, "another QoS policer walk is running (one at a time, D-132); retry")
	}
}

func (s *Service) qosReady(owner string) error {
	if err := s.checkOwner(owner); err != nil {
		return err
	}
	if !s.vpp.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	return nil
}

// qosErr maps a VPP-side error to a gRPC status.
func qosErr(what string, err error) error {
	switch {
	case errors.Is(err, policer.ErrNoPolicer):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, vpp.ErrDisconnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

func (g *server) QosPolicerState(ctx context.Context, req *vrxv1.QosPolicerStateRequest) (*vrxv1.QosPolicerStateResponse, error) {
	return g.svc.qosPolicerState(ctx, req, qosCountersOf(g))
}

func (g *server) QosPolicerReset(ctx context.Context, req *vrxv1.QosPolicerResetRequest) (*vrxv1.QosPolicerResetResponse, error) {
	return g.svc.qosPolicerReset(ctx, req)
}

func qosCounterOut(c qosCounter) *vrxv1.QosPolicerCounter {
	return &vrxv1.QosPolicerCounter{Packets: c.packets, Bytes: c.bytes}
}

// qosPolicerState implements QosPolicerState; counters nil = no stats segment.
func (s *Service) qosPolicerState(ctx context.Context, req *vrxv1.QosPolicerStateRequest, counters policerCounterSource) (*vrxv1.QosPolicerStateResponse, error) {
	if err := s.qosReady(req.GetOwner()); err != nil {
		return nil, err
	}
	release, err := s.qosWalk(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	states, err := policer.States(ctx, s.vpp, s.owner)
	if err != nil {
		return nil, qosErr("policer walk", err)
	}
	resp := &vrxv1.QosPolicerStateResponse{Owner: s.owner, RetrievedAt: timestamppb.New(s.now())}
	var byIndex map[uint32][3]qosCounter
	if counters == nil {
		resp.CountersError = "no stats segment configured (VRX_AGENT_VPP_STATS_SOCKET)"
	} else if byIndex, err = counters.PolicerCounters(); err != nil {
		resp.CountersError = err.Error()
	}
	want := map[string]bool{}
	for _, n := range req.GetNames() {
		want[n] = true
	}
	for _, st := range states {
		if len(want) > 0 && !want[st.Spec.Name] {
			continue
		}
		kind := "policer"
		if strings.HasPrefix(st.Spec.Name, desired.ShaperPrefix) {
			kind = "shaper"
		}
		c := byIndex[st.Index]
		resp.Policers = append(resp.Policers, &vrxv1.QosPolicerStatus{
			Name: st.Spec.Name, Kind: kind, Index: st.Index,
			Type: desired.QosPolicerTypeName(st.Spec.Type), RateUnit: st.Spec.RateType,
			Cir: st.Spec.CIR, Eir: st.Spec.EIR, Cb: st.Spec.CB, Eb: st.Spec.EB,
			CurrentBucket: st.CurrentBucket, CurrentLimit: st.CurrentLimit, ExtendedBucket: st.ExtendedBucket, ExtendedLimit: st.ExtendedLimit,
			Conform: qosCounterOut(c[0]), Exceed: qosCounterOut(c[1]), Violate: qosCounterOut(c[2]),
		})
	}
	return resp, nil
}

// qosPolicerReset implements QosPolicerReset.
func (s *Service) qosPolicerReset(ctx context.Context, req *vrxv1.QosPolicerResetRequest) (*vrxv1.QosPolicerResetResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !qosPolicerName.MatchString(req.GetName()) {
		return nil, status.Errorf(codes.InvalidArgument, "name %q is not a policer name (<name> or shaper:<name>)", req.GetName())
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	release, err := s.qosWalk(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	idx, err := policer.ResetIndex(ctx, s.vpp, s.owner, req.GetName())
	if err != nil {
		return nil, qosErr("policer reset", err)
	}
	s.log.Info("policer reset (token buckets refilled)", "policer", req.GetName(), "index", idx)
	return &vrxv1.QosPolicerResetResponse{Owner: s.owner, Name: req.GetName(), Index: idx, ResetAt: timestamppb.New(s.now())}, nil
}
