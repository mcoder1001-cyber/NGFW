package dns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The D-137 guard (F-unbound-chrony-syslog review L7, D-128 style): dns_resolve_name / dns_resolve_ip crash VPP 26.06
// unless an IPv4 name server was added since VPP started, so the agent sends them only through ResolveName /
// ResolveIP in this package's dns.go, which refuse without Ready. A later feature that calls the binapi directly
// (the service method, or the request type for a raw Invoke) would bypass that precondition.
//
// Every non-test Go file under apps/agent/ is scanned except the generated binapi tree (it defines the messages and
// sends nothing). A violation is
//   - the identifier DNSResolveName or DNSResolveIP (a call, a composite literal, new(...)) outside dns.go of this
//     package — the reply types (DNSResolveNameReply, …) are other identifiers and stay allowed;
//   - a string literal that looks like the CLI `show dns servers` (unique prefixes included): with IPv6-only servers
//     it crashes the same way (dns.c:2244-2246).

const agentRoot = "../../.." // apps/agent

var (
	resolveIdents = map[string]bool{"DNSResolveName": true, "DNSResolveIP": true}
	cliShowDNS    = regexp.MustCompile(`(?i)\bsh\w*\s+dns\s+ser`)
)

type resolveViolation struct {
	pos token.Position
	msg string
}

func (v resolveViolation) String() string { return v.pos.String() + ": " + v.msg }

// allowedResolveFile is the one file that may send the resolve messages (relative to root, slash-separated).
const allowedResolveFile = "internal/descriptors/dns/dns.go"

func scanResolveCallers(root string) ([]resolveViolation, int, error) {
	var out []resolveViolation
	allowed := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
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
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if resolveIdents[x.Name] {
					if rel == allowedResolveFile {
						allowed++
					} else {
						out = append(out, resolveViolation{fset.Position(x.Pos()), x.Name + " outside " + allowedResolveFile + " (use dns.ResolveName / ResolveIP with Ready, D-137)"})
					}
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					if s, err := strconv.Unquote(x.Value); err == nil && cliShowDNS.MatchString(s) {
						out = append(out, resolveViolation{fset.Position(x.Pos()), "CLI `show dns servers` (crashes VPP 26.06 with IPv6-only servers, D-137)"})
					}
				}
			}
			return true
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, allowed, err
}

func TestResolveMessagesOnlyThroughTheGuardedHelpers(t *testing.T) {
	v, allowed, err := scanResolveCallers(agentRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range v {
		t.Errorf("%s", x)
	}
	if allowed == 0 {
		t.Fatalf("no DNSResolveName/DNSResolveIP found in %s: the scan looks at the wrong tree", allowedResolveFile)
	}
}

// The scanner itself: a planted direct call, a raw request literal and the CLI form are all found; the helper file
// and the reply types are not flagged.
func TestResolveGuardFindsDirectCalls(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(allowedResolveFile, `package dns
func f(c interface{ DNSResolveName() }) { c.DNSResolveName() }
`)
	write("internal/feature/x.go", `package feature
type DNSResolveIP struct{}
type DNSResolveNameReply struct{}
func g(c interface{ DNSResolveName() }) { c.DNSResolveName(); _ = &DNSResolveIP{}; _ = DNSResolveNameReply{} }
const cli = "sh dns servers"
`)
	write("internal/feature/x_test.go", `package feature
func h(c interface{ DNSResolveName() }) { c.DNSResolveName() }
`)
	write("binapi/dns/dns.ba.go", `package dns
type DNSResolveName struct{}
`)
	v, allowed, err := scanResolveCallers(root)
	if err != nil {
		t.Fatal(err)
	}
	if allowed != 2 {
		t.Errorf("allowed sites = %d, want 2", allowed)
	}
	var got []string
	for _, x := range v {
		got = append(got, filepath.Base(x.pos.Filename)+":"+strconv.Itoa(x.pos.Line))
	}
	// x.go line 2 (type DNSResolveIP), line 4 (interface method, call, literal: 3), line 5 (CLI)
	want := []string{"x.go:2", "x.go:4", "x.go:4", "x.go:4", "x.go:5"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("violations %v, want %v", v, want)
	}
}
