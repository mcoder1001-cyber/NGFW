package ravpn

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// Fixed manager OpenFile source role; neither path nor role is profile input.
const sourceAgentExecutableRole = "source-agent-exe"
const sourceAgentExecutableReference = "/run/ngfw/ra/source-agent-exe"
const sourceAgentIdentityRecord = "/run/ngfw/ra/source-agent.json"

type sourceAgentReferenceRecord struct {
	Source bootid.Identity
}

// readSourceAgentReference validates a protected reference before or after the
// manager captures the actual source EXE. It does not substitute for comparing
// that held EXE descriptor to the installed fixed agent artifact.
func readSourceAgentReference(expected bootid.Identity) error {
	return readSourceAgentReferenceAt(filepath.Dir(sourceAgentIdentityRecord), expected)
}

func readSourceAgentReferenceAt(root string, expected bootid.Identity) error {
	if !expected.Complete() || expected.PID <= 1 || brokerProtectedParent(root) != nil || !(bootid.Reader{}).ForPID(expected.PID).Equal(expected) {
		return ErrBoundary
	}
	path := filepath.Join(root, "source-agent.json")
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "protected source agent identity")
	var held unix.Stat_t
	if unix.Fstat(fd, &held) != nil || held.Uid != 0 || held.Mode != unix.S_IFREG|0600 || held.Nlink != 1 || held.Size > 1024 {
		_ = file.Close()
		return ErrBoundary
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(data) > 1024 {
		return ErrBoundary
	}
	var record sourceAgentReferenceRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF || !record.Source.Equal(expected) {
		return ErrBoundary
	}
	link := filepath.Join(root, "source-agent-exe")
	var stat unix.Stat_t
	if unix.Lstat(link, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFLNK || stat.Uid != 0 || stat.Nlink != 1 {
		return ErrBoundary
	}
	target, err := os.Readlink(link)
	if err != nil || target != "/proc/"+strconv.Itoa(expected.PID)+"/exe" || !(bootid.Reader{}).ForPID(expected.PID).Equal(expected) {
		return ErrBoundary
	}
	return nil
}

// validateSourceAgentExecutable accepts only the manager-held actual source
// executable matched to the fixed protected installed agent artifact.
func validateSourceAgentExecutable(file *os.File) error {
	if file == nil || brokerProtectedParent("/usr/sbin") != nil {
		return ErrBoundary
	}
	fd, err := unix.Open("/usr/sbin/ngfw-agent", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	var installed, observed unix.Stat_t
	if unix.Fstat(fd, &installed) != nil || unix.Fstat(int(file.Fd()), &observed) != nil || installed.Uid != 0 || installed.Mode&unix.S_IFMT != unix.S_IFREG || installed.Mode&0022 != 0 || installed.Mode&0111 == 0 || installed.Dev != observed.Dev || installed.Ino != observed.Ino {
		return ErrBoundary
	}
	return nil
}
