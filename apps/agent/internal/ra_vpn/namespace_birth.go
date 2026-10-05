package ravpn

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const namespaceBirthReceipt = "namespace-placeholder-birth.json"

type namespaceBirthIdentity struct {
	Device uint64
	Inode  uint64
}
type namespaceBirthRecord struct {
	Version  int
	Instance string
	Bindings map[string]namespaceBirthIdentity
}

func namespaceBirthRole(role string) bool {
	return role == "hostnetns" || role == "netns" || role == "alias"
}

func readNamespaceBirth(instance string) (namespaceBirthRecord, error) {
	var record namespaceBirthRecord
	if !ValidInstance(instance) {
		return record, ErrBoundary
	}
	path := filepath.Join(InstanceRoot, instance, namespaceBirthReceipt)
	if ValidatePrivateFile(path, 4096) != nil {
		return record, ErrBoundary
	}
	data, err := trustedInstallationFile(path, 4096, false)
	if err != nil {
		return record, ErrBoundary
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF || record.Version != 1 || record.Instance != instance || len(record.Bindings) == 0 || len(record.Bindings) > 3 {
		return record, ErrBoundary
	}
	for role, identity := range record.Bindings {
		if !namespaceBirthRole(role) || identity.Inode == 0 {
			return record, ErrBoundary
		}
	}
	return record, nil
}

// Persist the exclusive ordinary-file birth before any bind mount obscures it.
func recordNamespaceBirth(instance, role string, original unix.Stat_t) error {
	if !ValidInstance(instance) || !namespaceBirthRole(role) || original.Uid != 0 || original.Mode != unix.S_IFREG|0600 || original.Nlink != 1 || original.Size != 0 || original.Ino == 0 {
		return ErrBoundary
	}
	root := filepath.Join(InstanceRoot, instance)
	if brokerProtectedParent(root) != nil {
		return ErrBoundary
	}
	path := filepath.Join(root, namespaceBirthReceipt)
	record := namespaceBirthRecord{Version: 1, Instance: instance, Bindings: map[string]namespaceBirthIdentity{}}
	if _, err := os.Lstat(path); err == nil {
		var readErr error
		record, readErr = readNamespaceBirth(instance)
		if readErr != nil {
			return ErrBoundary
		}
	} else if !os.IsNotExist(err) {
		return ErrBoundary
	}
	identity := namespaceBirthIdentity{Device: uint64(original.Dev), Inode: original.Ino}
	if prior, exists := record.Bindings[role]; exists {
		if prior == identity {
			return nil
		}
		return ErrBoundary
	}
	record.Bindings[role] = identity
	data, err := json.Marshal(record)
	if err != nil {
		return ErrBoundary
	}
	temporary := path + ".new"
	fd, err := unix.Open(temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "namespace placeholder birth")
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrBoundary
	}
	if os.Rename(temporary, path) != nil {
		return ErrBoundary
	}
	directory, err := os.Open(root)
	if err != nil {
		return ErrBoundary
	}
	syncErr := directory.Sync()
	closeErr = directory.Close()
	if syncErr != nil || closeErr != nil {
		return ErrBoundary
	}
	return nil
}

func verifyNamespaceBirth(instance, role, path string) error {
	record, err := readNamespaceBirth(instance)
	if err != nil {
		return ErrBoundary
	}
	identity, exists := record.Bindings[role]
	if !exists {
		return ErrBoundary
	}
	return verifyNamespaceBirthIdentity(path, identity)
}

func verifyNamespaceBirthIdentity(path string, identity namespaceBirthIdentity) error {
	var actual unix.Stat_t
	if unix.Lstat(path, &actual) != nil || actual.Uid != 0 || actual.Mode != unix.S_IFREG|0600 || actual.Nlink != 1 || actual.Size != 0 || actual.Ino != identity.Inode || uint64(actual.Dev) != identity.Device {
		return ErrBoundary
	}
	return nil
}
