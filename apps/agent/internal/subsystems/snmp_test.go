package subsystems

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/snmpd"
	"ngfw/agent/internal/scheduler"
)

const (
	fixtureCommunity = "VRX_TEST_PSK_F-snmp_ro1"     //nolint:gosec // test literal (envelope: VRX_TEST_PSK_F-snmp_*)
	fixtureAuth      = "VRX_TEST_PSK_F-snmp_auth1"   //nolint:gosec // test literal
	fixturePriv      = "VRX_TEST_PSK_F-snmp_priv1"   //nolint:gosec // test literal
	fixtureOther     = "VRX_TEST_PSK_F-snmp_other22" //nolint:gosec // test literal
)

type stoppedCtl struct{}

func (stoppedCtl) Reload(context.Context) error                 { return nil }
func (stoppedCtl) Restart(context.Context) error                { return nil }
func (stoppedCtl) Signal(context.Context, syscall.Signal) error { return nil }
func (stoppedCtl) MainPID(context.Context) (int, error)         { return 0, nil }

func writeFixtures(t *testing.T, m map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("{")
	first := true
	for k, v := range m {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + k + `":"` + v + `"`)
	}
	b.WriteString("}")
	f := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(f, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func newTestStage(t *testing.T, secrets map[string]string) (*SnmpStage, snmpd.Paths, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	p := snmpd.Paths{ConfFile: filepath.Join(dir, "snmpd.conf"), AgentXSocket: filepath.Join(dir, "agentx.sock"), PendingFile: filepath.Join(dir, "pending"), FileMode: 0o600}
	parseRun := renderers.NewRecordingRunner().On(snmpd.SnmpdBin, func(cmd renderers.Command) (renderers.Output, error) {
		for i, a := range cmd.Args { // the check instance's log: a clean parse
			if a == "-Lf" && i+1 < len(cmd.Args) {
				_ = os.WriteFile(cmd.Args[i+1], []byte("NET-SNMP version 5.9.4\n"), 0o600)
			}
			if a == "-p" && i+1 < len(cmd.Args) { // a PID that is not snmpd: never signalled (RF-4)
				_ = os.WriteFile(cmd.Args[i+1], fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600)
			}
		}
		return renderers.Output{}, nil
	})
	r := snmpd.New(parseRun, snmpd.WithPaths(p), snmpd.WithController(stoppedCtl{}),
		snmpd.WithSecretResolver(SnmpFixtureResolver(writeFixtures(t, secrets))))
	var logs bytes.Buffer
	st := NewSnmpStage(r, filepath.Join(dir, "state", "snmpd-w9.json"), nil, slog.New(slog.NewTextHandler(&logs, nil)))
	return st, p, &logs
}

func snmpValue() *vrxv1.SnmpService {
	return &vrxv1.SnmpService{
		Enabled: proto.Bool(true), SysName: proto.String("vrx-a"),
		Listen:      []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(3961)}},
		Communities: map[string]*vrxv1.SnmpService_Community{"ro": {SecretRef: proto.String("password/snmp-ro"), Sources: []string{"127.0.0.0/8"}}},
		V3Users: map[string]*vrxv1.SnmpService_V3User{"noc": {
			SecurityLevel: proto.String("authPriv"), AuthProtocol: proto.String("sha"), AuthRef: proto.String("password/noc-auth"),
			PrivProtocol: proto.String("aes"), PrivRef: proto.String("password/noc-priv"),
		}},
		Views: map[string]*vrxv1.SnmpView{"vrx": {Include: []string{"system", snmpagentPlaypen}}},
	}
}

const snmpagentPlaypen = ".1.3.6.1.4.1.8072.9999.9999"

var allFixtures = map[string]string{
	"password/snmp-ro": fixtureCommunity, "password/noc-auth": fixtureAuth, "password/noc-priv": fixturePriv, "password/other": fixtureOther,
}

