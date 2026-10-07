package ravpn

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNamespacePlaceholderBirthRefusesReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root-owned disposable placeholder")
	}
	root, err := os.MkdirTemp("/root", "ra-birth-")
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
	var original unix.Stat_t
	if unix.Lstat(path, &original) != nil {
		t.Fatal("missing placeholder")
	}
	identity := namespaceBirthIdentity{Device: uint64(original.Dev), Inode: original.Ino}
	if verifyNamespaceBirthIdentity(path, identity) != nil {
		t.Fatal("refused original birth")
	}
	// Keep the original inode allocated so the replacement cannot recycle it.
	old := filepath.Join(root, "original")
	if os.Rename(path, old) != nil || os.WriteFile(path, nil, 0600) != nil {
		t.Fatal("replacement setup")
	}
	if verifyNamespaceBirthIdentity(path, identity) == nil {
		t.Fatal("adopted foreign root600 replacement")
	}
	if os.Remove(path) != nil || os.Link(old, path) != nil {
		t.Fatal("hardlink setup")
	}
	if verifyNamespaceBirthIdentity(path, identity) == nil {
		t.Fatal("accepted multiply linked original inode")
	}
	if os.Remove(path) != nil || os.Symlink(old, path) != nil {
		t.Fatal("symlink setup")
	}
	if verifyNamespaceBirthIdentity(path, identity) == nil {
		t.Fatal("followed foreign symlink")
	}
}
