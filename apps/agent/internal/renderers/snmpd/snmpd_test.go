package snmpd

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

// Planted secrets: every test that renders checks they appear only where they must.
const (
	secRO   = "VRX_TEST_PSK_RF4_ro"
	secRW   = "VRX_TEST_PSK_RF4_rw"
	secAuth = "VRX_TEST_PSK_RF4_auth"
	secPriv = "VRX_TEST_PSK_RF4_priv"
	secU2   = "VRX_TEST_PSK_RF4_u2auth"
)

var plantedSecrets = []string{secRO, secRW, secAuth, secPriv, secU2}

func resolver(extra map[string]string) rfkit.SecretResolver {
	vals := map[string]string{
		"password/snmp-ro": secRO, "password/snmp-rw": secRW,
		"password/u1-auth": secAuth, "password/u1-priv": secPriv, "password/u2-auth": secU2,
	}
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

func doc(t testing.TB, snmp map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(map[string]any{"services": map[string]any{"snmp": snmp}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newRenderer(t testing.TB, opts ...Option) *Renderer {
	t.Helper()
	base := append([]Option{WithPaths(TestPaths("w0")), WithSecretResolver(resolver(nil))}, opts...)
	return New(renderers.NewRecordingRunner(), base...)
}

func base() map[string]any {
	return map[string]any{
		"enabled": true, "sysName": "vrx-w8", "sysLocation": "Rack 4, Hall B", "sysContact": "noc@example.net",
		"listen": []any{map[string]any{"address": "127.0.0.1", "port": 3861}},
	}
}

func with(m map[string]any, kv ...any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

var goldenCases = map[string]map[string]any{
	"disabled": {"enabled": false, "sysName": "ignored"},
	"v2c": with(base(), "communities", map[string]any{
		"ro":  map[string]any{"secretRef": "password/snmp-ro", "sources": []any{"127.0.0.1/32", "10.8.0.7/16", "2001:db8:8::/48"}},
		"rw":  map[string]any{"secretRef": "password/snmp-rw", "access": "rw", "sources": []any{"10.8.1.0/24"}},
		"any": map[string]any{"secretRef": "password/snmp-ro"},
	}),
	"v3": with(base(), "v3Users", map[string]any{
		"u1":   map[string]any{"securityLevel": "authPriv", "authProtocol": "sha256", "authRef": "password/u1-auth", "privProtocol": "aes", "privRef": "password/u1-priv"},
		"u2":   map[string]any{"securityLevel": "authNoPriv", "authProtocol": "sha512", "authRef": "password/u2-auth", "access": "rw"},
		"mon":  map[string]any{"securityLevel": "noAuthNoPriv", "view": "sys"},
		"md5d": map[string]any{"securityLevel": "authPriv", "authProtocol": "md5", "authRef": "password/u2-auth", "privProtocol": "des", "privRef": "password/u1-priv"},
	}, "views", map[string]any{"sys": map[string]any{"include": []any{"system"}}}),
	"traps": with(base(),
		"communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "sources": []any{"127.0.0.1/32"}}},
		"v3Users", map[string]any{"u1": map[string]any{"securityLevel": "authPriv", "authProtocol": "sha", "authRef": "password/u1-auth", "privRef": "password/u1-priv"}},
		"trapReceivers", []any{
			map[string]any{"address": "192.0.2.10", "version": "v2c", "community": "ro"},
			map[string]any{"address": "nms.example.net", "port": 10162, "version": "v2c", "community": "ro", "inform": true},
			map[string]any{"address": "2001:db8::10", "version": "v3", "user": "u1"},
			map[string]any{"address": "192.0.2.11", "version": "v3", "user": "u1", "inform": true},
		}),
	"full": with(base(),
		"engineId", "800007E5804A1B2C3D",
		"sysServices", 78,
		"listen", []any{map[string]any{"address": "127.0.0.1", "port": 3861}, map[string]any{"address": "::1", "port": 3861}, map[string]any{"address": "10.8.0.1"}},
		"communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "sources": []any{"127.0.0.1/32"}, "view": "ifs"}},
		"v3Users", map[string]any{"u1": map[string]any{"authProtocol": "sha256", "authRef": "password/u1-auth", "privRef": "password/u1-priv", "view": "ifs"}},
		"views", map[string]any{
			"ifs":  map[string]any{"include": []any{"system", "interfaces", "ifMIB", "1.3.6.1.4.1.2021.10"}, "exclude": []any{".1.3.6.1.2.1.1.9"}},
			"all2": map[string]any{"include": []any{"all"}},
		},
		"monitors", map[string]any{
			"disks": []any{map[string]any{"path": "/"}, map[string]any{"path": "/var/log", "minPercent": 20}},
			"load":  map[string]any{"max1": 12, "max5": 10, "max15": 8},
		}),
	"listen-any": with(base(), "listen", []any{}, "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro"}}),
	// A description-like field carrying shell syntax is plain printable text: it is rendered as
	// the rest of the sysLocation line and nothing else (the acceptance "; rm -rf / case).
	"hostile-location": with(base(), "sysLocation", `"; rm -rf / $(reboot) `+"`id`"+` # {x} | y \ z`),
}

func TestGolden(t *testing.T) {
	for name, c := range goldenCases {
		t.Run(name, func(t *testing.T) {
			r := newRenderer(t)
			files, err := r.Render(context.Background(), doc(t, c))
			if err != nil {
				t.Fatal(err)
			}
			f := files[r.paths.ConfFile]
			if f.Mode != 0o600 || !f.Secret {
				t.Fatalf("snmpd.conf must be 0600 and Secret, got %v secret=%v", f.Mode, f.Secret)
			}
			golden(t, name, f.Content)
			assertNoForeignDirectives(t, f.Content)
		})
	}
}

// Every rendered line starts with a directive the template emits (no injected lines).
var allowedDirective = regexp.MustCompile(`^(#.*|agentaddress|dontLogTCPWrappersConnects|exactEngineID|sysName|sysLocation|sysContact|sysServices|view|rocommunity6?|rwcommunity6?|createUser|rouser|rwuser|trap2sink|informsink|trapsess|master|agentXSocket|agentXPerms|disk|load)( |$)`)

func assertNoForeignDirectives(t *testing.T, content []byte) {
	t.Helper()
	for i, l := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
		if l != "" && !allowedDirective.MatchString(l) {
			t.Errorf("line %d %q is not a directive the renderer emits", i+1, l)
		}
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
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

func TestRenderDeterministic(t *testing.T) {
	r := newRenderer(t)
	a, err := r.Render(context.Background(), doc(t, goldenCases["full"]))
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		b, _ := r.Render(context.Background(), doc(t, goldenCases["full"]))
		if !bytes.Equal(a[r.paths.ConfFile].Content, b[r.paths.ConfFile].Content) {
			t.Fatal("render is not deterministic")
		}
	}
}

func TestTypedInput(t *testing.T) {
	en, port := true, uint32(3861)
	loc, ref, src := "Hall A", "password/snmp-ro", "127.0.0.1/32"
	ds := &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Snmp: &vrxv1.SnmpService{
		Enabled: &en, SysLocation: &loc,
		Listen:      []*vrxv1.SocketAddress{{Address: strp("127.0.0.1"), Port: &port}},
		Communities: map[string]*vrxv1.SnmpService_Community{"ro": {SecretRef: &ref, Sources: []string{src}}},
	}}}
	r := newRenderer(t)
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	c := string(files[r.paths.ConfFile].Content)
	for _, want := range []string{"agentaddress udp:127.0.0.1:3861\n", "sysLocation Hall A\n", "rocommunity " + secRO + " 127.0.0.1/32 -V vrx_all\n"} {
		if !strings.Contains(c, want) {
			t.Errorf("missing %q in\n%s", want, c)
		}
	}
}

func strp(s string) *string { return &s }

// hostile is the framework's list plus the task's cases.
var hostile = []string{
	`"; rm -rf /`, "a\nb", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "\xff\xfe", "\nrocommunity public\n",
	"é unicode", strings.Repeat("A", 5000), " leading", "trailing ", "a\tb",
}

func TestHostileStrings(t *testing.T) {
	type field struct {
		name string
		mk   func(v string) map[string]any
		// verbatim: printable ASCII is legal here (rest-of-line text) and must stay on its line.
		verbatim bool
	}
	fields := []field{
		{"sysLocation", func(v string) map[string]any { return with(base(), "sysLocation", v) }, true},
		{"sysContact", func(v string) map[string]any { return with(base(), "sysContact", v) }, true},
		{"sysName", func(v string) map[string]any { return with(base(), "sysName", v) }, false},
		{"community name", func(v string) map[string]any {
			return with(base(), "communities", map[string]any{v: map[string]any{"secretRef": "password/snmp-ro"}})
		}, false},
		{"community source", func(v string) map[string]any {
			return with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "sources": []any{v}}})
		}, false},
		{"secretRef", func(v string) map[string]any {
			return with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": v}})
		}, false},
		{"user name", func(v string) map[string]any {
			return with(base(), "v3Users", map[string]any{v: map[string]any{"authRef": "password/u1-auth", "privRef": "password/u1-priv"}})
		}, false},
		{"view name", func(v string) map[string]any {
			return with(base(), "views", map[string]any{v: map[string]any{"include": []any{"system"}}})
		}, false},
		{"view oid", func(v string) map[string]any {
			return with(base(), "views", map[string]any{"v": map[string]any{"include": []any{v}}})
		}, false},
		{"trap host", func(v string) map[string]any {
			return with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro"}},
				"trapReceivers", []any{map[string]any{"address": v, "community": "ro"}})
		}, false},
		{"listen address", func(v string) map[string]any {
			return with(base(), "listen", []any{map[string]any{"address": v}})
		}, false},
		{"disk path", func(v string) map[string]any {
			return with(base(), "monitors", map[string]any{"disks": []any{map[string]any{"path": "/" + v}}})
		}, false},
	}
	for _, f := range fields {
		for _, h := range hostile {
			t.Run(f.name+"/"+fmt.Sprintf("%.12q", h), func(t *testing.T) {
				if !utf8.ValidString(h) {
					t.Skip("not representable in a structpb document (see TestInvalidUTF8Typed)")
				}
				r := newRenderer(t)
				files, err := r.Render(context.Background(), doc(t, f.mk(h)))
				printable := regexp.MustCompile(`^[!-~][ -~]{0,253}[!-~]$`).MatchString(h)
				if f.verbatim && printable {
					if err != nil {
						t.Fatalf("printable text rejected: %v", err)
					}
					c := files[r.paths.ConfFile].Content
					assertNoForeignDirectives(t, c)
					if !bytes.Contains(c, []byte(" "+h+"\n")) {
						t.Fatalf("value not rendered verbatim on one line:\n%s", c)
					}
					return
				}
				if err == nil {
					t.Fatalf("hostile %q accepted:\n%s", h, files[r.paths.ConfFile].Content)
				}
				if !errors.Is(err, ErrInput) && !errors.Is(err, rfkit.ErrSecretRef) {
					t.Fatalf("error does not wrap ErrInput: %v", err)
				}
			})
		}
	}
}

