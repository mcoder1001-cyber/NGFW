package pppoe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ngfw/agent/internal/renderers"
)

// StopIPv6 verifies and stops this session's refresher and DHCP child before
// callers remove identity/state files or replace hooks. The fixed helper uses
// pidfds and its session lock; errors preserve evidence and abort the edit.
func (r *Renderer) StopIPv6(ctx context.Context, hostIf string) error {
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if !hostIfRe.MatchString(hostIf) {
		return fmt.Errorf("%w: invalid IPv6 session", ErrInput)
	}
	if _, err := os.Stat(filepath.Join(r.paths.StateDir, hostIf+".ipv6.pid")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("pppoe: IPv6 process evidence unavailable")
	}
	bound, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Paths and hostIf are renderer-owned, validated locations, never shell code.
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(Python3Bin))
	_, err := runner.Run(bound, renderers.Command{Path: Python3Bin, Args: []string{r.paths.ipv6Helper(hostIf), "stop"}, Timeout: 20 * time.Second})
	if err != nil {
		return fmt.Errorf("pppoe: owned IPv6 shutdown failed: %w", err)
	}
	return nil
}
