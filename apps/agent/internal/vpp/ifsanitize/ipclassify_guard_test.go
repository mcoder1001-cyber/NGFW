package ifsanitize_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// The ip classify zero-fill guard (INC-vpp-classify-crash, D-185, mitigation M1). VPP 26.06 grows
// the per-index ip4/ip6 classify vector with a ZERO fill, so an interface whose slot was never set
// explicitly may read "classify table 0"; an address add then installs a classify /32 that crashes
// VPP once table 0 is freed. Every interface creator in the agent module — product code, test
// fixtures and _test.go helpers alike — must therefore reset ip4+ip6 classify on the new index
// before any address is added:
//
//   - through ifsanitize.Acquire / iface.AcquireAndTag / a df6.IfSpec (Sanitize resets it), or
//   - by an ifsanitize.ResetIPClassify or ifsanitize.Sanitize call in the SAME function, after the
//     create and before any sw_interface_add_del_address add in that function.
//
// The creator set is the binapi-derived one of TestEveryInterfaceCreatorIsSanitized (fail closed).
//
// Limits of this static scan (review F8) — a reviewer checks these by hand:
//   - it is syntactic and per function: a reset done by a helper the creator calls (other than
//     ResetIPClassify / Sanitize / the test/ mirror resetIPClassify) is not seen, and fails closed;
//   - it is positional, not control-flow aware: a reset inside a branch that may not run (if/switch)
//     after the create counts;
//   - it does not check that the reset names the created index;
//   - address adds are recognised only as sw_interface_add_del_address calls/literals; an address added
//     through a helper (h.address, an agent descriptor, vppctl/cli_inband text) before the reset is missed;
//   - resets inside a func literal that does not contain the create, a defer or a go statement do not
//     count (they run later or elsewhere); address adds count wherever they are;
//   - a create request that is not a literal with IsAdd/MtIsAdd false is treated as a create;
//   - Python/TS fixtures are not scanned; shell fixtures are checked by tools/ci.sh (15-line window).
const agentModuleDir = "../../.."

// repoTestDir is the repository's test/ tree: the topology and integration Go modules (their own
// go.mod, they cannot import ifsanitize and use a local resetIPClassify mirror). Keys there are
// "test/<path>:<func>". Shell fixtures (*.sh) are checked by tools/ci.sh (review F3).
const repoTestDir = "../../../../../test"

// inheritMarker on the creator's line (or the line above) marks a deliberate V19 reproduction:
// an index that must inherit an old binding (to a live table) until Sanitize clears it.
const inheritMarker = "ipclassify:inherit"

// zeroFillExempt: <slash path relative to the agent module>:<function> → why the create needs no
// reset of its own.
var zeroFillExempt = map[string]string{
	// the holder takes an index Sanitize (whose third step is the ip classify reset) has just run on;
	// it stays admin-down and never gets an address
	"internal/vpp/ifsanitize/acquire.go:createHolder": "quarantine holder on an already reset index",
	// fake VPP clients: nothing reaches the host VPP
	"internal/vpp/ifsanitize/acquire_test.go:TestQuarantineHolderInstance": "fake VPP (sanitizetest)",
	"internal/agent/rpc_mpls_srmpls_test.go:addTunnel":                     "fake VPP (coretest)",
	"internal/descriptors/core/dhcplease_test.go:newLeaseRig":              "fake VPP (coretest)",
}

// zeroFillOutOfScope lists real-VPP creator sites in files outside this module's reach that still
// lack the reset. They are logged, not failed. Each entry must still be a violation — once the
// owner adds the reset, the test fails until the entry is removed — so the list can only shrink.
// Empty since review round 1 (D-191 granted the four sites of round 0).
var zeroFillOutOfScope = map[string]string{}

// zeroFillSite is one creator call and what follows it in its function.
type zeroFillSite struct {
	pos    token.Position
	fn     string // enclosing FuncDecl name (a literal inherits its declaration's name)
	msg    string
	reason string // "" = ok
	marked bool   // carries inheritMarker
}

// scanZeroFill checks every creator call of f.
func scanZeroFill(fset *token.FileSet, f *ast.File, creators map[string]bool) []zeroFillSite {
	marked := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, inheritMarker) {
				marked[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	var out []zeroFillSite
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		msg, req := creatorCall(call, creators)
		if msg == "" || isDelete(req) || guard(stack) != "" {
			return true
		}
		pos := fset.Position(call.Pos())
		site := zeroFillSite{pos: pos, fn: declName(stack), msg: msg}
		if marked[pos.Line] || marked[pos.Line-1] {
			site.marked = true
		} else {
			site.reason = resetFollows(enclosingBody(stack), call)
		}
		out = append(out, site)
		return true
	})
	return out
}

// enclosingBody is the innermost function body around the call.
func enclosingBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			return fn.Body
		case *ast.FuncDecl:
			return fn.Body
		}
	}
	return nil
}

func declName(stack []ast.Node) string {
	for _, n := range stack {
		if fd, ok := n.(*ast.FuncDecl); ok {
			return fd.Name.Name
		}
	}
	return "?"
}

