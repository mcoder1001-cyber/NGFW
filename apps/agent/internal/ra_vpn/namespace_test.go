package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNamespaceRejectsInvalidOrPreboundPlan(t *testing.T) {
	plan := networkFixture()
	plan.NamespaceInode = 1
	if CreateNamespace(context.Background(), plan) == nil {
		t.Fatal("prebound namespace accepted")
	}
	if RemoveNamespace("../../foreign", 1) == nil {
		t.Fatal("foreign path accepted")
	}
}

func TestIntegrationNamespaceProcessIsolationAndOwnedCleanup(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires disposable namespace fixture")
	}
	if os.Geteuid() != 0 {
		t.Fatal("namespace fixture requires root")
	}
	var before, after unix.Stat_t
	if unix.Stat("/proc/self/ns/net", &before) != nil {
		t.Fatal("current namespace inaccessible")
	}
	plan := networkFixture()
	plan.Owner = "w19-fixture"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	dir := filepath.Join(InstanceRoot, plan.Instance)
	if err := CreateNamespace(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
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
	if unix.Stat("/proc/self/ns/net", &after) != nil || before.Ino != after.Ino || before.Dev != after.Dev || plan.NamespaceInode == before.Ino {
		t.Fatal("agent namespace changed")
	}
	// #nosec G304 -- exact fixed manifest basename in the newly created owned fixture namespace root.
	data, err := os.ReadFile(filepath.Join(dir, "network.json"))
	if err != nil {
		t.Fatal(err)
	}
	var observed NetworkPlan
	if json.Unmarshal(data, &observed) != nil || observed.NamespaceInode != plan.NamespaceInode {
		t.Fatal("binding journal mismatch")
	}
	if RemoveNamespace(plan.Instance, plan.NamespaceInode+1) == nil {
		t.Fatal("foreign inode removed")
	}
}
