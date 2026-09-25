package subsystems

// TD-11a (review 3(a)/3(b), D-125, D-141): every descriptor and renderer package of the agent is
// accounted for — wired into the product, allowlisted as "pending" on the board row that wires it,
// or declared a library. The pending allowlist is shrink-only: a row that wires its package flips
// the entry to wired (the test fails until it does) and lowers maxPending; adding a pending entry
// means raising maxPending, which a reviewer sees.
//
//	descriptors: wired = a selector of the package is used inside subsystems' register() (the
//	             registry, wave-A-hotspots A1); options-only uses in Wiring methods do not count.
//	renderers:   wired = <pkg>.New is called by non-test code outside internal/renderers.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type reach int

const (
	wired   reach = iota // reachable from the desired state through the agent today
	pending              // built, not yet wired; row = the board row that wires it
	library              // shared code or not a registry item; row = why
)

type reachEntry struct {
	state reach
	row   string // board row id (wired/pending) or the reason (library)
}

// maxPending is the size of the pending allowlist. Lower it when you wire a package; never raise it
// without a board row that wires the new package (TD-11a, D-125).
const maxPending = 57

var descriptorReach = map[string]reachEntry{
	"abf":                 {pending, "F-rpf-adl-pbr"},
	"acl":                 {pending, "F-acl"},
	"adl":                 {pending, "F-rpf-adl-pbr"},
	"af_packet":           {wired, "P08"},
	"arp":                 {pending, "F-neighbors-ra"},
	"bfd":                 {pending, "F-bfd-redistribution"},
	"bond":                {pending, "F-bonding"},
	"classify":            {pending, "F-rpf-adl-pbr"},
	"cnat":                {pending, "F-det44-map-dslite-cnat"},
	"core":                {wired, "P05"},
	"det44":               {pending, "F-det44-map-dslite-cnat"},
	"df2":                 {library, "DF-2 shared helpers (keys, claims, canonicalisation)"},
	"df6":                 {library, "DF-6 shared helpers"},
	"df7":                 {library, "DF-7 shared helpers (codec, boot store, registry)"},
	"dfkit":               {library, "DF-8 / descriptor kit (D-077)"},
	"dhcp":                {wired, "P08"},
	"dns":                 {pending, "F-unbound-chrony-syslog"},
	"flowprobe":           {pending, "F-ipfix-sflow"},
	"gre":                 {pending, "F-tunnels"},
	"gtpu":                {pending, "F-tunnels"},
	"igmp":                {pending, "F-igmp-mfib"},
	"ikev2":               {pending, "F-ikev2-native"},
	"interface":           {wired, "P08"},
	"ip6_nd":              {pending, "F-neighbors-ra"},
	"ip_neighbor":         {pending, "F-neighbors-ra"},
	"ip_session_redirect": {pending, "F-rpf-adl-pbr"},
	"ipfix":               {pending, "F-ipfix-sflow"},
	"ipip":                {pending, "F-tunnels"},
	"ipsec":               {pending, "P11"},
	"l2":                  {pending, "F-bridge-l2"},
	"l2tp":                {pending, "F-tunnels"},
	"l3xc":                {pending, "F-bridge-l2"},
	"lb":                  {pending, "F-lb"},
	"lcp":                 {pending, "P12"},
	"lisp":                {pending, "F-lisp"},
	"lldp":                {pending, "F-loopback-bvi-gso-lldp-span"},
	"mapnat":              {pending, "F-det44-map-dslite-cnat"},
	"memif":               {library, "D-141: no product domain; lab/test fixture until a row adds one"},
	"mpls":                {pending, "F-mpls-srmpls"},
	"nat44ed":             {pending, "F-nat44-ed-sessions"},
	"nat44ei":             {pending, "F-nat44-ei-64-66-nptv6"},
	"nat64":               {pending, "F-nat44-ei-64-66-nptv6"},
	"nat66":               {pending, "F-nat44-ei-64-66-nptv6"},
	"natcommon":           {library, "DF-3 shared NAT helpers"},
	"pcap":                {pending, "F-capture-trace"},
	"pnat":                {pending, "F-nat44-ei-64-66-nptv6"},
	"policer":             {pending, "F-qos-flat"},
	"pppoe":               {pending, "F-tunnels"},
	"qos":                 {pending, "F-qos-flat"},
	"sflow":               {pending, "F-ipfix-sflow"},
	"span":                {pending, "F-loopback-bvi-gso-lldp-span"},
	"sr":                  {pending, "F-srv6"},
	"sr_mpls":             {pending, "F-mpls-srmpls"},
	"tapv2":               {library, "D-141: test rig creator (integration tests); no product domain"},
	"trace":               {pending, "F-capture-trace"},
	"urpf":                {pending, "F-rpf-adl-pbr"},
	"vpn":                 {library, "DF-5 shared types, secret contract, keyer"},
	"vrrp":                {pending, "F-vrrp-config-sync"},
	"vxlan":               {pending, "F-tunnels"},
	"vxlan_gpe":           {pending, "F-tunnels"},
	"wireguard":           {pending, "F-wireguard"},
}

