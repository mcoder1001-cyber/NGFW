package vpntest

import (
	"os"
	"syscall"
	"testing"
)

// GlobalsLock is the VPP-globals lock of docs/lab/shared-host-rules.md §7 (D-082).
const GlobalsLock = "/run/lock/vrx-globals.lock"

// LockGlobals holds GlobalsLock until the test ends: shared to read a VPP-wide setting, exclusive
// (only behind VRX_DF5_GLOBALS=1) to change one.
func LockGlobals(t testing.TB, exclusive bool) {
	t.Helper()
	f, err := os.OpenFile(GlobalsLock, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // shared lock file, no content
	if err != nil {
		t.Fatalf("globals lock: %v", err)
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		t.Fatalf("flock %s: %v", GlobalsLock, err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}

// SkipUnlessGlobals skips a test that changes a getter-less VPP-global (its previous value cannot
// be read back and restored) unless VRX_DF5_GLOBALS=1 — a manager window only (§7, D-082).
func SkipUnlessGlobals(t testing.TB, what string) {
	t.Helper()
	if os.Getenv("VRX_DF5_GLOBALS") != "1" {
		t.Skipf("%s changes a getter-less VPP-global; set VRX_DF5_GLOBALS=1 in a manager window", what)
	}
}
