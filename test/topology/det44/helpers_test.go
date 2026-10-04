package det44topo

// Harness of F-det44-map-dslite-cnat-host's topology test, copied (agent-only, no API stack: slot 16 has no Valkey
// database) from test/topology/nat44-ei-64-66-nptv6: the slot, the locks, the af_packet rig (tools/lab rig), the real
// ngfw-agent binary driven over gRPC, the V19 guard, the simulated loss through the binary API, vppctl for evidence
// (never `trace`, D-128) and the small traffic programs run inside the rig's namespaces. Test code only; every command
// is fixed (no user input); every process is stopped by PID.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"ngfw/agent/binapi/acl"
	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

const (
	labLock     = "/run/lock/ngfw-lab.lock"     // shared for the run (D-094)
	globalsLock = "/run/lock/ngfw-globals.lock" // exclusive around the global steps (D-082, D-167)
	apiSocket   = "/run/vpp/api.sock"
	noIndex     = ^uint32(0)
)

// ---- slot ---------------------------------------------------------------------------------------

type slot struct {
	prefix      string
	num         int
	metricsPort string
	tableBase   string
	runDir      string // /run/ngfw-test/<prefix>
	socket      string
	repo        string
	lab         string
}

func slotFromEnv(t *testing.T) slot {
	t.Helper()
	p := os.Getenv("NGFW_TEST_PREFIX")
	m := regexp.MustCompile(`^w([0-9]{1,2})$`).FindStringSubmatch(p)
	if m == nil {
		t.Fatalf("NGFW_TEST_PREFIX=%q: this test needs a slot prefix w<N> (eval \"$(tools/lab env <N>)\")", p)
	}
	n, _ := strconv.Atoi(m[1])
	env := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	s := slot{
		prefix:      p,
		num:         n,
		metricsPort: env("NGFW_METRICS_PORT", strconv.Itoa(9100+10*n+1)),
		tableBase:   env("NGFW_VPP_TABLE_BASE", strconv.Itoa(n*1000)),
		runDir:      "/run/ngfw-test/" + p,
		repo:        repoRoot(t),
	}
	s.socket = env("NGFW_AGENT_SOCKET", s.runDir+"/agent.sock")
	s.lab = filepath.Join(s.repo, "tools", "lab")
	return s
}

// hex6 is the slot's IPv6 block label: fd00:<slot in hex>::/32 (slot 16 → fd00:10::/32).
func (s slot) hex6() string { return strconv.FormatInt(int64(s.num), 16) }

// addr is an IPv4 address of the slot block: 10.<N>.<a>.<b>.
func (s slot) addr(a, b int) string {
	return "10." + strconv.Itoa(s.num) + "." + strconv.Itoa(a) + "." + strconv.Itoa(b)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "lab")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (containing tools/lab) not found")
		}
		dir = parent
	}
}

// ---- locks --------------------------------------------------------------------------------------

const (
	syscallLockSH = syscall.LOCK_SH
	syscallLockEX = syscall.LOCK_EX
)

func flock(t *testing.T, path string, how int) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // fixed lock path
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		t.Fatalf("flock %s: %v", path, err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
	return f
}

// ---- processes ----------------------------------------------------------------------------------

func run(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := run(t, name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

// nRestarts is `systemctl show vpp -p NRestarts` (pasted verbatim into the log, D-175).
func nRestarts(t *testing.T, label string) int {
	t.Helper()
	out := strings.TrimSpace(mustRun(t, "systemctl", "show", "vpp", "-p", "NRestarts"))
	t.Logf("%s %s: systemctl show vpp -p NRestarts → %s", time.Now().Format("15:04:05"), label, out)
	n, err := strconv.Atoi(strings.TrimPrefix(out, "NRestarts="))
	if err != nil {
		t.Fatalf("NRestarts: %q", out)
	}
	return n
}

// proc is a child process started by the test, stopped by PID.
type proc struct {
	name string
	cmd  *exec.Cmd
	log  string
	done chan struct{}
}

func start(t *testing.T, name, logPath string, env []string, bin string, args ...string) *proc {
	t.Helper()
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		t.Fatalf("start %s: %v", name, err)
	}
	p := &proc{name: name, cmd: cmd, log: logPath, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = f.Close(); close(p.done) }()
	t.Logf("started %s pid %d (log %s)", name, cmd.Process.Pid, logPath)
	return p
}

// stop sends SIGTERM to the PID we started and waits (SIGKILL after 10 s).
func (p *proc) stop(t *testing.T) {
	if p == nil || p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	t.Logf("stopped %s pid %d", p.name, p.cmd.Process.Pid)
}

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *proc) output() string {
	raw, _ := os.ReadFile(p.log) //nolint:gosec // our own log
	return string(raw)
}

