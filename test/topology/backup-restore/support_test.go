package backuprestore_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSupportCollectorAndFixedUpgradeInstances(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", filepath.Join(root, "deploy/support-bundle/test_collect.py"))
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
