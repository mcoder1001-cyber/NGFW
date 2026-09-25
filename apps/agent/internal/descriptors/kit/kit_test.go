package kit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/scheduler"
)

func TestParsePrefix(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.0/24": "10.0.0.0/24", " 2001:db8::/32 ": "2001:db8::/32", "0.0.0.0/0": "0.0.0.0/0", "10.1.1.1/32": "10.1.1.1/32",
	} {
		p, err := ParsePrefix(in)
		if err != nil || p.String() != want {
			t.Errorf("ParsePrefix(%q) = %v, %v; want %s", in, p, err, want)
		}
	}
	for _, in := range []string{"10.0.0.1/24", "2001:db8::1/64", "::ffff:10.0.0.0/104", "fe80::/64%eth0", "junk", "10.0.0.0"} {
		if _, err := ParsePrefix(in); !errors.Is(err, ErrBadPrefix) {
			t.Errorf("ParsePrefix(%q) err = %v; want ErrBadPrefix", in, err)
		}
	}
}

func TestRetrieveUnsupported(t *testing.T) {
	err := RetrieveUnsupported("x")
	if !errors.Is(err, scheduler.ErrRetrieveUnsupported) || !scheduler.IsRetrieveUnsupported(err) {
		t.Fatalf("%v does not match the scheduler sentinel", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	p := filepath.Join(dir, "claims.json")
	for _, s := range []string{"one", "two"} {
		if err := WriteFileAtomic(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p) // #nosec G304 -- p is under t.TempDir()
		if err != nil || string(b) != s {
			t.Fatalf("read %q, %v; want %q", b, err, s)
		}
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", st.Mode().Perm())
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("temp files left: %v", ents)
	}
	// Rename onto a directory fails; the temp file must be cleaned up.
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "d"), []byte("x"), 0o600); err == nil {
		t.Fatal("want error")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 2 {
		t.Fatalf("temp file leaked: %v", ents)
	}
}

type rec struct{ n []string }

func (r *rec) Register(d scheduler.Descriptor) { r.n = append(r.n, fmt.Sprint(d == nil)) }

func TestRegister(t *testing.T) {
	r := &rec{}
	var owners []string
	c := func(e Env) scheduler.Descriptor { owners = append(owners, e.Owner); return nil }
	Register(r, Env{Owner: "o"}, c, c)
	if len(r.n) != 2 || owners[0] != "o" || owners[1] != "o" {
		t.Fatalf("%v %v", r.n, owners)
	}
}

func TestMaskPrefix(t *testing.T) {
	p, err := MaskPrefix(" 2001:db8:3::5/64")
	if err != nil || p.String() != "2001:db8:3::/64" {
		t.Fatalf("%v %v", p, err)
	}
	for _, in := range []string{"::ffff:10.0.0.0/104", "fe80::/64%eth0", "x"} {
		if _, err := MaskPrefix(in); !errors.Is(err, ErrBadPrefix) {
			t.Errorf("MaskPrefix(%q) = %v", in, err)
		}
	}
}
