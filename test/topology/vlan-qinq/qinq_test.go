// Package vlanqinq is F-vlan-qinq's topology test: 802.1Q and QinQ sub-interfaces end to end against the REAL host VPP,
// on the slot rig's wan parent host-<prefix>w0 (path: af_packet, D-010), through the real API and agent.
//
//	TestVlanQinqTopology
//	  commit          parent host-<p>w0 committed alone (revision A), then through the API (candidate → diff → commit, B):
//	                  .100 = dot1q 100, .200 = dot1ad 200 + dot1q 100 (QinQ), .300 = dot1q 300 + dot1q 30, each with an
//	                  address; before B a duplicate (dot1ad, vlanId, innerVlanId) is refused with a 400 problem+json whose
//	                  pointer names the second entry. Evidence: the VPP API rows (tags, sub_if_flags, owner tag),
//	                  Retrieve (/state/interfaces `config`) == desired, `vppctl show interface` / `show interface address`
//	  packets         opt-in (VRX_QINQ_PACKETS=1, manager rule D-126/D-128; V19 guard + pre-flight first): Linux VLAN
//	                  devices in ns-<p>-wan ping VPP over the dot1q and the dot1q-in-dot1q stack; the proof is the answered
//	                  ping plus the rx/tx packet counters of exactly the pinged sub-interface (`vppctl show interface
//	                  <sub>` deltas, never a clear). No VPP packet trace on the shared VPP (D-128): VPP's trace formatter
//	                  jumps to a NULL function for a record of a deleted and reused interface node — that, not this
//	                  phase's frames, crashed VPP at 18:41 (F-vlan-qinq-questions.md Q0). No 802.1ad frame is sent because
//	                  it cannot reach a dot1ad sub-interface on this lab path: af_packet input re-tags the S-tag as
//	                  802.1Q (docs/vpp-code-track.md V-new (F-vlan-qinq)); a lab limit, not a safety measure
//	  restart-safety  stop the agent → delete the three sub-interfaces (addresses first) via binapi → start the agent →
//	                  all back with their addresses within 30 s, no config API call (agent log timestamps)
//	  rollback        rollback to revision A → the results delete each sub-interface's attributes before it; Retrieve has
//	                  no sub-interface; VPP has no host-<p>w0.<id>
//	  cleanup         the parent deleted through the API; nothing of ours with the prefix remains
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; every process it
// starts is stopped by PID; the slot database is created and dropped by deploy/dev/pg-test.sh. VPP is never restarted
// (D-012); NRestarts is checked before and after.
package vlanqinq

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
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
	}
}

// peers sets both veth pairs down/up (P08): down = no packet can enter VPP on the rig interfaces (nothing crosses before
// the V19 guard passed) and — D-101 / V24 — an af_packet interface is only deleted behind the agent's back with its veth
// down (the agent itself quiesces since TD-5).
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
	// keep IPv6 RS/MLD noise out of VPP (the rig does the same on the host side)
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "sysctl", "-qw", "net.ipv6.conf."+r.lanPeer+".disable_ipv6=1")
	_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+".disable_ipv6=1")
}

// sub is one sub-interface of the test: id, tag stack, address (VPP side .1, netns side .2).
type sub struct {
	id           string
	outer, inner uint16
	dot1ad       bool
	net          string // 10.<N>.<x>
	desc         string
}

func (s sub) vppAddr() string      { return s.net + ".1/24" }
func (s sub) nsAddr() string       { return s.net + ".2/24" }
func (s sub) vppIP() string        { return s.net + ".1" }
func (s sub) name(p string) string { return p + "." + s.id }

// config is the sub-interface as the API takes it.
func (s sub) config() map[string]any {
	c := map[string]any{"vlanId": s.outer, "enabled": true, "description": s.desc, "ipv4": []string{s.vppAddr()}}
	if s.inner != 0 {
		c["innerVlanId"] = s.inner
	}
	if s.dot1ad {
		c["dot1ad"] = true
	}
	return c
}

