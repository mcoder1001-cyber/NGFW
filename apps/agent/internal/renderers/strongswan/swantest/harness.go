// Package swantest runs test-scoped strongSwan daemons for integration tests (RF-2, and P11's
// topology test): each charon is a child process in a rig network namespace ns-<prefix>-<x>,
// with its own strongswan.conf, VICI socket and log under /run/vrx-test/<prefix>/swan/<x>. The
// system strongswan unit, /etc/strongswan.conf and /etc/swanctl are never used.
//
// Binaries. When strongSwan is installed (/usr/sbin/charon-systemd and /usr/sbin/swanctl
// exist) they are used directly. Otherwise the stock Ubuntu packages may be extracted without
// installing them (README.md: apt-get download + dpkg -x into
// /run/vrx-test/<prefix>/swan-stock/root): the stock binaries load their plugins from the
// compile-time path /usr/lib/ipsec/plugins and /run is mounted noexec, so every daemon and
// swanctl run happens on an OS thread that first enters a private mount namespace (propagation
// private, so nothing leaks back to the host) and mounts read-only overlays
// <root>/usr/lib:/usr/lib and <root>/usr/sbin:/usr/sbin. Only that process sees them.
//
// charon-systemd is used instead of /usr/lib/ipsec/charon: charon's pid file is the
// compile-time /var/run/charon.pid (strongswan.conf(5) has no setting for it) and would collide
// between instances; charon-systemd writes none. Each daemon is started through a symlink
// /run/vrx-test/<prefix>/swan/<x>/charon-systemd so `pgrep -f /run/vrx-test/<prefix>/swan`
// finds exactly the test daemons. They are stopped by cancelling the runner context of the
// process they are (never by pattern) in Close, which t.Cleanup calls.
package swantest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/strongswan"
)

// Binaries the harness runs (internal/renderers/ALLOWLIST.md, test-only rows).
const (
	// IPBin creates the rig namespaces/veths and reads xfrm state. Never `ip netns exec`.
	IPBin = "/usr/bin/ip"
	// CharonBin is the IKE daemon (inside the overlay mount namespace when extracted).
	CharonBin = "/usr/sbin/charon-systemd"
)

// StockRoot is where README.md extracts the stock packages for slot prefix.
func StockRoot(prefix string) string {
	return filepath.Join("/run/vrx-test", prefix, "swan-stock", "root")
}

