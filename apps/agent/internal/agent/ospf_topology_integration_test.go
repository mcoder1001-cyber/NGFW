package agent

// F-ospf topology test on this host (NGFW_INTEGRATION=1, shared lab lock, slot prefix; row F-ospf-host, the RV-A R7/R4/R1
// owed lists). Same layout as the P12 topology test (p12_topology_integration_test.go, whose helpers it reuses):
//
//	NGFW:   the in-process agent (owner = slot prefix, NGFW_FRR_PATHSPACE = slot) → VPP af_packet interfaces host-<p>l0/w0 on
//	       the slot's veth rig, each with a linux-cp pair (taps <p>-l0, <p>-w0); the slot's FRR (mgmtd, zebra, staticd,
//	       ospfd) runs OSPFv2 area 0 on both taps (point-to-point, hello 1 s / dead 4 s).
//	peers: two frrtest ospfd instances (p1 in ns-<p>-lan, p2 in ns-<p>-wan), each redistributing 50 blackhole statics
//	       (10.<N>.64–113.0/24 and 10.<N>.128–177.0/24) into area 0.
//
// Steps: commit → both adjacencies Full, 100 OSPF routes in FRR's RIB → Retrieve == desired → peer 1 withdraws → 50
// within 10 s → announces again → 100 → agent restart with both pairs deleted behind its back → pairs recreated,
// adjacencies and routes back without any API call (time logged, target ≤ 30 s) → rollback of routing.ospf → no
// `router ospf`, no OSPF route → cleanup (peers down before the agent deletes its af_packet interfaces, D-101/V24).
//
// Where FRR runs decides whether VPP's FIB can be checked (P12-questions Q1: linux_nl's one netlink socket opens in
// the lcp default netns — unset = root on the shared VPP — with the first pair and hears only pairs of that netns):
//
//   - default: the NGFW-side FRR is a frrtest harness in ns-<p>-frr and the pairs carry that netns (P12's T1 layout).
//     FRR's RIB is checked; VPP's FIB cannot hear it. Nothing runs in the root namespace.
//   - NGFW_OSPF_FIB=root (P12 Q1's T3, D-119 M3 "a root-netns zebra is always one slot at a time"): the NGFW-side FRR runs
//     in the ROOT network namespace with the slot pathspace (-N <p>, everything under /run/ngfw-test/<p>/frr, vty on
//     unix sockets only — the frrtest harness refuses the root namespace by design, so rootFRR below spawns it), the
//     pairs carry no netns, zebra's kernel routes reach VPP table 0 through linux_nl and the test also counts
//     ListRoutes(source lcp-rt-dynamic) and pastes `show ip fib`. Preconditions (fail closed): no LCP pair exists on the
//     VPP, the lcp default netns is unset, no FRR-protocol route is in the root kernel table (zebra sweeps those at
//     start), the system frr unit is inactive. The exclusive globals lock (/run/lock/ngfw-globals.lock, D-167) is held
//     for the whole run inside the shared lab lock; after the restart simulation a withdraw/announce cycle proves the
//     recreated pairs are heard (a deleted pair flushes no route, P12.md); every route is withdrawn before the last
//     pair is deleted, so nothing of this slot stays in the shared table 0.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/interface_types"
	lcpapi "ngfw/agent/binapi/lcp"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/renderers/frr/ospf"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// EnvOSPFTopology runs the OSPF topology test (test/topology/ospf/run.sh sets it).
const EnvOSPFTopology = "NGFW_OSPF_TOPOLOGY"

// EnvOSPFFIB = "root" runs the NGFW-side FRR in the root network namespace (see the file comment) and adds the VPP FIB
// checks; anything else keeps FRR in ns-<p>-frr and checks FRR's RIB only.
const EnvOSPFFIB = "NGFW_OSPF_FIB"

const globalsLockFile = "/run/lock/ngfw-globals.lock"

// rootFRR is a slot-scoped FRR (pathspace <prefix>, /run/ngfw-test/<prefix>/frr, unix vty sockets only) in the root
// network namespace — the one layout frrtest refuses. Same directories, argv, pidfiles, slot lock and stop rules as the
// harness, no `ip netns exec`.
type rootFRR struct {
	Paths   frr.Paths
	Base    string
	Runner  renderers.Runner
	daemons []string
	pids    map[string]int
	symlink string
	uid     int
	gid     int
	lock    *os.File
}

