package images

import (
	"os"
	"os/exec"
	"testing"
)

// The repository quick gate discovers Go modules; run the host-independent image
// source suite here without requiring loop devices, root, cloud accounts or boot.
func TestImageSource(t *testing.T) {
	cmd := exec.Command("python3", "-m", "unittest", "discover", "-s", ".", "-p", "test_*.py", "-v")
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplianceOfflineAcceptance(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("image artifact inspection requires NGFW_INTEGRATION=1 and NGFW_IMAGE_ROOT")
	}
	root := os.Getenv("NGFW_IMAGE_ROOT")
	if root == "" {
		t.Skip("no prepared image root: offline build is recorded separately")
	}
	profile := os.Getenv("NGFW_IMAGE_PROFILE")
	if profile == "" {
		profile = "vm"
	}
	cmd := exec.Command("python3", "../../../deploy/image/vm/image.py", "inspect", "--root", root, "--profile", profile)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
}
