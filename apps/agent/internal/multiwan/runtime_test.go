package multiwan

import (
	"context"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
	"testing"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func runtimeGroup() *ngfwv1.WanGroup {
	return &ngfwv1.WanGroup{Name: proto.String("edge"), Mode: proto.String("failover"), Members: []*ngfwv1.WanMember{{Interface: proto.String("wan0"), Weight: proto.Uint32(1), Priority: proto.Uint32(1)}}, Monitors: []*ngfwv1.WanMonitor{{Type: proto.String("http"), Target: proto.String("example.test"), IntervalMs: proto.Uint32(100), TimeoutMs: proto.Uint32(50), LossPct: proto.Uint32(100), DownAfter: proto.Uint32(1), UpAfter: proto.Uint32(1)}}}
}
func waitRuntime(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("runtime did not reach expected state")
}
func TestRuntimeReplaceDrainsAndDiscardsLateResult(t *testing.T) {
	first := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	r := NewRuntime(func(_ context.Context, _ string, _ *ngfwv1.WanMonitor) CheckResult {
		if calls.Add(1) == 1 {
			close(first)
			<-release
			return CheckResult{Sent: 1, Received: 1}
		}
		return CheckResult{Sent: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Replace(ctx, []*ngfwv1.WanGroup{runtimeGroup()}); err != nil {
		t.Fatal(err)
	}
	<-first
	group := runtimeGroup()
	group.Members[0].Interface = proto.String("wan1")
	replaced := make(chan error, 1)
	go func() { replaced <- r.Replace(ctx, []*ngfwv1.WanGroup{group}) }()
	select {
	case <-replaced:
		t.Fatal("replacement did not drain old probe")
	case <-time.After(20 * time.Millisecond):
	}
	if r.Ready() || len(r.Snapshot()) != 0 {
		t.Fatal("draining runtime exposed stale health")
	}
	if calls.Load() != 1 {
		t.Fatal("replacement overlapped previous generation")
	}
	close(release)
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, func() bool { return calls.Load() > 1 })
	snapshot := r.Snapshot()
	if snapshot[0].Members[0].Interface != "wan1" || snapshot[0].Members[0].Up || snapshot[0].Active != "" {
		t.Fatalf("stale result/fabricated route: %v", snapshot)
	}
	snapshot[0].Members[0].Interface = "mutated"
	if r.Snapshot()[0].Members[0].Interface != "wan1" {
		t.Fatal("snapshot aliases runtime")
	}
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestRuntimeMonitorIndependenceAndNoOverlap(t *testing.T) {
	blocked := make(chan struct{})
	release := make(chan struct{})
	var first atomic.Int32
	var second atomic.Int32
	r := NewRuntime(func(ctx context.Context, _ string, m *ngfwv1.WanMonitor) CheckResult {
		if m.GetTarget() == "blocked.test" {
			if first.Add(1) == 1 {
				close(blocked)
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return CheckResult{Sent: 1}
		}
		second.Add(1)
		return CheckResult{Sent: 1, Received: 1, AvgLatencyMs: 5}
	})
	group := runtimeGroup()
	group.Monitors[0].Target = proto.String("blocked.test")
	group.Monitors = append(group.Monitors, runtimeGroup().Monitors[0])
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Replace(ctx, []*ngfwv1.WanGroup{group}); err != nil {
		t.Fatal(err)
	}
	<-blocked
	waitRuntime(t, func() bool { return second.Load() >= 2 })
	// The blocked monitor is independently timed out; no next invocation overlaps it.
	if first.Load() < 1 {
		t.Fatal("missing blocked probe")
	}
	if r.Snapshot()[0].Members[0].Up {
		t.Fatal("one successful monitor masked a failed monitor")
	}
	close(release)
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestRuntimeBoundsInvalidConfigurationDoesNotReplace(t *testing.T) {
	var calls atomic.Int32
	allowTick := make(chan struct{})
	tickStarted := make(chan struct{})
	r := NewRuntime(func(ctx context.Context, _ string, _ *ngfwv1.WanMonitor) CheckResult {
		if calls.Load() > 0 {
			select {
			case <-allowTick:
			case <-ctx.Done():
				return CheckResult{Sent: 1}
			}
		}
		if calls.Add(1) == 1 {
			return CheckResult{Sent: 1, Received: 1}
		}
		if calls.Load() == 2 {
			close(tickStarted)
		}
		// Keep the scheduled invocation in flight so no new result can hide a
		// hysteresis reset. Close/incorrect replacement still cancels it normally.
		<-ctx.Done()
		return CheckResult{Sent: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := r.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	group := runtimeGroup()
	group.Monitors[0].TimeoutMs = proto.Uint32(60000)
	if err := r.Replace(ctx, []*ngfwv1.WanGroup{group}); err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].Up })
	r.mu.Lock()
	generation, done := r.generation, r.done
	member := r.groups["edge"].members["wan0"]
	history, since := member.samples[0].state, member.since
	r.mu.Unlock()
	unchanged := func(label string) {
		t.Helper()
		r.mu.Lock()
		current := r.groups["edge"].members["wan0"]
		preserved := r.generation == generation && r.done == done && current == member && current.samples[0].state == history && current.since.Equal(since) && current.up
		r.mu.Unlock()
		if !preserved {
			t.Fatalf("%s replaced generation or reset hysteresis", label)
		}
	}
	groups := make([]*ngfwv1.WanGroup, MaxWorkers+1)
	for i := range groups {
		g := runtimeGroup()
		g.Name = proto.String(time.Unix(int64(i), 0).String())
		groups[i] = g
	}
	if err := r.Replace(ctx, groups); err == nil {
		t.Fatal("accepted over-limit config")
	}
	unchanged("invalid config")
	before := calls.Load()
	close(allowTick)
	select {
	case <-tickStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled probe did not start")
	}
	if calls.Load() <= before {
		t.Fatal("fixture did not exercise legitimate scheduled probe")
	}
	if err := r.Replace(ctx, []*ngfwv1.WanGroup{runtimeGroupWithLongProbe()}); err != nil {
		t.Fatal(err)
	}
	unchanged("identical config")
}

func runtimeGroupWithLongProbe() *ngfwv1.WanGroup {
	group := runtimeGroup()
	group.Monitors[0].TimeoutMs = proto.Uint32(60000)
	return group
}

func TestRuntimeDeviceIdentityChangeInvalidatesHealth(t *testing.T) {
	var blocked atomic.Bool
	r := NewRuntime(func(ctx context.Context, _ string, _ *ngfwv1.WanMonitor) CheckResult {
		if blocked.Load() {
			<-ctx.Done()
			return CheckResult{Sent: 1}
		}
		return CheckResult{Sent: 1, Received: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.ReplaceWithIdentity(ctx, []*ngfwv1.WanGroup{runtimeGroup()}, "device-old"); err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].Up })
	blocked.Store(true)
	if err := r.ReplaceWithIdentity(ctx, []*ngfwv1.WanGroup{runtimeGroup()}, "device-new"); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot()[0].Members[0].Up {
		t.Fatal("device replacement retained old healthy observation")
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSinceTracksAggregateMemberTransitions(t *testing.T) {
	var firstUp atomic.Bool
	var secondUp atomic.Bool
	secondUp.Store(true)
	r := NewRuntime(func(_ context.Context, _ string, monitor *ngfwv1.WanMonitor) CheckResult {
		up := firstUp.Load()
		if monitor.GetTarget() == "second.test" {
			up = secondUp.Load()
		}
		latency := 30
		if up {
			latency = 1
		}
		if monitor.GetTarget() == "second.test" {
			latency = 2
			if up {
				latency = 40
			}
		}
		result := CheckResult{Sent: 1, AvgLatencyMs: latency}
		if up {
			result.Received = 1
		}
		return result
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := r.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	group := runtimeGroup()
	second := proto.Clone(group.Monitors[0]).(*ngfwv1.WanMonitor)
	second.Target = proto.String("second.test")
	group.Monitors = append(group.Monitors, second)
	if err := r.ReplaceWithIdentity(ctx, []*ngfwv1.WanGroup{group}, "original"); err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].LatencyMs == 40 })
	member := r.Snapshot()[0].Members[0]
	if member.Up || member.Since != nil {
		t.Fatalf("individual monitor fabricated member transition: %v", member)
	}
	firstUp.Store(true)
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].Up })
	firstSince := r.Snapshot()[0].Members[0].Since
	if firstSince == nil {
		t.Fatal("first aggregate up transition missing")
	}
	secondUp.Store(false)
	waitRuntime(t, func() bool { return !r.Snapshot()[0].Members[0].Up })
	downSince := r.Snapshot()[0].Members[0].Since
	if downSince == nil || !downSince.AsTime().After(firstSince.AsTime()) {
		t.Fatal("aggregate down did not advance Since")
	}
	firstUp.Store(false)
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].LatencyMs == 30 })
	secondUp.Store(true)
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].LatencyMs == 40 })
	if current := r.Snapshot()[0].Members[0]; current.Up || !proto.Equal(current.Since, downSince) {
		t.Fatal("partial monitor recovery changed aggregate down timestamp")
	}
	if err := r.ReplaceWithIdentity(ctx, []*ngfwv1.WanGroup{group}, "original"); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(r.Snapshot()[0].Members[0].Since, downSince) {
		t.Fatal("identical configuration reset member transition")
	}
	if err := r.ReplaceWithIdentity(ctx, []*ngfwv1.WanGroup{group}, "new-device"); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot()[0].Members[0].Since != nil {
		t.Fatal("new identity retained previous member transition")
	}
}
