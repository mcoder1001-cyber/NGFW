package subsystems

// TD-11a (review 3(a)/3(b), D-125, D-141): every descriptor and renderer package of the agent is
// accounted for — wired into the product, allowlisted as "pending" on the board row that wires it,
// or declared a library. The pending allowlist is shrink-only: a row that wires its package flips
// the entry to wired (the test fails until it does) and lowers maxPending; adding a pending entry
// means raising maxPending, which a reviewer sees.
//
//	descriptors: wired = a call <pkg>.Register(…) or <pkg>.New*(…) inside subsystems' register() (the
//	             registry, wave-A-hotspots A1), or inside a same-package function that register()
//	             calls directly (one level deep). Option/setter uses (With*, Set*, constants) and uses
//	             in Wiring methods do not count.
//	library:     pinned too — the sorted set of library entries must equal libraryPins, so moving a
//	             package from pending to library is a visible edit.
//	row column:  the board row that owns the wiring (wired: the row that wired it; pending: the row
//	             that will).
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
	"core":                {wired, "P08"},
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

// libraryPins is the exact library set; change it only with a reason in the entry (and a D-entry for
// a package that has descriptors, like D-141).
var libraryPins = []string{
	"descriptors/df2", "descriptors/df6", "descriptors/df7", "descriptors/dfkit", "descriptors/memif",
	"descriptors/natcommon", "descriptors/tapv2", "descriptors/vpn", "renderers/rfkit", "renderers/vppstartup",
}

const modPath = "ngfw/agent/internal/"

func TestReachabilityTable(t *testing.T) {
	rows := boardRows(t)
	regUsed := registerSelectors(t)
	newCalled := rendererNewCalls(t)
	npending := 0
	var libs []string
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
			case library:
				libs = append(libs, kind.dir+"/"+name)
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
	sort.Strings(libs)
	if strings.Join(libs, ",") != strings.Join(libraryPins, ",") {
		t.Errorf("library set %v != libraryPins %v: a library entry needs a reason and a reviewed pin", libs, libraryPins)
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

// importsOf maps the local names of a file's imports under internal/<sub>/ to their top-level package
// dir. An unaliased import's local name is the imported package's own package clause (read from
// srcRoot/<import path under ngfw/agent>), never the directory name (ip6_nd is package ip6nd).
func importsOf(t *testing.T, f *ast.File, sub, srcRoot string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		rest, ok := strings.CutPrefix(p, modPath+sub+"/")
		if !ok {
			continue
		}
		top := strings.SplitN(rest, "/", 2)[0]
		local := ""
		if im.Name != nil {
			local = im.Name.Name
		} else {
			local = packageName(t, filepath.Join(srcRoot, strings.TrimPrefix(p, "ngfw/agent/")))
		}
		m[local] = top
	}
	return m
}

// packageName returns the package clause of the non-test Go files in dir.
func packageName(t *testing.T, dir string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, fn := range files {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), fn, nil, parser.PackageClauseOnly)
		if err != nil {
			t.Fatal(err)
		}
		return f.Name.Name
	}
	t.Fatalf("no Go files in %s", dir)
	return ""
}

// agentRoot is apps/agent relative to this package.
var agentRoot = filepath.Join("..", "..")

// wiringCall reports whether call is <pkg>.Register(…) or <pkg>.New*(…) of an imported package.
func wiringCall(call *ast.CallExpr, imps map[string]string) string {
	se, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := se.X.(*ast.Ident)
	if !ok || imps[id.Name] == "" {
		return ""
	}
	if se.Sel.Name == "Register" || strings.HasPrefix(se.Sel.Name, "New") {
		return imps[id.Name]
	}
	return ""
}

// registerSelectors returns the descriptor packages that register() wires (see the header).
func registerSelectors(t *testing.T) map[string]bool {
	t.Helper()
	type fn struct {
		decl *ast.FuncDecl
		imps map[string]string
	}
	funcs := map[string]fn{}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		imps := importsOf(t, f, "descriptors", agentRoot)
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				funcs[fd.Name.Name] = fn{fd, imps}
			}
		}
	}
	reg, ok := funcs["register"]
	if !ok {
		t.Fatal("func register not found in internal/subsystems — update the TD-11a reachability test")
	}
	used := map[string]bool{}
	var helpers []fn
	ast.Inspect(reg.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if p := wiringCall(call, reg.imps); p != "" {
			used[p] = true
		}
		if id, ok := call.Fun.(*ast.Ident); ok {
			if h, ok := funcs[id.Name]; ok && id.Name != "register" {
				helpers = append(helpers, h)
			}
		}
		return true
	})
	for _, h := range helpers { // one level deep
		ast.Inspect(h.decl.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if p := wiringCall(call, h.imps); p != "" {
					used[p] = true
				}
			}
			return true
		})
	}
	return used
}

// TestImportsOfUnaliased: an unaliased import resolves to the package clause, not the dir name.
func TestImportsOfUnaliased(t *testing.T) {
	src := `package x
import (
	"ngfw/agent/internal/descriptors/ip6_nd"
	"ngfw/agent/internal/descriptors/ip_neighbor"
	"ngfw/agent/internal/descriptors/interface"
	al "ngfw/agent/internal/descriptors/core"
)`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	got := importsOf(t, f, "descriptors", agentRoot)
	want := map[string]string{"ip6nd": "ip6_nd", "ipneighbor": "ip_neighbor", "iface": "interface", "al": "core"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("importsOf[%q] = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

// rendererNewCalls returns the renderer packages whose New is called by non-test code outside internal/renderers.
func rendererNewCalls(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	root := agentRoot
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
		imps := importsOf(t, f, "renderers", agentRoot)
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
