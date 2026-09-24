// Package nftest is the test-only network-namespace harness of the nftables renderer (F-host-acl-nftables).
// The shared host is reached over SSH on ens192, so nothing is ever loaded into the root network namespace:
// the harness creates two namespaces of the slot, ns-<prefix>-hacl (the "host" the table is loaded into)
// and ns-<prefix>-hpeer (a client), joined by the veth pair <prefix>h0 ↔ <prefix>p0 on 10.<slot>.77.0/24,
// and records the root namespace's `nft list tables` (read only) before and after: the test fails when it
// changed. Namespaces are created with `ip` (a test-only allowlist row; the product allowlist is nft only)
// and deleted in t.Cleanup. Sockets are opened inside a namespace by a goroutine that locks its OS thread
// and enters it with setns(2) (the swantest pattern).
//
//	h := nftest.New(t)                       // skips unless VRX_INTEGRATION=1; takes the lab lock shared
//	r := nftables.New(h.Runner(), h.Paths()) // table inet vrx_<prefix> inside ns-<prefix>-hacl
//	stop := h.Listen(t, 2222)                // TCP server on the host side
//	err := h.Dial(2222, time.Second)         // TCP client from the peer side
package nftest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/vpp/vpptest"
)

// IPBin is the test-only binary of the harness (internal/renderers/ALLOWLIST.md).
const IPBin = "/usr/bin/ip"

// Harness is one slot's pair of namespaces.
type Harness struct {
	Prefix   string
	Slot     int
	Host     string // ns-<prefix>-hacl: the table lives here
	Peer     string // ns-<prefix>-hpeer
	HostAddr netip.Addr
	PeerAddr netip.Addr
	Dir      string // /run/vrx-test/<prefix>/nftables: rendered file and store
	ip       renderers.Runner
	nft      renderers.Runner // nft inside Host
}

// RootTables is `nft list tables` of the root network namespace (read only; allowed on the shared host).
func RootTables(ctx context.Context) (string, error) {
	out, err := renderers.NewSystemRunner(nftables.Binaries()).Run(ctx, renderers.Command{Path: nftables.NftBin, Args: []string{"list", "tables"}})
	return string(out.Stdout), err
}

