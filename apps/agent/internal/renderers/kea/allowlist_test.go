package kea

import (
	"strings"
	"testing"
)

// TestProductAllowlistHasNoTrampoline: the production allowlist never contains a binary that
// can run arbitrary programs (ip netns exec, env, shells).
func TestProductAllowlistHasNoTrampoline(t *testing.T) {
	for _, b := range Binaries() {
		base := b[strings.LastIndex(b, "/")+1:]
		if b == "/usr/bin/ip" || base == "env" || base == "sh" || base == "bash" || base == "systemctl" {
			t.Errorf("production allowlist contains %s", b)
		}
	}
}
