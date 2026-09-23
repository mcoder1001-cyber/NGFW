package unbound

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func unitPaths() Paths { return TestPaths("w0", 6) }

func newUnit() *Renderer { return New(renderers.NewRecordingRunner(), WithPaths(unitPaths())) }

func listen(addr string, port uint32) *vrxv1.SocketAddress {
	return &vrxv1.SocketAddress{Address: proto.String(addr), Port: proto.Uint32(port)}
}

func rec(name, typ, data string) *vrxv1.DnsRecord {
	return &vrxv1.DnsRecord{Name: proto.String(name), Type: proto.String(typ), Data: proto.String(data)}
}

func fullResolver() *vrxv1.DnsResolver {
	return &vrxv1.DnsResolver{
		Enabled:     proto.Bool(true),
		Description: proto.String("LAN resolver"),
		Vrf:         proto.String("default"),
		Listen:      []*vrxv1.SocketAddress{listen("127.0.0.1", 3653), listen("::1", 3653)},
		AccessControl: []*vrxv1.DnsAccessControl{
			{Prefix: proto.String("127.0.0.0/8"), Action: proto.String("allow")},
			{Prefix: proto.String("10.6.0.1/16")},
			{Prefix: proto.String("::1/128"), Action: proto.String("allow_snoop")},
		},
		Forwarders: []*vrxv1.DnsUpstream{
			{Address: proto.String("192.0.2.53")},
			{Address: proto.String("192.0.2.54"), Port: proto.Uint32(5353)},
		},
		ForwardZones: []*vrxv1.DnsForwardZone{
			{Zone: proto.String("corp.example.test"), ForwardFirst: proto.Bool(true), Forwarders: []*vrxv1.DnsUpstream{
				{Address: proto.String("2001:db8::53"), Tls: proto.Bool(true), TlsServerName: proto.String("dns.Example.test")},
				{Address: proto.String("192.0.2.1"), Tls: proto.Bool(true)},
			}},
		},
		LocalZones: []*vrxv1.DnsLocalZone{
			{Zone: proto.String("lan.example.test"), Records: []*vrxv1.DnsRecord{
				rec("gw.lan.example.test", "A", "10.6.0.1"),
				rec("gw.lan.example.test", "AAAA", "FD00:6::1"),
				rec("www.lan.example.test.", "CNAME", "gw.lan.example.test"),
				rec("lan.example.test", "MX", "10 mail.lan.example.test"),
				rec("lan.example.test", "NS", "gw.lan.example.test"),
				rec("_sip._tcp.lan.example.test", "SRV", "10 5 5060 sip.lan.example.test"),
				{Name: proto.String("txt.lan.example.test"), Type: proto.String("TXT"), TtlSec: proto.Uint32(60), Data: proto.String(`v=spf1 "quoted" \ back ; semi 'apos' include: /etc/passwd`)},
			}},
			{Zone: proto.String("6.10.in-addr.arpa"), Type: proto.String("transparent"), Records: []*vrxv1.DnsRecord{
				rec("1.0.6.10.in-addr.arpa", "PTR", "gw.lan.example.test"),
			}},
			{Zone: proto.String("blocked.test"), Type: proto.String("refuse")},
		},
		Cache:             &vrxv1.DnsResolver_Cache{MinTtlSec: proto.Uint32(30), MaxTtlSec: proto.Uint32(3600), Prefetch: proto.Bool(true), MsgCacheMb: proto.Uint32(16), RrsetCacheMb: proto.Uint32(32)},
		Threads:           proto.Uint32(2),
		QnameMinimisation: proto.Bool(true),
		HideIdentity:      proto.Bool(true),
		HideVersion:       proto.Bool(false),
		LogQueries:        proto.Bool(true),
	}
}

func dns(res map[string]*vrxv1.DnsResolver) *vrxv1.DnsService {
	return &vrxv1.DnsService{Resolvers: res}
}

