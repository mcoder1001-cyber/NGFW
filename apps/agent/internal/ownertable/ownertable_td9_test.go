package ownertable

import (
	"os"
	"path/filepath"
	"testing"
)

// TD-9 (review 1.5a): WriteAtomic fsyncs the directory after the rename, so the rename survives a
// power loss; the new content is in place when the directory is synced.
func TestWriteAtomicSyncsTheDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-state.json")
	var synced []string
	restore := SetSyncDir(func(d string) error {
		b, err := os.ReadFile(path) //nolint:gosec // test file
		if err != nil || string(b) != "new\n" {
			t.Errorf("directory synced before the rename: %q %v", b, err)
		}
		synced = append(synced, d)
		return SyncDir(d)
	})
	defer restore()
	if err := WriteAtomic(path, []byte("new\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != dir {
		t.Fatalf("directory fsyncs %v, want [%s]", synced, dir)
	}
	// The owner table writes through it.
	f, err := Open(dir, "w7")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Add("ip.route/0/10.7.9.0/24"); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 2 {
		t.Fatalf("owner table write: directory fsyncs %v", synced)
	}
}
