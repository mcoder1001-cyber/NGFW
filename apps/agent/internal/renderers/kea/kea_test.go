package kea

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

const hook = "/usr/lib/x86_64-linux-gnu/kea/hooks/libdhcp_lease_cmds.so"

func unitPaths() Paths { return TestPaths("w0", 6) }

func newUnit(opts ...Option) *Renderer {
	base := []Option{WithPaths(unitPaths()), WithLeaseCmdsHook(hook)}
	return New(renderers.NewRecordingRunner(), append(base, opts...)...)
}

func v4Server() *vrxv1.DhcpServer {
	return &vrxv1.DhcpServer{
		Enabled:        proto.Bool(true),
		Description:    proto.String("LAN clients"),
		Family:         proto.String("ipv4"),
		Vrf:            proto.String("default"),
		Interfaces:     []string{"w0-a"},
		LeaseTimeSec:   proto.Uint32(7200),
		RenewTimerSec:  proto.Uint32(1800),
		RebindTimerSec: proto.Uint32(3600),
		Authoritative:  proto.Bool(true),
		Options:        []*vrxv1.DhcpOption{{Code: proto.Uint32(66), Data: proto.String("tftp.example.test")}},
		Subnets: map[string]*vrxv1.DhcpSubnet{
			"lan": {
				Description:  proto.String("main LAN"),
				Subnet:       proto.String("10.6.10.0/24"),
				Pools:        []*vrxv1.DhcpPool{{Start: proto.String("10.6.10.100"), End: proto.String("10.6.10.199")}},
				Gateway:      proto.String("10.6.10.1"),
				DnsServers:   []string{"10.6.10.1", "10.6.0.53"},
				NtpServers:   []string{"10.6.10.1"},
				DomainName:   proto.String("lan.example.test"),
				DomainSearch: []string{"lan.example.test", "example.test"},
				Options: []*vrxv1.DhcpOption{
					{Code: proto.Uint32(224), Data: proto.String("vrx private text")},
					{Code: proto.Uint32(225), Data: proto.String("0x0a0b0c"), AlwaysSend: proto.Bool(true)},
				},
				Reservations: map[string]*vrxv1.DhcpReservation{
					"printer": {
						Mac: proto.String("AA-BB-CC-00-11-22"), Ip: proto.String("10.6.10.20"), Hostname: proto.String("Printer1"),
						Options: []*vrxv1.DhcpOption{{Code: proto.Uint32(15), Data: proto.String("print.example.test")}},
					},
				},
			},
			"guest": {
				Subnet:       proto.String("10.6.11.0/24"),
				Pools:        []*vrxv1.DhcpPool{{Start: proto.String("10.6.11.10"), End: proto.String("10.6.11.50")}, {Start: proto.String("10.6.11.60"), End: proto.String("10.6.11.70")}},
				LeaseTimeSec: proto.Uint32(600),
			},
		},
	}
}

func v6Server() *vrxv1.DhcpServer {
	return &vrxv1.DhcpServer{
		Family:     proto.String("ipv6"),
		Interfaces: []string{"w0-a", "w0-b"},
		Subnets: map[string]*vrxv1.DhcpSubnet{
			"lan6": {
				Subnet:       proto.String("fd00:6:10::/64"),
				Pools:        []*vrxv1.DhcpPool{{Start: proto.String("fd00:6:10::1000"), End: proto.String("FD00:6:10::1FFF")}},
				DnsServers:   []string{"fd00:6:10::1"},
				NtpServers:   []string{"fd00:6:10::1"},
				DomainName:   proto.String("lan.example.test"),
				DomainSearch: []string{"example.test"},
				Options:      []*vrxv1.DhcpOption{{Code: proto.Uint32(1000), Data: proto.String("hello v6")}},
				Reservations: map[string]*vrxv1.DhcpReservation{
					"nas": {Duid: proto.String("00:01:00:01:AA:BB:CC:DD:EE:FF"), Ip: proto.String("fd00:6:10::20"), Hostname: proto.String("nas")},
					"cam": {Mac: proto.String("aa:bb:cc:00:11:33"), Ip: proto.String("fd00:6:10::21")},
				},
			},
		},
	}
}

