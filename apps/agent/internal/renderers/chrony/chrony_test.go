package chrony

import (
	"context"
	"encoding/hex"
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

// Test fixture secrets use the literal VRX_TEST_PSK_<id> (00-CONTEXT).
var fixtureSecrets = map[string][]byte{
	"key/upstream": []byte("VRX_TEST_PSK_RF3_upstream"),
	"key/backup":   []byte("VRX_TEST_PSK_RF3_backup"),
}

func resolver(ref string) ([]byte, error) {
	if v, ok := fixtureSecrets[ref]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("no secret %s", ref)
}

func unitPaths() Paths { return TestPaths("w0", "unit") }

func newUnit(p Paths) *Renderer {
	return New(renderers.NewRecordingRunner(), WithPaths(p), WithSecrets(resolver))
}

func clientNTP() *vrxv1.NtpService {
	return &vrxv1.NtpService{
		Enabled: proto.Bool(true),
		Vrf:     proto.String("default"),
		Servers: []*vrxv1.NtpService_Server{
			{Address: proto.String("192.0.2.123"), Prefer: proto.Bool(true), MinPoll: proto.Int32(4), MaxPoll: proto.Int32(6), KeyRef: proto.String("key/upstream")},
			{Address: proto.String("NTP.Example.test"), Iburst: proto.Bool(false), KeyRef: proto.String("key/backup")},
			{Address: proto.String("2001:db8::123"), KeyRef: proto.String("key/upstream")},
			{Address: proto.String("nts.example.test"), Nts: proto.Bool(true)},
		},
		Pools: []string{"pool.example.test"},
		Port:  proto.Uint32(0),
	}
}

func serverNTP() *vrxv1.NtpService {
	return &vrxv1.NtpService{
		Enabled:      proto.Bool(true),
		Servers:      []*vrxv1.NtpService_Server{{Address: proto.String("127.0.0.1")}},
		Allow:        []string{"127.0.0.1/32", "10.6.0.9/16", "fd00:6::/64"},
		Deny:         []string{"10.6.66.0/24"},
		Listen:       []string{"127.0.0.1", "::1"},
		Port:         proto.Uint32(3623),
		RateLimit:    &vrxv1.NtpService_RateLimit{Interval: proto.Int32(-2), Burst: proto.Uint32(16), Leak: proto.Uint32(1)},
		LocalStratum: proto.Uint32(10),
		Orphan:       proto.Bool(true),
		RtcSync:      proto.Bool(true),
		Makestep:     &vrxv1.NtpService_Makestep{ThresholdSec: proto.Float64(0.5), Limit: proto.Int32(-1)},
	}
}

func render(t *testing.T, r *Renderer, desired proto.Message) renderers.Files {
	t.Helper()
	files, err := r.Render(context.Background(), desired)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return files
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

// TestRenderGolden covers chrony.conf and vrx.sources for every template path. chrony.keys
// is asserted structurally (TestKeys) instead of stored as a golden file: even fixture key
// material stays out of testdata.
func TestRenderGolden(t *testing.T) {
	prod := ProductPaths()
	cases := map[string]struct {
		paths   Paths
		desired proto.Message
	}{
		"disabled": {unitPaths(), nil},
		"client":   {unitPaths(), &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Ntp: clientNTP()}}},
		"server":   {unitPaths(), &vrxv1.ServicesConfig{Ntp: serverNTP()}},
		"product":  {prod, serverNTP()},
		"local-only": {unitPaths(), &vrxv1.NtpService{
			Enabled: proto.Bool(true), LocalStratum: proto.Uint32(8), Port: proto.Uint32(0),
		}},
		"test-source-port": {func() Paths { p := unitPaths(); p.SourcePort = 3623; return p }(), &vrxv1.NtpService{
			Enabled: proto.Bool(true), Servers: []*vrxv1.NtpService_Server{{Address: proto.String("127.0.0.1"), MinPoll: proto.Int32(-2), MaxPoll: proto.Int32(0)}}, Port: proto.Uint32(0),
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			files := render(t, newUnit(c.paths), c.desired)
			if len(files) != 3 {
				t.Fatalf("want 3 files, got %v", files.Paths())
			}
			golden(t, name+".chrony.conf", files[c.paths.Conf()].Content)
			golden(t, name+".vrx.sources", files[c.paths.Sources()].Content)
			k := files[c.paths.Keys()]
			if !k.Secret || k.Mode != c.paths.KeyMode || k.Owner != c.paths.KeyOwner {
				t.Fatalf("keys file: secret=%v mode=%v owner=%q", k.Secret, k.Mode, k.Owner)
			}
			again := render(t, newUnit(c.paths), c.desired)
			for p := range files {
				if string(files[p].Content) != string(again[p].Content) {
					t.Errorf("%s: render is not deterministic", p)
				}
			}
		})
	}
}

