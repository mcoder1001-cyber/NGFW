package frr

import (
	"context"
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func ptr[T any](v T) *T { return &v }

// allStaticsToFRR makes every routing.static entry FRR-owned for the test (proto inputs have
// no D-072 flag yet).
func allStaticsToFRR(t *testing.T) {
	t.Helper()
	selectorMu.Lock()
	prev := staticSelector
	staticSelector = func(int, *vrxv1.StaticRoute, *Extensions) bool { return true }
	selectorMu.Unlock()
	t.Cleanup(func() { selectorMu.Lock(); staticSelector = prev; selectorMu.Unlock() })
}

// testRenderer renders with the product paths, IdentityMapper and no registered protocol sections.
func testRenderer(extra ...Section) *Renderer {
	return New(renderers.NewRecordingRunner(), WithSections(extra...), WithInterfaceMapper(IdentityMapper))
}

func renderConf(t *testing.T, r *Renderer, desired proto.Message) string {
	t.Helper()
	files, err := r.Render(context.Background(), desired)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	f, ok := files[r.Paths().ConfFile()]
	if !ok {
		t.Fatalf("Render: no %s in %v", r.Paths().ConfFile(), files.Paths())
	}
	if f.Mode != 0o640 || f.Owner != "frr:frr" {
		t.Errorf("frr.conf mode/owner = %v/%q, want 0640/frr:frr", f.Mode, f.Owner)
	}
	return string(f.Content)
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// doc builds a *structpb.Struct configuration document (the D-055 stand-in input).
func doc(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func staticDoc(t *testing.T) *structpb.Struct {
	// "frr": true is the D-072 flag (stand-in): only flagged routes are FRR's; the last entry
	// is unflagged and must not be rendered (the agent programs it in VPP).
	return doc(t, map[string]any{
		"routing": map[string]any{"static": []any{
			map[string]any{"frr": true, "prefix": "10.12.200.0/24", "nextHops": []any{map[string]any{"address": "10.12.1.1", "weight": 1}}},
			map[string]any{"frr": true, "prefix": "10.12.201.7/24", "blackhole": true, "distance": 50, "tag": 100},
			map[string]any{"frr": true, "prefix": "10.12.202.0/24", "nextHops": []any{map[string]any{"interface": "w12f0"}}},
			map[string]any{"frr": true, "prefix": "10.12.203.0/24", "nextHops": []any{map[string]any{"address": "10.12.1.1", "interface": "w12f0"}}, "distance": 200},
			map[string]any{"frr": true, "prefix": "10.12.204.0/24", "vrf": "default", "nextHops": []any{
				map[string]any{"address": "10.12.1.3"}, map[string]any{"address": "10.12.1.2"},
			}},
			map[string]any{"frr": true, "prefix": "2001:DB8:12:0:0::/64", "nextHops": []any{map[string]any{"address": "2001:db8:12::1"}}},
			map[string]any{"frr": true, "prefix": "2001:db8:12:1::/64", "blackhole": true, "tag": 7},
			map[string]any{"frr": true, "prefix": "2001:db8:12:2::/64", "nextHops": []any{map[string]any{"address": "fe80::1", "interface": "w12f0"}}, "distance": 20},
			map[string]any{"frr": true, "prefix": "10.12.210.0/24", "vrf": "w12red", "blackhole": true},
			map[string]any{"frr": true, "prefix": "2001:db8:12:10::/64", "vrf": "w12red", "nextHops": []any{map[string]any{"interface": "w12r0"}}, "tag": 9, "distance": 5},
			map[string]any{"prefix": "10.12.250.0/24", "nextHops": []any{map[string]any{"address": "10.12.1.1"}}},
		}},
	})
}

func TestRenderGolden(t *testing.T) {
	cases := []struct {
		name    string
		desired proto.Message
		extra   []Section
	}{
		{name: "empty", desired: nil},
		{name: "empty-state", desired: &vrxv1.DesiredState{}},
		{name: "hostname", desired: &vrxv1.DesiredState{System: &vrxv1.SystemConfig{Hostname: ptr("vrx-a")}}},
		{name: "vrfs", desired: &vrxv1.DesiredState{Vrfs: map[string]*vrxv1.Vrf{
			"default": {Id: ptr(uint32(0))}, "w12red": {Id: ptr(uint32(12001))}, "w12blue": {Id: ptr(uint32(12002)), Description: ptr("ignored by FRR")},
		}}},
		{name: "descriptions", desired: &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{
			"w12f0":                   {Description: ptr("uplink to ISP-1 (10G), cct #4711")},
			"w12f1":                   {Description: ptr("lan")},
			"w12f2":                   {Enabled: ptr(true)}, // no description: no block
			"TenGigabitEthernet0/0/0": {Description: ptr("no Linux side yet: skipped")},
		}}},
		{name: "static", desired: staticDoc(t)},
		// D-072: proto routes carry no FRR flag → programmed by the agent in VPP, not rendered.
		{name: "static-proto", desired: &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("0.0.0.0/0"), NextHops: []*vrxv1.NextHop{{Address: ptr("10.12.1.1"), Weight: ptr(uint32(1))}}, Vrf: ptr("default"), Distance: ptr(uint32(1))},
			{Prefix: ptr("::/0"), NextHops: []*vrxv1.NextHop{{Address: ptr("2001:db8::1")}}, Distance: ptr(uint32(250))},
		}}}},
		{name: "full", desired: func() proto.Message {
			d := staticDoc(t)
			d.Fields["system"] = structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{"hostname": structpb.NewStringValue("vrx-a")}})
			vrfs, _ := structpb.NewStruct(map[string]any{"default": map[string]any{"id": 0}, "w12red": map[string]any{"id": 12001}})
			d.Fields["vrfs"] = structpb.NewStructValue(vrfs)
			ifs, _ := structpb.NewStruct(map[string]any{"w12f0": map[string]any{"description": "uplink"}})
			d.Fields["interfaces"] = structpb.NewStructValue(ifs)
			return d
		}(), extra: []Section{fakeBGP{}}},
		{name: "hostile-description", desired: &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{
			"w12f0": {Description: ptr(`"; rm -rf /`)},
			"w12f1": {Description: ptr(`a ! b # c; exit; end`)},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderConf(t, testRenderer(tc.extra...), tc.desired)
			golden(t, tc.name, got)
			if !strings.HasSuffix(got, "!\nend\n") {
				t.Errorf("config does not end with \"!\\nend\\n\"")
			}
		})
	}
}

