package nat44ei6466nptv6

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
)

// waitIfs waits until VPP has all names and returns their sw_if_index.
func waitIfs(t *testing.T, vc vppapi.Connection, names ...string) map[string]uint32 {
	t.Helper()
	out := map[string]uint32{}
	if !waitFor(30*time.Second, func() bool {
		ifs := dumpIfs(t, vc)
		for _, n := range names {
			i, ok := ifs[n]
			if !ok {
				return false
			}
			out[n] = i.idx
		}
		return true
	}) {
		t.Fatalf("VPP does not have %v", names)
	}
	return out
}

// runEnv is run with an explicit environment (the screenshot script's LD_LIBRARY_PATH).
func runEnv(t *testing.T, env []string, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestEvidenceParsers covers the evidence parsers and the address plan without a host (unit, always runs).
func TestEvidenceParsers(t *testing.T) {
	if src, port := srcOf("18:40:01.123456 IP 10.4.2.101.1024 > 10.4.2.2.8000: Flags [S], seq 1, length 0"); src != "10.4.2.101" || port != "1024" {
		t.Fatalf("srcOf: %s %s", src, port)
	}
	if src, port := srcOf6("00:01:02.000000 IP6 fd00:4:20:ffec::2.45001 > fd00:4:2::2.8006: Flags [S], seq 1, length 0"); src != "fd00:4:20:ffec::2" || port != "45001" {
		t.Fatalf("srcOf6: %s %s", src, port)
	}
	if src, _ := srcOf6("listening on w4w1, link-type EN10MB"); src != "" {
		t.Fatal("a non-packet line has a source")
	}
	if got := synth("fd00:4:64::/96", "10.4.2.2"); got != "fd00:4:64::a04:202" {
		t.Fatalf("synth: %s", got)
	}
	s := slot{prefix: "w11", num: 11}
	a := newV6(s)
	if a.nptInternal != "fd00:b:10::/48" || a.lanClient != "fd00:b:1::2" || hexSlot(slot{num: 4}) != "4" {
		t.Fatalf("v6 plan %+v", a)
	}
	if got := grepLines("a 10.4.2.1\nb 10.5.2.1\nc host-w4l0", "10.4.", "host-w4"); got != "a 10.4.2.1\nc host-w4l0" {
		t.Fatalf("grepLines: %q", got)
	}
	if m := npt66Line.FindStringSubmatch("[0] internal: fd00:4:10::/48 external: fd00:4:20::/48"); m == nil || m[2] != "fd00:4:10::/48" {
		t.Fatalf("npt66Line: %v", m)
	}
}
