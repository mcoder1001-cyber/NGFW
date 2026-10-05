package hastatesync

import (
	"os/exec"
	"testing"
)

func TestOfflineDriverSafety(t *testing.T) {
	cmd := exec.Command("python3", "-m", "unittest", "discover", "-s", ".", "-p", "test_*.py")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
