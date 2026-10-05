package ravpn

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
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
}

func (s *NamespaceTargetSnapshot) Validate() error {
	if s == nil {
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
	return nil
}

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
	return failure
}
