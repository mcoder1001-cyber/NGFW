package dfkit_test

import (
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/ownertable"
)

// TD-9 (review 1.5a): the boot store's atomic replace fsyncs its directory after the rename (through
// the shared ownertable.WriteAtomic helper).
func TestFileBootStoreSyncsItsDirectory(t *testing.T) {
	dir := t.TempDir()
	var synced []string
	restore := ownertable.SetSyncDir(func(d string) error { synced = append(synced, d); return ownertable.SyncDir(d) })
	defer restore()
	s, err := dfkit.NewFileBootStore(filepath.Join(dir, "boot-w7.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(dfkit.BootRecord{Key: "pcap.capture/global", Identity: "a/1/2", Value: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("pcap.capture/global"); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 2 || synced[0] != dir || synced[1] != dir {
		t.Fatalf("directory fsyncs %v, want 2 × %s", synced, dir)
	}
}