func dhcp(servers map[string]*vrxv1.DhcpServer) *vrxv1.DhcpService {
	return &vrxv1.DhcpService{Servers: servers}
}

func render(t *testing.T, r *Renderer, desired proto.Message) renderers.Files {
	t.Helper()
	files, err := r.Render(context.Background(), desired)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return files
}

// golden compares every file (named by base name) with testdata/<name>.<base>.golden.
func golden(t *testing.T, name string, files renderers.Files) {
	t.Helper()
	for _, p := range files.Paths() {
		gp := filepath.Join("testdata", name+"."+filepath.Base(p)+".golden")
		got := files[p].Content
		if *update {
			if err := os.WriteFile(gp, got, 0o644); err != nil { //nolint:gosec // test data
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(gp) //nolint:gosec // test data
		if err != nil {
			t.Fatalf("read %s (run go test -update): %v", gp, err)
		}
		if string(want) != string(got) {
			t.Errorf("%s differs from %s:\n--- got ---\n%s", p, gp, got)
		}
	}
}

func TestRenderGolden(t *testing.T) {
	ifs := map[string]*vrxv1.Interface{"w0-a": {Ipv4: []string{"10.6.10.1/24"}}}
	cases := map[string]proto.Message{
		"empty":    nil,
		"v4":       dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server()}),
		"v6":       dhcp(map[string]*vrxv1.DhcpServer{"lan6": v6Server()}),
		"both":     &vrxv1.ServicesConfig{Dhcp: dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server(), "lan6": v6Server()})},
		"bindaddr": &vrxv1.DesiredState{Interfaces: ifs, Services: &vrxv1.ServicesConfig{Dhcp: dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server()})}},
		"disabled": dhcp(map[string]*vrxv1.DhcpServer{"off": {Enabled: proto.Bool(false), Interfaces: []string{"ens192"}}}),
		"hostile-description": dhcp(map[string]*vrxv1.DhcpServer{"lan": func() *vrxv1.DhcpServer {
			s := v4Server()
			s.Description = proto.String(`"; rm -rf /`)
			s.Subnets["lan"].Description = proto.String(`"}]} , "Dhcp4": {"hooks-libraries": [{"library": "/tmp/evil.so"}]} ☃ \`)
			return s
		}()}),
	}
	for name, desired := range cases {
		t.Run(name, func(t *testing.T) {
			files := render(t, newUnit(), desired)
			if len(files) != 3 {
				t.Fatalf("want 3 files, got %v", files.Paths())
			}
			for p, f := range files {
				if f.Mode != 0o640 || !json.Valid(f.Content) {
					t.Errorf("%s: mode %v valid JSON %v", p, f.Mode, json.Valid(f.Content))
				}
			}
			golden(t, name, files)
			again := render(t, newUnit(), desired)
			for p := range files {
				if string(files[p].Content) != string(again[p].Content) {
					t.Errorf("%s: render is not deterministic", p)
				}
			}
		})
	}
}

// TestHostileDescriptionEscaped: the JSON break-out attempt stays one string value; the
// parsed config has exactly the rendered structure (one hook, the lease_cmds one).
func TestHostileDescriptionEscaped(t *testing.T) {
	s := v4Server()
	evil := `"}]} , "Dhcp4": {"hooks-libraries": [{"library": "/tmp/evil.so"}]}`
	s.Description = proto.String(`"; rm -rf /`)
	s.Subnets["lan"].Description = proto.String(evil)
	files := render(t, newUnit(), dhcp(map[string]*vrxv1.DhcpServer{"lan": s}))
	var root struct {
		Dhcp4 struct {
			Hooks   []hookLibrary `json:"hooks-libraries"`
			Subnet4 []subnet      `json:"subnet4"`
		}
	}
	if err := json.Unmarshal(files[unitPaths().Dhcp4Conf()].Content, &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Dhcp4.Hooks) != 1 || root.Dhcp4.Hooks[0].Library != hook {
		t.Fatalf("hooks changed by user data: %+v", root.Dhcp4.Hooks)
	}
	found := map[string]bool{}
	for _, sub := range root.Dhcp4.Subnet4 {
		found[sub.UserContext.VRX.Description] = true
		found[sub.UserContext.VRX.ServerDescription] = true
	}
	if !found[evil] || !found[`"; rm -rf /`] {
		t.Fatalf("descriptions not preserved verbatim as strings: %v", found)
	}
}

var hostile = []string{
	"\"; rm -rf /\ninclude: /etc/passwd",
	"a\nb", "a\r\nb", "a\x00b", "a\x1bb", "a\u2028b", "\xff\xfe",
}

func TestRejects(t *testing.T) {
	type mut func(s *vrxv1.DhcpServer)
	cases := map[string]mut{
		"desc-5k":         func(s *vrxv1.DhcpServer) { s.Description = proto.String(strings.Repeat("x", 5*1024)) },
		"iface-hostile":   func(s *vrxv1.DhcpServer) { s.Interfaces = []string{`w0-a"; rm -rf /`} },
		"iface-host-nic":  func(s *vrxv1.DhcpServer) { s.Interfaces = []string{"ens192"} },
		"iface-none":      func(s *vrxv1.DhcpServer) { s.Interfaces = nil },
		"vrf-hostile":     func(s *vrxv1.DhcpServer) { s.Vrf = proto.String("a b") },
		"family":          func(s *vrxv1.DhcpServer) { s.Family = proto.String("ipx") },
		"renew":           func(s *vrxv1.DhcpServer) { s.RenewTimerSec = proto.Uint32(9000) },
		"opt-code-0":      func(s *vrxv1.DhcpServer) { s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(0), Data: proto.String("x")}} },
		"opt-code-v4-300": func(s *vrxv1.DhcpServer) { s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(300), Data: proto.String("x")}} },
		"opt-dup": func(s *vrxv1.DhcpServer) {
			s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(66), Data: proto.String("a")}, {Code: proto.Uint32(66), Data: proto.String("b")}}
		},
		"opt-unicode": func(s *vrxv1.DhcpServer) { s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(66), Data: proto.String("☃")}} },
		"opt-5k":      func(s *vrxv1.DhcpServer) { s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(66), Data: proto.String(strings.Repeat("a", 5*1024))}} },
		"opt-typed-dup": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(3), Data: proto.String("10.6.10.2")}}
		},
		"subnet-name":   func(s *vrxv1.DhcpServer) { s.Subnets["bad name"] = s.Subnets["lan"] },
		"subnet-family": func(s *vrxv1.DhcpServer) { s.Subnets["lan"].Subnet = proto.String("fd00::/64") },
		"pool-outside": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Pools = []*vrxv1.DhcpPool{{Start: proto.String("10.6.10.1"), End: proto.String("10.6.12.1")}}
		},
		"pool-reversed": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Pools = []*vrxv1.DhcpPool{{Start: proto.String("10.6.10.9"), End: proto.String("10.6.10.1")}}
		},
		"pool-hostile": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Pools = []*vrxv1.DhcpPool{{Start: proto.String(`10.6.10.1"; rm -rf /`), End: proto.String("10.6.10.9")}}
		},
		"gateway-hostile": func(s *vrxv1.DhcpServer) { s.Subnets["lan"].Gateway = proto.String("10.6.10.1\ninclude") },
		"dns-hostile":     func(s *vrxv1.DhcpServer) { s.Subnets["lan"].DnsServers = []string{"1.1.1.1, 8.8.8.8"} },
		"domain-hostile":  func(s *vrxv1.DhcpServer) { s.Subnets["lan"].DomainName = proto.String(`x"; rm -rf /`) },
		"search-unicode":  func(s *vrxv1.DhcpServer) { s.Subnets["lan"].DomainSearch = []string{"bücher.example"} },
		"mac-hostile": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Reservations["printer"].Mac = proto.String(`aa:bb:cc:dd:ee:ff"}`)
		},
		"duid-on-v4": func(s *vrxv1.DhcpServer) {
			r := s.Subnets["lan"].Reservations["printer"]
			r.Mac, r.Duid = nil, proto.String("00:01")
		},
		"mac-and-duid": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Reservations["printer"].Duid = proto.String("00:01")
		},
		"res-outside": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Reservations["printer"].Ip = proto.String("10.6.99.1")
		},
		"res-hostname": func(s *vrxv1.DhcpServer) {
			s.Subnets["lan"].Reservations["printer"].Hostname = proto.String("a\nb")
		},
	}
	for i, h := range hostile {
		h := h
		cases["desc-hostile-"+string(rune('a'+i))] = func(s *vrxv1.DhcpServer) { s.Subnets["lan"].Description = proto.String(h) }
		cases["optdata-hostile-"+string(rune('a'+i))] = func(s *vrxv1.DhcpServer) {
			s.Options = []*vrxv1.DhcpOption{{Code: proto.Uint32(66), Data: proto.String(h)}}
		}
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			s := v4Server()
			m(s)
			_, err := newUnit().Render(context.Background(), dhcp(map[string]*vrxv1.DhcpServer{"lan": s}))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
	t.Run("vrf-mismatch", func(t *testing.T) {
		a, b := v4Server(), v4Server()
		b.Vrf = proto.String("blue")
		b.Subnets = nil
		_, err := newUnit().Render(context.Background(), dhcp(map[string]*vrxv1.DhcpServer{"a": a, "b": b}))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid, got %v", err)
		}
	})
	t.Run("wrong-type", func(t *testing.T) {
		_, err := newUnit().Render(context.Background(), &vrxv1.NtpService{})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid, got %v", err)
		}
	})
}

