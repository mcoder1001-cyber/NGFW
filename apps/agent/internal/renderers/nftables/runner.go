package nftables

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"

	"ngfw/agent/internal/renderers"
)

// NftBin is the only binary this renderer runs (internal/renderers/ALLOWLIST.md).
const NftBin = "/usr/sbin/nft"

// Binaries is the production allowlist: nft only — never `ip` (its `netns exec` runs any binary).
func Binaries() renderers.Allowlist { return renderers.NewAllowlist(NftBin) }

// netnsRunner runs every command inside a named network namespace: a fresh goroutine locks its OS thread,
// enters /run/netns/<ns> with setns(2) and starts the process from that thread, which the child inherits.
// The thread is never unlocked, so it exits with the goroutine and never runs other Go code in the
// namespace (the strongswan/swantest pattern). No `ip netns exec`: the allowlist stays nft only.
type netnsRunner struct {
	ns    string
	inner renderers.Runner
}

// NewNetnsRunner wraps inner so that its commands run inside the network namespace ns.
func NewNetnsRunner(ns string, inner renderers.Runner) renderers.Runner {
	return &netnsRunner{ns: ns, inner: inner}
}

func (r *netnsRunner) Run(ctx context.Context, cmd renderers.Command) (renderers.Output, error) {
	type result struct {
		out renderers.Output
		err error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread dies with the goroutine
		if err := enterNetns(r.ns); err != nil {
			ch <- result{err: err}
			return
		}
		out, err := r.inner.Run(ctx, cmd)
		ch <- result{out, err}
	}()
	select {
	case res := <-ch:
		return res.out, res.err
	case <-ctx.Done():
		return renderers.Output{}, ctx.Err()
	}
}

// enterNetns moves the calling (locked) OS thread into /run/netns/<ns>.
func enterNetns(ns string) error {
	if !netnsRe.MatchString(ns) {
		return fmt.Errorf("nftables: namespace %q is not ns-<prefix>-<name>", ns)
	}
	f, err := os.Open(filepath.Join("/run/netns", ns)) //nolint:gosec // validated ns-<prefix>-<name>
	if err != nil {
		return fmt.Errorf("nftables: open namespace %s: %w", ns, err)
	}
	defer func() { _ = f.Close() }()
	if err := unix.Setns(int(f.Fd()), unix.CLONE_NEWNET); err != nil { //nolint:gosec // an fd fits in int
		return fmt.Errorf("nftables: setns(%s): %w", ns, err)
	}
	return nil
}
