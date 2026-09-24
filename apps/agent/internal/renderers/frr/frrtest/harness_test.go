package frrtest

import (
	"context"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/vpp/vpptest"
)

func TestHarnessArgvScoped(t *testing.T) {
	h := &Harness{Paths: frr.TestPaths("w12"), Base: "/run/vrx-test/w12/frr", prefix: "w12", NetNS: "ns-w12-frr"}
	for _, d := range FrameworkDaemons {
		if err := h.assertScopedArgv(h.DaemonArgs(d)); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	bad := append(h.DaemonArgs("zebra"), "-A", "0.0.0.0")
	bad = slices.DeleteFunc(bad, func(s string) bool { return s == "127.0.0.1" })
	if err := h.assertScopedArgv(bad); err == nil {
		t.Error("argv without 127.0.0.1 accepted")
	}
	h.NetNS = ""
	if err := h.assertScopedArgv(h.DaemonArgs("zebra")); err == nil {
		t.Error("root namespace accepted")
	}
}

// ours() must not match a process that merely mentions the socket dir (review M4).
func TestOursNeedsExeAndPidfileArgv(t *testing.T) {
	h := &Harness{Paths: frr.TestPaths("w12"), prefix: "w12"}
	if h.ours("zebra", syscall.Getpid()) {
		t.Error("the test process is not a harness zebra")
	}
}

// M4: a second harness on the same slot waits for the first instead of killing its daemons.
func TestHarnessSlotLockSerialises(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	ctx := context.Background()
	h1, err := start(ctx, Options{Prefix: prefix})
	if err != nil {
		if h1 != nil {
			_ = h1.Stop()
		}
		t.Fatal(err)
	}
	pids := h1.PIDs()
	done := make(chan error, 1)
	var h2 *Harness
	go func() {
		var err error
		h2, err = start(ctx, Options{Prefix: prefix})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("second harness did not wait for the slot lock (err=%v)", err)
	case <-time.After(3 * time.Second):
	}
	for d, pid := range pids {
		if !alive(pid) || !h1.ours(d, pid) {
			t.Errorf("first harness's %s (pid %d) was killed while it held the slot", d, pid)
		}
	}
	t.Logf("M4: second Start blocked ≥3s on %s; first harness daemons %v alive", LockFile(prefix), pids)
	if err := h1.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second harness after release: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("second harness never started")
	}
	pids2 := h2.PIDs()
	if err := h2.Stop(); err != nil {
		t.Fatal(err)
	}
	for d, pid := range pids2 {
		if alive(pid) && strings.Contains(d, "d") && h2.ours(d, pid) {
			t.Errorf("%s still running after Stop", d)
		}
	}
	t.Logf("M4: second harness started after release with %v and stopped", pids2)
}