func TestSubnetIDsStable(t *testing.T) {
	one := render(t, newUnit(), dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server()}))
	s := v4Server()
	s.Subnets["another"] = &vrxv1.DhcpSubnet{Subnet: proto.String("10.6.12.0/24"),
		Pools: []*vrxv1.DhcpPool{{Start: proto.String("10.6.12.10"), End: proto.String("10.6.12.20")}}}
	two := render(t, newUnit(), dhcp(map[string]*vrxv1.DhcpServer{"lan": s}))
	ids := func(f renderers.Files) map[string]uint32 {
		var root dhcp4Root
		if err := json.Unmarshal(f[unitPaths().Dhcp4Conf()].Content, &root); err != nil {
			t.Fatal(err)
		}
		out := map[string]uint32{}
		for _, sub := range *root.Dhcp4.Subnet4 {
			out[sub.UserContext.VRX.Subnet] = sub.ID
		}
		return out
	}
	a, b := ids(one), ids(two)
	for k, v := range a {
		if b[k] != v {
			t.Errorf("subnet %s id changed %d -> %d when another subnet was added", k, v, b[k])
		}
	}
}

func TestInterfaceMapperAndPrefix(t *testing.T) {
	s := v4Server()
	s.Interfaces = []string{"GigabitEthernet0/8/0"}
	d := dhcp(map[string]*vrxv1.DhcpServer{"lan": s})
	if _, err := newUnit().Render(context.Background(), d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("VPP name without mapper: want ErrInvalid, got %v", err)
	}
	m := WithInterfaceMapper(func(string) (string, error) { return "w0-a", nil })
	files := render(t, newUnit(m), d)
	if !strings.Contains(string(files[unitPaths().Dhcp4Conf()].Content), `"w0-a"`) {
		t.Fatal("mapped name not rendered")
	}
}