func waitFor(timeout time.Duration, f func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return f()
}

// mkdirShared creates dir (0755, traversable for other test daemons of the slot).
func mkdirShared(dir string) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", dir)
		}
		return nil
	}
	if err := mkdirShared(filepath.Dir(dir)); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	return os.Chmod(dir, 0o755) //nolint:gosec // G302: a shared, traversable run directory (no secrets in it)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- the agent (real binary, owner = slot prefix, never the globals owner) -------------------------

type agent struct {
	s        slot
	bin      string
	stateDir string
	env      []string
	p        *proc
	log      string
	work     string
	c        ngfwv1.DataplaneClient
}

func newAgent(t *testing.T, s slot) *agent {
	t.Helper()
	bin := os.Getenv("NGFW_NAT_AGENT_BIN")
	if bin == "" { // run.sh passes a prebuilt one; otherwise build from this tree
		bin = filepath.Join(t.TempDir(), "ngfw-agent")
		out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/ngfw-agent")
		if err != nil {
			t.Fatalf("go build ngfw-agent: %v\n%s", err, out)
		}
	}
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(s.runDir, "det44")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	a := &agent{s: s, bin: bin, stateDir: filepath.Join(work, "agent-state"), log: filepath.Join(work, "agent.log"), work: work}
	a.env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"NGFW_AGENT_SOCKET=" + s.socket, "NGFW_OWNER=" + s.prefix, "NGFW_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"NGFW_AGENT_STATE_DIR=" + a.stateDir, "NGFW_METRICS_PORT=" + s.metricsPort, "NGFW_SOCKET_GROUP=root", "NGFW_LOG_LEVEL=info",
		"NGFW_VPP_TABLE_BASE=" + s.tableBase} // shared-host-rules §12: the slot's id range, passed through
	a.startAgent(t)
	t.Cleanup(func() { a.p.stop(t) })
	cc, err := grpc.NewClient("unix://"+s.socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	a.c = ngfwv1.NewDataplaneClient(cc)
	return a
}

func (a *agent) startAgent(t *testing.T) {
	t.Helper()
	a.p = start(t, "ngfw-agent", a.log, a.env, a.bin)
	if !waitFor(30*time.Second, func() bool {
		_, err := os.Stat(a.s.socket)
		return err == nil || a.p.exited()
	}) || a.p.exited() {
		t.Fatalf("ngfw-agent did not come up:\n%s", a.p.output())
	}
}

func (a *agent) restart(t *testing.T) time.Time {
	t.Helper()
	a.p.stop(t)
	a.startAgent(t)
	return time.Now()
}

func docOf(t *testing.T, js string) *ngfwv1.DesiredState {
	t.Helper()
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("desired state json: %v\n%s", err, js)
	}
	return ds
}

// apply sends one transaction (the whole document with every domain the test manages, D-041) and returns the
// response; the caller checks the status.
func (a *agent) apply(t *testing.T, txn, js string) *ngfwv1.ApplyResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := a.c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: txn, DesiredState: docOf(t, js), Owner: a.s.prefix})
	if err != nil {
		t.Fatalf("Apply %s: %v", txn, err)
	}
	return resp
}

// mustApplyDomains is mustApply with the domains named in ApplyRequest.subsystems: a named domain is authoritative even
// when it is empty — with an empty "interfaces" map and no subsystems the agent skips the domain (D-041), so this is
// the only way to delete every interface of the slot through the agent.
func (a *agent) mustApplyDomains(t *testing.T, txn, js string, domains ...string) *ngfwv1.ApplyResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := a.c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: txn, DesiredState: docOf(t, js), Owner: a.s.prefix, Subsystems: domains})
	if err != nil {
		t.Fatalf("Apply %s: %v", txn, err)
	}
	if resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("Apply %s (subsystems %v) → %s: %s", txn, domains, resp.GetStatus(), protojson.Format(resp))
	}
	t.Logf("Apply %s (subsystems %v) → %s results=%d summary=%s", txn, domains, resp.GetStatus(), len(resp.GetResults()), trunc(protojson.Format(resp), 1500))
	return resp
}

func (a *agent) mustApply(t *testing.T, txn, js string) *ngfwv1.ApplyResponse {
	t.Helper()
	resp := a.apply(t, txn, js)
	if resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("Apply %s → %s: %s", txn, resp.GetStatus(), protojson.Format(resp))
	}
	t.Logf("Apply %s → %s results=%d summary=%s", txn, resp.GetStatus(), len(resp.GetResults()), trunc(protojson.Format(resp), 1200))
	return resp
}

