package vppstartup

import (
	"slices"
	"strings"
	"testing"
)

func TestParseCanonical(t *testing.T) {
	a := []byte(`# comment
unix {
  nodaemon   # trailing comment
  log /var/log/vpp/vpp.log
}
cpu {
	## only comments
}
plugins { plugin a_plugin.so { enable } }
api-trace on
`)
	p, err := Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"api-trace on",
		"cpu {}",
		"plugins > plugin a_plugin.so > enable",
		"plugins > plugin a_plugin.so {}",
		"plugins {}",
		"unix > log /var/log/vpp/vpp.log",
		"unix > nodaemon",
		"unix {}",
	}
	if got := p.Canonical(); !slices.Equal(got, want) {
		t.Fatalf("canonical =\n%q\nwant\n%q", got, want)
	}
	// VPP drops everything from any '#' (also mid-token) to the end of the line (F8)
	q, err := Parse([]byte("dpdk {\n  no-pci#x\n}\n"))
	if err != nil || !slices.Equal(q.Canonical(), []string{"dpdk > no-pci", "dpdk {}"}) {
		t.Fatalf("%v %v", q, err)
	}
	for _, bad := range []string{"unix {\n", "}\n", "{ x }\n", "a { b { c }\n", "dpdk { # }\n}\n}\n"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestSemanticDiffIgnoresLayout(t *testing.T) {
	a := []byte("dpdk {\n  blacklist 0000:0b:00.0\n  no-pci\n}\nunix { nodaemon }\n")
	b := []byte("# reordered\nunix {\n\tnodaemon\n}\n\ndpdk {\n  no-pci   # c\n  blacklist 0000:0b:00.0\n}\n")
	onlyA, onlyB, err := SemanticDiff(a, b)
	if err != nil || len(onlyA)+len(onlyB) != 0 {
		t.Fatalf("%q %q %v", onlyA, onlyB, err)
	}
	c := []byte("dpdk {\n  blacklist 0000:0b:00.0\n  blacklist 0000:0b:00.0\n}\n")
	d := []byte("dpdk {\n  blacklist 0000:0b:00.0\n  dev 0000:04:00.0 { name wan }\n}\n")
	onlyA, onlyB, err = SemanticDiff(c, d)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(onlyA, []string{"dpdk > blacklist 0000:0b:00.0"}) ||
		!slices.Equal(onlyB, []string{"dpdk > dev 0000:04:00.0 > name wan", "dpdk > dev 0000:04:00.0 {}"}) {
		t.Fatalf("%q %q", onlyA, onlyB)
	}
	if _, _, err := SemanticDiff([]byte("{"), d); err == nil {
		t.Error("broken first file accepted")
	}
	if _, _, err := SemanticDiff(d, []byte("}")); err == nil {
		t.Error("broken second file accepted")
	}
}

func TestUnifiedDiff(t *testing.T) {
	if d, err := UnifiedDiff("a", "b", []byte("x\n"), []byte("x\n")); d != "" || err != nil {
		t.Fatalf("%q %v", d, err)
	}
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n"
	b := "1\n2\n3\nfour\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\nsixteen"
	got, err := UnifiedDiff("old", "new", []byte(a), []byte(b))
	if err != nil {
		t.Fatal(err)
	}
	want := `--- old
+++ new
@@ -1,7 +1,7 @@
 1
 2
 3
-4
+four
 5
 6
 7
@@ -13,3 +13,4 @@
 13
 14
 15
+sixteen
\ No newline at end of file
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// insertion into an empty file, deletion to empty
	got, _ = UnifiedDiff("old", "new", nil, []byte("a\nb\n"))
	if !strings.Contains(got, "@@ -0,0 +1,2 @@\n+a\n+b\n") {
		t.Fatalf("%q", got)
	}
	got, _ = UnifiedDiff("old", "new", []byte("a\n"), nil)
	if !strings.Contains(got, "@@ -1 +0,0 @@\n-a\n") {
		t.Fatalf("%q", got)
	}
	big := []byte(strings.Repeat("x\n", 5000))
	if _, err := UnifiedDiff("a", "b", big, append(big, 'y')); err != ErrDiffTooLarge {
		t.Fatalf("large input: %v", err)
	}
}
