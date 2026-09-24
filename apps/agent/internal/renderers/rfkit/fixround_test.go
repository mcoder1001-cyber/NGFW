package rfkit

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

// Review L1: whole-token redaction only.
func TestRedactWholeTokensOnly(t *testing.T) {
	var r Redactor
	r.Add("public12", "VRX_TEST_PSK_RF4_x")
	for in, want := range map[string]string{
		"community public12 refused":             "community <redacted> refused",
		"publication public123 xpublic12":        "publication public123 xpublic12",
		`"public12"`:                             `"<redacted>"`,
		"key=VRX_TEST_PSK_RF4_x;":                "key=<redacted>;",
		"VRX_TEST_PSK_RF4_xy VRX_TEST_PSK_RF4_x": "VRX_TEST_PSK_RF4_xy <redacted>",
	} {
		if got := r.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func fakeProc(t *testing.T, pid int, bootID string, start uint64) string {
	t.Helper()
	root := t.TempDir()
	if err := bootid.WriteFakeProc(root, bootID, map[int]uint64{pid: start}); err != nil {
		t.Fatal(err)
	}
	prev := procReader
	procReader = bootid.Reader{ProcRoot: root}
	t.Cleanup(func() { procReader = prev })
	return root
}

func TestPendingStartedAfter(t *testing.T) {
	root := fakeProc(t, 4242, "boot-a", 500)
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("6.00 1.00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.pending")
	if err := SetPending(path, "restart", "listen changed", 4242); err != nil {
		t.Fatal(err)
	}
	rec := GetPending(path)
	if rec == nil || rec.SinceTicks != 600 || rec.BootID != "boot-a" || rec.PID != 4242 {
		t.Fatalf("%+v", rec)
	}
	if rec.StartedAfter(4242) { // started at tick 500, before the request
		t.Fatal("old process counted as restarted")
	}
	_ = bootid.WriteFakeProc(root, "boot-a", map[int]uint64{4243: 700})
	if !rec.StartedAfter(4243) {
		t.Fatal("process started after the request not recognised")
	}
	_ = bootid.WriteFakeProc(root, "boot-b", map[int]uint64{4244: 5})
	if !rec.StartedAfter(4244) {
		t.Fatal("another boot not recognised")
	}
	ClearPending(path)
	if GetPending(path) != nil {
		t.Fatal("not cleared")
	}
}

func TestUDPListeners(t *testing.T) {
	pid := 777
	root := fakeProc(t, pid, "boot-a", 1)
	fd := filepath.Join(root, strconv.Itoa(pid), "fd")
	netDir := filepath.Join(root, strconv.Itoa(pid), "net")
	for _, d := range []string{fd, netDir, filepath.Join(root, "sys/net/ipv4")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i, ino := range []string{"101", "102", "103", "104"} {
		if err := os.Symlink("socket:["+ino+"]", filepath.Join(fd, strconv.Itoa(i+3))); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "sys/net/ipv4/ip_local_port_range"), []byte("32768\t60999\n"), 0o600)
	hdr := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n"
	udp := hdr +
		"  1: 0100007F:0F1D 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 101 2 0 0\n" + // 127.0.0.1:3869 listening
		"  2: 00000000:00A1 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 102 2 0 0\n" + // 0.0.0.0:161
		"  3: 00000000:9C40 0A02000A:00A2 01 00000000:00000000 00:00000000 00000000     0        0 103 2 0 0\n" + // connected trap socket
		"  4: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 999 2 0 0\n" // not ours
	udp6 := hdr + "  1: 00000000000000000000000001000000:0F1D 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 104 2 0 0\n"
	_ = os.WriteFile(filepath.Join(netDir, "udp"), []byte(udp), 0o600)
	_ = os.WriteFile(filepath.Join(netDir, "udp6"), []byte(udp6), 0o600)
	got, err := UDPListeners(pid)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"udp6:[::1]:3869", "udp:0.0.0.0:161", "udp:127.0.0.1:3869"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %q, want %q", got, want)
	}
}