func TestValidateArgv(t *testing.T) {
	rr := renderers.NewRecordingRunner().Succeed(Dhcp4Bin, "").Succeed(Dhcp6Bin, "").Succeed(CtrlAgentBin, "")
	r := New(rr, WithPaths(unitPaths()), WithLeaseCmdsHook(""))
	files := render(t, r, dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server()}))
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	calls := rr.Calls()
	if len(calls) != 3 {
		t.Fatalf("want 3 checker calls, got %v", calls)
	}
	for _, c := range calls {
		if len(c.Args) != 2 || c.Args[0] != "-t" || !strings.HasPrefix(c.Args[1], os.TempDir()) {
			t.Errorf("unexpected argv %v", c)
		}
	}
	want := []string{CtrlAgentBin, Dhcp4Bin, Dhcp6Bin} // Paths() order: kea-ctrl-agent, kea-dhcp4, kea-dhcp6
	for i, c := range calls {
		if c.Path != want[i] {
			t.Errorf("call %d: %s, want %s", i, c.Path, want[i])
		}
	}

	rr = renderers.NewRecordingRunner().FailWith(CtrlAgentBin, 1, "ERROR bad config").Succeed(Dhcp4Bin, "").Succeed(Dhcp6Bin, "")
	r = New(rr, WithPaths(unitPaths()), WithLeaseCmdsHook(""))
	if err := r.Validate(context.Background(), files); !errors.Is(err, ErrDaemon) {
		t.Fatalf("want ErrDaemon, got %v", err)
	}
	foreign := renderers.Files{"/etc/passwd": {Mode: 0o644, Content: []byte("{}")}}
	if err := r.Validate(context.Background(), foreign); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Fatalf("foreign file: want ErrInvalidFiles, got %v", err)
	}
}

