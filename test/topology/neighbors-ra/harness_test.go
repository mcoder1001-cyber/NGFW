package neighborsra

// newStack/startAgent: copied from P08's test/topology/interfaces (not this task's files), with this test's work dir
// ("nra"), agent binary variable (NGFW_NRA_AGENT_BIN) and the slot's table range for the agent (NGFW_VPP_TABLE_BASE).

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

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
	bin := os.Getenv("NGFW_NRA_AGENT_BIN")
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
	work := filepath.Join(s.runDir, "nra")
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
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_OWNER="+s.prefix, "NGFW_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"NGFW_AGENT_STATE_DIR="+st.stateDir, "NGFW_METRICS_PORT="+s.metricsPort, "NGFW_SOCKET_GROUP=root", "NGFW_LOG_LEVEL=info",
		"NGFW_VPP_TABLE_BASE="+strconv.Itoa(1000*s.num)) // slot table range N000–N999: proxy-ARP ranges (arp.Register)
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })

	adminPW := secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "NGFW_HTTP_PORT="+s.httpPort, "NGFW_HTTP_HOST=127.0.0.1",
		"NGFW_PG_DSN="+pg["NGFW_PG_DSN"], "NGFW_VALKEY_DB="+s.valkeyDB, "NGFW_VALKEY_PREFIX=ngfw:"+s.prefix+":nra:"+secret()[:6]+":",
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
	// The API keeps its gRPC channel across agent restarts. A listening socket does
	// not imply that channel has left reconnect backoff yet. Wait on a read-only
	// RPC before issuing commits; never retry a mutation to mask an outage.
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
