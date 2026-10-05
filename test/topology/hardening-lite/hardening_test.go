package hardening_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOfflineControlsAndSignedMetadata(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "deploy/hardening/tests/run.sh"))
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
