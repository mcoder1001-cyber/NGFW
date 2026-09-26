package ucs

// Harness of the F-unbound-chrony-syslog topology test: the slot, child processes stopped by PID, a small API client.
// (The same shape as test/topology/interfaces — P08 — kept in this module so the two tests stay independent.)

import (
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
)

const labLock = "/run/lock/vrx-lab.lock"

type slot struct {
	prefix, httpPort, webPort, metricsPort, valkeyDB, runDir, socket, repo string
	num                                                                    int
}

func slotFromEnv(t *testing.T) slot {
	t.Helper()
	p := os.Getenv("VRX_TEST_PREFIX")
	m := regexp.MustCompile(`^w([0-9]{1,2})$`).FindStringSubmatch(p)
	if m == nil {
		t.Fatalf("VRX_TEST_PREFIX=%q: needs a slot prefix w<N> (eval \"$(tools/lab env <N>)\")", p)
	}
	n, _ := strconv.Atoi(m[1])
	if n < 1 || n > 12 { // review L4: w0/w00 would take the product stack's ports (3000, 5000, 9101)
		t.Fatalf("VRX_TEST_PREFIX=%q: slot %d outside 1..12", p, n)
	}
	env := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	s := slot{
		prefix: p, num: n,
		httpPort:    env("VRX_HTTP_PORT", strconv.Itoa(3000+100*n)),
		webPort:     env("VRX_WEB_PORT", strconv.Itoa(5000+100*n)),
		metricsPort: env("VRX_METRICS_PORT", strconv.Itoa(9100+10*n+1)),
		valkeyDB:    env("VRX_VALKEY_DB", strconv.Itoa(n)),
		runDir:      "/run/vrx-test/" + p,
		repo:        repoRoot(t),
	}
	s.socket = env("VRX_AGENT_SOCKET", s.runDir+"/agent.sock")
	return s
}

// port returns the slot port 3<slot><suffix> (3<N>53 unbound, 3<N>23 NTP server, 3<N>16 syslog collector …).
func (s slot) port(suffix int) int { return 3000 + 100*s.num + suffix }

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

func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // fixed test argv
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
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // our own log
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // fixed test argv
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		t.Fatalf("start %s: %v", name, err)
	}
	p := &proc{name: name, cmd: cmd, log: logPath, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = f.Close(); close(p.done) }()
	t.Logf("started %s pid %d: %s %s", name, cmd.Process.Pid, bin, strings.Join(args, " "))
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

func waitFor(timeout time.Duration, f func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return f()
}

// mkdirShared creates dir (and missing parents) 0755 without re-moding an existing one (D-106/D-107).
func mkdirShared(dir string) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		return nil
	}
	if err := mkdirShared(filepath.Dir(dir)); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) { //nolint:gosec // traversable run dir
		return err
	}
	return os.Chmod(dir, 0o755) //nolint:gosec // traversable run dir
}

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

// ---- API client ------------------------------------------------------------------------------------------------

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
	req, _ := http.NewRequest(method, a.base+path, rd) //nolint:noctx // test client
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if a.token != "" {
		req.Header.Set("authorization", "Bearer "+a.token)
	}
	res, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
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

// commit commits the candidate and returns the body (status must be "applied").
func (a *api) commit(comment string) map[string]any {
	a.t.Helper()
	r := a.must(200, "POST", "/api/v1/config/commit?comment="+comment, nil)
	if r.body["status"] != "applied" {
		a.t.Fatalf("commit %s: %s", comment, r.raw)
	}
	return r.body
}

func jsonOf(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