// retrieved is what Retrieve must report for it (defaults explicit, empty lists absent).
func (s sub) retrieved() map[string]any {
	c := s.config()
	c["dot1ad"] = s.dot1ad
	c["vrf"] = "default"
	return c
}

// flags is the sub_if_flags VPP must hold.
func (s sub) flags() interface_types.SubIfFlags {
	f := interface_types.SUB_IF_API_FLAG_EXACT_MATCH | interface_types.SUB_IF_API_FLAG_ONE_TAG
	if s.inner != 0 {
		f = interface_types.SUB_IF_API_FLAG_EXACT_MATCH | interface_types.SUB_IF_API_FLAG_TWO_TAGS
	}
	if s.dot1ad {
		f |= interface_types.SUB_IF_API_FLAG_DOT1AD
	}
	return f
}

func qinqSubs(slotNum int) []sub {
	n := "10." + strconv.Itoa(slotNum)
	return []sub{
		{id: "100", outer: 100, net: n + ".100", desc: "dot1q 100"},
		{id: "200", outer: 200, inner: 100, dot1ad: true, net: n + ".200", desc: "QinQ dot1ad 200 + dot1q 100"},
		{id: "300", outer: 300, inner: 30, net: n + ".30", desc: "dot1q 300 + dot1q 30"},
	}
}

