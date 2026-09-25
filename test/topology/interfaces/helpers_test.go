package interfaces

import (
	"os"
	"path/filepath"
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

// TestEchoFramesExactlyN (D-128, replaces the trace-block test): the interface counters admit exactly n echo-sized frames
// plus a few small ARP frames — a missing echo, an extra echo-sized frame or too many extra frames all fail.
func TestEchoFramesExactlyN(t *testing.T) {
	const n, frame = 5, 1042 // the smallest echo frame the test sends (size 1000)
	for _, c := range []struct {
		name       string
		pkts, byts uint64
		ok         bool
	}{
		{"exactly n echo frames", n, n * frame, true},
		{"n echo + 2 ARP (42 B)", n + 2, n*frame + 84, true},
		{"n echo + 4 ARP (60 B)", n + 4, n*frame + 240, true},
		{"one echo missing, ARP makes up the count", n, (n-1)*frame + 42, false},
		{"one echo missing, 5 small frames", n + 4, (n-1)*frame + 5*200, false},
		{"an extra echo-sized frame", n + 1, (n + 1) * frame, false},
		{"too many extra frames", n + 5, n*frame + 5*42, false},
		{"fewer frames than echoes", n - 1, (n - 1) * frame, false},
		{"bytes of a larger frame", n, n*frame + 1, false},
	} {
		err := echoFrames(c.pkts, c.byts, n, frame)
		if (err == nil) != c.ok {
			t.Errorf("%s: echoFrames(%d, %d) = %v, want ok=%v", c.name, c.pkts, c.byts, err, c.ok)
		}
	}
	if err := echoFrames(n, n*800, n, 800); err == nil {
		t.Error("an echo frame of 800 bytes cannot be told from 4 small frames: want an error")
	}
}

// TestPingCounts: iputils ping summary lines.
func TestPingCounts(t *testing.T) {
	for in, want := range map[string][2]int{
		"5 packets transmitted, 5 received, 0% packet loss, time 1205ms":              {5, 5},
		"3 packets transmitted, 2 received, 33.3333% packet loss, time 602ms":         {3, 2},
		"2 packets transmitted, 0 received, +2 errors, 100% packet loss, time 1001ms": {2, 0},
		"1 packets transmitted, 1 packets received, 0.0% packet loss":                 {1, 1},
		"ping: sendmsg: Network is unreachable":                                       {-1, -1},
	} {
		if tx, rx := pingCounts(in); tx != want[0] || rx != want[1] {
			t.Errorf("pingCounts(%q) = %d, %d; want %v", in, tx, rx, want)
		}
	}
}

// TestShowIntCountersRegex: `vppctl show interface` rows (header row and continuation rows), bytes included.
func TestShowIntCountersRegex(t *testing.T) {
	out := `              Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
host-w1l0                         3      up          9000/0/0/0     rx packets                    26
                                                                    rx bytes                    2484
                                                                    tx packets                    24
                                                                    tx bytes                    2296
                                                                    drops                          2
`
	got := map[string]string{}
	for _, m := range counterRe.FindAllStringSubmatch(out, -1) {
		got[m[2]] = m[3]
	}
	want := map[string]string{"rx packets": "26", "rx bytes": "2484", "tx packets": "24", "tx bytes": "2296"}
	if js(got) != js(want) {
		t.Fatalf("counterRe: %v, want %v", got, want)
	}
}