func TestInvalidUTF8Typed(t *testing.T) {
	en := true
	for _, ds := range []*vrxv1.DesiredState{
		{Services: &vrxv1.ServicesConfig{Snmp: &vrxv1.SnmpService{Enabled: &en, SysLocation: strp("\xff\xfe")}}},
		{Services: &vrxv1.ServicesConfig{Snmp: &vrxv1.SnmpService{Enabled: &en, SysName: strp("a\xffb")}}},
		{Services: &vrxv1.ServicesConfig{Snmp: &vrxv1.SnmpService{Enabled: &en, V3Users: map[string]*vrxv1.SnmpService_V3User{"\xff": {}}}}},
	} {
		if _, err := newRenderer(t).Render(context.Background(), ds); !errors.Is(err, ErrInput) {
			t.Fatalf("invalid UTF-8 accepted: %v", err)
		}
	}
}

func TestHostileSecretValues(t *testing.T) {
	for _, h := range append(hostile, "pub lic", `pub"lic`, `pub\lic`, "#public", "short", rfkit.Redacted) {
		for _, which := range []string{"community", "passphrase"} {
			t.Run(which+"/"+fmt.Sprintf("%.12q", h), func(t *testing.T) {
				var snmp map[string]any
				if which == "community" {
					snmp = with(base(), "communities", map[string]any{"x": map[string]any{"secretRef": "password/evil"}})
				} else {
					snmp = with(base(), "v3Users", map[string]any{"u": map[string]any{"authRef": "password/evil", "privRef": "password/u1-priv"}})
				}
				r := New(renderers.NewRecordingRunner(), WithPaths(TestPaths("w0")), WithSecretResolver(resolver(map[string]string{"password/evil": h})))
				_, err := r.Render(context.Background(), doc(t, snmp))
				ok := regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`).MatchString(h)
				if which == "passphrase" {
					ok = regexp.MustCompile(`^[A-Za-z0-9_.,:;@%+=/~^*!?-]{8,64}$`).MatchString(h)
				}
				if h == rfkit.Redacted {
					ok = false
				}
				if ok {
					if err != nil {
						t.Fatalf("valid value rejected: %v", err)
					}
					return
				}
				if err == nil {
					t.Fatal("hostile secret accepted")
				}
				if len(h) > 3 && strings.Contains(err.Error(), h) {
					t.Fatalf("error echoes the secret value: %v", err)
				}
			})
		}
	}
}

func TestSecretRefRules(t *testing.T) {
	for _, tc := range []struct {
		name string
		snmp map[string]any
		opts []Option
		want error
	}{
		{"wrong kind", with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "key/snmp-ro"}}), nil, rfkit.ErrSecretRef},
		{"inline value", with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "public"}}), nil, rfkit.ErrSecretRef},
		{"no resolver", with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro"}}), []Option{WithSecretResolver(nil)}, rfkit.ErrNoSecretResolver},
		{"missing auth", with(base(), "v3Users", map[string]any{"u": map[string]any{"securityLevel": "authPriv", "privRef": "password/u1-priv"}}), nil, rfkit.ErrSecretRef},
		{"aes256 unsupported", with(base(), "v3Users", map[string]any{"u": map[string]any{"authRef": "password/u1-auth", "privProtocol": "aes256", "privRef": "password/u1-priv"}}), nil, ErrInput},
		{"vrf", with(base(), "vrf", "mgmt"), nil, ErrInput},
		{"undefined view", with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "view": "nope"}}), nil, ErrInput},
		{"unknown stand-in key", with(base(), "monitors", map[string]any{"disks": []any{}, "exec": "/bin/sh"}), nil, ErrInput},
		{"trap unknown community", with(base(), "trapReceivers", []any{map[string]any{"address": "192.0.2.1", "community": "nope"}}), nil, ErrInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRenderer(t, tc.opts...)
			_, err := r.Render(context.Background(), doc(t, tc.snmp))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------- Validate (parse run)

func TestCheckCopy(t *testing.T) {
	r := newRenderer(t)
	files, err := r.Render(context.Background(), doc(t, goldenCases["traps"]))
	if err != nil {
		t.Fatal(err)
	}
	got := string(CheckCopy(files[r.paths.ConfFile].Content, "/tmp/stage/check"))
	for _, bad := range []string{"trap2sink ", "informsink ", "trapsess ", "udp:127.0.0.1:3861", "agentx.sock\n"} {
		if strings.Contains(got, bad) && bad != "agentx.sock\n" {
			t.Errorf("check copy still contains %q", bad)
		}
	}
	for _, want := range []string{"agentaddress unix:/tmp/stage/check/agent.sock\n", "agentXSocket unix:/tmp/stage/check/agentx.sock\n", "[snmp] persistentDir /tmp/stage/check/persist\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("check copy lacks %q:\n%s", want, got)
		}
	}
	if a, b := strings.Count(string(files[r.paths.ConfFile].Content), "\n"), strings.Count(got, "\n"); b != a+1 {
		t.Errorf("check copy must keep line numbers (want %d lines, got %d)", a+1, b)
	}
}

// fakeSnmpd answers the parse run like the daemon: it writes the log file named by -Lf.
func fakeSnmpd(log string) func(renderers.Command) (renderers.Output, error) {
	return func(cmd renderers.Command) (renderers.Output, error) {
		for i, a := range cmd.Args {
			if a == "-Lf" && i+1 < len(cmd.Args) {
				_ = os.WriteFile(cmd.Args[i+1], []byte(log), 0o600)
			}
			// A pidfile naming a process that is not snmpd (this test): never signalled.
			if a == "-p" && i+1 < len(cmd.Args) {
				_ = os.WriteFile(cmd.Args[i+1], fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600)
			}
		}
		return renderers.Output{}, nil
	}
}

func TestValidateParseRun(t *testing.T) {
	rec := renderers.NewRecordingRunner().On(SnmpdBin, fakeSnmpd("NET-SNMP version 5.9.4.pre2\n"))
	r := New(rec, WithPaths(TestPaths("w0")), WithSecretResolver(resolver(nil)))
	files, err := r.Render(context.Background(), doc(t, goldenCases["full"]))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("want one snmpd run, got %v", calls)
	}
	argv := strings.Join(calls[0].Args, " ")
	if strings.Contains(argv, r.paths.ConfFile) || !strings.Contains(argv, "-C -c ") || strings.Contains(argv, " -f") {
		t.Fatalf("parse run must read only the staged check copy, daemonised: %s", argv)
	}
	for _, s := range plantedSecrets {
		if strings.Contains(argv, s) {
			t.Fatalf("secret in argv: %s", argv)
		}
	}
}

func TestValidateRejectsAndRedacts(t *testing.T) {
	// The daemon echoes an offending line that holds a secret: the error must mask it.
	log := "NET-SNMP version 5.9.4.pre2\n/x/check/snmpd.conf: line 20: Error: bad createUser u1 SHA-256 \"" + secAuth + "\" AES \"" + secPriv + "\"\n"
	rec := renderers.NewRecordingRunner().On(SnmpdBin, fakeSnmpd(log))
	r := New(rec, WithPaths(TestPaths("w0")), WithSecretResolver(resolver(nil)))
	files, err := r.Render(context.Background(), doc(t, goldenCases["v3"]))
	if err != nil {
		t.Fatal(err)
	}
	err = r.Validate(context.Background(), files)
	if !errors.Is(err, ErrDaemon) {
		t.Fatalf("want ErrDaemon, got %v", err)
	}
	for _, s := range plantedSecrets {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error leaks a secret: %v", err)
		}
	}
	if !strings.Contains(err.Error(), rfkit.Redacted) {
		t.Fatalf("error should show the redacted line: %v", err)
	}
}

func TestParseProblems(t *testing.T) {
	log := "NET-SNMP version 5.9.4.pre2\nCreated directory: /x/persist\nWarning: no access control information configured.\n  (Config search path: /etc/snmp)\n" +
		"/x/snmpd.conf: line 3: Warning: Unknown token: bogus.\n"
	p := parseProblems(log)
	if len(p) != 1 || !strings.Contains(p[0], "Unknown token") {
		t.Fatalf("got %q", p)
	}
}

// ---------------------------------------------------------------- Apply

type fakeCtl struct {
	mu       sync.Mutex
	reloads  int
	failNext bool
}

func (c *fakeCtl) Reload(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloads++
	if c.failNext {
		c.failNext = false
		return errors.New("reload failed")
	}
	return nil
}
func (c *fakeCtl) Restart(context.Context) error                { return nil }
func (c *fakeCtl) Signal(context.Context, syscall.Signal) error { return nil }

// fakeAgent answers like snmpd that has loaded the current file (or a stale one).
type fakeAgent struct {
	path  string
	stale bool
	err   error
	seen  []Target
}

func (a *fakeAgent) Get(_ context.Context, tg Target, _ []string) (map[string]string, error) {
	a.seen = append(a.seen, tg)
	if a.err != nil {
		return nil, a.err
	}
	b, _ := os.ReadFile(a.path)
	p := parseRendered(b)
	if a.stale {
		p.sysLoc = "old"
	}
	return map[string]string{OIDSysName: p.sysName, OIDSysLocation: p.sysLoc, OIDSysContact: p.sysContact, OIDSysUpTime: "42", OIDSysDescr: "Linux"}, nil
}

func tempPaths(t *testing.T) Paths {
	dir := t.TempDir()
	return Paths{ConfFile: filepath.Join(dir, "snmpd.conf"), AgentXSocket: filepath.Join(dir, "agentx.sock"), FileMode: 0o600}
}

func TestApplyConvergesAndRollsBack(t *testing.T) {
	p := tempPaths(t)
	ctl := &fakeCtl{}
	agent := &fakeAgent{path: p.ConfFile}
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(ctl), WithQuerier(agent),
		WithSecretResolver(resolver(nil)), WithVerifyTimeout(300_000_000))
	ctx := context.Background()
	d1 := with(base(), "communities", map[string]any{"ro": map[string]any{"secretRef": "password/snmp-ro", "sources": []any{"127.0.0.1/32"}}},
		"v3Users", map[string]any{"u1": map[string]any{"authProtocol": "sha256", "authRef": "password/u1-auth", "privRef": "password/u1-priv"}})
	f1, err := r.Render(ctx, doc(t, d1))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, f1); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(p.ConfFile); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if ctl.reloads != 1 || len(agent.seen) == 0 || agent.seen[0].User == nil || agent.seen[0].User.Name != "u1" || agent.seen[0].Port != 3861 {
		t.Fatalf("apply must reload once and verify with the v3 user on 127.0.0.1:3861: reloads=%d seen=%+v", ctl.reloads, agent.seen)
	}
	// A daemon that keeps the old sysLocation after the reload: not converged → rollback.
	f2, _ := r.Render(ctx, doc(t, with(d1, "sysLocation", "Hall C")))
	agent.stale = true
	err = r.Apply(ctx, f2)
	if !errors.Is(err, rfkit.ErrNotConverged) {
		t.Fatalf("want ErrNotConverged, got %v", err)
	}
	if got, _ := os.ReadFile(p.ConfFile); !bytes.Equal(got, f1[p.ConfFile].Content) {
		t.Fatal("previous file not restored")
	}
	if ctl.reloads != 3 {
		t.Fatalf("want reload + rollback reload (3 total), got %d", ctl.reloads)
	}
	// A failing reload also restores.
	agent.stale = false
	ctl.failNext = true
	if err := r.Apply(ctx, f2); err == nil {
		t.Fatal("reload failure not reported")
	}
	if got, _ := os.ReadFile(p.ConfFile); !bytes.Equal(got, f1[p.ConfFile].Content) {
		t.Fatal("previous file not restored after a failed reload")
	}
	// Idempotent.
	if err := r.Apply(ctx, f1); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, f1); err != nil {
		t.Fatal(err)
	}
}

func TestRetrieveRedactsAndNeverFails(t *testing.T) {
	p := tempPaths(t)
	agent := &fakeAgent{path: p.ConfFile}
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(&fakeCtl{}), WithQuerier(agent), WithSecretResolver(resolver(nil)))
	ctx := context.Background()
	files, err := r.Render(ctx, doc(t, goldenCases["v2c"]))
	if err != nil {
		t.Fatal(err)
	}
	if err := renderers.WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	// A fresh renderer (agent restart) still learns the secrets from the file itself.
	r2 := New(renderers.NewRecordingRunner(), WithPaths(p), WithQuerier(agent))
	agent.err = fmt.Errorf("request timeout (community %s)", secRO)
	msg, err := r2.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := msg.(*structpb.Struct)
	js := st.String()
	for _, s := range plantedSecrets {
		if strings.Contains(js, s) {
			t.Fatalf("Retrieve leaks %s: %s", s, js)
		}
	}
	if st.Fields["reachable"].GetBoolValue() || !strings.Contains(st.Fields["error"].GetStringValue(), rfkit.Redacted) {
		t.Fatalf("unexpected state %s", js)
	}
	agent.err = nil
	msg, _ = r2.Retrieve(ctx)
	st = msg.(*structpb.Struct)
	if !st.Fields["reachable"].GetBoolValue() || st.Fields["sysName"].GetStringValue() != "vrx-w8" || st.Fields["credential"].GetStringValue() != "v2c community" {
		t.Fatalf("unexpected state %s", st)
	}
	// Events: a changed location after the baseline.
	pl := r2.Poller()
	if ev := pl.Step(ctx); len(ev) != 0 {
		t.Fatalf("baseline produced events %v", ev)
	}
	agent.stale = true
	ev := pl.Step(ctx)
	if len(ev) != 1 || ev[0].Key != "sysLocation" || ev[0].New != "old" {
		t.Fatalf("events %v", ev)
	}
	agent.err = fmt.Errorf("timeout for %s", secRO)
	ev = pl.Step(ctx)
	if len(ev) != 1 || ev[0].Key != "reachable" || strings.Contains(ev[0].String(), secRO) {
		t.Fatalf("events %v", ev)
	}
}

func TestParseRenderedPicksLocalCredential(t *testing.T) {
	r := newRenderer(t)
	for name, want := range map[string]string{"v2c": "v2c community", "v3": "v3 user md5d", "traps": "v3 user u1", "full": "v3 user u1", "listen-any": "v2c community"} {
		files, err := r.Render(context.Background(), doc(t, goldenCases[name]))
		if err != nil {
			t.Fatal(err)
		}
		p := parseRendered(files[r.paths.ConfFile].Content)
		if !p.queryable() || p.target.Describe() != want || !p.target.Addr.IsLoopback() {
			t.Errorf("%s: target %+v, want %s on loopback", name, p.target, want)
		}
	}
	files, _ := r.Render(context.Background(), doc(t, goldenCases["disabled"]))
	if parseRendered(files[r.paths.ConfFile].Content).queryable() {
		t.Error("disabled config must not be queried")
	}
}

func TestProductAllowlist(t *testing.T) {
	for _, b := range Binaries() {
		if b == "/usr/bin/ip" || strings.HasSuffix(b, "/sh") || strings.HasSuffix(b, "/bash") {
			t.Fatalf("product allowlist contains a trampoline: %s", b)
		}
	}
}
