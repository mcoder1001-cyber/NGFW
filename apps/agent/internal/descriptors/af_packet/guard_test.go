package afpacket_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The V24 guard (D-101, TD-5): the agent sends af_packet_delete only through the quiesce helper
// (*HostInterfaceDescriptor).quiescedDelete, which brings the Linux netdev down first. Delete and
// Create's rollback used to be two separate raw sends (TD-3 re-review L7); a third must not appear.
//
// Every non-test Go file under apps/agent/ is scanned (test-helper packages included: they are
// not _test.go files), except the generated binapi tree, which defines the message and sends
// nothing. A violation is
//   - any use of the identifier AfPacketDelete (the service method or the request type: a call,
//     a composite literal for a raw Invoke, new(...)) outside quiescedDelete;
//   - inside quiescedDelete, an AfPacketDelete that no checked quiesce precedes: a top-level
//     `if err := d.quiesce(…); err != nil { …; return … }`, or `…, err := d.quiesce(…)` directly
//     followed by such an if (review N2: `_ = d.quiesce(…)` does not count);
//   - a string literal that matches cliDelete — the vppctl / cli_inband form, including the unique
//     prefixes VPP's CLI accepts (`del host-int …`, review N1).
//
// Not a violation: AfPacketDelete as the asserted type of a type assertion (`m.(*afpapi.AfPacketDelete)`,
// e.g. a fake VPP decoding a received request, D-118). An assertion only reads a message it was
// handed; sending one still needs the service method or a request value, which stay flagged.

const agentRoot = "../../.." // apps/agent

const (
	helperName = "quiescedDelete"
	helperPkg  = "afpacket"
	helperRecv = "HostInterfaceDescriptor"
	msgIdent   = "AfPacketDelete"
)

// cliDelete matches `delete host-interface` and its abbreviations in a string literal.
var cliDelete = regexp.MustCompile(`(?i)\bdel\w*\s+host-int`)

type v24Violation struct {
	pos token.Position
	msg string
}

func (v v24Violation) String() string { return v.pos.String() + ": " + v.msg }

// scanV24 scans every non-test, non-generated Go file under root. It returns the violations and
// the number of permitted sends (AfPacketDelete inside quiescedDelete, after quiesce).
func scanV24(root string) ([]v24Violation, int, error) {
	var out []v24Violation
	sites := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == "binapi" || rel == "bin" || strings.HasPrefix(d.Name(), ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		v, n := scanV24File(fset, f)
		out = append(out, v...)
		sites += n
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].pos.Filename != out[j].pos.Filename {
			return out[i].pos.Filename < out[j].pos.Filename
		}
		return out[i].pos.Line < out[j].pos.Line
	})
	return out, sites, err
}

func scanV24File(fset *token.FileSet, f *ast.File) ([]v24Violation, int) {
	var out []v24Violation
	sites := 0
	seen := map[int]bool{} // one violation per line
	flag := func(p token.Pos, msg string) {
		pos := fset.Position(p)
		if !seen[pos.Line] {
			seen[pos.Line] = true
			out = append(out, v24Violation{pos, msg})
		}
	}
	var helpers []*ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && isHelper(f, fd) {
			helpers = append(helpers, fd)
		}
	}
	inHelper := func(p token.Pos) *ast.FuncDecl {
		for _, h := range helpers {
			if h.Body != nil && p >= h.Body.Pos() && p < h.Body.End() {
				return h
			}
		}
		return nil
	}
	asserted := map[*ast.Ident]bool{} // AfPacketDelete named as a type assertion's type (D-118)
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeAssertExpr: // visited before its children
			if id := assertedIdent(x.Type); id != nil {
				asserted[id] = true
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				if s, err := strconv.Unquote(x.Value); err == nil && cliDelete.MatchString(s) {
					flag(x.Pos(), "\"delete host-interface\" CLI string (or an abbreviation): af_packet deletes go through (*HostInterfaceDescriptor).quiescedDelete (D-101, VPP V24)")
				}
			}
		case *ast.Ident:
			if x.Name != msgIdent || asserted[x] {
				return true
			}
			h := inHelper(x.Pos())
			if h == nil {
				flag(x.Pos(), "af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)")
				return true
			}
			q := checkedQuiesce(h.Body)
			if !q.IsValid() || q > x.Pos() {
				flag(x.Pos(), "af_packet_delete without a checked quiesce before it in quiescedDelete (`if err := d.quiesce(…); err != nil { return … }`, D-101, VPP V24)")
				return true
			}
			if !seen[-fset.Position(x.Pos()).Line] {
				seen[-fset.Position(x.Pos()).Line] = true
				sites++
			}
		}
		return true
	})
	return out, sites
}