func (a *agent) retrieveNat(t *testing.T) (*ngfwv1.NatConfig, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := a.c.Retrieve(ctx, &ngfwv1.RetrieveRequest{Subsystems: []string{"nat"}, Owner: a.s.prefix})
	if err != nil {
		return nil, err
	}
	return r.GetDesiredState().GetNat(), nil
}

func natJSON(t *testing.T, js string) *ngfwv1.NatConfig {
	t.Helper()
	n := &ngfwv1.NatConfig{}
	if err := protojson.Unmarshal([]byte(js), n); err != nil {
		t.Fatalf("nat json: %v\n%s", err, js)
	}
	return n
}

// ---- VPP (binary API + vppctl, evidence and the simulated loss only; never through the agent) -------

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func drain[T any](recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, d)
	}
}

type vppIf struct {
	idx  uint32
	name string
	tag  string
}

func dumpIfs(t *testing.T, conn vppapi.Connection) map[string]vppIf {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]vppIf{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n := strings.TrimRight(d.InterfaceName, "\x00")
		out[n] = vppIf{idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00")}
	}
	return out
}

// waitIfs waits until VPP has all names and returns their sw_if_index.
func waitIfs(t *testing.T, vc vppapi.Connection, names ...string) map[string]uint32 {
	t.Helper()
	out := map[string]uint32{}
	if !waitFor(30*time.Second, func() bool {
		ifs := dumpIfs(t, vc)
		for _, n := range names {
			i, ok := ifs[n]
			if !ok {
				return false
			}
			out[n] = i.idx
		}
		return true
	}) {
		t.Fatalf("VPP does not have %v", names)
	}
	return out
}

// v19Guard (D-095): before any packet crosses the rig, prove that no classify / ACL / SPD binding sits on the rig
// interfaces' sw_if_index, and reset the write-only ip/l2 classify bindings of our own interfaces to none (D-185).
func v19Guard(t *testing.T, conn vppapi.Connection, ifs map[string]uint32) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	var evidence []string
	cls := classify.NewServiceClient(conn)
	for name, idx := range ifs {
		r, err := cls.ClassifyTableByInterface(ctx, &classify.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("V19: classify_table_by_interface %s: %v", name, err)
		}
		if r.L2TableID != noIndex || r.IP4TableID != noIndex || r.IP6TableID != noIndex {
			t.Fatalf("V19: %s (sw_if_index %d) carries an input classify binding l2=%d ip4=%d ip6=%d — refusing to send packets", name, idx, r.L2TableID, r.IP4TableID, r.IP6TableID)
		}
		evidence = append(evidence, fmt.Sprintf("classify_table_by_interface %s sw_if_index=%d l2=~0 ip4=~0 ip6=~0", name, idx))
		st, err := acl.NewServiceClient(conn).ACLInterfaceListDump(ctx, &acl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("V19: acl_interface_list_dump %s: %v", name, err)
		}
		n := 0
		for {
			d, err := st.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("V19: acl_interface_list_dump %s: %v", name, err)
			}
			if uint32(d.SwIfIndex) == idx {
				n += len(d.Acls)
			}
		}
		if n != 0 {
			t.Fatalf("V19: %s (sw_if_index %d) has %d ACL bindings — refusing to send packets", name, idx, n)
		}
		evidence = append(evidence, fmt.Sprintf("acl_interface_list_dump %s: 0 ACLs", name))
	}
	sp, err := ipsec.NewServiceClient(conn).IpsecSpdInterfaceDump(ctx, &ipsec.IpsecSpdInterfaceDump{})
	if err != nil {
		evidence = append(evidence, "ipsec_spd_interface_dump: "+err.Error()+" (ipsec not loaded)")
	} else {
		for {
			d, err := sp.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("V19: ipsec_spd_interface_dump: %v", err)
			}
			for name, idx := range ifs {
				if uint32(d.SwIfIndex) == idx {
					t.Fatalf("V19: %s (sw_if_index %d) is bound to SPD %d — refusing to send packets", name, idx, d.SpdIndex)
				}
			}
		}
		evidence = append(evidence, "ipsec_spd_interface_dump: no SPD on the rig interfaces")
	}
	for name, idx := range ifs {
		for _, v6 := range []bool{false, true} {
			if _, err := cls.ClassifySetInterfaceIPTable(ctx, &classify.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: interface_types.InterfaceIndex(idx), TableIndex: noIndex}); err != nil {
				t.Fatalf("V19: classify_set_interface_ip_table %s ~0: %v", name, err)
			}
		}
		for _, in := range []bool{true, false} {
			if _, err := cls.ClassifySetInterfaceL2Tables(ctx, &classify.ClassifySetInterfaceL2Tables{SwIfIndex: interface_types.InterfaceIndex(idx), IP4TableIndex: noIndex, IP6TableIndex: noIndex, OtherTableIndex: noIndex, IsInput: in}); err != nil {
				t.Fatalf("V19: classify_set_interface_l2_tables %s ~0: %v", name, err)
			}
		}
		evidence = append(evidence, fmt.Sprintf("reset write-only ip4/ip6 classify table and l2 in/out tables of %s to ~0", name))
	}
	return evidence
}

