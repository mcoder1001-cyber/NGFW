package ownertable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilePersists(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "w7")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ip.route/7001/10.7.0.0/24", "ip.route/0/10.7.9.0/24", "other/x"} {
		if err := f.Add(k); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Remove("other/x"); err != nil {
		t.Fatal(err)
	}
	g, err := Open(dir, "w7")
	if err != nil {
		t.Fatal(err)
	}
	got := g.Keys("ip.route/")
	if len(got) != 2 || got[0] != "ip.route/0/10.7.9.0/24" || !g.Has("ip.route/7001/10.7.0.0/24") || g.Has("other/x") {
		t.Fatalf("reloaded keys %v", got)
	}
	// Another owner has its own file.
	h, err := Open(dir, "w7b")
	if err != nil || len(h.Keys("")) != 0 {
		t.Fatalf("w7b: %v %v", err, h.Keys(""))
	}
	// A file of another owner under our name is rejected.
	if err := os.WriteFile(filepath.Join(dir, "owned-x.json"), []byte(`{"owner":"y","keys":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, "x"); err == nil {
		t.Fatal("owner mismatch accepted")
	}
	if _, err := Open(dir, "a/b"); err == nil {
		t.Fatal("invalid owner accepted")
	}
}

func TestMemory(t *testing.T) {
	m := NewMemory()
	_ = m.Add("a/1")
	_ = m.Add("b/1")
	if !m.Has("a/1") || len(m.Keys("a/")) != 1 {
		t.Fatal("memory set")
	}
	_ = m.Remove("a/1")
	if m.Has("a/1") {
		t.Fatal("remove")
	}
}