func TestKeys(t *testing.T) {
	files := render(t, newUnit(unitPaths()), clientNTP())
	keys := string(files[unitPaths().Keys()].Content)
	lines := strings.Split(strings.TrimSpace(keys), "\n")
	// comment + two keys in sorted reference order, ids = keyID(ref) (review L6)
	idB, idU := keyID("key/backup"), keyID("key/upstream")
	if len(lines) != 3 || !keyLineRe.MatchString(lines[1]) || !keyLineRe.MatchString(lines[2]) {
		t.Fatalf("keys file lines: %d", len(lines))
	}
	if !strings.HasPrefix(lines[1], fmt.Sprintf("%d SHA256 HEX:", idB)+strings.ToUpper(hex.EncodeToString(fixtureSecrets["key/backup"]))) ||
		!strings.HasPrefix(lines[2], fmt.Sprintf("%d SHA256 HEX:", idU)+strings.ToUpper(hex.EncodeToString(fixtureSecrets["key/upstream"]))) {
		t.Fatal("key ids/values not as expected")
	}
	src := string(files[unitPaths().Sources()].Content)
	if !strings.Contains(src, fmt.Sprintf("server 192.0.2.123 iburst prefer minpoll 4 maxpoll 6 key %d", idU)) || !strings.Contains(src, fmt.Sprintf("server ntp.example.test key %d", idB)) {
		t.Fatalf("sources do not reference the key ids:\n%s", src)
	}
	// no secret material outside chrony.keys, in any encoding
	for p, f := range files {
		if p == unitPaths().Keys() {
			continue
		}
		for _, v := range fixtureSecrets {
			for _, enc := range []string{string(v), hex.EncodeToString(v), strings.ToUpper(hex.EncodeToString(v))} {
				if strings.Contains(string(f.Content), enc) {
					t.Fatalf("%s contains key material", p)
				}
			}
		}
	}
	red := files.Redacted()
	if string(red[unitPaths().Keys()].Content) != "<redacted>" {
		t.Fatal("Redacted() keeps key material")
	}
}

func TestSecretErrorsDoNotLeak(t *testing.T) {
	leaky := func(string) ([]byte, error) {
		return nil, fmt.Errorf("backend said: value VRX_TEST_PSK_RF3_leak is expired")
	}
	r := New(renderers.NewRecordingRunner(), WithPaths(unitPaths()), WithSecrets(leaky))
	_, err := r.Render(context.Background(), clientNTP())
	if !errors.Is(err, ErrSecret) || strings.Contains(err.Error(), "VRX_TEST_PSK") {
		t.Fatalf("want ErrSecret without the value, got %v", err)
	}
	long := func(string) ([]byte, error) { return []byte(strings.Repeat("k", MaxKeyBytes+1)), nil }
	r = New(renderers.NewRecordingRunner(), WithPaths(unitPaths()), WithSecrets(long))
	if _, err := r.Render(context.Background(), clientNTP()); !errors.Is(err, ErrSecret) || strings.Contains(err.Error(), "kkkk") {
		t.Fatalf("oversized key: %v", err)
	}
	r = New(renderers.NewRecordingRunner(), WithPaths(unitPaths()))
	if _, err := r.Render(context.Background(), clientNTP()); !errors.Is(err, ErrSecret) {
		t.Fatalf("no resolver: %v", err)
	}
}

var hostile = []string{
	`"; rm -rf /`, "x\ninclude /etc/passwd", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "\xff\xfe", "bücher.test", strings.Repeat("a", 5*1024),
}

