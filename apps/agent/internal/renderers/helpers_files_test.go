package renderers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesValidate(t *testing.T) {
	good := Files{"/etc/frr/frr.conf": {Mode: 0o640, Content: []byte("!\n")}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Files{
		"relative":   {"etc/frr.conf": {Mode: 0o644}},
		"unclean":    {"/etc//frr.conf": {Mode: 0o644}},
		"dotdot":     {"/etc/../frr.conf": {Mode: 0o644}},
		"root":       {"/": {Mode: 0o644}},
		"no mode":    {"/etc/frr.conf": {}},
		"type bits":  {"/etc/frr.conf": {Mode: os.ModeDir | 0o644}},
		"secret o+r": {"/etc/swanctl/secrets": {Mode: 0o644, Secret: true}},
		"secret o+x": {"/etc/swanctl/secrets": {Mode: 0o641, Secret: true}},
	}
	for name, f := range bad {
		if err := f.Validate(); !errors.Is(err, ErrInvalidFiles) {
			t.Errorf("%s: got %v, want ErrInvalidFiles", name, err)
		}
	}
	secretOK := Files{"/etc/swanctl/secrets": {Mode: 0o640, Secret: true}}
	if err := secretOK.Validate(); err != nil {
		t.Fatalf("group-readable secret must be allowed: %v", err)
	}
}

func TestFilesPathsAndRedacted(t *testing.T) {
	f := Files{
		"/b": {Mode: 0o644, Content: []byte("b")},
		"/a": {Mode: 0o600, Content: []byte("psk"), Secret: true},
	}
	if p := f.Paths(); len(p) != 2 || p[0] != "/a" || p[1] != "/b" {
		t.Fatalf("Paths = %v", p)
	}
	r := f.Redacted()
	if string(r["/a"].Content) != "<redacted>" || string(r["/b"].Content) != "b" {
		t.Fatalf("Redacted = %v", r)
	}
	if string(f["/a"].Content) != "psk" {
		t.Fatal("Redacted must not modify the original")
	}
}

func TestWriteFileAtomicReplacesAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frr.conf")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil { //nolint:gosec // test fixture: mode is under test
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, File{Mode: 0o600, Content: []byte("new")}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path) //nolint:gosec // test-controlled path
	if string(got) != "new" {
		t.Fatalf("content = %q", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
	if err := WriteFileAtomic(path, File{Content: []byte("x")}); !errors.Is(err, ErrInvalidFiles) {
		t.Fatalf("missing mode: got %v", err)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "nodir", "x"), File{Mode: 0o644}); err == nil {
		t.Fatal("missing parent directory must fail")
	}
	if err := WriteFileAtomic(path, File{Mode: 0o644, Owner: "no-such-user-vrx"}); err == nil {
		t.Fatal("unknown owner must fail")
	}
	if got, _ := os.ReadFile(path); string(got) != "new" { //nolint:gosec // test-controlled path
		t.Fatal("failed write must leave the old file intact")
	}
}

func TestWriteFilesAndSnapshotRestore(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.conf")
	fresh := filepath.Join(dir, "fresh.conf")
	if err := os.WriteFile(existing, []byte("v1"), 0o640); err != nil { //nolint:gosec // test fixture: mode is under test
		t.Fatal(err)
	}
	snap, err := TakeSnapshot(existing, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if p := snap.Paths(); len(p) != 2 || p[0] != existing {
		t.Fatalf("snapshot paths = %v", p)
	}
	files := Files{
		existing: {Mode: 0o600, Content: []byte("v2")},
		fresh:    {Mode: 0o644, Content: []byte("brand new")},
	}
	if err := WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(fresh); string(got) != "brand new" { //nolint:gosec // test-controlled path
		t.Fatalf("fresh = %q", got)
	}
	if err := snap.Restore(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(existing) //nolint:gosec // test-controlled path
	if err != nil || string(got) != "v1" {
		t.Fatalf("restored content = %q, %v", got, err)
	}
	info, _ := os.Stat(existing)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode = %v, want 0640", info.Mode())
	}
	if _, err := os.Stat(fresh); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file that did not exist before must be removed by Restore")
	}
	if err := snap.Restore(); err != nil {
		t.Fatalf("Restore must be idempotent: %v", err)
	}
	if _, err := TakeSnapshot("relative/path"); !errors.Is(err, ErrInvalidFiles) {
		t.Fatalf("relative snapshot path: got %v", err)
	}
	if _, err := TakeSnapshot(dir); err == nil {
		t.Fatal("snapshot of a directory must fail")
	}
}

func TestStage(t *testing.T) {
	files := Files{
		"/etc/frr/frr.conf":    {Mode: 0o640, Owner: "no-such-user-vrx", Content: []byte("frr")},
		"/etc/frr/daemons":     {Mode: 0o644, Content: []byte("bgpd=yes\n")},
		"/etc/swanctl/secrets": {Mode: 0o600, Secret: true, Content: []byte("VRX_TEST_PSK_x")},
	}
	st, err := Stage(files)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	}()
	if !strings.HasPrefix(st.Dir, os.TempDir()) {
		t.Fatalf("staging dir %q not under temp", st.Dir)
	}
	p := st.Path("/etc/frr/frr.conf")
	if p != filepath.Join(st.Dir, "etc", "frr", "frr.conf") {
		t.Fatalf("Path = %q", p)
	}
	got, err := os.ReadFile(p) //nolint:gosec // test-controlled path
	if err != nil || string(got) != "frr" {
		t.Fatalf("staged content = %q, %v (owner must be ignored when staging)", got, err)
	}
	info, _ := os.Stat(st.Path("/etc/swanctl/secrets"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("staged secret mode = %v", info.Mode())
	}
	dirInfo, _ := os.Stat(st.Dir)
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("staging dir mode = %v, want 0700", dirInfo.Mode())
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Close must remove the staging tree")
	}
	if _, err := Stage(Files{"relative": {Mode: 0o644}}); !errors.Is(err, ErrInvalidFiles) {
		t.Fatalf("invalid files must be rejected before staging: %v", err)
	}
}

func TestLookupOwner(t *testing.T) {
	uid, gid, err := lookupOwner("root")
	if err != nil || uid != 0 || gid != 0 {
		t.Fatalf("root -> %d:%d, %v", uid, gid, err)
	}
	if uid, gid, err = lookupOwner("0:0"); err != nil || uid != 0 || gid != 0 {
		t.Fatalf("numeric 0:0 -> %d:%d, %v", uid, gid, err)
	}
	for _, bad := range []string{"", ":root", "root:", "no-such-user-vrx", "root:no-such-group-vrx"} {
		if _, _, err := lookupOwner(bad); err == nil {
			t.Errorf("lookupOwner(%q) succeeded", bad)
		}
	}
}
