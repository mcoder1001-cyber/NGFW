// Package vpptest holds the shared-host test helpers every descriptor and renderer
// integration test uses (docs/lab/shared-host-rules.md): the VRX_INTEGRATION gate, the
// slot-derived names and ranges, and the shared lab lock. It deliberately does not open a VPP
// connection — that is P05's client; until then integration tests connect with govpp directly.
//
//	func TestLoopbackIntegration(t *testing.T) {
//		vpptest.SkipUnlessIntegration(t)
//		vpptest.LockLab(t)                       // flock -s /run/lock/vrx-lab.lock, released in Cleanup
//		name := vpptest.Name(t, "loop")          // "w2-loop"
//		inst := vpptest.LoopbackInstance(t, 1)   // 201 → VPP names it loop201
//		vrf := vpptest.TableBase(t) + 1          // 2001
//		t.Cleanup(func() { /* delete exactly what you created, via binapi */ })
//		...
//	}
package vpptest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Environment variables set by the task envelope / tools/lab env <N>.
const (
	EnvIntegration = "VRX_INTEGRATION"
	EnvPrefix      = "VRX_TEST_PREFIX"
	EnvSlot        = "VRX_SLOT"
	EnvTableBase   = "VRX_VPP_TABLE_BASE"
	EnvLabLock     = "VRX_LAB_LOCK"
)

// DefaultLabLock is the shared lab lock file; integration harnesses take it shared, the
// manager's full CI and (after handover) VPP restarts take it exclusive.
const DefaultLabLock = "/run/lock/vrx-lab.lock"

// Integration reports whether integration tests are enabled (VRX_INTEGRATION=1).
func Integration() bool { return os.Getenv(EnvIntegration) == "1" }

// SkipUnlessIntegration skips t unless VRX_INTEGRATION=1. Unit-only runs (make test,
// tools/ci.sh) must never touch VPP, daemons or the database.
func SkipUnlessIntegration(t testing.TB) {
	t.Helper()
	if !Integration() {
		t.Skip("integration test: set VRX_INTEGRATION=1 (and run under the shared lab lock)")
	}
}

// Prefix returns the worker's VRX_TEST_PREFIX ("w2"). Every object a test creates on the
// shared VPP, in the database or as a process carries it. The test fails when it is unset in
// integration mode; unit tests get "w0".
func Prefix(t testing.TB) string {
	t.Helper()
	p := os.Getenv(EnvPrefix)
	switch {
	case p == "" && Integration():
		t.Fatalf("%s is not set: refuse to create unprefixed objects on the shared host", EnvPrefix)
	case p == "":
		return "w0"
	case len(p) > 6:
		t.Fatalf("%s=%q is longer than 6 characters (IFNAMSIZ budget)", EnvPrefix, p)
	}
	return p
}

// Slot returns the numeric slot N: VRX_SLOT when set, otherwise parsed from the prefix "w<N>".
func Slot(t testing.TB) int {
	t.Helper()
	if s := os.Getenv(EnvSlot); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			t.Fatalf("%s=%q is not a slot number", EnvSlot, s)
		}
		return n
	}
	p := Prefix(t)
	n, err := strconv.Atoi(strings.TrimPrefix(p, "w"))
	if err != nil || !strings.HasPrefix(p, "w") {
		t.Fatalf("cannot derive the slot from %s=%q; set %s", EnvPrefix, p, EnvSlot)
	}
	return n
}

// Name returns "<prefix>-<suffix>", the naming rule for host-interfaces, veths, namespaces
// and daemon instances. It fails when the result exceeds 15 bytes (Linux IFNAMSIZ).
func Name(t testing.TB, suffix string) string {
	t.Helper()
	n := Prefix(t) + "-" + suffix
	if len(n) > 15 {
		t.Fatalf("name %q exceeds IFNAMSIZ (15)", n)
	}
	return n
}

// LoopbackInstance returns the loopback instance number for index i (0–99) in this slot's
// range: slot*100 + i, so VPP names the interface loop<slot><ii>.
func LoopbackInstance(t testing.TB, i int) uint32 {
	t.Helper()
	if i < 0 || i > 99 {
		t.Fatalf("loopback index %d outside the slot range 0–99", i)
	}
	return uint32(Slot(t)*100 + i) //nolint:gosec // bounded above
}

// TableBase returns the first VRF/table id this slot may allocate (VRX_VPP_TABLE_BASE,
// default slot*1000). A worker may use TableBase()..TableBase()+999.
func TableBase(t testing.TB) uint32 {
	t.Helper()
	if s := os.Getenv(EnvTableBase); s != "" {
		n, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			t.Fatalf("%s=%q is not a table id", EnvTableBase, s)
		}
		return uint32(n)
	}
	return uint32(Slot(t) * 1000) //nolint:gosec // slots are 1–12
}

// NATPool returns this slot's NAT address block "10.<slot>.0.0/16".
func NATPool(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("10.%d.0.0/16", Slot(t))
}

// LockLab takes the shared lab lock (flock -s on VRX_LAB_LOCK or DefaultLabLock) for the
// rest of the test and releases it in t.Cleanup. It blocks while the manager holds the
// exclusive lock (full CI, VPP restart). Call it right after SkipUnlessIntegration.
func LockLab(t testing.TB) {
	t.Helper()
	path := os.Getenv(EnvLabLock)
	if path == "" {
		path = DefaultLabLock
	}
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // lock file, no content
	if err != nil {
		t.Fatalf("open lab lock %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		t.Fatalf("flock -s %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}