// resetFollows returns "" when body has a reset call after create and no address add between
// them, else the violation text. A reset counts only when it runs in the create's own flow: calls
// inside a func literal that does not contain the create (a t.Cleanup / callback body runs later
// or elsewhere), inside a defer or inside a go statement are not resets (review F1: a reset only
// in t.Cleanup is the incident's own pattern). Address adds count everywhere (fail closed).
func resetFollows(body *ast.BlockStmt, create *ast.CallExpr) string {
	if body == nil {
		return "creator outside a function body"
	}
	reset, addr := token.NoPos, token.NoPos
	var walk func(n ast.Node, deferred bool)
	walk = func(n ast.Node, deferred bool) {
		ast.Inspect(n, func(m ast.Node) bool {
			if m == nil || m == n {
				return true
			}
			switch x := m.(type) {
			case *ast.FuncLit, *ast.DeferStmt, *ast.GoStmt:
				if !contains(x, create) {
					walk(x, true)
					return false
				}
			}
			c, ok := m.(*ast.CallExpr)
			if !ok || c.Pos() <= create.End() {
				return true
			}
			switch {
			case isResetCall(c) && !deferred:
				if !reset.IsValid() || c.Pos() < reset {
					reset = c.Pos()
				}
			case isAddressAdd(c):
				if !addr.IsValid() || c.Pos() < addr {
					addr = c.Pos()
				}
			}
			return true
		})
	}
	walk(body, false)
	switch {
	case !reset.IsValid():
		return "no ifsanitize.ResetIPClassify / Sanitize after it in the same function"
	case addr.IsValid() && addr < reset:
		return "an address is added before the ip classify reset"
	}
	return ""
}

func contains(n ast.Node, c *ast.CallExpr) bool { return n.Pos() <= c.Pos() && c.End() <= n.End() }

// isResetCall: ifsanitize.ResetIPClassify / ifsanitize.Sanitize, or the unqualified forms inside
// package ifsanitize itself.
func isResetCall(c *ast.CallExpr) bool {
	switch f := c.Fun.(type) {
	case *ast.SelectorExpr:
		x, _ := f.X.(*ast.Ident)
		return x != nil && x.Name == "ifsanitize" && (f.Sel.Name == "ResetIPClassify" || f.Sel.Name == "Sanitize")
	case *ast.Ident:
		return f.Name == "ResetIPClassify" || f.Name == "Sanitize" || f.Name == "resetIPClassify" // the test/ modules' mirror
	}
	return false
}

// isAddressAdd: a sw_interface_add_del_address that is not a literal IsAdd: false.
func isAddressAdd(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name == "SwInterfaceAddDelAddress" && len(c.Args) == 2 {
		return !isDelete(c.Args[1])
	}
	switch sel.Sel.Name {
	case "Invoke", "SendRequest":
		for _, a := range c.Args {
			if literalType(a) == "SwInterfaceAddDelAddress" {
				return !isDelete(a)
			}
		}
	}
	return false
}

