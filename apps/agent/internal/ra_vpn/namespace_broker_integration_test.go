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
	if validateNamespaceBrokerProcess(os.Getpid(), canonicalBrokerCapabilities) != nil {
		t.Fatal("actual child cap2/zero-inheritable-ambient/NNP boundary differs")
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
	// Exercise only the low-level four-role mutation/restoration algorithm here.
	// Canonical manager/peer authentication is a separate real-unit guest test.
	self, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := self.Close(); err != nil {
			t.Error(err)
		}
	}()
	var source unix.Stat_t
	if unix.Fstat(int(self.Fd()), &source) != nil {
		t.Fatal("held source mount")
	}
	opErr := runAttestedNamespaceBrokerFDs(args, []int{3, 4, 5, int(self.Fd())}, source.Ino)
	expectRefusal := os.Getenv("NGFW_RA_BROKER_EXPECT_REFUSAL") == "1"
	if (opErr != nil) != expectRefusal {
		t.Fatal("private broker result differs from expected ownership refusal", opErr)
	}
	current, err := os.Open("/proc/thread-self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	var actual unix.Stat_t
	statErr = unix.Fstat(int(current.Fd()), &actual)
	closeErr := current.Close()
	if statErr != nil || closeErr != nil || actual.Ino != source.Ino {
		t.Fatal("broker failed source mount restoration")
	}
	if _, err := os.Stat(filepath.Join(InstanceRoot, args[1], "network.json")); !os.IsNotExist(err) {
		t.Fatal("broker retained target root after restoration")
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
	var replacement unix.Stat_t
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 2 {
			// Retain the old inode so the foreign replacement cannot recycle it.
			if os.Rename(paths[0], paths[0]+".original") != nil || os.WriteFile(paths[0], nil, 0600) != nil || unix.Lstat(paths[0], &replacement) != nil {
				t.Fatal("foreign replacement setup")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// The child's /run deliberately hides the owned bindings. The actual Go
		// setns implementation must retarget its root to the target before removal.
		// #nosec G204 -- fixed private namespace/capability launcher executes this test binary only; no caller-provided command or namespace.
		command := exec.CommandContext(ctx, "/usr/bin/unshare", "--mount", "--propagation", "private", "--", "/bin/sh", "-c", "/usr/bin/mount -t tmpfs -o mode=0755 tmpfs /run\nexec \"$@\"", "sh", "/usr/bin/setpriv", "--no-new-privs", "--bounding-set=-all,+sys_admin,+sys_chroot", "--inh-caps=-all", "--ambient-caps=-all", binary, "-test.run=^TestNamespaceBrokerPrivateChild$", "-test.count=1", "-test.v")
		command.ExtraFiles = files
		command.Env = append(os.Environ(), "NGFW_RA_BROKER_PRIVATE_CHILD="+string(raw))
		if attempt == 2 {
			command.Env = append(command.Env, "NGFW_RA_BROKER_EXPECT_REFUSAL=1")
		}
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("actual broker retry %d failed: %v: %s", attempt, err, output)
		}
		if attempt == 2 {
			var after unix.Stat_t
			if unix.Lstat(paths[0], &after) != nil || after.Ino != replacement.Ino || after.Dev != replacement.Dev {
				t.Fatal("foreign replacement mutated")
			}
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
