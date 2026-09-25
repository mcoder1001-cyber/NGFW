package ucs

// F-unbound-chrony-syslog end to end on a test slot (shared-host rules: slot-local instances only, loopback only,
// every process started here is stopped by PID, the host's chrony.service / rsyslog.service / /dev/log are never
// touched):
//
//	real vrx-agent (owner = slot prefix, VRX_GLOBALS_OWNER=0 → renders into /run/vrx-test/<prefix>/…)
//	real vrx-api (slot port, slot database) — configuration through the generic pointer routes, commit, rollback
//	slot daemon instances started here on the agent's renderings, as the product's systemd would:
//	  unbound -d -c <slot>/unbound/unbound.conf                       (listens on 127.10.0.53:3<N>53)
//	  chronyd -f <slot>/chrony/agent/chrony.conf -n -x                (client of the slot NTP server below)
//	  chronyd -f <slot>/chrony/server/chrony.conf -n -x               (local stratum 10 on 127.0.0.1:3<N>23)
//	  rsyslogd -n -f <slot>/rsyslog/rsyslog.conf -i <slot>/rsyslog/rsyslogd.pid  (imuxsock on <slot>/rsyslog/log.sock)
//	  a TCP syslog collector on 127.0.0.1:3<N>16 (this test)
//
// Evidence: a Go resolver pinned to the slot address answers the local record; `chronyc -h <slot sock> sources`
// lists the server; `logger -u <slot log.sock>` reaches the collector; the state routes; agent restart with the
// rendered files deleted re-renders them and keeps the restart request pending until unbound restarted; rollback to
// the previous revision removes the record from list_local_data; forwarder = listen address → 400 with a pointer.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	unboundBin  = "/usr/sbin/unbound"
	chronydBin  = "/usr/sbin/chronyd"
	chronycBin  = "/usr/bin/chronyc"
	rsyslogdBin = "/usr/sbin/rsyslogd"
	loggerBin   = "/usr/bin/logger"
)

// collector is a TCP syslog collector (octet-counted or line framing; it only looks for substrings).
type collector struct {
	mu   sync.Mutex
	data strings.Builder
	l    net.Listener
}

func startCollector(t *testing.T, addr string) *collector {
	t.Helper()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("collector %s: %v", addr, err)
	}
	c := &collector{l: l}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				r := bufio.NewReader(conn)
				buf := make([]byte, 4096)
				for {
					n, err := r.Read(buf)
					if n > 0 {
						c.mu.Lock()
						c.data.Write(buf[:n])
						c.mu.Unlock()
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	return c
}

func (c *collector) has(s string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	all := c.data.String()
	for _, l := range strings.Split(all, "\n") {
		if strings.Contains(l, s) {
			return l, true
		}
	}
	return "", strings.Contains(all, s)
}

func resolverAt(addr string) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}}
}

func lookup(addr, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := resolverAt(addr).LookupHost(ctx, name)
	slices.Sort(ips)
	return ips, err
}

type stack struct {
	s        slot
	agentBin string
	agentEnv []string
	agentLog string
	agent    *proc
	apiProc  *proc
	api      *api
	adminPW  string
	work     string
}

func (st *stack) startAgent(t *testing.T) {
	t.Helper()
	st.agent = start(t, "vrx-agent", st.agentLog, st.agentEnv, st.agentBin)
	if !waitFor(30*time.Second, func() bool {
		c, err := net.Dial("unix", st.s.socket)
		if err == nil {
			_ = c.Close()
		}
		return err == nil || st.agent.exited()
	}) || st.agent.exited() {
		raw, _ := os.ReadFile(st.agentLog) //nolint:gosec // our own log
		t.Fatalf("vrx-agent did not come up:\n%s", raw)
	}
}

