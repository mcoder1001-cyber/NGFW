package snmpd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestSnmpdIntegration runs the real net-snmp agent as a child of the test (never the system
// unit, never /etc/snmp): config and state under /run/vrx-test/<prefix>/snmpd, bound only to
// 127.0.0.1:3<N>61, killed by the PID the test spawned.
func TestSnmpdIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	port, _ := strconv.Atoi(fmt.Sprintf("3%d61", slot))
	paths := TestPaths(prefix)
	base := filepath.Dir(paths.ConfFile)
	lockSlotDir(t, base)
	ctx := context.Background()

	d := map[string]any{
		"enabled": true, "sysName": prefix + "-snmpd", "sysLocation": "RF-4 rack one", "sysContact": "noc@example.net",
		"listen":      []any{map[string]any{"address": "127.0.0.1", "port": port}},
		"communities": map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "sources": []any{"127.0.0.1/32"}, "view": "sys"}},
		"v3Users": map[string]any{"u1": map[string]any{
			"securityLevel": "authPriv", "authProtocol": "sha256", "authRef": "password/u1-auth", "privProtocol": "aes", "privRef": "password/u1-priv",
		}},
		"views":       map[string]any{"sys": map[string]any{"include": []any{"system", "interfaces"}}},
		"monitors":    map[string]any{"disks": []any{map[string]any{"path": "/", "minPercent": 5}}, "load": map[string]any{"max1": 64, "max5": 48, "max15": 32}},
		"sysServices": 72,
	}
	sysRunner := renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...))
	var child *exec.Cmd
	ctl := &rfkit.ProcessController{Binary: SnmpdBin, PID: func() (int, error) {
		if child == nil || child.Process == nil {
			return 0, rfkit.ErrNotRunning
		}
		return child.Process.Pid, nil
	}}
	r := New(sysRunner, WithPaths(paths), WithController(ctl), WithSecretResolver(resolver(nil)))

	files, err := r.Render(ctx, doc(t, d))
	if err != nil {
		t.Fatal(err)
	}
	conf := files[paths.ConfFile].Content
	// The rendered listen list is exactly the slot port on loopback (never ens192).
	if got := grepLines(conf, "agentaddress "); len(got) != 1 || got[0] != fmt.Sprintf("agentaddress udp:127.0.0.1:%d", port) {
		t.Fatalf("rendered listen list %q", got)
	}

	// Validate: the daemon's parse run accepts the file (traps included) and rejects a broken copy.
	withTraps := with(d, "trapReceivers", []any{
		map[string]any{"address": "127.0.0.1", "port": port + 1, "version": "v2c", "community": "ro"},
		map[string]any{"address": "127.0.0.1", "port": port + 1, "version": "v3", "user": "u1", "inform": true},
	})
	tf, err := r.Render(ctx, doc(t, withTraps))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(ctx, tf); err != nil {
		t.Fatalf("parse run rejected the rendered file: %v", err)
	}
	bad := tf[paths.ConfFile]
	bad.Content = append(append([]byte{}, bad.Content...), "view broken included not-an-oid\n"...)
	err = r.Validate(ctx, renderers.Files{paths.ConfFile: bad})
	if !errors.Is(err, ErrDaemon) {
		t.Fatalf("parse run accepted a broken file: %v", err)
	}
	t.Logf("parse run rejects a broken line: %v", err)

	// Start the child with the rendered file.
	if err := os.MkdirAll(filepath.Join(base, "persist"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "mibs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := renderers.WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(base, "snmpd.log")
	startChild := func() {
		child = exec.Command(SnmpdBin, "-f", "-Lf", logFile, "-A", "-C", "-c", paths.ConfFile, "-p", filepath.Join(base, "snmpd.pid"), //nolint:gosec // test harness: fixed argv of an allow-listed binary
			"-m", "", "-M", filepath.Join(base, "mibs"))
		child.Env = []string{"PATH=/usr/sbin:/usr/bin", "SNMP_PERSISTENT_DIR=" + filepath.Join(base, "persist"), "SNMPCONFPATH=" + base}
		child.Dir = base
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
	}
	startChild()
	pid := child.Process.Pid
	t.Cleanup(func() { stopChild(t, child, base) })
	t.Logf("snmpd child pid %d: %s", pid, strings.Join(child.Args, " "))

	st := waitReachable(t, r, 5*time.Second)
	t.Logf("SNMP reply (%s via %s): sysName.0=%q sysLocation.0=%q sysContact.0=%q sysUpTime.0=%d sysDescr.0=%q",
		st.Endpoint, st.Credential, st.SysName, st.SysLocation, st.SysContact, st.SysUpTime, st.SysDescr)
	if st.SysName != prefix+"-snmpd" || st.SysLocation != "RF-4 rack one" || st.Credential != "v3 user u1" {
		t.Fatalf("unexpected state %+v", st)
	}
	// Both credentials work over the wire (gosnmp, in-process: no secret in any argv).
	v2, err := GoSNMP{}.Get(ctx, Target{Addr: netip.MustParseAddr("127.0.0.1"), Port: uint16(port), Community: secRO}, []string{OIDSysName}) //nolint:gosec // slot port < 65536
	if err != nil || v2[OIDSysName] != prefix+"-snmpd" {
		t.Fatalf("v2c get: %v %v", v2, err)
	}
	if _, err := (GoSNMP{}).Get(ctx, Target{Addr: netip.MustParseAddr("127.0.0.1"), Port: uint16(port), Community: "wrong-community"}, []string{OIDSysName}); err == nil { //nolint:gosec // slot port < 65536
		t.Fatal("a wrong community was answered")
	}

	// SIGHUP applies a sysLocation change in place: same PID, new value (convergence checked by Apply).
	files2, err := r.Render(ctx, doc(t, with(d, "sysLocation", "RF-4 rack two")))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, files2); err != nil {
		t.Fatalf("apply: %v", err)
	}
	st = waitReachable(t, r, 2*time.Second)
	if st.SysLocation != "RF-4 rack two" || child.Process.Pid != pid || !alive(pid) {
		t.Fatalf("after SIGHUP: location %q pid %d alive=%v (want same pid %d)", st.SysLocation, child.Process.Pid, alive(pid), pid)
	}
	t.Logf("after Apply+SIGHUP: sysLocation.0=%q, pid %d unchanged", st.SysLocation, pid)

	// A reload that the daemon never sees (control channel lost) must not report success: the
	// convergence check fails and the previous file is restored.
	lost := New(sysRunner, WithPaths(paths), WithSecretResolver(resolver(nil)), WithVerifyTimeout(1500*time.Millisecond),
		WithController(&fakeCtl{pid: child.Process.Pid})) // "reloads" without signalling the daemon
	files3, _ := lost.Render(ctx, doc(t, with(d, "sysLocation", "never applied")))
	lostErr := lost.Apply(ctx, files3)
	if lostErr == nil {
		t.Fatal("Apply reported success although snmpd never reloaded")
	}
	if cur, _ := os.ReadFile(paths.ConfFile); !bytes.Equal(cur, files2[paths.ConfFile].Content) {
		t.Fatal("previous snmpd.conf not restored after the failed Apply")
	}
	t.Logf("Apply without a reload is refused and rolled back: %v", lostErr)

	// Events: the poller reports a change applied by the next commit.
	pl := r.Poller()
	_ = pl.Step(ctx)
	files4, _ := r.Render(ctx, doc(t, with(d, "sysLocation", "RF-4 rack three")))
	if err := r.Apply(ctx, files4); err != nil {
		t.Fatal(err)
	}
	evs := pl.Step(ctx)
	if len(evs) != 1 || evs[0].Key != "sysLocation" {
		t.Fatalf("events %v", evs)
	}
	t.Logf("event: %s", evs[0])

	// Review H1, reproduced live: a listen-port change. SIGHUP re-reads the file but keeps the old
	// socket, so Apply must not reload: it returns a persisted restart request.
	answers := func(port int) error {
		_, err := GoSNMP{}.Get(ctx, Target{Addr: netip.MustParseAddr("127.0.0.1"), Port: uint16(port), Community: secRO}, []string{OIDSysName}) //nolint:gosec // slot port
		return err
	}
	portA, portB := port+1, port+2 // 3<N>62, 3<N>63
	dPort := func(p int) map[string]any {
		return with(d, "sysLocation", "RF-4 rack three", "listen", []any{map[string]any{"address": "127.0.0.1", "port": p}})
	}
	var ar *rfkit.ActionRequired
	fA, _ := r.Render(ctx, doc(t, dPort(portA)))
	reqErr := r.Apply(ctx, fA)
	if !errors.As(reqErr, &ar) || ar.Action != "restart" {
		t.Fatalf("listen change %d→%d: want a restart request, got %v", port, portA, reqErr)
	}
	t.Logf("listen %d→%d: Apply → %v", port, portA, reqErr)
	if err := r.Apply(ctx, fA); !errors.As(err, &ar) || !strings.Contains(ar.Reason, "still pending") {
		t.Fatalf("restart request not persisted: %v", err)
	}
	// The daemon itself, SIGHUPed on the new file, still holds the old socket (the reviewer's probe).
	if err := ctl.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	errOld, errNew := answers(port), answers(portA)
	socks, _ := rfkit.UDPListeners(child.Process.Pid)
	t.Logf("after a raw SIGHUP: :%d answers=%v, :%d error=%v, sockets %v", port, errOld == nil, portA, errNew, socks)
	if errOld != nil || errNew == nil {
		t.Fatalf("expected the SIGHUP to keep the old socket: old=%v new=%v", errOld, errNew)
	}
	if err := r.Converged(ctx); !errors.Is(err, rfkit.ErrNotConverged) {
		t.Fatalf("Converged must see the stale socket: %v", err)
	}
	t.Logf("Converged before the restart: %v", r.Converged(ctx))
	// The commit engine acts on the request: restart; the next Apply clears it and proves the sockets.
	restart := func() {
		stopChild(t, child, base)
		startChild()
		waitReachable(t, r, 5*time.Second)
	}
	restart()
	if err := r.Apply(ctx, fA); err != nil {
		t.Fatalf("apply after the restart: %v", err)
	}
	socks, _ = rfkit.UDPListeners(child.Process.Pid)
	if rfkit.GetPending(paths.PendingFile) != nil || answers(portA) != nil || answers(port) == nil {
		t.Fatalf("after restart: pending=%v sockets=%v", rfkit.GetPending(paths.PendingFile), socks)
	}
	t.Logf("after restart + Apply: pending cleared, snmpd listens on %v, :%d refused", socks, port)
	// The reviewer's exact case, 3<N>62 → 3<N>63.
	fB, _ := r.Render(ctx, doc(t, dPort(portB)))
	err = r.Apply(ctx, fB)
	if !errors.As(err, &ar) || answers(portA) != nil {
		t.Fatalf("%d→%d: %v", portA, portB, err)
	}
	restart()
	if err := r.Apply(ctx, fB); err != nil {
		t.Fatal(err)
	}
	if err := r.Converged(ctx); err != nil || answers(portB) != nil || answers(portA) == nil {
		t.Fatalf("%d→%d not converged: %v", portA, portB, err)
	}
	socks, _ = rfkit.UDPListeners(child.Process.Pid)
	t.Logf("%d→%d: restart request, restart, Apply converged: sockets %v, :%d refused", portA, portB, socks, portA)

	// Secrets: plaintext only in the 0600 snmpd.conf; nothing else under the slot dir, the
	// daemon log or anything the renderer returned holds them.
	info, err := os.Stat(paths.ConfFile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snmpd.conf mode %v %v", info.Mode(), err)
	}
	assertSecretsOnlyIn(t, base, paths.ConfFile)
	msg, _ := r.Retrieve(ctx)
	for _, s := range plantedSecrets {
		if strings.Contains(fmt.Sprint(msg), s) || strings.Contains(lostErr.Error(), s) {
			t.Fatalf("secret %s leaked through Retrieve or an error", s)
		}
	}
}

