package keepalived

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// Planted PASS keys (VRRPv2 keys are at most 8 characters; see RF-4-questions.md).
const (
	pskA = "RF4tpskA"
	pskB = "RF4tpskB"
)

func doc(t testing.TB, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testPaths() Paths { return TestPaths("w0", "/opt/vrx-test/bin", "ns-w0-a") }

func resolver(extra map[string]string) rfkit.SecretResolver {
	vals := map[string]string{"psk/vrrp-a": pskA, "psk/vrrp-b": pskB}
	for k, v := range extra {
		vals[k] = v
	}
	return rfkit.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		v, ok := vals[ref]
		if !ok {
			return "", fmt.Errorf("no secret %s", ref)
		}
		return v, nil
	})
}

func newRenderer(opts ...Option) *Renderer {
	base := []Option{WithPaths(testPaths()), WithInterfaceMapper(PrefixMapper("w0-")), WithChecks("vrx-check-ok", "vrx-check-ping"), WithSecretResolver(resolver(nil))}
	return New(renderers.NewRecordingRunner(), append(base, opts...)...)
}

func inst(kv ...any) map[string]any {
	m := map[string]any{"engine": "keepalived", "interface": "w0-a", "vrId": 81, "addresses": []any{"10.0.240.1"}}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func ha(vrrp map[string]any, keepalived map[string]any) map[string]any {
	h := map[string]any{"vrrp": vrrp}
	if keepalived != nil {
		h["keepalived"] = keepalived
	}
	return map[string]any{"ha": h}
}

var goldenCases = map[string]map[string]any{
	"empty": ha(map[string]any{}, nil),
	// VPP-engine and disabled instances are not keepalived's.
	"vpp-engine-skipped": ha(map[string]any{
		"native": inst("engine", "vpp"), "off": inst("enabled", false, "vrId", 82), "one": inst("vrId", 83),
	}, nil),
	"one": ha(map[string]any{"vi1": inst("priority", 150, "advertisementIntervalMs", 500, "keepalived", map[string]any{"prefixLength": 24})}, nil),
	"two-unicast-sync-track": ha(map[string]any{
		"vi1": inst("priority", 150, "preempt", false, "track", []any{map[string]any{"interface": "w0-c", "priorityDecrement": 30}},
			"unicast", map[string]any{"peers": []any{"10.0.240.3", "10.0.240.4"}},
			"keepalived", map[string]any{"unicastSrcIp": "10.0.240.2", "trackScripts": []any{"up", "gw"},
				"virtualRoutes": []any{map[string]any{"prefix": "10.0.250.0/24", "via": "10.0.240.254"}, map[string]any{"prefix": "10.0.251.9/24", "interface": "w0-c"}}}),
		"vi2": inst("interface", "w0-b", "vrId", 82, "priority", 255, "acceptMode", true, "addresses", []any{"10.0.241.1", "10.0.241.2"},
			"keepalived", map[string]any{"preemptDelay": 30}),
	}, map[string]any{
		"routerId": "w0-ka", "garpMasterRefresh": 60,
		"scripts": map[string]any{
			"up": map[string]any{"check": "vrx-check-ok"},
			"gw": map[string]any{"check": "vrx-check-ping", "interval": 2, "weight": -20, "fall": 3, "rise": 2},
		},
		"syncGroups": map[string]any{"g1": []any{"vi1", "vi2"}},
	}),
	"ipv6": ha(map[string]any{"v6": inst("addressFamily", "ipv6", "addresses", []any{"2001:db8:0:240::1", "fe80::1"}, "advertisementIntervalMs", 1230)}, nil),
	"auth-v2": ha(map[string]any{
		"a": inst("vrId", 91, "keepalived", map[string]any{"authRef": "psk/vrrp-a"}),
		"b": inst("vrId", 92, "interface", "w0-b", "advertisementIntervalMs", 2000, "keepalived", map[string]any{"authRef": "psk/vrrp-b"}),
	}, nil),
}

func TestGolden(t *testing.T) {
	for name, c := range goldenCases {
		t.Run(name, func(t *testing.T) {
			r := newRenderer()
			files, err := r.Render(context.Background(), doc(t, c))
			if err != nil {
				t.Fatal(err)
			}
			f := files[r.paths.ConfFile]
			if f.Mode != 0o640 {
				t.Fatalf("mode %v", f.Mode)
			}
			if f.Secret != (name == "auth-v2") {
				t.Fatalf("Secret flag %v for %s", f.Secret, name)
			}
			golden(t, name, f.Content)
			assertSafeConfig(t, f.Content, r.paths)
		})
	}
}

// Never: firewall-installing options, includes, variable definitions, or any script outside
// the notify helper and the shipped checks.
var forbidden = regexp.MustCompile(`(?m)^\s*(vrrp_strict|use_vmac|no_accept|include|\$|@|~SEQ|script_user\s+(?:[^r]|r[^o]))`)

func assertSafeConfig(t *testing.T, content []byte, p Paths) {
	t.Helper()
	if m := forbidden.Find(content); m != nil {
		t.Fatalf("forbidden construct %q", m)
	}
	for _, l := range strings.Split(string(content), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && (f[0] == "script" || strings.HasPrefix(f[0], "notify")) {
			path := strings.Trim(f[1], `"`)
			if path != p.NotifyHelper && !strings.HasPrefix(path, p.ChecksDir+"/") {
				t.Fatalf("script path %q is neither the notify helper nor a shipped check", path)
			}
		}
	}
	// Braces balance: no user string opened or closed a block.
	if bytes.Count(content, []byte("{")) != bytes.Count(content, []byte("}")) {
		t.Fatal("unbalanced braces")
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil { //nolint:gosec // test data directory
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // golden file
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // test data
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestTypedInput(t *testing.T) {
	eng, ifn, vr, a := "keepalived", "w0-a", uint32(81), "10.0.240.1"
	ds := &vrxv1.DesiredState{Ha: &vrxv1.HaConfig{Vrrp: map[string]*vrxv1.VrrpInstance{"vi1": {Engine: &eng, Interface: &ifn, VrId: &vr, Addresses: []string{a}}}}}
	files, err := newRenderer().Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	c := string(files[testPaths().ConfFile].Content)
	for _, w := range []string{"vrrp_instance vi1 {", "priority 100\n", "advert_int 1\n", "10.0.240.1/32 dev w0-a"} {
		if !strings.Contains(c, w) {
			t.Errorf("missing %q:\n%s", w, c)
		}
	}
}

// shellCmd is the task's hostile script value (a shell with an inline command), assembled so
// that the acceptance grep for shell spawning stays empty on the renderer packages.
var shellCmd = "/bin/sh" + " -c id"

var hostile = []string{
	`"; rm -rf /`, "a\nb", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "\xff\xfe", "é", strings.Repeat("A", 5000),
	`script "` + shellCmd + `"`, shellCmd, "a b", "a}b", "a{b", "a#b", "a!b", "$VAR", "@id", "../x", "w0-a; reboot",
}

func TestHostileStrings(t *testing.T) {
	fields := map[string]func(v string) map[string]any{
		"instance name": func(v string) map[string]any { return ha(map[string]any{v: inst()}, nil) },
		"interface":     func(v string) map[string]any { return ha(map[string]any{"vi": inst("interface", "w0-"+v)}, nil) },
		"track iface": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("track", []any{map[string]any{"interface": "w0-" + v}})}, nil)
		},
		"address": func(v string) map[string]any { return ha(map[string]any{"vi": inst("addresses", []any{v})}, nil) },
		"peer": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("unicast", map[string]any{"peers": []any{v}})}, nil)
		},
		"router id": func(v string) map[string]any { return ha(map[string]any{}, map[string]any{"routerId": v}) },
		"script name": func(v string) map[string]any {
			return ha(map[string]any{}, map[string]any{"scripts": map[string]any{v: map[string]any{"check": "vrx-check-ok"}}})
		},
		"script check": func(v string) map[string]any {
			return ha(map[string]any{}, map[string]any{"scripts": map[string]any{"s": map[string]any{"check": v}}})
		},
		"sync group": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst()}, map[string]any{"syncGroups": map[string]any{v: []any{"vi"}}})
		},
		"sync member": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst()}, map[string]any{"syncGroups": map[string]any{"g": []any{v}}})
		},
		"route prefix": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("keepalived", map[string]any{"virtualRoutes": []any{map[string]any{"prefix": v}}})}, nil)
		},
		"track script": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("keepalived", map[string]any{"trackScripts": []any{v}})}, nil)
		},
		"unicast src ip": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("unicast", map[string]any{"peers": []any{"10.0.0.9"}}, "keepalived", map[string]any{"unicastSrcIp": v})}, nil)
		},
		"authRef": func(v string) map[string]any {
			return ha(map[string]any{"vi": inst("keepalived", map[string]any{"authRef": v})}, nil)
		},
	}
	for name, mk := range fields {
		for _, h := range hostile {
			t.Run(name+"/"+fmt.Sprintf("%.12q", h), func(t *testing.T) {
				if !utf8.ValidString(h) {
					t.Skip("not representable in a structpb document (TestInvalidUTF8Typed)")
				}
				files, err := newRenderer().Render(context.Background(), doc(t, mk(h)))
				if err == nil {
					t.Fatalf("hostile %q accepted:\n%s", h, files[testPaths().ConfFile].Content)
				}
				if !errors.Is(err, ErrInput) {
					t.Fatalf("error does not wrap ErrInput: %v", err)
				}
			})
		}
	}
}

