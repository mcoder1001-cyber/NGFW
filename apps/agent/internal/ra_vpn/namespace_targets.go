package ravpn

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// NamespaceTargetProvider obtains both role namespaces from the manager before
// the fixed provider's capability drop. Profiles never select the unit or PID.
// Each Acquire is a fresh observation; a caller must compare a second snapshot
// after exporting, before any traffic is admitted.
type NamespaceTargetProvider interface {
	Acquire(context.Context) (*NamespaceTargetSnapshot, error)
	Preflight(context.Context) error
}

// NamespaceTargetSnapshot owns two held mount namespace descriptors, in VPP,
// manager order. Their identities remain pinned until Close. No descriptor
// number or filesystem path is accepted from configuration input.
type NamespaceTargetSnapshot struct {
	Targets [2]MountTarget
	Files   [2]*os.File
	// Source identifies the canonical agent independently of a helper requester.
	Source bootid.Identity
	// Executables pins actual VPP and canonical agent images, in that order.
	Executables [2]*os.File
}

// Validate checks all held namespace and executable roles without opening proc.
func (s *NamespaceTargetSnapshot) Validate() error {
	if s == nil || !s.Source.Complete() || s.Source.PID <= 1 {
		return ErrBoundary
	}
	for i, target := range s.Targets {
		if !target.Boot.Complete() || target.Boot.PID <= 0 || target.MountInode == 0 || s.Files[i] == nil || brokerNamespaceFD(int(s.Files[i].Fd()), unix.CLONE_NEWNS, target.MountInode) != nil {
			return ErrBoundary
		}
	}
	if s.Targets[1].Boot.PID != 1 {
		return ErrBoundary
	}
	for _, image := range s.Executables {
		if _, err := targetExecutableIdentity(image); err != nil {
			return ErrBoundary
		}
	}
	return nil
}

// SameTargets compares fresh held process, namespace and executable identities
// around a mutation. It does not claim continuous process immutability.
func (s *NamespaceTargetSnapshot) SameTargets(other *NamespaceTargetSnapshot) bool {
	if s.Validate() != nil || other.Validate() != nil || s.Targets != other.Targets || !s.Source.Equal(other.Source) {
		return false
	}
	for index, image := range s.Executables {
		before, err := targetExecutableIdentity(image)
		after, afterErr := targetExecutableIdentity(other.Executables[index])
		if err != nil || afterErr != nil || before != after {
			return false
		}
	}
	return true
}

type targetImageIdentity struct {
	Device uint64
	Inode  uint64
	Mode   uint32
	UID    uint32
	Links  uint64
}

func targetExecutableIdentity(file *os.File) (targetImageIdentity, error) {
	var stat unix.Stat_t
	if file == nil || unix.Fstat(int(file.Fd()), &stat) != nil || stat.Uid != 0 || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0022 != 0 || stat.Mode&0111 == 0 || stat.Nlink != 1 {
		return targetImageIdentity{}, ErrBoundary
	}
	return targetImageIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Mode: stat.Mode, UID: stat.Uid, Links: stat.Nlink}, nil
}

// Close releases all four held roles and is safe to call again.
func (s *NamespaceTargetSnapshot) Close() error {
	if s == nil {
		return nil
	}
	var failure error
	for i, file := range s.Files {
		if file != nil {
			if file.Close() != nil {
				failure = ErrBoundary
			}
			s.Files[i] = nil
		}
	}
	for i, file := range s.Executables {
		if file != nil {
			if file.Close() != nil {
				failure = ErrBoundary
			}
			s.Executables[i] = nil
		}
	}
	return failure
}
