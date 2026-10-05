package ravpn

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var numericOpenFileBoot = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validNumericOpenFileTarget(target bootid.Identity) bool {
	return target.Complete() && target.PID > 1 && uint64(target.PID) <= uint64(2147483647) && numericOpenFileBoot.MatchString(target.BootID)
}

// RenderTargetsOpenFile constructs the only accepted canonical VPP supplier
// drop-in. systemd OpenFile does not expand template instance specifiers.
func RenderTargetsOpenFile(target bootid.Identity) ([]byte, error) {
	if !validNumericOpenFileTarget(target) {
		return nil, ErrBoundary
	}
	content := fmt.Sprintf("# ngfw-ra-targets-v1 boot=%s pid=%d start=%d\n[Service]\nOpenFile=\nOpenFile=/proc/%d/ns/mnt:vpp-mount:read-only\nOpenFile=/proc/1/ns/mnt:manager-mount:read-only\nOpenFile=/proc/%d/exe:vpp-exe:read-only\nOpenFile=%s:%s:read-only\n", target.BootID, target.PID, target.StartTime, target.PID, target.PID, sourceAgentExecutableReference, sourceAgentExecutableRole)
	return []byte(content), nil
}

// ValidateTargetsOpenFile refuses all extra directives, paths and role aliases.
func ValidateTargetsOpenFile(target bootid.Identity, data []byte) error {
	expected, err := RenderTargetsOpenFile(target)
	if err != nil || !bytes.Equal(expected, data) {
		return ErrBoundary
	}
	return nil
}

// ParseTargetsOpenFile recognizes only a complete deterministic owned record.
func ParseTargetsOpenFile(data []byte) (bootid.Identity, error) {
	if len(data) > 4096 {
		return bootid.Identity{}, ErrBoundary
	}
	header, _, ok := strings.Cut(string(data), "\n")
	fields := strings.Fields(header)
	if !ok || len(fields) != 5 || fields[0] != "#" || fields[1] != "ngfw-ra-targets-v1" {
		return bootid.Identity{}, ErrBoundary
	}
	boot := strings.TrimPrefix(fields[2], "boot=")
	pidText := strings.TrimPrefix(fields[3], "pid=")
	startText := strings.TrimPrefix(fields[4], "start=")
	pid, err := strconv.ParseInt(pidText, 10, 32)
	if err != nil || strconv.FormatInt(pid, 10) != pidText {
		return bootid.Identity{}, ErrBoundary
	}
	start, err := strconv.ParseUint(startText, 10, 64)
	if err != nil || strconv.FormatUint(start, 10) != startText {
		return bootid.Identity{}, ErrBoundary
	}
	target := bootid.Identity{BootID: boot, PID: int(pid), StartTime: start}
	if ValidateTargetsOpenFile(target, data) != nil {
		return bootid.Identity{}, ErrBoundary
	}
	return target, nil
}

// TargetsOpenFilePath derives one fixed numeric unit-specific runtime path.
func TargetsOpenFilePath(pid int) (string, error) {
	if pid <= 1 || uint64(pid) > uint64(2147483647) {
		return "", ErrBoundary
	}
	return "/run/systemd/system/ngfw-ra-targets@" + strconv.Itoa(pid) + ".service.d/10-openfile.conf", nil
}

func readTargetsOpenFile(target bootid.Identity) error {
	path, err := TargetsOpenFilePath(target.PID)
	if err != nil {
		return ErrEngine
	}
	return readTargetsOpenFileAt(path, target)
}

// The alternative path is internal trusted-fixture input only. Production calls
// derive the sole fixed path above; profiles never select a path or drop-in.
func readTargetsOpenFileAt(path string, target bootid.Identity) error {
	if brokerProtectedParent(filepath.Dir(path)) != nil {
		return ErrEngine
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return ErrEngine
	}
	file := os.NewFile(uintptr(fd), "owned target OpenFile configuration")
	defer func() { _ = file.Close() }()
	var held, current unix.Stat_t
	if unix.Fstat(fd, &held) != nil || held.Uid != 0 || held.Mode != unix.S_IFREG|0600 || held.Nlink != 1 || held.Size <= 0 || held.Size > 4096 {
		return ErrEngine
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || ValidateTargetsOpenFile(target, data) != nil || unix.Lstat(path, &current) != nil || current.Dev != held.Dev || current.Ino != held.Ino || current.Mode != held.Mode || current.Uid != held.Uid || current.Nlink != held.Nlink || current.Ctim != held.Ctim || current.Mtim != held.Mtim {
		return ErrEngine
	}
	return nil
}

const targetSupplierServiceDigest = "9acdbfad058b5210977f1327c693b1b656ac6f195a91203068760946ebb1b810"

const targetSupplierSocketDigest = "e6d5185fc2f26d32cd6bf0dc5a046b6db0eac2fe53e0d1a2f877f8986cfc2d2d"
