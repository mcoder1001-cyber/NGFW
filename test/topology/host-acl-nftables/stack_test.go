package hostacl

// The stack under test (copied from test/topology/object-model, F-host-acl-nftables): the real ngfw-agent binary (built
// from this tree, owner = NGFW_TEST_PREFIX, the slot's socket and state dir, NGFW_HOST_ACL_NETNS = the slot's "host"
// namespace, so the agent loads table inet ngfw_<prefix> there and never in the root netns) and the real ngfw-api
// (apps/api/dist, slot port) on a throwaway slot database. Every process is started by this test and stopped by PID. No
// VPP object is created.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const labLock = "/run/lock/ngfw-lab.lock"

type slot struct {
	num                                              int
	prefix, httpPort, metricsPort, valkeyDB, webPort string
	runDir, socket, repo                             string
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
		num:         n,
		prefix:      p,
		httpPort:    env("NGFW_HTTP_PORT", strconv.Itoa(3000+100*n)),
		webPort:     env("NGFW_WEB_PORT", strconv.Itoa(5000+100*n)),
		metricsPort: env("NGFW_METRICS_PORT", strconv.Itoa(9100+10*n+1)),
		valkeyDB:    env("NGFW_VALKEY_DB", strconv.Itoa(n)),
		runDir:      "/run/ngfw-test/" + p,
		repo:        repoRoot(t),
	}
	s.socket = env("NGFW_AGENT_SOCKET", s.runDir+"/agent.sock")
	return s
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

// sharedLock holds flock -s on the lab lock for the test's duration (D-094: only during an actual run).
func sharedLock(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(labLock, os.O_RDONLY|os.O_CREATE, 0o666) //nolint:gosec // the shared lab lock file
	if err != nil {
		t.Fatalf("open %s: %v", labLock, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatalf("flock -s %s: %v", labLock, err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

// mkdirShared creates dir and missing parents 0755 whatever the umask; an existing directory is never re-moded (D-106/D-107).
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

// run executes a fixed command (no user input) and returns its combined output.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // fixed test commands
	return string(out), err
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := run(name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func nRestarts(t *testing.T) int {
	t.Helper()
	out := mustRun(t, "systemctl", "show", "vpp", "-p", "NRestarts")
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "NRestarts=")))
	if err != nil {
		t.Fatalf("NRestarts: %q", out)
	}
	return n
}

func secret() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// proc is a child process started by the test, stopped by PID.
type proc struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
}

func start(t *testing.T, name, logPath string, env []string, bin string, args ...string) *proc {
	t.Helper()
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // our own log
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // binaries this test built
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		t.Fatalf("start %s: %v", name, err)
	}
	p := &proc{name: name, cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = f.Close(); close(p.done) }()
	t.Logf("started %s pid %d (log %s)", name, cmd.Process.Pid, logPath)
	return p
}

