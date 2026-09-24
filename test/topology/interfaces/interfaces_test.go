// Package interfaces is P08's topology test: the interfaces vertical slice end to end against the REAL host VPP
// through the af_packet veth/netns rig (path: af_packet, D-010).
//
//	TestInterfacesVerticalSlice
//	  topology        rig up → both VPP host-interfaces configured with IPs through the API (candidate → commit) →
//	                  V19 guard → ping lan→wan through VPP + vppctl trace → vppctl show int counters vs the WS
//	                  iface.counters topic (±5 %) → MTU 1400 on the wan side + an extra address, commit → 1500-byte DF
//	                  ping fails → rollback to the first revision → Retrieve shows neither the MTU nor the address,
//	                  the DF ping succeeds again
//	  restart-safety  stop the agent → delete the prefixed host-interfaces and their addresses via binapi (simulated
//	                  loss, dependents first) → start the agent → the interfaces come back and the ping works within
//	                  30 s without any config API call (agent log timestamps); /state/interfaces goes absent → up
//	  cleanup         interfaces deleted through the API (commit) → nothing with the prefix remains in VPP
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; every process
// it starts is stopped by PID; the slot database is created and dropped by deploy/dev/pg-test.sh. The binaries are
// built by run.sh (VRX_P08_AGENT_BIN, apps/api/dist). VPP is never restarted (D-012); NRestarts is checked before and
// after and the test fails if it rises.
package interfaces

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"
)

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

// peers sets both veth pairs down/up. Down: no packet can enter VPP on the rig interfaces (nothing crosses an interface
// before the V19 guard has passed), and — D-101 / V24 — an af_packet interface may only be deleted (by the agent or behind
// its back) while its host-side veth is down: af_packet_delete with the veth up double-closes the socket fds and crashed
// the shared VPP at 07:27:32.
func (r rig) peers(t *testing.T, up bool) {
	t.Helper()
	st := "down"
	if up {
		st = "up"
	}
	if up {
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
	// keep IPv6 RS/MLD noise out of VPP (the rig does the same on the host side)
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "sysctl", "-qw", "net.ipv6.conf."+r.lanPeer+".disable_ipv6=1")
	_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+".disable_ipv6=1")
	// the default route disappears with the link: put it back
	mustRun(t, "ip", "-n", r.lanNS, "route", "replace", "default", "via", r.lanGW)
	mustRun(t, "ip", "-n", r.wanNS, "route", "replace", "default", "via", r.wanGW)
}

// ping from the lan namespace to the wan host; size is the ICMP payload (1472 = a 1500-byte IP packet), df sets DF.
func (r rig) ping(t *testing.T, count, size int, df bool) (string, bool) {
	t.Helper()
	args := []string{"netns", "exec", r.lanNS, "ping", "-n", "-c", strconv.Itoa(count), "-W", "1", "-i", "0.3", "-s", strconv.Itoa(size)}
	if df {
		args = append(args, "-M", "do")
	}
	args = append(args, r.wanIP)
	out, err := run(t, "ip", args...)
	return out, err == nil
}

