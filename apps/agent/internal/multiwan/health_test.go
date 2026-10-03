package multiwan

import "testing"

func TestHealthy(t *testing.T) {
	cfg := MonitorConfig{LossPct: 50, LatencyMs: 100, DownAfter: 3, UpAfter: 2}
	for _, tc := range []struct {
		name string
		r    CheckResult
		want bool
	}{
		{"all received, low latency", CheckResult{Sent: 4, Received: 4, AvgLatencyMs: 20}, true},
		{"total loss", CheckResult{Sent: 4, Received: 0}, false},
		{"loss at threshold", CheckResult{Sent: 4, Received: 2, AvgLatencyMs: 10}, false},   // 50% >= 50%
		{"loss under threshold", CheckResult{Sent: 4, Received: 3, AvgLatencyMs: 10}, true}, // 25% < 50%
		{"latency over", CheckResult{Sent: 4, Received: 4, AvgLatencyMs: 150}, false},
		{"no probes sent", CheckResult{}, false},
	} {
		if got := cfg.Healthy(tc.r); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	// latency check off when LatencyMs == 0
	if !(MonitorConfig{LossPct: 100, LatencyMs: 0}).Healthy(CheckResult{Sent: 1, Received: 1, AvgLatencyMs: 9999}) {
		t.Fatal("latency 0 should disable the latency check")
	}
}

func TestHysteresis(t *testing.T) {
	cfg := MonitorConfig{LossPct: 100, DownAfter: 3, UpAfter: 2}
	s := NewState()
	if !s.Up {
		t.Fatal("a member starts up")
	}
	fail := CheckResult{Sent: 1, Received: 0}
	ok := CheckResult{Sent: 1, Received: 1, AvgLatencyMs: 5}
	// two fails: not down yet
	if s.Observe(cfg, fail) {
		t.Fatal("down after 1 fail")
	}
	if s.Observe(cfg, fail) {
		t.Fatal("down after 2 fails")
	}
	if !s.Up {
		t.Fatal("still up after 2 of 3 fails")
	}
	// third fail: down (transition)
	if !s.Observe(cfg, fail) || s.Up {
		t.Fatal("should be down after 3 fails")
	}
	// a single ok resets the fail streak but not up yet (needs UpAfter=2)
	if s.Observe(cfg, ok) || s.Up {
		t.Fatal("up before UpAfter")
	}
	if !s.Observe(cfg, ok) || !s.Up {
		t.Fatal("should be up after 2 oks")
	}
	// an ok in the middle of failing resets the down streak (no flap)
	s2 := NewState()
	s2.Observe(cfg, fail)
	s2.Observe(cfg, fail)
	s2.Observe(cfg, ok) // resets
	if s2.Observe(cfg, fail) || !s2.Up {
		t.Fatal("fail streak should have reset after the ok")
	}
}

func TestFailoverActive(t *testing.T) {
	// lowest priority among healthy wins; ties by interface name
	members := []Member{
		{Interface: "wan2", Priority: 50, Up: true},
		{Interface: "wan1", Priority: 50, Up: true},
		{Interface: "wan0", Priority: 10, Up: false}, // best priority but down
	}
	if got := FailoverActive(members); got != "wan1" {
		t.Fatalf("got %q want wan1", got)
	}
	members[2].Up = true
	if got := FailoverActive(members); got != "wan0" {
		t.Fatalf("got %q want wan0 (lowest priority, now up)", got)
	}
	if FailoverActive([]Member{{Interface: "x", Up: false}}) != "" {
		t.Fatal("no healthy member → empty")
	}
}

func TestBalancePaths(t *testing.T) {
	members := []Member{
		{Interface: "wan1", Weight: 3, Up: true},
		{Interface: "wan0", Weight: 1, Up: true},
		{Interface: "wan2", Weight: 5, Up: false}, // down: excluded
		{Interface: "wan3", Weight: 0, Up: true},  // zero weight: excluded
	}
	got := BalancePaths(members)
	if len(got) != 2 || got[0].Interface != "wan0" || got[1].Interface != "wan1" {
		t.Fatalf("balance paths: %+v", got)
	}
}
