package keepalived

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const ipBin = "/usr/bin/ip" // test-only (never in a renderer allowlist)

// TestKeepalivedIntegration runs keepalived as a child of the test inside the slot's own
// network namespace ns-<prefix>-a, on a veth pair whose both ends live in that namespace, so no
// VRRP advertisement can ever reach a host link (ens192). Config and state under
// /run/vrx-test/<prefix>/keepalived; the notify helper and the check executable in a root-owned
// temp dir (/run is noexec). Killed by the PID the test spawned.
func TestKeepalivedIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	ctx := context.Background()
	ns, ifA, ifB := "ns-"+prefix+"-a", prefix+"-a", prefix+"-b"
	base := filepath.Join("/run/vrx-test", prefix, "keepalived")
	lockSlotDir(t, base)
	binDir := buildHelpers(t, prefix)
	paths := TestPaths(prefix, binDir, ns)
	for _, d := range []string{paths.StateDir, paths.DumpDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	makeNetNS(t, ns, ifA, ifB, fmt.Sprintf("10.%d.240.2/24", slot), fmt.Sprintf("10.%d.241.2/24", slot))

	var child *exec.Cmd
	ctl := &rfkit.ProcessController{Binary: KeepalivedBin, PID: func() (int, error) {
		if child == nil || child.Process == nil || child.ProcessState != nil {
			return 0, rfkit.ErrNotRunning
		}
		return child.Process.Pid, nil
	}}
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...))
	r := New(runner, WithPaths(paths), WithController(ctl), WithInterfaceMapper(PrefixMapper(prefix+"-")),
		WithChecks("vrx-check-ok"), WithSecretResolver(rfkit.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
			if ref == "psk/vrrp-vi2" {
				return planted, nil
			}
			return "", errors.New("unknown")
		})))

	vip1 := fmt.Sprintf("10.%d.240.1", slot)
	d1 := doc(t, map[string]any{
		"ha": map[string]any{
			"vrrp": map[string]any{"vi1": map[string]any{
				"engine": "keepalived", "interface": ifA, "vrId": slot*10 + 1, "priority": 150,
				"advertisementIntervalMs": 500, "addresses": []any{vip1},
				"keepalived": map[string]any{"prefixLength": 24, "trackScripts": []any{"ok"}},
			}},
			"keepalived": map[string]any{"routerId": prefix + "-ka", "scripts": map[string]any{"ok": map[string]any{"check": "vrx-check-ok", "interval": 1, "rise": 1, "fall": 2}}},
		},
	})
	files, err := r.Render(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}
	conf := files[paths.ConfFile].Content
	assertScoped(t, conf, prefix+"-")
	t.Logf("rendered keepalived.conf:\n%s", conf)
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("keepalived -t: %v", err)
	}
	// The checker is real: an interface that does not exist in the namespace is rejected.
	badFiles, _ := r.Render(ctx, doc(t, map[string]any{"ha": map[string]any{"vrrp": map[string]any{"x": map[string]any{
		"engine": "keepalived", "interface": prefix + "-nope", "vrId": 9, "addresses": []any{vip1}}}}}))
	if err := r.Validate(ctx, badFiles); !errors.Is(err, ErrDaemon) {
		t.Fatalf("keepalived -t accepted a missing interface: %v", err)
	}

	if err := renderers.WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(base, "keepalived.log")
	lf, err := os.Create(logFile) //nolint:gosec // test log
	if err != nil {
		t.Fatal(err)
	}
	child = exec.Command(ipBin, "netns", "exec", ns, KeepalivedBin, "-n", "-l", "-P", "-G", "-f", paths.ConfFile,
		"-p", filepath.Join(base, "keepalived.pid"), "-r", filepath.Join(base, "vrrp.pid"), "-c", filepath.Join(base, "checkers.pid"))
	child.Env = []string{"PATH=/usr/sbin:/usr/bin", "TMPDIR=" + paths.DumpDir}
	child.Stdout, child.Stderr, child.Dir = lf, lf, base
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid, started := child.Process.Pid, time.Now()
	t.Cleanup(func() { stopChild(t, child, base); _ = lf.Close() })
	t.Logf("keepalived child pid %d: %s", pid, strings.Join(child.Args, " "))

	waitState(t, paths.StateDir, "vi1", "MASTER", 5*time.Second)
	raw, _ := os.ReadFile(filepath.Join(paths.StateDir, "vi1.state"))
	t.Logf("vi1.state (MASTER after %s): %s", time.Since(started).Round(time.Millisecond), strings.TrimSpace(string(raw)))
	if !vipOn(t, ns, ifA, vip1+"/24") {
		t.Fatalf("VIP %s/24 not on %s", vip1, ifA)
	}
	t.Logf("ip -n %s -j addr show dev %s: %s/24 present", ns, ifA, vip1)
	st, err := r.State(ctx)
	if err != nil || st.DumpError != "" || len(st.Instances) != 1 || st.Instances[0].Dump == nil || st.Instances[0].Dump.State != "MASTER" {
		t.Fatalf("state %+v %v", st, err)
	}
	j, _ := json.Marshal(st.Instances[0])
	t.Logf("Retrieve (state file + keepalived JSON dump): %s", j)

	// Second commit: priority change + a VRRPv2 instance with a PASS key + sync group, applied
	// with SIGHUP (same PID); convergence is checked against keepalived's dump by Apply.
	vip2 := fmt.Sprintf("10.%d.241.1", slot)
	d2 := doc(t, map[string]any{
		"ha": map[string]any{
			"vrrp": map[string]any{
				"vi1": map[string]any{
					"engine": "keepalived", "interface": ifA, "vrId": slot*10 + 1, "priority": 160,
					"advertisementIntervalMs": 500, "addresses": []any{vip1},
					"keepalived": map[string]any{"prefixLength": 24, "trackScripts": []any{"ok"}},
				},
				"vi2": map[string]any{
					"engine": "keepalived", "interface": ifB, "vrId": slot*10 + 2, "priority": 120, "advertisementIntervalMs": 1000,
					"addresses": []any{vip2}, "keepalived": map[string]any{"prefixLength": 24, "authRef": "psk/vrrp-vi2"},
				},
			},
			"keepalived": map[string]any{"routerId": prefix + "-ka", "scripts": map[string]any{"ok": map[string]any{"check": "vrx-check-ok", "interval": 1, "rise": 1, "fall": 2}},
				"syncGroups": map[string]any{"g1": []any{"vi1", "vi2"}}},
		},
	})
	files2, err := r.Render(ctx, d2)
	if err != nil {
		t.Fatal(err)
	}
	assertScoped(t, files2[paths.ConfFile].Content, prefix+"-")
	if err := r.Validate(ctx, files2); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, files2); err != nil {
		t.Fatalf("apply: %v", err)
	}
	waitState(t, paths.StateDir, "vi2", "MASTER", 6*time.Second)
	st, err = r.State(ctx)
	if err != nil || len(st.Instances) != 2 || st.Instances[0].Dump == nil || st.Instances[0].Dump.BasePriority != 160 ||
		st.Instances[1].Dump == nil || st.Instances[1].Dump.Version != 2 || child.Process.Pid != pid || !alive(pid) {
		t.Fatalf("after SIGHUP: %+v %v (pid %d alive=%v)", st, err, pid, alive(pid))
	}
	j, _ = json.Marshal(st.Instances)
	t.Logf("after Apply+SIGHUP (pid %d unchanged): %s", pid, j)

	// A reload keepalived never sees is not reported as success: rolled back.
	lost := New(runner, WithPaths(paths), WithController(&noReload{ctl}), WithInterfaceMapper(PrefixMapper(prefix+"-")),
		WithChecks("vrx-check-ok"), WithVerifyTimeout(2*time.Second), WithSecretResolver(rfkit.SecretResolverFunc(func(context.Context, string) (string, error) { return planted, nil })))
	files3, err := lost.Render(ctx, doc(t, map[string]any{"ha": map[string]any{"vrrp": map[string]any{"vi1": map[string]any{
		"engine": "keepalived", "interface": ifA, "vrId": slot*10 + 1, "priority": 170, "addresses": []any{vip1}}}}}))
	if err != nil {
		t.Fatal(err)
	}
	lostErr := lost.Apply(ctx, files3)
	if !errors.Is(lostErr, rfkit.ErrNotConverged) {
		t.Fatalf("Apply without a reload: %v", lostErr)
	}
	if cur, _ := os.ReadFile(paths.ConfFile); !bytes.Equal(cur, files2[paths.ConfFile].Content) {
		t.Fatal("previous keepalived.conf not restored")
	}
	t.Logf("Apply without a reload is refused and rolled back: %v", lostErr)

	// Events from the state files; then stop keepalived: STOP is written and the VIP is gone.
	pl := r.Poller()
	_ = pl.Step(ctx)
	_ = child.Process.Signal(syscall.SIGTERM)
	waitState(t, paths.StateDir, "vi1", "STOP", 5*time.Second)
	evs := pl.Step(ctx)
	t.Logf("events after SIGTERM: %v", evs)
	if len(evs) == 0 || evs[0].Key != "vi1" || evs[0].New != "STOP" {
		t.Fatalf("events %v", evs)
	}
	done := make(chan struct{})
	go func() { _ = child.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keepalived did not exit")
	}
	if vipOn(t, ns, ifA, vip1+"/24") {
		t.Fatal("VIP still present after keepalived stopped")
	}
	raw, _ = os.ReadFile(filepath.Join(paths.StateDir, "vi1.state"))
	t.Logf("after kill: vi1.state %s; VIP gone from %s", strings.TrimSpace(string(raw)), ifA)

	// Secrets: the PASS key is only in keepalived.conf (0640); not in the log, not in any
	// state or dump file (the dump is deleted after reading), not in Retrieve or errors.
	if info, _ := os.Stat(paths.ConfFile); info.Mode().Perm() != 0o640 {
		t.Fatalf("keepalived.conf mode %v", info.Mode())
	}
	if !bytes.Contains(files2[paths.ConfFile].Content, []byte(planted)) {
		t.Fatal("planted key not rendered")
	}
	assertSecretOnlyIn(t, base, planted, paths.ConfFile)
	assertSecretOnlyIn(t, binDir, planted)
	for _, s := range []string{fmt.Sprint(st.Struct()), lostErr.Error()} {
		if strings.Contains(s, planted) {
			t.Fatalf("secret leaked: %s", s)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.DumpDir, "keepalived.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("keepalived.json left behind")
	}
}