func TestRenderVtyshConfAndPaths(t *testing.T) {
	r := New(renderers.NewRecordingRunner(), WithPaths(TestPaths("w12")), WithSections())
	files, err := r.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/run/vrx-test/w12/frr/etc/w12/frr.conf", "/run/vrx-test/w12/frr/etc/w12/vtysh.conf"}
	if got := files.Paths(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	v := files[want[1]]
	if v.Owner != "" || v.Mode != 0o640 {
		t.Errorf("vtysh.conf owner/mode = %q/%v, want \"\"/0640 (tests run as root)", v.Owner, v.Mode)
	}
	golden(t, "vtysh.conf", string(v.Content))
}

func TestRenderDeterministic(t *testing.T) {
	r := testRenderer()
	d := staticDoc(t)
	first := renderConf(t, r, d)
	for range 20 {
		if got := renderConf(t, r, d); got != first {
			t.Fatal("Render is not deterministic")
		}
	}
}

// hostile is the injection table (helpers_template_test.go + the RF-1 task list).
var hostile = []string{
	`"; rm -rf /`,
	"\nrouter bgp 65000\n",
	"x\nrouter bgp 65000",
	"!",
	"#",
	"! comment",
	"# comment",
	"a\r\nb",
	"nul\x00byte",
	"esc\x1b[31m",
	"line\u2028sep",
	"invalid\xffutf8",
	"ünïcödé",
	"tab\there",
	strings.Repeat("d", 300),
	" leading",
	"trailing ",
	"double  blank",
	"a | b",
	"a | include b",
	"uplink|ISP-A",
	"Null0",
	"null0",
	"blackhole",
	"bl",
	"reject",
	"tag",
	"10.12.1.1",
	"2001:db8::1",
	"10.0.0.0/8",
	"end",
	"",
}

func TestHostileStringsRejectedOrEscaped(t *testing.T) {
	allStaticsToFRR(t)
	r := testRenderer()
	render := func(d *vrxv1.DesiredState) (string, error) {
		files, err := r.Render(context.Background(), d)
		if err != nil {
			return "", err
		}
		return string(files[r.Paths().ConfFile()].Content), nil
	}
	fields := map[string]func(s string) *vrxv1.DesiredState{
		"system.hostname": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{System: &vrxv1.SystemConfig{Hostname: ptr(s)}}
		},
		"vrfs key": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Vrfs: map[string]*vrxv1.Vrf{s: {}}}
		},
		"interfaces key (mapped)": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{s: {Description: ptr("x")}}}
		},
		"interfaces.description": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"w12f0": {Description: ptr(s)}}}
		},
		"routing.static.vrf": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
				{Prefix: ptr("10.0.0.0/8"), Vrf: ptr(s), NextHops: []*vrxv1.NextHop{{Address: ptr("10.0.0.1")}}}}}}
		},
		"routing.static.prefix": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
				{Prefix: ptr(s), NextHops: []*vrxv1.NextHop{{Address: ptr("10.0.0.1")}}}}}}
		},
		"routing.static.nextHops.address": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
				{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Address: ptr(s)}}}}}}
		},
		"routing.static.nextHops.interface": func(s string) *vrxv1.DesiredState {
			return &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
				{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Interface: ptr(s)}}}}}}
		},
	}
	// validator decides whether a value may be accepted at all; everything else must fail with ErrInput.
	validator := map[string]func(string) error{
		"system.hostname":         func(s string) error { _, err := Hostname(s); return err },
		"vrfs key":                func(s string) error { _, err := VRFName(s); return err },
		"interfaces key (mapped)": func(s string) error { _, err := IfName(s); return err },
		"interfaces.description":  func(s string) error { _, err := Description(s); return err },
		"routing.static.vrf":      func(s string) error { _, err := VRFName(s); return err },
		"routing.static.prefix":   func(s string) error { _, err := netip.ParsePrefix(s); return err },
		"routing.static.nextHops.address": func(s string) error {
			_, err := gateway(s, netip.MustParsePrefix("10.0.0.0/8"), false)
			return err
		},
		"routing.static.nextHops.interface": func(s string) error { _, err := RouteIfName(s); return err },
	}
	for field, mk := range fields {
		for _, h := range hostile {
			out, err := render(mk(h))
			valid := validator[field](h) == nil
			switch {
			case field == "interfaces key (mapped)" && !valid:
				// IdentityMapper: not a Linux name = no Linux side, skipped and never rendered.
				if err != nil || strings.Contains(out, "interface ") {
					t.Errorf("%s=%q: want skipped, got err=%v\n%s", field, h, err, out)
				}
			case (field == "routing.static.vrf" || field == "system.hostname") && h == "":
				// "" is the default VRF / an unset hostname (not rendered).
				if err != nil {
					t.Errorf("%s=%q: %v", field, h, err)
				}
			case !valid:
				if err == nil {
					t.Errorf("%s=%q accepted:\n%s", field, h, out)
				} else if !errors.Is(err, ErrInput) {
					t.Errorf("%s=%q: error %v does not wrap ErrInput", field, h, err)
				}
			default:
				// Valid tokens (e.g. "end" as a VRF name, `"; rm -rf /` as a description) stay inside their line.
				if err != nil {
					t.Errorf("%s=%q: valid value rejected: %v", field, h, err)
					continue
				}
				lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
				for i, l := range lines {
					if i != len(lines)-1 && (l == "end" || l == h) || strings.Contains(l, "router bgp") {
						t.Errorf("%s=%q escaped its line (line %d %q):\n%s", field, h, i+1, l, out)
					}
				}
			}
		}
	}
}

