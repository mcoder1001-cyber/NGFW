package ravpn

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileTAPReceiptRejectsUntrustedStateParent(t *testing.T) {
	for _, path := range []string{"relative", "/", "/tmp"} {
		if store, err := NewFileTAPReceipts(path); err == nil {
			store.Close()
			t.Fatal("unsafe claim parent accepted")
		}
	}
}

func TestLazyReceiptFailsBeforeAnyVPPMutationOnUnsafeParent(t *testing.T) {
	guard, backend, _, _, endpoint := guardedFixture(t)
	guard.Store = &LazyTAPReceipts{StateDir: "/tmp"}
	if guard.CheckPersistent() != nil {
		t.Fatal("concrete file persistence declaration refused")
	}
	if _, err := guard.Create(context.Background(), endpoint); err == nil {
		t.Fatal("unsafe lazy parent accepted")
	}
	if len(backend.rows) != 0 {
		t.Fatal("VPP mutation preceded parent protection")
	}
	guard.Store = receiptMemory{}
	if guard.CheckPersistent() == nil {
		t.Fatal("volatile claim store accepted")
	}
}
func TestFileTAPReceiptPersistsAndRefusesLinkedOrForeignClaims(t *testing.T) {
	if os.Geteuid() != 0 {
		return
	} // non-root cannot create the required root-owned store
	// /tmp is deliberately rejected by the protected-parent walk. The fixture
	// uses an exclusively created root-private /run child and removes only it.
	dir, err := os.MkdirTemp("/run", "ngfw-ra-receipt-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	store, err := NewFileTAPReceipts(dir)
	if err != nil {
		t.Fatal(err)
	}
	guard, _, plan, boot, endpoint := guardedFixture(t)
	receipt := TAPReceipt{Instance: plan.Instance, NamespaceInode: plan.NamespaceInode, HostNamespaceInode: plan.HostNamespaceInode, Boot: *boot, Index: 19001, Endpoint: endpoint}
	if err = store.Save(endpoint.Name, receipt); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = NewFileTAPReceipts(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	read, err := store.Load(endpoint.Name)
	if err != nil || !receiptMatches(read, endpoint, plan, *boot) {
		t.Fatal("restart claim lost", err)
	}
	foreign := receipt
	foreign.Boot.StartTime++
	if store.Save(endpoint.Name, foreign) == nil {
		t.Fatal("foreign boot overwrote claim")
	}
	path := filepath.Join(dir, "ra-taps", endpoint.Name+".json")
	if os.Link(path, filepath.Join(dir, "alias")) != nil {
		t.Fatal("hardlink fixture")
	}
	if _, err = store.Load(endpoint.Name); err == nil {
		t.Fatal("hardlinked receipt accepted")
	}
	if os.Remove(filepath.Join(dir, "alias")) != nil {
		t.Fatal("hardlink cleanup")
	}
	if store.Remove(endpoint.Name) != nil {
		t.Fatal("own claim removal")
	}
	if os.Symlink("/etc/passwd", path) != nil {
		t.Fatal("symlink fixture")
	}
	if _, err = store.Load(endpoint.Name); err == nil {
		t.Fatal("symlink receipt accepted")
	}
	guard.Store = store
	if _, err = guard.Create(t.Context(), endpoint); err == nil {
		t.Fatal("unsafe existing claim replaced")
	}
}
