// Package acl is F-acl's topology test: ACLs end to end against the REAL host VPP through the af_packet veth/netns rig
// (path: af_packet, D-010), the real vrx-agent and vrx-api on the slot.
//
//	TestACLTopology
//	  commit          rig interfaces (rev 1) → a foreign owner's ACL planted on the lan port (D-066) → objects + L3/L4 list
//	                  on a zone + MACIP list on the wan port (rev 2) → `vppctl show acl-plugin acl|interface|macip …`,
//	                  Retrieve (vrx-agentctl) == running, /state/drift clean under /acl, /state/acl/* live views
//	  traffic         V19 pre-flight → ping lan→wan permitted (rule 10), ping to the wan gateway denied (rule 30); with the
//	                  counters flag on (opt-in VRX_ACL_STATS_GLOBALS=1: flock -x on the globals lock, never switched off,
//	                  V7) the API's per-rule counters rise by exactly the echo requests sent
//	  validation      a rule naming an empty address group → 400 problem+json with the rule's pointer, nothing applied
//	  restart-safety  stop the agent → unbind + delete our ACL and MACIP ACL via binapi (foreign ACL kept) → start → back
//	                  within 30 s, foreign ACL still first and unchanged, ping works
//	  scale           opt-in VRX_ACL_SCALE=<rules> (10000 first, then 100000 in a manager window, NRestarts around each):
//	                  raw acl_add_replace time, CSV import + commit time, first-page latency of the rule editor route
//	  rollback        to rev 1 → Retrieve has no acl, VPP has none of our ACLs/bindings, the foreign ACL alone remains
//	  cleanup         foreign ACL unbound + deleted, interfaces deleted through the API (veths down first, D-101)
//
// Runs only with VRX_INTEGRATION=1, as root, with a slot prefix (w<N>), under flock -s on the lab lock; every process it
// starts is stopped by PID; the slot database is created and dropped by deploy/dev/pg-test.sh. VPP is never restarted
// (D-012); NRestarts is checked before and after. No packet trace (D-128), no classify sweep (D-126).
package acl

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/acl_types"
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

// peers sets both veth pairs down/up (D-101: af_packet interfaces are deleted only with their veths down; nothing
// crosses an interface before the V19 checks passed).
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
	_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+".disable_ipv6=1")
	mustRun(t, "ip", "-n", r.lanNS, "route", "replace", "default", "via", r.lanGW)
	mustRun(t, "ip", "-n", r.wanNS, "route", "replace", "default", "via", r.wanGW)
}

// ping from the lan namespace; returns the output and the received count.
func (r rig) ping(t *testing.T, dst string, count int) (string, int) {
	t.Helper()
	out, _ := run(t, "ip", "netns", "exec", r.lanNS, "ping", "-n", "-c", strconv.Itoa(count), "-W", "1", "-i", "0.3", dst)
	_, rx := pingCounts(out)
	return out, rx
}

var pingRe = regexp.MustCompile(`(\d+) packets transmitted, (\d+) (?:packets )?received`)

func pingCounts(out string) (int, int) {
	m := pingRe.FindStringSubmatch(out)
	if m == nil {
		return -1, -1
	}
	tx, _ := strconv.Atoi(m[1])
	rx, _ := strconv.Atoi(m[2])
	return tx, rx
}

func (r rig) peerMAC(t *testing.T) string {
	t.Helper()
	out := mustRun(t, "ip", "-n", r.wanNS, "-o", "link", "show", r.wanPeer)
	m := regexp.MustCompile(`link/ether ([0-9a-f:]{17})`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no MAC in %q", out)
	}
	return m[1]
}

// stack is vrx-agent (owner = prefix, not the globals owner) + vrx-api on the slot ports and database.
type stack struct {
	s                slot
	agentBin, ctlBin string
	preflightBin     string
	stateDir         string
	agentEnv         []string
	agent            *proc
	agentLog         string
	apiProc          *proc
	api              *api
	adminPW          string
	work             string
}

