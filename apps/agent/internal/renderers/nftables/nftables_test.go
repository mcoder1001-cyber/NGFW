package nftables

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// hostile is the renderers package's hostile-string table plus nft-specific injections (quote, brace,
// statement separator, comment).
var hostile = []string{
	`"; rm -rf /`,
	"desc\nno service password-encryption",
	"desc\r\nline",
	"nul\x00byte",
	"tab\tinside",
	"esc\x1b[31m",
	"para sep",
	"\xff\xfeinvalid utf8",
	`eth0" accept`,
	"x } ; flush ruleset",
	"a # comment",
	"} table ip filter { chain input { drop",
}

func doc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("document: %v", err)
	}
	return ds
}

func fixture(t *testing.T) *vrxv1.DesiredState {
	t.Helper()
	raw, err := os.ReadFile("../../../../../packages/proto/test/fixtures/host-acl-nftables-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	return doc(t, string(raw))
}

const fullDoc = `{
 "objects": {
  "addresses": {
   "noc-v4": {"type": "host", "address": "192.0.2.10"},
   "noc-v6": {"type": "network", "prefix": "2001:db8:10::/48"},
   "backup": {"type": "range", "start": "198.51.100.10", "end": "198.51.100.20"},
   "portal": {"type": "fqdn", "fqdn": "portal.example.net"},
   "ghost": {"type": "fqdn", "fqdn": "ghost.example.net"}
  },
  "addressGroups": {"noc": {"members": ["noc-v4", "noc-v6"]}},
  "services": {
   "dns": {"protocol": "tcp-udp", "destinationPorts": ["53"]},
   "web-alt": {"protocol": "tcp", "destinationPorts": ["8000-8080", "8443"], "sourcePorts": ["1024-65535"]},
   "syn-only": {"protocol": "tcp", "destinationPorts": ["179"], "tcpFlags": {"mask": 18, "value": 2}},
   "echo": {"protocol": "icmp", "type": 8, "code": 0},
   "echo6": {"protocol": "icmp6", "type": 128},
   "sctp-sig": {"protocol": "sctp", "destinationPorts": ["2905"]},
   "gre": {"protocol": "other", "number": 47}
  },
  "serviceGroups": {"mixed": {"members": ["dns", "echo", "echo6", "sctp-sig", "gre"]}}
 },
 "acl": {
  "host": {
   "local-in": {"description": "local-in \"; flush ruleset", "rules": [
    {"sequence": 5, "action": "accept", "source": {"kind": "object", "name": "noc"}, "service": {"kind": "object", "name": "mixed"}, "log": true},
    {"sequence": 10, "action": "accept", "source": {"kind": "object", "name": "backup"}, "service": {"kind": "object", "name": "web-alt"}, "interface": "ens224"},
    {"sequence": 15, "action": "accept", "source": {"kind": "object", "name": "portal"}, "service": {"kind": "object", "name": "syn-only"}},
    {"sequence": 16, "action": "accept", "source": {"kind": "object", "name": "ghost"}},
    {"sequence": 20, "action": "reject", "ipVersion": "ipv6", "service": {"kind": "inline", "spec": {"protocol": "udp", "destinationPorts": ["161-162"]}}},
    {"sequence": 30, "action": "accept", "ipVersion": "ipv4", "destination": {"kind": "prefix", "prefix": "10.9.77.1/32"}, "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["22"]}}},
    {"sequence": 999, "enabled": false, "action": "accept"},
    {"sequence": 1000, "action": "drop", "log": true, "description": "x\n}"}
   ]},
   "egress": {"rules": [{"sequence": 10, "action": "drop", "destination": {"kind": "prefix", "prefix": "203.0.113.7/24"}}]},
   "transit": {"rules": [{"sequence": 10, "action": "drop", "interface": "ens256"}]}
  },
  "hostAttachments": [
   {"list": "local-in", "chain": "input", "priority": 10},
   {"list": "egress", "chain": "output", "priority": 0},
   {"list": "transit", "chain": "forward", "priority": -100},
   {"list": "egress", "chain": "input", "priority": -10, "enabled": false}
  ],
  "hostSettings": {"defaultInput": "drop", "allowIcmp": false, "antiLockout": {"enabled": true, "sources": ["10.9.0.0/16", "2001:db8:10::/48"], "interfaces": ["ens192", "ens224"], "ports": [22, 443, 8443]}}
 }
}`

func fqdn(name string) ([]netip.Addr, bool) {
	if name == "portal" {
		return []netip.Addr{netip.MustParseAddr("203.0.113.80"), netip.MustParseAddr("2001:db8::80")}, true
	}
	return nil, false
}

func build(t *testing.T, ds *vrxv1.DesiredState) (*HostTable, []Issue) {
	t.Helper()
	return Build(Input{Acl: ds.GetAcl(), Objects: ds.GetObjects(), FQDN: fqdn})
}

func errorsOf(issues []Issue) []Issue {
	var out []Issue
	for _, is := range issues {
		if !is.Warning {
			out = append(out, is)
		}
	}
	return out
}

// golden compares got with testdata/<name>.golden (-update rewrites it) and checks the file with `nft -c`
// when nft is installed and the test runs as root (a check never changes the ruleset; allowed in the
// root netns of the shared host).
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // testdata
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
	nftCheck(t, path)
}