func render(t *testing.T, r *Renderer, desired proto.Message) []byte {
	t.Helper()
	files, err := r.Render(context.Background(), desired)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want one file, got %v", files.Paths())
	}
	return files[r.paths.Conf()].Content
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	gp := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(gp, got, 0o644); err != nil { //nolint:gosec // test data
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(gp) //nolint:gosec // test data
	if err != nil {
		t.Fatalf("read %s (run go test -update): %v", gp, err)
	}
	if string(want) != string(got) {
		t.Errorf("render differs from %s:\n%s", gp, got)
	}
}

func TestRenderGolden(t *testing.T) {
	second := &vrxv1.DnsResolver{
		Listen:        []*vrxv1.SocketAddress{listen("127.0.0.2", 3653)},
		AccessControl: []*vrxv1.DnsAccessControl{{Prefix: proto.String("127.0.0.0/8"), Action: proto.String("allow")}},
		LocalZones: []*vrxv1.DnsLocalZone{{Zone: proto.String("guest.example.test"), Records: []*vrxv1.DnsRecord{
			rec("portal.guest.example.test", "A", "10.6.20.1"),
		}}},
	}
	first := &vrxv1.DnsResolver{
		Listen: []*vrxv1.SocketAddress{listen("127.0.0.1", 3653)},
		LocalZones: []*vrxv1.DnsLocalZone{{Zone: proto.String("lan.example.test"), Records: []*vrxv1.DnsRecord{
			rec("portal.lan.example.test", "A", "10.6.10.1"),
		}}},
		ForwardZones: []*vrxv1.DnsForwardZone{{Zone: proto.String("corp.test"), Forwarders: []*vrxv1.DnsUpstream{{Address: proto.String("192.0.2.1")}}}},
	}
	dnssecOff := &vrxv1.DnsResolver{Listen: []*vrxv1.SocketAddress{listen("127.0.0.1", 3653)}, Dnssec: &vrxv1.DnsResolver_Dnssec{Enabled: proto.Bool(false)}}
	dnssecStatic := &vrxv1.DnsResolver{Listen: []*vrxv1.SocketAddress{listen("127.0.0.1", 3653)}, Dnssec: &vrxv1.DnsResolver_Dnssec{TrustAnchorAuto: proto.Bool(false)}}
	hostile := fullResolver()
	hostile.Description = proto.String(`"; rm -rf / \ ☃`)
	cases := map[string]proto.Message{
		"empty":               nil,
		"full":                dns(map[string]*vrxv1.DnsResolver{"lan": fullResolver()}),
		"views":               &vrxv1.ServicesConfig{Dns: dns(map[string]*vrxv1.DnsResolver{"lan": first, "guest": second})},
		"dnssec-off":          &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Dns: dns(map[string]*vrxv1.DnsResolver{"r": dnssecOff})}},
		"dnssec-static":       dns(map[string]*vrxv1.DnsResolver{"r": dnssecStatic}),
		"disabled":            dns(map[string]*vrxv1.DnsResolver{"off": {Enabled: proto.Bool(false), Listen: []*vrxv1.SocketAddress{listen("172.30.126.195", 53)}}}),
		"hostile-description": dns(map[string]*vrxv1.DnsResolver{"lan": hostile}),
	}
	for name, desired := range cases {
		t.Run(name, func(t *testing.T) {
			got := render(t, newUnit(), desired)
			golden(t, name, got)
			if again := render(t, newUnit(), desired); string(again) != string(got) {
				t.Error("render is not deterministic")
			}
			assertNoInjection(t, got)
		})
	}
}

// assertNoInjection: every line is a known directive, a comment or a block header, and the
// token `include` never appears outside a comment.
func assertNoInjection(t *testing.T, conf []byte) {
	t.Helper()
	for i, l := range strings.Split(string(conf), "\n") {
		trim := strings.TrimSpace(l)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.Contains(strings.ToLower(trim), "include") {
			t.Errorf("line %d contains include: %q", i+1, l)
		}
		key, _, ok := strings.Cut(trim, ":")
		if !ok || strings.ContainsAny(key, ` "'`) {
			t.Errorf("line %d is not a directive: %q", i+1, l)
		}
	}
}