func buildBin(t *testing.T, envKey, repo, pkg string) string {
	t.Helper()
	if bin := os.Getenv(envKey); bin != "" {
		return bin
	}
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	if out, err := run(t, "go", "build", "-C", filepath.Join(repo, "apps", "agent"), "-o", bin, pkg); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
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
	work := filepath.Join(s.runDir, "acl")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) })
	st := &stack{
		s: s, work: work, stateDir: filepath.Join(work, "agent-state"), agentLog: filepath.Join(work, "agent.log"),
		agentBin:     buildBin(t, "VRX_ACL_AGENT_BIN", s.repo, "./cmd/vrx-agent"),
		ctlBin:       buildBin(t, "VRX_ACL_AGENTCTL_BIN", s.repo, "./cmd/vrx-agentctl"),
		preflightBin: buildBin(t, "VRX_ACL_PREFLIGHT_BIN", s.repo, "./cmd/vrx-vpp-preflight"),
	}
	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))
	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"VRX_AGENT_SOCKET="+s.socket, "VRX_OWNER="+s.prefix, "VRX_GLOBALS_OWNER=0", // D-071
		"VRX_AGENT_STATE_DIR="+st.stateDir, "VRX_METRICS_PORT="+s.metricsPort, "VRX_SOCKET_GROUP=root", "VRX_LOG_LEVEL=info",
		"VRX_VPP_TABLE_BASE="+strconv.Itoa(s.num*1000), "VRX_OBJECTS_DNS_SERVERS=127.0.0.1:9")
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })

	st.adminPW = secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "VRX_HTTP_PORT="+s.httpPort, "VRX_HTTP_HOST=127.0.0.1",
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":acl:"+secret()[:6]+":",
		"VRX_AGENT_SOCKET="+s.socket, "VRX_AGENT_OWNER="+s.prefix, "VRX_AGENT_TIMEOUT_MS=600000",
		"VRX_JWT_SECRET="+secret()+secret(), "VRX_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"VRX_BOOTSTRAP_ADMIN_PASSWORD="+st.adminPW, "VRX_COOKIE_SECURE=0", "VRX_LOG_LEVEL=warn")
	st.apiProc = start(t, "vrx-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(90*time.Second, func() bool {
		return st.apiProc.exited() || st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("vrx-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", st.adminPW)
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

// preflight runs TD-3's V19 pre-flight (envelope host rule: it must exit 0 before ANY ping through the rig).
func (st *stack) preflight(t *testing.T) {
	t.Helper()
	out, err := run(t, st.preflightBin)
	t.Logf("vrx-vpp-preflight: %v\n%s", errOK(err), strings.TrimSpace(out))
	if err != nil {
		t.Fatal("V19 pre-flight did not exit 0 — no packet is sent")
	}
}

// retrieveACL asks the agent itself (vrx-agentctl retrieve = the gRPC Retrieve RPC) for the acl domain.
func (st *stack) retrieveACL(t *testing.T) map[string]any {
	t.Helper()
	out, err := run(t, st.ctlBin, "-s", st.s.socket, "retrieve", "-subsystems", "acl")
	if err != nil {
		t.Fatalf("vrx-agentctl retrieve: %v\n%s", err, out)
	}
	var r struct {
		DesiredState struct {
			Acl map[string]any `json:"acl"`
		} `json:"desiredState"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("retrieve output: %v\n%s", err, trunc(out, 2000))
	}
	return r.DesiredState.Acl
}

// prune drops empty maps/lists and nulls (protobuf JSON omits them, the API document has them), recursively.
func prune(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if p := prune(e); p != nil {
				out[k] = p
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = prune(e)
		}
		return out
	default:
		return v
	}
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… (%d bytes)", len(s))
}

func errOK(err error) string {
	if err == nil {
		return "exit 0"
	}
	return err.Error()
}

// rulePage fetches the rule editor route and returns sequence → live {packets…}.
func rulePage(t *testing.T, a *api, list, query string) (map[string]any, map[float64]map[string]any) {
	t.Helper()
	r := a.must(200, "GET", "/api/v1/state/acl/lists/"+list+"/rules?"+query, nil)
	out := map[float64]map[string]any{}
	for _, it := range r.body["items"].([]any) {
		m := it.(map[string]any)
		live, _ := m["live"].(map[string]any)
		out[m["sequence"].(float64)] = live
	}
	return r.body, out
}

func packetsOf(live map[string]any) float64 {
	if live == nil {
		return -1
	}
	p, _ := live["packets"].(float64)
	return p
}

func globalsLock(t *testing.T) func() {
	t.Helper()
	f, err := os.OpenFile("/run/lock/vrx-globals.lock", os.O_RDONLY|os.O_CREATE, 0o666) //nolint:gosec // the shared globals lock
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }
}

func TestACLTopology(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-acl topology test: set VRX_INTEGRATION=1 (host VPP, rig, PostgreSQL) — run.sh does")
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
	owner := s.prefix
	foreignTag := owner + "-foreign:guard" // carries the slot prefix, but is another owner's tag (D-066)

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

	// rev 1: the rig interfaces
	a.patch("/interfaces", map[string]any{
		r.lanIf: map[string]any{"enabled": true, "description": "F-acl lan side", "ipv4": []string{r.lanGW + "/24"}},
		r.wanIf: map[string]any{"enabled": true, "description": "F-acl wan side", "ipv4": []string{r.wanGW + "/24"}},
	})
	c1 := a.commit("acl-rev1-interfaces")
	rev1 := c1["revision"].(map[string]any)["id"].(float64)
	idx := waitIfs(t, conn, r.lanIf, r.wanIf)
	for _, l := range v19Guard(t, conn, idx) {
		t.Log("V19 guard: " + l)
	}

	// another owner's ACL on the lan input, first in the list (never matches the rig traffic)
	foreign, _ := addACL(t, conn, foreignTag, []acl_types.ACLRule{foreignRule(s.num)})
	setIfaceACLs(t, conn, idx[r.lanIf], 1, foreign)
	t.Cleanup(func() {
		// unbind before delete (D-095c): keep whatever else is bound, drop the foreign ACL, then delete it
		if n, acls := ifaceACLs(t, conn, idx[r.lanIf]); len(acls) > 0 {
			var keep []uint32
			in := n
			for i, x := range acls {
				if x == foreign {
					if i < int(n) {
						in--
					}
					continue
				}
				keep = append(keep, x)
			}
			setIfaceACLs(t, conn, idx[r.lanIf], in, keep...)
		}
		delACL(t, conn, foreign)
		t.Logf("foreign ACL %d (%s) unbound and deleted", foreign, foreignTag)
	})
	t.Logf("planted foreign ACL %d tag %s on %s input", foreign, foreignTag, r.lanIf)

	peerMAC := r.peerMAC(t)
	objects := map[string]any{
		"addresses":     map[string]any{"wan-host": map[string]any{"type": "host", "address": r.wanIP}},
		"addressGroups": map[string]any{"wan-hosts": map[string]any{"members": []string{"wan-host"}}, "none": map[string]any{"members": []string{}}},
		"services":      map[string]any{"echo": map[string]any{"protocol": "icmp", "type": 8}},
		"zones":         map[string]any{"lan": map[string]any{"interfaces": []string{r.lanIf}}},
	}
	aclDoc := map[string]any{
		"lists": map[string]any{
			"lan-in": map[string]any{
				"description": "F-acl topology: lan ingress",
				"rules": []any{
					map[string]any{"sequence": 10, "action": "permit", "description": "echo to the wan host", "source": map[string]any{"kind": "prefix", "prefix": "10." + strconv.Itoa(s.num) + ".1.0/24"},
						"destination": map[string]any{"kind": "object", "name": "wan-hosts"}, "service": map[string]any{"kind": "object", "name": "echo"}},
					map[string]any{"sequence": 15, "action": "reflect", "destination": map[string]any{"kind": "object", "name": "wan-hosts"},
						"service": map[string]any{"kind": "inline", "spec": map[string]any{"protocol": "tcp", "destinationPorts": []string{"80", "443"}}}},
					map[string]any{"sequence": 30, "action": "deny", "ipVersion": "ipv4", "description": "everything else (IPv4)"},
					map[string]any{"sequence": 40, "action": "permit", "ipVersion": "ipv6"},
				},
			},
		},
		"macip": map[string]any{"wan-l2": map[string]any{"rules": []any{
			map[string]any{"sequence": 10, "action": "permit", "sourceMac": peerMAC, "sourcePrefix": r.wanIP + "/32"},
		}}},
		"attachments":      []any{map[string]any{"list": "lan-in", "target": map[string]any{"kind": "zone", "zone": "lan"}, "direction": "in", "sequence": 10}},
		"macipAttachments": []any{map[string]any{"list": "wan-l2", "interface": r.wanIf}},
	}

	var rev2 float64
	t.Run("commit", func(t *testing.T) {
		a.t = t
		a.patch("/objects", objects)
		a.patch("/acl", aclDoc)
		t.Logf("candidate diff: %s", trunc(a.must(200, "GET", "/api/v1/config/diff", nil).raw, 4000))
		c := a.commit("acl-rev2")
		rev2 = c["revision"].(map[string]any)["id"].(float64)
		t.Logf("commit → status %v revision %v results %s", c["status"], rev2, trunc(js(c["results"]), 3000))

		own := ownACLs(t, conn, owner)
		lan, ok := own["lan-in"]
		if !ok || lan.rules != 5 { // 10: 1 (v4) · 15: 2 ports (v4) · 30: 1 · 40: 1 (v6)
			t.Fatalf("VPP ACLs of %s: %v", owner, own)
		}
		n, acls := ifaceACLs(t, conn, idx[r.lanIf])
		if n != 2 || len(acls) != 2 || acls[0] != foreign || acls[1] != lan.idx {
			t.Fatalf("%s input list n_input=%d %v, want [foreign %d, lan-in %d]", r.lanIf, n, acls, foreign, lan.idx)
		}
		mac := ownMacips(t, conn, owner)
		mb := macipBindings(t, conn)
		bound := false
		for _, b := range mb {
			if b.swif == idx[r.wanIf] && b.acl == mac["wan-l2"].idx {
				bound = true
			}
		}
		if !bound || mac["wan-l2"].rules != 1 {
			t.Fatalf("MACIP: %v bindings %v", mac, mb)
		}
		t.Log("vppctl show acl-plugin acl (this slot's ACLs):\n" + showOwnACLs(vppctl(t, "show", "acl-plugin", "acl"), owner+":", foreignTag))
		t.Log("vppctl show acl-plugin interface sw_if_index " + strconv.Itoa(int(idx[r.lanIf])) + " acl:\n" +
			vppctl(t, "show", "acl-plugin", "interface", "sw_if_index", strconv.Itoa(int(idx[r.lanIf])), "acl"))
		t.Log("vppctl show acl-plugin macip acl index " + strconv.Itoa(int(mac["wan-l2"].idx)) + ":\n" +
			vppctl(t, "show", "acl-plugin", "macip", "acl", "index", strconv.Itoa(int(mac["wan-l2"].idx))))
		t.Log("vppctl show acl-plugin macip interface:\n" + grepLines(vppctl(t, "show", "acl-plugin", "macip", "interface"), r.wanIf, "sw_if_index "+strconv.Itoa(int(idx[r.wanIf]))))

		// Retrieve == running (the agent's own view, D-063), and the drift route is clean under /acl
		got := prune(st.retrieveACL(t))
		running := a.must(200, "GET", "/api/v1/config", nil).body
		want := prune(running["acl"])
		if js(got) != js(want) {
			t.Fatalf("Retrieve(acl) != running acl:\n got  %s\n want %s", js(got), js(want))
		}
		t.Logf("vrx-agentctl retrieve -subsystems acl == running acl (%d bytes of JSON)", len(js(got)))
		d := a.must(200, "GET", "/api/v1/state/drift", nil)
		for _, ch := range d.body["changes"].([]any) {
			if p := ch.(map[string]any)["pointer"].(string); strings.HasPrefix(p, "/acl") {
				t.Fatalf("drift under /acl: %v", ch)
			}
		}
		t.Logf("/state/drift: subsystems=%v changes=%d (none under /acl)", d.body["subsystems"], len(d.body["changes"].([]any)))

		lists := a.must(200, "GET", "/api/v1/state/acl/lists", nil)
		t.Logf("GET /state/acl/lists → %s", trunc(lists.raw, 2000))
		att := a.must(200, "GET", "/api/v1/state/acl/attachments", nil)
		t.Logf("GET /state/acl/attachments → %s", trunc(att.raw, 3000))
		for _, it := range att.body["interfaces"].([]any) {
			m := it.(map[string]any)
			if m["interface"] == r.lanIf {
				in := m["input"].([]any)
				if len(in) != 2 || in[0].(map[string]any)["foreign"] != true || in[1].(map[string]any)["name"] != "lan-in" || m["inSync"] != true {
					t.Fatalf("attachments view of %s: %s", r.lanIf, js(m))
				}
			}
		}
	})

	t.Run("traffic-and-counters", func(t *testing.T) {
		a.t = t
		countersOn := countersFlag(t, conn)
		if !countersOn && os.Getenv("VRX_ACL_STATS_GLOBALS") == "1" {
			unlock := globalsLock(t) // D-082: flock -x while changing a VPP-wide setting
			if !countersFlag(t, conn) {
				enableCounters(t, conn)
			}
			countersOn = countersFlag(t, conn)
			unlock()
			t.Logf("counters flag switched on under flock -x /run/lock/vrx-globals.lock (VRX_ACL_STATS_GLOBALS=1); left on (V7): now %v", countersOn)
		}
		t.Logf("VPP counters flag (show acl-plugin tables mask): %v", countersOn)
		st.preflight(t)
		r.peers(t, true)
		out, rx := r.ping(t, r.wanIP, 3) // ARP warm-up
		t.Logf("warm-up ping %s: received %d\n%s", r.wanIP, rx, out)
		page0, before := rulePage(t, a, "lan-in", "source=running")
		const n = 5
		out, rx = r.ping(t, r.wanIP, n)
		t.Logf("ping -c %d %s (permitted by rule 10):\n%s", n, r.wanIP, out)
		if rx != n {
			t.Fatalf("permitted ping: %d/%d received", rx, n)
		}
		out, rx = r.ping(t, r.wanGW, 3)
		t.Logf("ping -c 3 %s (VPP's wan address, denied by rule 30):\n%s", r.wanGW, out)
		if rx != 0 {
			t.Fatalf("denied ping got %d replies", rx)
		}
		page1, after := rulePage(t, a, "lan-in", "source=running")
		t.Logf("GET /state/acl/lists/lan-in/rules (after) → %s", trunc(js(page1), 3000))
		if !countersOn {
			if page1["countersAvailable"] != false || !strings.Contains(fmt.Sprint(page1["countersReason"]), "D-071") {
				t.Fatalf("counters off in VPP: the API must say unavailable with the reason: %v", page1)
			}
			t.Log("counters flag off (not the globals owner, VRX_ACL_STATS_GLOBALS not set): the API reports them unavailable — hit assertions skipped")
			return
		}
		if page0["countersAvailable"] != true || page1["countersAvailable"] != true {
			t.Fatalf("counters on in VPP but the API says unavailable: %v", page1["countersReason"])
		}
		d10 := packetsOf(after[10]) - packetsOf(before[10])
		d30 := packetsOf(after[30]) - packetsOf(before[30])
		t.Logf("rule 10 packets %v → %v (+%v), rule 30 packets %v → %v (+%v)", packetsOf(before[10]), packetsOf(after[10]), d10, packetsOf(before[30]), packetsOf(after[30]), d30)
		if d10 != n || d30 != 3 {
			t.Fatalf("hit counters: rule 10 +%v (want %d), rule 30 +%v (want 3)", d10, n, d30)
		}
	})

	t.Run("validation-empty-group", func(t *testing.T) {
		a.t = t
		a.must(200, "PUT", "/api/v1/config/acl/lists/lan-in/rules/4", map[string]any{
			"sequence": 50, "action": "permit", "destination": map[string]any{"kind": "object", "name": "none"},
		})
		rsp := a.call("POST", "/api/v1/config/commit?comment=acl-empty-group", nil)
		t.Logf("commit → %d %s", rsp.status, trunc(rsp.raw, 1500))
		if rsp.status != 400 || !strings.Contains(rsp.raw, `"pointer":"/acl/lists/lan-in/rules/4/destination/name"`) {
			t.Fatal("a rule naming an empty group must be a 400 problem+json with the rule's pointer")
		}
		a.must(200, "POST", "/api/v1/config/discard", nil)
	})

	t.Run("restart-safety", func(t *testing.T) {
		a.t = t
		r.peers(t, false)
		before := ownACLs(t, conn, owner)
		st.agent.stop(t)
		// simulated loss behind the stopped agent's back: unbind first (D-095c), then delete — ours only
		setIfaceACLs(t, conn, idx[r.lanIf], 1, foreign)
		delACL(t, conn, before["lan-in"].idx)
		mac := ownMacips(t, conn, owner)["wan-l2"]
		macipUnbind(t, conn, idx[r.wanIf], mac.idx)
		macipDel(t, conn, mac.idx)
		if left := ownACLs(t, conn, owner); len(left) != 0 {
			t.Fatalf("loss not simulated: %v", left)
		}
		t.Logf("loss: acl_interface_set_acl_list %s → [foreign %d]; acl_del %d (lan-in); macip_acl_interface_add_del del + macip_acl_del %d (wan-l2)", r.lanIf, foreign, before["lan-in"].idx, mac.idx)
		logFrom := fileSize(st.agentLog)
		t0 := time.Now()
		st.startAgent(t)
		var took time.Duration
		ok := waitFor(30*time.Second, func() bool {
			own := ownACLs(t, conn, owner)
			lan, ok := own["lan-in"]
			if !ok {
				return false
			}
			n, acls := ifaceACLs(t, conn, idx[r.lanIf])
			if n != 2 || len(acls) != 2 || acls[0] != foreign || acls[1] != lan.idx {
				return false
			}
			for _, b := range macipBindings(t, conn) {
				if b.swif == idx[r.wanIf] {
					took = time.Since(t0)
					return true
				}
			}
			return false
		})
		if !ok {
			t.Fatal("ACLs and bindings not back within 30 s")
		}
		t.Logf("ACLs and bindings back %.2f s after the agent start (foreign ACL %d still first)", took.Seconds(), foreign)
		lines, _ := readAgentLog(t, st.agentLog, logFrom)
		for _, l := range lines {
			if strings.Contains(l.Msg, "reconcile") || strings.Contains(l.Msg, "acl domain") {
				t.Logf("agent: %s %s mode=%s status=%s summary=%s", l.Time.Format(time.RFC3339Nano), l.Msg, l.Mode, l.Status, l.Summary)
			}
		}
		for _, e := range aclDump(t, conn) {
			if e.idx == foreign && (e.tag != foreignTag || e.rules != 1) {
				t.Fatalf("foreign ACL changed: %+v", e)
			}
		}
		if got := prune(st.retrieveACL(t)); js(got) != js(prune(a.must(200, "GET", "/api/v1/config", nil).body["acl"])) {
			t.Fatalf("Retrieve after restart != running: %s", js(got))
		}
		st.preflight(t)
		r.peers(t, true)
		out, rx := r.ping(t, r.wanIP, 3)
		if rx == 0 {
			out, rx = r.ping(t, r.wanIP, 3)
		}
		t.Logf("ping after recovery: %d received\n%s", rx, out)
		if rx == 0 {
			t.Fatal("no traffic after recovery")
		}
	})

	t.Run("scale", func(t *testing.T) {
		a.t = t
		n, _ := strconv.Atoi(os.Getenv("VRX_ACL_SCALE"))
		if n <= 0 {
			t.Skip("opt-in: VRX_ACL_SCALE=<rules> (10000 first, then 100000 in a manager window; D-064)")
		}
		scaleStep(t, st, conn, s, n)
	})

	t.Run("rollback", func(t *testing.T) {
		a.t = t
		r.peers(t, false)
		rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%d?comment=acl-rollback", int(rev1)), nil)
		t.Logf("POST /config/rollback/%d → %s", int(rev1), trunc(rb.raw, 1500))
		if got := st.retrieveACL(t); prune(got) != nil {
			t.Fatalf("Retrieve after rollback still has acl: %s", js(got))
		}
		if own := ownACLs(t, conn, owner); len(own) != 0 {
			t.Fatalf("our ACLs after rollback: %v", own)
		}
		if mac := ownMacips(t, conn, owner); len(mac) != 0 {
			t.Fatalf("our MACIP ACLs after rollback: %v", mac)
		}
		n, acls := ifaceACLs(t, conn, idx[r.lanIf])
		if n != 1 || len(acls) != 1 || acls[0] != foreign {
			t.Fatalf("after rollback %s has n_input %d %v, want only the foreign %d", r.lanIf, n, acls, foreign)
		}
		t.Logf("after rollback: vrx-agentctl retrieve -subsystems acl → {} ; %s input = [foreign %d] only", r.lanIf, foreign)
		t.Log("vppctl show acl-plugin acl (this slot):\n" + showOwnACLs(vppctl(t, "show", "acl-plugin", "acl"), owner+":", foreignTag))
	})

	t.Run("cleanup-through-api", func(t *testing.T) {
		a.t = t
		r.peers(t, false) // D-101
		for _, n := range []string{r.lanIf, r.wanIf} {
			a.must(200, "DELETE", "/api/v1/config/interfaces/"+n, nil)
		}
		// the objects that name the interfaces go too (zone)
		a.must(200, "PUT", "/api/v1/config/objects", map[string]any{})
		c := a.call("POST", "/api/v1/config/commit?comment=acl-cleanup", nil)
		t.Logf("commit (cleanup) → %d %s", c.status, trunc(c.raw, 800))
	})
	_ = rev2
}

// grepLines keeps the lines of out that contain one of the needles (plus the following indented lines).
func grepLines(out string, needles ...string) string {
	var b strings.Builder
	keep := false
	for _, l := range strings.Split(out, "\n") {
		hit := false
		for _, n := range needles {
			if strings.Contains(l, n) {
				hit = true
			}
		}
		if hit {
			keep = true
		} else if !strings.HasPrefix(l, " ") {
			keep = false
		}
		if keep {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// scaleStep measures, for n rules: the raw acl_add_replace/acl_del of an n-rule probe ACL (binapi), the CSV import and
// commit of an unattached n-rule list through the API, the rule editor's first page, and its removal. NRestarts is
// checked around the step (D-064).
func scaleStep(t *testing.T, st *stack, conn vppapi.Connection, s slot, n int) {
	t.Helper()
	a := st.api
	r0 := nRestarts(t)
	t.Logf("scale %d: NRestarts before = %d", n, r0)
	defer func() {
		r1 := nRestarts(t)
		t.Logf("scale %d: NRestarts after = %d", n, r1)
		if r1 != r0 {
			t.Errorf("VPP restarted during the %d-rule step", n)
		}
	}()
	probe, d := addACL(t, conn, s.prefix+"-probe:scale", scaleRules(s.num, n))
	t.Logf("scale %d: raw acl_add_replace (%d rules, one message) %.3f s → acl_index %d", n, n, d.Seconds(), probe)
	t0 := time.Now()
	delACL(t, conn, probe)
	t.Logf("scale %d: raw acl_del %.3f s", n, time.Since(t0).Seconds())

	var csv strings.Builder
	csv.WriteString("sequence,action,source,destination,service\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&csv, "%d,permit,10.%d.%d.%d/32,172.%d.%d.%d/32,tcp:443\n", i+1, s.num, 100+i/65536, i/256%256, 16+i/65536, i/256%256, i%256)
	}
	t0 = time.Now()
	dry := a.raw("POST", "/api/v1/actions/acl/import?list=scale&dryRun=true", "text/csv", csv.String())
	t.Logf("scale %d: CSV dry run (%d bytes) → %d in %.2f s: rows=%v valid=%v errors=%v", n, csv.Len(), dry.status, time.Since(t0).Seconds(), dry.body["rows"], dry.body["valid"], dry.body["errorCount"])
	t0 = time.Now()
	imp := a.raw("POST", "/api/v1/actions/acl/import?list=scale&dryRun=false", "text/csv", csv.String())
	t.Logf("scale %d: CSV import → %d in %.2f s: imported=%v", n, imp.status, time.Since(t0).Seconds(), imp.body["imported"])
	if imp.status != 200 {
		t.Fatalf("import: %s", trunc(imp.raw, 1000))
	}
	t0 = time.Now()
	logFrom := fileSize(st.agentLog)
	c := a.call("POST", "/api/v1/config/commit?comment=acl-scale", nil)
	commitTook := time.Since(t0)
	t.Logf("scale %d: commit → %d in %.2f s: %s", n, c.status, commitTook.Seconds(), trunc(c.raw, 1500))
	_, raw := readAgentLog(t, st.agentLog, logFrom)
	for _, l := range raw {
		if strings.Contains(l, "reconcile done") || strings.Contains(l, "reconcile start") {
			t.Logf("scale %d: agent: %s", n, trunc(l, 400))
		}
	}
	committed := c.status == 200 && c.body["status"] == "applied"
	if committed {
		if e, ok := ownACLs(t, conn, s.prefix)["scale"]; !ok || e.rules != n {
			t.Fatalf("scale list in VPP: %+v", e)
		}
	} else {
		t.Logf("scale %d: the commit did not apply (see the response above); the candidate is discarded", n)
	}
	for _, src := range []string{"candidate", "running"} {
		if !committed && src == "running" {
			continue
		}
		t0 = time.Now()
		pg := a.call("GET", "/api/v1/state/acl/lists/scale/rules?page=1&pageSize=100&source="+src, nil)
		t.Logf("scale %d: first page (100 rules, source=%s) → %d in %.3f s: total=%v applied=%v mappingKnown=%v", n, src, pg.status, time.Since(t0).Seconds(), pg.body["total"], pg.body["applied"], pg.body["mappingKnown"])
		t0 = time.Now()
		pg = a.call("GET", fmt.Sprintf("/api/v1/state/acl/lists/scale/rules?page=%d&pageSize=100&source=%s", n/100, src), nil)
		t.Logf("scale %d: last page (source=%s) → %d in %.3f s", n, src, pg.status, time.Since(t0).Seconds())
	}
	if committed {
		a.must(200, "DELETE", "/api/v1/config/acl/lists/scale", nil)
		t0 = time.Now()
		c = a.call("POST", "/api/v1/config/commit?comment=acl-scale-remove", nil)
		t.Logf("scale %d: removal commit → %d in %.2f s", n, c.status, time.Since(t0).Seconds())
	} else {
		a.must(200, "POST", "/api/v1/config/discard", nil)
	}
	if _, ok := ownACLs(t, conn, s.prefix)["scale"]; ok {
		t.Fatal("scale list left in VPP")
	}
	names := []string{}
	for k := range ownACLs(t, conn, s.prefix) {
		names = append(names, k)
	}
	sort.Strings(names)
	t.Logf("scale %d: our ACLs afterwards: %v", n, names)
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
