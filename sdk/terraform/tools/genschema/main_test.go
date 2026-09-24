package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func spec(title string, props map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{
		"info": map[string]any{"title": title, "version": "1\n}\nfunc init() { panic(1) }\n//"},
		"components": map[string]any{"schemas": map[string]any{
			"RootConfig": map[string]any{"type": "object"},
			"InterfacesConfig": map[string]any{"type": "object", "additionalProperties": map[string]any{
				"type": "object", "properties": props}},
		}},
	})
	return b
}

// topLevel lists the top-level declarations of a generated file.
func topLevel(t *testing.T, src []byte) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "gen.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("generated code does not parse: %v", err)
	}
	var names []string
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			names = append(names, "func "+x.Name.Name)
		case *ast.GenDecl:
			for _, sp := range x.Specs {
				if v, ok := sp.(*ast.ValueSpec); ok {
					for _, n := range v.Names {
						names = append(names, "var "+n.Name)
					}
				}
			}
		}
	}
	return names
}

func TestHostileSpecCannotInjectCode(t *testing.T) {
	title := "VRX\n*/ func init() { panic(\"pwned\") } /*\n// "
	props := map[string]any{
		"description": map[string]any{"type": "string", "title": "x\" + panic(1) + \"", "x-vrx-ui": map[string]any{"help": "`\n}\nfunc evil() {}\n"}},
		"enabled":     map[string]any{"type": "boolean", "default": false},
	}
	iface, secrets, _, err := generate(spec(title, props))
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range [][]byte{iface, secrets} {
		for _, n := range topLevel(t, src) {
			if strings.Contains(n, "init") || strings.Contains(n, "evil") {
				t.Fatalf("injected declaration %q", n)
			}
		}
		if strings.Contains(strings.SplitN(string(src), "\n", 2)[0], "\n") {
			t.Fatal("header spans lines")
		}
	}
	got := strings.Join(topLevel(t, iface), ",")
	if got != "var _,var _,var _,var _,var _,var _,var _,func interfaceGeneratedAttributes,var interfaceJSONNames,var interfaceSensitiveAttributes" {
		t.Fatalf("unexpected declarations: %s", got)
	}
}

func TestHostilePropertyNameFailsGeneration(t *testing.T) {
	for _, key := range []string{"a\"}; func evil() {}; var _ = map[string]int{\"", "ü-x", "x y", "9lives"} {
		_, _, _, err := generate(spec("VRX", map[string]any{key: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}}}))
		if err == nil {
			t.Errorf("property %q accepted", key)
		}
	}
}