func TestHostileEscaped(t *testing.T) {
	conf := string(render(t, newUnit(), dns(map[string]*vrxv1.DnsResolver{"lan": fullResolver()})))
	want := `local-data: 'txt.lan.example.test. 60 IN TXT "v=spf1 \034quoted\034 \092 back \059 semi \039apos\039 \105nclude\058 /etc/passwd"'`
	if !strings.Contains(conf, want) {
		t.Fatalf("TXT not escaped as expected; want line %s in\n%s", want, conf)
	}
	h := fullResolver()
	h.Description = proto.String(`"; rm -rf /`)
	conf = string(render(t, newUnit(), dns(map[string]*vrxv1.DnsResolver{"lan": h})))
	if !strings.Contains(conf, `# resolver "lan": "\"; rm -rf /"`) {
		t.Fatalf("description not quoted in its comment:\n%s", conf)
	}
}

var hostile = []string{
	"x\ninclude: /etc/passwd", "\ninclude: /etc/passwd", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "\xff\xfe",
}

func TestRejects(t *testing.T) {
	type mut func(r *vrxv1.DnsResolver)
	cases := map[string]mut{
		"desc-5k":      func(r *vrxv1.DnsResolver) { r.Description = proto.String(strings.Repeat("x", 5*1024)) },
		"zone-include": func(r *vrxv1.DnsResolver) { r.LocalZones[0].Zone = proto.String("include: /etc/passwd") },
		"zone-quote":   func(r *vrxv1.DnsResolver) { r.LocalZones[0].Zone = proto.String(`x"; rm -rf /`) },
		"zone-unicode": func(r *vrxv1.DnsResolver) { r.LocalZones[0].Zone = proto.String("bücher.test") },
		"zone-type":    func(r *vrxv1.DnsResolver) { r.LocalZones[0].Type = proto.String("static\ninclude: /x") },
		"record-name":  func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].Name = proto.String("a b") },
		"record-type":  func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].Type = proto.String("HINFO") },
		"record-a":     func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].Data = proto.String("10.0.0.1'; x") },
		"record-a-v6":  func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].Data = proto.String("::1") },
		"record-cname": func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[2].Data = proto.String(`x' include: '/etc/passwd`) },
		"record-mx":    func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[3].Data = proto.String("99999 mail") },
		"record-srv":   func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[5].Data = proto.String("1 2 3") },
		"record-ttl":   func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].TtlSec = proto.Uint32(1 << 30) },
		"txt-5k": func(r *vrxv1.DnsResolver) {
			r.LocalZones[0].Records[6].Data = proto.String(strings.Repeat("a", 5*1024))
		},
		"txt-unicode":       func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[6].Data = proto.String("☃") },
		"listen-host-nic":   func(r *vrxv1.DnsResolver) { r.Listen = []*vrxv1.SocketAddress{listen("172.30.126.195", 53)} },
		"listen-hostile":    func(r *vrxv1.DnsResolver) { r.Listen = []*vrxv1.SocketAddress{listen("127.0.0.1\ninclude: /x", 53)} },
		"listen-none":       func(r *vrxv1.DnsResolver) { r.Listen = nil },
		"listen-port":       func(r *vrxv1.DnsResolver) { r.Listen = []*vrxv1.SocketAddress{listen("127.0.0.1", 70000)} },
		"acl-prefix":        func(r *vrxv1.DnsResolver) { r.AccessControl[0].Prefix = proto.String("10.0.0.0/8 allow\ninclude: x") },
		"acl-action":        func(r *vrxv1.DnsResolver) { r.AccessControl[0].Action = proto.String("allow; rm") },
		"fwd-addr":          func(r *vrxv1.DnsResolver) { r.Forwarders[0].Address = proto.String("dns.google") },
		"fwd-tlsname":       func(r *vrxv1.DnsResolver) { r.ForwardZones[0].Forwarders[0].TlsServerName = proto.String("x#y") },
		"fwd-tlsname-plain": func(r *vrxv1.DnsResolver) { r.Forwarders[0].TlsServerName = proto.String("x.test") },
		"fwd-tls-mixed": func(r *vrxv1.DnsResolver) {
			r.ForwardZones[0].Forwarders[1].Tls = proto.Bool(false)
		},
		"fwd-zone-dup": func(r *vrxv1.DnsResolver) {
			r.ForwardZones = append(r.ForwardZones, &vrxv1.DnsForwardZone{Zone: proto.String("."), Forwarders: r.Forwarders})
		},
		"local-zone-dup": func(r *vrxv1.DnsResolver) { r.LocalZones = append(r.LocalZones, r.LocalZones[0]) },
		"cache-ttl":      func(r *vrxv1.DnsResolver) { r.Cache.MinTtlSec = proto.Uint32(99999) },
		"threads":        func(r *vrxv1.DnsResolver) { r.Threads = proto.Uint32(0) },
		"vrf":            func(r *vrxv1.DnsResolver) { r.Vrf = proto.String("a b") },
	}
	for i, h := range hostile {
		h := h
		cases["desc-hostile-"+string(rune('a'+i))] = func(r *vrxv1.DnsResolver) { r.Description = proto.String(h) }
		cases["txt-hostile-"+string(rune('a'+i))] = func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[6].Data = proto.String(h) }
		cases["name-hostile-"+string(rune('a'+i))] = func(r *vrxv1.DnsResolver) { r.LocalZones[0].Records[0].Name = proto.String(h) }
		cases["fwdzone-hostile-"+string(rune('a'+i))] = func(r *vrxv1.DnsResolver) { r.ForwardZones[0].Zone = proto.String(h) }
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			r := fullResolver()
			m(r)
			_, err := newUnit().Render(context.Background(), dns(map[string]*vrxv1.DnsResolver{"lan": r}))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
	t.Run("merge-conflicts", func(t *testing.T) {
		a, b := fullResolver(), fullResolver()
		b.Listen = []*vrxv1.SocketAddress{listen("127.0.0.2", 3653)}
		b.Threads = proto.Uint32(4)
		_, err := newUnit().Render(context.Background(), dns(map[string]*vrxv1.DnsResolver{"a": a, "b": b}))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("different settings: want ErrInvalid, got %v", err)
		}
		b = fullResolver()
		_, err = newUnit().Render(context.Background(), dns(map[string]*vrxv1.DnsResolver{"a": a, "b": b}))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("same listen address twice: want ErrInvalid, got %v", err)
		}
	})
}