// stack is the product stack of this test: vrx-agent (owner = prefix) + vrx-api on the slot ports and database.
type stack struct {
	s        slot
	work     string
	agentBin string
	stateDir string
	agentEnv []string
	agent    *proc
	agentLog string
	apiProc  *proc
	api      *api
	adminPW  string
	cliBin   string
	cliPW    string
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
	bin := os.Getenv("VRX_QINQ_AGENT_BIN")
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
	work := filepath.Join(s.runDir, "qinq")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &stack{s: s, work: work, agentBin: bin, stateDir: filepath.Join(work, "agent-state"), agentLog: filepath.Join(work, "agent.log")}

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
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":qinq:"+secret()[:6]+":",
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

// cli runs one vrx CLI command against the slot API as admin (built once from apps/cli; password from a 0600 file in
// the test's 0700 work dir; no session file).
func (st *stack) cli(t *testing.T, args ...string) string {
	t.Helper()
	if st.cliBin == "" {
		bin := filepath.Join(t.TempDir(), "vrx") // not in /run (noexec)
		if out, err := run(t, "go", "build", "-C", filepath.Join(st.s.repo, "apps", "cli"), "-o", bin, "./cmd/vrx"); err != nil {
			t.Fatalf("go build vrx (CLI): %v\n%s", err, out)
		}
		st.cliBin = bin
		t.Cleanup(func() { st.cliBin = "" }) // the temp dir goes with the (sub)test
	}
	if st.cliPW == "" {
		pw := filepath.Join(st.work, "cli.pw")
		if err := os.WriteFile(pw, []byte(st.adminPW+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		st.cliPW = pw
		t.Cleanup(func() { _ = os.Remove(pw); st.cliPW = "" }) // no password file outlives the (sub)test
	}
	full := append([]string{"--api", "http://127.0.0.1:" + st.s.httpPort, "--user", "admin", "--password-file", st.cliPW, "--no-session"}, args...)
	out, err := run(t, st.cliBin, full...)
	if err != nil {
		t.Fatalf("vrx %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimRight(out, "\n")
}

// setup is the common part of the topology and the screenshot run: NRestarts guard, rig up, the rig's wan side handed to
// the agent (quiesced first, D-101), the stack.
func setup(t *testing.T) (slot, rig, vppapi.Connection, *stack) {
	t.Helper()
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
	t.Log(mustRun(t, s.lab, "rig", "up", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, s.lab, "rig", "down", s.prefix)
		t.Logf("rig down: %v\n%s", err, out)
	})
	r.peers(t, false)
	// only the wan side is the parent of the sub-interfaces: the agent creates host-<p>w0 from the configuration; the
	// lan side stays the rig's (untagged, removed by rig down)
	for _, l := range deleteBehindBack(t, conn, r.wanDev) {
		t.Log("rig wan side handed to the agent: " + l)
	}
	return s, r, conn, newStack(t, s)
}

func TestVlanQinqTopology(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-vlan-qinq topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (netns, veth, VPP API socket)")
	}
	s, r, conn, st := setup(t)
	a := st.api
	W := r.wanIf
	subs := qinqSubs(s.num)
	names := func() []string {
		var out []string
		for _, x := range subs {
			out = append(out, x.name(W))
		}
		return out
	}()
	var revParent float64

	t.Run("commit", func(t *testing.T) {
		a.t = t
		a.patch("/interfaces", map[string]any{W: map[string]any{"enabled": true, "description": "F-vlan-qinq parent (rig wan)", "ipv4": []string{r.wanGW + "/24"}}})
		c := a.commit("qinq-parent")
		revParent = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit parent → %v revision %v", c["status"], revParent)

		for _, x := range subs {
			a.must(200, "PUT", "/api/v1/config/interfaces/"+W+"/subinterfaces/"+x.id, x.config())
		}
		// acceptance: a duplicate (dot1ad, vlanId, innerVlanId) → 400 problem+json with the pointer to the second entry
		dup := map[string]any{"vlanId": 200, "innerVlanId": 100, "dot1ad": true, "ipv4": []string{"10." + strconv.Itoa(s.num) + ".201.1/24"}}
		a.must(200, "PUT", "/api/v1/config/interfaces/"+W+"/subinterfaces/201", dup)
		rsp := a.call("POST", "/api/v1/config/commit?comment=qinq-duplicate", nil)
		t.Logf("commit with a duplicate tag stack → %d content-type problem+json; body %s", rsp.status, rsp.raw)
		want := `"pointer":"/interfaces/` + W + `/subinterfaces/201/vlanId"`
		if rsp.status != 400 || rsp.body["type"] != "https://vrx.dev/problems/validation" || !strings.Contains(js(rsp.body["errors"]), want) {
			t.Fatalf("duplicate stack: want 400 validation problem with %s", want)
		}
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+W+"/subinterfaces/201", nil)

		d := a.must(200, "GET", "/api/v1/config/diff", nil)
		t.Logf("candidate diff: %s", d.raw)
		c = a.commit("qinq")
		t.Logf("commit sub-interfaces → %v revision %v txn %v", c["status"], c["revision"].(map[string]any)["id"], c["txnId"])
		logResults(t, c)

		// VPP: the rows the agent created (binary API), owner tag, tag stack, flags
		waitIfs(t, conn, append([]string{W}, names...)...)
		ifs := dumpIfs(t, conn)
		for _, x := range subs {
			i := ifs[x.name(W)]
			t.Logf("sw_interface_dump %s: %s", x.name(W), i.stack())
			wantTags := uint8(1)
			if x.inner != 0 {
				wantTags = 2
			}
			if !i.sub || i.sup != ifs[W].idx || i.subID != mustAtoi(x.id) || i.nTags != wantTags || i.outer != x.outer || i.inner != x.inner ||
				i.subFlags != x.flags() || i.tag != s.prefix+":"+x.name(W) {
				t.Errorf("%s in VPP: %s, want outer %d inner %d flags %s tag %s:%s", x.name(W), i.stack(), x.outer, x.inner, subFlagNames(x.flags()), s.prefix, x.name(W))
			}
			if got := ipv4Of(t, conn, i.idx); !reflect.DeepEqual(got, []string{x.vppAddr()}) {
				t.Errorf("%s addresses in VPP %v, want %s", x.name(W), got, x.vppAddr())
			}
		}

		// Retrieve == desired: /state/interfaces `config` is the agent's Retrieve view (D-105)
		items := stateItems(t, a)
		for _, x := range subs {
			it := items[x.name(W)]
			t.Logf("/state/interfaces %s: kind=%v parent=%v state=%s config(Retrieve)=%s running=%s hasPendingChange=%v",
				x.name(W), it["kind"], it["parent"], js(it["state"]), js(it["config"]), js(it["running"]), it["hasPendingChange"])
			if got := canon(it["config"]); js(got) != js(canon(x.retrieved())) {
				t.Errorf("Retrieve %s = %s, want %s", x.name(W), js(got), js(canon(x.retrieved())))
			}
			stt, _ := it["state"].(map[string]any)
			if it["parent"] != W || stt == nil || stt["vlanId"] != float64(x.outer) || stt["innerVlanId"] != float64(x.inner) || stt["adminUp"] != true || stt["managed"] != true {
				t.Errorf("%s live state %s", x.name(W), js(stt))
			}
		}
		parent, _ := items[W]["config"].(map[string]any)
		if got, want := js(canon(parent["subinterfaces"])), js(canon(map[string]any{"100": subs[0].retrieved(), "200": subs[1].retrieved(), "300": subs[2].retrieved()})); got != want {
			t.Errorf("Retrieve %s subinterfaces = %s, want %s", W, got, want)
		}
		t.Log("vppctl show interface " + strings.Join(names, " ") + ":\n" + vppctl(t, append([]string{"show", "interface"}, names...)...))
		t.Log("vppctl show interface address:\n" + vppctl(t, append([]string{"show", "interface", "address", W}, names...)...))

		// the CLI equivalent (docs/user/interfaces/vlan-qinq.md): the committed stacks as set commands, the retrieved row
		for _, args := range [][]string{
			{"configure", "show", "interfaces", W, "subinterfaces", "set"},
			{"show", "interfaces", subs[1].name(W)},
		} {
			t.Logf("$ vrx %s\n%s", strings.Join(args, " "), st.cli(t, args...))
		}
	})
	if t.Failed() {
		return
	}

	t.Run("packets", func(t *testing.T) {
		a.t = t
		if os.Getenv("VRX_QINQ_PACKETS") != "1" {
			t.Skip("packet phase is opt-in: VRX_QINQ_PACKETS=1 (D-126/D-128)")
		}
		// V19: no packet before a dump proves no classify/ACL/SPD binding on these indices (+ the ci pre-flight)
		idx := waitIfs(t, conn, append([]string{W}, names...)...)
		for _, l := range v19Guard(t, conn, idx) {
			t.Log("V19 guard: " + l)
		}
		preflight(t, s)
		r.peers(t, true)
		t.Cleanup(func() { r.peers(t, false) })
		vlanDevices(t, r, subs)
		const echoes = 3
		for _, x := range subs {
			if x.dot1ad {
				t.Logf("%s (%s): no 802.1ad frame is sent — af_packet input re-tags the S-tag as 802.1Q, so it cannot reach a dot1ad sub-interface on this lab path (V-new)", x.name(W), x.desc)
				continue
			}
			before := ifCounters(t, names...)
			out, ok := pingFromWan(t, r, x.vppIP(), echoes, 56)
			if !ok { // the first echo can be lost to ARP
				out, ok = pingFromWan(t, r, x.vppIP(), echoes, 56)
			}
			time.Sleep(300 * time.Millisecond) // the interface counters are flushed per frame; let the last reply land
			after := ifCounters(t, names...)
			t.Logf("ping %s over %s (%s) from %s: ok=%v\n%s", x.vppIP(), x.name(W), x.desc, r.wanNS, ok, out)
			if !ok {
				t.Errorf("ping over %s failed", x.name(W))
			}
			// the frames were classified onto exactly this tag stack: its own rx/tx counters carry the echoes (plus ARP)
			for _, y := range subs {
				rx := after[y.name(W)]["rx packets"] - before[y.name(W)]["rx packets"]
				tx := after[y.name(W)]["tx packets"] - before[y.name(W)]["tx packets"]
				t.Logf("  counters %s: rx packets +%d, tx packets +%d", y.name(W), rx, tx)
				if y.id == x.id && (rx < echoes || tx < echoes) {
					t.Errorf("%s: rx +%d / tx +%d, want at least %d each — the echoes did not use this tag stack", y.name(W), rx, tx, echoes)
				}
			}
		}
		t.Log("vppctl show interface (counters after the pings):\n" + vppctl(t, append([]string{"show", "interface"}, names...)...))
	})

	t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		st.agent.stop(t)
		r.peers(t, false) // no packet may enter re-created interfaces before the V19 guard
		for _, l := range deleteSubsBehindBack(t, conn, names...) {
			t.Log("simulated loss: " + l)
		}
		ifs := dumpIfs(t, conn)
		for _, n := range names {
			if _, ok := ifs[n]; ok {
				t.Fatalf("%s still in VPP after the simulated loss", n)
			}
		}
		t.Logf("sw_interface_dump after the loss: none of %v exists; %s stays (sw_if_index %d)", names, W, ifs[W].idx)
		down := a.call("GET", "/api/v1/state/interfaces", nil)
		t.Logf("GET /state/interfaces while the agent is stopped → %d %s", down.status, trunc(down.raw, 160))

		t0 := time.Now()
		logFrom := fileSize(st.agentLog)
		st.startAgent(t)
		var back time.Duration
		if !waitFor(30*time.Second, func() bool {
			ifs := dumpIfs(t, conn)
			for _, x := range subs {
				i, ok := ifs[x.name(W)]
				if !ok || !reflect.DeepEqual(ipv4Of(t, conn, i.idx), []string{x.vppAddr()}) {
					return false
				}
			}
			back = time.Since(t0)
			return true
		}) {
			t.Fatalf("the sub-interfaces and their addresses are not back within 30 s of the agent start")
		}
		t.Logf("agent started at +0s; all three sub-interfaces back in VPP with their addresses at +%.2fs (no config API call)", back.Seconds())
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
			case strings.Contains(l.Msg, "VPP boot identity") || (strings.Contains(l.Msg, "reconcile") && l.Mode == "resync"):
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
		ifs = dumpIfs(t, conn)
		for _, x := range subs {
			i := ifs[x.name(W)]
			t.Logf("after recovery %s: %s ipv4=%v", x.name(W), i.stack(), ipv4Of(t, conn, i.idx))
			if i.subFlags != x.flags() || i.outer != x.outer || i.inner != x.inner {
				t.Errorf("%s recreated with another stack: %s", x.name(W), i.stack())
			}
		}
		t.Log("vppctl show interface address (after recovery):\n" + vppctl(t, append([]string{"show", "interface", "address"}, names...)...))
		items := stateItems(t, a)
		for _, x := range subs {
			if got := canon(items[x.name(W)]["config"]); js(got) != js(canon(x.retrieved())) {
				t.Errorf("Retrieve %s after recovery = %s", x.name(W), js(got))
			}
		}
	})

	t.Run("rollback", func(t *testing.T) {
		a.t = t
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=qinq-rollback", int(revParent)), nil)
		t.Logf("POST /config/rollback/%d → status %v revision %v", int(revParent), rb.body["status"], rb.body["revision"].(map[string]any)["id"])
		if rb.body["status"] != "applied" {
			t.Fatalf("rollback: %s", rb.raw)
		}
		logResults(t, rb.body)
		// the attributes of each sub-interface are deleted before the sub-interface (Q1 defect fix, e771ecb)
		pos := map[string]int{}
		for i, x := range rb.body["results"].([]any) {
			pos[x.(map[string]any)["key"].(string)] = i
		}
		for _, n := range names {
			sk, ok := pos["interface.subinterface/"+n]
			if !ok {
				t.Errorf("rollback did not delete %s", n)
				continue
			}
			for k, p := range pos {
				if (strings.HasSuffix(k, "/"+n) || strings.Contains(k, "/"+n+"/")) && !strings.HasPrefix(k, "interface.subinterface/") && p > sk {
					t.Errorf("%s deleted after the sub-interface %s", k, n)
				}
			}
		}
		items := stateItems(t, a)
		for _, n := range names {
			if it, ok := items[n]; ok {
				t.Errorf("/state/interfaces still lists %s: %s", n, js(it))
			}
		}
		parent, _ := items[W]["config"].(map[string]any)
		t.Logf("Retrieve after rollback: %s config=%s (rows for %v: none)", W, js(parent), names)
		if subsNow, _ := parent["subinterfaces"].(map[string]any); len(subsNow) != 0 {
			t.Errorf("Retrieve still has sub-interfaces: %s", js(subsNow))
		}
		ifs := dumpIfs(t, conn)
		var left []string
		for n := range ifs {
			if strings.HasPrefix(n, W+".") {
				left = append(left, n)
			}
		}
		t.Logf("sw_interface_dump after rollback: %d interface(s) named %s.* %v", len(left), W, left)
		if len(left) != 0 {
			t.Errorf("sub-interfaces left in VPP: %v", left)
		}
		show := vppctl(t, "show", "interface")
		t.Log("vppctl show interface (after rollback):\n" + show)
		if strings.Contains(show, W+".") {
			t.Errorf("vppctl still shows a %s.<id> sub-interface", W)
		}
	})

	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		r.peers(t, false)
		a.must(200, "DELETE", "/api/v1/config/interfaces/"+W, nil)
		c := a.commit("qinq-cleanup")
		t.Logf("commit (parent deleted) → %v revision %v", c["status"], c["revision"].(map[string]any)["id"])
		ifs := dumpIfs(t, conn)
		var ours []string
		for _, n := range sortedNames(ifs) {
			if n == W || strings.HasPrefix(n, W+".") {
				t.Errorf("%s still in VPP after the delete commit", n)
			}
			if strings.Contains(n, s.prefix) || strings.HasPrefix(ifs[n].tag, s.prefix+":") {
				ours = append(ours, fmt.Sprintf("%s (tag %q)", n, ifs[n].tag))
			}
		}
		t.Logf("sw_interface_dump, everything with prefix %s after the cleanup commit (rig down removes the rig's own lan side): %v", s.prefix, ours)
	})
}

