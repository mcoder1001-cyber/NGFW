package pppoe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Fence admission even when no refresher has published a PID yet. A late
	// hook must not create a new writer after a transition's stop completed.
	if err := os.MkdirAll(r.paths.StateDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(r.paths.StateDir, "ipv6-transitions"), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.paths.StateDir, "ipv6-transitions", hostIf), []byte("pending\n"), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.paths.StateDir, hostIf+".ipv6.blocked"), []byte("blocked\n"), 0600); err != nil {
		return err
	}
	if _, err := os.Stat(r.paths.ipv6Helper(hostIf)); os.IsNotExist(err) {
		if _, evidenceErr := os.Stat(filepath.Join(r.paths.StateDir, hostIf+".ipv6.pid")); !os.IsNotExist(evidenceErr) {
			return fmt.Errorf("pppoe: IPv6 helper missing with process evidence")
		}
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

// ResumeIPv6 opens admission only after the supervisor has stopped the old pppd
// unit and completed replacement/invalidation. The fence is never sessionFiles:
// removal must leave it in place for delayed hooks from the old session.
func (r *Renderer) ResumeIPv6(hostIf string) error {
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if !hostIfRe.MatchString(hostIf) {
		return fmt.Errorf("%w: invalid IPv6 session", ErrInput)
	}
	if err := os.MkdirAll(r.paths.StateDir, 0700); err != nil {
		return err
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.paths.StateDir, hostIf+".ipv6.admission"), []byte(hex.EncodeToString(token[:])+"\n"), 0600); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(r.paths.StateDir, hostIf+".ipv6.blocked")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CompleteIPv6Transition acknowledges successful unit startup. Keeping pending
// across file replacement, daemon reload and restart errors makes retries converge.
func (r *Renderer) CompleteIPv6Transition(hostIf string) error {
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if !hostIfRe.MatchString(hostIf) {
		return fmt.Errorf("%w: invalid IPv6 session", ErrInput)
	}
	if err := os.Remove(filepath.Join(r.paths.StateDir, "ipv6-transitions", hostIf)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ForgetIPv6Admission retires removed-session tombstones only after verified
// writer shutdown, unit stop, file removal and successful daemon reload. A queued
// hook then lacks admission; recreating the host allocates a fresh random token.
func (r *Renderer) ForgetIPv6Admission(hostIf string) error {
	if err := r.paths.Validate(); err != nil {
		return err
	}
	if !hostIfRe.MatchString(hostIf) {
		return fmt.Errorf("%w: invalid IPv6 session", ErrInput)
	}
	for _, suffix := range []string{".ipv6.admission", ".ipv6.blocked"} {
		if err := os.Remove(filepath.Join(r.paths.StateDir, hostIf+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