func TestTemplateHelpers(t *testing.T) {
	for _, bad := range []string{"x'y", `a\b`, "a include: b", "a\nb"} {
		if _, err := rrQuote(bad); err == nil {
			t.Errorf("rrQuote(%q) accepted", bad)
		}
	}
	if got, err := rrQuote(`t. 1 IN TXT "a\034b"`); err != nil || got != `'t. 1 IN TXT "a\034b"'` {
		t.Errorf("rrQuote: %q %v", got, err)
	}
	for _, bad := range []string{"1.2.3.4", "x@53", "1.2.3.4@0", "1.2.3.4@53\n"} {
		if _, err := sockaddr(bad); err == nil {
			t.Errorf("sockaddr(%q) accepted", bad)
		}
	}
	for _, bad := range []string{"1.2.3.4@53#X Y", "1.2.3.4@53#a\"b", "a@53"} {
		if _, err := fwdaddr(bad); err == nil {
			t.Errorf("fwdaddr(%q) accepted", bad)
		}
	}
}

func TestValidateArgv(t *testing.T) {
	rr := renderers.NewRecordingRunner().Succeed(CheckconfBin, "no errors")
	r := New(rr, WithPaths(unitPaths()))
	files, err := r.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	c := rr.Calls()
	if len(c) != 1 || c[0].Path != CheckconfBin || len(c[0].Args) != 1 || !strings.HasSuffix(c[0].Args[0], "/unbound/unbound.conf") ||
		strings.HasPrefix(c[0].Args[0], unitPaths().ConfDir) {
		t.Fatalf("argv %v", c)
	}
	rr = renderers.NewRecordingRunner().FailWith(CheckconfBin, 1, "error: bad")
	r = New(rr, WithPaths(unitPaths()))
	if err := r.Validate(context.Background(), files); !errors.Is(err, ErrDaemon) {
		t.Fatalf("want ErrDaemon, got %v", err)
	}
}

