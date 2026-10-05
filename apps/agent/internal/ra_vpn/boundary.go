package ravpn

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

var ErrBoundary = errors.New("remote-access: isolated runtime boundary refused")

// ReadPrivatePlan walks every parent with O_NOFOLLOW. An API-owned directory,
// symlink or writable manifest cannot redirect this privileged helper.
func ReadPrivatePlan(instance string) (*NetworkPlan, error) {
	if !ValidInstance(instance) {
		return nil, ErrBoundary
	}
	fd, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() { unix.Close(fd) }()
	for _, name := range []string{"run", "ngfw", "ra", instance} {
		next, err := unix.Openat(fd, name, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, ErrBoundary
		}
		unix.Close(fd)
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 {
			return nil, ErrBoundary
		}
		if name == instance && st.Mode&0077 != 0 {
			return nil, ErrBoundary
		}
	}
	manifest, err := unix.Openat(fd, "network.json", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	f := os.NewFile(uintptr(manifest), "network.json")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(manifest, &st) != nil || st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Nlink != 1 || st.Size > 16384 {
		return nil, ErrBoundary
	}
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 {
		return nil, ErrBoundary
	}
	plan, err := DecodePrivatePlan(data, instance)
	if err != nil {
		return nil, err
	}
	// Compare the held namespace binding with this helper's actual namespace.
	ns, err := unix.Openat(fd, "netns", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer unix.Close(ns)
	var binding, self, host unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(ns, &binding) != nil || unix.Fstatfs(ns, &fs) != nil || fs.Type != unix.NSFS_MAGIC || unix.Stat("/proc/self/ns/net", &self) != nil || unix.Stat("/proc/1/ns/net", &host) != nil {
		return nil, ErrBoundary
	}
	if plan.NamespaceInode != binding.Ino || self.Ino != binding.Ino || self.Dev != binding.Dev || self.Ino == host.Ino && self.Dev == host.Dev {
		return nil, ErrBoundary
	}
	return plan, nil
}

// DecodePrivatePlan is strict and requires the recorded namespace identity.
func DecodePrivatePlan(data []byte, instance string) (*NetworkPlan, error) {
	if len(data) > 16384 || !ValidInstance(instance) {
		return nil, ErrBoundary
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan NetworkPlan
	if decoder.Decode(&plan) != nil || decoder.Decode(new(any)) != io.EOF || plan.Instance != instance || plan.NamespaceInode == 0 || plan.Validate() != nil {
		return nil, ErrBoundary
	}
	return &plan, nil
}
