package ravpn

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func targetShapeFixture(t *testing.T) *NamespaceTargetSnapshot {
	t.Helper()
	source := (bootid.Reader{}).ForPID(os.Getpid())
	snapshot := &NamespaceTargetSnapshot{Source: source}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := range snapshot.Files {
		file, err := os.Open("/proc/self/ns/mnt")
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Files[i] = file
		var stat unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
			t.Fatal(err)
		}
		snapshot.Targets[i] = MountTarget{Boot: source, MountInode: stat.Ino}
	}
	// This fixture checks role shape/equality only; it does not assert that the
	// manager's actual namespace equals the test process namespace.
	snapshot.Targets[1].Boot = bootid.Identity{BootID: source.BootID, PID: 1, StartTime: 1}
	for i := range snapshot.Executables {
		file, err := os.Open("/usr/bin/true")
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Executables[i] = file
	}
	return snapshot
}

func TestNamespaceTargetsCompareHeldImagesAndCompleteGenerations(t *testing.T) {
	before, after := targetShapeFixture(t), targetShapeFixture(t)
	if !before.SameTargets(after) {
		t.Fatal("equal held role shapes refused")
	}
	after.Source.StartTime++
	if before.SameTargets(after) {
		t.Fatal("changed source generation accepted")
	}
	after.Source = before.Source
	different, err := os.Open("/usr/bin/false")
	if err != nil {
		t.Fatal(err)
	}
	if err := after.Executables[0].Close(); err != nil {
		t.Fatal(err)
	}
	after.Executables[0] = different
	if before.SameTargets(after) {
		t.Fatal("changed target executable accepted")
	}
	if err := after.Close(); err != nil {
		t.Fatal(err)
	}
	if after.Validate() == nil {
		t.Fatal("closed descriptor roles accepted")
	}
	if err := after.Close(); err != nil {
		t.Fatal("close not idempotent", err)
	}
	if (*NamespaceTargetSnapshot)(nil).SameTargets(before) {
		t.Fatal("nil snapshot accepted")
	}
}