func TestDescriptionRmRfIsConfinedToOneLine(t *testing.T) {
	// The acceptance criterion: `"; rm -rf /` in a description is rejected or escaped. FRR takes
	// the rest of the line as one LINE token and nothing reaches a shell, so it is written
	// verbatim inside `description` (golden: hostile-description.golden) and stays inert data.
	out := renderConf(t, testRenderer(), &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"w12f0": {Description: ptr(`"; rm -rf /`)}}})
	want := "interface w12f0\n description \"; rm -rf /\nexit\n"
	if !strings.Contains(out, want) {
		t.Fatalf("want block %q in\n%s", want, out)
	}
	for _, bad := range []string{"\n; rm", "\nrm -rf", "\n\"", "\n/"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q escaped its line", bad)
		}
	}
}

func TestModelErrors(t *testing.T) {
	allStaticsToFRR(t)
	cases := map[string]struct {
		d    proto.Message
		want string
	}{
		"family mismatch": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Address: ptr("2001:db8::1")}}}}}}, "address family"},
		"no next hop": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8")}}}}, "nextHops is empty"},
		"blackhole with next hops": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), Blackhole: ptr(true), NextHops: []*vrxv1.NextHop{{Address: ptr("10.0.0.1")}}}}}}, "blackhole route has no next hops"},
		"gateway unspecified": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("0.0.0.0/0"), NextHops: []*vrxv1.NextHop{{Address: ptr("0.0.0.0")}}}}}}, "unspecified, multicast or loopback"},
		"gateway v6 unspecified": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("::/0"), NextHops: []*vrxv1.NextHop{{Address: ptr("::")}}}}}}, "unspecified, multicast or loopback"},
		"gateway multicast": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Address: ptr("224.0.0.5")}}}}}}, "unspecified, multicast or loopback"},
		"gateway loopback": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Address: ptr("127.0.0.1")}}}}}}, "unspecified, multicast or loopback"},
		"link-local without interface": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("2001:db8::/64"), NextHops: []*vrxv1.NextHop{{Address: ptr("fe80::1")}}}}}}, "needs an interface"},
		"empty next hop": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Weight: ptr(uint32(1))}}}}}}, "needs an address"},
		"distance": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), Distance: ptr(uint32(256)), NextHops: []*vrxv1.NextHop{{Address: ptr("10.0.0.1")}}}}}}, "distance 256"},
		"zone": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("fe80::/64"), NextHops: []*vrxv1.NextHop{{Address: ptr("fe80::1%eth0")}}}}}}, "routing.static[0].nextHops[0].address"},
		"unmapped next-hop interface": {&vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Interface: ptr("TenGigabitEthernet0/0/0")}}}}}}, "has no Linux interface"},
		"bad tag": {doc(t, map[string]any{"routing": map[string]any{"static": []any{
			map[string]any{"prefix": "10.0.0.0/8", "tag": -1, "nextHops": []any{map[string]any{"address": "10.0.0.1"}}}}}}), "tag must be an integer"},
		"bad frr flag": {doc(t, map[string]any{"routing": map[string]any{"static": []any{
			map[string]any{"prefix": "10.0.0.0/8", "frr": "yes", "nextHops": []any{map[string]any{"address": "10.0.0.1"}}}}}}), "frr must be a boolean"},
		"wrong type": {&vrxv1.Vrf{}, "unsupported input type"},
		"two names map to one": {&vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{
			"a": {Description: ptr("x")}, "b": {Description: ptr("y")}}}, "map to Linux name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := New(renderers.NewRecordingRunner(), WithSections(), WithInterfaceMapper(func(n string) (string, bool) {
				if n == "a" || n == "b" {
					return "w12f0", true
				}
				return IdentityMapper(n)
			}))
			_, err := r.Render(context.Background(), tc.d)
			if err == nil || !errors.Is(err, ErrInput) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want ErrInput containing %q", err, tc.want)
			}
		})
	}
}

