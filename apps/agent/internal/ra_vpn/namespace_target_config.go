package ravpn

import (
	"bytes"
	"fmt"
	"ngfw/agent/internal/vpp/bootid"
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
	content := fmt.Sprintf("# ngfw-ra-targets-v1 boot=%s pid=%d start=%d\n[Service]\nOpenFile=\nOpenFile=/proc/%d/ns/mnt:vpp-mount:read-only\nOpenFile=/proc/1/ns/mnt:manager-mount:read-only\nOpenFile=%s:%s:read-only\n", target.BootID, target.PID, target.StartTime, target.PID, sourceAgentExecutableReference, sourceAgentExecutableRole)
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
