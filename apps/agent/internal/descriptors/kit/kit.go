// Package kit holds the helpers every descriptor family shares (TD-16, audit ARCH-06): one
// crash-safe atomic file write, one prefix-parsing policy, one ErrRetrieveUnsupported sentinel
// and one Register(r, Env) shape. It imports only the scheduler and the VPP client so that
// dfkit, df2, df6, df7 and vpn can all depend on it.
package kit

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ErrRetrieveUnsupported is the one "VPP has no dump for this object type" sentinel (D-063). It
// is scheduler.ErrRetrieveUnsupported itself, so errors.Is matches across every family.
var ErrRetrieveUnsupported = scheduler.ErrRetrieveUnsupported

// RetrieveUnsupported returns ErrRetrieveUnsupported wrapped with the descriptor name.
func RetrieveUnsupported(name string) error {
	return fmt.Errorf("%s: %w", name, ErrRetrieveUnsupported)
}

// ErrBadPrefix is wrapped by every ParsePrefix rejection; families re-wrap it with their own
// validation sentinel.
var ErrBadPrefix = errors.New("invalid prefix")

// MaskPrefix parses like ParsePrefix but masks host bits instead of rejecting them. It exists only
// for the df2/df6 legacy callers that still canonicalise keys from messy input (TD-16 open
// question); new code uses ParsePrefix.
func MaskPrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w %q: %w", ErrBadPrefix, s, err)
	}
	if p.Addr().Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%w %q: zones are not supported", ErrBadPrefix, s)
	}
	if p.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("%w %q: IPv4-mapped IPv6 prefixes are not supported", ErrBadPrefix, s)
	}
	return p.Masked(), nil
}

// ParsePrefix is the one prefix policy: surrounding space trimmed, zones and IPv4-mapped IPv6
// prefixes rejected, and host bits rejected (the caller must send the canonical network form;
// silently masking hides typos such as 10.0.0.1/24 meant as a host route).
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w %q: %w", ErrBadPrefix, s, err)
	}
	if p.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("%w %q: IPv4-mapped IPv6 prefixes are not supported", ErrBadPrefix, s)
	}
	if p.Masked() != p {
		return netip.Prefix{}, fmt.Errorf("%w %q: host bits set (canonical form %s)", ErrBadPrefix, s, p.Masked())
	}
	return p, nil
}

// WriteFileAtomic replaces path with data so that after a host crash the file holds either the
// old or the new content in full: temp file in the same directory, write, fsync, close, rename,
// then fsync the directory so the rename itself is durable. perm applies to the new file. The
// temp file is path+".tmp".
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// A fixed sibling name: callers serialise writes to one path, and a crash leaves at most one
	// stale temp file, which the next write truncates.
	tmp, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // agent state path chosen by the caller
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return SyncDir(dir)
}

// SyncDir fsyncs a directory so a rename or create inside it survives a host crash.
func SyncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // agent state directory chosen by the caller
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// Env is what a family's descriptors need at registration.
type Env struct {
	Client vpp.Client
	Owner  string
}

// Ctor builds one descriptor from the environment.
type Ctor func(Env) scheduler.Descriptor

// Register builds each descriptor from env and registers it with r, in order.
func Register(r scheduler.Registry, env Env, ctors ...Ctor) {
	for _, c := range ctors {
		r.Register(c(env))
	}
}