func tmpPaths(t *testing.T) Paths {
	p := unitPaths()
	d := t.TempDir()
	p.ConfDir = d
	p.ControlSocketPath = filepath.Join(d, "unbound.ctl")
	p.PidFilePath = filepath.Join(d, "unbound.pid")
	p.PendingFile = filepath.Join(d, "state", "vrx.pending")
	return p
}

// listenSocket stands in for unbound's control socket (Control probes it before unbound-control).
func listenSocket(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
}

// fakeUnbound answers unbound-control like a daemon (pid, forwards and local zones from the
// rendered file) and listens on a real loopback TCP port for the convergence check.
type fakeUnbound struct {
	pid      int
	conf     func() []byte
	reloads  int
	failNext bool
	forwards string // override for list_forwards ("" = derive from the file)
}

func (f *fakeUnbound) run(cmd renderers.Command) (renderers.Output, error) {
	verb := cmd.Args[len(cmd.Args)-1]
	switch verb {
	case "status":
		return renderers.Output{Stdout: []byte(fmt.Sprintf("version: 1.24.2\nunbound (pid %d) is running...\n", f.pid))}, nil
	case "reload_keep_cache":
		f.reloads++
		if f.failNext {
			f.failNext = false
			out := renderers.Output{Stderr: []byte("error: reload failed"), ExitCode: 1}
			return out, &renderers.ExitError{Command: cmd, Output: out}
		}
		return renderers.Output{Stdout: []byte("ok\n")}, nil
	case "list_forwards":
		if f.forwards != "" {
			return renderers.Output{Stdout: []byte(f.forwards)}, nil
		}
		var b strings.Builder
		sec := ""
		for _, l := range strings.Split(string(f.conf()), "\n") {
			if !strings.HasPrefix(l, "\t") && strings.HasSuffix(l, ":") {
				sec = l
			}
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "name: "); ok && sec == "forward-zone:" {
				fmt.Fprintf(&b, "%s IN forward 192.0.2.1\n", strings.Trim(v, `"`))
			}
		}
		return renderers.Output{Stdout: []byte(b.String())}, nil
	case "list_local_zones":
		var b strings.Builder
		for _, l := range strings.Split(string(f.conf()), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "local-zone: "); ok {
				z, typ, _ := strings.Cut(v, " ")
				fmt.Fprintf(&b, "%s %s\n", strings.Trim(z, `"`), typ)
			}
		}
		return renderers.Output{Stdout: []byte(b.String())}, nil
	}
	return renderers.Output{}, nil
}

func tcpPort(t *testing.T) uint32 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return uint32(l.Addr().(*net.TCPAddr).Port) //nolint:gosec // port
}

func resolverOn(ports ...uint32) *vrxv1.DnsService {
	r := fullResolver()
	r.Listen = nil
	for _, p := range ports {
		r.Listen = append(r.Listen, listen("127.0.0.1", p))
	}
	return dns(map[string]*vrxv1.DnsResolver{"lan": r})
}