func TestRejects(t *testing.T) {
	type mut func(n *vrxv1.NtpService)
	cases := map[string]mut{
		"listen-host-nic": func(n *vrxv1.NtpService) { n.Listen = []string{"172.30.126.195"} },
		"listen-two-v4":   func(n *vrxv1.NtpService) { n.Listen = []string{"127.0.0.1", "127.0.0.2"} },
		"listen-none":     func(n *vrxv1.NtpService) { n.Listen = nil },
		"port":            func(n *vrxv1.NtpService) { n.Port = proto.Uint32(70000) },
		"port0-serving":   func(n *vrxv1.NtpService) { n.Port = proto.Uint32(0) },
		"allow-dup":       func(n *vrxv1.NtpService) { n.Allow = []string{"10.0.0.0/8", "10.1.0.0/8"} },
		"nts-server": func(n *vrxv1.NtpService) {
			n.NtsServer = &vrxv1.NtpService_NtsServer{CertificateRef: proto.String("cert/x"), KeyRef: proto.String("key/x")}
		},
		"ratelimit":     func(n *vrxv1.NtpService) { n.RateLimit.Burst = proto.Uint32(999) },
		"ratelimit-cli": func(n *vrxv1.NtpService) { n.Allow = nil; n.Port = proto.Uint32(0); n.Listen = nil },
		"stratum":       func(n *vrxv1.NtpService) { n.LocalStratum = proto.Uint32(16) },
		"orphan":        func(n *vrxv1.NtpService) { n.LocalStratum = nil },
		"makestep":      func(n *vrxv1.NtpService) { n.Makestep.ThresholdSec = proto.Float64(0) },
		"poll":          func(n *vrxv1.NtpService) { n.Servers[0].MinPoll = proto.Int32(30) },
		"poll-order":    func(n *vrxv1.NtpService) { n.Servers[0].MinPoll, n.Servers[0].MaxPoll = proto.Int32(8), proto.Int32(4) },
		"nts-and-key": func(n *vrxv1.NtpService) {
			n.Servers[0].Nts, n.Servers[0].KeyRef = proto.Bool(true), proto.String("key/upstream")
		},
		"keyref":     func(n *vrxv1.NtpService) { n.Servers[0].KeyRef = proto.String("psk/upstream") },
		"server-dup": func(n *vrxv1.NtpService) { n.Servers = append(n.Servers, n.Servers[0]) },
		"pool-ip":    func(n *vrxv1.NtpService) { n.Pools = []string{"192.0.2.1"} },
		"all-digits": func(n *vrxv1.NtpService) { n.Servers[0].Address = proto.String("192.0.2") },
		"nothing": func(n *vrxv1.NtpService) {
			n.Servers, n.LocalStratum, n.Orphan, n.Allow, n.RateLimit, n.Port, n.Listen = nil, nil, nil, nil, nil, proto.Uint32(0), nil
		},
		"vrf": func(n *vrxv1.NtpService) { n.Vrf = proto.String("a b") },
	}
	for i, h := range hostile {
		h := h
		cases[fmt.Sprintf("server-hostile-%d", i)] = func(n *vrxv1.NtpService) { n.Servers[0].Address = proto.String(h) }
		cases[fmt.Sprintf("pool-hostile-%d", i)] = func(n *vrxv1.NtpService) { n.Pools = []string{h} }
		cases[fmt.Sprintf("keyref-hostile-%d", i)] = func(n *vrxv1.NtpService) { n.Servers[0].KeyRef = proto.String("key/" + h) }
		cases[fmt.Sprintf("allow-hostile-%d", i)] = func(n *vrxv1.NtpService) { n.Allow = []string{h} }
		cases[fmt.Sprintf("listen-hostile-%d", i)] = func(n *vrxv1.NtpService) { n.Listen = []string{h} }
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			n := serverNTP()
			m(n)
			_, err := New(renderers.NewRecordingRunner(), WithPaths(unitPaths()), WithSecrets(resolver)).Render(context.Background(), n)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestTemplateHelpers(t *testing.T) {
	for _, bad := range []string{"a b", "x\ny", `"q"`, "::1 include"} {
		if _, err := safeHost(bad); err == nil {
			t.Errorf("safeHost(%q) accepted", bad)
		}
	}
	if _, err := safePath("/a b"); err == nil {
		t.Error("safePath accepted a space")
	}
	if _, err := hexKey("abc"); err == nil {
		t.Error("hexKey accepted lower case / odd input")
	}
}

func TestValidate(t *testing.T) {
	rr := renderers.NewRecordingRunner().Succeed(ChronydBin, "")
	r := New(rr, WithPaths(unitPaths()), WithSecrets(resolver))
	files := render(t, r, clientNTP())
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	c := rr.Calls()
	if len(c) != 2 || c[0].Args[0] != "-p" || c[0].Args[1] != "-f" || !strings.HasSuffix(c[0].Args[2], "/chrony.conf") ||
		!strings.HasSuffix(c[1].Args[2], "/sources.d/vrx.sources") || strings.HasPrefix(c[0].Args[2], unitPaths().ConfDir) {
		t.Fatalf("argv %v", c)
	}
	bad := renderers.Files{}
	for p, f := range files {
		bad[p] = f
	}
	k := bad[unitPaths().Keys()]
	k.Content = []byte("1 SHA256 HEX:VRX_TEST_PSK_notHex\n")
	bad[unitPaths().Keys()] = k
	err := r.Validate(context.Background(), bad)
	if !errors.Is(err, ErrDaemon) || strings.Contains(err.Error(), "VRX_TEST_PSK") {
		t.Fatalf("malformed keys: %v", err)
	}
	k.Secret = false
	bad[unitPaths().Keys()] = k
	if err := r.Validate(context.Background(), bad); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Fatalf("keys not secret: %v", err)
	}
	rr = renderers.NewRecordingRunner().FailWith(ChronydBin, 1, "Fatal error : Invalid directive x at line 3")
	r = New(rr, WithPaths(unitPaths()), WithSecrets(resolver))
	if err := r.Validate(context.Background(), files); !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "Invalid directive") {
		t.Fatalf("checker failure: %v", err)
	}
}

