package pppoe

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func testRenderer() *Renderer { return New(WithPaths(PathsUnder("/srv"))) }

func fullSession() Session {
	return Session{ //nolint:gosec // G101: test fixture, not a real credential
		Iface: "GigabitEthernet0/0/0", HostIf: "wan0",
		Username: "alice@isp", Password: "s3cr#t \"x", ServiceName: "ngfw-fiber", //nolint:gosec // G101: test data, not a real credential
		MTU: 1492, MSSClamp: true, DefaultRoute: true, DNSFromPeer: true, IPv6: "slaac",
		HoldoffSec: 5, MaxFail: 0,
	}
}

func minimalSession() Session {
	return Session{
		Iface: "eth1", HostIf: "wan1", Username: "bob", Password: "pw",
		MTU: 1480, DefaultRoute: false, DNSFromPeer: false, IPv6: "off", HoldoffSec: 10, MaxFail: 3,
	}
}

func dhcp6Session() Session {
	return Session{
		Iface: "eth2", HostIf: "wan2", Username: "carol", Password: "pw2",
		MTU: 1492, MSSClamp: true, DefaultRoute: false, IPv6: "dhcpv6", HoldoffSec: 5,
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // golden file path from test constant
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestRenderGolden(t *testing.T) {
	files, err := testRenderer().Render([]Session{fullSession(), minimalSession(), dhcp6Session()})
	if err != nil {
		t.Fatal(err)
	}
	// one golden per rendered path (sorted), concatenated with a header line
	var b strings.Builder
	for _, p := range files.Paths() {
		f := files[p]
		b.WriteString("### " + strings.TrimPrefix(p, "/srv") + " mode=" + f.Mode.String())
		if f.Secret {
			b.WriteString(" secret")
		}
		b.WriteString("\n")
		b.Write(f.Content)
		b.WriteString("\n")
	}
	golden(t, "two-sessions", []byte(b.String()))
}

func TestNoSessionsRendersNoConfig(t *testing.T) {
	files, err := testRenderer().Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("want no files, got %v", files.Paths())
	}
}

func TestSecretsAreSecretAndNotInPeerFile(t *testing.T) {
	files, err := testRenderer().Render([]Session{fullSession()})
	if err != nil {
		t.Fatal(err)
	}
	peer := files["/srv/etc/ppp/peers/ngfw-wan0"]
	if strings.Contains(string(peer.Content), "s3cr") {
		t.Fatal("the password leaked into the peer file")
	}
	for _, p := range []string{"/srv/etc/ppp/chap-secrets", "/srv/etc/ppp/pap-secrets"} {
		f := files[p]
		if !f.Secret || f.Mode != 0o600 {
			t.Fatalf("%s must be Secret 0600, got secret=%v mode=%v", p, f.Secret, f.Mode)
		}
		if !strings.Contains(string(f.Content), `"alice@isp" ngfw-wan0 "s3cr#t \"x" *`) {
			t.Fatalf("%s missing the secrets line:\n%s", p, f.Content)
		}
	}
	// Redacted() hides the secret content (used in logs/diffs)
	if c := files.Redacted()["/srv/etc/ppp/chap-secrets"].Content; string(c) != "<redacted>" {
		t.Fatalf("Redacted did not hide the secret: %q", c)
	}
}

func TestHostileInputRejected(t *testing.T) {
	base := minimalSession()
	for name, mut := range map[string]func(*Session){
		"bad hostif":   func(s *Session) { s.HostIf = "wan 0" },
		"long hostif":  func(s *Session) { s.HostIf = strings.Repeat("x", 16) },
		"newline user": func(s *Session) { s.Username = "a\nplugin evil" },
		"quote user":   func(s *Session) { s.Username = "a\"b" },
		"newline pass": func(s *Session) { s.Password = "a\nrequire-pap" },
		"newline svc":  func(s *Session) { s.ServiceName = "a\nx" },
		"tiny mtu":     func(s *Session) { s.MTU = 64 },
		"huge mtu":     func(s *Session) { s.MTU = 9000 },
		"ipv6 mode":    func(s *Session) { s.IPv6 = "dhcpv6-pd" },
		"ipv6 mtu":     func(s *Session) { s.IPv6 = "slaac"; s.MTU = 1279 },
	} {
		t.Run(name, func(t *testing.T) {
			s := base
			mut(&s)
			if _, err := testRenderer().Render([]Session{s}); !errors.Is(err, ErrInput) {
				t.Fatalf("want ErrInput, got %v", err)
			}
		})
	}
}

func TestDuplicateHostIfRejected(t *testing.T) {
	a := minimalSession()
	b := fullSession()
	b.HostIf = a.HostIf
	if _, err := testRenderer().Render([]Session{a, b}); !errors.Is(err, ErrInput) {
		t.Fatalf("want ErrInput on duplicate host interface, got %v", err)
	}
}

func TestValidateMatchesRender(t *testing.T) {
	if err := testRenderer().Validate([]Session{fullSession()}); err != nil {
		t.Fatal(err)
	}
	bad := minimalSession()
	bad.HostIf = "bad/name"
	if err := testRenderer().Validate([]Session{bad}); !errors.Is(err, ErrInput) {
		t.Fatalf("want ErrInput, got %v", err)
	}
}

// every rendered file passes the shared CheckRendered backstop (no CR/NUL/ESC).
func TestRenderedFilesAreClean(t *testing.T) {
	files, err := testRenderer().Render([]Session{fullSession(), minimalSession(), dhcp6Session()})
	if err != nil {
		t.Fatal(err)
	}
	paths := files.Paths()
	sort.Strings(paths)
	for _, p := range paths {
		if err := renderers.CheckRendered(files[p].Content); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
