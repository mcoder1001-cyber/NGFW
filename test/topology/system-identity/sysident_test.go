package sysid

// F-system-identity end to end on a test slot (shared-host rules: the host's hostname, time zone, /etc/issue,
// /etc/motd and resolver are never touched — docs/lab/shared-host-rules.md):
//
//	real ngfw-agent (owner = slot prefix, NGFW_GLOBALS_OWNER=0 → renders into /run/ngfw-test/<prefix>/sysident/etc/…,
//	               never calls sethostname)
//	real ngfw-api (slot port, slot database) — `system` through the generic pointer routes, commit
//
// Evidence: the rendered files under the rig root match testdata/*.golden and /etc/localtime points at the zone;
// an unknown zone and a banner with an escape sequence are refused with 400 problem+json naming the pointer; an agent
// restart with nothing changed rewrites nothing (mtimes and the agent log); the host's own identity is unchanged.

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type stack struct {
	s        slot
	agentBin string
	agentEnv []string
	agentLog string
	agent    *proc
	apiProc  *proc
	api      *api
	work     string
}

func (st *stack) startAgent(t *testing.T) {
	t.Helper()
	st.agent = start(t, "ngfw-agent", st.agentLog, st.agentEnv, st.agentBin)
	if !waitFor(30*time.Second, func() bool {
		c, err := net.Dial("unix", st.s.socket)
		if err == nil {
			_ = c.Close()
		}
		return err == nil || st.agent.exited()
	}) || st.agent.exited() {
		raw, _ := os.ReadFile(st.agentLog) //nolint:gosec // our own log
		t.Fatalf("ngfw-agent did not come up:\n%s", raw)
	}
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
	bin := os.Getenv("NGFW_SYSID_AGENT_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "ngfw-agent")
		if out, err := run("go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/ngfw-agent"); err != nil {
			t.Fatalf("go build ngfw-agent: %v\n%s", err, out)
		}
	}
	apiMain := filepath.Join(s.repo, "apps", "api", "dist", "main.js")
	if _, err := os.Stat(apiMain); err != nil {
		t.Fatalf("%s missing (run.sh builds it): %v", apiMain, err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node not in PATH")
	}
	if err := mkdirShared(s.runDir); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(s.runDir, "sysid")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &stack{s: s, agentBin: bin, agentLog: filepath.Join(work, "agent.log"), work: work}
	t.Log(mustRun(t, filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "create", s.prefix))
	t.Cleanup(func() {
		out, err := run(filepath.Join(s.repo, "deploy", "dev", "pg-test.sh"), "drop", s.prefix)
		t.Logf("pg-test drop %s: %v\n%s", s.prefix, err, out)
	})
	pg := readEnvFile(t, filepath.Join(s.runDir, "pg.env"))
	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	st.agentEnv = append(append([]string{}, base...),
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_OWNER="+s.prefix, "NGFW_GLOBALS_OWNER=0", // D-071: a slot never owns globals
		"NGFW_AGENT_STATE_DIR="+filepath.Join(work, "agent-state"), "NGFW_METRICS_PORT="+s.metricsPort,
		"NGFW_VPP_TABLE_BASE="+strconv.Itoa(1000*s.num), "NGFW_SOCKET_GROUP=root", "NGFW_LOG_LEVEL=debug")
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })
	adminPW := secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "NGFW_HTTP_PORT="+s.httpPort, "NGFW_HTTP_HOST=127.0.0.1",
		"NGFW_PG_DSN="+pg["NGFW_PG_DSN"], "NGFW_VALKEY_DB="+s.valkeyDB, "NGFW_VALKEY_PREFIX=ngfw:"+s.prefix+":sysid:"+secret()[:6]+":",
		"NGFW_AGENT_SOCKET="+s.socket, "NGFW_AGENT_OWNER="+s.prefix, "NGFW_AGENT_TIMEOUT_MS=60000",
		"NGFW_JWT_SECRET="+secret()+secret(), "NGFW_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"NGFW_BOOTSTRAP_ADMIN_PASSWORD="+adminPW, "NGFW_COOKIE_SECURE=0", "NGFW_LOG_LEVEL=warn")
	st.apiProc = start(t, "ngfw-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(60*time.Second, func() bool {
		return st.apiProc.exited() || st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("ngfw-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", adminPW)
	return st
}

// hostIdentity is what must never change: the kernel host name and the host's own identity files.
func hostIdentity(t *testing.T) string {
	t.Helper()
	h, _ := os.Hostname()
	var b strings.Builder
	b.WriteString("kernel=" + h)
	for _, f := range []string{"/etc/hostname", "/etc/issue", "/etc/motd", "/etc/systemd/resolved.conf.d/ngfw.conf"} {
		raw, err := os.ReadFile(f) //nolint:gosec // fixed host paths, read only
		b.WriteString(" " + f + "=" + strconv.Quote(string(raw)) + "/" + strconv.FormatBool(err == nil))
	}
	lt, _ := os.Readlink("/etc/localtime")
	b.WriteString(" /etc/localtime->" + lt)
	return b.String()
}

