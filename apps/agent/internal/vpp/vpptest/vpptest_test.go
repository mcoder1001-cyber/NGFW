package vpptest

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSlotDerivedValues(t *testing.T) {
	t.Setenv(EnvIntegration, "")
	t.Setenv(EnvPrefix, "w2")
	t.Setenv(EnvSlot, "")
	t.Setenv(EnvTableBase, "")
	if Prefix(t) != "w2" || Slot(t) != 2 {
		t.Fatalf("prefix/slot = %q/%d", Prefix(t), Slot(t))
	}
	if Name(t, "loop") != "w2-loop" {
		t.Fatalf("Name = %q", Name(t, "loop"))
	}
	if LoopbackInstance(t, 1) != 201 || LoopbackInstance(t, 99) != 299 {
		t.Fatalf("LoopbackInstance = %d", LoopbackInstance(t, 1))
	}
	if TableBase(t) != 2000 {
		t.Fatalf("TableBase = %d", TableBase(t))
	}
	if NATPool(t) != "10.2.0.0/16" {
		t.Fatalf("NATPool = %q", NATPool(t))
	}
	t.Setenv(EnvTableBase, "7000")
	t.Setenv(EnvSlot, "7")
	if TableBase(t) != 7000 || Slot(t) != 7 {
		t.Fatal("explicit VRX_VPP_TABLE_BASE / VRX_SLOT must win")
	}
}

func TestUnitDefaultsWithoutEnv(t *testing.T) {
	t.Setenv(EnvIntegration, "")
	t.Setenv(EnvPrefix, "")
	if Prefix(t) != "w0" {
		t.Fatalf("unit default prefix = %q", Prefix(t))
	}
	if Integration() {
		t.Fatal("Integration() true without VRX_INTEGRATION=1")
	}
}

func TestSkipUnlessIntegration(t *testing.T) {
	t.Setenv(EnvIntegration, "")
	t.Run("skips", func(t *testing.T) {
		SkipUnlessIntegration(t)
		t.Fatal("must have skipped")
	})
}

func TestLockLabSharedAndReleased(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "lab.lock")
	t.Setenv(EnvLabLock, lock)
	t.Run("hold", func(t *testing.T) {
		LockLab(t)
		// A second shared lock must succeed while the first is held.
		f, err := os.Open(lock)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
			t.Fatalf("second shared lock: %v", err)
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	})
	// After the subtest's Cleanup the exclusive lock must be obtainable.
	f, err := os.Open(lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock not released after test: %v", err)
	}
}