// planted is the VRRPv2 PASS key used by the test (8 characters max: the VRX_TEST_PSK_<id>
// fixture literal does not fit; see RF-4-questions.md).
const planted = "RF4tpsk8"

type noReload struct{ rfkit.Controller }

func (noReload) Reload(context.Context) error { return nil }

func buildHelpers(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vrx-"+prefix+"-keepalived-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // keepalived script security: root-owned, not writable by others
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "vrx-keepalived-notify"), "./cmd/vrx-keepalived-notify")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build notify helper: %v\n%s", err, out)
	}
	if err := os.Mkdir(filepath.Join(dir, "checks"), 0o755); err != nil { //nolint:gosec // see above
		t.Fatal(err)
	}
	b, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks", "vrx-check-ok"), b, 0o755); err != nil { //nolint:gosec // executable
		t.Fatal(err)
	}
	return dir
}

func makeNetNS(t *testing.T, ns, ifA, ifB, cidrA, cidrB string) {
	t.Helper()
	run := func(args ...string) {
		if out, err := exec.Command(ipBin, args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	if exec.Command(ipBin, "netns", "pids", ns).Run() == nil {
		_ = exec.Command(ipBin, "netns", "delete", ns).Run() // stale namespace of an earlier run of this slot
	}
	run("netns", "add", ns)
	t.Cleanup(func() {
		if out, err := exec.Command(ipBin, "netns", "delete", ns).CombinedOutput(); err != nil {
			t.Errorf("ip netns delete %s: %v %s", ns, err, out)
		}
	})
	// Both ends are created inside the namespace: nothing touches the host's links.
	run("-n", ns, "link", "add", ifA, "type", "veth", "peer", "name", ifB)
	run("-n", ns, "link", "set", "lo", "up")
	run("-n", ns, "addr", "add", cidrA, "dev", ifA)
	run("-n", ns, "addr", "add", cidrB, "dev", ifB)
	run("-n", ns, "link", "set", ifA, "up")
	run("-n", ns, "link", "set", ifB, "up")
}

func vipOn(t *testing.T, ns, dev, cidr string) bool {
	t.Helper()
	out, err := exec.Command(ipBin, "-n", ns, "-j", "addr", "show", "dev", dev).Output()
	if err != nil {
		t.Fatalf("ip -j addr: %v", err)
	}
	var links []struct {
		AddrInfo []struct {
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(out, &links); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		for _, a := range l.AddrInfo {
			if a.Local+"/"+strconv.Itoa(a.PrefixLen) == cidr {
				return true
			}
		}
	}
	return false
}

func waitState(t *testing.T, dir, name, want string, d time.Duration) StateRecord {
	t.Helper()
	var rec StateRecord
	err := rfkit.Poll(context.Background(), d, 50*time.Millisecond, func(context.Context) error {
		var err error
		rec, err = ReadStateFile(dir, name)
		if err == nil && rec.State != want {
			err = fmt.Errorf("%s is %q", name, rec.State)
		}
		return err
	})
	if err != nil {
		t.Fatalf("%s did not reach %s within %s: %v", name, want, d, err)
	}
	return rec
}

// assertScoped: every interface keepalived will bind (interface, dev, track) carries the slot
// prefix — never ens192 or another host link.
func assertScoped(t *testing.T, conf []byte, ifPrefix string) {
	t.Helper()
	for _, l := range strings.Split(string(conf), "\n") {
		f := strings.Fields(l)
		for i, w := range f {
			if (w == "interface" || w == "dev") && i+1 < len(f) && !strings.HasPrefix(f[i+1], ifPrefix) {
				t.Fatalf("rendered interface %q outside the slot (%s*): %q", f[i+1], ifPrefix, l)
			}
		}
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

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

func assertSecretOnlyIn(t *testing.T, dir, secret string, allowed ...string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		for _, a := range allowed {
			if p == a {
				return nil
			}
		}
		b, _ := rfkit.ReadFileLimit(p, 64<<20)
		if bytes.Contains(b, []byte(secret)) {
			t.Errorf("secret found in %s", p)
		}
		return nil
	})
}
