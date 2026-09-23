package rsyslog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestRsyslogIntegration runs a test-scoped rsyslogd as a child (never the host's rsyslog
// unit, never /etc/rsyslog*, never /dev/log): a standalone config under
// /run/vrx-test/<prefix>/rsyslog with imuxsock on <dir>/log.sock and imtcp on
// 127.0.0.1:3<N>14, exporting to Go collectors on 127.0.0.1:3<N>15 (UDP) and 3<N>16 (TCP).
func TestRsyslogIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	port := func(suffix string) uint32 {
		n, _ := strconv.Atoi(fmt.Sprintf("3%d%s", slot, suffix))
		return uint32(n) //nolint:gosec // slot ports
	}
	inTCP, udpPort, tcpPort := port("14"), port("15"), port("16")
	hostPID := rsyslogUnitPID(t)
	ctx := context.Background()
	paths := TestPaths(prefix, inTCP)
	base := filepath.Dir(paths.ConfFile)
	lockSlotDir(t, base)
	if err := os.MkdirAll(paths.Standalone.WorkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	udp := udpCollector(t, udpPort)
	tcp := tcpCollector(t, tcpPort)

	var mu sync.Mutex
	var child *exec.Cmd
	start := func() error {
		mu.Lock()
		defer mu.Unlock()
		c := exec.Command(RsyslogdBin, "-n", "-iNONE", "-f", paths.ConfFile) //nolint:gosec // test harness: fixed argv of an allow-listed binary
		c.Env, c.Dir = []string{"PATH=/usr/sbin:/usr/bin"}, base
		lf, err := os.OpenFile(filepath.Join(base, "rsyslogd.out"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // test log
		if err != nil {
			return err
		}
		c.Stdout, c.Stderr = lf, lf
		if err := c.Start(); err != nil {
			return err
		}
		child = c
		return nil
	}
	stop := func() {
		mu.Lock()
		c := child
		mu.Unlock()
		if c == nil || c.ProcessState != nil {
			return
		}
		_ = c.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = c.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = c.Process.Kill()
			<-done
		}
	}
	ctl := &rfkit.ProcessController{
		Binary: RsyslogdBin,
		PID: func() (int, error) {
			mu.Lock()
			defer mu.Unlock()
			if child == nil || child.ProcessState != nil {
				return 0, rfkit.ErrNotRunning
			}
			return child.Process.Pid, nil
		},
		// rsyslog cannot reload a configuration: the test's restart = stop the child, start a new one.
		OnRestart: func(context.Context) error { stop(); return start() },
	}
	t.Cleanup(func() {
		stop()
		if left := procsUnder(base); len(left) > 0 {
			t.Errorf("processes still running with %s in their command line: %v", base, left)
		}
	})
	r := New(renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...)), WithPaths(paths), WithController(ctl),
		WithSecretResolver(tlsResolver()))

	d1 := doc(t, []any{
		map[string]any{"address": "127.0.0.1", "port": udpPort, "protocol": "udp", "severity": "info"},
		map[string]any{"address": "127.0.0.1", "port": tcpPort, "protocol": "tcp", "severity": "warning", "facilities": []any{"local3"}},
	})
	files, err := r.Render(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}
	conf := files[paths.ConfFile].Content
	assertLoopbackOnly(t, conf, paths)
	t.Logf("rendered rsyslog.conf:\n%s", conf)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("rsyslogd -N1: %v", err)
	}
	// The checker is real: an unknown action parameter is rejected by rsyslogd itself.
	bad := files[paths.ConfFile]
	bad.Content = bytes.Replace(bad.Content, []byte(`queue.type="LinkedList"`), []byte(`queue.type="LinkedList" bogus.param="1"`), 1)
	if err := r.Validate(ctx, renderers.Files{paths.ConfFile: bad}); !errors.Is(err, ErrDaemon) {
		t.Fatalf("rsyslogd -N1 accepted a bogus parameter: %v", err)
	}
	// TLS renders (key 0640, secret) but cannot pass Validate on this host: no ossl driver.
	tlsFiles, err := r.Render(ctx, doc(t, []any{map[string]any{"address": "collector.example.net", "port": 6514, "protocol": "tls",
		"tls": map[string]any{"caRef": "cert/syslog-ca", "certRef": "cert/syslog-client", "keyRef": "key/syslog-client"}}}))
	if err != nil {
		t.Fatal(err)
	}
	err = r.Validate(ctx, tlsFiles)
	t.Logf("TLS export on this host: %v", err)
	if _, statErr := os.Stat(filepath.Join(paths.ModuleDir, "lmnsd_ossl.so")); statErr != nil && !errors.Is(err, ErrDaemon) {
		t.Fatalf("TLS export accepted without the ossl driver: %v", err)
	}

	// First start: write the files and start the child; then prove the flow end to end.
	if err := renderers.WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	if err := start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "log.sock", func() bool { _, err := os.Stat(paths.Standalone.Socket); return err == nil })
	pid1, _ := ctl.PID()
	t.Logf("rsyslogd child pid %d: %s -n -iNONE -f %s", pid1, RsyslogdBin, paths.ConfFile)

	sendUnix(t, paths.Standalone.Socket, "<14>vrxtest: hello RF-4 user.info")        // user.info → UDP only
	sendUnix(t, paths.Standalone.Socket, "<156>vrxtest: hello RF-4 local3.warning")  // local3.warning → UDP and TCP
	sendUnix(t, paths.Standalone.Socket, "<159>vrxtest: hello RF-4 local3.debug")    // below both filters
	sendTCP(t, inTCP, "<13>1 2026-09-24T00:00:00Z h vrxtcp - - - hello via imtcp\n") // user.notice via imtcp → UDP
	rfc5424 := regexp.MustCompile(`^<14>1 \d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d+[+-]\d\d:\d\d \S+ vrxtest - - - +hello RF-4 user\.info\n?$`)
	got := udp.wait(t, 3, 5*time.Second)
	t.Logf("UDP collector 127.0.0.1:%d received (RFC 5424):", udpPort)
	for _, l := range got {
		t.Logf("  %q", l)
	}
	if !slices.ContainsFunc(got, rfc5424.MatchString) {
		t.Fatalf("no UDP message is the expected RFC 5424 line: %q", got)
	}
	if strings.Contains(strings.Join(got, "|"), "local3.debug") {
		t.Fatal("the severity filter let a debug message through")
	}
	tl := tcp.wait(t, 1, 5*time.Second)
	t.Logf("TCP collector 127.0.0.1:%d received (octet-counted): %q", tcpPort, tl[0])
	if !strings.Contains(tl[0], "local3.warning") || !regexp.MustCompile(`^[0-9]+ <156>1 `).MatchString(tl[0]) {
		t.Fatalf("TCP message %q", tl[0])
	}
	var st *State
	waitFor(t, 5*time.Second, "impstats", func() bool {
		st, err = r.State(ctx)
		return err == nil && len(st.Targets) == 2 && st.Targets[0].Processed >= 3 && st.Targets[1].Processed >= 1 && st.Inputs["imuxsock"] >= 3
	})
	for _, ts := range st.Targets {
		t.Logf("impstats %s → %s/%s: processed=%d failed=%d suspended=%d queue.enqueued=%d", ts.Name, ts.Target, ts.Protocol, ts.Processed, ts.Failed, ts.Suspended, ts.Enqueued)
	}
	t.Logf("impstats inputs: %v", st.Inputs)

	// Second commit: change the UDP filter → restart (new PID), convergence on the new action names.
	d2 := doc(t, []any{
		map[string]any{"address": "127.0.0.1", "port": udpPort, "protocol": "udp", "severity": "notice"},
		map[string]any{"address": "127.0.0.1", "port": tcpPort, "protocol": "tcp", "severity": "warning", "facilities": []any{"local3"}},
	})
	files2, err := r.Render(ctx, d2)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(ctx, files2); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, files2); err != nil {
		t.Fatalf("apply: %v", err)
	}
	pid2, _ := ctl.PID()
	if pid2 == pid1 {
		t.Fatal("rsyslog was not restarted")
	}
	t.Logf("Apply restarted rsyslogd: pid %d → %d; impstats reports %v", pid1, pid2, actionNames(files2[paths.ConfFile].Content))
	udp.reset()
	sendUnix(t, paths.Standalone.Socket, "<14>vrxtest: after restart user.info (filtered now)")
	sendUnix(t, paths.Standalone.Socket, "<13>vrxtest: after restart user.notice")
	_ = udp.wait(t, 1, 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	got = udp.snapshot()
	if len(got) != 1 || !strings.Contains(got[0], "user.notice") {
		t.Fatalf("after the new filter the collector got %q", got)
	}

	// A restart that never happens is not reported as success: rolled back.
	lost := New(renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...)), WithPaths(paths), WithVerifyTimeout(3*time.Second),
		WithController(&rfkit.ProcessController{PID: ctl.PID, Binary: RsyslogdBin, OnRestart: func(context.Context) error { return nil }}))
	files3, _ := lost.Render(ctx, doc(t, []any{map[string]any{"address": "127.0.0.1", "port": udpPort, "severity": "debug"}}))
	lostErr := lost.Apply(ctx, files3)
	if !errors.Is(lostErr, rfkit.ErrNotConverged) {
		t.Fatalf("Apply without a restart: %v", lostErr)
	}
	if cur, _ := os.ReadFile(paths.ConfFile); !bytes.Equal(cur, files2[paths.ConfFile].Content) {
		t.Fatal("previous config not restored")
	}
	t.Logf("Apply without a restart is refused and rolled back: %v", lostErr)

	// Events: a target that cannot be reached (TCP collector closed) reports suspension.
	pl := r.Poller()
	_ = pl.Step(ctx)
	tcp.close()
	var evs []rfkit.Event
	t.Cleanup(func() {
		if t.Failed() {
			st, _ := r.State(ctx)
			t.Logf("events so far: %v; state: %+v", evs, st)
			b, _ := os.ReadFile(filepath.Join(base, "rsyslogd.out")) //nolint:gosec // test temp dir / slot dir
			t.Logf("rsyslogd output: %s", b)
		}
	})
	waitFor(t, 10*time.Second, "suspension event", func() bool {
		sendUnix(t, paths.Standalone.Socket, "<156>vrxtest: while the collector is down")
		evs = append(evs, pl.Step(ctx)...)
		for _, e := range evs {
			if strings.HasSuffix(e.Key, "/queue") && e.New == "backlog" || strings.HasSuffix(e.Key, "/suspended") || strings.HasSuffix(e.Key, "/failed") {
				return true
			}
		}
		return false
	})
	t.Logf("events: %v", evs)

	stop()
	if cur := rsyslogUnitPID(t); cur != hostPID {
		t.Fatalf("the host's rsyslog changed: MainPID %s → %s", hostPID, cur)
	}
	t.Logf("host rsyslog.service MainPID %s unchanged", hostPID)
}

