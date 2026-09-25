package agent

// TD-9 test accessors for the new API, and the tests of settings that do not exist on the base. The
// base-first evidence (docs/status/tasks/TD-9.md) swaps this file for a shim that encodes the base's
// behaviour: no reply bound, no transaction deadline, no resync backoff, no drift check, no recovery.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/vpp"
)

func bounded(c vpp.Client, d time.Duration) vpp.Client { return vpp.Bounded(c, d) }

func setTxnTimeout(s *Service, d time.Duration) {
	_ = s.lock(context.Background())
	s.txnTimeout = d
	s.unlock()
}

func resyncDelayOf(s *Service) time.Duration {
	_ = s.lock(context.Background())
	defer s.unlock()
	return s.resyncDelay
}

func checkDrift(s *Service) { s.CheckDrift(context.Background()) }

func newRecoveringServer(log *slog.Logger, m *metrics) *grpc.Server {
	if log == nil {
		log = slog.Default()
	}
	return newGRPCServer(log, m)
}

func setLinkRetry(lo, hi time.Duration) (restore func()) {
	oldLo, oldHi := linkRetryMin, linkRetryMax
	linkRetryMin, linkRetryMax = lo, hi
	return func() { linkRetryMin, linkRetryMax = oldLo, oldHi }
}

func setDriftPlanTimeout(d time.Duration) (restore func()) {
	old := driftPlanTimeout
	driftPlanTimeout = d
	return func() { driftPlanTimeout = old }
}

func setConnectHookTimeout(d time.Duration) (restore func()) {
	old := connectHookTimeout
	connectHookTimeout = d
	return func() { connectHookTimeout = old }
}

// VRX_AGENT_VPP_REPLY_TIMEOUT reaches the VPP connection; VRX_METRICS_ALLOW_REMOTE opts in to a
// non-loopback /metrics (new settings: no base equivalent).
func TestConfigReplyTimeoutAndMetricsOptIn(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 0, "20": 20 * time.Second, "15500ms": 15500 * time.Millisecond, "2m": 2 * time.Minute} {
		t.Setenv("VRX_AGENT_VPP_REPLY_TIMEOUT", in)
		c := ConfigFromEnv()
		c.StateDir = t.TempDir()
		if err := c.Validate(); err != nil || c.VPPReplyTimeout != want {
			t.Errorf("%q: %v %v", in, c.VPPReplyTimeout, err)
		}
	}
	for _, in := range []string{"0", "-5s", "abc", "0s", "5"} {
		t.Setenv("VRX_AGENT_VPP_REPLY_TIMEOUT", in)
		if err := ConfigFromEnv().Validate(); err == nil || !strings.Contains(err.Error(), "VRX_AGENT_VPP_REPLY_TIMEOUT") {
			t.Errorf("%q accepted: %v", in, err)
		}
	}
	t.Setenv("VRX_AGENT_VPP_REPLY_TIMEOUT", "70")
	t.Setenv("VRX_METRICS_ADDR", "0.0.0.0:9171")
	if err := ConfigFromEnv().Validate(); err == nil {
		t.Fatal("non-loopback metrics without the opt-in")
	}
	t.Setenv("VRX_METRICS_ALLOW_REMOTE", "1")
	cfg := ConfigFromEnv()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("opt-in: %v", err)
	}
	// Start hands the reply timeout to the VPP connection.
	var got vpp.ConnOptions
	fc := &fakeConn{VPP: coretest.New(), states: make(chan vpp.ConnState, 1)}
	old := dialVPP
	dialVPP = func(_ string, o vpp.ConnOptions) vppConn { got = o; return fc }
	t.Cleanup(func() { dialVPP = old })
	tc := testConfig(t)
	tc.VPPReplyTimeout = cfg.VPPReplyTimeout
	a, err := Start(context.Background(), tc, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	a.Stop()
	if got.ReplyTimeout != 70*time.Second {
		t.Fatalf("ConnOptions.ReplyTimeout = %s", got.ReplyTimeout)
	}
}
