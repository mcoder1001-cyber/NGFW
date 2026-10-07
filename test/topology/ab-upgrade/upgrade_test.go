package upgrade_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSignedBundleAndLifecycle(t *testing.T) {
	cmd := exec.Command(filepath.Join(repository(t), "deploy/upgrade/tests/run.sh"))
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealthProbe(t *testing.T) {
	cmd := exec.Command("go", "test", "-race", "-count=1", "./...")
	cmd.Dir = filepath.Join(repository(t), "deploy/upgrade/probe")
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoopDiskImage(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires explicit NGFW_INTEGRATION=1 and root loop/mount capabilities")
	}
	cmd := exec.Command("python3", filepath.Join(repository(t), "deploy/upgrade/tests/loop_image.py"))
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
