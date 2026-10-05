package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func TestNamespaceBrokerPrivateChild(t *testing.T) {
	raw := os.Getenv("NGFW_RA_BROKER_PRIVATE_CHILD")
	if raw == "" {
		t.Skip("private subprocess only")
	}
	var args []string
	if json.Unmarshal([]byte(raw), &args) != nil {
		t.Fatal("invalid private test arguments")
	}
	identity, err := bootid.Parse(args[2])
	if err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	statErr := unix.Stat("/proc/"+strconv.Itoa(identity.PID)+"/ns/mnt", &stat)
	t.Logf("private target identity matches=%t mount-stat-errno=%v inode-matches=%t observed=%d expected=%s", (bootid.Reader{}).ForPID(identity.PID).Equal(identity), statErr, strconv.FormatUint(stat.Ino, 10) == args[3], stat.Ino, args[3])
	if err := RunNamespaceBroker(args); err != nil {
		t.Fatal("private bounded broker refused", err)
	}
}

func TestIntegrationBrokerPartialRemovalRetry(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires disposable network/mount namespace")
	}
	var own, initial unix.Stat_t
	if unix.Stat("/proc/self/ns/net", &own) != nil || unix.Stat("/proc/1/ns/net", &initial) != nil || own.Ino == initial.Ino {
		t.Fatal("shared network namespace refused")
	}
	if unix.Stat("/proc/self/ns/mnt", &own) != nil || unix.Stat("/proc/1/ns/mnt", &initial) != nil || own.Ino == initial.Ino {
		t.Fatal("shared mount namespace refused")
	}
	plan := networkFixture()
	plan.Owner = "w19-broker-retry"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	for _, path := range []string{InstanceRoot, filepath.Dir(NamespacePath(plan.Instance))} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := CreateNamespace(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	var mountStat unix.Stat_t
	if unix.Stat("/proc/self/ns/mnt", &mountStat) != nil {
		t.Fatal("private mount identity")
	}
	target := MountTarget{Boot: identity, MountInode: mountStat.Ino}
	if err := writeNamespaceExport(namespaceExportRecord{Instance: plan.Instance, Namespace: plan.NamespaceInode, HostNamespace: plan.HostNamespaceInode, Targets: []MountTarget{target, target}, Pending: true}, false); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(InstanceRoot, plan.Instance, "hostnetns"), filepath.Join(InstanceRoot, plan.Instance, "netns"), NamespacePath(plan.Instance)}
	var files []*os.File
	for _, path := range []string{"/proc/self/ns/mnt", paths[0], paths[1]} {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		defer func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	// Simulate a previous exporter/remover that stopped after its first binding.
	if err := unix.Unmount(paths[0], unix.MNT_DETACH); err != nil {
		t.Fatal(err)
	}
	args := []string{"remove", plan.Instance, identity.String(), strconv.FormatUint(mountStat.Ino, 10), strconv.FormatUint(plan.HostNamespaceInode, 10), strconv.FormatUint(plan.NamespaceInode, 10)}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// The child's /run deliberately hides the owned bindings. The actual Go
		// setns implementation must retarget its root to the target before removal.
		command := exec.CommandContext(ctx, "/usr/bin/unshare", "--mount", "--propagation", "private", "--", "/bin/sh", "-c", "/usr/bin/mount -t tmpfs -o mode=0755 tmpfs /run\nexec \"$@\"", "sh", "/usr/bin/setpriv", "--no-new-privs", "--bounding-set=-all,+sys_admin,+sys_chroot", "--inh-caps=-all", "--ambient-caps=-all", binary, "-test.run=^TestNamespaceBrokerPrivateChild$", "-test.count=1", "-test.v")
		command.ExtraFiles = files
		command.Env = append(os.Environ(), "NGFW_RA_BROKER_PRIVATE_CHILD="+string(raw))
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("actual broker retry %d failed: %v: %s", attempt, err, output)
		}
		for _, path := range paths {
			var fs unix.Statfs_t
			if unix.Statfs(path, &fs) != nil || fs.Type == unix.NSFS_MAGIC || brokerBindingState(path, 0, true) != nil {
				t.Fatal("owned target binding not removed", path)
			}
		}
	}
	// Retained receipt is necessary for repeat removal; the caller deletes it
	// only after every target's observed cleanup succeeds.
	if _, err := readNamespaceExport(plan); err != nil {
		t.Fatal("cleanup receipt lost", err)
	}
}
