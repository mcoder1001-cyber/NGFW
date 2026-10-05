package strongswan

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRASocketRestrictionPinsInodeAndVerifiedPeer(t *testing.T) {
	if os.Geteuid() != 0 {
		if RestrictRAVICISocket(context.Background(), filepath.Join(t.TempDir(), "vici.sock"), os.Getpid()) == nil {
			t.Fatal("non-root restriction accepted")
		}
		return
	}
	root, err := os.MkdirTemp("/run", "r19s-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	path := filepath.Join(root, "vici.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if os.Chmod(path, 0660) != nil {
		t.Fatal("fixture socket mode")
	}
	if RestrictRAVICISocket(context.Background(), path, os.Getpid()+1) == nil {
		t.Fatal("foreign peer socket restricted")
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0660 {
		t.Fatal("wrong peer changed mode")
	}
	alias := filepath.Join(root, "alias")
	if os.Link(path, alias) != nil {
		t.Fatal("hardlink fixture")
	}
	if RestrictRAVICISocket(context.Background(), path, os.Getpid()) == nil {
		t.Fatal("hardlinked socket accepted")
	}
	if os.Remove(alias) != nil {
		t.Fatal("hardlink cleanup")
	}
	if RestrictRAVICISocket(context.Background(), path, os.Getpid()) != nil {
		t.Fatal("owned socket inode restriction failed")
	}
	stat, _ = os.Stat(path)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("socket remained group accessible")
	}
	regular := filepath.Join(root, "regular")
	if os.WriteFile(regular, []byte("public fixture"), 0644) != nil {
		t.Fatal("regular file fixture")
	}
	symlink := filepath.Join(root, "symlink")
	if os.Symlink(regular, symlink) != nil {
		t.Fatal("symlink fixture")
	}
	if RestrictRAVICISocket(context.Background(), symlink, os.Getpid()) == nil {
		t.Fatal("symlink target accepted")
	}
	stat, _ = os.Stat(regular)
	if stat.Mode().Perm() != 0644 {
		t.Fatal("unrelated regular file permissions changed")
	}
}