func newStack(t *testing.T, s slot) *stack {
	t.Helper()
	bin := os.Getenv("VRX_UCS_AGENT_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "vrx-agent")
		if out, err := run("go", "build", "-C", filepath.Join(s.repo, "apps", "agent"), "-o", bin, "./cmd/vrx-agent"); err != nil {
			t.Fatalf("go build vrx-agent: %v\n%s", err, out)
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
	work := filepath.Join(s.runDir, "ucs")
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
		"VRX_AGENT_SOCKET="+s.socket, "VRX_OWNER="+s.prefix, "VRX_GLOBALS_OWNER=0", // D-071: a slot never owns globals
		"VRX_AGENT_STATE_DIR="+filepath.Join(work, "agent-state"), "VRX_METRICS_PORT="+s.metricsPort,
		"VRX_VPP_TABLE_BASE="+strconv.Itoa(1000*s.num), "VRX_SOCKET_GROUP=root", "VRX_LOG_LEVEL=info")
	st.startAgent(t)
	t.Cleanup(func() { st.agent.stop(t) })
	st.adminPW = secret()
	apiEnv := append(append([]string{}, base...),
		"NODE_ENV=production", "VRX_HTTP_PORT="+s.httpPort, "VRX_HTTP_HOST=127.0.0.1",
		"VRX_PG_DSN="+pg["VRX_PG_DSN"], "VRX_VALKEY_DB="+s.valkeyDB, "VRX_VALKEY_PREFIX=vrx:"+s.prefix+":ucs:"+secret()[:6]+":",
		"VRX_AGENT_SOCKET="+s.socket, "VRX_AGENT_OWNER="+s.prefix, "VRX_AGENT_TIMEOUT_MS=60000",
		"VRX_JWT_SECRET="+secret()+secret(), "VRX_SECRET_KEY_FILE="+filepath.Join(work, "secret.key"),
		"VRX_BOOTSTRAP_ADMIN_PASSWORD="+st.adminPW, "VRX_COOKIE_SECURE=0", "VRX_LOG_LEVEL=warn")
	st.apiProc = start(t, "vrx-api", filepath.Join(work, "api.log"), apiEnv, node, apiMain)
	t.Cleanup(func() { st.apiProc.stop(t) })
	st.api = &api{t: t, base: "http://127.0.0.1:" + s.httpPort}
	if !waitFor(60*time.Second, func() bool {
		return st.apiProc.exited() || st.api.call("GET", "/api/v1/health", nil).status == 200
	}) || st.apiProc.exited() {
		raw, _ := os.ReadFile(filepath.Join(work, "api.log")) //nolint:gosec // our own log
		t.Fatalf("vrx-api did not come up on %s:\n%s", s.httpPort, raw)
	}
	st.api.login("admin", st.adminPW)
	return st
}

func nRestarts(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustRun(t, "systemctl", "show", "vpp", "-p", "NRestarts"))
}

func hostUnits(t *testing.T) string {
	t.Helper()
	out, _ := run("systemctl", "show", "chrony", "rsyslog", "unbound", "-p", "Id,ActiveState,UnitFileState,MainPID")
	return strings.Join(strings.Fields(out), " ")
}

// chronyServer writes and starts the slot's NTP server instance (the source of the agent's chrony client): local
// stratum 10 on 127.0.0.1:3<N>23, loopback only, never the clock (-x). Its own directory is _chrony:_chrony 0750
// (chronyd refuses a command-socket directory it does not own).
func chronyServer(t *testing.T, s slot, logDir string) *proc {
	t.Helper()
	dir := filepath.Join(s.runDir, "chrony", "server")
	u, err := user.Lookup("_chrony")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	_ = os.Chown(dir, uid, gid)
	_ = os.Chmod(dir, 0o750) //nolint:gosec // _chrony traverses it
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	conf := filepath.Join(dir, "chrony.conf")
	body := fmt.Sprintf("bindaddress 127.0.0.1\nport %d\nbindcmdaddress %s/chronyd.sock\ncmdport 0\ndriftfile %s/chrony.drift\npidfile %s/chronyd.pid\nallow 127.0.0.0/8\nlocal stratum 10\n",
		s.port(23), dir, dir, dir)
	if err := os.WriteFile(conf, []byte(body), 0o644); err != nil { //nolint:gosec // test config, no secret
		t.Fatal(err)
	}
	return start(t, "chronyd(server)", filepath.Join(logDir, "chronyd-server.out"), nil, chronydBin, "-f", conf, "-n", "-x", "-l", filepath.Join(dir, "chronyd.log"))
}

