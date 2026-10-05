package ravpn

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var tapReceiptName = regexp.MustCompile(`^ra_[a-f0-9]{24}[oi]$`)

// FileTAPReceipts keeps a held root-owned directory descriptor. Daemon units
// hide the agent StateDir entirely; the daemon cannot forge or erase claims.
type FileTAPReceipts struct{ fd int }

// Persistent reports that receipts survive agent restart.
func (*FileTAPReceipts) Persistent() bool { return true }

// LazyTAPReceipts keeps production registration host independent. Every actual
// operation opens and validates root-owned parents before reading or mutating
// the persisted claim; an unsafe state directory can never reach a VPP call.
type LazyTAPReceipts struct{ StateDir string }

// Persistent validates the configured durable receipt location.
func (s *LazyTAPReceipts) Persistent() bool {
	return s != nil && filepath.IsAbs(s.StateDir) && filepath.Clean(s.StateDir) == s.StateDir && s.StateDir != "/"
}

// Load opens protected storage and reads one owned receipt.
func (s *LazyTAPReceipts) Load(name string) (receipt TAPReceipt, failure error) {
	store, err := NewFileTAPReceipts(s.StateDir)
	if err != nil {
		return TAPReceipt{}, err
	}
	defer func() { failure = errors.Join(failure, store.Close()) }()
	return store.Load(name)
}

// Save opens protected storage and persists one owned receipt.
func (s *LazyTAPReceipts) Save(name string, receipt TAPReceipt) (failure error) {
	store, err := NewFileTAPReceipts(s.StateDir)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, store.Close()) }()
	return store.Save(name, receipt)
}

// Remove opens protected storage and removes one verified receipt.
func (s *LazyTAPReceipts) Remove(name string) (failure error) {
	store, err := NewFileTAPReceipts(s.StateDir)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, store.Close()) }()
	return store.Remove(name)
}

// NewFileTAPReceipts opens a root-private directory through held parent descriptors.
func NewFileTAPReceipts(stateDir string) (*FileTAPReceipts, error) {
	if os.Geteuid() != 0 || !filepath.IsAbs(stateDir) || filepath.Clean(stateDir) != stateDir || stateDir == "/" {
		return nil, ErrBoundary
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	for _, component := range splitPath(stateDir) {
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		closeErr := unix.Close(fd)
		if closeErr != nil {
			if err == nil {
				_ = unix.Close(next)
			}
			return nil, ErrBoundary
		}
		if err != nil {
			return nil, ErrBoundary
		}
		fd = next
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Mode&0022 != 0 {
			_ = unix.Close(fd)
			return nil, ErrBoundary
		}
	}
	err = unix.Mkdirat(fd, "ra-taps", 0700)
	if err != nil && err != unix.EEXIST {
		_ = unix.Close(fd)
		return nil, ErrBoundary
	}
	next, err := unix.Openat(fd, "ra-taps", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	closeErr := unix.Close(fd)
	if closeErr != nil {
		if err == nil {
			_ = unix.Close(next)
		}
		return nil, ErrBoundary
	}
	if err != nil {
		return nil, ErrBoundary
	}
	var stat unix.Stat_t
	if unix.Fstat(next, &stat) != nil || stat.Uid != 0 || stat.Mode&0077 != 0 {
		_ = unix.Close(next)
		return nil, ErrBoundary
	}
	return &FileTAPReceipts{fd: next}, nil
}
func splitPath(path string) []string {
	var components []string
	for path != "/" {
		components = append([]string{filepath.Base(path)}, components...)
		path = filepath.Dir(path)
	}
	return components
}

// Close releases the held receipt directory.
func (s *FileTAPReceipts) Close() error { return unix.Close(s.fd) }

// Load validates an owned receipt before returning its public metadata.
func (s *FileTAPReceipts) Load(name string) (receipt TAPReceipt, failure error) {
	if !tapReceiptName.MatchString(name) {
		return receipt, ErrBoundary
	}
	fd, err := unix.Openat(s.fd, name+".json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err == unix.ENOENT {
		return receipt, os.ErrNotExist
	}
	if err != nil {
		return receipt, ErrBoundary
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { failure = errors.Join(failure, file.Close()) }()
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != 0 || stat.Mode&0077 != 0 || stat.Nlink != 1 || stat.Size > 16384 {
		return receipt, ErrBoundary
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 {
		return receipt, ErrBoundary
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || !validReceipt(name, receipt) {
		return TAPReceipt{}, ErrBoundary
	}
	return receipt, nil
}
func validReceipt(name string, receipt TAPReceipt) bool {
	return tapReceiptName.MatchString(name) && ValidInstance(receipt.Instance) && receipt.NamespaceInode != 0 && receipt.HostNamespaceInode != 0 && receipt.NamespaceInode != receipt.HostNamespaceInode && receipt.Boot.Complete() && receipt.Boot.PID > 0 && receipt.Endpoint != nil && receipt.Endpoint.Name == name && receipt.Endpoint.HostNamespace == NamespacePath(receipt.Instance) && (name == LinkName(receipt.Instance, true) || name == LinkName(receipt.Instance, false))
}

// Save atomically persists a receipt without adopting a foreign generation.
func (s *FileTAPReceipts) Save(name string, receipt TAPReceipt) (failure error) {
	if !validReceipt(name, receipt) {
		return ErrBoundary
	}
	if previous, err := s.Load(name); err == nil {
		if previous.Instance != receipt.Instance || previous.NamespaceInode != receipt.NamespaceInode || previous.HostNamespaceInode != receipt.HostNamespaceInode || !previous.Boot.Equal(receipt.Boot) || VerifyTransitTAP(previous.Endpoint, receipt.Endpoint) != nil {
			return ErrBoundary
		}
	} else if !os.IsNotExist(err) {
		return ErrBoundary
	}
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > 16384 {
		return ErrBoundary
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return ErrBoundary
	}
	temporary := ".receipt-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(s.fd, temporary, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrBoundary
	}
	defer func() {
		if err := unix.Unlinkat(s.fd, temporary, 0); err != nil && err != unix.ENOENT {
			failure = errors.Join(failure, err)
		}
	}()
	file := os.NewFile(uintptr(fd), temporary)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrBoundary
	}
	if unix.Renameat(s.fd, temporary, s.fd, name+".json") != nil || unix.Fsync(s.fd) != nil {
		return ErrBoundary
	}
	return nil
}

// Remove validates ownership before unlinking the receipt.
func (s *FileTAPReceipts) Remove(name string) error {
	if _, err := s.Load(name); err != nil {
		return err
	}
	if unix.Unlinkat(s.fd, name+".json", 0) != nil || unix.Fsync(s.fd) != nil {
		return ErrBoundary
	}
	return nil
}
