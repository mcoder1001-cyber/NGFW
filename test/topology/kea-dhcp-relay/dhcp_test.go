// Package keadhcprelay is F-kea-dhcp-relay's topology test: DHCP end to end on the REAL host VPP through the af_packet
// veth/netns rig (path: af_packet, D-010), with the real vrx-agent + vrx-api of this tree on the slot's ports and
// database, and a slot-local Kea test instance (kea-dhcp4 in ns-<p>-wan, own conf/run/lib/log dirs under
// /run/vrx-test/<p>/kea, own unix control socket — never the system units, never kea-ctrl-agent, D-079).
//
//	client netns ns-<p>-lan (dhclient on <p>l1)
//	   │ veth <p>l1 ↔ <p>l0 ── VPP host-<p>l0 10.<N>.1.1/24, VRF <p>-dhcp (table <N>001)  ← dhcp.proxy (relay)
//	   │                        VPP host-<p>w0 10.<N>.2.1/24, VRF <p>-dhcp               → relay source
//	   │ veth <p>w0 ↔ <p>w1 ── ns-<p>-wan 10.<N>.2.2/24: kea-dhcp4 (the agent's Kea, VRX_KEA_MODE=test)
//	   │ veth <p>c0 ↔ <p>c1 ── VPP host-<p>c0, VRF <p>-cli (table <N>002): VPP's DHCPv4 client (no server on its link)
//
//	TestKeaDhcpRelay
//	  commit         vrfs + interfaces + services.dhcp (server `lan` on host-<p>l0 → Kea's netdev <p>w1 via
//	                 VRX_KEA_IFMAP, relay `to-kea` in VRF <p>-dhcp → 10.<N>.2.2) + dhcpClient on host-<p>c0 through the
//	                 API → Kea not running yet: files written, status "start" (D-079) → Kea started → config-get ==
//	                 rendering (the agent's Retrieve; /state/drift clean for services)
//	  relay          V19 guard (TD-3 pre-flight + per-interface bindings) → veths up → dhclient in ns-<p>-lan gets a
//	                 lease through VPP's relay from Kea → the lease is on the API lease page; vppctl show dhcp proxy /
//	                 show dhcp client; interface counters of the relay path
//	  restart-safety agent stopped → relay and client deleted via binapi, Kea set to an idle configuration over its
//	                 socket (simulated loss) → agent started → all three back within 30 s (agent log)
//	  rollback       to the revision before DHCP → Retrieve and vppctl show no relay/client, Kea config-get has no
//	                 subnet; a pool outside its subnet → 400 problem+json with the pool's pointer
//	  cleanup        interfaces deleted through the API with the veths down (D-101), Kea stopped by PID
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; every process
// it starts is stopped by PID; NRestarts of vpp is checked before and after. Never a packet trace (D-128).
package keadhcprelay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
)

const (
	keaDhcp4 = "/usr/sbin/kea-dhcp4"
	dhclient = "/usr/sbin/dhclient"
)

type topo struct {
	s                           slot
	lanNS, wanNS                string
	lanDev, lanPeer             string // <p>l0 (root, VPP af_packet) / <p>l1 (ns-<p>-lan)
	wanDev, wanPeer             string // <p>w0 / <p>w1 (ns-<p>-wan, Kea)
	cliDev, cliPeer             string // <p>c0 / <p>c1 (ns-<p>-lan, VPP DHCP client link)
	lanIf, wanIf, cliIf         string // host-<p>l0 …
	lanGW, wanGW, keaIP         string
	subnet, poolLo, poolHi      string
	vrf, cliVrf                 string
	vrfID, cliVrfID             uint32
	keaBase                     string // /run/vrx-test/<p>/kea
	keaConf, keaSock, keaRunDir string
	work                        string // /run/vrx-test/<p>/kea-relay (0700): dhclient files, logs
}

func newTopo(s slot) *topo {
	p, n := s.prefix, strconv.Itoa(s.num)
	base := s.runDir + "/kea"
	return &topo{
		s: s, lanNS: "ns-" + p + "-lan", wanNS: "ns-" + p + "-wan",
		lanDev: p + "l0", lanPeer: p + "l1", wanDev: p + "w0", wanPeer: p + "w1", cliDev: p + "c0", cliPeer: p + "c1",
		lanIf: "host-" + p + "l0", wanIf: "host-" + p + "w0", cliIf: "host-" + p + "c0",
		lanGW: "10." + n + ".1.1", wanGW: "10." + n + ".2.1", keaIP: "10." + n + ".2.2",
		subnet: "10." + n + ".1.0/24", poolLo: "10." + n + ".1.100", poolHi: "10." + n + ".1.150",
		vrf: p + "-dhcp", cliVrf: p + "-cli", vrfID: uint32(s.num*1000 + 1), cliVrfID: uint32(s.num*1000 + 2), //nolint:gosec // slot 1–11
		keaBase: base, keaConf: base + "/etc/kea-dhcp4.conf", keaSock: base + "/run/kea4.sock", keaRunDir: base + "/run",
		work: s.runDir + "/kea-relay",
	}
}

