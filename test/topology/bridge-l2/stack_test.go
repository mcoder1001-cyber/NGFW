package bridgel2

// The stack under test: the real vrx-agent binary (built from this tree, owner = VRX_TEST_PREFIX) and the real
// vrx-api (apps/api/dist, slot port) on a throwaway slot database, the host VPP, the af_packet veth/netns rig.
// Every process is started by this test and stopped by PID; every object carries the slot prefix.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

	"golang.org/x/net/websocket"
)

const labLock = "/run/lock/vrx-lab.lock"

type slot struct {
	prefix      string
	num         int
	httpPort    string
	metricsPort string
	valkeyDB    string
	runDir      string // /run/vrx-test/<prefix>
	socket      string
	repo        string
	lab         string
}

func slotFromEnv(t *testing.T) slot {
	t.Helper()
	p := os.Getenv("VRX_TEST_PREFIX")
	m := regexp.MustCompile(`^w([0-9]{1,2})$`).FindStringSubmatch(p)
	if m == nil {
		t.Fatalf("VRX_TEST_PREFIX=%q: this test needs a slot prefix w<N> (eval \"$(tools/lab env <N>)\")", p)
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
		httpPort:    env("VRX_HTTP_PORT", strconv.Itoa(3000+100*n)),
		metricsPort: env("VRX_METRICS_PORT", strconv.Itoa(9100+10*n+1)),
		valkeyDB:    env("VRX_VALKEY_DB", strconv.Itoa(n)),
		runDir:      "/run/vrx-test/" + p,
		repo:        repoRoot(t),
	}
	s.socket = env("VRX_AGENT_SOCKET", s.runDir+"/agent.sock")
	s.lab = filepath.Join(s.repo, "tools", "lab")
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
	f, err := os.OpenFile(labLock, os.O_RDONLY|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatalf("open %s: %v", labLock, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatalf("flock -s %s: %v", labLock, err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

// mkdirShared creates dir and any missing parent as 0755 whatever the umask (the slot run dir /run/vrx-test/<prefix>
// and /run/vrx-test must stay traversable for the frr/_chrony test daemons, D-106/D-107). An existing directory is
// never re-moded; only the test's own work dir below it is private (0700).
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
		return resp{status: 0, raw: err.Error()}
	}
	defer res.Body.Close()
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

// commit commits the candidate and returns the response body.
func (a *api) commit(comment string) map[string]any {
	a.t.Helper()
	r := a.must(200, "POST", "/api/v1/config/commit?comment="+comment, nil)
	if r.body["status"] != "applied" {
		a.t.Fatalf("commit %s: %s", comment, r.raw)
	}
	return r.body
}

// ifState returns /state/interfaces items by name.
func (a *api) ifState() (map[string]map[string]any, int) {
	a.t.Helper()
	r := a.call("GET", "/api/v1/state/interfaces", nil)
	out := map[string]map[string]any{}
	if r.status != 200 {
		return out, r.status
	}
	items, _ := r.body["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if n, ok := m["name"].(string); ok {
			out[n] = m
		}
	}
	return out, r.status
}

// wsCounters subscribes to iface.counters and returns the first batch that contains all names.
func (a *api) wsCounters(names ...string) (map[string]map[string]any, error) {
	origin := "http://127.0.0.1/"
	cfg, err := websocket.NewConfig(strings.Replace(a.base, "http", "ws", 1)+"/api/v1/stream", origin)
	if err != nil {
		return nil, err
	}
	cfg.Protocol = []string{"vrx.v1", "bearer." + a.token}
	ws, err := websocket.DialConfig(cfg)
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	if err := websocket.JSON.Send(ws, map[string]any{"subscribe": []string{"iface.counters"}}); err != nil {
		return nil, err
	}
	_ = ws.SetDeadline(time.Now().Add(15 * time.Second))
	for {
		var msg struct {
			Type  string `json:"type"`
			Topic string `json:"topic"`
			Data  struct {
				Interfaces []map[string]any `json:"interfaces"`
			} `json:"data"`
		}
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			return nil, err
		}
		if msg.Topic != "iface.counters" {
			continue
		}
		got := map[string]map[string]any{}
		for _, c := range msg.Data.Interfaces {
			if n, ok := c["name"].(string); ok {
				got[n] = c
			}
		}
		ok := true
		for _, n := range names {
			if got[n] == nil {
				ok = false
			}
		}
		if ok {
			return got, nil
		}
	}
}

// ---- small helpers -------------------------------------------------------------------------------

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

func parseKV(s string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), ": "); ok && !strings.Contains(k, " ") {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
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

// agentLog holds the JSON log lines of one agent process.
type logLine struct {
	Time    time.Time `json:"time"`
	Msg     string    `json:"msg"`
	Mode    string    `json:"mode"`
	Status  string    `json:"status"`
	Summary string    `json:"summary"`
}

func readAgentLog(t *testing.T, path string, from int64) ([]logLine, []string) {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // our own log file
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var lines []logLine
	var raw []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l logLine
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			lines = append(lines, l)
			raw = append(raw, sc.Text())
		}
	}
	return lines, raw
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

var _ = fmt.Sprintf

// defaultRouteDev is the netdev of the host's default route (the management NIC on the lab VMs), "" if none.
func defaultRouteDev(t *testing.T) string {
	t.Helper()
	out, err := run(t, "ip", "-o", "route", "show", "default")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "dev" {
			return f[i+1]
		}
	}
	return ""
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
	bin := os.Getenv("VRX_BL2_AGENT_BIN")
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
	work := filepath.Join(s.runDir, "bl2")
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
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":bl2:"+secret()[:6]+":",
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