// ---------------------------------------------------------------- test harness helpers

func waitReachable(t *testing.T, r *Renderer, d time.Duration) *State {
	t.Helper()
	var st *State
	err := rfkit.Poll(context.Background(), d, 100*time.Millisecond, func(ctx context.Context) error {
		var err error
		st, err = r.State(ctx)
		if err != nil {
			return err
		}
		if !st.Reachable {
			return errors.New(st.Error)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snmpd not reachable: %v", err)
	}
	return st
}

func grepLines(b []byte, prefix string) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// lockSlotDir creates the slot directory and holds an exclusive lock on it for the test (two
// packages of one slot never share a daemon directory: RF-1 review M4).
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

// stopChild stops the daemon the test spawned (by its PID) and asserts nothing of ours runs.
func stopChild(t *testing.T, c *exec.Cmd, base string) {
	t.Helper()
	if c.Process != nil && c.ProcessState == nil {
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
	if left := procsUnder(base); len(left) > 0 {
		t.Errorf("processes still running with %s in their command line: %v", base, left)
	}
}

// procsUnder lists PIDs whose command line mentions dir (the pgrep -f check, without pgrep).
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

// assertSecretsOnlyIn walks dir: a planted secret may appear only in the allowed files.
func assertSecretsOnlyIn(t *testing.T, dir string, allowed ...string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSocket != 0 {
			return nil
		}
		for _, a := range allowed {
			if p == a {
				return nil
			}
		}
		b, _ := rfkit.ReadFileLimit(p, 8<<20)
		for _, s := range plantedSecrets {
			if bytes.Contains(b, []byte(s)) {
				t.Errorf("secret %s found in %s", s, p)
			}
		}
		return nil
	})
}