// keaEnv is the environment of the Kea binaries (renderers/kea Env: Kea 3.0 path restrictions).
func (tp *topo) keaEnv() []string {
	return []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "KEA_CONTROL_SOCKET_DIR=" + tp.keaRunDir, "KEA_DHCP_DATA_DIR=" + tp.keaBase + "/lib",
		"KEA_LOG_FILE_DIR=" + tp.keaBase + "/log", "KEA_PIDFILE_DIR=" + tp.keaRunDir, "KEA_LOCKFILE_DIR=" + tp.keaRunDir,
	}
}

// up builds the netns/veth side (the agent creates the VPP side from the configuration). Peers stay down until the
// V19 guard has passed.
func (tp *topo) up(t *testing.T) {
	t.Helper()
	for _, ns := range []string{tp.lanNS, tp.wanNS} {
		if _, err := run(t, "ip", "netns", "exec", ns, "true"); err == nil {
			t.Fatalf("netns %s exists (leftover? tools/lab rig gc %s and delete %s/%s)", ns, tp.s.prefix, tp.cliDev, tp.cliPeer)
		}
	}
	for _, d := range []string{tp.lanDev, tp.wanDev, tp.cliDev} {
		if _, err := run(t, "ip", "link", "show", d); err == nil {
			t.Fatalf("veth %s exists (leftover of an earlier run?)", d)
		}
	}
	t.Cleanup(func() { tp.down(t) })
	mustRun(t, "ip", "netns", "add", tp.lanNS)
	mustRun(t, "ip", "netns", "add", tp.wanNS)
	for _, pr := range [][3]string{{tp.lanDev, tp.lanPeer, tp.lanNS}, {tp.wanDev, tp.wanPeer, tp.wanNS}, {tp.cliDev, tp.cliPeer, tp.lanNS}} {
		mustRun(t, "ip", "link", "add", pr[0], "type", "veth", "peer", "name", pr[1])
		mustRun(t, "ip", "link", "set", pr[1], "netns", pr[2])
		_, _ = run(t, "sysctl", "-qw", "net.ipv6.conf."+pr[0]+".disable_ipv6=1") // keep RS/MLD noise out of VPP
		_, _ = run(t, "ip", "netns", "exec", pr[2], "sysctl", "-qw", "net.ipv6.conf."+pr[1]+".disable_ipv6=1")
	}
	for _, ns := range []string{tp.lanNS, tp.wanNS} {
		mustRun(t, "ip", "-n", ns, "link", "set", "lo", "up")
	}
	mustRun(t, "ip", "-n", tp.wanNS, "addr", "add", tp.keaIP+"/24", "dev", tp.wanPeer)
}

// peers sets the veth pairs down/up (D-101: an af_packet interface is deleted only with its veth down; no packet
// enters VPP before the V19 guard).
func (tp *topo) peers(t *testing.T, up bool) {
	t.Helper()
	st := "down"
	if up {
		st = "up"
	}
	for _, pr := range [][3]string{{tp.lanDev, tp.lanPeer, tp.lanNS}, {tp.wanDev, tp.wanPeer, tp.wanNS}, {tp.cliDev, tp.cliPeer, tp.lanNS}} {
		mustRun(t, "ip", "link", "set", pr[0], st)
		mustRun(t, "ip", "-n", pr[2], "link", "set", pr[1], st)
	}
	if up {
		mustRun(t, "ip", "-n", tp.wanNS, "route", "replace", "default", "via", tp.wanGW)
	}
}

// down removes what up created. The VPP side is normally gone already (cleanup through the API); after a failure a
// leftover host-interface is deleted here with its veth down first (D-101/V24), the agent being stopped by then.
func (tp *topo) down(t *testing.T) {
	for _, d := range []string{tp.lanDev, tp.wanDev, tp.cliDev} {
		_, _ = run(t, "ip", "link", "set", d, "down")
	}
	time.Sleep(200 * time.Millisecond)
	if out, err := run(t, "vppctl", "show", "interface"); err == nil {
		for _, d := range []string{tp.lanDev, tp.wanDev, tp.cliDev} {
			if regexp.MustCompile(`(?m)^host-` + regexp.QuoteMeta(d) + `\s`).MatchString(out) {
				o, err := run(t, "vppctl", "delete", "host-interface", "name", d)
				t.Logf("leftover host-%s deleted (veth down): %v %s", d, err, strings.TrimSpace(o))
			}
		}
	}
	for _, ns := range []string{tp.lanNS, tp.wanNS} {
		_, _ = run(t, "ip", "netns", "del", ns)
	}
	for _, d := range []string{tp.lanDev, tp.wanDev, tp.cliDev} {
		_, _ = run(t, "ip", "link", "del", d)
	}
	t.Logf("netns %s/%s and veths %s/%s/%s removed", tp.lanNS, tp.wanNS, tp.lanDev, tp.wanDev, tp.cliDev)
}

// keaDirs creates the slot's Kea directories: 0750 (Kea 3.0 refuses a looser socket directory; the agent refuses a
// group-writable or world-accessible one, D-079). Leftover leases/configs of an earlier run are removed first.
func (tp *topo) keaDirs(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll(tp.keaBase); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{tp.keaBase, tp.keaBase + "/etc", tp.keaRunDir, tp.keaBase + "/lib", tp.keaBase + "/log"} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o750); err != nil { //nolint:gosec // directory; Kea 3.0 wants ≤ 0750
			t.Fatal(err)
		}
	}
	_ = os.RemoveAll(tp.work)
	if err := os.MkdirAll(tp.work, 0o700); err != nil {
		t.Fatal(err)
	}
}

