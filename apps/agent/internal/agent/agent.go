// Package agent wires the process together: configuration, lifecycle, and (from P05 on)
// the VPP connection, reconciler and gRPC server.
package agent

import (
	"context"
	"os"
)

// Config is read once from VRX_AGENT_* environment variables (systemd EnvironmentFile).
type Config struct {
	// Socket is the unix socket path the gRPC server listens on.
	Socket string
	// VPPAPISocket is VPP's binary API socket (socksvr in startup.conf).
	VPPAPISocket string
	// StateDir holds the last applied desired state for resync after restart.
	StateDir string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv returns the configuration with production defaults.
func ConfigFromEnv() Config {
	return Config{
		Socket:       env("VRX_AGENT_SOCKET", "/run/vrx/agent.sock"),
		VPPAPISocket: env("VRX_AGENT_VPP_API_SOCKET", "/run/vpp/api.sock"),
		StateDir:     env("VRX_AGENT_STATE_DIR", "/var/lib/vrx/agent"),
	}
}

// Run blocks until ctx is cancelled. P05 replaces the body with the real lifecycle.
func Run(ctx context.Context, _ Config, _ string) error {
	<-ctx.Done()
	return nil
}
