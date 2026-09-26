// Package frrtest is the test-scoped FRR daemon harness for integration tests of the FRR
// renderer and of every protocol task that plugs into it (P12 bgpd, F-ospf ospfd, …). It
// follows docs/lab/shared-host-rules.md §3/§5: the daemons are child processes of the test
// with a pathspace (-N <prefix>), config and socket directories under
// /run/vrx-test/<prefix>/frr, inside a network namespace named after the slot prefix; vty
// bound to 127.0.0.1 with the TCP port disabled (-P 0, vtysh uses the unix sockets); never
// the system unit, never /etc/frr, never ens192. Daemons are stopped by the PIDs the harness
// spawned (their own pidfiles), never by pattern.
//
//	h := frrtest.Start(t, frrtest.Options{Prefix: "w12", Links: []frrtest.Link{{Name: "w12f0", Kind: "dummy", CIDR: "10.12.1.2/24"}}})
//	r := h.Renderer()              // frr.New(h.Runner, frr.WithPaths(h.Paths), …)
//	files, _ := r.Render(ctx, desired); h.AssertScoped(t, files); _ = r.Apply(ctx, files)
package frrtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
)

// Binaries the harness runs (listed in internal/renderers/ALLOWLIST.md, test-only rows).
const (
	IPBin      = "/usr/bin/ip"
	MgmtdBin   = "/usr/lib/frr/mgmtd"
	ZebraBin   = "/usr/lib/frr/zebra"
	StaticdBin = "/usr/lib/frr/staticd"
	// Protocol daemons for P12/F-* integration tests (started only when Options.Daemons names them).
	BgpdBin   = "/usr/lib/frr/bgpd"
	OspfdBin  = "/usr/lib/frr/ospfd"
	Ospf6dBin = "/usr/lib/frr/ospf6d"
	BfddBin   = "/usr/lib/frr/bfdd"
	PimdBin   = "/usr/lib/frr/pimd"
	IsisdBin  = "/usr/lib/frr/isisd"
	RipdBin   = "/usr/lib/frr/ripd"
	LdpdBin   = "/usr/lib/frr/ldpd"
)

var daemonBins = map[string]string{
	"mgmtd": MgmtdBin, "zebra": ZebraBin, "staticd": StaticdBin,
	"bgpd": BgpdBin, "ospfd": OspfdBin, "ospf6d": Ospf6dBin, "bfdd": BfddBin,
	"pimd": PimdBin, "isisd": IsisdBin, "ripd": RipdBin, "ldpd": LdpdBin,
}

// FrameworkDaemons are started by default, in this order (mgmtd first: FRR 10 routes
// staticd/zebra configuration through it).
var FrameworkDaemons = []string{"mgmtd", "zebra", "staticd"}

