package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/multiwan"
)

func TestFixedProbeRejectsUnboundedInputs(t *testing.T) {
	for _, args := range [][]string{
		{"--kind", "http", "--target", "example.test"},
		{"--kind", "http", "--target", "192.0.2.1:80"},
		{"--kind", "icmp", "--target", "::ffff:192.0.2.1"},
		{"--kind", "dns", "--target", "127.0.0.1"},
		{"--kind", "dns", "--target", "169.254.254.2"},
		{"--kind", "dns", "--target", "255.255.255.255"},
		{"--kind", "dns", "--target", "224.0.0.1"},
		{"--kind", "dns", "--target", "0.0.0.0"},
		{"--kind", "exec", "--target", "192.0.2.1"},
		{"--kind", "dns", "--target", "192.0.2.1", "--timeout-ms", "3001"},
		{"--kind", "dns", "--target", "192.0.2.1", "--device", "lo"},
	} {
		called := false
		err := run(context.Background(), args, &bytes.Buffer{}, func(context.Context, string, *ngfwv1.WanMonitor) multiwan.CheckResult {
			called = true
			return multiwan.CheckResult{}
		})
		if err == nil || called {
			t.Fatalf("unsafe request admitted: %v", args)
		}
	}
}

func TestFixedProbeDeadlineAndResult(t *testing.T) {
	for _, kind := range []string{"icmp", "dns", "http"} {
		output := &bytes.Buffer{}
		err := run(context.Background(), []string{"--kind", kind, "--target", "192.0.2.1", "--timeout-ms", "50"}, output, func(ctx context.Context, device string, m *ngfwv1.WanMonitor) multiwan.CheckResult {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 50*time.Millisecond || device != "ppp0" || m.GetType() != kind || m.GetTarget() != "192.0.2.1" {
				t.Fatal("request escaped bounds")
			}
			return multiwan.CheckResult{Sent: 1, Received: 1, AvgLatencyMs: 4}
		})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if json.Unmarshal(output.Bytes(), &got) != nil || len(got) != 4 || got["sent"] != float64(1) || got["received"] != float64(1) || got["latencyMs"] != float64(4) || got["unavailable"] != false {
			t.Fatal(output.String())
		}
	}
}