// stop sends SIGTERM to the PID we started and waits (SIGKILL after 10 s).
func (p *proc) stop(t *testing.T) {
	if p == nil || p.cmd.Process == nil || p.exited() {
		return
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

// ---- stack ---------------------------------------------------------------------------------------

type stack struct {
	s        slot
	work     string
	agentBin string
	ctlBin   string
	agentEnv []string
	agentLog string
	stateDir string
	agent    *proc
	apiProc  *proc
	api      *api
	adminPW  string
}

func buildBin(t *testing.T, envKey, repo, pkg string) string {
	t.Helper()
	if bin := os.Getenv(envKey); bin != "" {
		return bin
	}
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg)) // tools/ci.sh full: build from this tree
	if out, err := run("go", "build", "-C", filepath.Join(repo, "apps", "agent"), "-o", bin, pkg); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

// newStack starts agent + API on the slot; the agent's host firewall lives in the namespace hostNS.
func newStack(t *testing.T, s slot, hostNS string) *stack {
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
	work := filepath.Join(s.runDir, "host-acl-nftables")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) }) // the slot's agent state dir goes with it (envelope cleanup)
	st := &stack{
		s: s, work: work, stateDir: filepath.Join(work, "agent-state"), agentLog: filepath.Join(work, "agent.log"),
		agentBin: buildBin(t, "NGFW_HA_AGENT_BIN", s.repo, "./cmd/ngfw-agent"),
		ctlBin:   buildBin(t, "NGFW_HA_AGENTCTL_BIN", s.repo, "./cmd/ngfw-agentctl"),
	}
	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))

	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"NGFW_VPP_TABLE_BASE="+strconv.Itoa(1000*s.num),
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_OWNER="+s.prefix, "NGFW_GLOBALS_OWNER=0", // D-071: test slots never own globals
		"NGFW_AGENT_STATE_DIR="+st.stateDir, "NGFW_METRICS_PORT="+s.metricsPort, "NGFW_SOCKET_GROUP=root", "NGFW_LOG_LEVEL=info",
		"NGFW_OBJECTS_DNS_SERVERS=127.0.0.1:9", // no FQDN object here; never the host's resolver
		"NGFW_HOST_ACL_NETNS="+hostNS)
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })

	st.adminPW = secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "NGFW_HTTP_PORT="+s.httpPort, "NGFW_HTTP_HOST=127.0.0.1",
		"NGFW_PG_DSN="+pg["NGFW_PG_DSN"], "NGFW_VALKEY_DB="+s.valkeyDB, "NGFW_VALKEY_PREFIX=ngfw:"+s.prefix+":hacl:"+secret()[:6]+":",
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_AGENT_OWNER="+s.prefix, "NGFW_AGENT_TIMEOUT_MS=60000",
		"NGFW_JWT_SECRET="+secret()+secret(), "NGFW_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"NGFW_BOOTSTRAP_ADMIN_PASSWORD="+st.adminPW, "NGFW_COOKIE_SECURE=0", "NGFW_LOG_LEVEL=warn")
	st.apiProc = start(t, "ngfw-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(90*time.Second, func() bool {
		return st.apiProc.exited() || st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("ngfw-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", st.adminPW)
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

// retrieveACL asks the agent itself (ngfw-agentctl retrieve, the gRPC Retrieve RPC) for the acl domain (protobuf JSON).
func (st *stack) retrieveACL(t *testing.T) map[string]any {
	t.Helper()
	out, err := run(st.ctlBin, "-s", st.s.socket, "retrieve", "-subsystems", "acl")
	if err != nil {
		t.Fatalf("ngfw-agentctl retrieve: %v\n%s", err, out)
	}
	var r struct {
		DesiredState struct {
			Acl map[string]any `json:"acl"`
		} `json:"desiredState"`
		Subsystems []string `json:"subsystems"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("retrieve output: %v\n%s", err, out)
	}
	if strings.Join(r.Subsystems, ",") != "acl" {
		t.Fatalf("retrieve subsystems %v", r.Subsystems)
	}
	if r.DesiredState.Acl == nil {
		return map[string]any{}
	}
	return r.DesiredState.Acl
}

// agentLogLines returns the agent log lines (from byte offset from) whose message contains one of the needles.
func (st *stack) agentLogLines(t *testing.T, from int64, needles ...string) []string {
	t.Helper()
	f, err := os.Open(st.agentLog)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		for _, n := range needles {
			if strings.Contains(sc.Text(), n) {
				out = append(out, sc.Text())
				break
			}
		}
	}
	return out
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// ---- API client ----------------------------------------------------------------------------------

type api struct {
	t     *testing.T
	base  string
	token string
}

type resp struct {
	status int
	body   map[string]any
	raw    string
}

func (a *api) call(method, path string, body any, headers ...string) resp {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.base+path, rd)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if a.token != "" {
		req.Header.Set("authorization", "Bearer "+a.token)
	}
	c := &http.Client{Timeout: 120 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return resp{raw: err.Error()}
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	r := resp{status: res.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &r.body)
	return r
}

func (a *api) must(want int, method, path string, body any, headers ...string) resp {
	a.t.Helper()
	r := a.call(method, path, body, headers...)
	if r.status != want {
		a.t.Fatalf("%s %s → %d, want %d: %s", method, path, r.status, want, r.raw)
	}
	return r
}

func (a *api) login(user, pw string) {
	a.t.Helper()
	r := a.must(200, "POST", "/api/v1/auth/login", map[string]string{"username": user, "password": pw})
	tok, _ := r.body["accessToken"].(string)
	if tok == "" {
		a.t.Fatalf("login: no accessToken in %s", r.raw)
	}
	a.token = tok
}

// patch is an RFC 7386 merge patch at a config pointer.
func (a *api) patch(path string, body any) resp {
	a.t.Helper()
	return a.must(200, "PATCH", "/api/v1/config"+path, body, "content-type", "application/merge-patch+json")
}

// commit commits the candidate and returns the response body (acl is implemented by this build: "applied").
func (a *api) commit(comment string) map[string]any {
	a.t.Helper()
	r := a.must(200, "POST", "/api/v1/config/commit?comment="+comment, nil)
	if r.body["status"] != "applied" {
		a.t.Fatalf("commit %s: %s", comment, r.raw)
	}
	for _, res := range r.body["results"].([]any) {
		if m := res.(map[string]any); m["code"] != "ok" {
			a.t.Fatalf("commit %s: object result %v", comment, m)
		}
	}
	return r.body
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

// readEnvFile reads KEY=VALUE lines (pg-test.sh's pg.env).
func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // the slot's own env file
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, l := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
