package secretchannel

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const fixture = "NGFW_TEST_PSK_NATIVE_CHANNEL"

func TestSealedRestartAndRotation(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, "w8")
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.Stage(map[string][]byte{"psk/site": []byte(fixture)})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Activate(old); e != nil {
		t.Fatal(e)
	}
	fingerprint, e := s.Ref(context.Background(), "psk/site")
	if e != nil {
		t.Fatal(e)
	}
	newer, e := s.Stage(map[string][]byte{"psk/site": []byte(fixture + "_ROTATED")})
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Activate(newer)
	resolved, e := s.Resolve(context.Background(), fingerprint)
	if e != nil || string(resolved) != fixture {
		t.Fatal("historical fingerprint unavailable for rollback", e)
	}
	//nolint:gosec // Private test directory; deliberate tampering verifies rejection of unsafe cache files.
	raw, e := os.ReadFile(filepath.Join(dir, "secret-cache-w8.sealed"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte(fixture)) {
		t.Fatal("plaintext on disk")
	}
	if bytes.Contains([]byte(fmt.Sprintf("%v %+v %#v", s, s, s)), []byte(fixture)) {
		t.Fatal("formatter leaked material")
	}
	restored, e := Open(dir, "w8")
	if e != nil {
		t.Fatal(e)
	}
	if e = restored.Activate(old); e != nil {
		t.Fatal(e)
	}
	got, _ := restored.Ref(context.Background(), "psk/site")
	if got != fingerprint {
		t.Fatal("restart did not preserve confirmed key")
	}
	for _, name := range []string{"secret-cache-w8.key", "secret-cache-w8.sealed"} {
		fi, e := os.Stat(filepath.Join(dir, name))
		if e != nil || fi.Mode().Perm() != 0600 {
			t.Fatal("private file mode", name, e)
		}
	}
}
func TestDryRunSnapshotNeverSealed(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, "w8")
	id, e := s.Transient(map[string][]byte{"psk/site": []byte(fixture)})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Stage(map[string][]byte{"psk/other": []byte(fixture + "_OTHER")})
	if e != nil {
		t.Fatal(e)
	}
	restarted, e := Open(dir, "w8")
	if e != nil {
		t.Fatal(e)
	}
	if restarted.Activate(id) == nil {
		t.Fatal("validation-only snapshot persisted")
	}
	s.DiscardTransient(id)
	if s.Activate(id) == nil {
		t.Fatal("validation-only snapshot retained")
	}
}
func TestCacheTamperingAndMissingKeyFailClosed(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, "w8")
	_, e := s.Stage(map[string][]byte{"psk/site": []byte(fixture)})
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "secret-cache-w8.sealed")
	//nolint:gosec // Private test directory: read the sealed cache for the deliberate tampering regression.
	raw, _ := os.ReadFile(path)
	raw[len(raw)-1] ^= 1
	//nolint:gosec // Private test directory; deliberate tampering verifies rejection of unsafe cache files.
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(dir, "w8"); e == nil {
		t.Fatal("tampered cache accepted")
	}
	if e = os.Remove(filepath.Join(dir, "secret-cache-w8.key")); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(dir, "w8"); e == nil {
		t.Fatal("missing key silently replaced")
	}
}
func TestBundleLimitsAndPrivateFileSafety(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, "w8")
	for _, m := range []map[string][]byte{{"plaintext": []byte(fixture)}, {"psk/.hidden": []byte(fixture)}, {"psk/_leading": []byte(fixture)}, {"psk/" + string(bytes.Repeat([]byte("a"), 64)): []byte(fixture)}, {"psk/site": {}}, {"psk/site": make([]byte, maxValue+1)}} {
		if _, e := s.Stage(m); e == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
	path := filepath.Join(dir, "secret-cache-w8.key")
	//nolint:gosec // Private test directory; deliberate tampering verifies rejection of unsafe cache files.
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "w8"); e == nil {
		t.Fatal("public cache key accepted")
	}
}

func TestEmptyBundleIdentitySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, "w8")
	if e != nil {
		t.Fatal(e)
	}
	id, e := s.Stage(nil)
	if e != nil {
		t.Fatal(e)
	}
	empty, e := s.ID(map[string][]byte{})
	if e != nil || id != empty {
		t.Fatal("nil and empty secret bundles differ")
	}
	again, e := Open(dir, "w8")
	if e != nil {
		t.Fatal(e)
	}
	if e = again.Activate(id); e != nil {
		t.Fatal(e)
	}
}

func TestRetainCurrentConfirmedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "w8")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 3)
	for i, suffix := range []string{"OLD", "CONFIRMED", "CURRENT"} {
		ids[i], err = s.Stage(map[string][]byte{"psk/site": []byte(fixture + suffix)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Activate(ids[2]); err != nil {
		t.Fatal(err)
	}
	if err = s.Retain(ids[1], ids[2]); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, "w8")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Activate(ids[0]) == nil {
		t.Fatal("obsolete snapshot persisted")
	}
	for _, id := range ids[1:] {
		if err = reopened.Activate(id); err != nil {
			t.Fatal("rollback snapshot unavailable", err)
		}
	}
}
