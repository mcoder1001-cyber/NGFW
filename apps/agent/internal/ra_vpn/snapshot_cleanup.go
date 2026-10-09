package ravpn

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const snapshotReceiptName = "snapshot.json"

type snapshotNode struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Mode   uint32 `json:"mode"`
}
type snapshotReceipt struct {
	Instance      string                  `json:"instance"`
	Namespace     uint64                  `json:"namespace"`
	HostNamespace uint64                  `json:"hostNamespace"`
	Nodes         map[string]snapshotNode `json:"nodes"`
}

func snapshotStat(fd int, name string, directory bool) (snapshotNode, error) {
	var s unix.Stat_t
	if unix.Fstatat(fd, name, &s, unix.AT_SYMLINK_NOFOLLOW) != nil || s.Uid != 0 {
		return snapshotNode{}, ErrBoundary
	}
	mode := uint32(unix.S_IFREG | 0600)
	if directory {
		mode = unix.S_IFDIR | 0700
	}
	if s.Mode != mode || !directory && s.Nlink != 1 {
		return snapshotNode{}, ErrBoundary
	}
	// Agent-owned material must remain root-owned with exact private modes,
	// independent of its authenticated capabilities; group access is forbidden.
	return snapshotNode{uint64(s.Dev), s.Ino, s.Mode}, nil
}
func writeSnapshotReceipt(fd int, plan *NetworkPlan, files []string, directories []string) error {
	r := snapshotReceipt{Instance: plan.Instance, Namespace: plan.NamespaceInode, HostNamespace: plan.HostNamespaceInode, Nodes: map[string]snapshotNode{}}
	for _, name := range directories {
		n, e := snapshotStat(fd, name, true)
		if e != nil {
			return e
		}
		r.Nodes[name] = n
	}
	for _, name := range files {
		n, e := snapshotStat(fd, name, false)
		if e != nil {
			return e
		}
		r.Nodes[name] = n
	}
	data, e := json.Marshal(r)
	if e != nil {
		return ErrBoundary
	}
	child, e := unix.Openat(fd, snapshotReceiptName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrBoundary
	}
	f := os.NewFile(uintptr(child), snapshotReceiptName)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		_ = unix.Unlinkat(fd, snapshotReceiptName, 0)
		return ErrBoundary
	}
	return nil
}