func TestRenderOptionsChecked(t *testing.T) {
	for name, r := range map[string]*Renderer{
		"version":   New(renderers.NewRecordingRunner(), WithVersion("10.7.1\nrouter bgp 1")),
		"paths":     New(renderers.NewRecordingRunner(), WithPaths(Paths{ConfDir: "etc"})),
		"namespace": New(renderers.NewRecordingRunner(), WithPaths(func() Paths { p := TestPaths("w12"); p.Namespace = "../x"; return p }())),
		"mode":      New(renderers.NewRecordingRunner(), WithPaths(func() Paths { p := TestPaths("w12"); p.FileMode = 0o644; return p }())),
		"runner":    New(nil),
	} {
		if _, err := r.Render(context.Background(), nil); err == nil {
			t.Errorf("%s: invalid option accepted", name)
		}
	}
}

func TestInterfaceMapper(t *testing.T) {
	allStaticsToFRR(t)
	r := New(renderers.NewRecordingRunner(), WithSections(), WithInterfaceMapper(func(n string) (string, bool) {
		if n == "TenGigabitEthernet0/0/0" {
			return "vpp1", true
		}
		return "", false
	}))
	out := renderConf(t, r, &vrxv1.DesiredState{
		Interfaces: map[string]*vrxv1.Interface{"TenGigabitEthernet0/0/0": {Description: ptr("wan")}, "loop0": {Description: ptr("unmapped")}},
		Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
			{Prefix: ptr("10.0.0.0/8"), NextHops: []*vrxv1.NextHop{{Interface: ptr("TenGigabitEthernet0/0/0")}}}}},
	})
	if !strings.Contains(out, "interface vpp1\n description wan\nexit\n") || !strings.Contains(out, "ip route 10.0.0.0/8 vpp1\n") || strings.Contains(out, "loop0") {
		t.Fatalf("mapper not applied:\n%s", out)
	}
}
