package ravpn

import (
	"bytes"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"testing"
)

func TestNumericOpenFilePrivateMatchedReuseAndForeignPreservation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires private root-owned fixture")
	}
	root, err := os.MkdirTemp("/root", "ngfw-ra-openfile-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(root, "ngfw-ra-targets@42.service.d", "10-openfile.conf")
	data, err := RenderTargetsOpenFile(bootid.Identity{BootID: "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", PID: 42, StartTime: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := publishNumericOpenFileAt(path, data); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishNumericOpenFileAt(path, data); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("matching reuse replaced inode")
	}
	foreign := []byte("# foreign configuration\n")
	if err := os.WriteFile(path, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if publishNumericOpenFileAt(path, data) == nil {
		t.Fatal("replaced unknown contents")
	}
	// #nosec G304 -- path is derived by the fixed numeric publisher writer under the newly generated private fixture root; verifies preserved bytes.
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, foreign) {
		t.Fatal("modified foreign configuration")
	}
}

func TestNumericOpenFileReplacementChecksExpectedPrivateContent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires private root-owned fixture")
	}
	root, err := os.MkdirTemp("/root", "ngfw-ra-refresh-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(root, "ngfw-ra-targets@42.service.d", "10-openfile.conf")
	target := bootid.Identity{BootID: "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", PID: 42, StartTime: 1}
	old, err := RenderTargetsOpenFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishNumericOpenFileAt(path, old); err != nil {
		t.Fatal(err)
	}
	target.StartTime = 2
	replacement, err := RenderTargetsOpenFile(target)
	if err != nil {
		t.Fatal(err)
	}
	// This exercises the filesystem commit only; public refresh additionally
	// requires canonical current target, old generation gone and supplier empty.
	if err := replaceNumericOpenFileAt(path, old, replacement); err != nil {
		t.Fatal(err)
	}
	if err := replaceNumericOpenFileAt(path, old, replacement); err == nil {
		t.Fatal("accepted stale expected contents")
	}
	// #nosec G302 -- deliberately insecure private drop-in mode tests refusal without modifying its contents.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := replaceNumericOpenFileAt(path, replacement, old); err == nil {
		t.Fatal("replaced nonprivate configuration")
	}
	// #nosec G304 -- path is derived by the fixed numeric publisher writer under the newly generated private fixture root; verifies preserved bytes.
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, replacement) {
		t.Fatal("modified refused configuration")
	}
}