func TestSystemIdentity(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("F-system-identity topology test: set NGFW_INTEGRATION=1 (run.sh does)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (the slot run directory)")
	}
	if _, err := os.Stat("/usr/share/zoneinfo/Asia/Tehran"); err != nil {
		t.Skip("tzdata missing")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	host0 := hostIdentity(t)
	t.Cleanup(func() {
		if h := hostIdentity(t); h != host0 {
			t.Errorf("the host's identity changed:\n before %s\n after  %s", host0, h)
		} else {
			t.Logf("host identity unchanged: %s", h)
		}
	})
	rig := filepath.Join(s.runDir, "sysident")
	t.Cleanup(func() { _ = os.RemoveAll(rig) }) // our own slot directory only

	st := newStack(t, s)
	a := st.api
	t.Cleanup(func() {
		a.call("PUT", "/api/v1/config/system", map[string]any{})
		r := a.call("POST", "/api/v1/config/commit?comment=sysid-cleanup", nil)
		t.Logf("cleanup commit → %d %v", r.status, r.body["status"])
	})

	// ---- refused at the API with a pointer --------------------------------------------------------------------
	for _, c := range []struct {
		path string
		body any
		ptr  string
	}{
		{"/api/v1/config/system/timezone", "Mars/Olympus_Mons", "/system/timezone"},
		{"/api/v1/config/system/banner", map[string]any{"login": "hi\x1b[2J\x1b[Hforged"}, "/system/banner/login"},
	} {
		r := a.call("PUT", c.path, c.body)
		t.Logf("PUT %s → %d %s", c.path, r.status, r.raw)
		if r.status != 400 || !strings.Contains(r.raw, `"pointer":"`+c.ptr) {
			t.Fatalf("PUT %s: want 400 naming %s", c.path, c.ptr)
		}
	}

	// ---- commit and compare the rendering with the goldens ---------------------------------------------------
	a.must(200, "PUT", "/api/v1/config/system", map[string]any{
		"hostname": "ngfw-sysid.lab.example",
		"timezone": "Asia/Tehran",
		"banner":   map[string]any{"login": "Authorised access only.\n\tSlot rig (F-system-identity)", "motd": "Welcome to the lab rig."},
		"dns":      map[string]any{"servers": []string{"192.0.2.53", "2001:db8::53"}, "searchDomains": []string{"lab.example"}},
	})
	c1 := a.commit("sysid-1")
	t.Logf("commit sysid-1 → %v", c1["status"])
	files := map[string]string{
		"hostname.golden": filepath.Join(rig, "etc/hostname"), "issue.golden": filepath.Join(rig, "etc/issue"),
		"motd.golden": filepath.Join(rig, "etc/motd"), "resolved-ngfw.conf.golden": filepath.Join(rig, "etc/systemd/resolved.conf.d/ngfw.conf"),
	}
	mtimes := map[string]time.Time{}
	for golden, path := range files {
		got, err := os.ReadFile(path) //nolint:gosec // the slot rig
		if err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile(filepath.Join("testdata", golden)) //nolint:gosec // testdata
		have := string(got)
		if golden == "resolved-ngfw.conf.golden" { // the embedded render input line is the agent's own bookkeeping
			var keep []string
			for _, l := range strings.Split(have, "\n") {
				if !strings.HasPrefix(l, "#") {
					keep = append(keep, l)
				}
			}
			have = strings.Join(keep, "\n")
		}
		if have != string(want) {
			t.Errorf("%s:\n got %q\nwant %q", path, have, want)
		}
		t.Logf("$ cat %s\n%s", path, got)
		fi, _ := os.Stat(path)
		mtimes[path] = fi.ModTime()
	}
	if lt, err := os.Readlink(filepath.Join(rig, "etc/localtime")); err != nil || lt != "/usr/share/zoneinfo/Asia/Tehran" {
		t.Errorf("localtime -> %q (%v)", lt, err)
	}

	// ---- agent restart: nothing changed → nothing rewritten --------------------------------------------------
	st.agent.stop(t)
	off, _ := os.Stat(st.agentLog)
	st.startAgent(t)
	time.Sleep(5 * time.Second) // the initial resync
	for path, m := range mtimes {
		if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(m) {
			t.Errorf("%s rewritten after an agent restart with nothing changed", path)
		}
	}
	raw, _ := os.ReadFile(st.agentLog) //nolint:gosec // our own log
	tail := string(raw[off.Size():])
	if strings.Contains(tail, "system identity applied") {
		t.Errorf("restart re-applied the system identity:\n%s", tail)
	}
	for _, l := range strings.Split(tail, "\n") {
		if strings.Contains(l, "system identity") || strings.Contains(l, "reconcile done") {
			t.Logf("agent log after restart: %s", l)
		}
	}
}