// handToAgent removes the rig's VPP side (addresses first, then the af_packet interfaces, host veths already down —
// V24/D-101) through the binary API so the agent creates them from the configuration (P08 / F-nat44-ei pattern).
func handToAgent(t *testing.T, conn vppapi.Connection, netdevs ...string) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	ifs := dumpIfs(t, conn)
	cls := classify.NewServiceClient(conn)
	var ev []string
	for _, nd := range netdevs {
		i, ok := ifs["host-"+nd]
		if !ok {
			t.Fatalf("hand-over: host-%s is not in VPP", nd)
		}
		// D-185: reset the classify bindings BEFORE the address delete (a classify /32 survives it otherwise)
		for _, v6 := range []bool{false, true} {
			if _, err := cls.ClassifySetInterfaceIPTable(ctx, &classify.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: interface_types.InterfaceIndex(i.idx), TableIndex: noIndex}); err != nil {
				t.Fatalf("hand-over: classify_set_interface_ip_table %s ~0: %v", i.name, err)
			}
		}
		if _, err := interfaces.NewServiceClient(conn).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(i.idx), DelAll: true}); err != nil {
			t.Fatalf("hand-over: sw_interface_add_del_address del_all %s: %v", i.name, err)
		}
		ev = append(ev, fmt.Sprintf("classify reset (ip4+ip6 → ~0, D-185) + sw_interface_add_del_address sw_if_index=%d (%s) del_all=true → ok", i.idx, i.name))
		if _, err := afpapi.NewServiceClient(conn).AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: nd}); err != nil {
			t.Fatalf("hand-over: af_packet_delete %s: %v", nd, err)
		}
		ev = append(ev, fmt.Sprintf("af_packet_delete host_if_name=%s (tag %q) → ok", nd, i.tag))
	}
	return ev
}

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// grepLines keeps the lines of a vppctl output that mention any of the needles (the shared VPP holds other slots').
func grepLines(out string, needles ...string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		for _, n := range needles {
			if strings.Contains(l, n) {
				keep = append(keep, l)
				break
			}
		}
	}
	return strings.Join(keep, "\n")
}

// ---- rig ---------------------------------------------------------------------------------------

type rig struct {
	prefix       string
	lanNS, wanNS string
	lanIf, wanIf string // VPP (and logical) names: host-<p>l0 / host-<p>w0
	lanDev       string // host-side veths (af_packet netdevs)
	wanDev       string
	lanPeer      string // netns-side veths
	wanPeer      string
	lanGW, wanGW string // VPP addresses
	lanIP, wanIP string // netns addresses
	slot         int
}

func newRig(s slot) rig {
	p := s.prefix
	n := strconv.Itoa(s.num)
	return rig{
		prefix: p, slot: s.num,
		lanNS: "ns-" + p + "-lan", wanNS: "ns-" + p + "-wan",
		lanIf: "host-" + p + "l0", wanIf: "host-" + p + "w0",
		lanDev: p + "l0", wanDev: p + "w0", lanPeer: p + "l1", wanPeer: p + "w1",
		lanGW: "10." + n + ".1.1", wanGW: "10." + n + ".2.1",
		lanIP: "10." + n + ".1.2", wanIP: "10." + n + ".2.2",
	}
}

