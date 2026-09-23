package rfkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"ngfw/agent/internal/renderers"
)

// RollbackTimeout bounds the restore-and-reload after a failed Apply. The rollback runs on a
// context of its own: the failure may have been the caller's deadline (RF-1 review L2).
const RollbackTimeout = 60 * time.Second

// ErrNotConverged is wrapped when the daemon was reloaded but does not show the configuration
// that was written (RF-1 review H2: Apply never reports success without that proof).
var ErrNotConverged = errors.New("rfkit: daemon did not converge")

// ApplyFiles is the AD-4 apply sequence shared by the renderers:
//
//	snapshot(files) → WriteFiles (atomic, mode/owner) → activate → verify
//	on any failure: Restore the snapshot → activate again (fresh context) → return the error
//
// activate is the control-channel call (Controller.Reload or Restart); verify reads the
// daemon's own state and returns an error wrapping ErrNotConverged when the daemon does not
// report the new configuration. verify may be nil only where the daemon exposes nothing to
// check (documented per renderer). When the snapshot had no previous files at all, the
// rollback activates nothing: there is no previous configuration to go back to.
func ApplyFiles(ctx context.Context, files renderers.Files, activate, verify func(context.Context) error) error {
	if err := files.Validate(); err != nil {
		return err
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	applyErr := renderers.WriteFiles(files)
	if applyErr == nil {
		applyErr = activate(ctx)
		if applyErr == nil && verify != nil {
			applyErr = verify(ctx)
		}
		if applyErr == nil {
			return nil
		}
	}
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), RollbackTimeout)
	defer cancel()
	restoreErr := snap.Restore()
	if hadAny(snap) {
		if err := activate(rbCtx); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("rfkit: re-activating the previous configuration: %w", err))
		}
	}
	return errors.Join(applyErr, restoreErr)
}

// hadAny reports whether any snapshotted path existed before the write.
func hadAny(s *renderers.Snapshot) bool {
	for _, p := range s.Paths() {
		if _, err := os.Lstat(p); err == nil {
			return true
		}
	}
	return false
}

// Poll calls check every interval until it returns nil or ctx / timeout expires, and returns
// the last error (for convergence checks on asynchronous reloads).
func Poll(ctx context.Context, timeout, interval time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		err := check(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-t.C:
		}
	}
}

// ErrTooLarge is returned by ReadFileLimit for files above the limit.
var ErrTooLarge = errors.New("rfkit: file too large")

// ReadFileLimit reads a whole file of at most limit bytes (RF-1 review M3: no unbounded
// reads of daemon output).
func ReadFileLimit(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // renderer-owned path from Paths
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrTooLarge, path, limit)
	}
	return b, nil
}

// ReadTail returns the last at most limit bytes of a file, starting at a line boundary when
// the file is longer than limit, plus the file size.
func ReadTail(path string, limit int64) ([]byte, int64, error) {
	f, err := os.Open(path) //nolint:gosec // renderer-owned path from Paths
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := info.Size()
	off := int64(0)
	if size > limit {
		off = size - limit
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, 0, err
	}
	b, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, 0, err
	}
	if off > 0 {
		// Drop the partial first line.
		for i, c := range b {
			if c == '\n' {
				b = b[i+1:]
				break
			}
		}
	}
	return b, size, nil
}