// fakeCtrl records commands and answers per family.
type fakeCtrl struct {
	calls   []string
	running map[int]bool
	failSet map[int]int // family → fail the n-th config-set (1-based)
	sets    map[int]int
	last    map[int]string
}

func (f *fakeCtrl) Command(_ context.Context, fam int, cmd string, args any) (Response, error) {
	f.calls = append(f.calls, cmd+"/"+string(rune('0'+fam)))
	if !f.running[fam] {
		return Response{}, ErrNotRunning
	}
	if cmd == "config-set" {
		f.sets[fam]++
		if f.failSet[fam] == f.sets[fam] {
			return Response{Result: 1, Text: "bad"}, ErrCommand
		}
		raw, _ := args.(json.RawMessage)
		f.last[fam] = string(raw)
	}
	return Response{Result: 0}, nil
}

func newFake(running ...int) *fakeCtrl {
	f := &fakeCtrl{running: map[int]bool{}, failSet: map[int]int{}, sets: map[int]int{}, last: map[int]string{}}
	for _, r := range running {
		f.running[r] = true
	}
	return f
}

func tmpPaths(t *testing.T) Paths {
	p := unitPaths()
	dir := t.TempDir()
	p.ConfDir = dir
	return p
}

func TestApply(t *testing.T) {
	ctx := context.Background()
	d := dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server()})

	t.Run("config-set on running servers", func(t *testing.T) {
		p, fc := tmpPaths(t), newFake(4, 6)
		r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(fc), WithCtrlAgent(nil), WithLeaseCmdsHook(""))
		files := render(t, r, d)
		if err := r.Apply(ctx, files); err != nil {
			t.Fatal(err)
		}
		if strings.Join(fc.calls, ",") != "config-set/4,config-set/6" {
			t.Fatalf("calls %v", fc.calls)
		}
		if fc.last[4] != string(files[p.Dhcp4Conf()].Content) {
			t.Fatal("config-set did not carry the rendered config")
		}
		for path, f := range files {
			b, err := os.ReadFile(path) //nolint:gosec // test
			if err != nil || string(b) != string(f.Content) {
				t.Fatalf("%s not written: %v", path, err)
			}
		}
	})

	t.Run("idle family not running is skipped and active one needs start", func(t *testing.T) {
		p, fc := tmpPaths(t), newFake(6)
		r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(fc), WithCtrlAgent(nil), WithLeaseCmdsHook(""))
		err := r.Apply(ctx, render(t, r, d))
		var ar *ActionRequired
		if !errors.As(err, &ar) || ar.Daemon != "kea-dhcp4" || ar.Unit != "kea-dhcp4-server" || ar.Action != "start" {
			t.Fatalf("want ActionRequired for kea-dhcp4, got %v", err)
		}
		if _, err := os.Stat(p.Dhcp4Conf()); err != nil {
			t.Fatal("files must stay written on ActionRequired")
		}
		fc = newFake()
		r = New(renderers.NewRecordingRunner(), WithPaths(p), WithController(fc), WithCtrlAgent(nil), WithLeaseCmdsHook(""))
		if err := r.Apply(ctx, render(t, r, nil)); err != nil {
			t.Fatalf("idle config, nothing running: %v", err)
		}
	})

	t.Run("config-set failure restores files and previous config", func(t *testing.T) {
		p := tmpPaths(t)
		fc := newFake(4, 6)
		r := New(renderers.NewRecordingRunner(), WithPaths(p), WithController(fc), WithCtrlAgent(nil), WithLeaseCmdsHook(""))
		old := render(t, r, nil)
		if err := r.Apply(ctx, old); err != nil {
			t.Fatal(err)
		}
		fc.calls = nil
		fc.failSet[6] = 2 // the second config-set on dhcp6 (the new config) fails
		err := r.Apply(ctx, render(t, r, dhcp(map[string]*vrxv1.DhcpServer{"lan": v4Server(), "lan6": v6Server()})))
		if !errors.Is(err, ErrDaemon) {
			t.Fatalf("want ErrDaemon, got %v", err)
		}
		if strings.Join(fc.calls, ",") != "config-set/4,config-set/6,config-set/4,config-set/6" {
			t.Fatalf("calls %v", fc.calls)
		}
		for path, f := range old {
			b, _ := os.ReadFile(path) //nolint:gosec // test
			if string(b) != string(f.Content) {
				t.Fatalf("%s not restored", path)
			}
		}
		if fc.last[4] != string(old[p.Dhcp4Conf()].Content) {
			t.Fatal("dhcp4 not set back to the previous config")
		}
	})
}

