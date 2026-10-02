package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/multiwan"
)

func TestWanRPCObservedStateFilteringAndOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := multiwan.NewRuntime(func(context.Context, string, *vrxv1.WanMonitor) multiwan.CheckResult {
		return multiwan.CheckResult{Sent: 1, Received: 1, AvgLatencyMs: 17}
	})
	group := func(name string) *vrxv1.WanGroup {
		return &vrxv1.WanGroup{Name: proto.String(name), Mode: proto.String("failover"), Members: []*vrxv1.WanMember{{Interface: proto.String("wan0"), Weight: proto.Uint32(1)}}, Monitors: []*vrxv1.WanMonitor{{Type: proto.String("http"), Target: proto.String("example.test"), IntervalMs: proto.Uint32(100), TimeoutMs: proto.Uint32(50), LossPct: proto.Uint32(100), UpAfter: proto.Uint32(1)}}}
	}
	if err := runtime.Replace(ctx, []*vrxv1.WanGroup{group("a"), group("z")}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runtime.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	server := &server{svc: &Service{owner: "w7", now: time.Now}, wan: runtime}
	deadline := time.Now().Add(time.Second)
	for !runtime.Snapshot()[0].Members[0].Up && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	response, err := server.WanState(ctx, &vrxv1.WanStateRequest{Owner: "w7", Groups: []string{"a", "a"}})
	if err != nil || len(response.GetGroups()) != 1 || response.GetGroups()[0].GetName() != "a" || response.GetGroups()[0].GetActive() != "" || !response.GetGroups()[0].GetMembers()[0].GetUp() || response.GetGroups()[0].GetMembers()[0].GetLatencyMs() != 17 {
		t.Fatalf("unexpected scoped state: %v %v", response, err)
	}
	if _, err := server.WanState(ctx, &vrxv1.WanStateRequest{Owner: "foreign"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}
	if _, err := server.WanState(ctx, &vrxv1.WanStateRequest{Groups: []string{"missing"}}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown group: %v", err)
	}
	server.wan = nil
	if _, err := server.WanState(ctx, &vrxv1.WanStateRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("unwired runtime: %v", err)
	}
}

func TestWanDeviceFailClosedForMissingNamespaceAndVRF(t *testing.T) {
	saved := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"wan0": {Lcp: &vrxv1.InterfaceLcp{HostIfName: proto.String("lcp0")}}}}
	if device, err := wanDevice(saved, "wan0"); err != nil || device != "lcp0" {
		t.Fatalf("device %s %v", device, err)
	}
	if _, err := wanDevice(saved, "missing"); err == nil {
		t.Fatal("missing interface allowed unbound probe")
	}
	saved.Interfaces["wan0"].Lcp.Netns = proto.String("other")
	if _, err := wanDevice(saved, "wan0"); err == nil {
		t.Fatal("namespace probe allowed")
	}
	saved.Interfaces["wan0"].Lcp.Netns = nil
	saved.Interfaces["wan0"].Vrf = proto.String("blue")
	if _, err := wanDevice(saved, "wan0"); err == nil {
		t.Fatal("nondefault VRF allowed")
	}
}

func TestWanRuntimeReadsDurableApplyAndRemoval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	service := newSvc(t, coretest.New(), t.TempDir())
	runtime := multiwan.NewRuntime(func(context.Context, string, *vrxv1.WanMonitor) multiwan.CheckResult {
		return multiwan.CheckResult{Sent: 1, Received: 1}
	})
	agent := &Agent{svc: service, log: service.log}
	stopped := make(chan struct{})
	go func() { defer close(stopped); agent.watchWAN(ctx, runtime) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Error("WAN watcher did not drain")
		}
	})
	desired := doc(t, `{"routing":{"wanGroups":[{"name":"edge","mode":"failover","members":[{"interface":"wan0","weight":1,"priority":10}],"monitors":[{"type":"http","target":"probe.test","intervalMs":100,"timeoutMs":50,"lossPct":100,"upAfter":1,"downAfter":1}]}]}}`)
	response := apply(t, service, &vrxv1.ApplyRequest{TxnId: "wan-add", DesiredState: desired, Subsystems: []string{"routing"}})
	mustStatus(t, response, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	deadline := time.Now().Add(3 * time.Second)
	for len(runtime.Snapshot()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(runtime.Snapshot()) != 1 {
		t.Fatal("committed WAN group never started")
	}
	response = apply(t, service, &vrxv1.ApplyRequest{TxnId: "wan-remove", DesiredState: doc(t, `{"routing":{}}`), Subsystems: []string{"routing"}})
	mustStatus(t, response, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	deadline = time.Now().Add(3 * time.Second)
	for len(runtime.Snapshot()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(runtime.Snapshot()) != 0 {
		t.Fatal("committed removal retained WAN monitors")
	}
}

func TestWanRPCPolicyUnavailableIsScopedAndRecovers(t *testing.T) {
	var allowed atomic.Bool
	runtime := multiwan.NewRuntime(func(_ context.Context, member string, _ *vrxv1.WanMonitor) multiwan.CheckResult {
		if member == "restricted" && !allowed.Load() {
			return multiwan.CheckResult{Sent: 1, Unavailable: true}
		}
		return multiwan.CheckResult{Sent: 1, Received: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	groups := []*vrxv1.WanGroup{}
	for _, name := range []string{"restricted", "normal"} {
		groups = append(groups, &vrxv1.WanGroup{Name: proto.String(name), Members: []*vrxv1.WanMember{{Interface: proto.String(name)}}, Monitors: []*vrxv1.WanMonitor{{Type: proto.String("icmp"), IntervalMs: proto.Uint32(100), TimeoutMs: proto.Uint32(50), LossPct: proto.Uint32(100), UpAfter: proto.Uint32(1)}}})
	}
	if err := runtime.Replace(ctx, groups); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, unavailable := runtime.SnapshotWithAvailability()
		if unavailable["restricted"] {
			break
		}
		time.Sleep(time.Millisecond)
	}
	server := &server{svc: &Service{owner: "w7", now: time.Now}, wan: runtime}
	if _, err := server.WanState(ctx, &vrxv1.WanStateRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("policy failure exposed as packet loss: %v", err)
	}
	if response, err := server.WanState(ctx, &vrxv1.WanStateRequest{Groups: []string{"normal"}}); err != nil || len(response.GetGroups()) != 1 {
		t.Fatalf("unrelated group unavailable: %v %v", response, err)
	}
	allowed.Store(true)
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, unavailable := runtime.SnapshotWithAvailability()
		if !unavailable["restricted"] {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if response, err := server.WanState(ctx, &vrxv1.WanStateRequest{Groups: []string{"restricted"}}); err != nil || !response.GetGroups()[0].GetMembers()[0].GetUp() {
		t.Fatalf("recovered permission not observed: %v %v", response, err)
	}
}
