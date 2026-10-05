package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// trafficBProbe is opt-in and fixed to the reviewed repository probe. Ordinary
// topology fixtures retain their existing behavior. Never use a shared VPP.
func trafficBProbe(t *testing.T, repo, phase string, slot int, fib bool) {
	t.Helper()
	if os.Getenv("NGFW_TRAFFIC_B") != "1" {
		return
	}
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || !fib {
		t.Fatal("Wave-B packet hook requires private VPP and the FIB proof")
	}
	output := os.Getenv("NGFW_TRAFFIC_B_EVIDENCE")
	if output == "" {
		t.Fatal("Wave-B evidence directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	//nolint:gosec // fixed repository-owned helper and validated slot fixture values
	out, err := exec.CommandContext(ctx, "python3", filepath.Join(repo, "test/topology/traffic-b/probe.py"), "--slot", strconv.Itoa(slot), "--phase", phase, "--output", output).CombinedOutput()
	if err != nil {
		t.Fatalf("Wave-B %s packet probe: %v: %s", phase, err, out)
	}
	t.Logf("Wave-B %s packet probe accepted; text evidence %s", phase, output)
}
