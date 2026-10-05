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
	if os.Geteuid() != 0 || len(args) != 6 {
		return ErrBoundary
	}
	if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil {
		return ErrBoundary
	}
	operation, instance := args[0], args[1]
	if operation != "export" && operation != "verify" && operation != "remove" && operation != "preflight" {
		return ErrBoundary
	}
	if !ValidInstance(instance) {
		return ErrBoundary
	}
	targetBoot, err := bootid.Parse(args[2])
	if err != nil || !targetBoot.Complete() {
		return ErrBoundary
	}
	mountInode, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil || mountInode == 0 {
		return ErrBoundary
	}
	hostInode, err := strconv.ParseUint(args[4], 10, 64)
	if err != nil {
		return ErrBoundary
	}
	privateInode, err := strconv.ParseUint(args[5], 10, 64)
	if err != nil {
		return ErrBoundary
	}
	if !(bootid.Reader{}).ForPID(targetBoot.PID).Equal(targetBoot) || brokerNamespaceFD(fds[0], unix.CLONE_NEWNS, mountInode) != nil {
		return ErrBoundary
	}
	if operation != "preflight" && (hostInode == 0 || privateInode == 0 || hostInode == privateInode || brokerNamespaceFD(fds[1], unix.CLONE_NEWNET, hostInode) != nil || brokerNamespaceFD(fds[2], unix.CLONE_NEWNET, privateInode) != nil) {
		return ErrBoundary
	}
	runtime.LockOSThread()
	if unix.Unshare(unix.CLONE_FS) != nil || unix.Setns(fds[0], unix.CLONE_NEWNS) != nil {
		return ErrBoundary
	}
	var actual unix.Stat_t
	if unix.Stat("/proc/thread-self/ns/mnt", &actual) != nil || actual.Ino != mountInode || !(bootid.Reader{}).ForPID(targetBoot.PID).Equal(targetBoot) {
		return ErrBoundary
	}
	if operation == "preflight" {
		return brokerProtectedParent(InstanceRoot)
	}
	root := filepath.Join(InstanceRoot, instance)
	if brokerProtectedParent(root) != nil {
		return ErrBoundary
	}
	manifest := filepath.Join(root, "network.json")
	if ValidatePrivateFile(manifest, 16384) != nil {
		return ErrBoundary
	}
	fd, err := unix.Open(manifest, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
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
		return ErrBoundary
	}
	record, err := readNamespaceExport(&plan)
	if err != nil {
		return ErrBoundary
	}
	matched := false
	for _, target := range record.Targets {
		if target.MountInode == mountInode && target.Boot.Equal(targetBoot) {
			matched = true
		}
	}
	if !matched {
		return ErrBoundary
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
		if err := brokerBindingState(entry.path, entry.inode, operation == "export"); err != nil {
			return err
		}
	}
	for _, entry := range paths {
		var fs unix.Statfs_t
		if unix.Statfs(entry.path, &fs) != nil {
			return ErrBoundary
		}
		if operation == "export" && fs.Type != unix.NSFS_MAGIC {
			if unix.Mount(fmt.Sprintf("/proc/self/fd/%d", entry.fd), entry.path, "", unix.MS_BIND, "") != nil {
				return ErrBoundary
			}
		} else if operation == "remove" {
			if unix.Unmount(entry.path, 0) != nil {
				return ErrBoundary
			}
		}
	}
	if operation != "remove" {
		for _, entry := range paths {
			if brokerBindingState(entry.path, entry.inode, false) != nil {
				return ErrBoundary
			}
		}
	}
	if !(bootid.Reader{}).ForPID(targetBoot.PID).Equal(targetBoot) {
		return ErrBoundary
	}
	return nil
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
	if !placeholder || st.Uid != 0 || st.Gid != 0 || st.Mode != unix.S_IFREG|0600 || st.Nlink != 1 || st.Size != 0 {
		return ErrBoundary
	}
	return nil
}