// stack is the product stack of this test: vrx-agent (owner = prefix) + vrx-api on the slot ports and database.
type stack struct {
	s        slot
	agentBin string
	stateDir string
	agentEnv []string
	agent    *proc
	agentLog string
	apiProc  *proc
	api      *api
	adminPW  string
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
	bin := os.Getenv("VRX_P08_AGENT_BIN")
	if bin == "" { // tools/ci.sh full: build the agent from this tree (run.sh passes a prebuilt one)
		bin = filepath.Join(t.TempDir(), "vrx-agent")
		out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/vrx-agent")
		if err != nil {
			t.Fatalf("go build vrx-agent: %v\n%s", err, out)
		}
	}
	apiMain := filepath.Join(s.repo, "apps", "api", "dist", "main.js")
	if _, err := os.Stat(apiMain); err != nil {
		t.Fatalf("%s missing — run.sh (or the turbo build of tools/ci.sh) builds it: %v", apiMain, err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node not in PATH")
	}
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(s.runDir, "p08")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &stack{s: s, agentBin: bin, stateDir: filepath.Join(work, "agent-state"), agentLog: filepath.Join(work, "agent.log")}

	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))

	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"VRX_AGENT_SOCKET="+s.socket, "VRX_OWNER="+s.prefix, "VRX_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"VRX_AGENT_STATE_DIR="+st.stateDir, "VRX_METRICS_PORT="+s.metricsPort, "VRX_SOCKET_GROUP=root", "VRX_LOG_LEVEL=info")
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })

	adminPW := secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "VRX_HTTP_PORT="+s.httpPort, "VRX_HTTP_HOST=127.0.0.1",
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":p08:"+secret()[:6]+":",
		"VRX_AGENT_SOCKET="+s.socket, "VRX_AGENT_OWNER="+s.prefix, "VRX_AGENT_TIMEOUT_MS=60000",
		"VRX_JWT_SECRET="+secret()+secret(), "VRX_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"VRX_BOOTSTRAP_ADMIN_PASSWORD="+adminPW, "VRX_COOKIE_SECURE=0", "VRX_LOG_LEVEL=warn")
	st.apiProc = start(t, "vrx-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(60*time.Second, func() bool {
		if st.apiProc.exited() {
			return true
		}
		return st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
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

func TestInterfacesVerticalSlice(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("P08 topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
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
	r := newRig(s)
	conn := connectVPP(t)

	// ---- rig: veth/netns + VPP side; the VPP side is removed again (dependents first) so that the AGENT creates
	// both host-interfaces from the configuration. Peers stay down until the V19 guard passed.
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)
	for _, l := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
		t.Log("rig VPP side handed to the agent: " + l)
	}

	st := newStack(t, s)
	a := st.api

	var rev1 float64
	t.Run("topology", func(t *testing.T) {
		a.t = t
		// ---- D-105 (review F4): af_packet may take only a veth. The management NIC is refused at validation
		// (the API's validation problem, 400, with the interface's pointer; running untouched) — it never reaches VPP.
		if mgmt := defaultRouteDev(t); mgmt != "" {
			bad := "host-" + mgmt
			a.patch("/interfaces", map[string]any{bad: map[string]any{"enabled": true}})
			rsp := a.call("POST", "/api/v1/config/commit?comment=p08-f4-mgmt-nic", nil)
			t.Logf("commit of %s (management NIC %s) → %d %v tier=%v errors=%s", bad, mgmt, rsp.status, rsp.body["title"], rsp.body["tier"], js(rsp.body["errors"]))
			if rsp.status != 400 || !strings.Contains(js(rsp.body["errors"]), `"/interfaces/`+bad+`"`) || !strings.Contains(js(rsp.body["errors"]), "interfaces.af-packet-veth") {
				t.Fatalf("committing af_packet on the management NIC %s: want a 400 validation problem with pointer /interfaces/%s (rule interfaces.af-packet-veth)", mgmt, bad)
			}
			a.must(200, "POST", "/api/v1/config/discard", nil)
		}
		a.patch("/interfaces", map[string]any{
			r.lanIf: map[string]any{"enabled": true, "description": "P08 lan side", "ipv4": []string{r.lanGW + "/24"}},
			r.wanIf: map[string]any{"enabled": true, "description": "P08 wan side", "ipv4": []string{r.wanGW + "/24"}},
		})
		d := a.must(200, "GET", "/api/v1/config/diff", nil)
		t.Logf("candidate diff: %s", d.raw)
		c := a.commit("p08-rev1")
		rev1 = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit → status %v revision %v txn %v", c["status"], rev1, c["txnId"])

		idx := waitIfs(t, conn, r.lanIf, r.wanIf)
		for _, l := range v19Guard(t, conn, idx) {
			t.Log("V19 guard: " + l)
		}
		r.peers(t, true)

		items := stateItems(t, a)
		for _, n := range []string{r.lanIf, r.wanIf} {
			it := items[n]
			stt, _ := it["state"].(map[string]any)
			act, _ := it["config"].(map[string]any) // the agent's Retrieve view (D-105)
			t.Logf("/state/interfaces %s: state=%s config(Retrieve)=%s running=%s hasPendingChange=%v", n, js(stt), js(act), js(it["running"]), it["hasPendingChange"])
			if stt == nil || stt["adminUp"] != true || stt["managed"] != true {
				t.Fatalf("%s: live state %v", n, stt)
			}
			if act == nil || act["enabled"] != true {
				t.Fatalf("%s: Retrieve %v", n, act)
			}
		}
		if got := items[r.lanIf]["config"].(map[string]any)["ipv4"]; js(got) != js([]string{r.lanGW + "/24"}) {
			t.Fatalf("Retrieve ipv4 of %s = %v", r.lanIf, got)
		}
		t.Log("vppctl show interface address:\n" + vppctl(t, "show", "interface", "address", r.lanIf, r.wanIf))

		// ---- ping through VPP with a trace of the first packets
		out, ok := r.ping(t, 3, 56, false)
		if !ok { // the first attempt may lose the ARP round
			out, ok = r.ping(t, 3, 56, false)
		}
		t.Log("ping:\n" + out)
		if !ok {
			t.Fatal("ping lan → wan through VPP failed")
		}
		// The trace buffer is shared and never cleared by us (no global `clear trace`, shared-host rules §2): our packet
		// is the echo request with a payload size unique to this run, and the newest matching block wins.
		size := 200 + os.Getpid()%700
		vppctl(t, "trace", "add", "af-packet-input", "20")
		out, ok = r.ping(t, 1, size, false)
		if !ok {
			t.Fatalf("traced ping failed:\n%s", out)
		}
		time.Sleep(300 * time.Millisecond)
		all := vppctl(t, "show", "trace", "max", "5000")
		tr, found := ourTrace(all, r.lanIP, r.wanIP, size+28)
		if !found {
			t.Fatalf("no trace block for our ICMP echo request %s -> %s (IP length %d); trace buffer starts:\n%s", r.lanIP, r.wanIP, size+28, trunc(all, 3000))
		}
		t.Log("vppctl show trace (our ICMP echo request):\n" + tr)
		for _, node := range []string{"af-packet-input", "ip4-lookup", "ip4-rewrite", r.wanIf + "-output"} {
			if !strings.Contains(tr, node) {
				t.Errorf("trace lacks node %s", node)
			}
		}

		// ---- counters: vppctl show int vs the WS iface.counters topic (quiet rig, ±5 %)
		out, ok = r.ping(t, 20, 56, false)
		if !ok {
			t.Fatalf("counter ping failed:\n%s", out)
		}
		time.Sleep(2500 * time.Millisecond)
		ws, err := a.wsCounters(r.lanIf, r.wanIf)
		if err != nil {
			t.Fatalf("WS iface.counters: %v", err)
		}
		cli, raw := showIntCounters(t, r.lanIf, r.wanIf)
		t.Log("vppctl show interface:\n" + raw)
		for _, n := range []string{r.lanIf, r.wanIf} {
			for cliK, wsK := range map[string]string{"rx packets": "rxPackets", "tx packets": "txPackets"} {
				w, _ := strconv.ParseUint(fmt.Sprint(ws[n][wsK]), 10, 64)
				c := cli[n][cliK]
				t.Logf("counters %s %s: vppctl=%d ws=%d", n, cliK, c, w)
				if c < 20 || !within(c, w, 0.05) {
					t.Errorf("%s %s: vppctl %d vs WS %d (want ≥ 20 and within 5 %%)", n, cliK, c, w)
				}
			}
		}
		cr := a.must(200, "GET", "/api/v1/state/interfaces/"+r.lanIf+"/counters", nil)
		t.Logf("GET /state/interfaces/%s/counters → %s", r.lanIf, cr.raw)

		// ---- baseline: a 1500-byte packet with DF passes
		if out, ok := r.ping(t, 2, 1472, true); !ok {
			t.Fatalf("1500-byte DF ping before the MTU change failed:\n%s", out)
		}

		// ---- MTU 1400 on the wan side + an extra lan address, commit
		a.patch("/interfaces/"+r.wanIf, map[string]any{"mtu": 1400})
		extra := "10." + strconv.Itoa(r.slot) + ".3.1/24"
		a.patch("/interfaces/"+r.lanIf, map[string]any{"ipv4": []string{r.lanGW + "/24", extra}})
		c2 := a.commit("p08-mtu1400")
		t.Logf("commit → status %v revision %v", c2["status"], c2["revision"].(map[string]any)["id"])
		items = stateItems(t, a)
		t.Logf("Retrieve after the MTU commit: %s mtu=%v; %s ipv4=%v", r.wanIf, items[r.wanIf]["config"].(map[string]any)["mtu"], r.lanIf, items[r.lanIf]["config"].(map[string]any)["ipv4"])
		if mtu := items[r.wanIf]["config"].(map[string]any)["mtu"]; mtu != float64(1400) {
			t.Fatalf("Retrieve mtu of %s = %v, want 1400", r.wanIf, mtu)
		}
		showWan := vppctl(t, "show", "interface", r.wanIf)
		t.Log("vppctl show interface " + r.wanIf + ":\n" + showWan)
		if !regexp.MustCompile(`\b1400/0/0/0\b`).MatchString(showWan) {
			t.Errorf("vppctl does not show MTU 1400/0/0/0 on %s", r.wanIf)
		}
		out, ok = r.ping(t, 2, 1472, true)
		t.Log("1500-byte DF ping with MTU 1400 (must fail):\n" + out)
		if ok {
			t.Fatal("a 1500-byte DF ping passed an MTU-1400 interface")
		}
		if out, ok := r.ping(t, 2, 56, false); !ok {
			t.Fatalf("a small ping must still pass:\n%s", out)
		}

		// ---- rollback to revision 1: Retrieve (not assumption) shows neither the MTU nor the extra address
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=p08-rollback", int(rev1)), nil)
		t.Logf("POST /config/rollback/%d → %s", int(rev1), rb.raw)
		if rb.body["status"] != "applied" {
			t.Fatalf("rollback: %s", rb.raw)
		}
		items = stateItems(t, a)
		wanAct := items[r.wanIf]["config"].(map[string]any)
		lanAct := items[r.lanIf]["config"].(map[string]any)
		wanSt := items[r.wanIf]["state"].(map[string]any)
		t.Logf("Retrieve after rollback: %s config=%s live mtu=%v; %s config=%s", r.wanIf, js(wanAct), wanSt["mtu"], r.lanIf, js(lanAct))
		if _, has := wanAct["mtu"]; has {
			t.Errorf("Retrieve still reports an MTU object on %s: %v", r.wanIf, wanAct["mtu"])
		}
		if wanSt["mtu"] == float64(1400) {
			t.Errorf("VPP still has MTU 1400 on %s", r.wanIf)
		}
		if js(lanAct["ipv4"]) != js([]string{r.lanGW + "/24"}) {
			t.Errorf("Retrieve ipv4 of %s after rollback = %v", r.lanIf, lanAct["ipv4"])
		}
		addrs := vppctl(t, "show", "interface", "address", r.lanIf, r.wanIf)
		t.Log("vppctl show interface address (after rollback):\n" + addrs)
		if strings.Contains(addrs, strings.TrimSuffix(extra, "/24")) {
			t.Errorf("VPP still has %s", extra)
		}
		t.Log("vppctl show interface " + r.wanIf + " (after rollback):\n" + vppctl(t, "show", "interface", r.wanIf))
		// the lan host learnt PMTU 1400 from VPP's "frag needed" (an exception in its route cache): forget it, the
		// question is what VPP forwards now
		mustRun(t, "ip", "-n", r.lanNS, "route", "flush", "cache")
		out, ok = r.ping(t, 2, 1472, true)
		t.Log("1500-byte DF ping after rollback (must pass):\n" + out)
		if !ok {
			t.Fatal("1500-byte DF ping fails after the rollback")
		}
	})
	if t.Failed() {
		return
	}

	t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		st.agent.stop(t)
		r.peers(t, false) // no packet may enter re-created interfaces before the V19 guard
		for _, l := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
			t.Log("simulated loss: " + l)
		}
		ifs := dumpIfs(t, conn)
		for _, n := range []string{r.lanIf, r.wanIf} {
			if _, ok := ifs[n]; ok {
				t.Fatalf("%s still in VPP after the simulated loss", n)
			}
		}
		t.Log("sw_interface_dump after the loss: neither " + r.lanIf + " nor " + r.wanIf + " exists")
		sAgentDown := a.call("GET", "/api/v1/state/interfaces", nil)
		t.Logf("GET /state/interfaces while the agent is stopped → %d %s", sAgentDown.status, trunc(sAgentDown.raw, 200))

		// observe /state/interfaces (read-only) while the agent recovers: absent/down → up
		type obs struct {
			at    time.Duration
			state string
		}
		var seen []obs
		stopObs := make(chan struct{})
		obsDone := make(chan struct{})
		t0 := time.Now()
		logFrom := fileSize(st.agentLog)
		go func() {
			defer close(obsDone)
			last := ""
			for {
				select {
				case <-stopObs:
					return
				default:
				}
				items, code := a.ifState()
				cur := fmt.Sprintf("http %d", code)
				if code == 200 {
					cur = describe(items[r.lanIf]) + " / " + describe(items[r.wanIf])
				}
				if cur != last {
					seen = append(seen, obs{time.Since(t0), cur})
					last = cur
				}
				time.Sleep(150 * time.Millisecond)
			}
		}()
		st.startAgent(t)
		idx := waitIfs(t, conn, r.lanIf, r.wanIf)
		tRecreated := time.Since(t0)
		for _, l := range v19Guard(t, conn, idx) {
			t.Log("V19 guard: " + l)
		}
		r.peers(t, true)
		var tPing time.Duration
		ok := waitFor(30*time.Second-time.Since(t0), func() bool {
			_, ok := r.ping(t, 1, 56, false)
			return ok
		})
		tPing = time.Since(t0)
		time.Sleep(time.Second)
		close(stopObs)
		<-obsDone
		for _, o := range seen {
			t.Logf("  /state/interfaces +%6.2fs  %s / %s: %s", o.at.Seconds(), r.lanIf, r.wanIf, o.state)
		}
		t.Logf("agent started at +0s; interfaces back in VPP at +%.2fs; ping OK at +%.2fs (no config API call)", tRecreated.Seconds(), tPing.Seconds())
		if !ok || tPing > 30*time.Second {
			t.Fatalf("ping did not recover within 30 s of the agent start (ok=%v, %.1fs)", ok, tPing.Seconds())
		}
		lines, raw := readAgentLog(t, st.agentLog, logFrom)
		var tStart, tResync time.Time
		for i, l := range lines {
			switch {
			case l.Msg == "vrx-agent starting" && tStart.IsZero():
				tStart = l.Time
				t.Log("agent log: " + raw[i])
			case l.Msg == "resync finished" && tResync.IsZero():
				tResync = l.Time
				t.Log("agent log: " + raw[i])
			case strings.Contains(l.Msg, "resync") || strings.Contains(l.Msg, "VPP boot identity"):
				t.Log("agent log: " + trunc(raw[i], 400))
			}
		}
		if tStart.IsZero() || tResync.IsZero() {
			t.Fatal("agent log lacks 'vrx-agent starting' / 'resync finished'")
		}
		t.Logf("reconcile after simulated loss: %s → %s = %.3fs (agent log timestamps)", tStart.Format(time.RFC3339Nano), tResync.Format(time.RFC3339Nano), tResync.Sub(tStart).Seconds())
		if tResync.Sub(tStart) > 30*time.Second {
			t.Errorf("reconcile took %s", tResync.Sub(tStart))
		}
		items := stateItems(t, a)
		for _, n := range []string{r.lanIf, r.wanIf} {
			stt, _ := items[n]["state"].(map[string]any)
			if stt == nil || stt["adminUp"] != true || stt["linkUp"] != true {
				t.Errorf("%s not up after recovery: %v", n, stt)
			}
		}
		t.Log("vppctl show interface address (after recovery):\n" + vppctl(t, "show", "interface", "address", r.lanIf, r.wanIf))
	})

	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
		for _, n := range []string{r.lanIf, r.wanIf} {
			a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
		}
		c := a.commit("p08-cleanup")
		t.Logf("commit (interfaces deleted) → %v revision %v", c["status"], c["revision"].(map[string]any)["id"])
		ifs := dumpIfs(t, conn)
		for n := range ifs {
			if strings.HasPrefix(n, "host-"+s.prefix+"l") || strings.HasPrefix(n, "host-"+s.prefix+"w") {
				t.Errorf("%s still in VPP after the delete commit", n)
			}
		}
	})
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