// New creates the namespaces and the veth pair (after removing leftovers of an earlier run of the same
// slot) and registers their deletion and the root-namespace check with t.Cleanup.
func New(t *testing.T) *Harness {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	if _, err := os.Stat(nftables.NftBin); err != nil {
		t.Skipf("%s not installed", nftables.NftBin)
	}
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	h := &Harness{
		Prefix: prefix, Slot: slot,
		Host: "ns-" + prefix + "-hacl", Peer: "ns-" + prefix + "-hpeer",
		HostAddr: netip.MustParseAddr(fmt.Sprintf("10.%d.77.1", slot)),
		PeerAddr: netip.MustParseAddr(fmt.Sprintf("10.%d.77.2", slot)),
		Dir:      filepath.Join("/run/vrx-test", prefix, "nftables"),
		ip:       renderers.NewSystemRunner(renderers.NewAllowlist(IPBin)),
	}
	h.nft = nftables.NewNetnsRunner(h.Host, renderers.NewSystemRunner(nftables.Binaries()))
	ctx := context.Background()
	before, err := RootTables(ctx)
	if err != nil {
		t.Fatalf("root nft list tables: %v", err)
	}
	t.Logf("root netns `nft list tables` before:\n%s", before)
	h.teardown(ctx)
	if err := os.MkdirAll(h.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.teardown(context.Background())
		_ = os.RemoveAll(h.Dir)
		after, err := RootTables(context.Background())
		if err != nil {
			t.Errorf("root nft list tables: %v", err)
			return
		}
		t.Logf("root netns `nft list tables` after:\n%s", after)
		if after != before {
			t.Errorf("the root network namespace ruleset changed:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})
	hv, pv := prefix+"h0", prefix+"p0"
	steps := [][]string{
		{"netns", "add", h.Host},
		{"netns", "add", h.Peer},
		{"link", "add", hv, "netns", h.Host, "type", "veth", "peer", "name", pv, "netns", h.Peer},
		{"-n", h.Host, "addr", "add", h.HostAddr.String() + "/24", "dev", hv},
		{"-n", h.Peer, "addr", "add", h.PeerAddr.String() + "/24", "dev", pv},
		{"-n", h.Host, "link", "set", "lo", "up"},
		{"-n", h.Peer, "link", "set", "lo", "up"},
		{"-n", h.Host, "link", "set", hv, "up"},
		{"-n", h.Peer, "link", "set", pv, "up"},
	}
	for _, s := range steps {
		if _, err := h.ip.Run(ctx, renderers.Command{Path: IPBin, Args: s}); err != nil {
			t.Fatalf("ip %s: %v", strings.Join(s, " "), err)
		}
	}
	return h
}

func (h *Harness) teardown(ctx context.Context) {
	for _, ns := range []string{h.Peer, h.Host} {
		if _, err := os.Stat(filepath.Join("/run/netns", ns)); err == nil {
			_, _ = h.ip.Run(ctx, renderers.Command{Path: IPBin, Args: []string{"netns", "delete", ns}})
		}
	}
}

// Paths are the renderer's test paths: table vrx_<prefix> in the Host namespace, files in Dir.
func (h *Harness) Paths() nftables.Paths { return nftables.TestPaths(h.Prefix, h.Host, h.Dir) }

// Runner is the product runner (nft only); nftables.New wraps it for the namespace.
func (h *Harness) Runner() renderers.Runner { return renderers.NewSystemRunner(nftables.Binaries()) }

// Nft runs nft with args inside the Host namespace and returns its stdout.
func (h *Harness) Nft(t *testing.T, args ...string) string {
	t.Helper()
	out, err := h.nft.Run(context.Background(), renderers.Command{Path: nftables.NftBin, Args: args})
	if err != nil {
		t.Fatalf("nft %s (in %s): %v", strings.Join(args, " "), h.Host, err)
	}
	return string(out.Stdout)
}

// NftStdin runs `nft -f -` with script inside the Host namespace (test setup: a foreign table).
func (h *Harness) NftStdin(t *testing.T, script string) {
	t.Helper()
	if _, err := h.nft.Run(context.Background(), renderers.Command{Path: nftables.NftBin, Args: []string{"-f", "-"}, Stdin: []byte(script)}); err != nil {
		t.Fatalf("nft -f - (in %s): %v", h.Host, err)
	}
}

// Listen serves TCP port on HostAddr inside the Host namespace (each connection gets "ok\n"); the
// listener closes in t.Cleanup.
func (h *Harness) Listen(t *testing.T, port int) {
	t.Helper()
	var l net.Listener
	err := inNetns(h.Host, func() error {
		var err error
		l, err = net.Listen("tcp", net.JoinHostPort(h.HostAddr.String(), strconv.Itoa(port)))
		return err
	})
	if err != nil {
		t.Fatalf("listen %d in %s: %v", port, h.Host, err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(c, "ok\n")
			_ = c.Close()
		}
	}()
}

// Dial connects from the Peer namespace to HostAddr:port and reads the greeting; an error means the
// connection did not complete within timeout (dropped) or was refused (rejected).
func (h *Harness) Dial(port int, timeout time.Duration) error {
	return inNetns(h.Peer, func() error {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(h.HostAddr.String(), strconv.Itoa(port)), timeout)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		buf := make([]byte, 3)
		if _, err := io.ReadFull(c, buf); err != nil {
			return err
		}
		if string(buf) != "ok\n" {
			return fmt.Errorf("unexpected greeting %q", buf)
		}
		return nil
	})
}

// inNetns runs f on a fresh goroutine whose OS thread entered ns; the thread is never unlocked, so it
// exits with the goroutine.
func inNetns(ns string, f func() error) error {
	ch := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		fd, err := os.Open(filepath.Join("/run/netns", ns)) //nolint:gosec // ns-<prefix>-<name> of the harness
		if err != nil {
			ch <- err
			return
		}
		defer func() { _ = fd.Close() }()
		if err := unix.Setns(int(fd.Fd()), unix.CLONE_NEWNET); err != nil { //nolint:gosec // an fd fits in int
			ch <- fmt.Errorf("setns(%s): %w", ns, err)
			return
		}
		ch <- f()
	}()
	return <-ch
}
