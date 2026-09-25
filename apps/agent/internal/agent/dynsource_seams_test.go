package agent

// Test seams of the per-key quarantine (TD-8b). The tests reach the quarantine only through these
// helpers, so the pre-fix run (TD-8b.md) compiles them against a shim over the base API.

import (
	"context"
	"errors"
	"testing"
	"time"

	"ngfw/agent/internal/subsystems"
)

// quarantinedKeys lists the quarantined keys of the source name, sorted.
func quarantinedKeys(s *Service, name string) []string {
	ds := s.source(name)
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	var out []string
	for _, k := range sortedQuarantine(ds.quarantine) {
		out = append(out, string(k))
	}
	return out
}

// quarantineDelay is the retry backoff of the quarantined key k of the source name (0: none).
func quarantineDelay(s *Service, name, k string) time.Duration {
	ds := s.source(name)
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	for key, q := range ds.quarantine {
		if string(key) == k {
			return q.delay
		}
	}
	return 0
}

// retryQuarantinedNow runs the agent's key retry of the source name at once, as if every
// quarantined key were due.
func retryQuarantinedNow(t *testing.T, s *Service, name string) {
	t.Helper()
	ds := s.source(name)
	if err := s.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	ds.qmu.Lock()
	for _, q := range ds.quarantine {
		q.due = time.Time{}
	}
	ds.qmu.Unlock()
	gen := ds.keyGen
	s.unlock()
	s.retryKeys(ds, gen)
}

// isQuarantinedErr reports whether a sync's error says that it applied all but quarantined keys.
func isQuarantinedErr(err error) bool { return errors.Is(err, subsystems.ErrQuarantined) }

// setSyncTimeout sets the agent's own deadline of a dynamic-source sync for one test.
func setSyncTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := sourceSyncTimeout
	sourceSyncTimeout = d
	t.Cleanup(func() { sourceSyncTimeout = old })
}
