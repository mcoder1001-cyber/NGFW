package ravpn

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSnapshotPrivateGroupDoesNotRequireChown(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root-owned fixture files")
	}
	root := t.TempDir()
	path := filepath.Join(root, "credential")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 65534); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	if _, err := snapshotStat(fd, "credential", false); err != nil {
		t.Fatalf("root-private material with inaccessible group: %v", err)
	}
	// #nosec G302 -- deliberately incorrect snapshot file group mode exercises strict rejection; restored private mode follows.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotStat(fd, "credential", false); err == nil {
		t.Fatal("accepted group-readable material")
	}
	directory := filepath.Join(root, "private")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 0, 65534); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotStat(fd, "private", true); err != nil {
		t.Fatalf("root-private directory with inaccessible group: %v", err)
	}
	// #nosec G302 -- deliberately group-searchable credential directory tests strict refusal.
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotStat(fd, "private", true); err == nil {
		t.Fatal("accepted group-searchable credential directory")
	}
}
