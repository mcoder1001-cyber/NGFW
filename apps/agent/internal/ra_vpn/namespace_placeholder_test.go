package ravpn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrokerPlaceholderRootOwnerWithPrivateNonzeroGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root ownership in a disposable private directory")
	}
	root, err := os.MkdirTemp("/root", "ngfw-ra-placeholder-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(root, "binding")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 65534); err != nil {
		t.Fatal(err)
	}
	if err := brokerBindingState(path, 1, true); err != nil {
		t.Fatalf("root-only placeholder with inaccessible group: %v", err)
	}
	// #nosec G302 -- deliberately insecure placeholder mode must be refused before namespace mutation.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if brokerBindingState(path, 1, true) == nil {
		t.Fatal("accepted group-readable placeholder")
	}
}