// vlanDevices creates the Linux VLAN devices of the 802.1Q stacks on the netns side of the wan veth (ns-<p>-wan, removed
// with the netns by rig down): dot1q → <peer>.<outer>; dot1q-in-dot1q → <peer>.<outer> + <peer>.<outer>.<inner>. A dot1ad
// stack gets none: its frames cannot reach the sub-interface on the af_packet lab path (V-new (F-vlan-qinq)).
func vlanDevices(t *testing.T, r rig, subs []sub) {
	t.Helper()
	ip := func(args ...string) { mustRun(t, "ip", append([]string{"-n", r.wanNS}, args...)...) }
	const proto = "802.1Q"
	for _, x := range subs {
		if x.dot1ad {
			continue
		}
		outer := r.wanPeer + "." + strconv.Itoa(int(x.outer))
		noV6 := func(dev string) { // no IPv6 RS/MLD noise into VPP: off before the link comes up
			_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+strings.ReplaceAll(dev, ".", "/")+".disable_ipv6=1")
		}
		ip("link", "add", "link", r.wanPeer, "name", outer, "type", "vlan", "proto", proto, "id", strconv.Itoa(int(x.outer)))
		noV6(outer)
		ip("link", "set", outer, "up")
		dev := outer
		if x.inner != 0 {
			dev = outer + "." + strconv.Itoa(int(x.inner))
			ip("link", "add", "link", outer, "name", dev, "type", "vlan", "proto", "802.1Q", "id", strconv.Itoa(int(x.inner)))
			noV6(dev)
			ip("link", "set", dev, "up")
		}
		ip("addr", "add", x.nsAddr(), "dev", dev)
		t.Logf("netns %s: %s (%s %d%s) %s", r.wanNS, dev, proto, x.outer, map[bool]string{true: " + 802.1Q " + strconv.Itoa(int(x.inner)), false: ""}[x.inner != 0], x.nsAddr())
	}
	t.Log(mustRun(t, "ip", "-n", r.wanNS, "-d", "-br", "link", "show"))
}

