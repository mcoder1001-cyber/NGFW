package multiwan

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// MaxWorkers is the schema maximum: 16 groups x 16 members x 8 monitors.
const MaxWorkers = 2048

// Probe must honor cancellation and the supplied deadline. Each logical monitor
// has one worker and never overlaps its own previous probe.
type Probe func(context.Context, string, *vrxv1.WanMonitor) CheckResult

type sample struct {
	state  State
	result CheckResult
	seen   bool
}
type memberSamples struct {
	member  *vrxv1.WanMember
	samples []sample
	up      bool
	since   time.Time
}
type groupSamples struct {
	group   *vrxv1.WanGroup
	members map[string]*memberSamples
}

// Runtime owns bounded probes and snapshots. Replace drains the previous
// generation before starting another, including late non-cooperative completions.
type Runtime struct {
	replace    sync.Mutex
	mu         sync.Mutex
	groups     map[string]*groupSamples
	config     []*vrxv1.WanGroup
	identity   string
	ready      bool
	cancel     context.CancelFunc
	done       chan struct{}
	generation uint64
	probe      Probe
}

// NewRuntime constructs an inactive runtime; Replace starts probes.
func NewRuntime(probe Probe) *Runtime {
	return &Runtime{probe: probe, groups: map[string]*groupSamples{}}
}

// Replace is all-or-nothing for configuration validation. An identical config
// preserves hysteresis; the caller owns ctx for the lifetime of these probes.
func (r *Runtime) Replace(ctx context.Context, groups []*vrxv1.WanGroup) error {
	return r.ReplaceWithIdentity(ctx, groups, "")
}

// ReplaceWithIdentity also invalidates observations when the member device or
// VRF changes without a change to the group itself.
func (r *Runtime) ReplaceWithIdentity(ctx context.Context, groups []*vrxv1.WanGroup, identity string) error {
	return r.ReplaceWithProbe(ctx, groups, identity, r.probe)
}