// startChild starts a short-lived process: its start time is "after" any pending request made
// before, so it stands in for a restarted unbound.
func startChild(t *testing.T) int {
	t.Helper()
	time.Sleep(30 * time.Millisecond) // process start times have 10 ms resolution (USER_HZ)
	cmd := exec.Command("/usr/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

func TestApply(t *testing.T) {
	ctx := context.Background()
	full := dns(map[string]*vrxv1.DnsResolver{"lan": fullResolver()})
	setup := func(t *testing.T) (Paths, *fakeUnbound, *Renderer) {
		p := tmpPaths(t)
		listenSocket(t, p.ControlSocket())
		fu := &fakeUnbound{pid: os.Getpid(), conf: func() []byte { b, _ := os.ReadFile(p.Conf()); return b }} //nolint:gosec // test
		return p, fu, New(renderers.NewRecordingRunner().On(ControlBin, fu.run), WithPaths(p))
	}
	apply := func(t *testing.T, r *Renderer, d proto.Message) error {
		t.Helper()
		files, err := r.Render(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		return r.Apply(ctx, files)
	}

	t.Run("reload with convergence check", func(t *testing.T) {
		p, fu, r := setup(t)
		port := tcpPort(t)
		if err := os.WriteFile(p.Conf(), render(t, r, resolverOn(port)), 0o600); err != nil { // what unbound runs
			t.Fatal(err)
		}
		changed := resolverOn(port)
		changed.Resolvers["lan"].LocalZones[0].Records = changed.Resolvers["lan"].LocalZones[0].Records[:1]
		if err := apply(t, r, changed); err != nil {
			t.Fatal(err)
		}
		if fu.reloads != 1 {
			t.Fatalf("reloads %d", fu.reloads)
		}
		fu.forwards = ". IN forward 192.0.2.53\n" // corp.example.test. missing after reload
		changed.Resolvers["lan"].LocalZones[0].Records = nil
		if err := apply(t, r, changed); !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "corp.example.test.") {
			t.Fatalf("missing forward zone: want ErrDaemon, got %v", err)
		}
	})

	t.Run("listen change needs restart and stays pending", func(t *testing.T) {
		p, fu, r := setup(t)
		port, extra := tcpPort(t), tcpPort(t)
		if err := os.WriteFile(p.Conf(), render(t, r, resolverOn(port)), 0o600); err != nil {
			t.Fatal(err)
		}
		var ar *ActionRequired
		for i := 0; i < 2; i++ { // the second Apply of the same document must still ask (M2)
			if err := apply(t, r, resolverOn(port, extra)); !errors.As(err, &ar) || ar.Action != "restart" {
				t.Fatalf("apply %d: want restart, got %v", i, err)
			}
		}
		if fu.reloads != 0 || getPending(p.PendingFile) == nil {
			t.Fatalf("no reload may happen, the request must be persisted: reloads=%d", fu.reloads)
		}
		// a new Renderer (agent restart) still sees the request
		r2 := New(renderers.NewRecordingRunner().On(ControlBin, fu.run), WithPaths(p))
		if err := apply(t, r2, resolverOn(port, extra)); !errors.As(err, &ar) {
			t.Fatalf("after agent restart: want restart, got %v", err)
		}
		fu.pid = startChild(t) // unbound restarted after the request
		if err := apply(t, r2, resolverOn(port, extra)); err != nil {
			t.Fatalf("after the restart: %v", err)
		}
		if getPending(p.PendingFile) != nil || fu.reloads != 1 {
			t.Fatalf("pending not cleared or no reload: reloads=%d", fu.reloads)
		}
	})

	t.Run("stale socket is not running", func(t *testing.T) {
		p := tmpPaths(t)
		l, err := net.Listen("unix", p.ControlSocket())
		if err != nil {
			t.Fatal(err)
		}
		l.(*net.UnixListener).SetUnlinkOnClose(false)
		_ = l.Close() // the socket file stays, nobody listens (unbound killed)
		r := New(renderers.NewRecordingRunner(), WithPaths(p))
		var ar *ActionRequired
		if err := apply(t, r, full); !errors.As(err, &ar) || ar.Action != "start" {
			t.Fatalf("stale socket: want start, got %v", err)
		}
		if err := apply(t, r, nil); err != nil {
			t.Fatalf("idle config, not running: %v", err)
		}
	})

	t.Run("reload failure restores", func(t *testing.T) {
		p, fu, r := setup(t)
		port := tcpPort(t)
		old := render(t, r, resolverOn(port))
		if err := os.WriteFile(p.Conf(), old, 0o600); err != nil {
			t.Fatal(err)
		}
		fu.failNext = true
		changed := resolverOn(port)
		changed.Resolvers["lan"].LocalZones = nil
		if err := apply(t, r, changed); !errors.Is(err, ErrDaemon) {
			t.Fatalf("want ErrDaemon, got %v", err)
		}
		if b, _ := os.ReadFile(p.Conf()); string(b) != string(old) { //nolint:gosec // test
			t.Fatal("snapshot not restored")
		}
		if fu.reloads != 2 {
			t.Fatalf("want reload + reload of the restored file, got %d", fu.reloads)
		}
	})

	t.Run("output cap is an error", func(t *testing.T) {
		p := tmpPaths(t)
		listenSocket(t, p.ControlSocket())
		big := strings.Repeat("x", renderers.DefaultMaxOutput)
		r := New(renderers.NewRecordingRunner().Succeed(ControlBin, big), WithPaths(p))
		if _, err := r.Control(ctx, "list_local_data"); !errors.Is(err, ErrOutputTruncated) {
			t.Fatalf("want ErrOutputTruncated, got %v", err)
		}
		st, err := r.State(ctx)
		if err == nil || st.LocalDataTruncated {
			t.Logf("state: %v %v", st.LocalDataTruncated, err)
		}
	})
}

// TestProductPaths (review M4): the product paths validate, render the Debian control socket
// and pidfile directly in /run, and nothing points into a directory the package never creates.
func TestProductPaths(t *testing.T) {
	p := ProductPaths()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	r := New(renderers.NewRecordingRunner(), WithPaths(p))
	conf := string(render(t, r, dns(map[string]*vrxv1.DnsResolver{"lan": {Listen: []*vrxv1.SocketAddress{listen("10.0.0.1", 53)}}})))
	for _, want := range []string{`control-interface: "/run/unbound.ctl"`, `pidfile: "/run/unbound.pid"`, `directory: "/etc/unbound"`,
		`auto-trust-anchor-file: "/var/lib/unbound/root.key"`, `username: "unbound"`, "use-syslog: yes"} {
		if !strings.Contains(conf, want) {
			t.Errorf("product render lacks %s", want)
		}
	}
	if strings.Contains(conf, "/run/unbound/") {
		t.Error("product render points into /run/unbound/ (no RuntimeDirectory in unbound.service)")
	}
	idle := string(render(t, New(renderers.NewRecordingRunner(), WithPaths(unitPaths())), nil))
	if !strings.Contains(idle, "interface: 127.0.0.1@3653") || strings.Contains(idle, "@53\n") {
		t.Errorf("idle test instance must use the slot port (review L5):\n%s", idle)
	}
}

func TestParsers(t *testing.T) {
	z := ParseZones([]byte(". IN forward 192.0.2.53 192.0.2.54@5353\ncorp.example.test. IN forward +i +t 2001:db8::53#dns.example.test\n"))
	if len(z) != 2 || z[0].Zone != "." || len(z[0].Addrs) != 2 || z[1].Flags[1] != "+t" {
		t.Fatalf("%+v", z)
	}
	lz := ParseLocalZones([]byte("lan.example.test. static\nlocalhost. redirect\n"))
	if len(lz) != 2 || lz[0].Type != "static" {
		t.Fatalf("%+v", lz)
	}
	ld := ParseLocalData([]byte("gw.lan.example.test.\t3600\tIN\tA\t10.6.0.1\n"))
	if len(ld) != 1 || ld[0] != "gw.lan.example.test. 3600 IN A 10.6.0.1" {
		t.Fatalf("%q", ld)
	}
	s := ParseStats([]byte("total.num.queries=12\ntime.up=3.5\n"))
	if s["total.num.queries"] != "12" {
		t.Fatal(s)
	}
	st := ParseStatus([]byte("version: 1.24.2\nverbosity: 1\nunbound (pid 12) is running...\n"))
	if st["version"] != "1.24.2" || st["state"] == "" {
		t.Fatal(st)
	}
}