func stateItems(t *testing.T, a *api) map[string]map[string]any {
	t.Helper()
	items, code := a.ifState()
	if code != 200 {
		t.Fatalf("GET /state/interfaces → %d", code)
	}
	return items
}

func describe(it map[string]any) string {
	if it == nil {
		return "absent"
	}
	stt, _ := it["state"].(map[string]any)
	if stt == nil {
		return "no live state (not in VPP)"
	}
	adm, link := "DOWN", "DOWN"
	if stt["adminUp"] == true {
		adm = "UP"
	}
	if stt["linkUp"] == true {
		link = "UP"
	}
	return "admin " + adm + " link " + link
}

// ourTrace returns the newest trace block of our ICMP echo request (run-unique IP length) and whether there is one;
// the shared trace buffer also holds other packets and earlier runs, so nothing outside that block may count (N3).
func ourTrace(all, src, dst string, ipLen int) (string, bool) {
	want := "length " + strconv.Itoa(ipLen) + ","
	found := ""
	for _, blk := range strings.Split(all, "\nPacket ") {
		if strings.Contains(blk, "ICMP: "+src+" -> "+dst) && strings.Contains(blk, "echo_request") && strings.Contains(blk, want) {
			found = "Packet " + strings.TrimPrefix(blk, "Packet ")
		}
	}
	return found, found != ""
}

func within(a, b uint64, frac float64) bool {
	if a == b {
		return true
	}
	hi, lo := a, b
	if lo > hi {
		hi, lo = lo, hi
	}
	return float64(hi-lo) <= frac*float64(hi)
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

func itoa(n int) string { return strconv.Itoa(n) }
