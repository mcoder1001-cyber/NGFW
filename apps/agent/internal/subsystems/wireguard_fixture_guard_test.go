package subsystems

// Review F2: the file secret channel (wireguard_fixture.go) exists only in test builds. These guards fail if the hook's
// init ever moves out of the tagged file, or the tag is dropped from it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Static and untagged (valid in any build): only files constrained by the test-secrets build tag may set the hook.
func TestWireguardFixtureHookOnlyInTaggedFile(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	const tag = "//go:build vrxtestsecrets"
	fset := token.NewFileSet()
	var setters []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // G304: this package's own sources
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok {
				for _, l := range as.Lhs {
					if id, ok := l.(*ast.Ident); ok && id.Name == "wireguardFixture" {
						setters = append(setters, f)
						if !strings.HasPrefix(string(src), tag+"\n") {
							t.Errorf("%s sets wireguardFixture without the %q constraint: the file secret channel would reach product builds", f, tag)
						}
					}
				}
			}
			return true
		})
	}
	if len(setters) != 1 || setters[0] != "wireguard_fixture.go" {
		t.Fatalf("wireguardFixture must be set only by wireguard_fixture.go, set by %v", setters)
	}
}
