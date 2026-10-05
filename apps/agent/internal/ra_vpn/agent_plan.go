package ravpn

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// ReadAgentPlan is for the trusted agent, before any VPP handoff. It verifies
// both held namespace bindings against the root-private manifest. Unlike the
// helper entry point it requires the caller in the recorded host namespace.
func ReadAgentPlan(instance string) (*NetworkPlan, error) {
	if os.Geteuid() != 0 || !ValidInstance(instance) {
		return nil, ErrBoundary
	}
	path := filepath.Join(InstanceRoot, instance, "network.json")
	if ValidatePrivateFile(path, 16384) != nil {
		return nil, ErrBoundary
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrBoundary
	}
	plan, err := DecodePrivatePlan(data, instance)
	if err != nil {
		return nil, ErrBoundary
	}
	var private, host, self unix.Stat_t
	for _, entry := range []struct {
		name  string
		inode uint64
		stat  *unix.Stat_t
	}{
		{"netns", plan.NamespaceInode, &private}, {"hostnetns", plan.HostNamespaceInode, &host},
	} {
		fd, err := unix.Open(filepath.Join(InstanceRoot, instance, entry.name), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, ErrBoundary
		}
		var fs unix.Statfs_t
		bad := unix.Fstat(fd, entry.stat) != nil || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || entry.stat.Ino != entry.inode
		unix.Close(fd)
		if bad {
			return nil, ErrBoundary
		}
	}
	if unix.Stat("/proc/self/ns/net", &self) != nil || self.Ino != host.Ino || self.Dev != host.Dev || private.Ino == host.Ino && private.Dev == host.Dev {
		return nil, ErrBoundary
	}
	if validateNamespaceAlias(plan) != nil {
		return nil, ErrBoundary
	}
	return plan, nil
}
