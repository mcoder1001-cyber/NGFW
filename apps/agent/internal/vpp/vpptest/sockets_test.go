package vpptest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSockets: unset → the shared VPP's /run/vpp/*; set (tools/lab env <N> with the slot VPP up) → the slot's paths.
func TestSockets(t *testing.T) {
	t.Setenv(EnvAPISocket, "")
	t.Setenv(EnvCLISocket, "")
	t.Setenv(EnvStatsSocket, "")
	if APISocket() != "/run/vpp/api.sock" || CLISocket() != "/run/vpp/cli.sock" || StatsSocket() != "/run/vpp/stats.sock" {
		t.Fatalf("defaults: %s %s %s", APISocket(), CLISocket(), StatsSocket())
	}
	d := "/run/ngfw-test/w20/vpp"
	t.Setenv(EnvAPISocket, d+"/api.sock")
	t.Setenv(EnvCLISocket, d+"/cli.sock")
	t.Setenv(EnvStatsSocket, d+"/stats.sock")
	if APISocket() != d+"/api.sock" || CLISocket() != d+"/cli.sock" || StatsSocket() != d+"/stats.sock" {
		t.Fatalf("slot: %s %s %s", APISocket(), CLISocket(), StatsSocket())
	}
}

// TestVPPCtlUsesCLISocket runs a fake vppctl from PATH and checks it gets `-s <CLISocket()>` first.
func TestVPPCtlUsesCLISocket(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "vppctl"), []byte("#!/bin/sh\necho \"$@\"\n"), 0o755); err != nil { //nolint:gosec // fake binary
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvCLISocket, "/run/ngfw-test/w20/vpp/cli.sock")
	out, err := VPPCtl(context.Background(), "show", "version")
	if err != nil || strings.TrimSpace(string(out)) != "-s /run/ngfw-test/w20/vpp/cli.sock show version" {
		t.Fatalf("%q %v", out, err)
	}
}
