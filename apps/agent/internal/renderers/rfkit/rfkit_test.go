package rfkit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
)

func TestSystemdControllerArgv(t *testing.T) {
	rec := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
	c := &SystemdController{Runner: rec, Unit: "keepalived"}
	ctx := context.Background()
	if err := c.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Signal(ctx, 36); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, cmd := range rec.Calls() {
		got = append(got, strings.Join(cmd.Args, " "))
	}
	want := []string{"reload keepalived", "restart keepalived", "kill --kill-whom=main --signal=36 keepalived"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("argv %q, want %q", got, want)
	}
	for _, bad := range []string{"", "vpp; reboot", "../x", "Snmpd"} {
		if err := (&SystemdController{Runner: rec, Unit: bad}).Reload(ctx); err == nil {
			t.Errorf("unit %q accepted", bad)
		}
	}
	if err := c.Signal(ctx, 0); err == nil {
		t.Error("signal 0 accepted")
	}
}

func TestProcessControllerRefusesForeignPID(t *testing.T) {
	cmd := exec.Command("/usr/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := func() (int, error) { return cmd.Process.Pid, nil }
	// A PID running another binary is never signalled (recycled PID guard).
	c := &ProcessController{PID: pid, Binary: "/usr/sbin/snmpd"}
	if err := c.Reload(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("got %v", err)
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatal("the foreign process was signalled")
	}
	ok := &ProcessController{PID: pid, Binary: "/usr/bin/sleep"}
	if err := ok.Signal(context.Background(), syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if err := ok.Restart(context.Background()); err == nil {
		t.Fatal("restart without hook accepted")
	}
	// Pidfile parsing.
	dir := t.TempDir()
	pf := filepath.Join(dir, "x.pid")
	for content, good := range map[string]bool{strconv.Itoa(cmd.Process.Pid) + "\n": true, "1\n": false, "abc": false, "": false} {
		_ = os.WriteFile(pf, []byte(content), 0o600)
		_, err := PIDFile(pf)()
		if (err == nil) != good {
			t.Errorf("pidfile %q: %v", content, err)
		}
	}
}

func TestSecretsAndRedactor(t *testing.T) {
	for ref, ok := range map[string]bool{"password/a": true, "psk/x-1.y_z": true, "public": false, "password/": false, "pw/a": false, "password/a b": false} {
		if (CheckRef(ref) == nil) != ok {
			t.Errorf("CheckRef(%q)", ref)
		}
	}
	if err := CheckRef("key/a", "password"); !errors.Is(err, ErrSecretRef) {
		t.Error("kind not enforced")
	}
	calls := 0
	s := &Secrets{Resolver: SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		calls++
		return map[string]string{"password/a": "VRX_TEST_PSK_RF4_a", "password/e": "", "password/r": "x" + Redacted}[ref], nil //nolint:gosec // test fixture, not a credential
	})}
	v, err := s.Resolve("password/a", nil)
	if err != nil || v != "VRX_TEST_PSK_RF4_a" {
		t.Fatal(v, err)
	}
	_, _ = s.Resolve("password/a", nil)
	if calls != 1 {
		t.Fatal("resolved twice in one render")
	}
	for _, ref := range []string{"password/e", "password/r"} {
		if _, err := s.Resolve(ref, nil); !errors.Is(err, ErrSecretValue) {
			t.Errorf("%s: %v", ref, err)
		}
	}
	_, err = s.Resolve("password/a2", func(string) error { return errors.New("bad shape") })
	if err != nil && strings.Contains(err.Error(), "VRX_TEST") {
		t.Fatal("error echoes the value")
	}
	if _, err := (&Secrets{}).Resolve("password/a", nil); !errors.Is(err, ErrNoSecretResolver) {
		t.Fatal(err)
	}

	var r Redactor
	pem := "-----BEGIN PRIVATE KEY-----\nVRX_TEST_PSK_RF4_line_one\nshort\n-----END PRIVATE KEY-----\n"
	r.Add("VRX_TEST_PSK_RF4_a", "VRX_TEST_PSK_RF4_ab", pem, "")
	got := r.Redact("x VRX_TEST_PSK_RF4_ab y VRX_TEST_PSK_RF4_a | VRX_TEST_PSK_RF4_line_one | short")
	if got != "x <redacted> y <redacted> | <redacted> | short" {
		t.Fatalf("got %q", got)
	}
	base := errors.New("boom VRX_TEST_PSK_RF4_a")
	re := r.Error(base)
	if strings.Contains(re.Error(), "VRX_TEST") || !errors.Is(re, base) {
		t.Fatalf("got %v", re)
	}
	if r.Error(nil) != nil {
		t.Fatal("nil error")
	}
}

func TestApplyFilesRollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.conf")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := renderers.Files{path: {Mode: 0o600, Content: []byte("new\n")}}
	activations := 0
	activate := func(context.Context) error { activations++; return nil }
	err := ApplyFiles(context.Background(), files, activate, func(context.Context) error {
		return errors.Join(ErrNotConverged, errors.New("daemon shows old"))
	})
	if !errors.Is(err, ErrNotConverged) {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "old\n" || activations != 2 { //nolint:gosec // test temp dir / slot dir
		t.Fatalf("content %q activations %d", b, activations)
	}
	// Cancelled caller context: the rollback still activates the old configuration.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var rollbackCtxErr error
	n := 0
	err = ApplyFiles(ctx, files, func(c context.Context) error {
		n++
		if n == 1 {
			return c.Err()
		}
		rollbackCtxErr = c.Err()
		return nil
	}, nil)
	if err == nil || rollbackCtxErr != nil {
		t.Fatalf("err %v, rollback ctx %v", err, rollbackCtxErr)
	}
	// No previous file: removed again, nothing re-activated.
	fresh := filepath.Join(dir, "fresh.conf")
	activations = 0
	_ = ApplyFiles(context.Background(), renderers.Files{fresh: {Mode: 0o600, Content: []byte("x")}}, activate,
		func(context.Context) error { return ErrNotConverged })
	if _, err := os.Stat(fresh); !errors.Is(err, os.ErrNotExist) || activations != 1 {
		t.Fatalf("fresh file left behind or re-activated (%d)", activations)
	}
	// Success.
	if err := ApplyFiles(context.Background(), files, activate, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPoller(t *testing.T) {
	states := []map[string]string{{"a": "1"}, {"a": "2", "b": "x"}, nil, nil, {"b": "x"}}
	errs := []error{nil, nil, errors.New("down VRX_TEST_PSK_RF4_a"), errors.New("down VRX_TEST_PSK_RF4_a"), nil}
	i := 0
	var red Redactor
	red.Add("VRX_TEST_PSK_RF4_a")
	p := &Poller{Source: "d", Redact: red.Redact, Snap: func(context.Context) (map[string]string, error) {
		defer func() { i++ }()
		return states[i], errs[i]
	}}
	ctx := context.Background()
	var all [][]Event
	for range states {
		all = append(all, p.Step(ctx))
	}
	if len(all[0]) != 0 || len(all[1]) != 2 || len(all[2]) != 1 || len(all[3]) != 0 || len(all[4]) != 1 {
		t.Fatalf("%v", all)
	}
	if all[2][0].Error != "down <redacted>" || all[2][0].ToProto().Kind.String() != "EVENT_KIND_ERROR" {
		t.Fatalf("%+v", all[2][0])
	}
	if all[4][0].Key != "a" || all[4][0].New != "" || all[4][0].String() != "d a: 2 -> -" {
		t.Fatalf("%+v", all[4][0])
	}
	// Run stops with the context.
	ch := make(chan Event, 10)
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	(&Poller{Source: "d", Snap: func(context.Context) (map[string]string, error) { return map[string]string{}, nil }}).Run(ctx2, 10*time.Millisecond, ch)
}

func TestExtAndDecode(t *testing.T) {
	doc, _ := structpb.NewStruct(map[string]any{
		"services": map[string]any{"snmp": map[string]any{"enabled": true, "x": map[string]any{"n": 5, "neg": -3, "s": "v", "l": []any{"a", "b"}, "b": true, "f": 1.5}}},
	})
	ds, ext, err := Decode(doc)
	if err != nil || !ds.GetServices().GetSnmp().GetEnabled() {
		t.Fatal(err)
	}
	x := ext.Get("services", "snmp", "x")
	if n, ok, err := x.Get("n").Uint(1, 10); n != 5 || !ok || err != nil {
		t.Fatal(n, ok, err)
	}
	if _, _, err := x.Get("f").Uint(0, 10); err == nil {
		t.Fatal("fraction accepted")
	}
	if n, _, err := x.Get("neg").Int(-5, 5); n != -3 || err != nil {
		t.Fatal(n, err)
	}
	if l, err := x.Get("l").Strings(); len(l) != 2 || err != nil {
		t.Fatal(l, err)
	}
	if _, _, err := x.Get("s").Bool(); !errors.Is(err, ErrExt) {
		t.Fatal(err)
	}
	if err := x.OnlyKeys("n", "neg"); !errors.Is(err, ErrExt) {
		t.Fatal("unknown keys accepted")
	}
	if v, ok, err := x.Get("missing").String(); v != "" || ok || err != nil {
		t.Fatal("absent key")
	}
	var nilExt *Ext
	if nilExt.Get("a") != nil || nilExt.Path() != "" {
		t.Fatal("nil Ext")
	}
	if _, _, err := Decode(&structpb.Value{}); !errors.Is(err, ErrExt) {
		t.Fatal("unsupported input accepted")
	}
}

func TestBoundedReads(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	_ = os.WriteFile(p, []byte(strings.Repeat("line\n", 1000)), 0o600)
	if _, err := ReadFileLimit(p, 100); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	b, size, err := ReadTail(p, 23)
	if err != nil || size != 5000 || string(b) != "line\nline\nline\nline\n" {
		t.Fatalf("%q %d %v", b, size, err)
	}
}