func nftCheck(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(NftBin); err != nil || os.Geteuid() != 0 {
		t.Logf("nft -c skipped (nft missing or not root)")
		return
	}
	abs, _ := filepath.Abs(path)
	if _, err := renderers.NewSystemRunner(Binaries()).Run(context.Background(), renderers.Command{Path: NftBin, Args: []string{"-c", "-f", abs}}); err != nil {
		t.Errorf("nft -c -f %s: %v", path, err)
	}
}

func TestGoldenBasic(t *testing.T) {
	v, issues := build(t, fixture(t))
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	text, err := RenderText("vrx_w9", v)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "basic", text)
	prod, err := RenderText(ProductTable, v)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "basic-product", prod)
}

func TestGoldenFull(t *testing.T) {
	v, issues := build(t, doc(t, fullDoc))
	if errs := errorsOf(issues); len(errs) != 0 {
		t.Fatalf("errors: %+v", errs)
	}
	var got []string
	for _, is := range issues {
		got = append(got, is.Rule+" "+is.Pointer)
	}
	want := []string{
		RuleFQDN + " /acl/host/local-in/rules/3/source/name",
		RuleEmpty + " /acl/host/local-in/rules/3",
		RuleLockoutShadow + " /acl/host/local-in/rules/7",
	}
	if !slices.Equal(got, want) {
		t.Errorf("warnings:\n got %q\nwant %q", got, want)
	}
	text, err := RenderText("vrx_w9", v)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "full", text)
	if bytes.Contains(text, []byte("flush")) || bytes.Contains(text, []byte("description")) {
		t.Error("descriptions must never reach the rendering")
	}
}

func TestGoldenEmptyAndRemoved(t *testing.T) {
	v, issues := build(t, doc(t, `{"acl": {"hostSettings": {}}}`))
	if len(issues) != 0 || v == nil || len(v.GetChains()) != 0 {
		t.Fatalf("settings only: value %v issues %+v", v, issues)
	}
	text, err := RenderText("vrx_w9", v)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "empty", text)
	removed, err := RenderText("vrx_w9", nil)
	if err != nil || !bytes.Equal(removed, text) {
		t.Errorf("Render(nil) must equal the empty rendering: %v\n%s", err, removed)
	}
	if v, issues := build(t, doc(t, `{"acl": {}}`)); v != nil || issues != nil {
		t.Errorf("no host configuration → no value, got %v %v", v, issues)
	}
}

