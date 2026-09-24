package ifsanitize_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The guard (TD-3 re-review H1): every VPP interface-create message a descriptor sends must go
// through ifsanitize — DF-5 added ipsec.itf and wireguard.interface without it, and the next
// creator must not be able to forget it silently.
//
// The creator set is derived from the generated bindings (00-CONTEXT rule 6: no message name from
// memory): every request whose reply carries a sw_if_index field (SwIfIndex, HostSwIfIndex, …),
// minus the reads listed in notCreators, plus the creators whose reply carries none
// (extraCreators). A new binapi message with a sw_if_index in its reply is a creator until it is
// listed here as a read — fail closed.
const (
	binapiDir      = "../../../binapi"
	descriptorsDir = "../../descriptors"
)

// notCreators reply with a sw_if_index but create nothing.
var notCreators = []string{"BfdUDPGetEchoSource", "ClassifyTableByInterface", "CnatGetSnatAddresses"}

// extraCreators create an interface but their reply carries no sw_if_index.
var extraCreators = []string{"GpeAddDelIface"}

var (
	creatorsOnce sync.Once
	creatorsSet  map[string]bool
	creatorsErr  string
)

// interfaceCreators parses the binapi tree once per test binary.
func interfaceCreators(t *testing.T) map[string]bool {
	t.Helper()
	creatorsOnce.Do(func() { creatorsSet, creatorsErr = parseCreators() })
	if creatorsErr != "" {
		t.Fatal(creatorsErr)
	}
	return creatorsSet
}

func parseCreators() (map[string]bool, string) {
	requests, creators := map[string]bool{}, map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(binapiDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".ba.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, spec := range g.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				requests[ts.Name.Name] = true
				req, isReply := strings.CutSuffix(ts.Name.Name, "Reply")
				if !isReply {
					continue
				}
				for _, fld := range st.Fields.List {
					for _, n := range fld.Names {
						if strings.HasSuffix(n.Name, "SwIfIndex") {
							creators[req] = true
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err.Error()
	}
	for _, n := range append(slices.Clone(notCreators), extraCreators...) {
		if !requests[n] {
			return nil, n + " is not a binapi message (guard lists must name generated messages only)"
		}
	}
	for _, n := range notCreators {
		delete(creators, n)
	}
	for _, n := range extraCreators {
		creators[n] = true
	}
	for c := range creators {
		if !requests[c] {
			delete(creators, c) // a reply without a request (events)
		}
	}
	return creators, ""
}

// violation is one creator call that does not go through ifsanitize.
type violation struct {
	pos token.Position
	msg string
}

func (v violation) String() string { return v.pos.String() + ": " + v.msg }

// site kinds a guarded creator call can sit in
const (
	viaAcquire = "acquire" // a func literal passed to ifsanitize.Acquire / AcquireAndTag
	viaIfSpec  = "ifspec"  // the Add/Del func of a df6.IfSpec (df6.IfDescriptor runs them through ifsanitize)
)

// scanFile reports every call of a creator message in f that is not inside an ifsanitize.Acquire /
// AcquireAndTag closure or a df6.IfSpec Add/Del func. A call whose request literal says
// IsAdd/MtIsAdd: false is a delete and not checked. It also returns the guard kinds it saw.
func scanFile(fset *token.FileSet, f *ast.File, creators map[string]bool) ([]violation, map[string]bool) {
	var out []violation
	seen := map[string]bool{}
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
		if msg == "" || isDelete(req) {
			return true
		}
		if kind := guard(stack); kind != "" {
			seen[kind] = true
			return true
		}
		out = append(out, violation{fset.Position(call.Pos()), msg + " sent outside ifsanitize.Acquire / iface.AcquireAndTag / a df6.IfSpec Add (VPP V19, D-095)"})
		return true
	})
	return out, seen
}

// creatorCall returns the creator message a call sends and its request argument: a generated
// service-client method (svc.TapCreateV3(ctx, req)) or a raw Invoke/SendRequest of a literal.
func creatorCall(call *ast.CallExpr, creators map[string]bool) (string, ast.Expr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil
	}
	if creators[sel.Sel.Name] && len(call.Args) == 2 {
		return sel.Sel.Name, call.Args[1]
	}
	switch sel.Sel.Name {
	case "Invoke", "SendRequest", "SendMultiRequest":
		for _, a := range call.Args {
			if name := literalType(a); creators[name] {
				return name, a
			}
		}
	}
	return "", nil
}

