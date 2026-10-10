//go:build lab_ra_readiness_cost

package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

// Supplemental fixed-artifact cost measurement only: no manager connection,
// unit activation, namespace mutation, peer attestation or readiness authority.
func TestReadOnlyActualReadinessArtifactCost(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("NGFW_RA_READINESS_COST_PROBE") != "1" {
		t.Fatal("explicit root read-only cost probe authorization required")
	}
	readinessCostArtifactGuard(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	started := time.Now()
	engineErr := DefaultEngineInstallation().Verify(ctx)
	engineElapsed := time.Since(started)
	engineContextClass := readinessCostClass(ctx.Err())
	cancel()
	t.Logf("phase=1 elapsed_ms=%d result_class=%d context_class=%d", engineElapsed.Milliseconds(), readinessCostClass(engineErr), engineContextClass)

	// This exact production routine intentionally has its own original
	// background-parent installation context. Do not substitute a clipped call.
	started = time.Now()
	observerErr := unitObserverInstallation()
	t.Logf("phase=2 elapsed_ms=%d result_class=%d", time.Since(started).Milliseconds(), readinessCostClass(observerErr))
	readinessCostArtifactGuard(t)
	if engineErr != nil || engineContextClass != 0 || observerErr != nil {
		t.Fatal("actual read-only cost phase failed")
	}
}

func readinessCostClass(err error) int {
	if err == nil {
		return 0
	}
	if err == context.DeadlineExceeded {
		return 2
	}
	if err == context.Canceled {
		return 3
	}
	return 1
}

func readinessCostArtifactGuard(t *testing.T) {
	t.Helper()
	for _, item := range []struct{ path, digest string }{
		{"/usr/lib/ngfw/ngfw-ra-namespace-broker", "c8a4b99515aa7994d59f2e7b1870bd945c5090c7b9992972ebb541b6c3829145"},
		{"/usr/lib/ngfw/ngfw-ra-daemon", "48f9ec89fc7abd9d9a1dac80bcb4cbc7bb7379314f48a0b27fa253ff54693a38"},
	} {
		data, err := trustedInstallationFile(item.path, 32<<20, true)
		digest := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(digest[:]) != item.digest {
			t.Fatal("fixed source-paired artifact guard refused")
		}
	}
}