// ReplaceWithProbe captures a generation-specific probe, including its device mapping.
// Old workers keep their original mapping while they drain.
func (r *Runtime) ReplaceWithProbe(ctx context.Context, groups []*vrxv1.WanGroup, identity string, probe Probe) error {
	r.replace.Lock()
	defer r.replace.Unlock()
	if probe == nil {
		return errors.New("WAN probe is not configured")
	}
	count := 0
	seen := map[string]bool{}
	cloned := make([]*vrxv1.WanGroup, 0, len(groups))
	for _, g := range groups {
		if g == nil || g.GetName() == "" || seen[g.GetName()] {
			return errors.New("invalid or duplicate WAN group")
		}
		seen[g.GetName()] = true
		members := map[string]bool{}
		for _, m := range g.GetMembers() {
			if m == nil || m.GetInterface() == "" || members[m.GetInterface()] {
				return errors.New("invalid or duplicate WAN member")
			}
			members[m.GetInterface()] = true
		}
		for _, monitor := range g.GetMonitors() {
			if monitor == nil || monitor.GetIntervalMs() < 100 || monitor.GetIntervalMs() > 600000 || monitor.GetTimeoutMs() < 50 || monitor.GetTimeoutMs() > 60000 {
				return errors.New("invalid WAN monitor timing")
			}
		}
		count += len(g.GetMembers()) * len(g.GetMonitors())
		if count > MaxWorkers {
			return fmt.Errorf("WAN probe limit %d exceeded", MaxWorkers)
		}
		cloned = append(cloned, proto.Clone(g).(*vrxv1.WanGroup))
	}
	r.mu.Lock()
	same := len(cloned) == len(r.config) && identity == r.identity
	for i := range cloned {
		if !same || !proto.Equal(cloned[i], r.config[i]) {
			same = false
			break
		}
	}
	if same && r.done != nil {
		select {
		case <-r.done:
			same = false
		default:
		}
	}
	if same && r.done != nil {
		r.mu.Unlock()
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	oldDone := r.done
	r.ready = false
	r.generation++ // stale results are rejected even while draining
	generation := r.generation
	r.mu.Unlock()
	if oldDone != nil {
		select {
		case <-oldDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.mu.Lock()
	r.ready = true
	r.config = cloned
	r.identity = identity
	r.groups = map[string]*groupSamples{}
	r.cancel = cancel
	r.done = done
	var wg sync.WaitGroup
	for _, g := range cloned {
		gs := &groupSamples{group: g, members: map[string]*memberSamples{}}
		r.groups[g.GetName()] = gs
		for _, m := range g.GetMembers() {
			ms := &memberSamples{member: m, samples: make([]sample, len(g.GetMonitors()))}
			gs.members[m.GetInterface()] = ms
			for i, monitor := range g.GetMonitors() {
				// Unobserved links are not reported healthy.
				ms.samples[i].state = State{Up: false}
				wg.Add(1)
				go func(group, member string, index int, mon *vrxv1.WanMonitor) {
					defer wg.Done()
					r.worker(workCtx, generation, group, member, index, mon, probe)
				}(g.GetName(), m.GetInterface(), i, monitor)
			}
		}
	}
	r.mu.Unlock()
	go func() { wg.Wait(); close(done) }()
	return nil
}

func (r *Runtime) worker(ctx context.Context, gen uint64, group, member string, index int, mon *vrxv1.WanMonitor, probe Probe) {
	for {
		if ctx.Err() != nil {
			return
		}
		pctx, cancel := context.WithTimeout(ctx, time.Duration(mon.GetTimeoutMs())*time.Millisecond)
		result := probe(pctx, member, mon)
		cancel()
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		if r.generation != gen {
			r.mu.Unlock()
			return
		}
		ms := r.groups[group].members[member]
		sm := &ms.samples[index]
		// Treat malformed probe output as failure, never a negative loss.
		if result.Sent <= 0 || result.Sent > 1000000 || result.Received < 0 || result.Received > result.Sent || result.AvgLatencyMs < 0 || result.AvgLatencyMs > 60000 {
			result = CheckResult{Sent: 1}
		}
		sm.state.Observe(MonitorConfig{LossPct: int(mon.GetLossPct()), LatencyMs: int(mon.GetLatencyMs()), DownAfter: int(mon.GetDownAfter()), UpAfter: int(mon.GetUpAfter())}, result)
		sm.result = result
		sm.seen = true
		up := len(ms.samples) > 0
		for _, monitor := range ms.samples {
			up = up && monitor.seen && monitor.state.Up
		}
		if up != ms.up {
			ms.up = up
			ms.since = time.Now()
		}
		r.mu.Unlock()
		timer := time.NewTimer(time.Duration(mon.GetIntervalMs()) * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

// Snapshot returns independent messages; callers cannot mutate worker state.
// Active remains empty until a routing controller confirms an installed route.
func (r *Runtime) Snapshot() []*vrxv1.WanGroupState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ready {
		return nil
	}
	out := make([]*vrxv1.WanGroupState, 0, len(r.groups))
	for name, gs := range r.groups {
		group := &vrxv1.WanGroupState{Name: name, Mode: gs.group.GetMode()}
		for name, ms := range gs.members {
			member := &vrxv1.WanMemberState{Interface: name, Weight: ms.member.GetWeight(), Priority: ms.member.GetPriority(), Up: ms.up}
			for _, sm := range ms.samples {
				loss := uint32(100)
				if sm.seen && sm.result.Sent > 0 {
					value := (sm.result.Sent - sm.result.Received) * 100 / sm.result.Sent
					if value >= 0 && value <= 100 {
						loss = uint32(value)
					}
				}
				if loss > member.LossPct {
					member.LossPct = loss
				}
				latency := sm.result.AvgLatencyMs
				if latency >= 0 && latency <= 60000 && uint32(latency) > member.LatencyMs {
					member.LatencyMs = uint32(latency)
				}
			}
			if !ms.since.IsZero() {
				member.Since = timestamppb.New(ms.since)
			}
			group.Members = append(group.Members, member)
		}
		sort.Slice(group.Members, func(i, j int) bool { return group.Members[i].Interface < group.Members[j].Interface })
		out = append(out, group)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Close cancels probes and drains them within the supplied deadline.
func (r *Runtime) Close(ctx context.Context) error {
	r.replace.Lock()
	defer r.replace.Unlock()
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.generation++
	r.ready = false
	done := r.done
	r.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Ready reports whether a configuration is installed and is not draining.
func (r *Runtime) Ready() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.ready }
