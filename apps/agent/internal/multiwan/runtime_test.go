package multiwan

import (
	"context"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
	"testing"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

func runtimeGroup() *vrxv1.WanGroup {
	return &vrxv1.WanGroup{Name: proto.String("edge"), Mode: proto.String("failover"), Members: []*vrxv1.WanMember{{Interface: proto.String("wan0"), Weight: proto.Uint32(1), Priority: proto.Uint32(1)}}, Monitors: []*vrxv1.WanMonitor{{Type: proto.String("http"), Target: proto.String("example.test"), IntervalMs: proto.Uint32(100), TimeoutMs: proto.Uint32(50), LossPct: proto.Uint32(100), DownAfter: proto.Uint32(1), UpAfter: proto.Uint32(1)}}}
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
	r := NewRuntime(func(ctx context.Context, _ string, _ *vrxv1.WanMonitor) CheckResult {
		if calls.Add(1) == 1 {
			close(first)
			<-release
			return CheckResult{Sent: 1, Received: 1}
		}
		return CheckResult{Sent: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Replace(ctx, []*vrxv1.WanGroup{runtimeGroup()}); err != nil {
		t.Fatal(err)
	}
	<-first
	group := runtimeGroup()
	group.Members[0].Interface = proto.String("wan1")
	replaced := make(chan error, 1)
	go func() { replaced <- r.Replace(ctx, []*vrxv1.WanGroup{group}) }()
	select {
	case <-replaced:
		t.Fatal("replacement did not drain old probe")
	case <-time.After(20 * time.Millisecond):
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
	r := NewRuntime(func(ctx context.Context, _ string, m *vrxv1.WanMonitor) CheckResult {
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
	if err := r.Replace(ctx, []*vrxv1.WanGroup{group}); err != nil {
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
	r := NewRuntime(func(context.Context, string, *vrxv1.WanMonitor) CheckResult {
		calls.Add(1)
		return CheckResult{Sent: 1, Received: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Replace(ctx, []*vrxv1.WanGroup{runtimeGroup()}); err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, func() bool { return r.Snapshot()[0].Members[0].Up })
	groups := make([]*vrxv1.WanGroup, MaxWorkers+1)
	for i := range groups {
		g := runtimeGroup()
		g.Name = proto.String(time.Unix(int64(i), 0).String())
		groups[i] = g
	}
	if err := r.Replace(ctx, groups); err == nil {
		t.Fatal("accepted over-limit config")
	}
	if len(r.Snapshot()) != 1 || !r.Snapshot()[0].Members[0].Up {
		t.Fatal("invalid config disturbed healthy current generation")
	}
	before := calls.Load()
	if err := r.Replace(ctx, []*vrxv1.WanGroup{runtimeGroup()}); err != nil {
		t.Fatal(err)
	}
	if !r.Snapshot()[0].Members[0].Up || calls.Load() != before {
		t.Fatal("identical config reset hysteresis")
	}
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