// ---------------------------------------------------------------- helpers

func tlsResolver() rfkit.SecretResolver {
	return rfkit.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		switch ref {
		case "cert/syslog-ca", "cert/syslog-client":
			return "-----BEGIN CERTIFICATE-----\nVRX_TEST_PSK_RF4_cert\n-----END CERTIFICATE-----\n", nil
		case "key/syslog-client":
			return "-----BEGIN PRIVATE KEY-----\nVRX_TEST_PSK_RF4_tlskey\n-----END PRIVATE KEY-----\n", nil // VRX_TEST_PSK_ fixture
		}
		return "", errors.New("unknown")
	})
}

func assertLoopbackOnly(t *testing.T, conf []byte, p Paths) {
	t.Helper()
	for _, m := range regexp.MustCompile(`input\(type="([a-z]+)"[^)]*\)`).FindAll(conf, -1) {
		s := string(m)
		switch {
		case strings.Contains(s, `type="imuxsock"`) && strings.Contains(s, `Socket="`+p.Standalone.Socket+`"`):
		case strings.Contains(s, `type="imtcp"`) && strings.Contains(s, `address="127.0.0.1"`):
		default:
			t.Fatalf("input %s is not a slot-scoped listener", s)
		}
	}
	if !bytes.Contains(conf, []byte(`SysSock.Use="off"`)) {
		t.Fatal("imuxsock would bind /dev/log")
	}
	for _, m := range regexp.MustCompile(`target="([^"]+)"`).FindAllSubmatch(conf, -1) {
		if string(m[1]) != "127.0.0.1" {
			t.Fatalf("export target %s is not loopback", m[1])
		}
	}
}

