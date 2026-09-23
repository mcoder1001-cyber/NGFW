package agent

import (
	"context"
	"testing"
	"time"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("VRX_AGENT_SOCKET", "")
	cfg := ConfigFromEnv()
	if cfg.Socket != "/run/vrx/agent.sock" {
		t.Fatalf("unexpected default socket %q", cfg.Socket)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := Run(ctx, ConfigFromEnv(), "test"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