func TestBuildIsDeterministicAndCarriesConfig(t *testing.T) {
	ds := doc(t, fullDoc)
	a, _ := build(t, ds)
	b, _ := build(t, doc(t, fullDoc))
	if !proto.Equal(a, b) {
		t.Fatal("Build is not deterministic")
	}
	want := &vrxv1.AclConfig{Host: ds.GetAcl().GetHost(), HostAttachments: ds.GetAcl().GetHostAttachments(), HostSettings: ds.GetAcl().GetHostSettings()}
	if !proto.Equal(a.GetConfig(), want) {
		t.Error("the value's config must be acl.host* of the document")
	}
	// A description change changes only the config, never the rendering.
	ds.GetAcl().GetHost()["local-in"].Description = proto.String("changed")
	c, _ := build(t, ds)
	ta, _ := RenderText("vrx", a)
	tc, _ := RenderText("vrx", c)
	if !bytes.Equal(ta, tc) || proto.Equal(a, c) {
		t.Error("description: same rendering, different value")
	}
}

func TestHostileInput(t *testing.T) {
	base := func() *vrxv1.DesiredState {
		return doc(t, `{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "interface": "eth0"}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`)
	}
	clean, cleanIssues := build(t, base())
	cleanText, _ := RenderText("vrx", clean)
	for _, h := range hostile {
		// descriptions: never rendered
		ds := base()
		ds.GetAcl().GetHost()["l"].Description = proto.String(h)
		ds.GetAcl().GetHost()["l"].GetRules()[0].Description = proto.String(h)
		ds.GetAcl().GetHostAttachments()[0].Description = proto.String(h)
		v, issues := build(t, ds)
		text, err := RenderText("vrx", v)
		if !slices.Equal(issues, cleanIssues) || err != nil || !bytes.Equal(text, cleanText) {
			t.Errorf("description %q changed the rendering (%v, %+v)", h, err, issues)
		}
		// interface names: refused at the rule's pointer
		ds = base()
		ds.GetAcl().GetHost()["l"].GetRules()[0].Interface = proto.String(h)
		expectRefused(t, ds, "/acl/host/l/rules/0/interface", h)
		// list names: refused
		ds = base()
		ds.GetAcl().GetHost()[h] = ds.GetAcl().GetHost()["l"]
		ds.GetAcl().GetHostAttachments()[0].List = proto.String(h)
		expectRefused(t, ds, ptr("acl", "host", h), h)
		// object names and anti-lockout interfaces: refused
		ds = base()
		ds.GetAcl().GetHost()["l"].GetRules()[0].Source = &vrxv1.AddressMatch{Kind: proto.String("object"), Name: proto.String(h)}
		expectRefused(t, ds, "/acl/host/l/rules/0/source/name", h)
		ds = base()
		ds.GetAcl().HostSettings = &vrxv1.HostAclSettings{AntiLockout: &vrxv1.HostAclAntiLockout{Interfaces: []string{h}}}
		expectRefused(t, ds, "/acl/hostSettings/antiLockout/interfaces/0", h)
		ds = base()
		ds.GetAcl().GetHost()["l"].GetRules()[0].Source = &vrxv1.AddressMatch{Kind: proto.String("prefix"), Prefix: proto.String("10.0.0.0/8 " + h)}
		expectRefused(t, ds, "/acl/host/l/rules/0/source/prefix", h)
	}
}

func expectRefused(t *testing.T, ds *vrxv1.DesiredState, pointer, h string) {
	t.Helper()
	v, issues := build(t, ds)
	errs := errorsOf(issues)
	if len(errs) == 0 || errs[0].Pointer != pointer {
		t.Errorf("%q: want an error at %s, got %+v", h, pointer, issues)
	}
	if text, err := RenderText("vrx", v); err != nil || bytes.Contains(text, []byte(h)) {
		t.Errorf("%q reached the rendering (%v)", h, err)
	}
}