// FindRoot returns "/" when strongSwan is installed, the extracted stock root when present,
// else "" (the integration test skips with the instructions).
func FindRoot(prefix string) string {
	if isFile(CharonBin) && isFile(strongswan.SwanctlBin) {
		return "/"
	}
	root := StockRoot(prefix)
	if isFile(filepath.Join(root, "usr/sbin/charon-systemd")) && isFile(filepath.Join(root, "usr/sbin/swanctl")) &&
		isFile(filepath.Join(root, "usr/lib/ipsec/libcharon.so.0")) {
		return root
	}
	return ""
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

var prefixRe = regexp.MustCompile(`^w[0-9]{1,2}[a-z]?$`)

// Harness owns the slot's strongSwan test area. One per slot at a time (exclusive lock).
type Harness struct {
	Prefix string
	Root   string
	// Base is /run/vrx-test/<prefix>/swan.
	Base string
	// Runner runs ip (host namespace).
	Runner renderers.Runner
	lock   *os.File
	mu     sync.Mutex
	netns  []string
	procs  map[string]*Daemon
}

// Daemon is one running charon.
type Daemon struct {
	Name   string
	NetNS  string
	Paths  strongswan.Paths
	cancel context.CancelFunc
	done   chan error
}

// New takes the slot's strongSwan lock (waiting up to 5 minutes for another package's run —
// RF-1 review M4: harnesses of one slot must never kill each other's daemons) and prepares
// an empty base directory. Close releases everything.
func New(prefix, root string) (*Harness, error) {
	if !prefixRe.MatchString(prefix) {
		return nil, fmt.Errorf("swantest: prefix %q must match %s", prefix, prefixRe)
	}
	if root == "" {
		return nil, errors.New("swantest: no strongSwan binaries (see README.md)")
	}
	slot := filepath.Join("/run/vrx-test", prefix)
	if err := os.MkdirAll(slot, 0o755); err != nil { //nolint:gosec // shared test area
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(slot, "swan.lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // fixed path
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // fd fits in int
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = lock.Close()
			return nil, fmt.Errorf("swantest: %s/swan.lock held by another run", slot)
		}
		time.Sleep(time.Second)
	}
	h := &Harness{
		Prefix: prefix, Root: root, Base: filepath.Join(slot, "swan"), lock: lock, procs: map[string]*Daemon{},
		Runner: &renderers.SystemRunner{Allow: renderers.NewAllowlist(IPBin)},
	}
	// A previous run that was killed may have left the base; its daemons (if any) would still
	// answer on their sockets: refuse rather than guess which PIDs are ours.
	if entries, err := os.ReadDir(h.Base); err == nil {
		for _, e := range entries {
			sock := filepath.Join(h.Base, e.Name(), "charon.vici")
			if _, err := os.Stat(sock); err == nil {
				if c, err := strongswan.DialVICI(context.Background(), sock); err == nil {
					_ = c.Close()
					h.release()
					return nil, fmt.Errorf("swantest: a charon from a previous run still answers on %s; stop it by its PID (pgrep -af %s)", sock, h.Base)
				}
			}
		}
	}
	if err := os.RemoveAll(h.Base); err != nil {
		h.release()
		return nil, err
	}
	if err := os.MkdirAll(h.Base, 0o700); err != nil {
		h.release()
		return nil, err
	}
	return h, nil
}

func (h *Harness) release() {
	if h.lock != nil {
		_ = syscall.Flock(int(h.lock.Fd()), syscall.LOCK_UN) //nolint:gosec // fd fits in int
		_ = h.lock.Close()
		h.lock = nil
	}
}

// NetNS returns the namespace name ns-<prefix>-<x>.
func (h *Harness) NetNS(x string) string { return "ns-" + h.Prefix + "-" + x }

func (h *Harness) ip(ctx context.Context, args ...string) (string, error) {
	out, err := h.Runner.Run(ctx, renderers.Command{Path: IPBin, Args: args, Timeout: 10 * time.Second})
	if err != nil {
		return "", fmt.Errorf("swantest: ip %s: %w", strings.Join(args, " "), err)
	}
	return string(out.Stdout), nil
}

// AddNetNS creates ns-<prefix>-<x> with lo up (deleting a stale one of the same name).
func (h *Harness) AddNetNS(ctx context.Context, x string) (string, error) {
	ns := h.NetNS(x)
	if _, err := os.Stat(filepath.Join("/run/netns", ns)); err == nil {
		if _, err := h.ip(ctx, "netns", "delete", ns); err != nil {
			return "", err
		}
	}
	if _, err := h.ip(ctx, "netns", "add", ns); err != nil {
		return "", err
	}
	h.mu.Lock()
	h.netns = append(h.netns, ns)
	h.mu.Unlock()
	_, err := h.ip(ctx, "-n", ns, "link", "set", "lo", "up")
	return ns, err
}

// Link creates a veth pair <prefix>-<x> in ns-<prefix>-<x> ↔ <prefix>-<y> in ns-<prefix>-<y>
// with the given addresses (CIDR) and brings both ends up.
func (h *Harness) Link(ctx context.Context, x, addrX, y, addrY string) error {
	ifX, ifY := h.Prefix+"-"+x, h.Prefix+"-"+y
	steps := [][]string{
		{"link", "add", ifX, "netns", h.NetNS(x), "type", "veth", "peer", "name", ifY, "netns", h.NetNS(y)},
		{"-n", h.NetNS(x), "addr", "add", addrX, "dev", ifX},
		{"-n", h.NetNS(y), "addr", "add", addrY, "dev", ifY},
		{"-n", h.NetNS(x), "link", "set", ifX, "up"},
		{"-n", h.NetNS(y), "link", "set", ifY, "up"},
	}
	for _, s := range steps {
		if _, err := h.ip(ctx, s...); err != nil {
			return err
		}
	}
	return nil
}

// XfrmState returns `ip -n <ns> xfrm state` with key material removed (only the src/dst and
// proto/spi/mode lines are kept).
func (h *Harness) XfrmState(ctx context.Context, ns string) (string, error) {
	out, err := h.ip(ctx, "-n", ns, "xfrm", "state")
	if err != nil {
		return "", err
	}
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "src ") || strings.HasPrefix(t, "proto ") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n"), nil
}

// XfrmPolicy returns `ip -n <ns> xfrm policy` (no key material in policies).
func (h *Harness) XfrmPolicy(ctx context.Context, ns string) (string, error) {
	return h.ip(ctx, "-n", ns, "xfrm", "policy")
}

// Paths returns the renderer paths of instance x.
func (h *Harness) Paths(x string) strongswan.Paths { return strongswan.TestPaths(h.Prefix, x) }

