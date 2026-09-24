package interfaces

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestMkdirSharedModes (review N2, D-106/D-107): missing directories are created 0755 even under umask 077, an existing
// directory keeps its mode.
func TestMkdirSharedModes(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	dir := filepath.Join(existing, "vrx-test", "w1")
	if err := mkdirShared(dir); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{existing: 0o700, filepath.Dir(dir): 0o755, dir: 0o755} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s: mode %o, want %o", path, got, want)
		}
	}
	if err := os.Chmod(dir, 0o711); err != nil {
		t.Fatal(err)
	}
	if err := mkdirShared(dir); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o711 {
		t.Errorf("an existing directory was re-moded to %o", fi.Mode().Perm())
	}
}

// TestOurTraceOnlyOurBlock (review N3): a trace buffer without our run-unique echo request is "not found", and the
// block returned is exactly ours (nodes of other packets never count).
func TestOurTraceOnlyOurBlock(t *testing.T) {
	other := `Packet 1

00:00:01:000001: af-packet-input
  af_packet: hw_if_index 2 rx-queue 0 next-index 4
00:00:01:000002: ip4-input
  ICMP: 10.1.1.2 -> 10.1.2.2
    tos 0x00, ttl 64, length 84, checksum 0x0000 dscp CS0 ecn NON_ECN
  ICMP echo_request checksum 0x1 id 1
00:00:01:000003: ip4-lookup
00:00:01:000004: ip4-rewrite
00:00:01:000005: host-w1w0-output
`
	if blk, ok := ourTrace(other, "10.1.1.2", "10.1.2.2", 693); ok {
		t.Fatalf("found a block for another packet:\n%s", blk)
	}
	ours := strings.ReplaceAll(strings.ReplaceAll(other, "Packet 1", "Packet 2"), "length 84,", "length 693,")
	ours = strings.Replace(ours, "host-w1w0-output", "error-drop", 1)
	blk, ok := ourTrace(other+"\n"+ours, "10.1.1.2", "10.1.2.2", 693)
	if !ok || !strings.HasPrefix(blk, "Packet 2") || strings.Contains(blk, "host-w1w0-output") {
		t.Fatalf("ok=%v block:\n%s", ok, blk)
	}
}
