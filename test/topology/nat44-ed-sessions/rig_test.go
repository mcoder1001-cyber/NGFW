package nat44edsessions

// The rig (docs/lab: tools/lab rig up <prefix>) and the product stack of this test, adapted from P08's topology test
// (test/topology/interfaces): the real ngfw-agent (owner = slot prefix, NGFW_GLOBALS_OWNER=0) and ngfw-api on the slot's
// ports and throwaway database.

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

// addr is an address of this slot: 10.<N>.<a>.<b>.
func (r rig) addr(a, b int) string {
	return "10." + strconv.Itoa(r.slot) + "." + strconv.Itoa(a) + "." + strconv.Itoa(b)
}

// peers sets both veth pairs down/up. Down: no packet enters VPP before the V19 guard, and (D-101 / V24) an
// af_packet interface is only deleted with its host-side veth down.
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
	_, _ = run(t, "ip", "netns", "exec", r.lanNS, "sysctl", "-qw", "net.ipv6.conf."+r.lanPeer+".disable_ipv6=1")
	_, _ = run(t, "ip", "netns", "exec", r.wanNS, "sysctl", "-qw", "net.ipv6.conf."+r.wanPeer+".disable_ipv6=1")
	mustRun(t, "ip", "-n", r.lanNS, "route", "replace", "default", "via", r.lanGW)
	mustRun(t, "ip", "-n", r.wanNS, "route", "replace", "default", "via", r.wanGW)
	// The namespaces' kernels hand TCP/UDP to the veth with a PARTIAL checksum (tx checksum offload); VPP's af_packet
	// input keeps it partial, NAT44-ED rewrites addresses/ports on top of it and the far host drops the segment
	// silently (ICMP, checksummed in software, passes). A real NIC delivers complete checksums: offload off in the rig
	// namespaces (test side only; F-nat44-ed-sessions questions Q7 / V-new).
	mustRun(t, "ip", "netns", "exec", r.lanNS, "ethtool", "-K", r.lanPeer, "tx", "off")
	mustRun(t, "ip", "netns", "exec", r.wanNS, "ethtool", "-K", r.wanPeer, "tx", "off")
}

// inNS runs a fixed command in a namespace of the rig.
func inNS(t *testing.T, ns string, args ...string) (string, error) {
	t.Helper()
	return run(t, "ip", append([]string{"netns", "exec", ns}, args...)...)
}

// stack is the product stack of this test.
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
	work     string
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
	bin := os.Getenv("NGFW_NAT_AGENT_BIN")
	if bin == "" { // tools/ci.sh full: build the agent from this tree (run.sh passes a prebuilt one)
		bin = filepath.Join(t.TempDir(), "ngfw-agent")
		out, err := run(t, "go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/ngfw-agent")
		if err != nil {
			t.Fatalf("go build ngfw-agent: %v\n%s", err, out)
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
	work := filepath.Join(s.runDir, "nat44ed")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &stack{s: s, agentBin: bin, stateDir: filepath.Join(work, "agent-state"), agentLog: filepath.Join(work, "agent.log"), work: work}

	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))

	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"NGFW_VPP_TABLE_BASE="+strconv.Itoa(1000*s.num),
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_OWNER="+s.prefix, "NGFW_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"NGFW_AGENT_STATE_DIR="+st.stateDir, "NGFW_METRICS_PORT="+s.metricsPort, "NGFW_SOCKET_GROUP=root", "NGFW_LOG_LEVEL=info")
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })

	adminPW := secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "NGFW_HTTP_PORT="+s.httpPort, "NGFW_HTTP_HOST=127.0.0.1",
		"NGFW_PG_DSN="+pg["NGFW_PG_DSN"], "NGFW_VALKEY_DB="+s.valkeyDB, "NGFW_VALKEY_PREFIX=ngfw:"+s.prefix+":nat:"+secret()[:6]+":",
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_AGENT_OWNER="+s.prefix, "NGFW_AGENT_TIMEOUT_MS=60000",
		"NGFW_JWT_SECRET="+secret()+secret(), "NGFW_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"NGFW_BOOTSTRAP_ADMIN_PASSWORD="+adminPW, "NGFW_COOKIE_SECURE=0", "NGFW_LOG_LEVEL=warn")
	st.apiProc = start(t, "ngfw-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(60*time.Second, func() bool {
		if st.apiProc.exited() {
			return true
		}
		return st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("ngfw-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", adminPW)
	st.adminPW = adminPW
	return st
}

func (st *stack) startAgent(t *testing.T) {
	t.Helper()
	st.agent = start(t, "ngfw-agent", st.agentLog, st.agentEnv, st.agentBin)
	if !waitFor(30*time.Second, func() bool {
		if st.agent.exited() {
			return true
		}
		c, err := net.DialTimeout("unix", st.s.socket, time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	}) || st.agent.exited() {
		raw, _ := os.ReadFile(st.agentLog) //nolint:gosec // our own log
		t.Fatalf("ngfw-agent did not come up:\n%s", raw)
	}
	// Wait for the persistent API gRPC channel to recover before mutations.
	// Socket readiness alone does not end its reconnect backoff.
	if st.api != nil && !waitFor(15*time.Second, func() bool {
		return st.agent.exited() || st.api.call("GET", "/api/v1/state/drift", nil).status == 200
	}) {
		t.Fatal("API channel did not reconnect to the restarted agent within 15 s")
	}
	if st.agent.exited() {
		raw, _ := os.ReadFile(st.agentLog)
		t.Fatalf("ngfw-agent exited during readiness check:\n%s", raw)
	}

}