// TestEveryInterfaceCreatorResetsIPClassify is the M1 guard over the whole agent module (binapi
// excluded: product code, *test packages and _test.go files) and the Go modules under test/.
func TestEveryInterfaceCreatorResetsIPClassify(t *testing.T) {
	creators := interfaceCreators(t)
	fset := token.NewFileSet()
	var bad []string
	ok, exempt, marked, okTest := 0, 0, 0, 0
	seenOOS := map[string]bool{}
	for _, root := range []struct{ dir, prefix string }{{agentModuleDir, ""}, {repoTestDir, "test/"}} {
		err := filepath.WalkDir(root.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root.dir, path)
			rel = root.prefix + filepath.ToSlash(rel)
			if d.IsDir() {
				if d.Name() == "node_modules" || rel == "binapi" || rel == "gen" || strings.HasPrefix(d.Name(), ".") && path != root.dir {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
			if err != nil {
				return err
			}
			for _, s := range scanZeroFill(fset, f, creators) {
				key := rel + ":" + s.fn
				switch {
				case zeroFillExempt[key] != "":
					exempt++
				case zeroFillOutOfScope[key] != "":
					seenOOS[key] = true
					if s.reason == "" {
						bad = append(bad, key+": fixed — remove it from zeroFillOutOfScope")
					} else {
						t.Logf("OUT OF SCOPE (not owned): %s:%d %s: %s", rel, s.pos.Line, s.msg, s.reason)
					}
				case s.marked:
					marked++
				case s.reason == "":
					ok++
					if root.prefix != "" {
						okTest++
					}
				default:
					bad = append(bad, rel+":"+strconv.Itoa(s.pos.Line)+" ("+s.fn+"): "+s.msg+": "+s.reason+" (INC-vpp-classify-crash M1, D-185)")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for k := range zeroFillOutOfScope {
		if !seenOOS[k] {
			bad = append(bad, k+": listed in zeroFillOutOfScope but no creator found there — remove the entry")
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if ok < 10 || okTest < 3 {
		t.Fatalf("only %d guarded creator sites found (%d under test/): the scan is not looking at the module", ok, okTest)
	}
	t.Logf("%d creator sites (%d under test/) reset ip classify before any address; %d marked V19 reproductions; %d exempt; %d out of scope; %d violations", ok, okTest, marked, exempt, len(seenOOS), len(bad))
}

// TestZeroFillGuardCatches: the scanner flags a create without a reset, a reset after an address
// add, a reset in another closure, a reset only in t.Cleanup, a deferred reset and a reset in a
// goroutine, and accepts the reset, Sanitize, Acquire and marked forms.
func TestZeroFillGuardCatches(t *testing.T) {
	creators := interfaceCreators(t)
	src := `package x

func noReset(ctx context.Context, svc interfaces.RPCService) {
	rep, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // bad: no reset
	_ = rep
}

func addrFirst(ctx context.Context, svc interfaces.RPCService, c vpp.Client) {
	rep, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // bad: address before reset
	svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{IsAdd: true, SwIfIndex: rep.SwIfIndex})
	_ = ifsanitize.ResetIPClassify(ctx, c, uint32(rep.SwIfIndex))
}

func otherFunc(ctx context.Context, svc tapapi.RPCService, c vpp.Client) {
	t.Cleanup(func() { _ = ifsanitize.ResetIPClassify(ctx, c, 1) })
	svc.TapCreateV3(ctx, req) // bad: the reset is in a closure that runs earlier/elsewhere
}

func resetOnlyInCleanup(ctx context.Context, svc interfaces.RPCService, c vpp.Client) {
	rep, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // bad: reset only at delete (the incident)
	t.Cleanup(func() { _ = ifsanitize.ResetIPClassify(ctx, c, uint32(rep.SwIfIndex)) })
}

func resetDeferredOrGo(ctx context.Context, svc interfaces.RPCService, c vpp.Client) {
	rep, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // bad: deferred reset
	defer ifsanitize.ResetIPClassify(ctx, c, uint32(rep.SwIfIndex))
	rep2, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // bad: reset in a goroutine
	go ifsanitize.ResetIPClassify(ctx, c, uint32(rep2.SwIfIndex))
}

func good(ctx context.Context, svc interfaces.RPCService, c vpp.Client) {
	rep, _ := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{}) // ok
	svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{IsAdd: false})
	_ = ifsanitize.ResetIPClassify(ctx, c, uint32(rep.SwIfIndex))
	svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{IsAdd: true, SwIfIndex: rep.SwIfIndex})
	rep2, _ := svc.CreateSubif(ctx, req) // ok: Sanitize
	_, _ = ifsanitize.Sanitize(ctx, c, uint32(rep2.SwIfIndex), "x")
	ifsanitize.Acquire(ctx, c, o, n, func() (uint32, error) { svc.CreateLoopback(ctx, r); return 0, nil }, del) // ok: Acquire
	svc.CreateLoopback(ctx, r) // ipclassify:inherit ok: marked
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var badLines []int
	for _, s := range scanZeroFill(fset, f, creators) {
		if s.reason != "" {
			badLines = append(badLines, s.pos.Line)
			t.Logf("flagged: %s (%s): %s", s.pos, s.fn, s.reason)
		}
	}
	if want := []int{4, 9, 16, 20, 25, 27}; !slices.Equal(badLines, want) {
		t.Fatalf("violations at lines %v, want %v", badLines, want)
	}
}

// TestResetIPClassifySendsBothFamilies: ResetIPClassify is exactly two
// classify_set_interface_ip_table calls, ip4 then ip6, both table_index ~0, on the given index.
func TestResetIPClassifySendsBothFamilies(t *testing.T) {
	f, _ := setup()
	if err := ifsanitize.ResetIPClassify(context.Background(), f, 9); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("classify_set_interface_ip_table")
	if len(calls) != 2 || len(f.Calls()) != 2 {
		t.Fatalf("sent %d classify_set_interface_ip_table of %d calls, want 2 of 2", len(calls), len(f.Calls()))
	}
	for i, c := range calls {
		r := c.(*classifyapi.ClassifySetInterfaceIPTable)
		if r.IsIPv6 != (i == 1) || r.SwIfIndex != 9 || r.TableIndex != ifsanitize.NoIndex {
			t.Fatalf("call %d: %+v", i, r)
		}
	}
}

// TestResetIPClassifyError: a VPP error names the family and the index.
func TestResetIPClassifyError(t *testing.T) {
	f := fake.New()
	f.On("classify_set_interface_ip_table", func(api.Message) ([]api.Message, error) {
		return []api.Message{&classifyapi.ClassifySetInterfaceIPTableReply{Retval: -2}}, nil
	})
	err := ifsanitize.ResetIPClassify(context.Background(), f, 5)
	if err == nil || !strings.Contains(err.Error(), "reset ip4, sw_if_index 5") {
		t.Fatalf("err = %v", err)
	}
}