// CleanupSnapshot removes only the exact recorded credential generation. The
// supervisor must first stop its verified daemon. A live VICI listener, replaced
// inode, unexpected directory member or namespace mismatch refuses cleanup.
func CleanupSnapshot(instance string) error {
	plan, e := ReadAgentPlan(instance)
	if e != nil {
		return ErrBoundary
	}
	root := filepath.Join(InstanceRoot, instance)
	fd, e := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	child, e := unix.Openat(fd, snapshotReceiptName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	if _, e = snapshotStat(fd, snapshotReceiptName, false); e != nil {
		_ = unix.Close(child)
		return ErrBoundary
	}
	f := os.NewFile(uintptr(child), snapshotReceiptName)
	data, e := io.ReadAll(io.LimitReader(f, 8193))
	_ = f.Close()
	if e != nil || len(data) > 8192 {
		return ErrBoundary
	}
	var r snapshotReceipt
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil || r.Instance != instance || r.Namespace != plan.NamespaceInode || r.HostNamespace != plan.HostNamespaceInode || len(r.Nodes) < 9 || len(r.Nodes) > 11 {
		return ErrBoundary
	}
	dirs := []string{"private", "x509", "x509ca", "x509crl", "daemon"}
	allowed := map[string]bool{"strongswan.conf": true, "swanctl.conf": true}
	for _, name := range dirs {
		wanted, ok := r.Nodes[name]
		actual, e := snapshotStat(fd, name, true)
		if !ok || e != nil || wanted != actual {
			return ErrBoundary
		}
		allowed[name] = true
		df, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return ErrBoundary
		}
		directory := os.NewFile(uintptr(df), name)
		entries, e := directory.ReadDir(4)
		_ = directory.Close()
		if len(entries) > 1 || e != nil && e != io.EOF {
			return ErrBoundary
		}
		for _, entry := range entries {
			n := name + "/" + entry.Name()
			if name == "daemon" {
				if entry.Name() != "vici.sock" {
					return ErrBoundary
				}
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".pem") || !credentialName.MatchString(strings.TrimSuffix(entry.Name(), ".pem")) {
				return ErrBoundary
			}
			if _, ok := r.Nodes[n]; !ok {
				return ErrBoundary
			}
			allowed[n] = true
		}
	}
	var files []string
	for name, wanted := range r.Nodes {
		if !allowed[name] {
			return ErrBoundary
		}
		if wanted.Mode&unix.S_IFMT == unix.S_IFDIR {
			continue
		}
		actual, e := snapshotStat(fd, name, false)
		if e != nil || actual != wanted {
			return ErrBoundary
		}
		files = append(files, name)
	}
	socket := filepath.Join(root, "daemon", "vici.sock")
	var sock unix.Stat_t
	if e = unix.Fstatat(fd, "daemon/vici.sock", &sock, unix.AT_SYMLINK_NOFOLLOW); e == nil {
		if sock.Uid != 0 || sock.Gid != 0 || sock.Mode != unix.S_IFSOCK|0600 || sock.Nlink != 1 {
			return ErrBoundary
		}
		connection, e := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if e == nil {
			_ = connection.Close()
			return ErrBoundary
		}
		// Only a positively refused listener is safe; timeout/permissions are not inactivity.
		op, ok := e.(*net.OpError)
		if !ok {
			return ErrBoundary
		}
		se, ok := op.Err.(*os.SyscallError)
		if !ok || se.Err != unix.ECONNREFUSED {
			return ErrBoundary
		}
		var again unix.Stat_t
		if unix.Fstatat(fd, "daemon/vici.sock", &again, unix.AT_SYMLINK_NOFOLLOW) != nil || again.Ino != sock.Ino || again.Dev != sock.Dev {
			return ErrBoundary
		}
		if unix.Unlinkat(fd, "daemon/vici.sock", 0) != nil {
			return ErrBoundary
		}
	} else if e != unix.ENOENT {
		return ErrBoundary
	}
	sort.Strings(files)
	for _, name := range files {
		if unix.Unlinkat(fd, name, 0) != nil {
			return ErrBoundary
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if unix.Unlinkat(fd, dirs[i], unix.AT_REMOVEDIR) != nil {
			return ErrBoundary
		}
	}
	if unix.Unlinkat(fd, snapshotReceiptName, 0) != nil || unix.Fsync(fd) != nil {
		return ErrBoundary
	}
	return nil
}

// verifySnapshotContents is for restart adoption. The immutable sealed-cache
// generation must exactly reproduce every credential and configuration byte.
func verifySnapshotContents(instance string, expected map[string][]byte) error {
	plan, e := ReadAgentPlan(instance)
	if e != nil {
		return ErrBoundary
	}
	fd, e := unix.Open(filepath.Join(InstanceRoot, instance), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	child, e := unix.Openat(fd, snapshotReceiptName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	f := os.NewFile(uintptr(child), snapshotReceiptName)
	var st unix.Stat_t
	if unix.Fstat(child, &st) != nil || st.Mode != unix.S_IFREG|0600 || st.Uid != 0 || st.Nlink != 1 || st.Size > 8192 {
		_ = f.Close()
		return ErrBoundary
	}
	data, e := io.ReadAll(io.LimitReader(f, 8193))
	_ = f.Close()
	if e != nil || len(data) > 8192 {
		return ErrBoundary
	}
	var r snapshotReceipt
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || r.Instance != instance || r.Namespace != plan.NamespaceInode || r.HostNamespace != plan.HostNamespaceInode || len(r.Nodes) != len(expected)+5 {
		return ErrBoundary
	}
	for _, name := range []string{"private", "x509", "x509ca", "x509crl", "daemon"} {
		actual, e := snapshotStat(fd, name, true)
		if e != nil || actual != r.Nodes[name] {
			return ErrBoundary
		}
		df, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return ErrBoundary
		}
		directory := os.NewFile(uintptr(df), name)
		entries, e := directory.ReadDir(4)
		_ = directory.Close()
		if len(entries) > 1 || e != nil && e != io.EOF {
			return ErrBoundary
		}
		for _, entry := range entries {
			path := name + "/" + entry.Name()
			if name == "daemon" {
				if entry.Name() != "vici.sock" {
					return ErrBoundary
				}
				var sock unix.Stat_t
				if unix.Fstatat(fd, path, &sock, unix.AT_SYMLINK_NOFOLLOW) != nil || sock.Mode != unix.S_IFSOCK|0600 || sock.Uid != 0 || sock.Gid != 0 || sock.Nlink != 1 {
					return ErrBoundary
				}
				continue
			}
			if _, ok := expected[path]; !ok {
				return ErrBoundary
			}
		}
	}
	for name, wanted := range expected {
		// Names originate only from the validated renderer and credential names.
		actual, e := snapshotStat(fd, name, false)
		if e != nil || actual != r.Nodes[name] {
			return ErrBoundary
		}
		child, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return ErrBoundary
		}
		f := os.NewFile(uintptr(child), name)
		var held unix.Stat_t
		if unix.Fstat(child, &held) != nil || held.Ino != actual.Inode || uint64(held.Dev) != actual.Device {
			_ = f.Close()
			return ErrBoundary
		}
		data, e := io.ReadAll(io.LimitReader(f, int64(len(wanted)+1)))
		_ = f.Close()
		equal := e == nil && bytes.Equal(data, wanted)
		clear(data)
		if !equal {
			return ErrBoundary
		}
	}
	return nil
}
