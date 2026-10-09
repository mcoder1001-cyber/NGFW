package sysident

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProductProvisioningGuard(t *testing.T) {
	root := t.TempDir()
	uid := uint32(os.Getuid()) //nolint:gosec // operating system uid is uint32 on Linux
	if err := verifyProductPaths(root, uid); err == nil {
		t.Fatal("missing provisioning accepted")
	}
	for _, path := range []string{"etc", "var/lib/ngfw-system-identity"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"hostname", "localtime", "issue", "issue.net", "motd"} {
		if err := os.Symlink("/var/lib/ngfw-system-identity/"+name, filepath.Join(root, "etc", name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyProductPaths(root, uid); err != nil {
		t.Fatal(err)
	}
	if err := verifyProductPaths(root, uid+1); err == nil {
		t.Fatal("foreign directory owner accepted")
	}
	if err := os.Remove(filepath.Join(root, "etc", "hostname")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "etc", "hostname")); err != nil {
		t.Fatal(err)
	}
	if err := verifyProductPaths(root, uid); err == nil {
		t.Fatal("redirected public link accepted")
	}
	if New(ProductPaths(), nil).provisioned == nil {
		t.Fatal("product writes lack provisioning guard")
	}
}