func tmpPaths(t *testing.T) Paths {
	p := unitPaths()
	d := t.TempDir()
	p.ConfDir, p.RunDir, p.StateDir, p.LogDir = d, d, d, d
	p.FileOwner, p.KeyOwner = "", ""
	p.PendingFile = filepath.Join(d, "state", "vrx.pending")
	if err := os.MkdirAll(p.SourceDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	return p
}

func writePid(t *testing.T, p Paths, pid int) {
	t.Helper()
	if err := os.WriteFile(p.PidFile(), []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// startChild starts a process that stands in for a restarted chronyd (it starts after any
// request recorded before).
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

func TestKeyIDsStable(t *testing.T) {
	a := render(t, newUnit(unitPaths()), clientNTP())
	n := clientNTP()
	n.Servers = append(n.Servers, &vrxv1.NtpService_Server{Address: proto.String("192.0.2.200"), KeyRef: proto.String("key/aaa")})
	fixtureSecrets["key/aaa"] = []byte("VRX_TEST_PSK_RF3_aaa")
	defer delete(fixtureSecrets, "key/aaa")
	b := render(t, newUnit(unitPaths()), n)
	for _, l := range strings.Split(string(a[unitPaths().Sources()].Content), "\n") {
		if strings.HasPrefix(l, "server ") && !strings.Contains(string(b[unitPaths().Sources()].Content), l) {
			t.Fatalf("adding key/aaa changed %q", l)
		}
	}
}

func TestApply(t *testing.T) {
	ctx := context.Background()
	socket := func(p Paths) { // stands in for chronyd's command socket
		c, err := net.ListenPacket("unixgram", p.Socket())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
	}
	t.Run("stale socket is not running", func(t *testing.T) {
		p := tmpPaths(t)
		if err := os.WriteFile(p.Socket(), nil, 0o600); err != nil { // file left behind, nobody listens
			t.Fatal(err)
		}
		r := New(renderers.NewRecordingRunner(), WithPaths(p), WithSecrets(resolver))
		var ar *ActionRequired
		if err := r.Apply(ctx, render(t, r, clientNTP())); !errors.As(err, &ar) || ar.Action != "start" {
			t.Fatalf("want start, got %v", err)
		}
	})
	t.Run("not running", func(t *testing.T) {
		p := tmpPaths(t)
		r := New(renderers.NewRecordingRunner(), WithPaths(p), WithSecrets(resolver))
		var ar *ActionRequired
		if err := r.Apply(ctx, render(t, r, clientNTP())); !errors.As(err, &ar) || ar.Action != "start" || ar.Unit != "chrony" {
			t.Fatalf("want start, got %v", err)
		}
		if err := r.Apply(ctx, render(t, r, nil)); err != nil {
			t.Fatalf("disabled, not running: %v", err)
		}
	})
	t.Run("sources and keys reload and conf restart", func(t *testing.T) {
		p := tmpPaths(t)
		socket(p)
		rr := renderers.NewRecordingRunner().Succeed(ChronycBin, "200 OK")
		r := New(rr, WithPaths(p), WithSecrets(resolver))
		base := clientNTP()
		first := render(t, r, base)
		var ar *ActionRequired
		if err := r.Apply(ctx, first); !errors.As(err, &ar) || ar.Action != "restart" {
			t.Fatalf("first write changes chrony.conf: want restart, got %v", err)
		}
		// M2: the unchanged file must still ask for the restart until chronyd restarted, also
		// from a new Renderer (agent restart).
		writePid(t, p, os.Getpid()) // a daemon that started before the request
		for _, rx := range []*Renderer{r, New(rr, WithPaths(p), WithSecrets(resolver))} {
			if err := rx.Apply(ctx, first); !errors.As(err, &ar) || ar.Action != "restart" || !strings.Contains(ar.Reason, "still pending") {
				t.Fatalf("second Apply: want pending restart, got %v", err)
			}
		}
		writePid(t, p, startChild(t)) // chronyd restarted after the request
		rr.Reset()
		if err := r.Apply(ctx, first); err != nil || len(rr.Calls()) != 0 {
			t.Fatalf("idempotent re-apply: %v %v", err, rr.Calls())
		}
		changed := clientNTP()
		changed.Servers[0].Prefer = proto.Bool(false)
		if err := r.Apply(ctx, render(t, r, changed)); err != nil {
			t.Fatal(err)
		}
		if c := rr.Calls(); len(c) != 1 || strings.Join(c[0].Args, " ") != "-h "+p.Socket()+" reload sources" {
			t.Fatalf("want reload sources, got %v", c)
		}
		rr.Reset()
		fixtureSecrets["key/backup"] = []byte("VRX_TEST_PSK_RF3_backup2")
		defer func() { fixtureSecrets["key/backup"] = []byte("VRX_TEST_PSK_RF3_backup") }()
		if err := r.Apply(ctx, render(t, r, changed)); err != nil {
			t.Fatal(err)
		}
		if c := rr.Calls(); len(c) != 1 || c[0].Args[2] != "rekey" {
			t.Fatalf("want rekey, got %v", c)
		}
		if info, _ := os.Stat(p.Keys()); info.Mode().Perm() != 0o600 {
			t.Fatalf("keys mode %v", info.Mode())
		}
	})
	t.Run("chronyc failure restores", func(t *testing.T) {
		p := tmpPaths(t)
		socket(p)
		ok := renderers.NewRecordingRunner().Succeed(ChronycBin, "200 OK")
		r := New(ok, WithPaths(p), WithSecrets(resolver))
		first := render(t, r, clientNTP())
		_ = r.Apply(ctx, first) // restart request (conf written)
		writePid(t, p, startChild(t))
		rr := renderers.NewRecordingRunner().FailWith(ChronycBin, 1, "501 Not authorised")
		r = New(rr, WithPaths(p), WithSecrets(resolver))
		changed := clientNTP()
		changed.Pools = nil
		err := r.Apply(ctx, render(t, r, changed))
		if !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "501") {
			t.Fatalf("want ErrDaemon, got %v", err)
		}
		b, _ := os.ReadFile(p.Sources()) //nolint:gosec // test
		if string(b) != string(first[p.Sources()].Content) {
			t.Fatal("sources not restored")
		}
	})
}

func TestParsers(t *testing.T) {
	src, err := ParseSources([]byte("^,*,127.0.0.1,10,6,377,33,0.000000123,0.000000456,0.000012345\n^,?,192.0.2.1,0,6,0,-,+0.000000000,+0.000000000,0.000000000\n"))
	if err == nil {
		t.Logf("sources: %+v", src)
	}
	src, err = ParseSources([]byte("^,*,127.0.0.1,10,6,377,33,0.000000123,0.000000456,0.000012345\n"))
	if err != nil || len(src) != 1 || src[0].State != "*" || src[0].Stratum != 10 || src[0].Reach != "377" {
		t.Fatalf("%+v %v", src, err)
	}
	tr, err := ParseTracking([]byte("7F000001,127.0.0.1,11,1790198400.123,0.000000001,0.000000002,0.000000003,-1.5,0.001,0.02,0.000100,0.000200,64.0,Normal\n"))
	if err != nil || tr.Stratum != 11 || tr.Leap != "Normal" || tr.RefName != "127.0.0.1" {
		t.Fatalf("%+v %v", tr, err)
	}
	ss, err := ParseSourceStats([]byte("127.0.0.1,5,3,130,0.001,0.050,0.000000010,0.000001000\n"))
	if err != nil || len(ss) != 1 || ss[0].NP != 5 {
		t.Fatalf("%+v %v", ss, err)
	}
	sv, err := ParseServerStats([]byte("12,0,40,0,0,0,0,0,0,0,0,1,2,3,4,5,6,7\n"))
	if err != nil || sv["ntpPacketsReceived"] != "12" || sv["field17"] != "7" {
		t.Fatalf("%v %v", sv, err)
	}
	if _, err := ParseTracking([]byte("x,y\n")); err == nil {
		t.Fatal("short tracking row accepted")
	}
}
