package ravpn

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ReadDaemonPlan reads only the sealed daemon-visible plan and private namespace
// binding. It proves that binding matches this process and differs from the
// recorded host namespace; it never opens a host namespace descriptor.
func ReadDaemonPlan(instance string) (plan *NetworkPlan, result error) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil || os.Geteuid() != 0 || ValidateHelperCapabilities(string(status)) != nil || !ValidInstance(instance) {
		return nil, ErrBoundary
	}
	fd, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if unix.Close(fd) != nil {
			plan, result = nil, ErrBoundary
		}
	}()
	for _, name := range []string{"run", "ngfw", "ra"} {
		next, openErr := unix.Openat(fd, name, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, ErrBoundary
		}
		closeErr := unix.Close(fd)
		fd = next
		if closeErr != nil || protectedDaemonDirectory(fd, false) != nil {
			return nil, ErrBoundary
		}
	}
	return readDaemonPlanAt(fd, instance)
}

func protectedDaemonDirectory(fd int, private bool) error {
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0022 != 0 || private && st.Mode&0077 != 0 {
		return ErrBoundary
	}
	return nil
}

func readDaemonPlanAt(root int, instance string) (plan *NetworkPlan, result error) {
	if !ValidInstance(instance) || protectedDaemonDirectory(root, false) != nil {
		return nil, ErrBoundary
	}
	fd, err := unix.Openat(root, instance, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if unix.Close(fd) != nil {
			plan, result = nil, ErrBoundary
		}
	}()
	if protectedDaemonDirectory(fd, true) != nil {
		return nil, ErrBoundary
	}
	manifest, err := unix.Openat(fd, "network.json", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	f := os.NewFile(uintptr(manifest), "network.json")
	defer func() {
		if f.Close() != nil {
			plan, result = nil, ErrBoundary
		}
	}()
	var st unix.Stat_t
	if unix.Fstat(manifest, &st) != nil || st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Nlink != 1 || st.Size > 16384 {
		return nil, ErrBoundary
	}
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 {
		return nil, ErrBoundary
	}
	plan, err = DecodePrivatePlan(data, instance)
	if err != nil {
		return nil, ErrBoundary
	}
	binding, err := unix.Openat(fd, "netns", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if unix.Close(binding) != nil {
			plan, result = nil, ErrBoundary
		}
	}()
	self, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if unix.Close(self) != nil {
			plan, result = nil, ErrBoundary
		}
	}()
	if verifyDaemonNamespace(plan, binding, self) != nil {
		return nil, ErrBoundary
	}
	return plan, nil
}

func verifyDaemonNamespace(plan *NetworkPlan, binding, self int) error {
	if plan == nil || plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode {
		return ErrBoundary
	}
	var boundStat, selfStat unix.Stat_t
	for _, fd := range []int{binding, self} {
		var fs unix.Statfs_t
		typ, err := unix.IoctlRetInt(fd, unix.NS_GET_NSTYPE)
		if unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || err != nil || typ != unix.CLONE_NEWNET {
			return ErrBoundary
		}
	}
	if unix.Fstat(binding, &boundStat) != nil || unix.Fstat(self, &selfStat) != nil || boundStat.Ino != plan.NamespaceInode || selfStat.Ino != plan.NamespaceInode || boundStat.Dev != selfStat.Dev {
		return ErrBoundary
	}
	return nil
}
