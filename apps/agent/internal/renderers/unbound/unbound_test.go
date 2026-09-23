package unbound

import (
	"context"
	"errors"
	"flag"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func unitPaths() Paths { return TestPaths("w0") }

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
	p.ConfDir, p.RunDir = d, d
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

func TestApply(t *testing.T) {
	ctx := context.Background()
	full := dns(map[string]*vrxv1.DnsResolver{"lan": fullResolver()})

	t.Run("reload through unbound-control", func(t *testing.T) {
		p := tmpPaths(t)
		listenSocket(t, p.ControlSocket())
		rr := renderers.NewRecordingRunner().Succeed(ControlBin, "ok")
		r := New(rr, WithPaths(p))
		files, _ := r.Render(ctx, full)
		if err := r.Apply(ctx, files); err != nil {
			t.Fatal(err)
		}
		c := rr.Calls()
		if len(c) != 1 || strings.Join(c[0].Args, " ") != "-c "+p.Conf()+" reload_keep_cache" {
			t.Fatalf("argv %v", c)
		}
		b, _ := os.ReadFile(p.Conf()) //nolint:gosec // test
		if string(b) != string(files[p.Conf()].Content) {
			t.Fatal("file not written")
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
		files, _ := r.Render(ctx, full)
		var ar *ActionRequired
		if err := r.Apply(ctx, files); !errors.As(err, &ar) {
			t.Fatalf("stale socket: want ActionRequired, got %v", err)
		}
	})

	t.Run("not running", func(t *testing.T) {
		p := tmpPaths(t)
		r := New(renderers.NewRecordingRunner(), WithPaths(p))
		files, _ := r.Render(ctx, full)
		var ar *ActionRequired
		if err := r.Apply(ctx, files); !errors.As(err, &ar) || ar.Action != "start" {
			t.Fatalf("want ActionRequired start, got %v", err)
		}
		files, _ = r.Render(ctx, nil)
		if err := r.Apply(ctx, files); err != nil {
			t.Fatalf("idle config, not running: %v", err)
		}
	})

	t.Run("reload failure restores", func(t *testing.T) {
		p := tmpPaths(t)
		listenSocket(t, p.ControlSocket())
		old := []byte("# old\n")
		if err := os.WriteFile(p.Conf(), old, 0o600); err != nil {
			t.Fatal(err)
		}
		rr := renderers.NewRecordingRunner().FailWith(ControlBin, 1, "error: reload failed")
		r := New(rr, WithPaths(p))
		files, _ := r.Render(ctx, full)
		if err := r.Apply(ctx, files); !errors.Is(err, ErrDaemon) {
			t.Fatalf("want ErrDaemon, got %v", err)
		}
		if b, _ := os.ReadFile(p.Conf()); string(b) != string(old) { //nolint:gosec // test
			t.Fatal("snapshot not restored")
		}
		if len(rr.Calls()) != 2 {
			t.Fatalf("want reload + reload of the restored file, got %v", rr.Calls())
		}
	})
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
