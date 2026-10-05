package ravpn

import (
	"bytes"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetsOpenFileExactNumericRecord(t *testing.T) {
	target := bootid.Identity{BootID: "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", PID: 4242, StartTime: 987654}
	data, err := RenderTargetsOpenFile(target)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTargetsOpenFile(data)
	if err != nil || !parsed.Equal(target) {
		t.Fatal("failed exact roundtrip")
	}
	if bytes.Contains(data, []byte("%i")) || !bytes.Contains(data, []byte("/proc/4242/ns/mnt")) || !bytes.Contains(data, []byte("/proc/4242/exe:vpp-exe:read-only")) {
		t.Fatal("missing literal numeric path")
	}
	for _, bad := range [][]byte{append(append([]byte(nil), data...), []byte("ExecStart=/bin/false\n")...), bytes.Replace(data, []byte("pid=4242"), []byte("pid=04242"), 1), bytes.Replace(data, []byte("vpp-mount"), []byte("foreign"), 1)} {
		if _, err := ParseTargetsOpenFile(bad); err == nil {
			t.Fatal("accepted modified configuration")
		}
	}
	target.BootID = "bad\n[Service]"
	if _, err := RenderTargetsOpenFile(target); err == nil {
		t.Fatal("accepted injected boot header")
	}
	if _, err := TargetsOpenFilePath(-1); err == nil {
		t.Fatal("accepted negative PID")
	}
}

func TestTargetsOpenFileReadbackProtectsOwnedFile(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("positive protected root-owned file requires UID 0")
	}
	identity := bootid.Identity{BootID: "12345678-1234-1234-1234-123456789abc", PID: 123, StartTime: 456}
	content, err := RenderTargetsOpenFile(identity)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// #nosec G302 -- private directory needs owner traversal; no group/other access.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "10-openfile.conf")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := readTargetsOpenFileAt(path, identity); err != nil {
		t.Fatal(err)
	}
	other := identity
	other.StartTime++
	if readTargetsOpenFileAt(path, other) == nil {
		t.Fatal("foreign generation accepted")
	}
	// #nosec G302 -- deliberate group-readable negative fixture; production rejects it.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if readTargetsOpenFileAt(path, identity) == nil {
		t.Fatal("group-readable configuration accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Link(path, linked); err != nil {
		t.Fatal(err)
	}
	if readTargetsOpenFileAt(path, identity) == nil {
		t.Fatal("multiply-linked configuration accepted")
	}
	if err := os.Remove(linked); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if readTargetsOpenFileAt(alias, identity) == nil {
		t.Fatal("symlink configuration accepted")
	}
	if err := os.WriteFile(path, append(content, []byte("ExecStart=/bin/sh\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if readTargetsOpenFileAt(path, identity) == nil {
		t.Fatal("extra command accepted")
	}
}
