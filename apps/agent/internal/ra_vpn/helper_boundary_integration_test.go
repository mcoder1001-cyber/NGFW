package ravpn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestHelperReadBoundaryProbe(t *testing.T) {
	instance := os.Getenv("NGFW_RA_BOUNDARY_PROBE")
	if instance == "" {
		t.Skip("subprocess probe only")
	}
	_, err := ReadPrivatePlan(instance)
	wantPrivate := os.Getenv("NGFW_RA_BOUNDARY_EXPECT") == "private"
	if (err == nil) != wantPrivate {
		t.Fatal("actual low-cap namespace boundary mismatch")
	}
}

func TestIntegrationLowCapabilityHelperRequiresActualPrivateNamespace(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires own disposable namespace")
	}
	plan := networkFixture()
	plan.Owner = "w19-lowcap"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	if err := CreateNamespace(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(InstanceRoot, plan.Instance)
	t.Cleanup(func() {
		if err := RemoveNamespace(plan.Instance, plan.NamespaceInode); err != nil {
			t.Error(err)
			return
		}
		if err := os.Remove(filepath.Join(dir, "network.json")); err != nil {
			t.Error(err)
		}
		if err := os.Remove(dir); err != nil {
			t.Error(err)
		}
	})
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []bool{false, true} {
		args := []string{"--bounding-set=-all,+net_admin,+net_bind_service,+ipc_lock", "--", self, "-test.run=^TestHelperReadBoundaryProbe$", "-test.v"}
		tool := "/usr/bin/setpriv"
		expect := "host"
		if private {
			tool = "/usr/bin/nsenter"
			args = append([]string{"--net=" + filepath.Join(dir, "netns"), "--", "/usr/bin/setpriv"}, args...)
			expect = "private"
		}
		var cmd *exec.Cmd
		if tool == "/usr/bin/setpriv" {
			cmd = exec.Command("/usr/bin/setpriv", args...)
		} else {
			cmd = exec.Command("/usr/bin/nsenter", args...)
		}
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "NGFW_RA_BOUNDARY_PROBE=" + plan.Instance, "NGFW_RA_BOUNDARY_EXPECT=" + expect}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s boundary probe failed: %v %s", expect, err, output)
		}
	}
}
