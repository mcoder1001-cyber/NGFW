package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// CreateNamespace runs in a fixed short-lived subprocess. It never changes a
// Go runtime thread (which might also be the process leader).
func CreateNamespace(ctx context.Context, plan *NetworkPlan) error {
	if plan.Validate() != nil || plan.NamespaceInode != 0 || plan.HostNamespaceInode != 0 || os.Geteuid() != 0 {
		return ErrBoundary
	}
	dir := filepath.Join(InstanceRoot, plan.Instance)
	if err := prepareInstance(plan.Instance); err != nil {
		return err
	}
	var err error
	plan.HostNamespaceInode, err = mountNamespaceBinding(ctx, plan.Instance, "hostnetns", false)
	if err == nil {
		plan.NamespaceInode, err = mountNamespaceBinding(ctx, plan.Instance, "netns", true)
	}
	if err == nil && (plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode) {
		err = ErrBoundary
	}
	if err == nil {
		data, _ := json.Marshal(plan)
		var manifest *os.File
		manifest, err = os.OpenFile(filepath.Join(dir, "network.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, err = manifest.Write(data)
			if err == nil {
				err = manifest.Sync()
			}
			closeErr := manifest.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		for _, binding := range []struct {
			name  string
			inode uint64
		}{{"netns", plan.NamespaceInode}, {"hostnetns", plan.HostNamespaceInode}} {
			if binding.inode != 0 {
				if removeNamedBinding(plan.Instance, binding.name, binding.inode) != nil {
					return ErrBoundary
				}
			} else {
				_ = os.Remove(filepath.Join(dir, binding.name))
			}
		}
		_ = os.Remove(filepath.Join(dir, "network.json"))
		_ = os.Remove(dir)
		plan.NamespaceInode = 0
		plan.HostNamespaceInode = 0
	}
	return err
}

func mountNamespaceBinding(ctx context.Context, instance, name string, isolated bool) (uint64, error) {
	path := filepath.Join(InstanceRoot, instance, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0600)
	if err != nil {
		return 0, ErrBoundary
	}
	file.Close()
	if isolated {
		_, err = command(ctx, "/usr/bin/unshare", nil, "--net", "--", "/usr/bin/mount", "--bind", "/proc/self/ns/net", path)
	} else {
		_, err = command(ctx, "/usr/bin/mount", nil, "--bind", "/proc/self/ns/net", path)
	}
	var stat unix.Stat_t
	var fs unix.Statfs_t
	if unix.Stat(path, &stat) == nil && unix.Statfs(path, &fs) == nil && fs.Type == unix.NSFS_MAGIC {
		return stat.Ino, err
	}
	return 0, ErrBoundary
}

func prepareInstance(instance string) error {
	if !ValidInstance(instance) {
		return ErrBoundary
	}
	fd, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { unix.Close(fd) }()
	for _, name := range []string{"run", "ngfw", "ra"} {
		if name != "run" {
			if err = unix.Mkdirat(fd, name, 0700); err != nil && err != unix.EEXIST {
				return ErrBoundary
			}
		}
		next, err := unix.Openat(fd, name, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			return ErrBoundary
		}
		unix.Close(fd)
		fd = next
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Mode&0022 != 0 {
			return ErrBoundary
		}
	}
	if unix.Mkdirat(fd, instance, 0700) != nil {
		return ErrBoundary
	}
	return nil
}

// RemoveNamespace never recursively deletes an operator-controlled directory.
// All child services/TAPs must already be stopped/deleted by dependency ordering.
func RemoveNamespace(instance string, inode uint64) error {
	if !ValidInstance(instance) || inode == 0 {
		return ErrBoundary
	}
	path := filepath.Join(InstanceRoot, instance, "network.json")
	if ValidatePrivateFile(path, 16384) != nil {
		return ErrBoundary
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ErrBoundary
	}
	plan, err := DecodePrivatePlan(data, instance)
	if err != nil || plan.NamespaceInode != inode {
		return ErrBoundary
	}
	if removeNamedBinding(instance, "netns", inode) != nil {
		return ErrBoundary
	}
	return removeNamedBinding(instance, "hostnetns", plan.HostNamespaceInode)
}

func removeNamedBinding(instance, name string, inode uint64) error {
	if !ValidInstance(instance) || inode == 0 || (name != "netns" && name != "hostnetns") {
		return ErrBoundary
	}
	path := filepath.Join(InstanceRoot, instance, name)
	fd, err := unix.Open(path, unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &stat) != nil || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || stat.Ino != inode {
		return ErrBoundary
	}
	// The held identity fd pins this exact mount. Detach only the verified
	// binding; closing the fd then releases the already-stopped namespace.
	if unix.Unmount(path, unix.MNT_DETACH) != nil {
		return ErrBoundary
	}
	if os.Remove(path) != nil {
		return ErrBoundary
	}
	return nil // keep root-private state for explicit descriptor file cleanup
}