// TestRenderRefusesHostileValues: the value may come from the store file, so RenderText checks every
// token again.
func TestRenderRefusesHostileValues(t *testing.T) {
	ok := func() *HostTable {
		v, _ := build(t, fixture(t))
		return v
	}
	cases := map[string]func(v *HostTable){
		"set name":        func(v *HostTable) { v.Sets[0].Name = "a4_x { }" },
		"set type":        func(v *HostTable) { v.Sets[0].Type = "ipv4_addr; flush ruleset" },
		"set element":     func(v *HostTable) { v.Sets[0].Elements = []string{"10.0.0.0/24 }"} },
		"set element 6":   func(v *HostTable) { v.Sets[0].Elements = []string{"2001:db8::/32"} },
		"non-canonical":   func(v *HostTable) { v.Sets[0].Elements = []string{"10.0.0.1/24"} },
		"chain name":      func(v *HostTable) { v.Chains[0].Name = "in_x;" },
		"chain hook":      func(v *HostTable) { v.Chains[0].Hook = "output" },
		"chain policy":    func(v *HostTable) { v.Chains[0].Policy = "accept; flush ruleset" },
		"chain priority":  func(v *HostTable) { v.Chains[0].Priority = 501 },
		"rule comment":    func(v *HostTable) { v.Chains[0].Rules[0].Comment = `vrx:@x/0:00000000" accept` },
		"rule newline":    func(v *HostTable) { v.Chains[0].Rules[0].Text = "counter accept\nflush ruleset" },
		"rule semicolon":  func(v *HostTable) { v.Chains[0].Rules[0].Text = "counter accept; flush ruleset; counter accept" },
		"rule hash":       func(v *HostTable) { v.Chains[0].Rules[0].Text = "counter accept # x" },
		"rule quote":      func(v *HostTable) { v.Chains[0].Rules[0].Text = `iifname "a;b" counter accept` },
		"rule brace":      func(v *HostTable) { v.Chains[0].Rules[0].Text = "counter accept }" },
		"rule no verdict": func(v *HostTable) { v.Chains[0].Rules[0].Text = "counter" },
		"rule escape":     func(v *HostTable) { v.Chains[0].Rules[0].Text = `iifname "a\"" counter accept` },
	}
	for name, mutate := range cases {
		v := ok()
		mutate(v)
		if _, err := RenderText("vrx", v); !errors.Is(err, renderers.ErrUnsafe) {
			t.Errorf("%s: accepted (%v)", name, err)
		}
	}
	if _, err := RenderText("vrx; flush ruleset", ok()); !errors.Is(err, renderers.ErrUnsafe) {
		t.Errorf("table name accepted: %v", err)
	}
}