// peers sets both veth pairs down/up. Down: no packet enters VPP before the V19 guard, and (D-101 / V24) an
// af_packet interface is only deleted with its host-side veth down.
func (r rig) peers(t *testing.T, up bool) {
	t.Helper()
	st := "down"
	if up {
		st = "up"
		mustRun(t, "ip", "link", "set", r.lanDev, "up")
		mustRun(t, "ip", "link", "set", r.wanDev, "up")
	}
	mustRun(t, "ip", "-n", r.lanNS, "link", "set", r.lanPeer, st)
	mustRun(t, "ip", "-n", r.wanNS, "link", "set", r.wanPeer, st)
	if !up {
		mustRun(t, "ip", "link", "set", r.lanDev, "down")
		mustRun(t, "ip", "link", "set", r.wanDev, "down")
		return
	}
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "sysctl", "-qw", "net.ipv6.conf."+r.lanPeer+".disable_ipv6=1")
	mustRun(t, "ip", "-n", r.lanNS, "route", "replace", "default", "via", r.lanGW)
	mustRun(t, "ip", "-n", r.wanNS, "route", "replace", "default", "via", r.wanGW)
	// no TX checksum offload on the netns end: NAT'ed TCP from a veth with offload is silently dropped (D-129)
	mustRun(t, "ip", "netns", "exec", r.lanNS, "ethtool", "-K", r.lanPeer, "tx", "off")
	mustRun(t, "ip", "netns", "exec", r.wanNS, "ethtool", "-K", r.wanPeer, "tx", "off")
}

func inNS(t *testing.T, ns string, args ...string) (string, error) {
	t.Helper()
	return run(t, "ip", append([]string{"netns", "exec", ns}, args...)...)
}

// ---- traffic ------------------------------------------------------------------------------------

// holdServer accepts TCP connections, greets and holds them open (a live NAT session per client).
const holdServer = `import socket, sys
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((sys.argv[1], int(sys.argv[2])))
s.listen(64)
held = []
while True:
    c, _ = s.accept()
    try:
        c.sendall((sys.argv[1] + " ok\n").encode())
    except OSError:
        pass
    held.append(c)
`

// holdClient connects from a fixed source port, prints the greeting and holds the connection.
const holdClient = `import socket, sys, time
c = socket.socket()
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print(c.recv(64).decode().strip(), flush=True)
time.sleep(600)
`

// manyClients connects n times from consecutive source ports, prints one line per attempt (greeting or error).
const manyClients = `import socket, sys
src, base, n, dst, dport = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4], int(sys.argv[5])
ok = 0
for i in range(n):
    c = socket.socket()
    c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    c.bind((src, base + i))
    c.settimeout(3)
    try:
        c.connect((dst, dport))
        print(base + i, c.recv(64).decode().strip(), flush=True)
        ok += 1
    except OSError as e:
        print(base + i, "error", e, flush=True)
    c.close()
print("connected", ok, "of", n, flush=True)
`

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// inNSProc starts a long-running program inside a namespace and stops it by PID in Cleanup.
func inNSProc(t *testing.T, name, logDir, ns string, args ...string) *proc {
	t.Helper()
	p := start(t, name, filepath.Join(logDir, name+".log"), []string{"PATH=" + os.Getenv("PATH")}, "ip", append([]string{"netns", "exec", ns}, args...)...)
	t.Cleanup(func() { p.stop(t) })
	return p
}

type capture struct {
	p   *proc
	log string
}

func startCapture(t *testing.T, name, logDir, ns, dev string, filter ...string) *capture {
	t.Helper()
	args := append([]string{"tcpdump", "-n", "-l", "-i", dev}, filter...)
	c := &capture{log: filepath.Join(logDir, name+".log")}
	c.p = inNSProc(t, name, logDir, ns, args...)
	// wait until tcpdump says "listening on" (its capture is open) instead of a fixed 700 ms: under host load the start
	// took longer and a det44 handshake went by uncaptured (F-det44-cnat-fix, load 16)
	if !waitFor(10*time.Second, func() bool {
		raw, _ := os.ReadFile(c.log) //nolint:gosec // our own log
		return strings.Contains(string(raw), "listening on")
	}) {
		t.Logf("%s: tcpdump did not report \"listening on\" within 10 s", name)
	}
	time.Sleep(200 * time.Millisecond)
	return c
}

// lines stops the capture and returns its packet lines.
func (c *capture) lines(t *testing.T) []string {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	c.p.stop(t)
	raw, _ := os.ReadFile(c.log) //nolint:gosec // our own log
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, " IP ") || strings.Contains(l, " IP6 ") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

var tcpdumpIP = regexp.MustCompile(` IP (\d+\.\d+\.\d+\.\d+)\.(\d+) > (\d+\.\d+\.\d+\.\d+)\.(\d+):`)

// v4Tuple returns src, sport, dst, dport of a tcpdump IPv4 line ("" when it has none).
func v4Tuple(line string) (string, string, string, string) {
	m := tcpdumpIP.FindStringSubmatch(line)
	if m == nil {
		return "", "", "", ""
	}
	return m[1], m[2], m[3], m[4]
}