// Binaries is the harness allowlist: ip, the FRR daemons and the renderer's own tools.
func Binaries() []string {
	out := append([]string{IPBin}, frr.Binaries()...)
	for _, p := range daemonBins {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Link is a network device the harness creates inside its own namespace.
type Link struct {
	Name string // must start with the slot prefix
	Kind string // "dummy" or "vrf"
	// CIDR is an address for a dummy link ("10.12.1.2/24"); "" = none.
	CIDR string
	// Table is the kernel table of a vrf link (use the slot's VRX_VPP_TABLE_BASE range).
	Table uint32
	// Master enslaves a dummy link to a vrf link created before it.
	Master string
}

// Options configure Start.
type Options struct {
	// Prefix is the slot prefix (VRX_TEST_PREFIX, "w12"); required.
	Prefix string
	// NetNS is an existing namespace to reuse (e.g. the rig's ns-<prefix>-lan); it must carry
	// the prefix and is not deleted. "" = create ns-<prefix>-frr and delete it afterwards.
	NetNS string
	// Daemons to start, in order; nil = FrameworkDaemons.
	Daemons []string
	// Links to create in a harness-owned namespace (ignored with NetNS).
	Links []Link
	// Instance names one more FRR instance of the same slot ("p1", "p2": 1–3 of [a-z0-9]; "" = the slot's main
	// instance). It gets its own pathspace <Prefix><Instance>, base /run/vrx-test/<Prefix>/frr-<Instance>, lock
	// /run/vrx-test/<Prefix>/frr-<Instance>.lock and default namespace ns-<Prefix>-<Instance>, so a topology test can
	// run the VRX-side FRR and its peers in one slot (P12). Object names still carry the slot prefix.
	Instance string
}

// Harness is a running test-scoped FRR.
type Harness struct {
	Paths  frr.Paths
	NetNS  string
	Runner renderers.Runner
	// Base is /run/vrx-test/<prefix>/frr.
	Base string

	prefix   string
	instance string
	daemons  []string
	pids     map[string]int
	ownNetNS bool
	symlink  string
	uid, gid int // of user frr
	lock     *os.File
}

// LockFile is the slot-scoped harness lock (flock LOCK_EX for the harness lifetime).
func LockFile(prefix string) string { return filepath.Join("/run/vrx-test", prefix, "frr.lock") }

// InstanceLockFile is the lock of one named instance of a slot ("" = LockFile).
func InstanceLockFile(prefix, instance string) string {
	if instance == "" {
		return LockFile(prefix)
	}
	return filepath.Join("/run/vrx-test", prefix, "frr-"+instance+".lock")
}

func (h *Harness) lockSlot() error {
	path := InstanceLockFile(h.prefix, h.instance)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // /run/vrx-test/<prefix>, shared-host layout
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // slot lock file, no content
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return fmt.Errorf("lock %s: %w", path, err)
	}
	h.lock = f
	return nil
}

var (
	prefixRe   = regexp.MustCompile(`^[a-z][a-z0-9]{1,5}$`)
	instanceRe = regexp.MustCompile(`^[a-z0-9]{1,3}$`)
)

const startTimeout = 30 * time.Second

// Start creates the directories and namespace, starts the daemons and registers Stop with
// t.Cleanup. Any failure is fatal for t (after cleaning up what was created).
func Start(t testing.TB, opts Options) *Harness {
	t.Helper()
	h, err := start(context.Background(), opts)
	if h != nil {
		t.Cleanup(func() {
			if err := h.Stop(); err != nil {
				t.Errorf("frrtest: stop: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("frrtest: %v", err)
	}
	return h
}

func start(ctx context.Context, opts Options) (*Harness, error) {
	if !prefixRe.MatchString(opts.Prefix) {
		return nil, fmt.Errorf("prefix %q must match %s (VRX_TEST_PREFIX)", opts.Prefix, prefixRe)
	}
	daemons := opts.Daemons
	if daemons == nil {
		daemons = FrameworkDaemons
	}
	for _, d := range daemons {
		if _, ok := daemonBins[d]; !ok {
			return nil, fmt.Errorf("unknown daemon %q", d)
		}
	}
	if opts.Instance != "" && (!instanceRe.MatchString(opts.Instance) || len(opts.Prefix)+len(opts.Instance) > 6) {
		return nil, fmt.Errorf("instance %q must match %s and fit the 6-character pathspace with the prefix", opts.Instance, instanceRe)
	}
	paths := frr.TestInstancePaths(opts.Prefix, opts.Instance)
	h := &Harness{
		Paths:    paths,
		Runner:   &renderers.SystemRunner{Allow: renderers.NewAllowlist(Binaries()...), MaxOutput: frr.MaxShowOutput},
		Base:     filepath.Dir(paths.ConfDir),
		prefix:   opts.Prefix,
		instance: opts.Instance,
		daemons:  slices.Clone(daemons),
		pids:     map[string]int{},
		symlink:  filepath.Join("/run/frr", paths.Namespace),
	}
	if !strings.HasPrefix(h.Base, "/run/vrx-test/"+opts.Prefix+"/") {
		return nil, fmt.Errorf("base %s is not under /run/vrx-test/%s", h.Base, opts.Prefix)
	}
	// One harness per slot prefix at a time (RF-1 review M4): packages run in parallel under
	// the same VRX_TEST_PREFIX would otherwise kill each other's daemons in killStale and
	// remove each other's directories. The lock file lives next to (not in) the base dir.
	if err := h.lockSlot(); err != nil {
		return nil, err
	}
	// Leftovers of a killed earlier run of this harness (same prefix, same paths) go first.
	h.killStale()
	if err := os.RemoveAll(h.Base); err != nil {
		return nil, err
	}
	if err := h.prepareDirs(); err != nil {
		return h, err
	}
	if err := h.prepareNetNS(ctx, opts); err != nil {
		return h, err
	}
	for _, d := range h.daemons {
		if err := h.startDaemon(ctx, d); err != nil {
			return h, err
		}
	}
	return h, nil
}

func (h *Harness) prepareDirs() error {
	frrUser, err := user.Lookup("frr")
	if err != nil {
		return fmt.Errorf("user frr: %w (is FRR installed?)", err)
	}
	h.uid, _ = strconv.Atoi(frrUser.Uid)
	h.gid, _ = strconv.Atoi(frrUser.Gid)
	// The base and run directories must be traversable by user frr (sockets, logs and
	// zebra.conf live below them); the config dir stays root-only; the socket dir belongs to
	// frr because the daemons drop privileges (they refuse -u root: vty group).
	sock := h.Paths.SocketDir()
	for _, dir := range []string{h.Base, h.Paths.RunDir} {
		if err := os.MkdirAll(dir, 0o711); err != nil { //nolint:gosec // traverse-only for user frr
			return err
		}
		if err := os.Chmod(dir, 0o711); err != nil { //nolint:gosec // traverse-only for user frr
			return err
		}
	}
	if err := os.MkdirAll(h.Paths.ConfSubdir(), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(sock, 0o750); err != nil {
		return err
	}
	if err := os.Chown(sock, h.uid, h.gid); err != nil {
		return err
	}
	if err := os.WriteFile(h.zebraConf(), nil, 0o644); err != nil { //nolint:gosec // empty daemon config, not secret
		return err
	}
	// mgmtd binds mgmtd_fe.sock/mgmtd_be.sock in /var/run/frr/<pathspace> whatever
	// --vty_socket says (FRR 10.7), and the daemons connect there: point that pathspace
	// directory at the test socket directory. Only a missing path or our own symlink is
	// accepted.
	if info, err := os.Lstat(h.symlink); err == nil {
		target, _ := os.Readlink(h.symlink)
		if info.Mode()&os.ModeSymlink == 0 || target != sock {
			return fmt.Errorf("%s exists and is not the harness symlink to %s: refusing to touch it", h.symlink, sock)
		}
		return nil
	}
	if _, err := os.Stat(filepath.Dir(h.symlink)); err != nil {
		return fmt.Errorf("%s missing (FRR package installs it via tmpfiles): %w", filepath.Dir(h.symlink), err)
	}
	return os.Symlink(sock, h.symlink)
}

func (h *Harness) zebraConf() string { return filepath.Join(h.Base, "zebra.conf") }

func (h *Harness) ip(ctx context.Context, args ...string) (string, error) {
	out, err := h.Runner.Run(ctx, renderers.Command{Path: IPBin, Args: args, Timeout: startTimeout})
	if err != nil {
		return "", fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, bytes.TrimSpace(out.Stderr))
	}
	return string(out.Stdout), nil
}

func (h *Harness) prepareNetNS(ctx context.Context, opts Options) error {
	if opts.NetNS != "" {
		if !strings.Contains(opts.NetNS, h.prefix) {
			return fmt.Errorf("namespace %q does not carry the prefix %q", opts.NetNS, h.prefix)
		}
		h.NetNS = opts.NetNS
	} else {
		h.NetNS = "ns-" + h.prefix + "-frr"
		if h.instance != "" {
			h.NetNS = "ns-" + h.prefix + "-" + h.instance
		}
		list, err := h.ip(ctx, "netns", "list")
		if err != nil {
			return err
		}
		for _, l := range strings.Split(list, "\n") {
			if f := strings.Fields(l); len(f) > 0 && f[0] == h.NetNS {
				if _, err := h.ip(ctx, "netns", "delete", h.NetNS); err != nil { // leftover of a killed run
					return err
				}
			}
		}
		if _, err := h.ip(ctx, "netns", "add", h.NetNS); err != nil {
			return err
		}
		h.ownNetNS = true
		if _, err := h.ip(ctx, "-n", h.NetNS, "link", "set", "lo", "up"); err != nil {
			return err
		}
		for _, l := range opts.Links {
			if err := h.addLink(ctx, l); err != nil {
				return err
			}
		}
	}
	links, err := h.ip(ctx, "-n", h.NetNS, "-o", "link", "show")
	if err != nil {
		return err
	}
	if strings.Contains(links, "ens192") {
		return fmt.Errorf("namespace %s contains the management NIC ens192: refusing to start FRR there", h.NetNS)
	}
	return nil
}

func (h *Harness) addLink(ctx context.Context, l Link) error {
	if !strings.HasPrefix(l.Name, h.prefix) || len(l.Name) > 15 {
		return fmt.Errorf("link %q must start with %q and fit IFNAMSIZ", l.Name, h.prefix)
	}
	ns := []string{"-n", h.NetNS}
	switch l.Kind {
	case "vrf":
		if _, err := h.ip(ctx, append(ns, "link", "add", l.Name, "type", "vrf", "table", strconv.FormatUint(uint64(l.Table), 10))...); err != nil {
			return err
		}
	case "dummy":
		if _, err := h.ip(ctx, append(ns, "link", "add", l.Name, "type", "dummy")...); err != nil {
			return err
		}
		if l.Master != "" {
			if _, err := h.ip(ctx, append(ns, "link", "set", l.Name, "master", l.Master)...); err != nil {
				return err
			}
		}
		if l.CIDR != "" {
			if _, err := h.ip(ctx, append(ns, "addr", "add", l.CIDR, "dev", l.Name)...); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("link %q: kind %q is not dummy or vrf", l.Name, l.Kind)
	}
	_, err := h.ip(ctx, append(ns, "link", "set", l.Name, "up")...)
	return err
}

// DaemonArgs is the argv (after `ip netns exec <ns>`) used for daemon d; exported so tests
// can assert what is bound before anything starts.
func (h *Harness) DaemonArgs(d string) []string {
	sock := h.Paths.SocketDir()
	args := []string{daemonBins[d], "-d", "-N", h.Paths.Namespace, "--vty_socket", sock,
		"-i", h.pidFile(d), "-A", "127.0.0.1", "-P", "0",
		"--log", "file:" + filepath.Join(sock, d+".log"), "--log-level", "warn"}
	if d != "mgmtd" {
		args = append(args, "-z", filepath.Join(sock, "zserv.api"))
	}
	if d == "zebra" {
		args = append(args, "-f", h.zebraConf())
	}
	return args
}

func (h *Harness) pidFile(d string) string { return filepath.Join(h.Paths.SocketDir(), d+".pid") }

// assertScopedArgv enforces the binding rules before a daemon starts.
func (h *Harness) assertScopedArgv(args []string) error {
	joined := strings.Join(args, " ")
	switch {
	case !strings.Contains(joined, " -A 127.0.0.1 -P 0"):
		return fmt.Errorf("daemon argv does not bind vty to 127.0.0.1 with the TCP port disabled: %s", joined)
	case strings.Contains(joined, "ens192"), strings.Contains(joined, "/etc/frr"):
		return fmt.Errorf("daemon argv names ens192 or /etc/frr: %s", joined)
	case h.NetNS == "":
		return errors.New("refusing to start a daemon in the root network namespace")
	}
	for i, a := range args {
		if (a == "-i" || a == "--vty_socket" || a == "-z" || a == "-f") && !strings.HasPrefix(args[i+1], h.Base+"/") {
			return fmt.Errorf("daemon path %s %s is outside %s", a, args[i+1], h.Base)
		}
	}
	return nil
}

func (h *Harness) startDaemon(ctx context.Context, d string) error {
	args := h.DaemonArgs(d)
	if err := h.assertScopedArgv(args); err != nil {
		return err
	}
	// FRR opens --log file: after dropping to user frr and does not create it: pre-create it.
	logFile := filepath.Join(h.Paths.SocketDir(), d+".log")
	if err := os.WriteFile(logFile, nil, 0o640); err != nil { //nolint:gosec // test daemon log
		return err
	}
	if err := os.Chown(logFile, h.uid, h.gid); err != nil {
		return err
	}
	// -d: FRR forks and the parent exits once the daemon is up; the pidfile names the daemon.
	// The daemon keeps the inherited stdout/stderr open, so the runner reports
	// exec.ErrWaitDelay after its 2 s grace period although ip exited 0: that is success.
	out, err := h.Runner.Run(ctx, renderers.Command{Path: IPBin, Args: append([]string{"netns", "exec", h.NetNS}, args...), Timeout: startTimeout})
	if err != nil && (!errors.Is(err, exec.ErrWaitDelay) || out.ExitCode != 0) {
		return fmt.Errorf("start %s: %w: %s", d, err, bytes.TrimSpace(out.Stderr))
	}
	deadline := time.Now().Add(startTimeout)
	for {
		pid, err := h.readPID(d)
		if err == nil && alive(pid) && h.ours(d, pid) {
			if _, err := os.Stat(filepath.Join(h.Paths.SocketDir(), d+".vty")); err == nil {
				h.pids[d] = pid
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("start %s: no live pid/vty socket after %s (see %s)", d, startTimeout, filepath.Join(h.Paths.SocketDir(), d+".log"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *Harness) readPID(d string) (int, error) {
	b, err := os.ReadFile(h.pidFile(d))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// ours reports whether pid is daemon d started by this harness: /proc/<pid>/exe is the
// daemon binary and its argv carries `-i <this harness's pidfile for d>` (a guard against a
// recycled PID or an unrelated process whose cmdline mentions the path, e.g. `tail -f`).
func (h *Harness) ours(d string, pid int) bool {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || exe != daemonBins[d] {
		return false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return err == nil && bytes.Contains(b, []byte("\x00-i\x00"+h.pidFile(d)+"\x00"))
}

// PIDs returns the current PID of each started daemon (from its pidfile).
func (h *Harness) PIDs() map[string]int {
	out := map[string]int{}
	for _, d := range h.daemons {
		if pid, err := h.readPID(d); err == nil {
			out[d] = pid
		}
	}
	return out
}

// Renderer returns an FRR renderer bound to this harness (paths, runner, IdentityMapper —
// the test namespace's devices are Linux interfaces; later opts override).
func (h *Harness) Renderer(opts ...frr.Option) *frr.Renderer {
	base := []frr.Option{frr.WithPaths(h.Paths), frr.WithInterfaceMapper(frr.IdentityMapper)}
	return frr.New(h.Runner, append(base, opts...)...)
}

// AssertScoped fails t when a rendered config names an interface or VRF without the slot
// prefix, or mentions ens192 — checked before anything is applied.
func (h *Harness) AssertScoped(t testing.TB, files renderers.Files) {
	t.Helper()
	for p, f := range files {
		if !strings.HasPrefix(p, h.Base+"/") {
			t.Fatalf("frrtest: rendered path %s outside %s", p, h.Base)
		}
		for i, l := range strings.Split(string(f.Content), "\n") {
			fields := strings.Fields(l)
			if strings.Contains(l, "ens192") {
				t.Fatalf("frrtest: %s:%d mentions ens192: %q", p, i+1, l)
			}
			if len(fields) >= 2 && (fields[0] == "interface" || fields[0] == "vrf") && !strings.HasPrefix(fields[1], h.prefix) {
				t.Fatalf("frrtest: %s:%d %s %q lacks the prefix %q", p, i+1, fields[0], fields[1], h.prefix)
			}
		}
	}
}

// Stop terminates the daemons (SIGTERM, then SIGKILL after 10 s) by the PIDs this harness
// spawned, removes the pathspace symlink, the harness-owned namespace and the base
// directory. It is idempotent.
func (h *Harness) Stop() error {
	var errs []error
	for _, d := range slices.Backward(h.daemons) {
		pid, ok := h.pids[d]
		if !ok {
			if p, err := h.readPID(d); err == nil && h.ours(d, p) {
				pid, ok = p, true
			}
		}
		if ok {
			if err := h.kill(d, pid); err != nil {
				errs = append(errs, fmt.Errorf("%s (pid %d): %w", d, pid, err))
			}
			delete(h.pids, d)
		}
	}
	if target, err := os.Readlink(h.symlink); err == nil && target == h.Paths.SocketDir() {
		if err := os.Remove(h.symlink); err != nil {
			errs = append(errs, err)
		}
	}
	if h.ownNetNS {
		if _, err := h.ip(context.Background(), "netns", "delete", h.NetNS); err != nil {
			errs = append(errs, err)
		}
		h.ownNetNS = false
	}
	if err := os.RemoveAll(h.Base); err != nil {
		errs = append(errs, err)
	}
	if h.lock != nil {
		_ = h.lock.Close() // releases the slot flock
		h.lock = nil
	}
	return errors.Join(errs...)
}

func (h *Harness) kill(d string, pid int) error {
	if !alive(pid) {
		return nil
	}
	if !h.ours(d, pid) {
		return fmt.Errorf("pid %d is not a harness daemon any more: not signalled", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for alive(pid) && h.ours(d, pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			time.Sleep(200 * time.Millisecond)
			if alive(pid) && h.ours(d, pid) {
				return errors.New("still running after SIGKILL")
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// killStale stops daemons left by an earlier, killed run of this harness (pidfiles under
// the same test paths, cmdline naming the same socket dir) and removes the stale symlink.
func (h *Harness) killStale() {
	for d := range daemonBins {
		if pid, err := h.readPID(d); err == nil && alive(pid) && h.ours(d, pid) {
			_ = h.kill(d, pid)
		}
	}
	if target, err := os.Readlink(h.symlink); err == nil && target == h.Paths.SocketDir() {
		_ = os.Remove(h.symlink)
	}
}
