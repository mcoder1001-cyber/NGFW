package hostacl

// The slot's two namespaces (the same shape as apps/agent/internal/renderers/nftables/nftest): ns-<prefix>-hacl is the
// "host" whose table the slot agent renders (VRX_HOST_ACL_NETNS), ns-<prefix>-hpeer a client, joined by the veth pair
// <prefix>h0 ↔ <prefix>p0 on 10.<slot>.77.0/24. Sockets are opened by a goroutine that locked its OS thread and entered
// the namespace with setns(2). The root netns ruleset is only ever listed (`nft list tables`) and must not change.

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type netns struct {
	host, peer         string
	hostAddr, peerAddr string
}

func rootTables(t *testing.T) string {
	t.Helper()
	return mustRun(t, "nft", "list", "tables")
}

// newNetns creates the namespaces (after deleting leftovers of an earlier run) and deletes them in t.Cleanup.
func newNetns(t *testing.T, s slot) *netns {
	t.Helper()
	n, _ := strconv.Atoi(strings.TrimPrefix(s.prefix, "w"))
	ns := &netns{host: "ns-" + s.prefix + "-hacl", peer: "ns-" + s.prefix + "-hpeer",
		hostAddr: fmt.Sprintf("10.%d.77.1", n), peerAddr: fmt.Sprintf("10.%d.77.2", n)}
	teardown := func() {
		for _, x := range []string{ns.peer, ns.host} {
			if _, err := os.Stat(filepath.Join("/run/netns", x)); err == nil {
				_, _ = run("ip", "netns", "delete", x)
			}
		}
	}
	teardown()
	t.Cleanup(teardown)
	hv, pv := s.prefix+"h0", s.prefix+"p0"
	for _, args := range [][]string{
		{"netns", "add", ns.host},
		{"netns", "add", ns.peer},
		{"link", "add", hv, "netns", ns.host, "type", "veth", "peer", "name", pv, "netns", ns.peer},
		{"-n", ns.host, "addr", "add", ns.hostAddr + "/24", "dev", hv},
		{"-n", ns.peer, "addr", "add", ns.peerAddr + "/24", "dev", pv},
		{"-n", ns.host, "link", "set", "lo", "up"},
		{"-n", ns.peer, "link", "set", "lo", "up"},
		{"-n", ns.host, "link", "set", hv, "up"},
		{"-n", ns.peer, "link", "set", pv, "up"},
	} {
		mustRun(t, "ip", args...)
	}
	return ns
}

// nft runs nft inside the host namespace (test code only: `ip netns exec`, never in the product).
func (ns *netns) nft(t *testing.T, args ...string) string {
	t.Helper()
	return mustRun(t, "ip", append([]string{"netns", "exec", ns.host, "nft"}, args...)...)
}

var counterRe = regexp.MustCompile(`counter packets \d+ bytes \d+`)

// listing is `nft list table inet <table>` in the host namespace with the counter values blanked.
func (ns *netns) listing(t *testing.T, table string) string {
	t.Helper()
	return counterRe.ReplaceAllString(ns.nft(t, "list", "table", "inet", table), "counter")
}

// sysSetns is setns(2) on linux/amd64 (the syscall package does not export it; the module has no dependencies).
const sysSetns = 308

func inNetns(name string, f func() error) error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("setns: syscall number known for amd64 only (GOARCH %s)", runtime.GOARCH)
	}
	ch := make(chan error, 1)
	go func() {
		runtime.LockOSThread()                                // never unlocked: the thread exits with the goroutine
		fd, err := os.Open(filepath.Join("/run/netns", name)) //nolint:gosec // ns-<prefix>-<name> of this test
		if err != nil {
			ch <- err
			return
		}
		defer func() { _ = fd.Close() }()
		if _, _, errno := syscall.RawSyscall(sysSetns, fd.Fd(), syscall.CLONE_NEWNET, 0); errno != 0 {
			ch <- fmt.Errorf("setns(%s): %w", name, errno)
			return
		}
		ch <- f()
	}()
	return <-ch
}

// listen serves TCP port on the host address inside the host namespace ("ok\n" per connection).
func (ns *netns) listen(t *testing.T, port int) {
	t.Helper()
	var l net.Listener
	if err := inNetns(ns.host, func() error {
		var err error
		l, err = net.Listen("tcp", net.JoinHostPort(ns.hostAddr, strconv.Itoa(port)))
		return err
	}); err != nil {
		t.Fatalf("listen %d in %s: %v", port, ns.host, err)
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

// dial connects from the peer namespace to the host address and reads the greeting.
func (ns *netns) dial(port int, timeout time.Duration) error {
	return inNetns(ns.peer, func() error {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ns.hostAddr, strconv.Itoa(port)), timeout)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		buf := make([]byte, 3)
		if _, err := io.ReadFull(c, buf); err != nil {
			return err
		}
		return nil
	})
}