func rsyslogUnitPID(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/systemctl", "show", "rsyslog", "-p", "MainPID", "--value").Output()
	if err != nil {
		t.Fatalf("systemctl show rsyslog: %v", err)
	}
	return strings.TrimSpace(string(out))
}

type collector struct {
	mu    sync.Mutex
	lines []string
	stop  func()
}

func (c *collector) add(s string) { c.mu.Lock(); c.lines = append(c.lines, s); c.mu.Unlock() }
func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}
func (c *collector) reset() { c.mu.Lock(); c.lines = nil; c.mu.Unlock() }
func (c *collector) close() { c.stop() }
func (c *collector) wait(t *testing.T, n int, d time.Duration) []string {
	t.Helper()
	waitFor(t, d, fmt.Sprintf("%d collected messages", n), func() bool { return len(c.snapshot()) >= n })
	return c.snapshot()
}

func udpCollector(t *testing.T, port uint32) *collector {
	t.Helper()
	pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	c := &collector{stop: func() { _ = pc.Close() }}
	t.Cleanup(c.stop)
	go func() {
		buf := make([]byte, 65536)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			c.add(string(buf[:n]))
		}
	}()
	return c
}

func tcpCollector(t *testing.T, port uint32) *collector {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	var cm sync.Mutex
	c := &collector{}
	c.stop = func() {
		_ = ln.Close()
		cm.Lock()
		for _, cn := range conns {
			_ = cn.Close()
		}
		cm.Unlock()
	}
	t.Cleanup(c.stop)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			cm.Lock()
			conns = append(conns, conn)
			cm.Unlock()
			go func() {
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					c.add(sc.Text())
				}
			}()
		}
	}()
	return c
}

func sendUnix(t *testing.T, sock, msg string) {
	t.Helper()
	conn, err := net.Dial("unixgram", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func sendTCP(t *testing.T, port uint32, msg string) {
	t.Helper()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", d, what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func lockSlotDir(t *testing.T, base string) {
	t.Helper()
	if !strings.HasPrefix(base, "/run/vrx-test/") {
		t.Fatalf("refusing to use %s", base)
	}
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(base+".lock", os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // slot lock
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(base); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(base)
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		_ = os.Remove(base + ".lock")
	})
}

func procsUnder(dir string) []int {
	var out []int
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline")) //nolint:gosec // procfs
		if err == nil && bytes.Contains(b, []byte(dir)) {
			out = append(out, pid)
		}
	}
	return out
}