// assertedIdent returns the type name of a type assertion's asserted type — T, *T, pkg.T or *pkg.T
// (parenthesised or not) — or nil (`.(type)` in a type switch, or any other type expression).
func assertedIdent(t ast.Expr) *ast.Ident {
	for {
		switch x := t.(type) {
		case *ast.ParenExpr:
			t = x.X
		case *ast.StarExpr:
			t = x.X
		case *ast.SelectorExpr:
			return x.Sel
		case *ast.Ident:
			return x
		default:
			return nil
		}
	}
}

// isHelper reports whether fd is (*HostInterfaceDescriptor).quiescedDelete in package afpacket.
func isHelper(f *ast.File, fd *ast.FuncDecl) bool {
	if f.Name.Name != helperPkg || fd.Name.Name != helperName || fd.Recv == nil || len(fd.Recv.List) != 1 {
		return false
	}
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	id, ok := t.(*ast.Ident)
	return ok && id.Name == helperRecv
}

// checkedQuiesce returns the end of the first top-level statement of body that calls quiesce and
// returns when its error is not nil — `if err := d.quiesce(…); err != nil { …; return … }`, or
// `…, err := d.quiesce(…)` directly followed by `if err != nil { …; return … }` — or NoPos.
func checkedQuiesce(body *ast.BlockStmt) token.Pos {
	for i, st := range body.List {
		switch s := st.(type) {
		case *ast.IfStmt:
			if as, ok := s.Init.(*ast.AssignStmt); ok && returnsOnErr(s, quiesceErr(as)) {
				return s.End()
			}
		case *ast.AssignStmt:
			if i+1 < len(body.List) {
				if next, ok := body.List[i+1].(*ast.IfStmt); ok && next.Init == nil && returnsOnErr(next, quiesceErr(s)) {
					return next.End()
				}
			}
		}
	}
	return token.NoPos
}

// quiesceErr returns the name the error result of `…, err := x.quiesce(…)` is bound to ("" when
// as is not such an assignment, or the error is discarded).
func quiesceErr(as *ast.AssignStmt) string {
	if len(as.Rhs) != 1 || len(as.Lhs) == 0 {
		return ""
	}
	c, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	name := ""
	switch fn := c.Fun.(type) {
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	case *ast.Ident:
		name = fn.Name
	}
	id, ok := as.Lhs[len(as.Lhs)-1].(*ast.Ident)
	if name != "quiesce" || !ok || id.Name == "_" {
		return ""
	}
	return id.Name
}

// returnsOnErr reports whether s is `if <errName> != nil { …; return … }`.
func returnsOnErr(s *ast.IfStmt, errName string) bool {
	if errName == "" || s.Body == nil || len(s.Body.List) == 0 {
		return false
	}
	cond, ok := s.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ {
		return false
	}
	x, ok := cond.X.(*ast.Ident)
	y, ok2 := cond.Y.(*ast.Ident)
	if !ok || !ok2 || x.Name != errName || y.Name != "nil" {
		return false
	}
	_, ret := s.Body.List[len(s.Body.List)-1].(*ast.ReturnStmt)
	return ret
}