func pingFromWan(t *testing.T, r rig, dst string, count, size int) (string, bool) {
	t.Helper()
	out, err := run(t, "ip", "netns", "exec", r.wanNS, "ping", "-n", "-c", strconv.Itoa(count), "-W", "1", "-i", "0.3", "-s", strconv.Itoa(size), dst)
	return out, err == nil
}

// preflight runs TD-3's read-only V19 pre-flight (the one tools/ci.sh full runs) and fails on a crash vector.
func preflight(t *testing.T, s slot) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vrx-vpp-preflight")
	if out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/vrx-vpp-preflight"); err != nil {
		t.Fatalf("build vrx-vpp-preflight: %v\n%s", err, out)
	}
	out, err := run(t, bin)
	t.Logf("vrx-vpp-preflight: exit %v\n%s", err, strings.TrimSpace(out))
	if err != nil {
		t.Fatal("V19 pre-flight found a crash vector (or VPP is unreachable): no packets")
	}
}

// ---- helpers ----------------------------------------------------------------------------------------------------------

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

// logResults prints a commit/rollback's object results in order (key, op, code).
func logResults(t *testing.T, body map[string]any) {
	t.Helper()
	res, _ := body["results"].([]any)
	var b strings.Builder
	for i, x := range res {
		m, _ := x.(map[string]any)
		fmt.Fprintf(&b, "  %2d %-6v %-4v %v\n", i, m["op"], m["code"], m["key"])
	}
	t.Logf("results (%d, in order) summary=%s:\n%s", len(res), js(body["summary"]), b.String())
}