func TestAntiLockout(t *testing.T) {
	const pre = `{"acl": {"hostAttachments": [{"list": "l", "chain": "input"}], "host": {"l": {"rules": [`
	settings := func(enabled bool, extra string) string {
		e := "false"
		if enabled {
			e = "true"
		}
		return `]}}, "hostSettings": {` + extra + `"antiLockout": {"enabled": ` + e + `, "sources": ["10.0.0.0/24"], "interfaces": ["ens192"]}}}}`
	}
	const ssh = `"service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["22"]}}`
	const both = `"service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["22", "443"]}}`
	cases := []struct {
		name, rules string
		enabled     bool
		extra       string
		wantRule    string // "" = none
		wantPointer string
	}{
		{"drop ssh, lockout off", `{"sequence": 1, "action": "drop", ` + ssh + `}`, false, "", RuleAntiLockout, "/acl/host/l/rules/0"},
		{"drop ssh, lockout on", `{"sequence": 1, "action": "drop", ` + ssh + `}`, true, "", RuleLockoutShadow, "/acl/host/l/rules/0"},
		{"reject all from the mgmt source", `{"sequence": 1, "action": "reject", "source": {"kind": "prefix", "prefix": "10.0.0.128/25"}}`, false, "", RuleAntiLockout, "/acl/host/l/rules/0"},
		{"accept mgmt then drop all", `{"sequence": 1, "action": "accept", "source": {"kind": "prefix", "prefix": "10.0.0.0/16"}, ` + both + `}, {"sequence": 2, "action": "drop"}`, false, "", "", ""},
		{"accept ssh only then drop all", `{"sequence": 1, "action": "accept", "source": {"kind": "prefix", "prefix": "10.0.0.0/16"}, ` + ssh + `}, {"sequence": 2, "action": "drop"}`, false, "", RuleAntiLockout, "/acl/host/l/rules/1"},
		{"partial accept then drop all", `{"sequence": 1, "action": "accept", "source": {"kind": "prefix", "prefix": "10.0.0.0/25"}, ` + both + `}, {"sequence": 2, "action": "drop"}`, false, "", RuleAntiLockout, "/acl/host/l/rules/1"},
		{"accept on another interface then drop", `{"sequence": 1, "action": "accept", "interface": "ens224", ` + both + `}, {"sequence": 2, "action": "drop"}`, false, "", RuleAntiLockout, "/acl/host/l/rules/1"},
		{"drop on another interface", `{"sequence": 1, "action": "drop", "interface": "ens224"}`, false, "", "", ""},
		{"drop udp 22", `{"sequence": 1, "action": "drop", "service": {"kind": "inline", "spec": {"protocol": "udp", "destinationPorts": ["22"]}}}`, false, "", "", ""},
		{"drop ipv6 only", `{"sequence": 1, "action": "drop", "ipVersion": "ipv6"}`, false, "", "", ""},
		{"default drop, nothing accepts", `{"sequence": 1, "action": "accept", "service": {"kind": "inline", "spec": {"protocol": "udp", "destinationPorts": ["53"]}}}`, false, `"defaultInput": "drop", `, RuleAntiLockout, "/acl/hostSettings/defaultInput"},
		{"default drop, lockout on", `{"sequence": 1, "action": "accept", "service": {"kind": "inline", "spec": {"protocol": "udp", "destinationPorts": ["53"]}}}`, true, `"defaultInput": "drop", `, RuleLockoutShadow, "/acl/hostSettings/defaultInput"},
		{"default drop, rules accept", `{"sequence": 1, "action": "accept", "source": {"kind": "prefix", "prefix": "10.0.0.0/24"}, ` + both + `}`, false, `"defaultInput": "drop", `, "", ""},
		{"disabled drop rule", `{"sequence": 1, "enabled": false, "action": "drop"}`, false, "", "", ""},
	}
	for _, c := range cases {
		v, issues := build(t, doc(t, pre+c.rules+settings(c.enabled, c.extra)))
		var got []Issue
		for _, is := range issues {
			if is.Rule == RuleAntiLockout || is.Rule == RuleLockoutShadow {
				got = append(got, is)
			}
		}
		switch {
		case c.wantRule == "" && len(got) != 0:
			t.Errorf("%s: unexpected %+v", c.name, got)
		case c.wantRule != "" && (len(got) != 1 || got[0].Rule != c.wantRule || got[0].Pointer != c.wantPointer):
			t.Errorf("%s: want %s at %s, got %+v", c.name, c.wantRule, c.wantPointer, got)
		case c.wantRule == RuleAntiLockout && !strings.Contains(got[0].Message, "management TCP"):
			t.Errorf("%s: message %q", c.name, got[0].Message)
		}
		if v == nil {
			t.Errorf("%s: no value", c.name)
		}
	}
}

// Two input base chains: an accept in the first does not protect against a drop in the second.
func TestAntiLockoutAcrossChains(t *testing.T) {
	ds := doc(t, `{"acl": {"host": {
	  "a": {"rules": [{"sequence": 1, "action": "accept"}]},
	  "b": {"rules": [{"sequence": 1, "action": "drop", "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["443"]}}}]}},
	 "hostAttachments": [{"list": "b", "chain": "input", "priority": 50}, {"list": "a", "chain": "input", "priority": -50}, {"list": "b", "chain": "output"}],
	 "hostSettings": {"antiLockout": {"enabled": false}}}}`)
	v, issues := build(t, ds)
	errs := errorsOf(issues)
	if len(errs) != 1 || errs[0].Pointer != "/acl/host/b/rules/0" || !strings.Contains(errs[0].Message, "TCP 443") || !strings.Contains(errs[0].Message, "in_b") {
		t.Fatalf("errors: %+v", errs)
	}
	var names []string
	for _, c := range v.GetChains() {
		names = append(names, c.GetName())
	}
	if !slices.Equal(names, []string{"in_a", "in_b", "out_b"}) {
		t.Errorf("chain order %v", names)
	}
}

