package rfkit

import (
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

func TestPendingUptimeDecimalPrecision(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  uint64
	}{
		{"10.03", 1003}, {"16.15", 1615}, {"6.00", 600}, {"6", 600}, {"1.5", 150},
		{"184467440737095516.15", 18446744073709551615},
	} {
		got, err := parseUptimeTicks(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("%q: %d/%v, want%d", tc.value, got, err, tc.want)
		}
	}
	for _, value := range []string{"NaN", "-1.00", "1.001", "1.", "1.x", "184467440737095516.16", "184467440737095517"} {
		if _, err := parseUptimeTicks(value); err == nil {
			t.Fatalf("accepted invalid/overflow uptime%q", value)
		}
	}
}

func TestPendingAtCurrentProcessStartTick(t *testing.T) {
	root := fakeProc(t, 4242, "boot-a", 1003)
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("10.03 1.00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "restart.pending")
	if err := SetPending(path, "restart", "changed", 4242); err != nil {
		t.Fatal(err)
	}
	rec := GetPending(path)
	if rec == nil || rec.SinceTicks != 1003 || rec.StartedAfter(4242) {
		t.Fatalf("unchanged current process acknowledged request: %+v", rec)
	}
	if err := bootid.WriteFakeProc(root, "boot-a", map[int]uint64{4243: 1003, 4244: 1004}); err != nil {
		t.Fatal(err)
	}
	if !rec.StartedAfter(4243) || !rec.StartedAfter(4244) {
		t.Fatal("real replacement failed to acknowledge request")
	}
}
