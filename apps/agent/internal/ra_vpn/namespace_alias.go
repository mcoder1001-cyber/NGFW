package ravpn

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

var namespaceAliasName = regexp.MustCompile(`^[a-f0-9]{32}$`)

// NamespacePath returns the short root-owned NSFS alias below VPP's string[64]
// limit. The full instance identity remains in the protected network.json.
func NamespacePath(instance string) string {
	if !ValidInstance(instance) {
		return ""
	}
	return filepath.Join(InstanceRoot, "n", instance[:32])
}

// NamespaceKeyID derives the bounded alias key from a validated full instance.
func NamespaceKeyID(instance string) string {
	if !ValidInstance(instance) {
		return ""
	}
	return instance[:32]
}
func openAliasParent(create bool) (int, error) {
	fd, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrBoundary
	}
	for _, name := range []string{"run", "ngfw", "ra", "n"} {
		if name == "n" && create {
			if err := unix.Mkdirat(fd, name, 0700); err != nil && err != unix.EEXIST {
				_ = unix.Close(fd)
				return -1, ErrBoundary
			}
		}
		next, err := unix.Openat(fd, name, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return -1, ErrBoundary
		}
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 || name == "n" && st.Mode&0077 != 0 {
			_ = unix.Close(fd)
			return -1, ErrBoundary
		}
	}
	return fd, nil
}
func createNamespaceAlias(ctx context.Context, plan *NetworkPlan) (bool, error) {
	fd, err := openAliasParent(true)
	if err != nil {
		return false, err
	}
	defer func() { _ = unix.Close(fd) }()
	name := NamespaceKeyID(plan.Instance)
	if name == "" || plan.NamespaceInode == 0 {
		return false, ErrBoundary
	}
	child, err := unix.Openat(fd, name, unix.O_CREAT|unix.O_EXCL|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return false, ErrBoundary
	}
	var original unix.Stat_t
	statErr := unix.Fstat(child, &original)
	closeErr := unix.Close(child)
	if statErr != nil || closeErr != nil || recordNamespaceBirth(plan.Instance, "alias", original) != nil {
		// The exclusive placeholder exists, but its ownership identity was not
		// established. Preserve it for recovery instead of mounting or unlinking.
		return true, ErrBoundary
	}
	_, mountErr := command(ctx, "/usr/bin/mount", nil, "--bind", filepath.Join(InstanceRoot, plan.Instance, "netns"), NamespacePath(plan.Instance))
	if validateNamespaceAlias(plan) == nil && mountErr == nil {
		return true, nil
	}
	var current unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstatat(fd, name, &current, unix.AT_SYMLINK_NOFOLLOW) == nil && unix.Statfs(NamespacePath(plan.Instance), &fs) == nil && fs.Type != unix.NSFS_MAGIC && current.Ino == original.Ino && current.Dev == original.Dev {
		if unix.Unlinkat(fd, name, 0) == nil {
			return false, ErrBoundary
		}
	}
	return true, ErrBoundary // retain uncertain identity for partial-create recovery
}
func validateNamespaceAlias(plan *NetworkPlan) error {
	fd, err := openAliasParent(false)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	name := NamespaceKeyID(plan.Instance)
	if name == "" || plan.NamespaceInode == 0 {
		return ErrBoundary
	}
	child, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(child) }()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(child, &st) != nil || unix.Fstatfs(child, &fs) != nil || fs.Type != unix.NSFS_MAGIC || st.Ino != plan.NamespaceInode {
		return ErrBoundary
	}
	return nil
}
func removeNamespaceAlias(plan *NetworkPlan) error {
	record, birthErr := readNamespaceBirth(plan.Instance)
	if birthErr != nil || record.Bindings["alias"].Inode == 0 {
		return ErrBoundary
	}
	path := NamespacePath(plan.Instance)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if verifyNamespaceBirth(plan.Instance, "alias", path) == nil {
		return os.Remove(path)
	}
	if validateNamespaceAlias(plan) != nil {
		return ErrBoundary
	}
	fd, err := openAliasParent(false)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	child, err := unix.Openat(fd, NamespaceKeyID(plan.Instance), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(child) }()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(child, &st) != nil || unix.Fstatfs(child, &fs) != nil || fs.Type != unix.NSFS_MAGIC || st.Ino != plan.NamespaceInode {
		return ErrBoundary
	}
	if unix.Unmount(NamespacePath(plan.Instance), unix.MNT_DETACH) != nil || verifyNamespaceBirth(plan.Instance, "alias", NamespacePath(plan.Instance)) != nil || unix.Unlinkat(fd, NamespaceKeyID(plan.Instance), 0) != nil {
		return ErrBoundary
	}
	return nil
}

// ReadAgentPlanByNamespace maps an exact short alias to one protected full
// instance manifest, then verifies alias/full NSFS identity. A truncated-prefix
// collision, symlink, stale alias or foreign binding is never adopted.
func ReadAgentPlanByNamespace(path string) (*NetworkPlan, error) {
	if filepath.Dir(path) != filepath.Join(InstanceRoot, "n") || !namespaceAliasName.MatchString(filepath.Base(path)) {
		return nil, ErrBoundary
	}
	fd, err := openAliasParent(false)
	if err != nil {
		return nil, err
	}
	_ = unix.Close(fd)
	entries, err := os.ReadDir(InstanceRoot)
	if err != nil || len(entries) > 2048 {
		return nil, ErrBoundary
	}
	instance := ""
	for _, entry := range entries {
		if entry.IsDir() && ValidInstance(entry.Name()) && strings.HasPrefix(entry.Name(), filepath.Base(path)) {
			if instance != "" {
				return nil, ErrBoundary
			}
			instance = entry.Name()
		}
	}
	if instance == "" {
		return nil, ErrBoundary
	}
	plan, err := ReadAgentPlan(instance)
	if err != nil || NamespacePath(instance) != path || validateNamespaceAlias(plan) != nil {
		return nil, ErrBoundary
	}
	return plan, nil
}
