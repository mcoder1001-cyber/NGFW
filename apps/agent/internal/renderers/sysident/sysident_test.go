package sysident

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// testPaths is a temp root with a fake tzdata holding UTC and Asia/Tehran.
func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	p := PathsUnder(filepath.Join(root, "rig"))
	p.ZoneinfoDir = filepath.Join(root, "zoneinfo")
	for _, z := range []string{"UTC", "Asia/Tehran"} {
		f := filepath.Join(p.ZoneinfoDir, z)
		if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("TZif"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func sample() *vrxv1.SystemConfig {
	return &vrxv1.SystemConfig{
		Hostname: proto.String("vrx-a.lab.example"),
		Timezone: proto.String("Asia/Tehran"),
		Banner: &vrxv1.SystemBanner{
			Login: proto.String("Authorised access only.\n\tLab w10"),
			Motd:  proto.String("Welcome to vrx-a\n"),
		},
		Dns: &vrxv1.SystemDns{
			Servers:       []string{"10.10.53.1", "2001:db8::53"},
			SearchDomains: []string{"lab.example", "corp.example"},
			Vrf:           proto.String("default"),
		},
	}
}

func newDesc(p Paths) (*Descriptor, *string) {
	kernel := "host-kernel"
	d := New(p, nil)
	d.hostname = func() (string, error) { return kernel, nil }
	d.sethost = func(h string) error { kernel = h; return nil }
	return d, &kernel
}

// TestRenderGolden: every rendered file under the temp root matches its golden (go test -update rewrites them).
func TestRenderGolden(t *testing.T) {
	p := testPaths(t)
	d, _ := newDesc(p)
	if err := d.Apply(Input(sample())); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"hostname": p.Hostname, "issue": p.Issue, "issue.net": p.IssueNet, "motd": p.Motd, "resolved-vrx.conf": p.ResolvedDropIn,
	} {
		got, err := os.ReadFile(path) //nolint:gosec // temp rig
		if err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join("testdata", name+".golden")
		if *update {
			if err := os.WriteFile(golden, got, 0o600); err != nil { //nolint:gosec // testdata
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden) //nolint:gosec // testdata
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
			t.Errorf("%s mode %v", name, fi.Mode())
		}
	}
	if l, err := os.Readlink(p.Localtime); err != nil || l != filepath.Join(p.ZoneinfoDir, "Asia/Tehran") {
		t.Fatalf("localtime -> %q (%v)", l, err)
	}
}

// TestUnchangedWritesNothing: a second apply of the same input (an agent restart's resync) writes no file.
func TestUnchangedWritesNothing(t *testing.T) {
	p := testPaths(t)
	p.SetKernelHostname = true
	d, kernel := newDesc(p)
	ch, err := d.apply(Input(sample()))
	if err != nil || len(ch.Files) != 5 || !ch.Localtime || !ch.Kernel || *kernel != "vrx-a.lab.example" {
		t.Fatalf("first apply %+v %v kernel=%q", ch, err, *kernel)
	}
	fi, _ := os.Stat(p.Hostname)
	d2, _ := newDesc(p) // a restarted agent
	d2.hostname = func() (string, error) { return *kernel, nil }
	d2.sethost = func(string) error { t.Fatal("sethostname on an unchanged document"); return nil }
	ch, err = d2.apply(Input(sample()))
	if err != nil || len(ch.Files) != 0 || ch.Localtime || ch.Kernel {
		t.Fatalf("second apply wrote %+v %v", ch, err)
	}
	if fi2, _ := os.Stat(p.Hostname); !fi2.ModTime().Equal(fi.ModTime()) {
		t.Fatal("hostname rewritten")
	}
	// only the motd changes
	in := Input(sample())
	in.Banner.Motd = proto.String("changed")
	ch, err = d2.apply(in) // the drop-in embeds the input, so it changes too
	if err != nil || len(ch.Files) != 2 {
		t.Fatalf("motd change: %+v %v", ch, err)
	}
}

func TestDescriptorLifecycle(t *testing.T) {
	ctx := context.Background()
	p := testPaths(t)
	d, _ := newDesc(p)
	if d.Name() != Name || Key != "system.identity/vrx" || d.KeyOf(nil) != Key || d.Dependencies(nil) != nil || d.Stage() != scheduler.StageDaemon {
		t.Fatal("identity")
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 0 {
		t.Fatalf("nothing written: %v %v", kvs, err)
	}
	in := Input(sample())
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	if len(kvs) != 1 || !proto.Equal(kvs[0].Value, in) {
		t.Fatalf("retrieve after create: %v", kvs)
	}
	// hand edit → drift → update repairs
	if err := os.WriteFile(p.Motd, []byte("edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	drift, ok := kvs[0].Value.(*structpb.Struct)
	if !ok || !strings.Contains(drift.String(), "motd") {
		t.Fatalf("drift %v", kvs)
	}
	if _, err := d.Update(ctx, drift, in, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); !proto.Equal(kvs[0].Value, in) {
		t.Fatalf("after update: %v", kvs)
	}
	// a re-pointed localtime is drift too
	if err := symlinkAtomic(filepath.Join(p.ZoneinfoDir, "UTC"), p.Localtime); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); proto.Equal(kvs[0].Value, in) {
		t.Fatal("localtime drift not seen")
	}
	if err := d.Delete(ctx, in, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %v", kvs)
	}
	if _, err := os.Stat(p.Hostname); err != nil {
		t.Fatal("delete must leave the host name file")
	}
}

func TestInputDefaults(t *testing.T) {
	in := Input(nil)
	if in.GetHostname() != "vrx" || in.GetTimezone() != "UTC" || in.GetDns().GetVrf() != "default" || in.Banner == nil {
		t.Fatalf("defaults %v", in)
	}
	e := Input(&vrxv1.SystemConfig{Banner: &vrxv1.SystemBanner{Login: proto.String("")}})
	if e.Banner.Login != nil {
		t.Fatal("empty banner kept")
	}
}

// TestValidator: the tier-3 validator rejects a bad zone and control characters, naming the leaf.
func TestValidator(t *testing.T) {
	p := testPaths(t)
	d, _ := newDesc(p)
	ctx := context.Background()
	if err := d.Validate(ctx, Key, sample(), nil); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for name, tc := range map[string]struct {
		mut  func(*vrxv1.SystemConfig)
		ptr  string
		frag string
	}{
		"unknown zone":    {func(c *vrxv1.SystemConfig) { c.Timezone = proto.String("Mars/Olympus") }, "/system/timezone", "unknown time zone"},
		"zone traversal":  {func(c *vrxv1.SystemConfig) { c.Timezone = proto.String("Asia/../../etc/passwd") }, "/system/timezone", "IANA"},
		"escape in login": {func(c *vrxv1.SystemConfig) { c.Banner.Login = proto.String("hi\x1b[2J") }, "/system/banner/login", "U+001B"},
		"CR in motd":      {func(c *vrxv1.SystemConfig) { c.Banner.Motd = proto.String("a\rb") }, "/system/banner/motd", "U+000D"},
		"C1 in motd":      {func(c *vrxv1.SystemConfig) { c.Banner.Motd = proto.String("a\u009bb") }, "/system/banner/motd", "U+009B"},
		"bidi in login":   {func(c *vrxv1.SystemConfig) { c.Banner.Login = proto.String("a\u202eb") }, "/system/banner/login", "bidirectional"},
		"bad host":        {func(c *vrxv1.SystemConfig) { c.Hostname = proto.String("-bad") }, "/system/hostname", "RFC 1123"},
		"bad server":      {func(c *vrxv1.SystemConfig) { c.Dns.Servers = []string{"ns1"} }, "/system/dns/servers/0", "IP address"},
		"vrf":             {func(c *vrxv1.SystemConfig) { c.Dns.Vrf = proto.String("mgmt") }, "/system/dns/vrf", "default VRF"},
	} {
		c := sample()
		tc.mut(c)
		err := d.Validate(ctx, Key, c, nil)
		var ve *scheduler.ValidationError
		if !errors.As(err, &ve) || ve.Pointer != tc.ptr || !strings.Contains(err.Error(), tc.frag) {
			t.Errorf("%s: %v", name, err)
		}
		// Apply refuses the same input and writes nothing
		if err := d.Apply(Input(c)); err == nil {
			t.Errorf("%s: applied", name)
		}
		if _, err := os.Stat(p.Hostname); err == nil {
			t.Errorf("%s: wrote files", name)
		}
	}
}

func TestBannerOK(t *testing.T) {
	if err := BannerOK("مرحبا — Authorised only\n\tok"); err != nil {
		t.Fatal(err)
	}
	if BannerOK("\xff") == nil || BannerOK("\x07") == nil || BannerOK("\x7f") == nil {
		t.Fatal("invalid accepted")
	}
}

func TestPaths(t *testing.T) {
	if err := ProductPaths().Validate(); err != nil {
		t.Fatal(err)
	}
	u := PathsUnder("/run/vrx-test/w3/sysident")
	if u.SetKernelHostname || u.Hostname != "/run/vrx-test/w3/sysident/etc/hostname" || u.ZoneinfoDir != "/usr/share/zoneinfo" {
		t.Fatalf("%+v", u)
	}
	bad := ProductPaths()
	bad.Motd = "relative"
	if bad.Validate() == nil {
		t.Fatal("relative path accepted")
	}
}
