package nat44edsessions

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

// TestEvidenceParsers covers the evidence parsers without a host (unit, always runs).
func TestEvidenceParsers(t *testing.T) {
	l := "18:40:01.123456 IP 10.4.2.101.1024 > 10.4.2.2.8000: Flags [S], seq 1, win 64240, length 0"
	if src, port := srcOf(l); src != "10.4.2.101" || port != "1024" {
		t.Fatalf("srcOf: %s %s", src, port)
	}
	if src, _ := srcOf("listening on w4w1, link-type EN10MB"); src != "" {
		t.Fatal("a non-packet line has a source")
	}
	if got := grepLines("a 10.4.2.1\nb 10.5.2.1\nc host-w4l0", "10.4.", "host-w4"); got != "a 10.4.2.1\nc host-w4l0" {
		t.Fatalf("grepLines: %q", got)
	}
}