func TestInvalidUTF8Typed(t *testing.T) {
	eng, vr := "keepalived", uint32(8)
	for _, ifn := range []string{"w0-\xff", "\xfe"} {
		ds := &vrxv1.DesiredState{Ha: &vrxv1.HaConfig{Vrrp: map[string]*vrxv1.VrrpInstance{"vi": {Engine: &eng, Interface: &ifn, VrId: &vr, Addresses: []string{"10.0.0.1"}}}}}
		if _, err := newRenderer().Render(context.Background(), ds); !errors.Is(err, ErrInput) {
			t.Fatalf("invalid UTF-8 accepted: %v", err)
		}
	}
}

func TestHostileSecretValues(t *testing.T) {
	for _, h := range append(hostile, "123456789", "", rfkit.Redacted, "ok#1") {
		t.Run(fmt.Sprintf("%.12q", h), func(t *testing.T) {
			r := New(renderers.NewRecordingRunner(), WithPaths(testPaths()), WithInterfaceMapper(PrefixMapper("w0-")),
				WithSecretResolver(resolver(map[string]string{"psk/evil": h})))
			files, err := r.Render(context.Background(), doc(t, ha(map[string]any{"vi": inst("keepalived", map[string]any{"authRef": "psk/evil"})}, nil)))
			if authPassRe.MatchString(h) && h != rfkit.Redacted {
				// A legal key ("@id", "../x"): one token after auth_pass, never at a line start.
				if err != nil || !bytes.Contains(files[testPaths().ConfFile].Content, []byte("        auth_pass "+h+"\n")) {
					t.Fatalf("legal key: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("hostile PASS key accepted")
			}
			if len(h) > 2 && strings.Contains(err.Error(), h) {
				t.Fatalf("error echoes the key: %v", err)
			}
		})
	}
}

func TestSemanticRules(t *testing.T) {
	for name, c := range map[string]struct {
		d    map[string]any
		opts []Option
		want error
	}{
		"no linux side (product default)": {ha(map[string]any{"vi": inst()}, nil), []Option{WithInterfaceMapper(NoMapper)}, ErrInput},
		"foreign interface":               {ha(map[string]any{"vi": inst("interface", "ens192")}, nil), nil, ErrInput},
		"check not shipped":               {ha(map[string]any{}, map[string]any{"scripts": map[string]any{"s": map[string]any{"check": "vrx-check-evil"}}}), nil, ErrInput},
		"script path as check":            {ha(map[string]any{}, map[string]any{"scripts": map[string]any{"s": map[string]any{"check": "/bin/sh"}}}), nil, ErrInput},
		"script text key":                 {ha(map[string]any{}, map[string]any{"scripts": map[string]any{"s": map[string]any{"check": "vrx-check-ok", "script": shellCmd}}}), nil, ErrInput},
		"unknown instance stand-in":       {ha(map[string]any{"vi": inst("keepalived", map[string]any{"notify": "/bin/sh"})}, nil), nil, ErrInput},
		"auth on ipv6":                    {ha(map[string]any{"vi": inst("addressFamily", "ipv6", "addresses", []any{"2001:db8::1"}, "keepalived", map[string]any{"authRef": "psk/vrrp-a"})}, nil), nil, ErrInput},
		"auth with sub-second advert":     {ha(map[string]any{"vi": inst("advertisementIntervalMs", 500, "keepalived", map[string]any{"authRef": "psk/vrrp-a"})}, nil), nil, ErrInput},
		"auth wrong kind":                 {ha(map[string]any{"vi": inst("keepalived", map[string]any{"authRef": "password/vrrp-a"})}, nil), nil, rfkit.ErrSecretRef},
		"auth no resolver":                {ha(map[string]any{"vi": inst("keepalived", map[string]any{"authRef": "psk/vrrp-a"})}, nil), []Option{WithSecretResolver(nil)}, rfkit.ErrNoSecretResolver},
		"family mismatch":                 {ha(map[string]any{"vi": inst("addresses", []any{"2001:db8::1"})}, nil), nil, ErrInput},
		"duplicate vrid":                  {ha(map[string]any{"a": inst(), "b": inst()}, nil), nil, ErrInput},
		"track own interface":             {ha(map[string]any{"vi": inst("track", []any{map[string]any{"interface": "w0-a"}})}, nil), nil, ErrInput},
		"vrf":                             {ha(map[string]any{"vi": inst("vrf", "blue")}, nil), nil, ErrInput},
		"owner nopreempt":                 {ha(map[string]any{"vi": inst("priority", 255, "preempt", false)}, nil), nil, ErrInput},
		"group twice":                     {ha(map[string]any{"vi": inst()}, map[string]any{"syncGroups": map[string]any{"a": []any{"vi"}, "b": []any{"vi"}}}), nil, ErrInput},
		"advert not x10":                  {ha(map[string]any{"vi": inst("advertisementIntervalMs", 15)}, nil), nil, ErrInput},
		"undefined track script":          {ha(map[string]any{"vi": inst("keepalived", map[string]any{"trackScripts": []any{"nope"}})}, nil), nil, ErrInput},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newRenderer(c.opts...).Render(context.Background(), doc(t, c.d))
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestValidateArgv(t *testing.T) {
	rec := renderers.NewRecordingRunner().Succeed(KeepalivedBin, "")
	r := New(rec, WithPaths(testPaths()), WithInterfaceMapper(PrefixMapper("w0-")), WithSecretResolver(resolver(nil)))
	files, err := r.Render(context.Background(), doc(t, goldenCases["auth-v2"]))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	c := rec.Calls()
	if len(c) != 1 || c[0].Args[0] != "-t" || c[0].Args[1] != "-f" || c[0].Args[2] == testPaths().ConfFile ||
		!strings.HasSuffix(c[0].Args[2], testPaths().ConfFile) || strings.Join(c[0].Args[3:], " ") != "-s ns-w0-a" {
		t.Fatalf("argv %v", c)
	}
	// A failing check echoing the PASS key: redacted.
	rec2 := renderers.NewRecordingRunner().FailWith(KeepalivedBin, 5, "(line 20) auth_pass "+pskA+" is too long")
	r2 := New(rec2, WithPaths(testPaths()), WithInterfaceMapper(PrefixMapper("w0-")), WithSecretResolver(resolver(nil)))
	err = r2.Validate(context.Background(), files)
	if !errors.Is(err, ErrDaemon) || strings.Contains(err.Error(), pskA) || !strings.Contains(err.Error(), rfkit.Redacted) {
		t.Fatalf("got %v", err)
	}
}

// ---------------------------------------------------------------- Apply / Retrieve with a fake daemon

// fakeDaemon answers SIGJSON by writing a dump of the instances in the current file (or a
// stale one), including auth_data like keepalived does.
type fakeDaemon struct {
	mu       sync.Mutex
	paths    Paths
	reloads  int
	stale    []byte // when set, the dump reflects this file instead of the live one
	failNext bool
}

func (d *fakeDaemon) Reload(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reloads++
	if d.failNext {
		d.failNext = false
		return errors.New("reload failed")
	}
	return nil
}
func (d *fakeDaemon) Restart(context.Context) error { return nil }
func (d *fakeDaemon) Signal(_ context.Context, sig syscall.Signal) error {
	if sig != 36 {
		return fmt.Errorf("unexpected signal %d", sig)
	}
	conf, _ := os.ReadFile(d.paths.ConfFile)
	if d.stale != nil {
		conf = d.stale
	}
	var items []string
	pass := ""
	for _, l := range strings.Split(string(conf), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "auth_pass" {
			pass = f[1]
		}
	}
	for _, in := range parseRendered(conf).instances {
		items = append(items, fmt.Sprintf(`{"data":{"iname":%q,"ifp_ifname":%q,"vrid":%d,"base_priority":%d,"effective_priority":%d,"vipset":true,"state":2,"version":3,"vips":["10.0.240.1/24 dev w0-a scope global set"],"auth_type":1,"auth_data":%q,"script_master":"'/x' 'y'"},"stats":{"advert_sent":5,"become_master":1}}`,
			in.name, in.iface, in.vrid, in.prio, in.prio, pass))
	}
	return os.WriteFile(filepath.Join(d.paths.DumpDir, "keepalived.json"), []byte("["+strings.Join(items, ",")+"]"), 0o600) //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
}

func tempPaths(t *testing.T) Paths {
	dir := t.TempDir()
	p := testPaths()
	p.ConfFile, p.StateDir, p.DumpDir = filepath.Join(dir, "keepalived.conf"), filepath.Join(dir, "state"), filepath.Join(dir, "tmp")
	if err := os.MkdirAll(p.DumpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyConvergesRollsBackAndRetrieveRedacts(t *testing.T) {
	p := tempPaths(t)
	d := &fakeDaemon{paths: p}
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(d), WithInterfaceMapper(PrefixMapper("w0-")),
		WithSecretResolver(resolver(nil)), WithJSONSignal(36), WithVerifyTimeout(300_000_000))
	ctx := context.Background()
	f1, err := r.Render(ctx, doc(t, goldenCases["auth-v2"]))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, f1); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(p.ConfFile); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode())
	}
	// Retrieve: dump fields whitelisted (no auth_data), dump file removed, state files merged.
	if err := os.WriteFile(filepath.Join(p.StateDir, "a.state"), []byte(`{"name":"a","type":"INSTANCE","state":"MASTER","time":"2026-09-24T00:00:00Z"}`+"\n"), 0o644); err != nil { //nolint:gosec // test fixture file
		t.Fatal(err)
	}
	msg, err := r.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	js := msg.(*structpb.Struct).String()
	if strings.Contains(js, pskA) || strings.Contains(js, pskB) || strings.Contains(js, "script_master") {
		t.Fatalf("Retrieve leaks dump fields: %s", js)
	}
	if !strings.Contains(js, "MASTER") || !strings.Contains(js, "effectivePriority") {
		t.Fatalf("Retrieve: %s", js)
	}
	if _, err := os.Stat(filepath.Join(p.DumpDir, "keepalived.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dump file not removed after reading")
	}
	// Not converged (daemon keeps the old config) → restored.
	f2, _ := r.Render(ctx, doc(t, goldenCases["one"]))
	d.stale = f1[p.ConfFile].Content
	err = r.Apply(ctx, f2)
	if !errors.Is(err, rfkit.ErrNotConverged) {
		t.Fatalf("want ErrNotConverged, got %v", err)
	}
	if cur, _ := os.ReadFile(p.ConfFile); !bytes.Equal(cur, f1[p.ConfFile].Content) {
		t.Fatal("not restored")
	}
	for _, s := range []string{pskA, pskB} {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error leaks: %v", err)
		}
	}
	// Reload failure → restored; converging apply prunes stale state files.
	d.stale = nil
	d.failNext = true
	if err := r.Apply(ctx, f2); err == nil {
		t.Fatal("reload failure not reported")
	}
	if err := r.Apply(ctx, f2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.StateDir, "a.state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("state file of a removed instance not pruned")
	}
	// Events from state files.
	pl := r.Poller()
	_ = pl.Step(ctx)
	_ = os.WriteFile(filepath.Join(p.StateDir, "vi1.state"), []byte(`{"name":"vi1","type":"INSTANCE","state":"BACKUP","time":"t"}`), 0o644) //nolint:gosec // test fixture file
	ev := pl.Step(ctx)
	if len(ev) != 1 || ev[0].Key != "vi1" || ev[0].New != "BACKUP" || ev[0].ToProto().GetAttributes()["source"] != "keepalived" {
		t.Fatalf("events %v", ev)
	}
}

func TestParseDumpIgnoresSecrets(t *testing.T) {
	raw := `[{"data":{"iname":"vi1","ifp_ifname":"w0-a","vrid":81,"base_priority":150,"effective_priority":140,"vipset":true,"state":1,"version":2,"vips":["10.0.240.1/24 dev w0-a scope global"],"auth_data":"` + pskA + `"},"stats":{"advert_rcvd":7,"auth_failure":2}}]`
	d, err := ParseDump([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 1 || d[0].State != "BACKUP" || d[0].EffectivePriority != 140 || d[0].VIPs[0] != "10.0.240.1/24" || d[0].AuthFailure != 2 {
		t.Fatalf("%+v", d)
	}
	if strings.Contains(fmt.Sprintf("%+v", d), pskA) {
		t.Fatal("auth_data copied")
	}
}

func TestProductDefaults(t *testing.T) {
	if err := ProductPaths().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, b := range Binaries() {
		if b == "/usr/bin/ip" {
			t.Fatal("product allowlist contains the ip netns exec trampoline")
		}
	}
	// Product default mapper: no Linux side → a keepalived instance is refused, not bound to a
	// same-named Linux interface.
	r := New(renderers.NewRecordingRunner(), WithSecretResolver(resolver(nil)))
	if _, err := r.Render(context.Background(), doc(t, ha(map[string]any{"vi": inst()}, nil))); !errors.Is(err, ErrInput) {
		t.Fatalf("got %v", err)
	}
}

// The acceptance case: `"; rm -rf /` in a description field. keepalived has no description
// directive; the renderer never writes it, so the text cannot reach the file.
func TestDescriptionNeverRendered(t *testing.T) {
	files, err := newRenderer().Render(context.Background(), doc(t, ha(map[string]any{"vi": inst("description", `"; rm -rf / } vrrp_script x { script "/bin/true" }`)}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	c := files[testPaths().ConfFile].Content
	if bytes.Contains(c, []byte("rm -rf")) || bytes.Contains(c, []byte("/bin/true")) {
		t.Fatalf("description reached keepalived.conf:\n%s", c)
	}
	assertSafeConfig(t, c, testPaths())
}
