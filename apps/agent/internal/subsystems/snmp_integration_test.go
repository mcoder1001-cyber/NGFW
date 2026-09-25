package subsystems

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/renderers/snmpd"
	"ngfw/agent/internal/snmpagent"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestSnmpStageIntegration (F-snmp acceptance, VRX_INTEGRATION=1, needs /usr/sbin/snmpd): the stage renders
// and applies services.snmp for a slot snmpd child (never the system unit, never /etc/snmp; loopback slot
// ports only), a gosnmp TrapListener on 3<N>62 receives snmpd's coldStart, in-process gosnmp walks (v2c and
// v3 authPriv) return sysName and the VRX-MIB interface table through the AgentX subagent, an snmpd restart
// is followed by re-registration within 30 s, an agent restart (fresh stage) re-renders the same file, and a
// rolled-back community stops answering. Secrets: VRX_TEST_PSK_F-snmp_* literals only; walks log names, never
// values. The VRX-MIB source is a fixed snapshot here (no VPP needed; the stats path is unit-tested).
func TestSnmpStageIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if _, err := os.Stat(snmpd.SnmpdBin); err != nil {
		t.Skipf("%s not installed", snmpd.SnmpdBin)
	}
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	var port, trapPort uint32
	_, _ = fmt.Sscanf(fmt.Sprintf("3%d61 3%d62", slot, slot), "%d %d", &port, &trapPort)
	paths := snmpd.TestPaths(prefix)
	base := filepath.Dir(paths.ConfFile)
	lockSlot(t, base)
	ctx := context.Background()

	var child *exec.Cmd
	ctl := &rfkit.ProcessController{Binary: snmpd.SnmpdBin, PID: func() (int, error) {
		if child == nil || child.Process == nil || child.ProcessState != nil {
			return 0, rfkit.ErrNotRunning
		}
		return child.Process.Pid, nil
	}}
	secrets := writeFixtures(t, allFixtures)
	src := snmpagent.SourceFunc(func(context.Context) (snmpagent.Snapshot, error) {
		return snmpagent.Snapshot{
			Agent:      snmpagent.AgentInfo{Version: "it", VppConnected: true, Revision: "txn-it", Commits: 1},
			Interfaces: []snmpagent.Interface{{SwIfIndex: 1, Name: "loop0", AdminUp: true, OperUp: true, InOctets: 1234}},
		}, nil
	})
	newStage := func() *SnmpStage {
		r := snmpd.New(renderers.NewSystemRunner(renderers.NewAllowlist(snmpd.Binaries()...)), snmpd.WithPaths(paths), snmpd.WithController(ctl),
			snmpd.WithSecretResolver(SnmpFixtureResolver(secrets)))
		return NewSnmpStage(r, filepath.Join(base, "state", "snmpd-"+prefix+".json"), src, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	}
	st := newStage()
	t.Cleanup(st.Close)

	v := &vrxv1.SnmpService{
		Enabled: proto.Bool(true), SysName: proto.String(prefix + "-snmp"),
		Listen:      []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(port)}},
		Communities: map[string]*vrxv1.SnmpService_Community{"ro": {SecretRef: proto.String("password/snmp-ro"), Sources: []string{"127.0.0.1/32"}}},
		V3Users: map[string]*vrxv1.SnmpService_V3User{"noc": {
			SecurityLevel: proto.String("authPriv"), AuthProtocol: proto.String("sha"), AuthRef: proto.String("password/noc-auth"),
			PrivProtocol: proto.String("aes"), PrivRef: proto.String("password/noc-priv"),
		}},
		TrapReceivers: []*vrxv1.SnmpService_TrapReceiver{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(trapPort), Version: proto.String("v2c"), Community: proto.String("ro")}},
	}
	if _, err := st.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.State(ctx); !strings.Contains(s.PendingAction, "start") {
		t.Fatalf("snmpd not running: want a pending start, got %+v", s)
	}

	traps := make(chan *gosnmp.SnmpPacket, 8)
	tl := gosnmp.NewTrapListener()
	tl.Params = &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: fixtureCommunity, Timeout: 2 * time.Second}
	tl.OnNewTrap = func(p *gosnmp.SnmpPacket, _ *net.UDPAddr) { traps <- p }
	go func() { _ = tl.Listen(fmt.Sprintf("127.0.0.1:%d", trapPort)) }()
	<-tl.Listening()
	t.Cleanup(tl.Close)

	start := func() {
		child = exec.Command(snmpd.SnmpdBin, "-f", "-Lf", filepath.Join(base, "snmpd.log"), "-C", "-c", paths.ConfFile, "-p", filepath.Join(base, "snmpd.pid"), //nolint:gosec // fixed argv, allow-listed binary
			"-m", "", "-M", filepath.Join(base, "mibs"))
		child.Env = []string{"PATH=/usr/sbin:/usr/bin", "SNMP_PERSISTENT_DIR=" + filepath.Join(base, "persist"), "SNMPCONFPATH=" + base}
		child.Dir = base
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		t.Logf("snmpd child pid %d", child.Process.Pid)
	}
	stop := func() {
		if child != nil && child.ProcessState == nil {
			_ = child.Process.Signal(syscall.SIGTERM)
			_ = child.Wait()
		}
	}
	_ = os.MkdirAll(filepath.Join(base, "mibs"), 0o700)
	start()
	t.Cleanup(stop)

	select {
	case p := <-traps:
		t.Logf("trap received on 127.0.0.1:%d: %d varbinds, snmpTrapOID=%v", trapPort, len(p.Variables), trapOID(p))
	case <-time.After(10 * time.Second):
		t.Fatal("no trap (coldStart) within 10 s")
	}
	waitRegistered(t, st, 1, 30*time.Second)

	v2c := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(port), Version: gosnmp.Version2c, Community: fixtureCommunity, Timeout: 2 * time.Second, Retries: 1}
	v3 := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(port), Version: gosnmp.Version3, Timeout: 2 * time.Second, Retries: 1,
		SecurityModel: gosnmp.UserSecurityModel, MsgFlags: gosnmp.AuthPriv,
		SecurityParameters: &gosnmp.UsmSecurityParameters{UserName: "noc", AuthenticationProtocol: gosnmp.SHA, AuthenticationPassphrase: fixtureAuth,
			PrivacyProtocol: gosnmp.AES, PrivacyPassphrase: fixturePriv}}
	for name, c := range map[string]*gosnmp.GoSNMP{"v2c community ro": v2c, "v3 user noc (authPriv)": v3} {
		walk(t, name, c, prefix+"-snmp")
	}

	// snmpd restart → the subagent re-registers
	stop()
	restartAt := time.Now()
	start()
	waitRegistered(t, st, 2, 30*time.Second)
	t.Logf("subagent re-registered %v after the snmpd restart", time.Since(restartAt).Round(time.Millisecond))

	// agent restart: a fresh stage reports the applied value (file unchanged) and re-applies it idempotently
	before, _ := os.ReadFile(paths.ConfFile)
	st.Close()
	st = newStage()
	kvs, err := st.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("fresh stage retrieve: %v %v", kvs, err)
	}
	if _, err := st.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(paths.ConfFile)
	if string(before) != string(after) {
		t.Fatal("agent restart changed snmpd.conf")
	}
	waitRegistered(t, st, 1, 30*time.Second)
	t.Log("agent restart: snmpd.conf re-rendered identically, subagent registered again")

	// rollback removes a community: add "other", roll back, walk with it fails
	v2 := proto.Clone(v).(*vrxv1.SnmpService)
	v2.Communities["other"] = &vrxv1.SnmpService_Community{SecretRef: proto.String("password/other"), Sources: []string{"127.0.0.1/32"}}
	if _, err := st.Update(ctx, v, v2, nil); err != nil {
		t.Fatal(err)
	}
	newOther := func() *gosnmp.GoSNMP {
		return &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(port), Version: gosnmp.Version2c, Community: fixtureOther, Timeout: 2 * time.Second, Retries: 1}
	}
	walk(t, "v2c community other (added)", newOther(), prefix+"-snmp")
	if _, err := st.Update(ctx, v2, v, nil); err != nil { // the scheduler's rollback path
		t.Fatal(err)
	}
	other := newOther()
	if err := other.Connect(); err == nil {
		defer other.Conn.Close()
		if _, err := other.Get([]string{".1.3.6.1.2.1.1.5.0"}); err == nil {
			t.Fatal("rolled-back community still answers")
		} else {
			t.Logf("after rollback, community other: %v (expected: no answer)", err)
		}
	}
	for _, f := range []string{filepath.Join(base, "snmpd.log"), filepath.Join(base, "state", "snmpd-"+prefix+".json")} {
		b, _ := os.ReadFile(f)
		assertNoSecrets(t, f, string(b))
	}
}