func TestConfigDrift(t *testing.T) {
	rendered := []byte(`{"Dhcp4":{"a":1,"l":[{"x":"y"}],"s":"t"}}`)
	running := []byte(`{"Dhcp4":{"a":1,"extra":true,"l":[{"x":"y","z":2}],"s":"t"}}`)
	if d, err := ConfigDrift(rendered, running); err != nil || len(d) != 0 {
		t.Fatalf("want no drift, got %v %v", d, err)
	}
	running = []byte(`{"Dhcp4":{"a":2,"l":[],"s":"t"}}`)
	d, err := ConfigDrift(rendered, running)
	if err != nil || len(d) != 2 {
		t.Fatalf("want 2 drifts, got %v %v", d, err)
	}
}

func TestFlattenStatsAndEvents(t *testing.T) {
	m, err := flattenStats(json.RawMessage(`{"pkt4-received":[[5,"2026-01-01"],[4,"x"]],"subnet[1].assigned-addresses":[[0,"x"]],"x":[]}`))
	if err != nil || m["pkt4-received"] != "5" || m["subnet[1].assigned-addresses"] != "0" {
		t.Fatalf("%v %v", m, err)
	}
	ev := diff("kea-dhcp4", map[string]string{"a": "1", "gone": "x"}, map[string]string{"a": "2", "new": "y"})
	if len(ev) != 3 || ev[0].Key != "a" || ev[1].Key != "new" || ev[2].Key != "gone" || ev[2].New != "" {
		t.Fatalf("%v", ev)
	}
	if p := ev[0].ToProto(); p.GetAttributes()["source"] != "kea" {
		t.Fatal(p)
	}
}