// keaCmd is one command over the Kea control socket (the test's own client: evidence and the simulated loss).
func (tp *topo) keaCmd(t *testing.T, command string, args any) (map[string]any, error) {
	t.Helper()
	conn, err := net.DialTimeout("unix", tp.keaSock, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	req := map[string]any{"command": command}
	if args != nil {
		req["arguments"] = args
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.NewDecoder(io.LimitReader(conn, 64<<20)).Decode(&resp); err != nil {
		return nil, err
	}
	if r, _ := resp["result"].(float64); r != 0 && r != 3 {
		return resp, fmt.Errorf("kea %s: result %v: %v", command, resp["result"], resp["text"])
	}
	return resp, nil
}

// keaSubnets returns "prefix(id)" of every subnet4 in config-get.
func (tp *topo) keaSubnets(t *testing.T) ([]string, bool) {
	t.Helper()
	resp, err := tp.keaCmd(t, "config-get", nil)
	if err != nil {
		return nil, false
	}
	args, _ := resp["arguments"].(map[string]any)
	d4, _ := args["Dhcp4"].(map[string]any)
	var out []string
	subs, _ := d4["subnet4"].([]any)
	for _, s := range subs {
		m, _ := s.(map[string]any)
		id, _ := m["id"].(float64)
		out = append(out, fmt.Sprintf("%v(id %.0f)", m["subnet"], id))
	}
	_, hasInput := d4["user-context"]
	return out, hasInput
}

func (tp *topo) startKea(t *testing.T) *proc {
	t.Helper()
	// Kea opens its sockets only on interfaces that are administratively up at (re)configuration time. Only the
	// netns end goes up here: its host end (VPP's af_packet netdev) stays down until the V19 guard, so no packet
	// reaches VPP yet.
	mustRun(t, "ip", "-n", tp.wanNS, "link", "set", tp.wanPeer, "up")
	p := start(t, "kea-dhcp4", filepath.Join(tp.work, "kea-dhcp4.stderr"), tp.keaEnv(), "/usr/bin/ip", "netns", "exec", tp.wanNS, keaDhcp4, "-c", tp.keaConf)
	if !waitFor(30*time.Second, func() bool {
		_, err := tp.keaCmd(t, "status-get", nil)
		return err == nil || p.exited()
	}) || p.exited() {
		raw, _ := os.ReadFile(filepath.Join(tp.work, "kea-dhcp4.stderr")) //nolint:gosec // our own log
		t.Fatalf("kea-dhcp4 did not come up:\n%s", raw)
	}
	return p
}

// dhclientLease runs dhclient once in the lan namespace (no script: -sf /bin/true never touches the host's
// resolv.conf) and returns the fixed-address of the lease it got.
func (tp *topo) dhclientLease(t *testing.T) (string, string) {
	t.Helper()
	conf := filepath.Join(tp.work, "dhclient.conf")
	if err := os.WriteFile(conf, []byte("timeout 40;\nretry 5;\nrequest subnet-mask, routers, domain-name-servers;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lf, pf := filepath.Join(tp.work, "dhclient.leases"), filepath.Join(tp.work, "dhclient.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ip", "netns", "exec", tp.lanNS, dhclient, "-4", "-1", "-v", "-cf", conf, "-sf", "/bin/true", "-lf", lf, "-pf", pf, tp.lanPeer).CombinedOutput() //nolint:gosec // fixed argv
	t.Log("dhclient:\n" + strings.TrimSpace(string(out)))
	if err != nil {
		tp.debug(t)
		t.Fatalf("dhclient got no lease through the relay: %v", err)
	}
	t.Cleanup(func() { // stop the daemonised dhclient (its pid file; our own child); no DHCPRELEASE: the unconfigured interface cannot unicast it
		o, err := exec.Command("ip", "netns", "exec", tp.lanNS, dhclient, "-4", "-x", "-v", "-cf", conf, "-sf", "/bin/true", "-lf", lf, "-pf", pf, tp.lanPeer).CombinedOutput() //nolint:gosec // fixed argv
		t.Logf("dhclient -x (stop, lease kept until it expires): %v %s", err, lastLine(string(o)))
	})
	raw, err := os.ReadFile(lf) //nolint:gosec // our own lease file
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`fixed-address ([0-9.]+);`).FindAllStringSubmatch(string(raw), -1)
	if len(m) == 0 {
		t.Fatalf("no fixed-address in %s:\n%s", lf, raw)
	}
	return m[len(m)-1][1], string(raw)
}

// debug logs read-only evidence of where a DHCP exchange stopped: Kea's log, the dhcp node error counters, the
// relay interfaces' counters.
func (tp *topo) debug(t *testing.T) {
	t.Helper()
	if b, err := os.ReadFile(tp.keaBase + "/log/kea-dhcp4.log"); err == nil { //nolint:gosec // our own Kea log
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 25 {
			lines = lines[len(lines)-25:]
		}
		t.Log("kea-dhcp4.log (tail):\n" + strings.Join(lines, "\n"))
	}
	if out, err := run(t, "vppctl", "show", "errors"); err == nil {
		var keep []string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "dhcp") || strings.Contains(l, "Count") {
				keep = append(keep, l)
			}
		}
		t.Log("vppctl show errors (dhcp nodes):\n" + strings.Join(keep, "\n"))
	}
	_, raw := showIntCounters(t, tp.lanIf, tp.wanIf)
	t.Log("vppctl show interface:\n" + raw)
}

func (tp *topo) peerMAC(t *testing.T) string {
	t.Helper()
	out := mustRun(t, "ip", "-n", tp.lanNS, "-o", "link", "show", tp.lanPeer)
	m := regexp.MustCompile(`link/ether ([0-9a-f:]{17})`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no MAC of %s: %s", tp.lanPeer, out)
	}
	return m[1]
}

// ---- stack --------------------------------------------------------------------------------------------------

type stack struct {
	s        slot
	agentBin string
	agentEnv []string
	agent    *proc
	agentLog string
	apiProc  *proc
	api      *api
	adminPW  string
	work     string
}

func newStack(t *testing.T, s slot, tp *topo) *stack {
	t.Helper()
	bin := os.Getenv("VRX_KEA_AGENT_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "vrx-agent")
		if out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/vrx-agent"); err != nil {
			t.Fatalf("go build vrx-agent: %v\n%s", err, out)
		}
	}
	apiMain := filepath.Join(s.repo, "apps", "api", "dist", "main.js")
	if _, err := os.Stat(apiMain); err != nil {
		t.Fatalf("%s missing — run.sh builds it: %v", apiMain, err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node not in PATH")
	}
	st := &stack{s: s, agentBin: bin, work: tp.work, agentLog: filepath.Join(tp.work, "agent.log")}
	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))
	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"VRX_AGENT_SOCKET="+s.socket, "VRX_OWNER="+s.prefix, "VRX_GLOBALS_OWNER=0", // D-071
		"VRX_AGENT_STATE_DIR="+filepath.Join(tp.work, "agent-state"), "VRX_METRICS_PORT="+s.metricsPort, "VRX_SOCKET_GROUP=root",
		"VRX_LOG_LEVEL=info", "VRX_VPP_TABLE_BASE="+strconv.Itoa(s.num*1000),
		// the slot's Kea test instance (subsystems/kea.go): checkers in the Kea netns, host-<p>l0 served by Kea on <p>w1
		"VRX_KEA_MODE=test", "VRX_KEA_NETNS="+tp.wanNS, "VRX_KEA_IFMAP="+tp.lanIf+"="+tp.wanPeer)
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })
	adminPW := secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "VRX_HTTP_PORT="+s.httpPort, "VRX_HTTP_HOST=127.0.0.1",
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":kea:"+secret()[:6]+":",
		"VRX_AGENT_SOCKET="+s.socket, "VRX_AGENT_OWNER="+s.prefix, "VRX_AGENT_TIMEOUT_MS=60000",
		"VRX_JWT_SECRET="+secret()+secret(), "VRX_SECRET_KEY_FILE="+filepath.Join(tp.work, "secret.key"),
		"VRX_BOOTSTRAP_ADMIN_PASSWORD="+adminPW, "VRX_COOKIE_SECURE=0", "VRX_LOG_LEVEL=warn")
	st.apiProc = start(t, "vrx-api", filepath.Join(tp.work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(60*time.Second, func() bool {
		return st.apiProc.exited() || st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(tp.work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("vrx-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", adminPW)
	st.adminPW = adminPW
	return st
}

func (st *stack) startAgent(t *testing.T) {
	t.Helper()
	st.agent = start(t, "vrx-agent", st.agentLog, st.agentEnv, st.agentBin)
	if !waitFor(30*time.Second, func() bool {
		_, err := os.Stat(st.s.socket)
		return err == nil || st.agent.exited()
	}) || st.agent.exited() {
		raw, _ := os.ReadFile(st.agentLog) //nolint:gosec // our own log
		t.Fatalf("vrx-agent did not come up:\n%s", raw)
	}
}

// ---- VPP (binary API; show commands for evidence) ------------------------------------------------------------

func dumpProxies(t *testing.T, conn vppapi.Connection) []*dhcp.DHCPProxyDetails {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := dhcp.NewServiceClient(conn).DHCPProxyDump(ctx, &dhcp.DHCPProxyDump{IsIP6: false})
	if err != nil {
		t.Fatal(err)
	}
	var out []*dhcp.DHCPProxyDetails
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
}

func hasProxy(t *testing.T, conn vppapi.Connection, rx uint32, server string) bool {
	t.Helper()
	for _, d := range dumpProxies(t, conn) {
		if d.RxVrfID != rx {
			continue
		}
		for _, s := range d.Servers {
			if ip4(s.DHCPServer) == server {
				return true
			}
		}
	}
	return false
}

func ip4(a ip_types.Address) string {
	b := a.Un.GetIP4()
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

func dumpClients(t *testing.T, conn vppapi.Connection) map[uint32]dhcp.DHCPClientDetails {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := dhcp.NewServiceClient(conn).DHCPClientDump(ctx, &dhcp.DHCPClientDump{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]dhcp.DHCPClientDetails{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[uint32(d.Client.SwIfIndex)] = *d
	}
}

// preflight runs TD-3's read-only V19 pre-flight (apps/agent/cmd/vrx-vpp-preflight) over the whole VPP.
func preflight(t *testing.T, s slot) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vrx-vpp-preflight")
	if out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/vrx-vpp-preflight"); err != nil {
		t.Fatalf("build vrx-vpp-preflight: %v\n%s", err, out)
	}
	out, err := run(t, bin)
	if err != nil {
		t.Fatalf("V19 pre-flight (TD-3) FAILED — no DHCP traffic is sent:\n%s", out)
	}
	return out
}

// ---- the test -------------------------------------------------------------------------------------------------

func TestKeaDhcpRelay(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-kea-dhcp-relay topology test: set VRX_INTEGRATION=1 (host VPP, Kea, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket, Kea)")
	}
	for _, b := range []string{keaDhcp4, dhclient} {
		if _, err := os.Stat(b); err != nil {
			t.Skipf("%s not installed", b)
		}
	}
	s := slotFromEnv(t)
	sharedLock(t)
	restarts0 := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before) = %d", restarts0)
	t.Cleanup(func() {
		n := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after) = %d", n)
		if n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %d → %d", restarts0, n)
		}
	})
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	tp := newTopo(s)
	tp.keaDirs(t)
	tp.up(t)
	tp.peers(t, false)
	conn := connectVPP(t)
	st := newStack(t, s, tp)
	a := st.api

	var revBase, revDHCP float64
	parent := t
	// best-effort cleanup through the API (runs before the stack stops, LIFO): nothing of ours stays in VPP even when a
	// step fails; the veths go down first (D-101)
	t.Cleanup(func() { cleanupThroughAPI(parent, st, tp, conn) })
	t.Run("commit", func(t *testing.T) {
		a.t = t
		a.patch("/vrfs", map[string]any{tp.vrf: map[string]any{"id": tp.vrfID}, tp.cliVrf: map[string]any{"id": tp.cliVrfID}})
		a.patch("/interfaces", map[string]any{
			tp.lanIf: map[string]any{"enabled": true, "vrf": tp.vrf, "description": "DHCP clients (relayed)", "ipv4": []string{tp.lanGW + "/24"}},
			tp.wanIf: map[string]any{"enabled": true, "vrf": tp.vrf, "description": "towards the Kea test instance", "ipv4": []string{tp.wanGW + "/24"}},
			tp.cliIf: map[string]any{"enabled": true, "vrf": tp.cliVrf, "description": "VPP DHCP client"},
		})
		revBase = a.commit("kea-base")["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit kea-base → revision %v", revBase)

		a.patch("/services", map[string]any{"dhcp": map[string]any{
			"servers": map[string]any{"lan": map[string]any{
				"description": "LAN (served through the relay)", "vrf": tp.vrf, "interfaces": []string{tp.lanIf}, "leaseTimeSec": 600,
				"subnets": map[string]any{"lan": map[string]any{
					"subnet": tp.subnet, "pools": []any{map[string]any{"start": tp.poolLo, "end": tp.poolHi}},
					"gateway": tp.lanGW, "dnsServers": []string{tp.lanGW}, "domainName": s.prefix + ".example.test",
					"reservations": map[string]any{"printer": map[string]any{"mac": "02:00:00:00:02:01", "ip": "10." + strconv.Itoa(s.num) + ".1.20", "hostname": "printer"}},
				}},
			}},
			"relays": map[string]any{"to-kea": map[string]any{
				"description": "LAN → Kea", "vrf": tp.vrf, "interfaces": []string{tp.lanIf}, "servers": []string{tp.keaIP}, "sourceAddress": tp.wanGW,
			}},
		}})
		a.patch("/interfaces/"+tp.cliIf, map[string]any{"dhcpClient": map[string]any{"hostname": s.prefix + "-vppclient"}})
		d := a.must(200, "GET", "/api/v1/config/diff", nil)
		t.Logf("candidate diff: %s", trunc(d.raw, 1500))
		c := a.commit("kea-dhcp")
		revDHCP = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit kea-dhcp → status %v revision %v txn %v", c["status"], revDHCP, c["txnId"])

		// Kea is not running yet: the configuration is written and the start request is reported (D-079)
		ls := a.must(200, "GET", "/api/v1/state/dhcp/leases?family=ipv4", nil)
		t.Logf("GET /state/dhcp/leases (Kea not started) → servers=%s", js(ls.body["servers"]))
		srv := ls.body["servers"].([]any)[0].(map[string]any)
		if srv["running"] != false || srv["active"] != true || srv["actionRequired"] != "start" {
			t.Fatalf("want running=false active=true actionRequired=start, got %v", srv)
		}
		raw, err := os.ReadFile(tp.keaConf) //nolint:gosec // the agent's rendered file
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s (rendered by the agent, %d bytes, mode %v):\n%s", tp.keaConf, len(raw), fileMode(tp.keaConf), trunc(string(raw), 2500))
		if !strings.Contains(string(raw), `"`+tp.wanPeer+`"`) || !strings.Contains(string(raw), tp.subnet) {
			t.Fatal("rendered kea-dhcp4.conf lacks the mapped interface or the subnet")
		}

		keaProc := tp.startKea(t)
		parent.Cleanup(func() { keaProc.stop(parent) })
		subs, hasInput := tp.keaSubnets(t)
		t.Logf("Kea config-get (own unix socket %s, dir mode %v): subnet4=%v user-context.vrx present=%v", tp.keaSock, fileMode(tp.keaRunDir), subs, hasInput)
		if len(subs) != 1 || !strings.HasPrefix(subs[0], tp.subnet) || !hasInput {
			t.Fatalf("Kea does not run the rendered configuration: %v", subs)
		}
		ls = a.must(200, "GET", "/api/v1/state/dhcp/leases?family=ipv4", nil)
		srv = ls.body["servers"].([]any)[0].(map[string]any)
		t.Logf("GET /state/dhcp/leases (Kea started) → servers=%s", js(ls.body["servers"]))
		if srv["running"] != true || srv["actionRequired"] != "" {
			t.Fatalf("Kea status after start: %v", srv)
		}
		// Retrieve == running for services: the agent's Retrieve re-renders the embedded input and ConfigDrift-checks
		// config-get; /state/drift compares the running document with that Retrieve.
		dr := a.must(200, "GET", "/api/v1/state/drift", nil)
		t.Logf("GET /state/drift → %s", trunc(dr.raw, 2000))
		for _, ch := range dr.body["changes"].([]any) {
			if p, _ := ch.(map[string]any)["pointer"].(string); strings.HasPrefix(p, "/services") {
				t.Errorf("drift in services: %v", ch)
			}
		}
		rel := a.must(200, "GET", "/api/v1/state/dhcp/relays", nil)
		t.Logf("GET /state/dhcp/relays → %s", rel.raw)
		if items := rel.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["state"] != "applied" {
			t.Fatalf("relay state: %s", rel.raw)
		}
	})
	if t.Failed() {
		return
	}

	var lease string
	t.Run("relay", func(t *testing.T) {
		a.t = t
		idx := waitIfs(t, conn, tp.lanIf, tp.wanIf, tp.cliIf)
		t.Log("V19 pre-flight (TD-3 vrx-vpp-preflight): " + strings.TrimSpace(trunc(preflight(t, s), 800)))
		for _, l := range v19Guard(t, conn, idx) {
			t.Log("V19 guard: " + l)
		}
		tp.peers(t, true)
		t.Log("vppctl show dhcp proxy:\n" + vppctl(t, "show", "dhcp", "proxy"))
		t.Log("vppctl show ip fib table " + strconv.Itoa(int(tp.vrfID)) + " 255.255.255.255/32:\n" + vppctl(t, "show", "ip", "fib", "table", strconv.Itoa(int(tp.vrfID)), "255.255.255.255/32"))
		before, _ := showIntCounters(t, tp.lanIf, tp.wanIf)

		var raw string
		lease, raw = tp.dhclientLease(t)
		t.Logf("dhclient lease file:\n%s", trunc(raw, 1200))
		if !strings.HasPrefix(lease, "10."+strconv.Itoa(s.num)+".1.") {
			t.Fatalf("lease %s outside %s", lease, tp.subnet)
		}
		after, rawInt := showIntCounters(t, tp.lanIf, tp.wanIf)
		t.Log("vppctl show interface (relay path):\n" + rawInt)
		// af_packet interfaces of this VPP count received packets per protocol ("ip4"), not as "rx packets"
		for _, n := range []string{tp.lanIf, tp.wanIf} {
			t.Logf("counters %s: ip4 (received) %d→%d, tx packets %d→%d", n, before[n]["ip4"], after[n]["ip4"], before[n]["tx packets"], after[n]["tx packets"])
			if after[n]["ip4"] <= before[n]["ip4"] || after[n]["tx packets"] <= before[n]["tx packets"] {
				t.Errorf("%s: no received/transmitted packets on the relay path", n)
			}
		}
		mac := tp.peerMAC(t)
		var got map[string]any
		if !waitFor(10*time.Second, func() bool {
			r := a.call("GET", "/api/v1/state/dhcp/leases?server=lan&filter="+mac, nil)
			if r.status != 200 {
				return false
			}
			for _, it := range r.body["items"].([]any) {
				if m := it.(map[string]any); m["address"] == lease {
					got = m
					return true
				}
			}
			return false
		}) {
			t.Fatalf("lease %s (%s) not on the API lease page", lease, mac)
		}
		t.Logf("GET /api/v1/state/dhcp/leases?server=lan&filter=%s → %s", mac, js(got))
		page := a.must(200, "GET", "/api/v1/state/dhcp/leases?page=1&pageSize=10", nil)
		t.Logf("GET /api/v1/state/dhcp/leases?page=1&pageSize=10 → total=%v truncated=%v servers=%s", page.body["total"], page.body["truncated"], js(page.body["servers"]))
		t.Log("vppctl show dhcp client:\n" + vppctl(t, "show", "dhcp", "client"))
		cl := a.must(200, "GET", "/api/v1/state/interfaces/"+tp.cliIf+"/dhcp-client", nil)
		t.Logf("GET /state/interfaces/%s/dhcp-client → %s", tp.cliIf, cl.raw)
		if cl.body["configured"] != true || cl.body["state"] == "" {
			t.Fatalf("dhcp client state: %s", cl.raw)
		}
		if err := runShots(t, s, st, tp); err != nil {
			t.Errorf("screenshots: %v", err)
		}
	})
	if t.Failed() {
		return
	}

	t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		st.agent.stop(t)
		ifs := dumpIfs(t, conn)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// simulated loss behind the agent's back: relay server, DHCP client, Kea configuration
		src := ip_types.NewAddress(net.ParseIP(tp.wanGW).To4())
		srv := ip_types.NewAddress(net.ParseIP(tp.keaIP).To4())
		if _, err := dhcp.NewServiceClient(conn).DHCPProxyConfig(ctx, &dhcp.DHCPProxyConfig{RxVrfID: tp.vrfID, ServerVrfID: tp.vrfID, IsAdd: false, DHCPServer: srv, DHCPSrcAddress: src}); err != nil {
			t.Fatalf("loss: dhcp_proxy_config is_add=0: %v", err)
		}
		t.Logf("simulated loss: dhcp_proxy_config rx_vrf=%d server=%s is_add=0 → ok", tp.vrfID, tp.keaIP)
		cli := ifs[tp.cliIf]
		if _, err := dhcp.NewServiceClient(conn).DHCPClientConfig(ctx, &dhcp.DHCPClientConfig{IsAdd: false, Client: dhcp.DHCPClient{SwIfIndex: interface_types.InterfaceIndex(cli.idx), Hostname: s.prefix + "-vppclient"}}); err != nil {
			t.Fatalf("loss: dhcp_client_config is_add=0: %v", err)
		}
		t.Logf("simulated loss: dhcp_client_config sw_if_index=%d (%s) is_add=0 → ok", cli.idx, tp.cliIf)
		idle := map[string]any{"Dhcp4": map[string]any{
			"interfaces-config": map[string]any{"interfaces": []any{}},
			"control-sockets":   []any{map[string]any{"socket-type": "unix", "socket-name": tp.keaSock}},
			"lease-database":    map[string]any{"type": "memfile", "persist": true, "name": tp.keaBase + "/lib/leases4.csv"},
			"subnet4":           []any{},
		}}
		if _, err := tp.keaCmd(t, "config-set", idle); err != nil {
			t.Fatalf("loss: Kea config-set idle: %v", err)
		}
		subs, _ := tp.keaSubnets(t)
		t.Logf("simulated loss: Kea config-set (idle configuration over its socket) → config-get subnet4=%v", subs)
		if _, has := dumpClients(t, conn)[cli.idx]; has || hasProxy(t, conn, tp.vrfID, tp.keaIP) || len(subs) != 0 {
			t.Fatal("loss not complete")
		}
		t0 := time.Now()
		logFrom := fileSize(st.agentLog)
		st.startAgent(t)
		var tProxy, tClient, tKea time.Duration
		ok := waitFor(30*time.Second, func() bool {
			if tProxy == 0 && hasProxy(t, conn, tp.vrfID, tp.keaIP) {
				tProxy = time.Since(t0)
			}
			if tClient == 0 {
				if _, has := dumpClients(t, conn)[cli.idx]; has {
					tClient = time.Since(t0)
				}
			}
			if tKea == 0 {
				if subs, in := tp.keaSubnets(t); len(subs) == 1 && in {
					tKea = time.Since(t0)
				}
			}
			return tProxy > 0 && tClient > 0 && tKea > 0
		})
		t.Logf("agent started at +0s: relay back at +%.2fs, DHCP client at +%.2fs, Kea configuration at +%.2fs (no config API call)", tProxy.Seconds(), tClient.Seconds(), tKea.Seconds())
		if !ok {
			t.Fatal("not everything came back within 30 s")
		}
		lines, raw := readAgentLog(t, st.agentLog, logFrom)
		for i, l := range lines {
			if l.Msg == "vrx-agent starting" || l.Msg == "resync finished" || l.Msg == "kea wired" || strings.Contains(l.Msg, "reconcile") {
				t.Log("agent log: " + trunc(raw[i], 500))
			}
		}
		t.Log("vppctl show dhcp proxy (after recovery):\n" + vppctl(t, "show", "dhcp", "proxy"))
		t.Log("vppctl show dhcp client (after recovery):\n" + vppctl(t, "show", "dhcp", "client"))
		// the path works again: a new lease exchange through the recovered relay and Kea
		l2, _ := tp.dhclientLease(t)
		t.Logf("lease after recovery: %s (first run %s)", l2, lease)
	})
	if t.Failed() {
		return
	}

	t.Run("rollback-and-validation", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=kea-rollback", int(revBase)), nil)
		t.Logf("POST /config/rollback/%d → %s", int(revBase), trunc(rb.raw, 600))
		if rb.body["status"] != "applied" {
			t.Fatalf("rollback: %s", rb.raw)
		}
		ret := a.must(200, "GET", "/api/v1/state/dhcp/relays", nil)
		t.Logf("GET /state/dhcp/relays after rollback → %s", ret.raw)
		if items := ret.body["items"].([]any); len(items) != 0 {
			t.Errorf("relays after rollback: %v", items)
		}
		if hasProxy(t, conn, tp.vrfID, tp.keaIP) {
			t.Error("VPP still relays after the rollback")
		}
		ifs := dumpIfs(t, conn)
		if _, has := dumpClients(t, conn)[ifs[tp.cliIf].idx]; has {
			t.Error("VPP still has the DHCP client after the rollback")
		}
		t.Log("vppctl show dhcp proxy (after rollback):\n" + vppctl(t, "show", "dhcp", "proxy"))
		t.Log("vppctl show dhcp client (after rollback):\n" + vppctl(t, "show", "dhcp", "client"))
		subs, in := tp.keaSubnets(t)
		t.Logf("Kea config-get after rollback: subnet4=%v user-context.vrx present=%v", subs, in)
		if len(subs) != 0 || in {
			t.Error("Kea still runs the DHCP configuration after the rollback")
		}
		cl := a.must(200, "GET", "/api/v1/state/interfaces/"+tp.cliIf+"/dhcp-client", nil)
		t.Logf("GET /state/interfaces/%s/dhcp-client after rollback → %s", tp.cliIf, cl.raw)
		if cl.body["configured"] != false {
			t.Error("client still configured")
		}
		// validation: a pool outside its subnet → 400 problem+json with the pool's pointer (schema tier, at edit time)
		bad := map[string]any{"dhcp": map[string]any{"servers": map[string]any{"bad": map[string]any{
			"vrf": tp.vrf, "interfaces": []string{tp.lanIf},
			"subnets": map[string]any{"lan": map[string]any{"subnet": tp.subnet, "pools": []any{map[string]any{"start": "10.99.0.10", "end": "10.99.0.20"}}}},
		}}}}
		r := a.call("PATCH", "/api/v1/config/services", bad, "content-type", "application/merge-patch+json")
		t.Logf("PATCH /config/services with a pool outside its subnet → %d %s", r.status, trunc(r.raw, 900))
		if r.status != 400 || !strings.Contains(r.raw, "/services/dhcp/servers/bad/subnets/lan/pools/0/start") {
			t.Errorf("want 400 with pointer …/pools/0/start")
		}
		a.must(200, "POST", "/api/v1/config/discard", nil)
	})

	t.Run("cleanup", func(t *testing.T) {
		a.t = t
		cleanupThroughAPI(t, st, tp, conn)
		for n := range dumpIfs(t, conn) {
			if strings.HasPrefix(n, "host-"+s.prefix) {
				t.Errorf("%s still in VPP after the cleanup commit", n)
			}
		}
		for _, d := range dumpProxies(t, conn) {
			if d.RxVrfID/1000 == uint32(s.num) { //nolint:gosec // slot 1–11
				t.Errorf("proxy of rx vrf %d left", d.RxVrfID)
			}
		}
	})
}

