package basepolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigNoShellOrDuplicateAssignments(t *testing.T) {
	valid := "NGFW_BOOTSTRAP_MGMT_IF=mgmt0\nNGFW_BOOTSTRAP_PUNT_IFS=tap2,tap1\n"
	got, err := ParseConfig([]byte(valid))
	if err != nil || got.Management != "mgmt0" || strings.Join(got.Permanent, ",") != "tap1,tap2" {
		t.Fatalf("%v %v", got, err)
	}
	for _, data := range []string{valid + "NGFW_BOOTSTRAP_MGMT_IF=\n", strings.Replace(valid, "mgmt0", "$(touch /tmp/x)", 1), strings.Replace(valid, "tap2,tap1", "mgmt0", 1), strings.Replace(valid, "tap2,tap1", "\"tap1\"", 1), "NGFW_BOOTSTRAP_MGMT_IF=mgmt0\n", valid + "OTHER=1\n", strings.Repeat("x", maxConfig+1)} {
		if _, err := ParseConfig([]byte(data)); err == nil {
			t.Fatal("accepted unsafe config")
		}
	}
}
func TestConfigRejectsSymlinkAndNonRegular(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "env")
	if err := os.WriteFile(regular, []byte("NGFW_BOOTSTRAP_MGMT_IF=mgmt0\nNGFW_BOOTSTRAP_PUNT_IFS=\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(link); err == nil {
		t.Fatal("followed symlink")
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("accepted directory")
	}
	// Deliberately add public-read bits to this isolated nonsecret fixture.
	info, err := os.Stat(regular)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(regular, info.Mode().Perm()|0044); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(regular); err == nil {
		t.Fatal("accepted world-readable config")
	}
}