// canon drops empty lists/objects and the nulls of a JSON value (Retrieve omits empty lists; the API's running copy has
// Zod's `[]` defaults), so two canonical forms compare as JSON.
func canon(v any) any {
	raw, _ := json.Marshal(v)
	var x any
	_ = json.Unmarshal(raw, &x)
	var walk func(any) (any, bool)
	walk = func(v any) (any, bool) {
		switch t := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, e := range t {
				if c, ok := walk(e); ok {
					out[k] = c
				}
			}
			return out, len(out) > 0
		case []any:
			return t, len(t) > 0
		case nil:
			return nil, false
		}
		return v, true
	}
	c, _ := walk(x)
	return c
}

// ifCounters reads the packet counters of the named interfaces (`vppctl show interface <names>`; read-only, the shared
// counters are never cleared, callers compare deltas): name → "rx packets"/"tx packets"/… → value.
func ifCounters(t *testing.T, names ...string) map[string]map[string]uint64 {
	t.Helper()
	return parseIfCounters(vppctl(t, append([]string{"show", "interface"}, names...)...))
}

// parseIfCounters parses `show interface`: a row that starts in column 0 names the interface (name, idx, state, mtu,
// then its first counter); the indented rows below it carry one counter each ("rx packets 3", "drops 1").
func parseIfCounters(out string) map[string]map[string]uint64 {
	res := map[string]map[string]uint64{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] == "Name" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			if len(f) < 4 {
				cur = ""
				continue
			}
			cur, f = f[0], f[4:]
			res[cur] = map[string]uint64{}
		}
		if cur == "" || len(f) < 2 {
			continue
		}
		n, err := strconv.ParseUint(f[len(f)-1], 10, 64)
		if err != nil {
			continue
		}
		res[cur][strings.Join(f[:len(f)-1], " ")] = n
	}
	return res
}

// TestParseIfCounters is the unit check of the counter parser (runs without VRX_INTEGRATION): vppctl's CRLF output, a
// header, an interface without counters, one with several.
func TestParseIfCounters(t *testing.T) {
	out := "              Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count     \r\n" +
		"host-w5w0.100                     3      up           0/0/0/0       rx packets                     5\r\n" +
		"                                                                    rx bytes                     430\r\n" +
		"                                                                    tx packets                     4\r\n" +
		"                                                                    tx bytes                     392\r\n" +
		"                                                                    ip4                            5\r\n" +
		"host-w5w0.300                     1      up           0/0/0/0       \r\n" +
		"local0                            0     down          0/0/0/0       drops                          4\r\n"
	got := parseIfCounters(out)
	want := map[string]map[string]uint64{
		"host-w5w0.100": {"rx packets": 5, "rx bytes": 430, "tx packets": 4, "tx bytes": 392, "ip4": 5},
		"host-w5w0.300": {},
		"local0":        {"drops": 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIfCounters = %v, want %v", got, want)
	}
}

func mustAtoi(s string) uint32 {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint32(n)
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

func sortedNames(m map[string]vppIf) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