// Start writes conf as instance x's strongswan.conf (rendered by the renderer: it names the
// VICI socket and the log), creates the swanctl directories and starts charon-systemd in
// netns ns. It returns when the VICI socket answers.
func (h *Harness) Start(ctx context.Context, x, ns string, conf []byte) (*Daemon, error) {
	p := h.Paths(x)
	dir := filepath.Dir(p.StrongswanConf)
	if !strings.HasPrefix(dir, h.Base+"/") {
		return nil, fmt.Errorf("swantest: instance dir %s outside %s", dir, h.Base)
	}
	for _, d := range []string{"conf.d", "x509", "x509ca", "pubkey", "private"} {
		if err := os.MkdirAll(filepath.Join(p.SwanctlDir, d), 0o700); err != nil {
			return nil, err
		}
	}
	// The packaged top-level swanctl.conf (constant).
	if err := renderers.WriteFileAtomic(p.SwanctlConf(), renderers.File{Mode: 0o600, Content: []byte("include conf.d/*.conf\n")}); err != nil {
		return nil, err
	}
	if err := renderers.WriteFileAtomic(p.StrongswanConf, renderers.File{Mode: 0o640, Content: conf}); err != nil {
		return nil, err
	}
	link := filepath.Join(dir, "charon-systemd")
	_ = os.Remove(link)
	if err := os.Symlink(CharonBin, link); err != nil {
		return nil, err
	}
	d := &Daemon{Name: x, NetNS: ns, Paths: p, done: make(chan error, 1)}
	runner := &renderers.SystemRunner{
		Allow: renderers.NewAllowlist(link),
		Env:   []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "STRONGSWAN_CONF=" + p.StrongswanConf},
	}
	dctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	started := make(chan error, 1)
	go func() {
		// This goroutine's OS thread enters the namespaces and is never handed back to the
		// scheduler (no UnlockOSThread): it exits with the goroutine.
		runtime.LockOSThread()
		if err := enter(h.Root, ns); err != nil {
			started <- err
			return
		}
		started <- nil
		_, err := runner.Run(dctx, renderers.Command{Path: link, Timeout: 2 * time.Hour})
		d.done <- err
	}()
	if err := <-started; err != nil {
		cancel()
		return nil, fmt.Errorf("swantest: enter namespaces for %s: %w", x, err)
	}
	h.mu.Lock()
	h.procs[x] = d
	h.mu.Unlock()
	deadline := time.Now().Add(15 * time.Second)
	for {
		c, err := strongswan.DialVICI(ctx, p.ViciSocket)
		if err == nil {
			_ = c.Close()
			return d, nil
		}
		select {
		case err := <-d.done:
			d.done <- err
			return nil, fmt.Errorf("swantest: charon %s exited during start: %v (log %s)", x, err, p.LogFile)
		default:
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("swantest: charon %s: VICI socket %s not answering: %v", x, p.ViciSocket, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop ends instance x like `systemctl stop` would: SIGTERM to the process the harness
// spawned (identified by its argv0 — the instance's symlink — and by being our child), so
// charon deletes its SAs and kernel state; SIGKILL through the runner context after 10 s.
// A SIGKILLed charon (kernel-netlink) leaves its xfrm states behind in the namespace.
func (h *Harness) Stop(x string) error {
	h.mu.Lock()
	d := h.procs[x]
	delete(h.procs, x)
	h.mu.Unlock()
	if d == nil {
		return nil
	}
	if pid := h.childPID(filepath.Join(filepath.Dir(d.Paths.StrongswanConf), "charon-systemd")); pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	select {
	case <-d.done:
	case <-time.After(10 * time.Second):
		d.cancel() // SIGKILL: charon cannot remove its kernel state, so the harness does
		select {
		case <-d.done:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("swantest: charon %s did not exit", x)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.FlushXfrm(ctx, d.NetNS)
	}
	d.cancel()
	_ = os.Remove(d.Paths.ViciSocket)
	return nil
}

// Crash SIGKILLs instance x (the PID the harness spawned) to simulate a charon crash: its
// kernel state (xfrm SAs/policies) stays behind, as it would in production. FlushXfrm cleans it.
func (h *Harness) Crash(x string) error {
	h.mu.Lock()
	d := h.procs[x]
	delete(h.procs, x)
	h.mu.Unlock()
	if d == nil {
		return fmt.Errorf("swantest: no daemon %s", x)
	}
	if pid := h.childPID(filepath.Join(filepath.Dir(d.Paths.StrongswanConf), "charon-systemd")); pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	d.cancel()
	select {
	case <-d.done:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("swantest: charon %s did not exit", x)
	}
	_ = os.Remove(d.Paths.ViciSocket)
	return nil
}

// FlushXfrm removes every xfrm state and policy in the namespace (only rig namespaces of this
// harness: ns-<prefix>-*).
func (h *Harness) FlushXfrm(ctx context.Context, ns string) error {
	if !strings.HasPrefix(ns, "ns-"+h.Prefix+"-") {
		return fmt.Errorf("swantest: %s is not a namespace of this harness", ns)
	}
	if _, err := h.ip(ctx, "-n", ns, "xfrm", "state", "flush"); err != nil {
		return err
	}
	_, err := h.ip(ctx, "-n", ns, "xfrm", "policy", "flush")
	return err
}

// childPID finds our own child process whose argv0 is argv0 (0 when none).
func (h *Harness) childPID(argv0 string) int {
	self := os.Getpid()
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline")) //nolint:gosec // procfs
		if err != nil {
			continue
		}
		if a0, _, _ := strings.Cut(string(cmd), "\x00"); a0 != argv0 {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat")) //nolint:gosec // procfs
		if err != nil {
			continue
		}
		// stat: pid (comm) state ppid …; comm may contain blanks, so split after the last ')'.
		rest := string(stat[strings.LastIndexByte(string(stat), ')')+1:])
		fields := strings.Fields(rest)
		if len(fields) > 1 && fields[1] == strconv.Itoa(self) {
			return pid
		}
	}
	return 0
}

// Running lists the PIDs of test daemons of this harness still alive: processes whose argv[0]
// is below Base (the per-instance charon-systemd symlink).
func (h *Harness) Running() []string {
	var pids []string
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		if strings.Trim(e.Name(), "0123456789") != "" {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline")) //nolint:gosec // procfs
		if err != nil {
			continue
		}
		argv0, _, _ := strings.Cut(string(cmd), "\x00")
		if strings.HasPrefix(argv0, h.Base+"/") {
			pids = append(pids, e.Name())
		}
	}
	return pids
}

// NSRunner returns a runner that executes allow-listed commands (swanctl) inside the overlay
// mount namespace (host network namespace; VICI is a unix socket), with STRONGSWAN_CONF set.
func (h *Harness) NSRunner(strongswanConf string) renderers.Runner {
	return &nsRunner{root: h.Root, inner: &renderers.SystemRunner{
		Allow: renderers.NewAllowlist(strongswan.SwanctlBin),
		Env:   []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "STRONGSWAN_CONF=" + strongswanConf},
	}}
}

type nsRunner struct {
	root  string
	inner renderers.Runner
}

func (n *nsRunner) Run(ctx context.Context, cmd renderers.Command) (renderers.Output, error) {
	type result struct {
		out renderers.Output
		err error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread dies with the goroutine
		if err := enter(n.root, ""); err != nil {
			ch <- result{err: err}
			return
		}
		out, err := n.inner.Run(ctx, cmd)
		ch <- result{out, err}
	}()
	r := <-ch
	return r.out, r.err
}

// enter moves the calling (locked) OS thread into a private mount namespace with the stock
// overlays (root != "/") and, when ns is set, into that network namespace.
func enter(root, ns string) error {
	if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
		return fmt.Errorf("unshare(CLONE_NEWNS): %w", err)
	}
	// Private propagation first: the overlays must never propagate to the host namespace.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make / rprivate: %w", err)
	}
	if root != "/" {
		for _, d := range []string{"/usr/lib", "/usr/sbin"} {
			if err := syscall.Mount("overlay", d, "overlay", syscall.MS_RDONLY, "lowerdir="+root+d+":"+d); err != nil {
				return fmt.Errorf("overlay %s: %w", d, err)
			}
		}
	}
	if ns != "" {
		f, err := os.Open(filepath.Join("/run/netns", ns)) //nolint:gosec // ns-<prefix>-<x>
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := unix.Setns(int(f.Fd()), unix.CLONE_NEWNET); err != nil { //nolint:gosec // fd fits in int
			return fmt.Errorf("setns(%s): %w", ns, err)
		}
	}
	return nil
}

// Close stops every daemon, deletes the namespaces, removes Base and releases the lock. It
// returns the PIDs still running afterwards as an error (must never happen).
func (h *Harness) Close() error {
	var errs []error
	h.mu.Lock()
	names := make([]string, 0, len(h.procs))
	for x := range h.procs {
		names = append(names, x)
	}
	h.mu.Unlock()
	for _, x := range names {
		if err := h.Stop(x); err != nil {
			errs = append(errs, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, ns := range h.netns {
		if _, err := h.ip(ctx, "netns", "delete", ns); err != nil {
			errs = append(errs, err)
		}
	}
	if left := h.Running(); len(left) > 0 {
		errs = append(errs, fmt.Errorf("swantest: test daemons still running: %v", left))
	}
	if err := os.RemoveAll(h.Base); err != nil {
		errs = append(errs, err)
	}
	h.release()
	return errors.Join(errs...)
}