func TestUnboundChronySyslog(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("F-unbound-chrony-syslog topology test: set VRX_INTEGRATION=1 (run.sh does)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root (the chrony instance directories belong to _chrony)")
	}
	for _, b := range []string{unboundBin, chronydBin, chronycBin, rsyslogdBin, loggerBin} {
		if _, err := os.Stat(b); err != nil {
			t.Skipf("%s missing", b)
		}
	}
	s := slotFromEnv(t)
	sharedLock(t)
	t.Logf("systemctl show vpp -p NRestarts (before): %s", nRestarts(t))
	units0 := hostUnits(t)
	t.Logf("host units (before): %s", units0)
	t.Cleanup(func() {
		t.Logf("systemctl show vpp -p NRestarts (after): %s", nRestarts(t))
		if u := hostUnits(t); u != units0 {
			t.Errorf("the host's chrony/rsyslog/unbound units changed:\n before %s\n after  %s", units0, u)
		} else {
			t.Logf("host units (after, unchanged): %s", u)
		}
	})

	slotDir := s.runDir
	ubConf := filepath.Join(slotDir, "unbound", "unbound.conf")
	chConf := filepath.Join(slotDir, "chrony", "agent", "chrony.conf")
	chSrc := filepath.Join(slotDir, "chrony", "agent", "sources.d", "vrx.sources")
	chSock := filepath.Join(slotDir, "chrony", "agent", "chronyd.sock")
	rsDir := filepath.Join(slotDir, "rsyslog")
	rsConf := filepath.Join(rsDir, "rsyslog.conf")
	t.Cleanup(func() { // the agent's slot renderings (our own slot directory only)
		for _, d := range []string{"unbound", "chrony/agent", "rsyslog"} {
			_ = os.RemoveAll(filepath.Join(slotDir, d))
		}
		_ = os.Remove(filepath.Join(slotDir, "chrony"))
	})

	st := newStack(t, s)
	a := st.api
	lo := fmt.Sprintf("loop%d53", s.num) // the slot's loopback (shared-host rules: loop<N>xx)
	listenIP := fmt.Sprintf("127.%d.0.53", s.num)
	dnsPort, dnsPort2 := s.port(53), s.port(54)
	collectorAddr := fmt.Sprintf("127.0.0.1:%d", s.port(16))
	coll := startCollector(t, collectorAddr)

	resolver := func(port int, extra ...map[string]any) map[string]any {
		recs := []map[string]any{{"name": "gw.lab.example.", "type": "A", "data": fmt.Sprintf("10.%d.53.1", s.num)}}
		for _, e := range extra {
			recs = append(recs, e)
		}
		return map[string]any{
			"description":  "slot resolver (F-unbound-chrony-syslog)",
			"listen":       []map[string]any{{"address": listenIP, "port": port}},
			"forwardZones": []map[string]any{{"zone": "corp.example.", "forwarders": []map[string]any{{"address": fmt.Sprintf("10.%d.99.53", s.num)}}}},
			"localZones":   []map[string]any{{"zone": "lab.example.", "records": recs}},
		}
	}

	// ---- configure through the pointer routes and commit ------------------------------------------------------
	a.must(200, "PUT", "/api/v1/config/interfaces/"+lo, map[string]any{"enabled": true, "description": "slot resolver address", "ipv4": []string{listenIP + "/32"}})
	a.must(200, "PUT", "/api/v1/config/services/dns/resolvers/lab", resolver(dnsPort))
	a.must(200, "PUT", "/api/v1/config/services/ntp", map[string]any{"enabled": true, "servers": []map[string]any{{"address": "127.0.0.1", "minPoll": -2, "maxPoll": 0}}, "port": 0})
	a.must(200, "PUT", "/api/v1/config/management/syslog", []map[string]any{{
		"address": "127.0.0.1", "port": s.port(16), "protocol": "tcp", "severity": "info", "facilities": []string{"local7"}, "queueSize": 1000,
	}})
	c1 := a.commit("ucs-1")
	t.Logf("commit ucs-1: revision %v, status %v", c1["revision"], c1["status"])
	for _, f := range []string{ubConf, chConf, chSrc, rsConf} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("not rendered: %v", err)
		}
	}
	dns := a.must(200, "GET", "/api/v1/state/dns", nil).body
	t.Logf("GET /state/dns before any daemon runs: running=%v pendingActions=%s", dns["running"], jsonOf(dns["pendingActions"]))

	// ---- forwarder equal to a listen address → 400 problem+json with the pointer -----------------------------
	bad := resolver(dnsPort)
	bad["forwarders"] = []map[string]any{{"address": listenIP, "port": dnsPort}}
	r400 := a.call("PUT", "/api/v1/config/services/dns/resolvers/lab", bad)
	t.Logf("PUT resolver with forwarder = listen address → %d %s", r400.status, r400.raw)
	if r400.status != 400 || !strings.Contains(r400.raw, `"pointer":"/services/dns/resolvers/lab/forwarders/0"`) {
		t.Fatalf("want 400 with the forwarder pointer, got %d %s", r400.status, r400.raw)
	}

	// ---- start the slot instances on the agent's renderings (what systemd does in the product) ----------------
	logDir := st.work
	startUnbound := func() *proc {
		p := start(t, "unbound", filepath.Join(logDir, "unbound.out"), nil, unboundBin, "-d", "-c", ubConf)
		t.Cleanup(func() { p.stop(t) })
		return p
	}
	ub := startUnbound()
	srv := chronyServer(t, s, logDir)
	t.Cleanup(func() { srv.stop(t) })
	ch := start(t, "chronyd(agent)", filepath.Join(logDir, "chronyd-agent.out"), nil, chronydBin, "-f", chConf, "-n", "-x", "-l", filepath.Join(slotDir, "chrony", "agent", "log", "chronyd.log"))
	t.Cleanup(func() { ch.stop(t) })
	startRsyslog := func() *proc {
		p := start(t, "rsyslogd", filepath.Join(logDir, "rsyslogd.out"), nil, rsyslogdBin, "-n", "-f", rsConf, "-i", filepath.Join(rsDir, "rsyslogd.pid"))
		t.Cleanup(func() { p.stop(t) })
		return p
	}
	rs := startRsyslog()

	// ---- unbound answers the local record at the slot address (Go resolver; no dig on the host) --------------
	addr := net.JoinHostPort(listenIP, strconv.Itoa(dnsPort))
	var ips []string
	if !waitFor(30*time.Second, func() bool {
		var err error
		ips, err = lookup(addr, "gw.lab.example")
		return err == nil
	}) {
		t.Fatalf("unbound on %s does not answer", addr)
	}
	t.Logf("net.Resolver{%s}.LookupHost(gw.lab.example) = %v", addr, ips)
	if !slices.Contains(ips, fmt.Sprintf("10.%d.53.1", s.num)) {
		t.Fatalf("wrong answer %v", ips)
	}

	// ---- chronyc -h <slot sock> sources lists (and selects) the server ---------------------------------------
	var sources string
	if !waitFor(60*time.Second, func() bool {
		sources, _ = run(chronycBin, "-h", chSock, "sources")
		return strings.Contains(sources, "^*")
	}) {
		t.Logf("chronyc sources (not selected yet):\n%s", sources)
	}
	t.Logf("$ chronyc -h %s sources\n%s", chSock, sources)
	if !strings.Contains(sources, "127.0.0.1") {
		t.Fatalf("the server is not listed:\n%s", sources)
	}

	// ---- rsyslog forwards a logger line from the slot socket to the slot collector ---------------------------
	nonce := "ucs-" + secret()[:8]
	sock := filepath.Join(rsDir, "log.sock")
	if !waitFor(20*time.Second, func() bool { _, err := os.Stat(sock); return err == nil }) {
		t.Fatalf("rsyslogd did not create %s", sock)
	}
	t.Log(mustRun(t, loggerBin, "-u", sock, "-p", "local7.notice", "-t", "vrx-ucs-"+s.prefix, "hello from "+s.prefix+" "+nonce))
	var got string
	if !waitFor(20*time.Second, func() bool { var ok bool; got, ok = coll.has(nonce); return ok }) {
		t.Fatalf("the collector did not receive %s", nonce)
	}
	t.Logf("$ logger -u %s -p local7.notice -t vrx-ucs-%s 'hello from %s %s'\ncollector %s received: %q", sock, s.prefix, s.prefix, nonce, collectorAddr, got)
	_, _ = run(loggerBin, "-u", sock, "-p", "daemon.info", "-t", "vrx-ucs-"+s.prefix, "filtered out "+nonce+"-daemon")
	time.Sleep(2 * time.Second)
	if _, leaked := coll.has(nonce + "-daemon"); leaked {
		t.Fatal("the facility filter let daemon.info through")
	}

	// ---- the state routes over the running instances ------------------------------------------------------------
	waitFor(20*time.Second, func() bool {
		b := a.must(200, "GET", "/api/v1/state/syslog", nil).body
		ts, _ := b["targets"].([]any)
		if len(ts) != 1 {
			return false
		}
		t0, _ := ts[0].(map[string]any)
		n, _ := t0["processed"].(float64)
		return t0["reported"] == true && n >= 1
	})
	for _, p := range []string{"/api/v1/state/dns", "/api/v1/state/ntp", "/api/v1/state/syslog", "/api/v1/state/logs?severity=notice&pageSize=3"} {
		b := a.must(200, "GET", p, nil).body
		delete(b, "stats")
		delete(b, "serverStats")
		delete(b, "status")
		delete(b, "sourceStats")
		t.Logf("GET %s →\n%s", p, jsonOf(b))
	}
	lk := a.must(200, "POST", "/api/v1/actions/dns-lookup", map[string]any{"name": "gw.lab.example", "timeoutMs": 2000})
	t.Logf("POST /api/v1/actions/dns-lookup (real agent; VPP's dns plugin is not enabled on the shared VPP — only the globals owner enables it) → %s", lk.raw)

	// ---- a listen-port change needs a restart (D-079) → pending; agent restart with the renderings deleted -----
	a.must(200, "PUT", "/api/v1/config/services/dns/resolvers/lab", resolver(dnsPort2))
	c2 := a.commit("ucs-2-port")
	rev2 := c2["revision"]
	pend := a.must(200, "GET", "/api/v1/state/dns", nil).body["pendingActions"]
	t.Logf("after the listen-port commit (revision %v): pendingActions=%s", rev2, jsonOf(pend))
	if !strings.Contains(jsonOf(pend), `"restart"`) {
		t.Fatalf("no pending restart: %s", jsonOf(pend))
	}
	st.agent.stop(t)
	for _, f := range []string{ubConf, chSrc, rsConf} {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("agent stopped; deleted %s, %s, %s (simulated loss)", ubConf, chSrc, rsConf)
	logFrom := fileSize(st.agentLog)
	t0 := time.Now()
	st.startAgent(t)
	if !waitFor(30*time.Second, func() bool {
		for _, f := range []string{ubConf, chSrc, rsConf} {
			if _, err := os.Stat(f); err != nil {
				return false
			}
		}
		return true
	}) {
		t.Fatal("the restarted agent did not re-render the files within 30 s")
	}
	t.Logf("files re-rendered %.2f s after the agent start", time.Since(t0).Seconds())
	time.Sleep(time.Second)
	t.Log("agent log excerpt after the restart:\n" + logExcerpt(t, st.agentLog, logFrom, "reconcile", "configuration written", "applied", "host services", "resync"))
	pend = a.must(200, "GET", "/api/v1/state/dns", nil).body["pendingActions"]
	t.Logf("after the agent restart: pendingActions=%s", jsonOf(pend))
	if !strings.Contains(jsonOf(pend), `"restart"`) {
		t.Fatalf("the restart request did not survive the agent restart: %s", jsonOf(pend))
	}
	if b, _ := os.ReadFile(ubConf); !strings.Contains(string(b), fmt.Sprintf("interface: %s@%d", listenIP, dnsPort2)) { //nolint:gosec // slot file
		t.Fatalf("re-rendered unbound.conf does not listen on %d", dnsPort2)
	}
	// act on it: restart unbound (the product's systemd would) → cleared, and the new port answers
	ub.stop(t)
	ub = startUnbound()
	addr2 := net.JoinHostPort(listenIP, strconv.Itoa(dnsPort2))
	if !waitFor(30*time.Second, func() bool { _, err := lookup(addr2, "gw.lab.example"); return err == nil }) {
		t.Fatalf("unbound does not answer on %s after the restart", addr2)
	}
	rs.stop(t)
	rs = startRsyslog()
	waitFor(15*time.Second, func() bool {
		d, _ := a.must(200, "GET", "/api/v1/state/dns", nil).body["pendingActions"].([]any)
		y, _ := a.must(200, "GET", "/api/v1/state/syslog", nil).body["pendingActions"].([]any)
		return len(d) == 0 && len(y) == 0
	})
	t.Logf("after restarting unbound and rsyslogd: dns pendingActions=%s syslog pendingActions=%s",
		jsonOf(a.must(200, "GET", "/api/v1/state/dns", nil).body["pendingActions"]),
		jsonOf(a.must(200, "GET", "/api/v1/state/syslog", nil).body["pendingActions"]))

	// ---- rollback restores the previous rendering (list_local_data, Retrieve via /state/drift) ---------------
	a.must(200, "PUT", "/api/v1/config/services/dns/resolvers/lab", resolver(dnsPort2, map[string]any{"name": "new.lab.example.", "type": "A", "data": fmt.Sprintf("10.%d.53.2", s.num)}))
	c3 := a.commit("ucs-3-record")
	ld := jsonOf(a.must(200, "GET", "/api/v1/state/dns", nil).body["localData"])
	ips3, _ := lookup(addr2, "new.lab.example")
	t.Logf("revision %v: list_local_data=%s; new.lab.example → %v", c3["revision"], ld, ips3)
	if !strings.Contains(ld, "new.lab.example.") {
		t.Fatal("the new record is not served")
	}
	rb := a.must(200, "POST", fmt.Sprintf("/api/v1/config/rollback/%v", rev2), nil)
	t.Logf("POST /api/v1/config/rollback/%v → status %v, revision %v", rev2, rb.body["status"], rb.body["revision"])
	ld = jsonOf(a.must(200, "GET", "/api/v1/state/dns", nil).body["localData"])
	_, err := lookup(addr2, "new.lab.example")
	t.Logf("after rollback: list_local_data=%s; new.lab.example → %v", ld, err)
	if strings.Contains(ld, "new.lab.example.") {
		t.Fatal("rollback left the record in unbound")
	}
	drift := a.must(200, "GET", "/api/v1/state/drift", nil).body
	t.Logf("GET /state/drift after rollback (Retrieve vs running): %s", jsonOf(map[string]any{"subsystems": drift["subsystems"], "changes": drift["changes"]}))
	if ch, _ := drift["changes"].([]any); len(ch) != 0 {
		for _, c := range ch {
			if p, _ := c.(map[string]any)["pointer"].(string); strings.HasPrefix(p, "/services") || strings.HasPrefix(p, "/management") {
				t.Errorf("drift after rollback: %v", c)
			}
		}
	}

	// ---- screenshots (optional evidence run) -------------------------------------------------------------------
	if script, out := os.Getenv("VRX_UCS_SHOTS"), os.Getenv("VRX_UCS_SHOTS_OUT"); script != "" && out != "" {
		shots(t, st, script, out)
	}

	// ---- leave nothing behind: remove the configuration (idle / disabled / empty renderings), stop everything --
	a.must(200, "PUT", "/api/v1/config/services/dns/resolvers", map[string]any{})
	a.must(200, "PUT", "/api/v1/config/services/ntp", map[string]any{"enabled": false})
	a.must(200, "PUT", "/api/v1/config/management/syslog", []any{})
	a.must(200, "DELETE", "/api/v1/config/interfaces/"+lo, nil)
	a.commit("ucs-cleanup")
	b, _ := os.ReadFile(ubConf) //nolint:gosec // slot file
	t.Logf("after the cleanup commit unbound.conf is idle: %v", strings.Contains(string(b), "# no enabled resolver"))
	for _, p := range []*proc{rs, ub, ch} {
		p.stop(t)
	}
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// logExcerpt returns the agent log lines after offset that contain one of the words.
func logExcerpt(t *testing.T, path string, offset int64, words ...string) string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // our own log
	if err != nil {
		return err.Error()
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, 0); err != nil {
		return err.Error()
	}
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		for _, w := range words {
			if strings.Contains(l, w) {
				if len(l) > 400 {
					l = l[:400] + "…"
				}
				out = append(out, l)
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

// shots runs the external screenshot script against vite preview of the production build (kept outside the repo,
// like P07a/P07b/P08): node <script> <baseUrl> <outDir> <adminPasswordFile>.
func shots(t *testing.T, st *stack, script, out string) {
	t.Helper()
	web := filepath.Join(st.s.repo, "apps", "web")
	env := append(os.Environ(), "VRX_HTTP_PORT="+st.s.httpPort, "VRX_WEB_PORT="+st.s.webPort)
	pv := start(t, "vite-preview", filepath.Join(st.work, "vite.log"), env, filepath.Join(web, "node_modules", ".bin", "vite"), "preview", web)
	defer pv.stop(t)
	pwFile := filepath.Join(st.work, "admin.pw")
	if err := os.WriteFile(pwFile, []byte(st.adminPW), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(pwFile) }()
	if !waitFor(30*time.Second, func() bool {
		o, err := run("curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:"+st.s.webPort+"/")
		return err == nil && o == ""
	}) {
		t.Fatal("vite preview did not come up")
	}
	// a pending (uncommitted) change so the pending-change bar shows the diff
	st.api.must(200, "PATCH", "/api/v1/config/services/ntp", map[string]any{"pools": []string{"pool.ntp.org"}}, "content-type", "application/merge-patch+json")
	outp, err := run("node", script, "http://127.0.0.1:"+st.s.webPort, out, pwFile)
	t.Log("screenshots:\n" + strings.TrimSpace(outp))
	if err != nil {
		t.Errorf("screenshot script: %v", err)
	}
	st.api.must(200, "POST", "/api/v1/config/discard", nil)
}