func literalType(e ast.Expr) string {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.AND {
		e = u.X
	}
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	if sel, ok := cl.Type.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return ""
}

// isDelete reports whether req is a literal with IsAdd / MtIsAdd set to false.
func isDelete(req ast.Expr) bool {
	if u, ok := req.(*ast.UnaryExpr); ok && u.Op == token.AND {
		req = u.X
	}
	cl, ok := req.(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		k, _ := kv.Key.(*ast.Ident)
		v, _ := kv.Value.(*ast.Ident)
		if k != nil && v != nil && (k.Name == "IsAdd" || k.Name == "MtIsAdd") && v.Name == "false" {
			return true
		}
	}
	return false
}

// guard walks the ancestors of a call (innermost last) for a func literal that is an argument of
// ifsanitize.Acquire / AcquireAndTag, or the Add/Del value of a df6.IfSpec literal.
func guard(stack []ast.Node) string {
	for i := len(stack) - 1; i > 0; i-- {
		fl, ok := stack[i].(*ast.FuncLit)
		if !ok {
			continue
		}
		switch p := stack[i-1].(type) {
		case *ast.CallExpr:
			if isAcquire(p.Fun) && slices.Contains(p.Args, ast.Expr(fl)) {
				return viaAcquire
			}
		case *ast.KeyValueExpr:
			k, _ := p.Key.(*ast.Ident)
			if k == nil || (k.Name != "Add" && k.Name != "Del") || i < 2 {
				continue
			}
			if cl, ok := stack[i-2].(*ast.CompositeLit); ok && isIfSpec(cl.Type) {
				return viaIfSpec
			}
		}
	}
	return ""
}

func isAcquire(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		x, _ := f.X.(*ast.Ident)
		return (x != nil && x.Name == "ifsanitize" && f.Sel.Name == "Acquire") || f.Sel.Name == "AcquireAndTag"
	case *ast.Ident:
		return f.Name == "AcquireAndTag"
	}
	return false
}

func isIfSpec(t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.IndexListExpr:
		t = x.X
	case *ast.IndexExpr:
		t = x.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, _ := sel.X.(*ast.Ident)
	return pkg != nil && pkg.Name == "df6" && sel.Sel.Name == "IfSpec"
}

