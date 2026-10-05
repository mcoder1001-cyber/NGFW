package strongswan

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RestrictRAVICISocket handles charon's default0660 socket without relaxing
// DialRAVICI's private0600 contract. An O_PATH fd pins the exact socket inode;
// peer credentials are verified through that inode before fchmodat2. A daemon
// rename/symlink/hardlink race cannot chmod an unrelated host file.
func RestrictRAVICISocket(ctx context.Context, path string, expectedPID int) error {
	if os.Geteuid() != 0 || expectedPID <= 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrRAObservation
	}
	fd, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrRAObservation
	}
	defer func() { unix.Close(fd) }()
	components := strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/")
	for _, component := range components {
		next, err := unix.Openat(fd, component, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return ErrRAObservation
		}
		unix.Close(fd)
		fd = next
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Mode&0022 != 0 {
			return ErrRAObservation
		}
	}
	var parent unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&0077 != 0 {
		return ErrRAObservation
	}
	socket, err := unix.Openat(fd, filepath.Base(path), unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrRAObservation
	}
	defer unix.Close(socket)
	var stat unix.Stat_t
	if unix.Fstat(socket, &stat) != nil || stat.Uid != 0 || stat.Nlink != 1 || stat.Mode&unix.S_IFMT != unix.S_IFSOCK || (stat.Mode&0777 != 0600 && stat.Mode&0777 != 0660) {
		return ErrRAObservation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", fmt.Sprintf("/proc/self/fd/%d", socket))
	if err != nil {
		return ErrRAObservation
	}
	defer connection.Close()
	raw, err := connection.(*net.UnixConn).SyscallConn()
	if err != nil {
		return ErrRAObservation
	}
	valid := false
	if raw.Control(func(fd uintptr) {
		peer, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		valid = err == nil && peer.Uid == 0 && peer.Pid == int32(expectedPID)
	}) != nil || !valid {
		return ErrRAObservation
	}
	if unix.Fchmodat(socket, "", 0600, unix.AT_EMPTY_PATH) != nil {
		return ErrRAObservation
	}
	var after unix.Stat_t
	if unix.Fstatat(fd, filepath.Base(path), &after, unix.AT_SYMLINK_NOFOLLOW) != nil || after.Dev != stat.Dev || after.Ino != stat.Ino || after.Mode&0777 != 0600 {
		return ErrRAObservation
	}
	return nil
}