// cleanupThroughAPI deletes the test's configuration through the API (veths down first, D-101) and commits. Idempotent:
// a second call finds nothing to delete. Errors are logged, not fatal (it also runs from t.Cleanup after a failure).
func cleanupThroughAPI(t *testing.T, st *stack, tp *topo, conn vppapi.Connection) {
	if st.apiProc.exited() || st.agent.exited() {
		t.Log("cleanup: API or agent not running — nothing deleted through the API")
		return
	}
	a := &api{t: t, base: st.api.base, token: st.api.token}
	for _, pr := range [][3]string{{tp.lanDev, tp.lanPeer, tp.lanNS}, {tp.wanDev, tp.wanPeer, tp.wanNS}, {tp.cliDev, tp.cliPeer, tp.lanNS}} {
		_, _ = run(t, "ip", "link", "set", pr[0], "down")
		_, _ = run(t, "ip", "-n", pr[2], "link", "set", pr[1], "down")
	}
	_ = a.call("POST", "/api/v1/config/discard", nil)
	changed := false
	for _, p := range []string{"/services/dhcp/servers/lan", "/services/dhcp/relays/to-kea", "/interfaces/" + tp.lanIf, "/interfaces/" + tp.wanIf, "/interfaces/" + tp.cliIf, "/vrfs/" + tp.vrf, "/vrfs/" + tp.cliVrf} {
		if r := a.call("DELETE", "/api/v1/config"+p, nil); r.status == 200 {
			changed = true
		}
	}
	if !changed {
		return
	}
	r := a.call("POST", "/api/v1/config/commit?comment=kea-cleanup", nil)
	t.Logf("cleanup commit → %d %s", r.status, trunc(r.raw, 300))
	_ = conn
}

