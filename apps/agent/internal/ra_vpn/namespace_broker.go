package ravpn

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// RunNamespaceBroker is the fixed privileged subprocess entry point. FD3 is a
// held target mount namespace; FD4/5 are held host/private network namespaces.
// No operator-controlled path or source is accepted. The helper exits after
// this operation, without restoring a Go runtime thread into another namespace.
func RunNamespaceBroker(args []string) error { return runNamespaceBrokerFDs(args, []int{3, 4, 5}) }

func runNamespaceBrokerFDs(args []string, fds []int) error {
	return runNamespaceBrokerHeldFDs(args, fds, false)
}

func runAttestedNamespaceBrokerFDs(args []string, fds []int) error {
	return runNamespaceBrokerHeldFDs(args, fds, true)
}

func runNamespaceBrokerHeldFDs(args []string, fds []int, attested bool) error {
	if os.Geteuid() != 0 || len(args) != 6 || len(fds) != 3 {
		return fmt.Errorf("%w: namespace broker authority", ErrBoundary)
	}
	if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil {
		return fmt.Errorf("%w: namespace broker no-new-privileges", ErrBoundary)
	}
	operation, instance := args[0], args[1]
	if operation != "export" && operation != "verify" && operation != "remove" && operation != "preflight" {
		return fmt.Errorf("%w: namespace broker operation", ErrBoundary)
	}
	if !ValidInstance(instance) {
		return fmt.Errorf("%w: namespace broker instance", ErrBoundary)
	}
	targetBoot, err := bootid.Parse(args[2])
	if err != nil || !targetBoot.Complete() {
		return fmt.Errorf("%w: namespace broker target-boot", ErrBoundary)
	}
	mountInode, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil || mountInode == 0 {
		return fmt.Errorf("%w: namespace broker target-mount", ErrBoundary)
	}
	hostInode, err := strconv.ParseUint(args[4], 10, 64)
	if err != nil {
		return fmt.Errorf("%w: namespace broker host-network", ErrBoundary)
	}
	privateInode, err := strconv.ParseUint(args[5], 10, 64)
	if err != nil {
		return fmt.Errorf("%w: namespace broker private-network", ErrBoundary)
	}
	if !attested && !brokerCurrentMount(targetBoot, mountInode) {
		return fmt.Errorf("%w: namespace broker target-current", ErrBoundary)
	}
	if brokerNamespaceFD(fds[0], unix.CLONE_NEWNS, mountInode) != nil {
		return fmt.Errorf("%w: namespace broker held-target", ErrBoundary)
	}
	if operation != "preflight" && (hostInode == 0 || privateInode == 0 || hostInode == privateInode || brokerNamespaceFD(fds[1], unix.CLONE_NEWNET, hostInode) != nil || brokerNamespaceFD(fds[2], unix.CLONE_NEWNET, privateInode) != nil) {
		return fmt.Errorf("%w: namespace broker held-networks", ErrBoundary)
	}
	runtime.LockOSThread()
	if unix.Unshare(unix.CLONE_FS) != nil || unix.Setns(fds[0], unix.CLONE_NEWNS) != nil {
		return fmt.Errorf("%w: namespace broker setns", ErrBoundary)
	}
	var actual unix.Stat_t
	if unix.Stat("/proc/thread-self/ns/mnt", &actual) != nil || actual.Ino != mountInode || !(bootid.Reader{}).ForPID(targetBoot.PID).Equal(targetBoot) {
		return fmt.Errorf("%w: namespace broker entered-target", ErrBoundary)
	}
	if operation == "preflight" {
		return brokerProtectedParent(InstanceRoot)
	}
	root := filepath.Join(InstanceRoot, instance)
	if brokerProtectedParent(root) != nil {
		return fmt.Errorf("%w: namespace broker target-parent", ErrBoundary)
	}
	manifest := filepath.Join(root, "network.json")
	if ValidatePrivateFile(manifest, 16384) != nil {
		return fmt.Errorf("%w: namespace broker manifest-mode", ErrBoundary)
	}
	fd, err := unix.Open(manifest, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("%w: namespace broker manifest-open", ErrBoundary)
	}
	file := os.NewFile(uintptr(fd), "protected namespace manifest")
	var plan NetworkPlan
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	decodeError := decoder.Decode(&plan)
	if decodeError == nil && decoder.Decode(new(any)) != io.EOF {
		decodeError = ErrBoundary
	}
	closeError := file.Close()
	if decodeError != nil || closeError != nil || plan.Validate() != nil || plan.Instance != instance || plan.NamespaceInode != privateInode || plan.HostNamespaceInode != hostInode {
		return fmt.Errorf("%w: namespace broker manifest-binding", ErrBoundary)
	}
	record, err := readNamespaceExport(&plan)
	if err != nil {
		return fmt.Errorf("%w: namespace broker export-receipt", ErrBoundary)
	}
	matched := false
	for _, target := range record.Targets {
		if target.MountInode == mountInode && target.Boot.Equal(targetBoot) {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("%w: namespace broker owned-role", ErrBoundary)
	}
	// All paths are derived from the validated full instance, and every parent is
	// protected against rename. Ordinary placeholders must be owned single-link
	// empty files; pre-existing foreign mounts are refused before any mutation.
	paths := []struct {
		path  string
		fd    int
		inode uint64
	}{{filepath.Join(root, "hostnetns"), fds[1], hostInode}, {filepath.Join(root, "netns"), fds[2], privateInode}, {NamespacePath(instance), fds[2], privateInode}}
	for _, entry := range paths {
		if err := brokerBindingState(entry.path, entry.inode, operation == "export" || operation == "remove"); err != nil {
			return err
		}
	}
	for _, entry := range paths {
		var fs unix.Statfs_t
		if unix.Statfs(entry.path, &fs) != nil {
			return fmt.Errorf("%w: namespace broker mount-readback", ErrBoundary)
		}
		if operation == "export" && fs.Type != unix.NSFS_MAGIC {
			if unix.Mount(fmt.Sprintf("/proc/self/fd/%d", entry.fd), entry.path, "", unix.MS_BIND, "") != nil {
				return fmt.Errorf("%w: namespace broker mount-export", ErrBoundary)
			}
		} else if operation == "remove" && fs.Type == unix.NSFS_MAGIC {
			if unix.Unmount(entry.path, unix.MNT_DETACH) != nil {
				return fmt.Errorf("%w: namespace broker mount-remove", ErrBoundary)
			}
		}
	}
	if operation != "remove" {
		for _, entry := range paths {
			if brokerBindingState(entry.path, entry.inode, false) != nil {
				return fmt.Errorf("%w: namespace broker removed-binding", ErrBoundary)
			}
		}
	}
	if operation == "remove" {
		for _, entry := range paths {
			var fs unix.Statfs_t
			if unix.Statfs(entry.path, &fs) != nil || fs.Type == unix.NSFS_MAGIC || brokerBindingState(entry.path, entry.inode, true) != nil {
				return fmt.Errorf("%w: namespace broker target-still-current", ErrBoundary)
			}
		}
	}
	if !attested && !brokerCurrentMount(targetBoot, mountInode) {
		return fmt.Errorf("%w: namespace broker final-boundary-23", ErrBoundary)
	}
	return nil
}

func brokerCurrentMount(identity bootid.Identity, inode uint64) bool {
	var stat unix.Stat_t
	return identity.Complete() && inode != 0 && (bootid.Reader{}).ForPID(identity.PID).Equal(identity) && unix.Stat("/proc/"+strconv.Itoa(identity.PID)+"/ns/mnt", &stat) == nil && stat.Ino == inode
}

func brokerNamespaceFD(fd, kind int, inode uint64) error {
	var st unix.Stat_t
	var fs unix.Statfs_t
	actual, err := unix.IoctlRetInt(fd, unix.NS_GET_NSTYPE)
	if err != nil || actual != kind || unix.Fstat(fd, &st) != nil || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || st.Ino != inode {
		return ErrBoundary
	}
	return nil
}
func brokerProtectedParent(path string) error {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return ErrBoundary
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	for _, name := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		next, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return ErrBoundary
		}
		_ = unix.Close(fd)
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 {
			return ErrBoundary
		}
	}
	return nil
}
func brokerBindingState(path string, inode uint64, placeholder bool) error {
	if brokerProtectedParent(filepath.Dir(path)) != nil {
		return ErrBoundary
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &st) != nil || unix.Fstatfs(fd, &fs) != nil {
		return ErrBoundary
	}
	if fs.Type == unix.NSFS_MAGIC {
		if st.Ino != inode {
			return ErrBoundary
		}
		return nil
	}
	// The fixed agent runs root:ngfw and has no CAP_CHOWN. Exact 0600 grants
	// no group access, so its gid does not change the root-only boundary.
	if !placeholder || st.Uid != 0 || st.Mode != unix.S_IFREG|0600 || st.Nlink != 1 || st.Size != 0 {
		return ErrBoundary
	}
	return nil
}