var rendererReach = map[string]reachEntry{
	"chrony":     {pending, "F-unbound-chrony-syslog"},
	"frr":        {pending, "P12"},
	"kea":        {pending, "F-kea-dhcp-relay"},
	"keepalived": {pending, "F-vrrp-config-sync"},
	"rfkit":      {library, "RF-4 shared daemon-renderer kit"},
	"rsyslog":    {pending, "F-unbound-chrony-syslog"},
	"snmpd":      {pending, "F-snmp"},
	"strongswan": {pending, "P11"},
	"unbound":    {pending, "F-unbound-chrony-syslog"},
	"vppstartup": {library, "startup.conf generator: cmd/vrx-startupgen (F-startup-gen), not an agent registry item"},
}

const modPath = "ngfw/agent/internal/"

func TestReachabilityTable(t *testing.T) {
	rows := boardRows(t)
	regUsed := registerSelectors(t)
	newCalled := rendererNewCalls(t)
	npending := 0
	for _, kind := range []struct {
		dir   string
		table map[string]reachEntry
		used  map[string]bool
		how   string
	}{
		{"descriptors", descriptorReach, regUsed, "used in subsystems register()"},
		{"renderers", rendererReach, newCalled, "<pkg>.New called by non-test code outside internal/renderers"},
	} {
		dirs := packageDirs(t, filepath.Join("..", kind.dir))
		for _, d := range dirs {
			if _, ok := kind.table[d]; !ok {
				t.Errorf("%s/%s: not in the TD-11a reachability table — add it as pending on the row that wires it, or library with a reason", kind.dir, d)
			}
		}
		for name, e := range kind.table {
			if !contains(dirs, name) {
				t.Errorf("%s/%s: table entry for a package that does not exist — remove it", kind.dir, name)
				continue
			}
			if e.row == "" {
				t.Errorf("%s/%s: empty row/reason", kind.dir, name)
			}
			switch e.state {
			case wired:
				if !kind.used[name] {
					t.Errorf("%s/%s: marked wired (%s) but not %s", kind.dir, name, e.row, kind.how)
				}
			case pending:
				npending++
				if kind.used[name] {
					t.Errorf("%s/%s: now %s — flip it to wired and lower maxPending (shrink-only allowlist)", kind.dir, name, kind.how)
				}
				if !rows[e.row] {
					t.Errorf("%s/%s: pending on %q, which is not a row of plan/tasks.yaml", kind.dir, name, e.row)
				}
			}
		}
	}
	if npending != maxPending {
		t.Errorf("pending allowlist has %d entries, maxPending is %d: the allowlist only shrinks — set maxPending to %d when you wire a package; growing it needs a board row", npending, maxPending, npending)
	}
}

func contains(ss []string, s string) bool {
	i := sort.SearchStrings(ss, s)
	return i < len(ss) && ss[i] == s
}

// packageDirs lists the immediate sub-directories of dir that hold Go files (sorted).
func packageDirs(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if m, _ := filepath.Glob(filepath.Join(dir, e.Name(), "*.go")); len(m) > 0 {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// boardRows reads the row ids of plan/tasks.yaml (a "- id: X" line each; no YAML dependency).
func boardRows(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "plan", "tasks.yaml"))
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	rows := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if id, ok := strings.CutPrefix(l, "- id: "); ok {
			rows[strings.TrimSpace(id)] = true
		}
	}
	return rows
}

// importsOf maps the local names of a file's imports under internal/<sub>/ to their top-level package dir.
func importsOf(f *ast.File, sub string) map[string]string {
	m := map[string]string{}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		rest, ok := strings.CutPrefix(p, modPath+sub+"/")
		if !ok {
			continue
		}
		top := strings.SplitN(rest, "/", 2)[0]
		local := filepath.Base(p)
		if im.Name != nil {
			local = im.Name.Name
		} else if top == "interface" && local == "interface" {
			local = "iface" // package iface lives in descriptors/interface
		} else if top == "ip_session_redirect" {
			local = "sessionredirect"
		}
		m[local] = top
	}
	return m
}

// registerSelectors returns the descriptor packages whose identifiers are used inside func register.
func registerSelectors(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	found := false
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	for _, fn := range files {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, fn, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		imps := importsOf(f, "descriptors")
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != "register" {
				continue
			}
			found = true
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if se, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && imps[id.Name] != "" {
						used[imps[id.Name]] = true
					}
				}
				return true
			})
		}
	}
	if !found {
		t.Fatal("func register not found in internal/subsystems — update the TD-11a reachability test")
	}
	// descriptor wrappers declared at file level (vethOnly, newDefaultTolerant) reach register through
	// the packages already counted above.
	return used
}

// rendererNewCalls returns the renderer packages whose New is called by non-test code outside internal/renderers.
func rendererNewCalls(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "renderers", "binapi", "gen", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		imps := importsOf(f, "renderers")
		if len(imps) == 0 {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok && se.Sel.Name == "New" {
				if id, ok := se.X.(*ast.Ident); ok && imps[id.Name] != "" {
					used[imps[id.Name]] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return used
}
