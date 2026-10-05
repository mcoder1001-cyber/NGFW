package ravpn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDaemonNamespaceRequiresHeldNETAndRecordedSeparation(t *testing.T) {
	ns, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(ns); err != nil {
			t.Error(err)
		}
	})
	var st unix.Stat_t
	if err := unix.Fstat(ns, &st); err != nil {
		t.Fatal(err)
	}
	plan := networkFixture()
	plan.NamespaceInode, plan.HostNamespaceInode = st.Ino, st.Ino+1
	if err := verifyDaemonNamespace(plan, ns, ns); err != nil {
		t.Fatal("actual self NET binding refused", err)
	}
	for _, mutate := range []func(*NetworkPlan){
		func(p *NetworkPlan) { p.HostNamespaceInode = p.NamespaceInode },
		func(p *NetworkPlan) { p.HostNamespaceInode = 0 },
		func(p *NetworkPlan) { p.NamespaceInode++ },
	} {
		changed := *plan
		mutate(&changed)
		if verifyDaemonNamespace(&changed, ns, ns) == nil {
			t.Fatal("ambiguous namespace identity accepted")
		}
	}
	mount, err := unix.Open("/proc/self/ns/mnt", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(mount); err != nil {
			t.Error(err)
		}
	}()
	if verifyDaemonNamespace(plan, mount, ns) == nil {
		t.Fatal("non-NET NSFS accepted as private binding")
	}
	ordinary, err := os.CreateTemp(t.TempDir(), "ordinary-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ordinary.Close(); err != nil {
			t.Error(err)
		}
	}()
	if verifyDaemonNamespace(plan, int(ordinary.Fd()), ns) == nil {
		t.Fatal("ordinary file accepted as NSFS")
	}
}

func TestDaemonReaderRefusesUnsealedManifestAndBinding(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned sealed filesystem fixtures require root; NET identity checks run independently")
	}
	for _, kind := range []string{"manifest-symlink", "manifest-hardlink", "manifest-public", "foreign-instance", "missing-binding", "ordinary-binding", "binding-symlink", "public-directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			// #nosec G302 -- test-owned directory needs owner traversal; no group/other access is granted.
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			plan := networkFixture()
			plan.NamespaceInode, plan.HostNamespaceInode = 1234, 1235
			dir := filepath.Join(root, plan.Instance)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "foreign-instance" {
				plan.Instance = InstanceID("foreign", "road")
			}
			data, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(dir, "network.json")
			if err := os.WriteFile(manifest, data, 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "manifest-symlink":
				if err := os.Rename(manifest, filepath.Join(dir, "foreign.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("foreign.json", manifest); err != nil {
					t.Fatal(err)
				}
			case "manifest-hardlink":
				if err := os.Link(manifest, filepath.Join(dir, "second.json")); err != nil {
					t.Fatal(err)
				}
			case "manifest-public":
				// #nosec G302 -- deliberately unsafe owned manifest tests fail-closed mode verification.
				if err := os.Chmod(manifest, 0644); err != nil {
					t.Fatal(err)
				}
			case "ordinary-binding":
				if err := os.WriteFile(filepath.Join(dir, "netns"), []byte("not NSFS"), 0600); err != nil {
					t.Fatal(err)
				}
			case "binding-symlink":
				if err := os.Symlink("/proc/self/ns/net", filepath.Join(dir, "netns")); err != nil {
					t.Fatal(err)
				}
			case "public-directory":
				// #nosec G302 -- deliberately unsafe owned parent tests private directory refusal.
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			fd, err := unix.Open(root, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := unix.Close(fd); err != nil {
					t.Error(err)
				}
			}()
			if _, err := readDaemonPlanAt(fd, networkFixture().Instance); err == nil {
				t.Fatal("unsealed or ambiguous runtime accepted")
			}
			if _, err := os.Lstat(filepath.Join(dir, "hostnetns")); !os.IsNotExist(err) {
				t.Fatal("reader created or needed host namespace binding")
			}
		})
	}
}
