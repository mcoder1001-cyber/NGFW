package agent

import (
	"os"
	"testing"
)

// TestMain points every non-owner host renderer (F-system-identity's sysident, the host services) at a temporary
// directory: an agent test that commits a `system` document must never write under /run/ngfw-test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ngfw-agent-test-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("NGFW_HOST_SERVICES_DIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