func walk(t *testing.T, name string, c *gosnmp.GoSNMP, sysName string) {
	t.Helper()
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	defer c.Conn.Close()
	r, err := c.Get([]string{".1.3.6.1.2.1.1.5.0"})
	if err != nil || len(r.Variables) != 1 || fmt.Sprint(string(r.Variables[0].Value.([]byte))) != sysName {
		t.Fatalf("%s: sysName: %v %v", name, r, err)
	}
	pdus, err := c.BulkWalkAll(snmpagent.VRXMIBOID.String())
	if err != nil || len(pdus) == 0 {
		t.Fatalf("%s: VRX-MIB walk: %d %v", name, len(pdus), err)
	}
	t.Logf("%s: sysName.0 = %s; VRX-MIB walk returned %d varbinds:", name, sysName, len(pdus))
	for _, p := range pdus {
		t.Logf("  %s = %v", p.Name, printable(p))
	}
}

func printable(p gosnmp.SnmpPDU) any {
	if b, ok := p.Value.([]byte); ok {
		return string(b)
	}
	return p.Value
}

func trapOID(p *gosnmp.SnmpPacket) any {
	for _, v := range p.Variables {
		if v.Name == ".1.3.6.1.6.3.1.1.4.1.0" {
			return v.Value
		}
	}
	return nil
}

func waitRegistered(t *testing.T, st *SnmpStage, n uint64, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if s, _ := st.State(context.Background()); s != nil && s.Subagent != nil && s.Subagent.Registered && s.Subagent.Registrations >= n {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	s, _ := st.State(context.Background())
	t.Fatalf("subagent not registered (#%d) within %v: %+v", n, within, s)
}

func lockSlot(t *testing.T, base string) {
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
	_ = os.RemoveAll(base)
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

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