var rootFRRBins = map[string]string{"mgmtd": frrtest.MgmtdBin, "zebra": frrtest.ZebraBin, "staticd": frrtest.StaticdBin, "ospfd": frrtest.OspfdBin}

func startRootFRR(t *testing.T, prefix string, daemons []string) *rootFRR {
	t.Helper()
	paths := frr.TestPaths(prefix)
	h := &rootFRR{Paths: paths, Base: filepath.Dir(paths.ConfDir), daemons: daemons, pids: map[string]int{},
		symlink: filepath.Join("/run/frr", paths.Namespace),
		Runner:  &renderers.SystemRunner{Allow: renderers.NewAllowlist(frrtest.Binaries()...), MaxOutput: frr.MaxShowOutput}}
	t.Cleanup(func() {
		if err := h.Stop(); err != nil {
			t.Errorf("rootFRR: stop: %v", err)
		}
	})
	// the harness's slot lock: one FRR of this pathspace at a time
	lockPath := frrtest.LockFile(prefix)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil { //nolint:gosec // /run/ngfw-test/<prefix>
		t.Fatal(err)
	}
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // slot lock file
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("rootFRR: %s is held (another FRR of pathspace %s is running): %v", lockPath, prefix, err)
	}
	h.lock = f
	frrUser, err := user.Lookup("frr")
	if err != nil {
		t.Fatalf("user frr: %v", err)
	}
	h.uid, _ = strconv.Atoi(frrUser.Uid)
	h.gid, _ = strconv.Atoi(frrUser.Gid)
	if err := os.RemoveAll(h.Base); err != nil {
		t.Fatal(err)
	}
	sock := paths.SocketDir()
	for _, dir := range []string{h.Base, paths.RunDir} {
		if err := os.MkdirAll(dir, 0o711); err != nil { //nolint:gosec // traverse-only for user frr
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o711); err != nil { //nolint:gosec // traverse-only for user frr
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(paths.ConfSubdir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sock, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(sock, h.uid, h.gid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.Base, "zebra.conf"), nil, 0o644); err != nil { //nolint:gosec // empty daemon config
		t.Fatal(err)
	}
	if info, err := os.Lstat(h.symlink); err == nil {
		if target, _ := os.Readlink(h.symlink); info.Mode()&os.ModeSymlink == 0 || target != sock {
			t.Fatalf("%s exists and is not this test's symlink to %s: refusing to touch it", h.symlink, sock)
		}
	} else if err := os.Symlink(sock, h.symlink); err != nil {
		t.Fatal(err)
	}
	for _, d := range daemons {
		if err := h.start(d); err != nil {
			t.Fatalf("rootFRR: %v", err)
		}
	}
	t.Logf("rootFRR: %v started in the root netns, pathspace %s, base %s, pids %v", daemons, paths.Namespace, h.Base, h.pids)
	return h
}

func (h *rootFRR) pidFile(d string) string { return filepath.Join(h.Paths.SocketDir(), d+".pid") }

func (h *rootFRR) args(d string) []string {
	sock := h.Paths.SocketDir()
	args := []string{"-d", "-N", h.Paths.Namespace, "--vty_socket", sock, "-i", h.pidFile(d), "-A", "127.0.0.1", "-P", "0",
		"--log", "file:" + filepath.Join(sock, d+".log"), "--log-level", "warn"}
	if d != "mgmtd" {
		args = append(args, "-z", filepath.Join(sock, "zserv.api"))
	}
	if d == "zebra" {
		args = append(args, "-f", filepath.Join(h.Base, "zebra.conf"))
	}
	return args
}

func (h *rootFRR) start(d string) error {
	bin, ok := rootFRRBins[d]
	if !ok {
		return fmt.Errorf("unknown daemon %q", d)
	}
	logFile := filepath.Join(h.Paths.SocketDir(), d+".log")
	if err := os.WriteFile(logFile, nil, 0o640); err != nil { //nolint:gosec // test daemon log
		return err
	}
	if err := os.Chown(logFile, h.uid, h.gid); err != nil {
		return err
	}
	args := h.args(d)
	if strings.Contains(strings.Join(args, " "), "/etc/frr") {
		return errors.New("argv names /etc/frr")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil { //nolint:gosec // fixed FRR binary + slot paths
		return fmt.Errorf("start %s: %w: %s", d, err, out)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if pid, err := h.readPID(d); err == nil && h.ours(d, pid) {
			if _, err := os.Stat(filepath.Join(h.Paths.SocketDir(), d+".vty")); err == nil {
				h.pids[d] = pid
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("start %s: no live pid/vty socket after 30 s (see %s)", d, logFile)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *rootFRR) readPID(d string) (int, error) {
	b, err := os.ReadFile(h.pidFile(d))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// ours: /proc/<pid>/exe is the daemon and its argv carries our pidfile (never signal anything else).
func (h *rootFRR) ours(d string, pid int) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil {
		return false
	}
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || exe != rootFRRBins[d] {
		return false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return err == nil && strings.Contains(string(b), "\x00-i\x00"+h.pidFile(d)+"\x00")
}

// PIDs returns the current PID of each daemon (from its pidfile).
func (h *rootFRR) PIDs() map[string]int {
	out := map[string]int{}
	for _, d := range h.daemons {
		if pid, err := h.readPID(d); err == nil {
			out[d] = pid
		}
	}
	return out
}

// Renderer is an FRR renderer bound to this instance (for show commands).
func (h *rootFRR) Renderer() *frr.Renderer {
	return frr.New(h.Runner, frr.WithPaths(h.Paths), frr.WithInterfaceMapper(frr.IdentityMapper))
}

// Stop terminates the daemons by the PIDs this test spawned (SIGTERM, SIGKILL after 10 s), removes the symlink and the
// base directory and releases the slot lock. Idempotent.
func (h *rootFRR) Stop() error {
	var errs []error
	for i := len(h.daemons) - 1; i >= 0; i-- {
		d := h.daemons[i]
		pid, err := h.readPID(d)
		if err != nil || !h.ours(d, pid) {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
		deadline := time.Now().Add(10 * time.Second)
		for h.ours(d, pid) {
			if time.Now().After(deadline) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				time.Sleep(200 * time.Millisecond)
				if h.ours(d, pid) {
					errs = append(errs, fmt.Errorf("%s (pid %d) still running after SIGKILL", d, pid))
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if target, err := os.Readlink(h.symlink); err == nil && target == h.Paths.SocketDir() {
		if err := os.Remove(h.symlink); err != nil {
			errs = append(errs, err)
		}
	}
	if h.Base != "" {
		if err := os.RemoveAll(h.Base); err != nil {
			errs = append(errs, err)
		}
	}
	if h.lock != nil {
		_ = h.lock.Close()
		h.lock = nil
	}
	return errors.Join(errs...)
}

// ospfPeerDoc is a peer's FRR document: OSPF area 0 on its rig veth, 50 blackhole statics redistributed (or none).
func ospfPeerDoc(t *testing.T, slot, n int, itf string, announce bool) *structpb.Struct {
	t.Helper()
	o := map[string]any{"routerId": fmt.Sprintf("10.%d.%d.2", slot, n),
		"areas":      map[string]any{"0": map[string]any{}},
		"interfaces": map[string]any{itf: map[string]any{"area": "0", "networkType": "point-to-point", "helloIntervalSec": 1, "deadIntervalSec": 4}}}
	routing := map[string]any{"ospf": o}
	if announce {
		var statics []any
		for k := 0; k < 50; k++ {
			statics = append(statics, map[string]any{"prefix": fmt.Sprintf("10.%d.%d.0/24", slot, 64*n+k), "blackhole": true, "frr": true})
		}
		o["redistribute"] = map[string]any{"static": map[string]any{}}
		routing["static"] = statics
	}
	s, err := structpb.NewStruct(map[string]any{"routing": routing})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type ospfEnv struct {
	*p12Env
	rootFRR *rootFRR // NGFW_OSPF_FIB=root
}

// doc is the configuration document the agent gets: both host-interfaces with a linux-cp pair, OSPF area 0 on both.
func (e *ospfEnv) doc(withOSPF bool) *ngfwv1.DesiredState {
	e.t.Helper()
	n, p := e.slot, e.prefix
	lcp := func(host string) map[string]any {
		if e.fib { // root netns: the lcp default netns of the shared VPP is unset, linux_nl hears root
			return map[string]any{"hostIfName": host, "hostIfType": "tap"}
		}
		return map[string]any{"hostIfName": host, "hostIfType": "tap", "netns": e.frrNS}
	}
	d := map[string]any{"interfaces": map[string]any{
		"host-" + p + "l0": map[string]any{"enabled": true, "ipv4": []any{fmt.Sprintf("10.%d.1.1/24", n)}, "lcp": lcp(p + "-l0")},
		"host-" + p + "w0": map[string]any{"enabled": true, "ipv4": []any{fmt.Sprintf("10.%d.2.1/24", n)}, "lcp": lcp(p + "-w0")},
	}}
	if withOSPF {
		itf := func() map[string]any {
			return map[string]any{"area": "0", "networkType": "point-to-point", "helloIntervalSec": 1, "deadIntervalSec": 4}
		}
		d["routing"] = map[string]any{"ospf": map[string]any{
			"routerId":   fmt.Sprintf("10.%d.1.1", n),
			"areas":      map[string]any{"0": map[string]any{}},
			"interfaces": map[string]any{"host-" + p + "l0": itf(), "host-" + p + "w0": itf()},
		}}
	}
	raw, err := structpb.NewStruct(d)
	if err != nil {
		e.t.Fatal(err)
	}
	js, _ := protojson.Marshal(raw)
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal(js, ds); err != nil {
		e.t.Fatal(err)
	}
	return ds
}

// neighbors reads the ospfNeighbors reader through the agent's RoutingState RPC.
func (e *ospfEnv) neighbors() []ospf.Neighbor {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := e.c.RoutingState(ctx, &ngfwv1.RoutingStateRequest{Readers: []string{ospf.NeighborsReader}})
	if err != nil {
		e.t.Fatalf("RoutingState: %v", err)
	}
	ns, err := ospf.ParseNeighbors(json.RawMessage(st.GetReaders()[ospf.NeighborsReader]))
	if err != nil {
		e.t.Fatalf("parse %s: %v", ospf.NeighborsReader, err)
	}
	return ns
}

func (e *ospfEnv) waitFull(within time.Duration) {
	e.t.Helper()
	start := time.Now()
	var last string
	for {
		ns := e.neighbors()
		full := 0
		for _, n := range ns {
			if strings.HasPrefix(n.State, "Full") {
				full++
			}
		}
		last = fmt.Sprintf("%+v", ns)
		if full == 2 {
			e.t.Logf("both adjacencies Full after %v: %s", time.Since(start).Round(100*time.Millisecond), last)
			return
		}
		if time.Since(start) > within {
			e.diag()
			e.t.Fatalf("adjacencies not Full within %v: %s", within, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// diag dumps what an adjacency needs: both sides' OSPF interface and neighbour views, the taps, VPP's mfib entries of
// the pairs (224.0.0.0/24 punt to the tap) and interface counters, and 3 s of OSPF packets on the NGFW tap and a peer veth.
func (e *ospfEnv) diag() {
	e.t.Helper()
	show := func(name string, r *frr.Renderer, cmd string) {
		out, err := r.Show(context.Background(), frr.ShowCommand(cmd))
		e.t.Logf("[diag] %s: %q: %v\n%s", name, cmd, err, truncateLines(string(out), 60))
	}
	var vr *frr.Renderer
	if e.rootFRR != nil {
		vr = e.rootFRR.Renderer()
	} else {
		vr = e.ngfwFRR.Renderer()
	}
	show("ngfw", vr, "show ip ospf interface")
	show("ngfw", vr, "show ip ospf neighbor")
	show("ngfw", vr, "show interface brief")
	for i, p := range e.peerFRR {
		show(fmt.Sprintf("peer%d", i+1), p.Renderer(), "show ip ospf interface")
		show(fmt.Sprintf("peer%d", i+1), p.Renderer(), "show ip ospf neighbor")
	}
	nsArgs := func(ns string, args ...string) []string {
		if ns == "" {
			return args
		}
		return append([]string{"netns", "exec", ns}, args...)
	}
	ngfwNS := ""
	if !e.fib {
		ngfwNS = e.frrNS
	}
	for _, c := range [][]string{
		{"vppctl", "show", "ip", "mfib", "224.0.0.0/24"},
		{"vppctl", "show", "interface", "host-" + e.prefix + "l0", "host-" + e.prefix + "w0", e.prefix + "-l0", e.prefix + "-w0"},
		{"vppctl", "show", "lcp"},
		linkCmd(ngfwNS, e.prefix+"-l0"),
	} {
		out, err := e.cmd(c[0], c[1:]...)
		e.t.Logf("[diag] %s: %v\n%s", strings.Join(c, " "), err, truncateLines(out, 40))
	}
	for _, cap := range []struct{ ns, itf string }{{ngfwNS, e.prefix + "-l0"}, {"ns-" + e.prefix + "-lan", e.prefix + "l1"}} {
		args := nsArgs(cap.ns, "timeout", "4", "tcpdump", "-nn", "-c", "4", "-i", cap.itf, "proto", "ospf")
		var out string
		var err error
		if cap.ns == "" {
			out, err = e.cmd(args[0], args[1:]...)
		} else {
			out, err = e.cmd("ip", args...)
		}
		e.t.Logf("[diag] tcpdump ospf on %s/%s: %v\n%s", cap.ns, cap.itf, err, truncateLines(out, 8))
	}
}

// waitOSPF waits until FRR has want OSPF routes (and, with the FIB proof, VPP has them too).
func (e *ospfEnv) waitOSPF(what string, want int, within time.Duration) time.Duration {
	e.t.Helper()
	start := time.Now()
	var last string
	for {
		st := e.state()
		rib := int(st.GetRibCounts()["ipv4/default/ospf"])
		vppN := -1
		if e.fib {
			vppN = e.vppFRRRoutes()
		}
		last = fmt.Sprintf("FRR RIB ospf=%d, VPP lcp-rt-dynamic=%d", rib, vppN)
		if rib == want && (!e.fib || vppN == want) {
			d := time.Since(start)
			e.t.Logf("%s: %s after %v", what, last, d.Round(100*time.Millisecond))
			return d
		}
		if time.Since(start) > within {
			e.t.Fatalf("%s: not reached within %v: %s", what, within, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// runningConfig is the NGFW-side FRR's running config.
func (e *ospfEnv) runningConfig() string {
	e.t.Helper()
	var r *frr.Renderer
	if e.rootFRR != nil {
		r = e.rootFRR.Renderer()
	} else {
		r = e.ngfwFRR.Renderer()
	}
	out, err := r.Show(context.Background(), frr.ShowRunningConfig)
	if err != nil {
		e.t.Fatalf("show running-config: %v", err)
	}
	return string(out)
}

func (e *ospfEnv) evidence(when string) {
	e.t.Helper()
	lcp, err := e.cmd("vppctl", "show", "lcp")
	if err != nil {
		e.t.Fatalf("cannot verify LCP preconditions: %v", err)
	}
	e.t.Logf("[%s] vppctl show lcp:\n%s", when, lcp)
	ns := "root"
	addrs, _ := e.cmd("ip", "-br", "addr", "show")
	if !e.fib {
		ns = e.frrNS
		addrs, _ = e.cmd("ip", "-n", e.frrNS, "-br", "addr", "show")
	}
	e.t.Logf("[%s] ip -br addr show (%s, slot lines):\n%s", when, ns, ospfGrepLines(addrs, e.prefix))
	var r *frr.Renderer
	if e.rootFRR != nil {
		r = e.rootFRR.Renderer()
	} else {
		r = e.ngfwFRR.Renderer()
	}
	if out, err := r.Show(context.Background(), frr.ShowCommand("show ip ospf neighbor")); err == nil {
		e.t.Logf("[%s] vtysh -N %s \"show ip ospf neighbor\":\n%s", when, r.Paths().Namespace, out)
	}
	if e.fib {
		kr, _ := e.cmd("ip", "-4", "route", "show", "proto", "ospf")
		e.t.Logf("[%s] ip -4 route show proto ospf (root netns): %d routes\n%s", when, len(strings.Fields(strings.ReplaceAll(strings.TrimSpace(kr), " ", "_"))), truncateLines(kr, 6))
		for _, pfx := range []string{fmt.Sprintf("10.%d.64.0/24", e.slot), fmt.Sprintf("10.%d.128.0/24", e.slot)} {
			fib, _ := e.cmd("vppctl", "show", "ip", "fib", pfx)
			e.t.Logf("[%s] vppctl show ip fib %s:\n%s", when, pfx, fib)
		}
	}
}

// linkCmd is `ip -d link show <itf>`, inside ns when ns is not the root namespace.
func linkCmd(ns, itf string) []string {
	if ns == "" {
		return []string{"ip", "-d", "link", "show", itf}
	}
	return []string{"ip", "-n", ns, "-d", "link", "show", itf}
}

func ospfGrepLines(s, needle string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, needle) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func truncateLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… (%d more)", len(lines)-n)
}

// checkRootPreconditions fails closed before anything of the root mode starts.
func (e *ospfEnv) checkRootPreconditions() {
	e.t.Helper()
	if vppSocket() != "/run/vpp/api.sock" {
		e.t.Logf("%s=root on %s (not the shared VPP)", EnvOSPFFIB, vppSocket())
	}
	lcp, err := e.cmd("vppctl", "show", "lcp")
	if err != nil {
		e.t.Fatalf("cannot verify LCP preconditions: %v", err)
	}
	if strings.Contains(lcp, "itf-pair") {
		e.t.Fatalf("%s=root: LCP pairs already exist on this VPP (linux_nl's socket is bound to their netns):\n%s", EnvOSPFFIB, lcp)
	}
	if !strings.Contains(lcp, "lcp default netns '<unset>'") {
		e.t.Fatalf("%s=root: the lcp default netns is not unset:\n%s", EnvOSPFFIB, lcp)
	}
	kr, err := e.cmd("ip", "-4", "route", "show")
	if err != nil {
		e.t.Fatalf("cannot verify root kernel routes: %v", err)
	}
	for _, l := range strings.Split(kr, "\n") {
		for _, p := range []string{"proto zebra", "proto bgp", "proto ospf", "proto isis", "proto rip", "proto 196", "proto 11 "} {
			if strings.Contains(l, p) {
				e.t.Fatalf("%s=root: an FRR-protocol route is in the root kernel table (zebra would sweep it): %s", EnvOSPFFIB, l)
			}
		}
	}
	out, serviceErr := e.cmd("systemctl", "is-active", "frr")
	var serviceExit *exec.ExitError
	if !errors.As(serviceErr, &serviceExit) || serviceExit.ExitCode() != 3 || (strings.TrimSpace(out) != "inactive" && strings.TrimSpace(out) != "failed") {
		e.t.Fatalf("%s=root: cannot verify system frr is inactive: %q (%v)", EnvOSPFFIB, out, serviceErr)
	}
	e.t.Logf("%s=root preconditions: no LCP pair, default netns unset, no FRR-protocol kernel route, system frr inactive; show lcp:\n%s", EnvOSPFFIB, lcp)
}

func TestOSPFTopologyOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv(EnvOSPFTopology) != "1" {
		t.Skipf("OSPF topology test: set %s=1 (test/topology/ospf/run.sh) — it takes the slot's rig over and runs 3 FRR instances", EnvOSPFTopology)
	}
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	e := &ospfEnv{p12Env: &p12Env{t: t, prefix: prefix, slot: slot, repo: repoRootP12(t), frrNS: "ns-" + prefix + "-frr", fib: os.Getenv(EnvOSPFFIB) == "root"}}
	if e.fib {
		// the globals window (D-167): exclusive globals lock inside the shared lab lock, released last
		f, err := os.OpenFile(globalsLockFile, os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // shared lock file
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("waiting for the exclusive globals lock %s …", globalsLockFile)
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			t.Fatal(err)
		}
		t.Logf("globals window open at %s", time.Now().Format(time.RFC3339))
		t.Cleanup(func() {
			t.Logf("globals window closed at %s", time.Now().Format(time.RFC3339))
			_ = f.Close()
		})
	}
	restarts0 := nRestartsP12(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestartsP12(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	e.raw = raw
	if _, err := lcpapi.NewServiceClient(raw).LcpDefaultNsGet(context.Background(), &lcpapi.LcpDefaultNsGet{}); err != nil {
		t.Skipf("linux_cp is not loaded on this VPP: %v", err)
	}
	if e.fib {
		e.checkRootPreconditions()
	}

	// V19 preflight (D-095) before any packet crosses the rig
	pre, err := e.cmd("go", "-C", filepath.Join(e.repo, "apps", "agent"), "run", "./cmd/ngfw-vpp-preflight")
	t.Logf("ngfw-vpp-preflight: %v\n%s", err, pre)
	if err != nil {
		t.Fatal("V19 preflight failed: no packet may cross the rig")
	}
	lab := filepath.Join(e.repo, "tools", "lab")
	t.Log(e.must(lab, "rig", "up", prefix))
	t.Cleanup(func() {
		out, err := e.cmd(lab, "rig", "down", prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	e.peers(false)
	e.handRigToAgent()

	// FRR: the NGFW side (root netns or ns-<p>-frr), the peers in the rig namespaces
	daemons := []string{"mgmtd", "zebra", "staticd", "ospfd"}
	if e.fib {
		e.rootFRR = startRootFRR(t, prefix, daemons)
	} else {
		e.ngfwFRR = frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: daemons})
	}
	e.peerFRR[0] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p1", NetNS: "ns-" + prefix + "-lan", Daemons: daemons})
	e.peerFRR[1] = frrtest.Start(t, frrtest.Options{Prefix: prefix, Instance: "p2", NetNS: "ns-" + prefix + "-wan", Daemons: daemons})
	lanIf, wanIf := prefix+"l1", prefix+"w1"
	applyFRR(t, e.peerFRR[0], ospfPeerDoc(t, slot, 1, lanIf, true))
	applyFRR(t, e.peerFRR[1], ospfPeerDoc(t, slot, 2, wanIf, true))

	// the agent (FRR = the slot instance; table range of the slot)
	t.Setenv(subsystems.EnvFRRPathspace, prefix)
	t.Setenv(subsystems.EnvTableBase, strconv.Itoa(1000*slot))
	e.cfg = hostConfig(t, prefix)
	ids, err := subsystems.ResolveIDScope()
	if err != nil {
		t.Fatal(err)
	}
	e.cfg.IDs = ids
	start := func() {
		a, err := Start(context.Background(), e.cfg, "ospf-it", nil)
		if err != nil {
			t.Fatal(err)
		}
		e.a = a
		e.c = dialAgent(t, e.cfg.Socket)
		waitReady(t, e.c)
	}
	start()
	t.Cleanup(func() {
		// peers down first; then everything of this owner is removed through the agent (routes withdrawn before the
		// pairs go, D-101/V24 for the af_packet interfaces); then the agent stops
		e.peers(false)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cc, err := grpc.NewClient("unix://"+e.cfg.Socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			c := ngfwv1.NewDataplaneClient(cc)
			resp, aerr := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: prefix + "-ospf-cleanup", Subsystems: []string{"interfaces", "routing"}})
			t.Logf("cleanup apply: %v %s", aerr, protojson.Format(resp.GetSummary()))
			if e.fib {
				if r, err := c.ListRoutes(ctx, &ngfwv1.ListRoutesRequest{Family: "ipv4", Prefix: fmt.Sprintf("10.%d.0.0/16", slot), Source: "lcp-rt-dynamic", Limit: 1}); err == nil {
					t.Logf("cleanup: %d lcp-rt-dynamic routes of 10.%d/16 left in VPP table 0", r.GetTotal(), slot)
					if r.GetTotal() != 0 {
						t.Errorf("cleanup left %d lcp-rt-dynamic routes of this slot in the shared table 0", r.GetTotal())
					}
				}
			}
			_ = cc.Close()
		}
		e.a.Stop()
	})

	// ---- 1. commit: pairs + FRR config; the peers come up after the preflight
	if e.fib {
		if n := e.vppFRRRoutes(); n != 0 {
			t.Fatalf("%d linux-nl routes of 10.%d/16 already in VPP table 0 before the test", n, slot)
		}
	} else {
		t.Logf("VPP FIB checks not requested (%s=root runs the NGFW-side FRR in the root netns): FRR's RIB is checked", EnvOSPFFIB)
	}
	full := e.doc(true)
	e.apply("commit", full)
	e.peers(true)
	e.waitFull(90 * time.Second)
	e.waitOSPF("100 routes after commit", 100, 60*time.Second)
	e.evidence("after commit")

	// Retrieve == desired for routing.ospf and the pairs
	got, err := e.c.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.GetDesiredState().GetRouting().GetOspf(), full.GetRouting().GetOspf()) {
		t.Errorf("Retrieve routing.ospf differs:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetRouting().GetOspf()), protojson.Format(full.GetRouting().GetOspf()))
	} else {
		t.Logf("Retrieve routing.ospf == desired: %s", protojson.Format(got.GetDesiredState().GetRouting().GetOspf()))
	}
	for name, itf := range full.GetInterfaces() {
		if !proto.Equal(got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp()) {
			t.Errorf("Retrieve %s.lcp = %v, want %v", name, got.GetDesiredState().GetInterfaces()[name].GetLcp(), itf.GetLcp())
		}
	}
	idem := e.apply("idempotent", full)
	if len(idem.GetResults()) != 0 {
		t.Errorf("second apply changed %v", idem.GetResults())
	}
	rc := e.runningConfig()
	for _, want := range []string{"router ospf", " ospf router-id " + fmt.Sprintf("10.%d.1.1", slot), "interface " + prefix + "-l0", " ip ospf area 0", " ip ospf network point-to-point"} {
		if !strings.Contains(rc, want) {
			t.Errorf("running config lacks %q:\n%s", want, rc)
		}
	}
	if e.fib && strings.Contains(rc, "ens192") {
		t.Fatalf("running config mentions the management NIC:\n%s", rc)
	}

	// ---- 2. peer 1 withdraws → its 50 prefixes are gone within 10 s; announces again → 100
	applyFRR(t, e.peerFRR[0], ospfPeerDoc(t, slot, 1, lanIf, false))
	if d := e.waitOSPF("peer 1 withdrew", 50, 10*time.Second); d > 10*time.Second {
		t.Errorf("withdrawal took %v", d)
	}
	applyFRR(t, e.peerFRR[0], ospfPeerDoc(t, slot, 1, lanIf, true))
	e.waitOSPF("peer 1 announces again", 100, 30*time.Second)

	// ---- 3. agent restart with both pairs deleted behind its back → recreated, adjacencies and routes back, no API call
	e.a.Stop()
	names, err := dumpNames(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"host-" + prefix + "l0", "host-" + prefix + "w0"} {
		if _, err := lcpapi.NewServiceClient(raw).LcpItfPairAddDelV3(context.Background(), &lcpapi.LcpItfPairAddDelV3{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(names[n])}); err != nil {
			t.Fatalf("simulated loss: delete pair of %s: %v", n, err)
		}
		t.Logf("simulated loss: lcp_itf_pair_add_del_v3 del %s (sw_if_index %d) with the agent stopped", n, names[n])
	}
	restartAt := time.Now()
	start()
	e.waitFull(120 * time.Second)
	e.waitOSPF("after agent restart + pair loss", 100, 60*time.Second)
	recovered := time.Since(restartAt)
	t.Logf("recovered %v after the agent restart (target ≤ 30 s)", recovered.Round(100*time.Millisecond))
	if recovered > 30*time.Second {
		t.Errorf("recovery took %v, more than 30 s", recovered.Round(time.Second))
	}
	if e.fib {
		// a deleted pair flushes no route (lcp_router.c): only a withdrawal that reaches VPP proves linux-nl hears the
		// recreated pairs
		applyFRR(t, e.peerFRR[0], ospfPeerDoc(t, slot, 1, lanIf, false))
		e.waitOSPF("peer 1 withdrew after the restart", 50, 10*time.Second)
		applyFRR(t, e.peerFRR[0], ospfPeerDoc(t, slot, 1, lanIf, true))
		e.waitOSPF("peer 1 announces again after the restart", 100, 30*time.Second)
	}
	e.evidence("after restart")

	// ---- 4. rollback of routing.ospf → no `router ospf`, no OSPF route (FRR, and VPP with the FIB proof)
	e.apply("rollback", e.doc(false))
	e.waitOSPF("after rollback", 0, 30*time.Second)
	got, err = e.c.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetDesiredState().GetRouting().GetOspf() != nil {
		t.Errorf("Retrieve still holds routing.ospf after the rollback: %s", protojson.Format(got.GetDesiredState().GetRouting()))
	}
	if ns := e.neighbors(); len(ns) != 0 {
		t.Errorf("neighbours left after rollback: %+v", ns)
	}
	rc = e.runningConfig()
	for _, gone := range []string{"router ospf", "ip ospf"} {
		if strings.Contains(rc, gone) {
			t.Errorf("FRR still holds %q after rollback:\n%s", gone, rc)
		}
	}
	if !strings.Contains(rc, "interface "+prefix+"-l0") {
		t.Errorf("the pair's interface block (addresses) vanished with the OSPF rollback:\n%s", rc)
	}
	e.evidence("after rollback")
}