func fileMode(p string) os.FileMode {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.Mode().Perm()
}

// runShots takes the UI screenshots against this real stack when VRX_KEA_SHOTS (a node script outside the repo:
// playwright-core + Chrome from env paths, P07a/P07b) and VRX_KEA_SHOTS_OUT are set: `vite preview` of apps/web/dist
// on the slot web port, stopped by PID. The script is called as node <script> <baseUrl> <outDir> <adminPasswordFile>.
func runShots(t *testing.T, s slot, st *stack, tp *topo) error {
	script, out := os.Getenv("VRX_KEA_SHOTS"), os.Getenv("VRX_KEA_SHOTS_OUT")
	if script == "" || out == "" {
		t.Log("screenshots skipped (VRX_KEA_SHOTS / VRX_KEA_SHOTS_OUT unset)")
		return nil
	}
	webPort := os.Getenv("VRX_WEB_PORT")
	if webPort == "" {
		return errors.New("VRX_WEB_PORT unset")
	}
	web := filepath.Join(s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+s.httpPort, "VRX_WEB_PORT="+webPort)
	pv := start(t, "vite-preview", filepath.Join(tp.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	defer pv.stop(t)
	pwFile := filepath.Join(tp.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(pwFile) }()
	if !waitFor(30*time.Second, func() bool {
		o, err := run(t, "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+webPort+"/")
		return err == nil && o == ""
	}) {
		return errors.New("vite preview did not come up")
	}
	o, err := run(t, "node", script, "http://127.0.0.1:"+webPort, out, pwFile)
	sc := bufio.NewScanner(strings.NewReader(o))
	for sc.Scan() {
		t.Log("shots: " + sc.Text())
	}
	return err
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

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