func TestBuildErrors(t *testing.T) {
	cases := map[string]struct{ doc, pointer, rule string }{
		"unknown list":      {`{"acl": {"hostAttachments": [{"list": "nope", "chain": "input"}]}}`, "/acl/hostAttachments/0/list", RuleAttachment},
		"bad chain":         {`{"acl": {"host": {"l": {}}, "hostAttachments": [{"list": "l", "chain": "prerouting"}]}}`, "/acl/hostAttachments/0/chain", RuleAttachment},
		"priority":          {`{"acl": {"host": {"l": {}}, "hostAttachments": [{"list": "l", "chain": "input", "priority": 900}]}}`, "/acl/hostAttachments/0/priority", RuleAttachment},
		"twice on a chain":  {`{"acl": {"host": {"l": {}}, "hostAttachments": [{"list": "l", "chain": "input"}, {"list": "l", "chain": "input", "priority": 5}]}}`, "/acl/hostAttachments/1/chain", RuleAttachment},
		"unknown object":    {`{"objects": {}, "acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "source": {"kind": "object", "name": "x"}}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`, "/acl/host/l/rules/0/source/name", RuleObject},
		"unknown service":   {`{"objects": {}, "acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "service": {"kind": "object", "name": "x"}}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`, "/acl/host/l/rules/0/service/name", RuleObject},
		"bad action":        {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "permit"}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`, "/acl/host/l/rules/0/action", RuleRule},
		"sequence twice":    {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop"}, {"sequence": 1, "action": "drop"}]}}}}`, "/acl/host/l/rules/1/sequence", RuleRule},
		"default input":     {`{"acl": {"hostSettings": {"defaultInput": "reject"}}}`, "/acl/hostSettings/defaultInput", RuleSettings},
		"lockout port":      {`{"acl": {"hostSettings": {"antiLockout": {"ports": [70000]}}}}`, "/acl/hostSettings/antiLockout/ports/0", RuleSettings},
		"lockout source":    {`{"acl": {"hostSettings": {"antiLockout": {"sources": ["10.0.0.0/33"]}}}}`, "/acl/hostSettings/antiLockout/sources/0", RuleSettings},
		"inline bad port":   {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["0"]}}}]}}}}`, "/acl/host/l/rules/0/service/spec", RuleObject},
		"expansion limit":   {limitDoc(), "/acl/host/l/rules/0/source/name", RuleLimit},
		"address kind":      {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "source": {"kind": "zone"}}]}}}}`, "/acl/host/l/rules/0/source/kind", RuleRule},
		"ip version":        {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "ipVersion": "ipv5"}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`, "/acl/host/l/rules/0/ipVersion", RuleRule},
		"no objects at all": {`{"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "source": {"kind": "object", "name": "x"}}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`, "/acl/host/l/rules/0/source/name", RuleObject},
	}
	for name, c := range cases {
		_, issues := build(t, doc(t, c.doc))
		errs := errorsOf(issues)
		if len(errs) == 0 || errs[0].Pointer != c.pointer || errs[0].Rule != c.rule {
			t.Errorf("%s: want %s at %s, got %+v", name, c.rule, c.pointer, issues)
		}
	}
}

// limitDoc has an address group of 10001 hosts (more than objects.MaxEntries).
func limitDoc() string {
	var b strings.Builder
	b.WriteString(`{"objects": {"addresses": {`)
	var members []string
	for i := 0; i < 10001; i++ {
		n := "h" + strconv.Itoa(i)
		members = append(members, `"`+n+`"`)
		if i > 0 {
			b.WriteString(",")
		}
		a := 2 * i // every other address: nothing aggregates
		b.WriteString(`"` + n + `": {"type": "host", "address": "10.` + strconv.Itoa(a>>16&255) + `.` + strconv.Itoa(a>>8&255) + `.` + strconv.Itoa(a&255) + `"}`)
	}
	b.WriteString(`}, "addressGroups": {"big": {"members": [` + strings.Join(members, ",") + `]}}},`)
	b.WriteString(`"acl": {"host": {"l": {"rules": [{"sequence": 1, "action": "drop", "source": {"kind": "object", "name": "big"}}]}}, "hostAttachments": [{"list": "l", "chain": "input"}]}}`)
	return b.String()
}