// calls reports whether the file calls ifsanitize.<name>.
func calls(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "ifsanitize" && sel.Sel.Name == name {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// productFile reports whether path is product code of a descriptor: not a _test.go file and not
// in a test-helper package (ifacetest, df6test, …: fixtures that create raw on purpose and
// sanitize in their own cleanup).
func productFile(path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if strings.HasSuffix(part, "test") {
			return false
		}
	}
	return true
}

// TestEveryInterfaceCreatorIsSanitized is the H1 guard over the real descriptor tree: every
// creator call goes through ifsanitize, and every package that creates through Acquire also calls
// ifsanitize.BeforeDelete (df6 does both for its IfSpec types).
func TestEveryInterfaceCreatorIsSanitized(t *testing.T) {
	creators := interfaceCreators(t)
	for _, want := range []string{"IpsecItfCreate", "WireguardInterfaceCreate", "TapCreateV3", "CreateLoopbackInstance", "LcpItfPairAddDelV3", "MplsTunnelAddDel", "GreTunnelAddDelV2"} {
		if !creators[want] {
			t.Fatalf("creator set derived from binapi lacks %s: %v", want, creators)
		}
	}
	fset := token.NewFileSet()
	var bad []string
	acquirePkgs, deletePkgs := map[string][]string{}, map[string]bool{}
	sites := 0
	err := filepath.WalkDir(descriptorsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !productFile(path) {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		v, seen := scanFile(fset, f, creators)
		for _, x := range v {
			bad = append(bad, x.String())
		}
		dir := filepath.Dir(path)
		if seen[viaAcquire] {
			acquirePkgs[dir] = append(acquirePkgs[dir], filepath.Base(path))
			sites++
		}
		if seen[viaIfSpec] {
			sites++
		}
		if calls(f, "BeforeDelete") {
			deletePkgs[dir] = true
		}
		if filepath.Base(dir) == "df6" && filepath.Base(path) == "ifdesc.go" {
			for _, fn := range []string{"Acquire", "Sanitize", "BeforeDelete"} {
				if !calls(f, fn) {
					bad = append(bad, path+": df6.IfDescriptor no longer calls ifsanitize."+fn+" (the IfSpec Add/Del guard relies on it)")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for dir, files := range acquirePkgs {
		if !deletePkgs[dir] {
			bad = append(bad, dir+": creates through ifsanitize.Acquire ("+strings.Join(files, ", ")+") but never calls ifsanitize.BeforeDelete before the delete (D-095 c)")
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if sites < 10 {
		t.Fatalf("only %d guarded creator files found: the scan is not looking at the descriptor tree", sites)
	}
	t.Logf("%d creator messages from binapi; %d descriptor files create interfaces through ifsanitize, %d violations; BeforeDelete in %d packages", len(creators), sites, len(bad), len(deletePkgs))
}

// TestGuardCatchesARawCreate: the scanner itself flags a raw create, a raw Invoke and a create
// in a closure that is not Acquire's, and accepts the guarded forms.
func TestGuardCatchesARawCreate(t *testing.T) {
	creators := interfaceCreators(t)
	src := `package x

func raw(ctx context.Context, svc ipsec.RPCService) {
	svc.IpsecItfCreate(ctx, &ipsec.IpsecItfCreate{})                                  // bad: raw
	_ = c.Invoke(ctx, &wireguard.WireguardInterfaceCreate{}, &wireguard.WireguardInterfaceCreateReply{}) // bad: raw Invoke
	go func() { svc.TapCreateV3(ctx, req) }()                                         // bad: closure, not Acquire's
	other(ctx, func() (uint32, error) { svc.MemifCreateV2(ctx, req); return 0, nil }) // bad: not Acquire
	svc.MplsTunnelAddDel(ctx, &mpls.MplsTunnelAddDel{MtIsAdd: false})                 // ok: a delete
	svc.LcpItfPairAddDelV3(ctx, &lcp.LcpItfPairAddDelV3{IsAdd: true})                 // bad: an add
	ifsanitize.Acquire(ctx, c, o, n, func() (uint32, error) { svc.IpsecItfCreate(ctx, r); return 0, nil }, del) // ok
	iface.AcquireAndTag(ctx, c, o, n, func() (uint32, error) { svc.BondCreate2(ctx, r); return 0, nil }, del)   // ok
	svc.ClassifyTableByInterface(ctx, r)                                              // ok: a read
}

var spec = df6.IfSpec[*T, D]{
	Add: func(ctx context.Context, c vpp.Client) { svc.GreTunnelAddDelV2(ctx, r) }, // ok: df6 runs Add through ifsanitize
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "raw.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	v, seen := scanFile(fset, f, creators)
	var lines []int
	for _, x := range v {
		lines = append(lines, x.pos.Line)
	}
	if want := []int{4, 5, 6, 7, 9}; !slices.Equal(lines, want) {
		t.Fatalf("violations at lines %v, want %v: %v", lines, want, v)
	}
	if !seen[viaAcquire] || !seen[viaIfSpec] {
		t.Fatalf("guards seen %v", seen)
	}
	for _, x := range v {
		t.Logf("flagged: %s", x)
	}
}

func TestGuardPathsExist(t *testing.T) {
	for _, d := range []string{binapiDir, descriptorsDir} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("%s: %v", d, err)
		}
	}
}