// TestSnmpStageLifecycle: Create writes snmpd.conf (0600, secrets only there), a stopped daemon is a
// pending "start" (not a failure), Retrieve reports the applied value, drift makes it absent, Delete
// renders the disabled configuration and forgets the record; no secret reaches the record or the log.
func TestSnmpStageLifecycle(t *testing.T) {
	st, p, logs := newTestStage(t, allFixtures)
	ctx := context.Background()
	v := snmpValue()
	if _, err := st.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	conf, err := os.ReadFile(p.ConfFile)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p.ConfFile); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	for _, s := range []string{fixtureCommunity, fixtureAuth, fixturePriv} {
		if !bytes.Contains(conf, []byte(s)) {
			t.Fatalf("snmpd.conf lacks a resolved secret")
		}
	}
	state, err := st.State(ctx)
	if err != nil || !state.Configured || !strings.Contains(state.PendingAction, "start") {
		t.Fatalf("state %+v %v", state, err)
	}
	kvs, err := st.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Key != desired.SnmpKey || !proto.Equal(kvs[0].Value, v) {
		t.Fatalf("retrieve %v %v", kvs, err)
	}
	// drift: someone edited the file
	if err := os.WriteFile(p.ConfFile, append(conf, []byte("rocommunity public\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := st.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("drifted file still reported: %v", kvs)
	}
	if err := st.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	conf, _ = os.ReadFile(p.ConfFile)
	if bytes.Contains(conf, []byte(fixtureCommunity)) || bytes.Contains(conf, []byte("rocommunity")) {
		t.Fatalf("disabled rendering still grants access:\n%s", conf)
	}
	if kvs, _ := st.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("deleted config still reported: %v", kvs)
	}
	assertNoSecrets(t, "log", logs.String())
}

// TestSnmpStageRollbackRemovesCommunity: an Update to a second community followed by the scheduler's
// rollback (Update back) leaves exactly the original community in the file — and a removed community
// (Delete of the object) is gone from it.
func TestSnmpStageRollbackRemovesCommunity(t *testing.T) {
	st, p, _ := newTestStage(t, allFixtures)
	ctx := context.Background()
	v1 := snmpValue()
	v2 := proto.Clone(v1).(*vrxv1.SnmpService)
	v2.Communities["other"] = &vrxv1.SnmpService_Community{SecretRef: proto.String("password/other")}
	if _, err := st.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(ctx, v1, v2, nil); err != nil {
		t.Fatal(err)
	}
	if conf, _ := os.ReadFile(p.ConfFile); !bytes.Contains(conf, []byte(fixtureOther)) {
		t.Fatal("update did not add the community")
	}
	if _, err := st.Update(ctx, v2, v1, nil); err != nil { // rollback
		t.Fatal(err)
	}
	if conf, _ := os.ReadFile(p.ConfFile); bytes.Contains(conf, []byte(fixtureOther)) {
		t.Fatal("rollback left the added community in snmpd.conf")
	}
}

// TestSnmpProjectionChecksBeforeVPP (D-125): a bad services.snmp is a projection error with a JSON
// pointer — the transaction never reaches the scheduler, so no VPP write happens; the secret value is
// never in the message.
func TestSnmpProjectionChecksBeforeVPP(t *testing.T) {
	saved := desired.SnapshotSnmpChecks() // other tests' register() calls leave their owners registered
	desired.RestoreSnmpChecks(nil)
	t.Cleanup(func() { desired.RestoreSnmpChecks(saved) })
	st, _, _ := newTestStage(t, map[string]string{"password/snmp-ro": fixtureCommunity})
	desired.SetSnmpCheck("w9", st.Check)
	t.Cleanup(func() { desired.SetSnmpCheck("w9", nil) })
	sink := &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Snmp: snmpValue()}) // noc-auth is not resolvable
	if len(sink.errs) != 1 || !strings.HasPrefix(sink.errs[0], "/services/snmp/v3Users/noc/authRef ") || len(sink.kvs) != 0 {
		t.Fatalf("errors %v kvs %d", sink.errs, len(sink.kvs))
	}
	assertNoSecrets(t, "issue", strings.Join(sink.errs, "\n"))

	ok := snmpValue()
	ok.V3Users = nil
	sink = &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Snmp: ok, Dhcp: &vrxv1.DhcpService{Servers: map[string]*vrxv1.DhcpServer{"a": {}}}})
	if len(sink.errs) != 0 || len(sink.kvs) != 1 || len(sink.warns) != 1 {
		t.Fatalf("errs %v warns %v kvs %d", sink.errs, sink.warns, len(sink.kvs))
	}
	// disabled → no object (an applied config is replaced by the disabled rendering through Delete)
	sink = &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Snmp: &vrxv1.SnmpService{Enabled: proto.Bool(false)}})
	if len(sink.kvs) != 0 || len(sink.errs) != 0 {
		t.Fatalf("disabled: %+v", sink)
	}
}

