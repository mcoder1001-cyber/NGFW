package ravpn

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// publishSourceAgentGeneration is called only after canonical source-unit
// authentication. It never starts a service or accepts an operator path.
func publishSourceAgentGeneration(ctx context.Context, root string, source bootid.Identity) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx = bounded
	if !source.Complete() || source.PID <= 1 || brokerProtectedParent(root) != nil || !(bootid.Reader{}).ForPID(source.PID).Equal(source) {
		return ErrBoundary
	}
	fd, err := unix.Open(filepath.Join(root, "source-agent.lock"), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	var lock unix.Stat_t
	if unix.Fstat(fd, &lock) != nil || lock.Uid != 0 || lock.Mode != unix.S_IFREG|0600 || lock.Nlink != 1 {
		return ErrBoundary
	}
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EWOULDBLOCK {
			return ErrBoundary
		}
		select {
		case <-ctx.Done():
			return ErrBoundary
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer func() { _ = unix.Flock(fd, unix.LOCK_UN) }()
	current := filepath.Join(root, "source-agent-current")
	old, err := os.Readlink(current)
	if err == nil {
		if !strings.HasPrefix(old, "source-agent-") || len(old) != len("source-agent-")+64 || filepath.Base(old) != old {
			return ErrBoundary
		}
		// The existing pointer is accepted only with the complete owned immutable
		// layout; a dead previous source may be replaced without adopting its PID.
		var oldDir unix.Stat_t
		if unix.Lstat(filepath.Join(root, old), &oldDir) != nil || oldDir.Uid != 0 || oldDir.Mode != unix.S_IFDIR|0700 {
			return ErrBoundary
		}
		data, readErr := readSourceGenerationRecord(filepath.Join(root, old, "identity.json"))
		if readErr != nil || len(data) > 1024 {
			return ErrBoundary
		}
		var record sourceAgentReferenceRecord
		if json.Unmarshal(data, &record) != nil || readSourceAgentGeneration(root, record.Source, false) != nil {
			return ErrBoundary
		}
	} else if !os.IsNotExist(err) {
		return ErrBoundary
	}
	generation := sourceAgentGeneration(source)
	dir := filepath.Join(root, generation)
	if err := os.Mkdir(dir, 0700); err != nil {
		// Reuse is permitted only when already the fully authenticated current
		// generation. Unreferenced/preexisting nodes are never overwritten.
		if !os.IsExist(err) || old != generation || readSourceAgentReferenceAt(root, source) != nil {
			return ErrBoundary
		}
		return nil
	}
	data, err := json.Marshal(sourceAgentReferenceRecord{Source: source, Version: 1, Owner: "ngfw-ra-source"})
	if err != nil {
		return ErrBoundary
	}
	recordPath := filepath.Join(dir, "identity.json")
	// #nosec G304 -- fixed identity basename under a verified private root and canonical full-boot generation; exclusive creation refuses existing nodes.
	file, err := os.OpenFile(recordPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrBoundary
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrBoundary
	}
	if err := os.Symlink("/proc/"+strconv.Itoa(source.PID)+"/exe", filepath.Join(dir, "exe")); err != nil {
		return ErrBoundary
	}
	for name, target := range map[string]string{"source-agent.json": "source-agent-current/identity.json", "source-agent-exe": "source-agent-current/exe"} {
		path := filepath.Join(root, name)
		if err := os.Symlink(target, path); err != nil && (!os.IsExist(err) || !sourceOwnedLink(path, target)) {
			return ErrBoundary
		}
	}
	if !(bootid.Reader{}).ForPID(source.PID).Equal(source) {
		return ErrBoundary
	}
	temporary := filepath.Join(root, generation+".current")
	if err := os.Symlink(generation, temporary); err != nil {
		return ErrBoundary
	}
	if err := os.Rename(temporary, current); err != nil {
		return ErrBoundary
	}
	// #nosec G304 -- root has already passed protected ownership and no-link checks; this descriptor only synchronizes that fixed directory.
	directory, err := os.Open(root)
	if err != nil {
		return ErrBoundary
	}
	syncErr = directory.Sync()
	closeErr = directory.Close()
	if syncErr != nil || closeErr != nil {
		return ErrBoundary
	}
	return readSourceAgentReferenceAt(root, source)
}

func readSourceGenerationRecord(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "source generation record")
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Mode != unix.S_IFREG|0600 || stat.Nlink != 1 || stat.Size > 1024 {
		_ = file.Close()
		return nil, ErrBoundary
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	closeErr := file.Close()
	if err != nil || closeErr != nil || len(data) > 1024 {
		return nil, ErrBoundary
	}
	return data, nil
}
