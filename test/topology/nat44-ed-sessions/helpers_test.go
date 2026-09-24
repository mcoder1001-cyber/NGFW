package nat44edsessions

import (
	"encoding/json"
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

// TestTcpdumpAndTraceParsers covers the evidence parsers without a host (unit, always runs).
func TestTcpdumpAndTraceParsers(t *testing.T) {
	l := "18:40:01.123456 IP 10.4.2.101.1024 > 10.4.2.2.8000: Flags [S], seq 1, win 64240, length 0"
	if src, port := srcOf(l); src != "10.4.2.101" || port != "1024" {
		t.Fatalf("srcOf: %s %s", src, port)
	}
	if src, _ := srcOf("listening on w4w1, link-type EN10MB"); src != "" {
		t.Fatal("a non-packet line has a source")
	}
	buf := "Packet 1\n\n00:00:01: af-packet-input\n  TCP: 10.4.2.2 -> 10.4.2.110\n    41001 -> 8080\n00:00:01: nat44-ed-out2in\n" +
		"\nPacket 2\n\n00:00:02: af-packet-input\n  TCP: 10.4.2.2 -> 10.4.2.110\n    41002 -> 8080\n00:00:02: ip4-lookup\n"
	blk, ok := natTraceBlock(buf, "nat44-ed-out2in", "TCP: 10.4.2.2 -> 10.4.2.110", "41001")
	if !ok || blk[:8] != "Packet 1" {
		t.Fatalf("trace block: %v %q", ok, blk)
	}
	if _, ok := natTraceBlock(buf, "nat44-ed-out2in", "TCP: 10.4.2.2 -> 10.4.2.110", "41002"); ok {
		t.Fatal("found a block that did not pass nat44-ed-out2in")
	}
	if got := grepLines("a 10.4.2.1\nb 10.5.2.1\nc host-w4l0", "10.4.", "host-w4"); got != "a 10.4.2.1\nc host-w4l0" {
		t.Fatalf("grepLines: %q", got)
	}
}
