package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishSourceGenerationAndForeignPointerRefusal(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires private root-owned fixture")
	}
	root, err := os.MkdirTemp("/root", "ngfw-ra-generation-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	if err := publishSourceAgentGeneration(context.Background(), root, identity); err != nil {
		t.Fatal(err)
	}
	if err := publishSourceAgentGeneration(context.Background(), root, identity); err != nil {
		t.Fatalf("idempotent current generation: %v", err)
	}
	current := filepath.Join(root, "source-agent-current")
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("foreign", current); err != nil {
		t.Fatal(err)
	}
	if publishSourceAgentGeneration(context.Background(), root, identity) == nil {
		t.Fatal("adopted foreign pointer")
	}
	target, err := os.Readlink(current)
	if err != nil || target != "foreign" {
		t.Fatal("modified foreign pointer")
	}
}
