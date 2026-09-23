package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// TestProjectSchemaExamples projects every valid example document of packages/schema: no panic,
// no ERROR issue, and assemble(project(doc)) round-trips the implemented domains of the document
// modulo the leaves the core descriptors cannot represent.
func TestProjectSchemaExamples(t *testing.T) {
	files, err := filepath.Glob("../../../../packages/schema/examples/*.json")
	if err != nil || len(files) == 0 {
		t.Skipf("no schema examples: %v", err)
	}
	lenient := protojson.UnmarshalOptions{DiscardUnknown: true} // secret leaves are stripped by the API
	n := 0
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasPrefix(base, "invalid-") || strings.Contains(base, "-invalid-") || strings.Contains(base, "semantic") {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // test corpus
		if err != nil {
			t.Fatal(err)
		}
		ds := &vrxv1.DesiredState{}
		if err := lenient.Unmarshal(b, ds); err != nil {
			t.Logf("%s: not a DesiredState (%v) — skipped", base, err)
			continue
		}
		n++
		pj := project(ds, implementedDomains(), nil)
		for _, is := range pj.issues {
			if is.severity == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
				t.Errorf("%s: %s %s: %s", base, is.pointer, is.rule, is.message)
			}
		}
		// Round trip of what the core descriptors represent.
		out := assemble(pj.kvs, implementedDomains(), nil)
		for _, kv := range pj.kvs {
			if kv.Key == "" || kv.Value == nil {
				t.Fatalf("%s: empty kv", base)
			}
		}
		again := project(out, implementedDomains(), nil)
		if !sameKVs(pj.kvs, again.kvs) {
			t.Errorf("%s: project(assemble(project(doc))) != project(doc)", base)
		}
	}
	t.Logf("projected %d example documents", n)
}

func sameKVs(a, b []scheduler.KV) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[scheduler.Key]proto.Message{}
	for _, kv := range a {
		m[kv.Key] = kv.Value
	}
	for _, kv := range b {
		if !proto.Equal(m[kv.Key], kv.Value) {
			return false
		}
	}
	return true
}
