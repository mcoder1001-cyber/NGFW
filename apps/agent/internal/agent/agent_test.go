package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	for _, k := range []string{"VRX_AGENT_SOCKET", "VRX_OWNER", "VRX_METRICS_ADDR", "VRX_METRICS_PORT", "VRX_AGENT_STATE_DIR"} {
		t.Setenv(k, "")
	}
	cfg := ConfigFromEnv()
	if cfg.Socket != "/run/vrx/agent.sock" || cfg.Owner != "vrx" || cfg.MetricsAddr != "127.0.0.1:9101" || cfg.StateDir != "/var/lib/vrx/agent" {
		t.Fatalf("unexpected defaults %+v", cfg)
	}
	t.Setenv("VRX_METRICS_PORT", "9171")
	if cfg := ConfigFromEnv(); cfg.MetricsAddr != "127.0.0.1:9171" {
		t.Fatalf("slot metrics port %q", cfg.MetricsAddr)
	}
	for _, bad := range []string{"", "a:b", "a/b", "a b"} {
		c := cfg
		c.Owner = bad
		if c.Validate() == nil {
			t.Fatalf("owner %q accepted", bad)
		}
	}
}

// testConfig never touches product paths: temp socket/state dir, a VPP socket that does not exist.
func testConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		Socket: filepath.Join(dir, "agent.sock"), SocketGroup: "vrx-group-that-does-not-exist",
		VPPAPISocket: filepath.Join(dir, "no-vpp.sock"), VPPStatsSocket: filepath.Join(dir, "no-stats.sock"),
		StateDir: filepath.Join(dir, "state"), Owner: "w0", MetricsAddr: "127.0.0.1:0",
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	cfg := testConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := Run(ctx, cfg, "test"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(cfg.Socket); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: %v", err)
	}
}

func TestSocketPermissionsAndInUse(t *testing.T) {
	cfg := testConfig(t)
	a, err := Start(context.Background(), cfg, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	fi, err := os.Stat(cfg.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o660 || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket mode %v", fi.Mode())
	}
	if _, err := listenUnix(cfg.Socket, "", nil); err == nil {
		t.Fatal("second listener stole a live socket")
	}
	// A stale socket file (no listener) is replaced.
	stale := filepath.Join(t.TempDir(), "stale.sock")
	l, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = l.Close()
	l2, err := listenUnix(stale, "", nil)
	if err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	}
	_ = l2.Close()
}
