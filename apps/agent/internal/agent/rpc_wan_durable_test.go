package agent

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/multiwan"
	"testing"
	"time"
)

func TestReviewWANFailedPersistenceMustNotStart(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "baseline", DesiredState: doc(t, `{"routing":{}}`), Subsystems: []string{"routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	saveHook = func(stage string) error {
		if stage == "state" {
			return errors.New("review injected state failure")
		}
		return nil
	}
	t.Cleanup(func() { saveHook = nil })
	resp := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "failed-monitor", DesiredState: doc(t, `{"routing":{"wanGroups":[{"name":"edge","members":[{"interface":"wan0"}],"monitors":[{"type":"http","target":"probe.test","intervalMs":100,"timeoutMs":50,"lossPct":100,"upAfter":1}]}]}}`), Subsystems: []string{"routing"}})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	saveHook = nil
	runtime := multiwan.NewRuntime(func(context.Context, string, *ngfwv1.WanMonitor) multiwan.CheckResult {
		return multiwan.CheckResult{Sent: 1, Received: 1}
	})
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	a := &Agent{svc: s, log: s.log}
	go func() { defer close(stopped); a.watchWAN(ctx, runtime) }()
	t.Cleanup(func() { cancel(); <-stopped })
	deadline := time.Now().Add(time.Second)
	for !runtime.Ready() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(runtime.Snapshot()) != 0 {
		t.Fatal("WAN watcher activated a monitor whose desired-state save failed (Apply DEGRADED)")
	}
}

func TestWanSavedSnapshotFailureMirrorRestartAndIsolation(t *testing.T) {
	st := newState(t.TempDir(), testOwner)
	baseline := doc(t, `{"interfaces":{"wan0":{"lcp":{"hostIfName":"old0"}}},"routing":{"wanGroups":[{"name":"old"}]}}`)
	st.desired = baseline
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	old := st.wanSaved
	st.desired = doc(t, `{"interfaces":{"wan0":{"lcp":{"hostIfName":"new0"}}},"routing":{"wanGroups":[{"name":"new"}]}}`)
	saveHook = func(stage string) error {
		if stage == "state" {
			return errors.New("injected state failure")
		}
		return nil
	}
	t.Cleanup(func() { saveHook = nil })
	if err := st.save(); err == nil {
		t.Fatal("state failure not injected")
	}
	if st.wanSaved != old {
		t.Fatal("failed replacement published monitor snapshot")
	}
	if device, _ := wanDevice(st.wanSaved, "wan0"); device != "old0" {
		t.Fatalf("failed replacement changed device: %s", device)
	}
	st.desired = &ngfwv1.DesiredState{}
	if err := st.save(); err == nil {
		t.Fatal("removal failure not injected")
	}
	if st.wanSaved != old {
		t.Fatal("failed removal discarded saved monitors")
	}
	st.desired = doc(t, `{"interfaces":{"wan0":{"lcp":{"hostIfName":"new0"}}},"routing":{"wanGroups":[{"name":"new"}]}}`)
	saveHook = func(stage string) error {
		if stage == "mirror" {
			return errors.New("injected mirror failure")
		}
		return nil
	}
	if err := st.save(); !errors.Is(err, errMirror) {
		t.Fatalf("mirror error: %v", err)
	}
	if st.wanSaved.GetRouting().GetWanGroups()[0].GetName() != "new" {
		t.Fatal("durable mirror failure did not publish monitors")
	}
	if device, _ := wanDevice(old, "wan0"); device != "old0" {
		t.Fatal("new save mutated an existing probe generation")
	}
	st.desired.Interfaces["wan0"].Lcp.HostIfName = proto.String("unsaved0")
	if device, _ := wanDevice(st.wanSaved, "wan0"); device != "new0" {
		t.Fatal("saved snapshot aliases candidate interfaces")
	}
	saveHook = nil
	restarted, err := loadState(st.dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(restarted.wanSaved, st.wanSaved) {
		t.Fatal("restart lost durable WAN input")
	}
	st.desired = &ngfwv1.DesiredState{}
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	if len(st.wanSaved.GetRouting().GetWanGroups()) != 0 {
		t.Fatal("successful removal retained monitors")
	}
}

func TestWanSavedSnapshotConfirmedRollback(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "baseline-wan", DesiredState: doc(t, `{"routing":{}}`), Subsystems: []string{"routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	ds := doc(t, `{"routing":{"wanGroups":[{"name":"edge","members":[{"interface":"wan0"}],"monitors":[{"type":"http","target":"probe.test","intervalMs":100,"timeoutMs":50,"lossPct":100,"upAfter":1}]}]}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "pending-wan", DesiredState: ds, Subsystems: []string{"routing"}, ConfirmTimeoutSec: 60}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(s.st.wanSaved.GetRouting().GetWanGroups()) != 1 {
		t.Fatal("persisted pending monitor absent")
	}
	s.revert("pending-wan")
	if len(s.st.wanSaved.GetRouting().GetWanGroups()) != 0 {
		t.Fatal("confirmed rollback retained candidate monitor")
	}
	restarted, err := loadState(s.st.dir, testOwner)
	if err != nil || len(restarted.wanSaved.GetRouting().GetWanGroups()) != 0 {
		t.Fatalf("rollback not durable: %v", err)
	}
}