// TestEveryAfPacketDeleteIsQuiesced is the guard over the real apps/agent tree.
func TestEveryAfPacketDeleteIsQuiesced(t *testing.T) {
	if st, err := os.Stat(filepath.Join(agentRoot, "binapi", "af_packet")); err != nil || !st.IsDir() {
		t.Fatalf("%s is not apps/agent: %v", agentRoot, err)
	}
	v, sites, err := scanV24(agentRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range v {
		t.Error(x)
	}
	if sites != 1 {
		t.Fatalf("%d quiesced af_packet_delete sends found, want exactly 1 (quiescedDelete): the scan is not looking at the tree", sites)
	}
	t.Logf("apps/agent: %d violations; af_packet_delete is sent at %d site (quiescedDelete, after quiesce)", len(v), sites)
}

// TestGuardCatchesARawAfPacketDelete plants raw af_packet deletes in a temporary tree and checks
// that the scan flags each one and accepts the quiesced helper, test files and generated binapi.
func TestGuardCatchesARawAfPacketDelete(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		// the permitted forms: the if-init check, and the product's two-result form
		"internal/descriptors/af_packet/good.go": `package afpacket

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string, between func() error) error {
	if err := d.quiesce(ctx, dev); err != nil {
		return err
	}
	_, err := d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: dev})
	return err
}
`,
		"internal/descriptors/af_packet/good2.go": `package afpacket

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string, between func() error) error {
	downed, err := d.quiesce(ctx, dev)
	if err != nil {
		d.restore(dev, downed, err)
		return err
	}
	_, err = d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: dev})
	return err
}
`,
		// review N2: the quiesce error discarded, or checked without returning
		"internal/descriptors/af_packet/unchecked.go": `package afpacket

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string) error {
	_, _ = d.quiesce(ctx, dev)
	_, err := d.svc().AfPacketDelete(ctx, nil)
	return err
}
`,
		"internal/descriptors/af_packet/no_return.go": `package afpacket

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string) error {
	if _, err := d.quiesce(ctx, dev); err != nil {
		log(err)
	}
	_, err := d.svc().AfPacketDelete(ctx, nil)
	return err
}
`,
		// review N1: VPP's CLI accepts unique prefixes, in any case
		"internal/descriptors/x/cli.go": `package x

var (
	a = "del host-int name w2-l0"
	b = "DELETE Host-Interface name w2-l0"
	c = "show host-interface"
)
`,
		// a helper that deletes before it quiesces
		"internal/descriptors/af_packet/bad_order.go": `package afpacket

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string) error {
	_, _ = d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: dev})
	return d.quiesce(ctx, dev)
}
`,
		// raw sends in product code
		"internal/descriptors/x/raw.go": `package x

func raw(ctx context.Context, c vpp.Client, svc afpapi.RPCService) {
	svc.AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: "w2-l0"})
	_ = c.Invoke(ctx, &afpapi.AfPacketDelete{HostIfName: "w2-l0"}, &afpapi.AfPacketDeleteReply{})
	req := new(afpapi.AfPacketDelete)
	_ = vppctl("delete host-interface name w2-l0")
	svc.AfPacketCreateV3(ctx, nil)
}

func (d *HostInterfaceDescriptor) quiescedDelete(ctx context.Context, dev string) error {
	d.quiesce(ctx, dev)
	return d.svc().AfPacketDelete(ctx, nil)
}
`,
		// a test-helper package is not a _test.go file: scanned
		"internal/descriptors/x/xtest/fixture.go": `package xtest

func Lose(ctx context.Context, svc afpapi.RPCService) { svc.AfPacketDelete(ctx, nil) }
`,
		// D-118: a fake VPP decoding a received request (type assertion) sends nothing — not flagged;
		// a raw send of the decoded request and a type-switch case still are
		"internal/descriptors/x/xtest/fakevpp.go": `package xtest

func (v *VPP) install() {
	v.On("af_packet_delete", func(m api.Message) ([]api.Message, error) {
		req := m.(*afpapi.AfPacketDelete)
		_ = m.((*AfPacketDelete))
		return v.drop(req.HostIfName)
	})
}

func (v *VPP) resend(ctx context.Context, svc afpapi.RPCService, m api.Message) {
	svc.AfPacketDelete(ctx, m.(*afpapi.AfPacketDelete))
	switch m.(type) {
	case *afpapi.AfPacketDelete:
	}
}
`,
		// not scanned: tests and generated bindings
		"internal/descriptors/x/raw_test.go":   "package x\n\nfunc TestX() { svc.AfPacketDelete(ctx, nil) }\n",
		"binapi/af_packet/af_packet.ba.go":     "package af_packet\n\ntype AfPacketDelete struct{}\n\nfunc (m *AfPacketDelete) Reset() { *m = AfPacketDelete{} }\n",
		"binapi/af_packet/af_packet_rpc.ba.go": "package af_packet\n\nfunc (c *serviceClient) AfPacketDelete(ctx context.Context, in *AfPacketDelete) {}\n",
	}
	for name, src := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	v, sites, err := scanV24(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range v {
		rel, _ := filepath.Rel(root, x.pos.Filename)
		got = append(got, rel+":"+strconv.Itoa(x.pos.Line))
		t.Logf("flagged: %s:%d: %s", rel, x.pos.Line, x.msg)
	}
	want := []string{
		"internal/descriptors/af_packet/bad_order.go:4", // AfPacketDelete before quiesce
		"internal/descriptors/af_packet/no_return.go:7", // the quiesce error checked, not returned (N2)
		"internal/descriptors/af_packet/unchecked.go:5", // the quiesce error discarded (N2)
		"internal/descriptors/x/cli.go:4",               // "del host-int" (N1)
		"internal/descriptors/x/cli.go:5",               // "DELETE Host-Interface" (N1)
		"internal/descriptors/x/raw.go:4",               // raw service call
		"internal/descriptors/x/raw.go:5",               // raw Invoke of a request literal
		"internal/descriptors/x/raw.go:6",               // new(AfPacketDelete)
		"internal/descriptors/x/raw.go:7",               // vppctl "delete host-interface"
		"internal/descriptors/x/raw.go:13",              // a quiescedDelete outside package afpacket
		"internal/descriptors/x/xtest/fakevpp.go:12",    // a raw send of a decoded request (the assertion itself is not flagged, D-118)
		"internal/descriptors/x/xtest/fakevpp.go:14",    // a type-switch case is not a type assertion
		"internal/descriptors/x/xtest/fixture.go:3",     // a test-helper package
	}
	if !slices.Equal(got, want) {
		t.Fatalf("flagged %v, want %v", got, want)
	}
	if sites != 2 {
		t.Fatalf("permitted sites %d, want 2 (good.go, good2.go)", sites)
	}
}
