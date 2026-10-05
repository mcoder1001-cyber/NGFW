package ravpn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

func TestSourceAgentReferenceOwnershipAndProcessBinding(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root ownership in a disposable temporary directory")
	}
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	// Production rejects writable ancestors, including /tmp and /dev/shm.
	// Use an exclusively created root-private fixture and retain those checks.
	root, err := os.MkdirTemp("/root", "ngfw-ra-source-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	generation := sourceAgentGeneration(identity)
	dir := filepath.Join(root, generation)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"source-agent.json": "source-agent-current/identity.json", "source-agent-exe": "source-agent-current/exe", "source-agent-current": generation} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	record := filepath.Join(dir, "identity.json")
	link := filepath.Join(dir, "exe")
	data, err := json.Marshal(sourceAgentReferenceRecord{Source: identity, Version: 1, Owner: "ngfw-ra-source"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/proc/"+strconv.Itoa(identity.PID)+"/exe", link); err != nil {
		t.Fatal(err)
	}
	if err := readSourceAgentReferenceAt(root, identity); err != nil {
		t.Fatalf("valid root-private reference: %v", err)
	}
	if err := os.Chmod(record, 0644); err != nil {
		t.Fatal(err)
	}
	if readSourceAgentReferenceAt(root, identity) == nil {
		t.Fatal("accepted non-private record")
	}
	if err := os.Chmod(record, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/proc/1/exe", link); err != nil {
		t.Fatal(err)
	}
	if readSourceAgentReferenceAt(root, identity) == nil {
		t.Fatal("accepted foreign executable reference")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/proc/"+strconv.Itoa(identity.PID)+"/exe", link); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(record, filepath.Join(root, "duplicate-record")); err != nil {
		t.Fatal(err)
	}
	if readSourceAgentReferenceAt(root, identity) == nil {
		t.Fatal("accepted multiply linked identity record")
	}
}
