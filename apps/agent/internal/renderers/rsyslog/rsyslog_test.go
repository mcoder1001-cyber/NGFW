package rsyslog

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
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// Planted TLS material (VRX_TEST_PSK_ fixtures).
const (
	keyPEM  = "-----BEGIN PRIVATE KEY-----\nVRX_TEST_PSK_RF4_tlskey\n-----END PRIVATE KEY-----\n" // VRX_TEST_PSK_ fixture
	certPEM = "-----BEGIN CERTIFICATE-----\nVRX_TEST_PSK_RF4_cert\n-----END CERTIFICATE-----\n"
	keyMark = "VRX_TEST_PSK_RF4_tlskey"
)

func doc(t testing.TB, targets []any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(map[string]any{"management": map[string]any{"syslog": targets}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func resolver(extra map[string]string) rfkit.SecretResolver {
	vals := map[string]string{"cert/syslog-ca": certPEM, "cert/syslog-client": certPEM, "key/syslog-client": keyPEM}
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

func productLike() Paths {
	p := ProductPaths()
	p.FileOwner, p.KeyOwner = "", ""
	return p
}

func newRenderer(p Paths, opts ...Option) *Renderer {
	return New(renderers.NewRecordingRunner(), append([]Option{WithPaths(p), WithSecretResolver(resolver(nil))}, opts...)...)
}

func tgt(kv ...any) map[string]any {
	m := map[string]any{"address": "192.0.2.10"}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

var tlsTarget = tgt("address", "logs.example.net", "port", 6514, "protocol", "tls", "severity", "notice",
	"tls", map[string]any{"caRef": "cert/syslog-ca", "certRef": "cert/syslog-client", "keyRef": "key/syslog-client", "permittedPeers": []any{"logs.example.net"}})

var goldenCases = map[string]struct {
	paths   Paths
	targets []any
}{
	"product-empty": {productLike(), []any{}},
	"product-udp":   {productLike(), []any{tgt()}},
	"product-mixed": {productLike(), []any{
		tgt("port", 5514, "severity", "warning", "facilities", []any{"local7", "kern", "local7"}),
		tgt("address", "2001:db8::5", "protocol", "tcp", "severity", "error", "format", "rfc3164", "queueSize", 50000),
		tgt("address", "Collector.Example.NET", "protocol", "tcp", "severity", "debug"),
	}},
	"product-tls":    {productLike(), []any{tlsTarget}},
	"standalone":     {TestPaths("w0", 3014), []any{tgt("address", "127.0.0.1", "port", 3015)}},
	"standalone-0tg": {TestPaths("w0", 0), []any{}},
}

func TestGolden(t *testing.T) {
	for name, c := range goldenCases {
		t.Run(name, func(t *testing.T) {
			r := newRenderer(c.paths)
			files, err := r.Render(context.Background(), doc(t, c.targets))
			if err != nil {
				t.Fatal(err)
			}
			conf := files[c.paths.ConfFile]
			if conf.Mode != 0o644 || conf.Secret {
				t.Fatalf("config must be 0644 and carry no secret: %v %v", conf.Mode, conf.Secret)
			}
			golden(t, name, conf.Content)
			assertSafe(t, conf.Content)
			if bytes.Contains(conf.Content, []byte(keyMark)) {
				t.Fatal("key material in the config file")
			}
		})
	}
}

// Never: other action types, includes, legacy $ directives, backticks (RainerScript
// constant-from-command), or a second template statement.
var forbidden = regexp.MustCompile("omprog|omshell|omusrmsg|omfile|ommail|\\$IncludeConfig|include\\(|(?m)^\\s*\\$|`|exec\\(")

func assertSafe(t *testing.T, c []byte) {
	t.Helper()
	if m := forbidden.Find(c); m != nil {
		t.Fatalf("forbidden construct %q", m)
	}
	for _, m := range regexp.MustCompile(`action\(type="([a-z]+)"`).FindAllSubmatch(c, -1) {
		if string(m[1]) != "omfwd" {
			t.Fatalf("action type %s", m[1])
		}
	}
	if n := bytes.Count(c, []byte("template(")); n > 1 {
		t.Fatalf("%d template statements", n)
	}
	if bytes.Count(c, []byte("{")) != bytes.Count(c, []byte("}")) || bytes.Count(c, []byte("(")) != bytes.Count(c, []byte(")")) {
		t.Fatal("unbalanced braces/parentheses")
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

func TestTLSFiles(t *testing.T) {
	p := ProductPaths()
	r := newRenderer(p)
	files, err := r.Render(context.Background(), doc(t, []any{tlsTarget}))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("want config + CA + cert + key, got %v", files.Paths())
	}
	for path, f := range files {
		switch {
		case strings.HasSuffix(path, "-key.pem"):
			if !f.Secret || f.Mode != 0o640 || f.Owner != "root:syslog" || string(f.Content) != keyPEM {
				t.Fatalf("key file %s: secret=%v mode=%v owner=%q", path, f.Secret, f.Mode, f.Owner)
			}
		default:
			if f.Secret || bytes.Contains(f.Content, []byte(keyMark)) {
				t.Fatalf("%s holds key material", path)
			}
		}
	}
	for _, f := range files.Redacted() {
		if bytes.Contains(f.Content, []byte(keyMark)) {
			t.Fatal("Files.Redacted leaks the key")
		}
	}
}

func TestTypedInput(t *testing.T) {
	a, sev := "192.0.2.1", "critical"
	ds := &vrxv1.DesiredState{Management: &vrxv1.ManagementConfig{Syslog: []*vrxv1.SyslogTarget{{Address: &a, Severity: &sev}}}}
	r := newRenderer(productLike())
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	c := string(files[ProductPaths().ConfFile].Content)
	for _, w := range []string{`target="192.0.2.1" port="514" protocol="udp"`, `if prifilt("*.crit") then {`} {
		if !strings.Contains(c, w) {
			t.Errorf("missing %q:\n%s", w, c)
		}
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{`a"b`: `"a\"b"`, `a\b`: `"a\\b"`, `"; rm -rf /`: `"\"; rm -rf /"`, "é": `"é"`} {
		got, err := Quote(in)
		if err != nil || got != want {
			t.Errorf("Quote(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"a\nb", "a\x00", "a\u2028", "\xff"} {
		if _, err := Quote(bad); err == nil {
			t.Errorf("Quote(%q) accepted", bad)
		}
	}
}

var hostile = []string{
	`"; rm -rf /`, "a\nb", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "é", strings.Repeat("A", 5000),
	`x" action(type="omprog" binary="/bin/sh")`, `a\"b`, "$IncludeConfig /tmp/x", "`echo x`", "a b", "*",
}

func TestHostileStrings(t *testing.T) {
	fields := map[string]func(v string) []any{
		"address":  func(v string) []any { return []any{tgt("address", v)} },
		"protocol": func(v string) []any { return []any{tgt("protocol", v)} },
		"severity": func(v string) []any { return []any{tgt("severity", v)} },
		"facility": func(v string) []any { return []any{tgt("facilities", []any{v})} },
		"format":   func(v string) []any { return []any{tgt("format", v)} },
		"vrf":      func(v string) []any { return []any{tgt("vrf", v)} },
		"tls peer": func(v string) []any {
			return []any{tgt("protocol", "tls", "tls", map[string]any{"caRef": "cert/syslog-ca", "permittedPeers": []any{v}})}
		},
		"tls auth": func(v string) []any {
			return []any{tgt("protocol", "tls", "tls", map[string]any{"caRef": "cert/syslog-ca", "authMode": v})}
		},
		"tls ref":   func(v string) []any { return []any{tgt("protocol", "tls", "tls", map[string]any{"caRef": v})} },
		"extra key": func(v string) []any { return []any{tgt("template", v)} },
	}
	for name, mk := range fields {
		for _, h := range hostile {
			t.Run(name+"/"+fmt.Sprintf("%.12q", h), func(t *testing.T) {
				if !utf8.ValidString(h) {
					t.Skip("not representable in structpb")
				}
				files, err := newRenderer(productLike()).Render(context.Background(), doc(t, mk(h)))
				if err == nil {
					t.Fatalf("hostile %q accepted:\n%s", h, files[ProductPaths().ConfFile].Content)
				}
				if !errors.Is(err, ErrInput) {
					t.Fatalf("error does not wrap ErrInput: %v", err)
				}
			})
		}
	}
}

func TestInvalidUTF8Typed(t *testing.T) {
	a := "logs\xff.example"
	ds := &vrxv1.DesiredState{Management: &vrxv1.ManagementConfig{Syslog: []*vrxv1.SyslogTarget{{Address: &a}}}}
	if _, err := newRenderer(productLike()).Render(context.Background(), ds); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}

func TestHostileSecretValues(t *testing.T) {
	for _, h := range []string{"not a pem", "-----BEGIN PRIVATE KEY-----\nabc\x00\n-----END PRIVATE KEY-----\n",
		"-----BEGIN X-----\n" + strings.Repeat("A\n", 40000) + "-----END X-----\n", rfkit.Redacted} {
		r := New(renderers.NewRecordingRunner(), WithPaths(productLike()), WithSecretResolver(resolver(map[string]string{"key/syslog-client": h})))
		_, err := r.Render(context.Background(), doc(t, []any{tlsTarget}))
		if err == nil {
			t.Fatalf("hostile key %.20q accepted", h)
		}
		if strings.Contains(err.Error(), "abc") || strings.Contains(err.Error(), "not a pem") {
			t.Fatalf("error echoes the value: %v", err)
		}
	}
}

func TestSemanticRules(t *testing.T) {
	for name, c := range map[string]struct {
		targets []any
		opts    []Option
		want    error
	}{
		"tls needs ca":         {[]any{tgt("protocol", "tls", "tls", map[string]any{"keyRef": "key/syslog-client"})}, nil, ErrInput},
		"tls key wrong kind":   {[]any{tgt("protocol", "tls", "tls", map[string]any{"caRef": "cert/syslog-ca", "certRef": "cert/syslog-client", "keyRef": "password/x"})}, nil, rfkit.ErrSecretRef},
		"tls cert without key": {[]any{tgt("protocol", "tls", "tls", map[string]any{"caRef": "cert/syslog-ca", "certRef": "cert/syslog-client"})}, nil, ErrInput},
		"tls anon refused":     {[]any{tgt("protocol", "tls", "tls", map[string]any{"caRef": "cert/syslog-ca", "authMode": "anon"})}, nil, ErrInput},
		"tls on udp":           {[]any{tgt("tls", map[string]any{"caRef": "cert/syslog-ca"})}, nil, ErrInput},
		"tls no resolver":      {[]any{tlsTarget}, []Option{WithSecretResolver(nil)}, rfkit.ErrNoSecretResolver},
		"duplicate":            {[]any{tgt(), tgt("severity", "debug")}, nil, ErrInput},
		"too many":             {make17(), nil, ErrInput},
		"port 0":               {[]any{tgt("port", 0)}, nil, ErrInput},
		"multicast":            {[]any{tgt("address", "239.1.1.1")}, nil, ErrInput},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newRenderer(productLike(), c.opts...).Render(context.Background(), doc(t, c.targets))
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func make17() []any {
	out := make([]any, 17)
	for i := range out {
		out[i] = tgt("port", 1000+i)
	}
	return out
}

func TestValidate(t *testing.T) {
	rec := renderers.NewRecordingRunner().Succeed(RsyslogdBin, "")
	r := New(rec, WithPaths(productLike()), WithSecretResolver(resolver(nil)))
	ctx := context.Background()
	files, _ := r.Render(ctx, doc(t, goldenCases["product-mixed"].targets))
	if err := r.Validate(ctx, files); err != nil {
		t.Fatal(err)
	}
	c := rec.Calls()
	if len(c) != 1 || c[0].Args[0] != "-N1" || c[0].Args[1] != "-f" || c[0].Args[2] == ProductPaths().ConfFile || !strings.HasSuffix(c[0].Args[2], ProductPaths().ConfFile) {
		t.Fatalf("argv %v", c)
	}
	// Empty export: no daemon run (rsyslogd rejects configs without any action).
	rec.Reset()
	empty, _ := r.Render(ctx, doc(t, []any{}))
	if err := r.Validate(ctx, empty); err != nil || len(rec.Calls()) != 0 {
		t.Fatalf("empty export: %v %v", err, rec.Calls())
	}
	// TLS without the ossl driver: refused before any daemon run.
	p := productLike()
	p.ModuleDir = t.TempDir()
	r2 := New(rec, WithPaths(p), WithSecretResolver(resolver(nil)))
	tf, _ := r2.Render(ctx, doc(t, []any{tlsTarget}))
	if err := r2.Validate(ctx, tf); !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "lmnsd_ossl.so") {
		t.Fatalf("got %v", err)
	}
	if err := os.WriteFile(filepath.Join(p.ModuleDir, "lmnsd_ossl.so"), nil, 0o644); err != nil { //nolint:gosec // empty stand-in
		t.Fatal(err)
	}
	if err := r2.Validate(ctx, tf); err != nil {
		t.Fatalf("with the driver present: %v", err)
	}
	// A failing check that echoes key material: redacted.
	rec3 := renderers.NewRecordingRunner().FailWith(RsyslogdBin, 1, "rsyslogd: error in "+keyPEM)
	r3 := New(rec3, WithPaths(p), WithSecretResolver(resolver(nil)))
	tf3, _ := r3.Render(ctx, doc(t, []any{tlsTarget}))
	err := r3.Validate(ctx, tf3)
	if !errors.Is(err, ErrDaemon) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), keyMark) {
		t.Fatalf("error leaks the key: %v", err)
	}
}

// fakeRsyslog restarts by appending an impstats batch for the actions of the live (or a stale)
// config, stamped one second after the restart.
type fakeRsyslog struct {
	p        Paths
	stale    []byte
	failNext bool
}

func (f *fakeRsyslog) Reload(context.Context) error                 { return errors.New("rsyslog cannot reload") }
func (f *fakeRsyslog) Signal(context.Context, syscall.Signal) error { return nil }
func (f *fakeRsyslog) Restart(context.Context) error {
	if f.failNext {
		f.failNext = false
		return errors.New("restart failed")
	}
	conf, _ := os.ReadFile(f.p.ConfFile)
	if f.stale != nil {
		conf = f.stale
	}
	ts := time.Now().Add(1100 * time.Millisecond).Format(statsTimeLayout)
	var b strings.Builder
	for _, n := range actionNames(conf) {
		fmt.Fprintf(&b, "%s: { \"name\": %q, \"origin\": \"core.action\", \"processed\": 7, \"failed\": 1, \"suspended\": 0 }\n", ts, n)
		fmt.Fprintf(&b, "%s: { \"name\": \"%s queue\", \"origin\": \"core.queue\", \"size\": 2, \"enqueued\": 9, \"discarded.full\": 0, \"discarded.nf\": 0 }\n", ts, n)
	}
	fmt.Fprintf(&b, "%s: { \"name\": \"imuxsock\", \"origin\": \"imuxsock\", \"submitted\": 9 }\n", ts)
	fh, err := os.OpenFile(f.p.StatsFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	_, err = fh.WriteString(b.String())
	return err
}

func TestApplyConvergesRollsBackAndRetrieves(t *testing.T) {
	dir := t.TempDir()
	p := productLike()
	p.ConfFile, p.StatsFile, p.TLSDir = filepath.Join(dir, "50-vrx-export.conf"), filepath.Join(dir, "impstats.json"), filepath.Join(dir, "tls")
	p.ModuleDir = dir
	fake := &fakeRsyslog{p: p}
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(fake), WithSecretResolver(resolver(nil)), WithVerifyTimeout(3*time.Second))
	ctx := context.Background()
	f1, err := r.Render(ctx, doc(t, []any{tgt(), tlsTarget}))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, f1); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(p.TLSDir, "export-1-key.pem")
	if info, err := os.Stat(key); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("key file: %v %v", info, err)
	}
	st, err := r.State(ctx)
	if err != nil || len(st.Targets) != 2 || !st.Targets[0].Reported || st.Targets[0].Processed != 7 || st.Targets[1].QueueSize != 2 || st.Inputs["imuxsock"] != 9 {
		t.Fatalf("state %+v %v", st, err)
	}
	msg, _ := r.Retrieve(ctx)
	if strings.Contains(msg.(*structpb.Struct).String(), keyMark) {
		t.Fatal("Retrieve leaks key material")
	}
	// A restart that leaves the old configuration running (stale action names): rollback.
	f2, _ := r.Render(ctx, doc(t, []any{tgt("severity", "debug")}))
	fake.stale = f1[p.ConfFile].Content
	err = r.Apply(ctx, f2)
	if !errors.Is(err, rfkit.ErrNotConverged) {
		t.Fatalf("want ErrNotConverged, got %v", err)
	}
	if cur, _ := os.ReadFile(p.ConfFile); !bytes.Equal(cur, f1[p.ConfFile].Content) {
		t.Fatal("previous config not restored")
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatal("TLS key removed by a failed apply")
	}
	// Converging apply without TLS: stale TLS files pruned.
	fake.stale = nil
	if err := r.Apply(ctx, f2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("TLS key of a removed target not pruned")
	}
	// Restart failure → restored.
	fake.failNext = true
	if err := r.Apply(ctx, f1); err == nil {
		t.Fatal("restart failure not reported")
	}
	if cur, _ := os.ReadFile(p.ConfFile); !bytes.Equal(cur, f2[p.ConfFile].Content) {
		t.Fatal("not restored after a failed restart")
	}
}

func TestParseStatsSince(t *testing.T) {
	now := time.Date(2026, 9, 24, 2, 0, 0, 0, time.Local)
	lines := now.Format(statsTimeLayout) + `: { "name": "old", "origin": "core.action", "processed": 1 }` + "\n" +
		now.Add(time.Second).Format(statsTimeLayout) + `: { "name": "new", "origin": "core.action", "processed": 2 }` + "\n" +
		"garbage line\n" + now.Add(2*time.Second).Format(statsTimeLayout) + `: { "name": "partial", "or`
	got := ParseStatsSince([]byte(lines), now.Add(300*time.Millisecond))
	if _, ok := got["old"]; ok || got["new"].Values["processed"] != 2 || len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if all := ParseStats([]byte(lines)); len(all) != 2 {
		t.Fatalf("%+v", all)
	}
}

func TestStatsTailBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "impstats.json")
	var b bytes.Buffer
	for b.Len() < 3*maxStatsTail {
		fmt.Fprintf(&b, "Thu Sep 24 02:00:00 2026: { \"name\": \"n%d\", \"origin\": \"core.action\", \"processed\": 1 }\n", b.Len())
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readStatsFrom(path, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got); n == 0 || n > maxStatsTail/60 {
		t.Fatalf("read %d records: the tail is not bounded", n)
	}
}

func TestProductDefaults(t *testing.T) {
	if err := ProductPaths().Validate(); err != nil {
		t.Fatal(err)
	}
	if ProductPaths().Standalone != nil {
		t.Fatal("product config must not declare inputs or global() (the host's rsyslog owns them)")
	}
	for _, b := range Binaries() {
		if b == "/usr/bin/ip" {
			t.Fatal("trampoline in the product allowlist")
		}
	}
}