func TestSnmpFixtureResolverRefusesRealSecrets(t *testing.T) {
	ctx := context.Background()
	if _, err := SnmpFixtureResolver("").Resolve(ctx, "password/x"); err == nil || !strings.Contains(err.Error(), "PENDING-secret-channel") {
		t.Fatalf("no channel: %v", err)
	}
	f := writeFixtures(t, map[string]string{"password/x": "public-community"})
	if _, err := SnmpFixtureResolver(f).Resolve(ctx, "password/x"); err == nil {
		t.Fatal("a non-fixture value was accepted")
	}
	if err := os.Chmod(f, 0o644); err != nil { //nolint:gosec // testing the mode check
		t.Fatal(err)
	}
	if _, err := SnmpFixtureResolver(f).Resolve(ctx, "password/x"); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("world-readable fixture file: %v", err)
	}
}

func assertNoSecrets(t *testing.T, what, s string) {
	t.Helper()
	for _, v := range allFixtures {
		if strings.Contains(s, v) {
			t.Fatalf("%s contains a secret value", what)
		}
	}
}

type recSink struct {
	kvs         []scheduler.KV
	errs, warns []string
}

func (r *recSink) Add(k scheduler.Key, v proto.Message, _ string) {
	r.kvs = append(r.kvs, scheduler.KV{Key: k, Value: v})
}

func (r *recSink) Errorf(pointer, _, format string, a ...any) {
	r.errs = append(r.errs, pointer+" "+fmtS(format, a...))
}

func (r *recSink) Warnf(pointer, _, format string, a ...any) {
	r.warns = append(r.warns, pointer+" "+fmtS(format, a...))
}

func fmtS(format string, a ...any) string { return fmt.Sprintf(format, a...) }

// TestServicesUnsupportedByReflection: every set services field not marked handled is reported,
// including fields added later; a field marked by its feature is not.
func TestServicesUnsupportedByReflection(t *testing.T) {
	sink := &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{
		Dhcp: &vrxv1.DhcpService{Servers: map[string]*vrxv1.DhcpServer{"a": {}}},
		Lldp: &vrxv1.LldpService{},
		Ntp:  &vrxv1.NtpService{},
		Snmp: &vrxv1.SnmpService{Enabled: proto.Bool(false)},
	})
	want := []string{"/services/dhcp", "/services/lldp", "/services/ntp"}
	if len(sink.warns) != len(want) {
		t.Fatalf("warnings %v", sink.warns)
	}
	for i, w := range want {
		if !strings.HasPrefix(sink.warns[i], w+" ") {
			t.Fatalf("warning %d = %q, want %s", i, sink.warns[i], w)
		}
	}
	desired.MarkServicesHandled("lldp")
	sink = &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Lldp: &vrxv1.LldpService{}})
	if len(sink.warns) != 0 {
		t.Fatalf("handled field still reported: %v", sink.warns)
	}
}

// TestSnmpCheckPerOwner: stages register per owner, Close unregisters; two owners fail closed.
func TestSnmpCheckPerOwner(t *testing.T) {
	saved := desired.SnapshotSnmpChecks() // other tests' register() calls leave their owners registered
	desired.RestoreSnmpChecks(nil)
	t.Cleanup(func() { desired.RestoreSnmpChecks(saved) })
	calls := 0
	desired.SetSnmpCheck("a", func(*vrxv1.SnmpService) error { calls++; return nil })
	t.Cleanup(func() { desired.SetSnmpCheck("a", nil); desired.SetSnmpCheck("b", nil) })
	v := snmpValue()
	sink := &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Snmp: v})
	if calls != 1 || len(sink.kvs) != 1 {
		t.Fatalf("calls %d kvs %d errs %v", calls, len(sink.kvs), sink.errs)
	}
	desired.SetSnmpCheck("b", func(*vrxv1.SnmpService) error { return nil })
	sink = &recSink{}
	desired.Snmp(sink, &vrxv1.ServicesConfig{Snmp: v})
	if len(sink.errs) != 1 || len(sink.kvs) != 0 {
		t.Fatalf("two owners: %+v", sink)
	}
	desired.SetSnmpCheck("b", nil)
	st, _, _ := newTestStage(t, allFixtures)
	st.owner = "c"
	desired.SetSnmpCheck("c", st.Check)
	snmpStages.Store("c", st)
	st.Close()
	if _, ok := SnmpStageOf("c"); ok {
		t.Fatal("Close left the stage registered")
	}
}
