package ravpn

import (
	"context"

	"golang.org/x/sys/unix"
)

// NamespaceBrokerAttestedFDDispatch extends the archived three-descriptor
// protocol with the authenticated agent's own mount namespace. The agent opens
// that descriptor internally; no profile supplies a PID, path or descriptor.
// Ordered roles are target MNT, owned host NET, owned private NET, source MNT.
// The receiver must reject missing, extra, repeated or incorrectly typed FDs.
type NamespaceBrokerAttestedFDDispatch interface {
	Preflight(context.Context) error
	RunAttestedFDs(context.Context, NamespaceBrokerMessage, [4]int) error
}

func validateAttestedBrokerFDs(request NamespaceBrokerMessage, fds [4]int) error {
	if !request.Source.Boot.Complete() || request.Source.MountInode == 0 || !request.Target.Boot.Complete() || request.Target.MountInode == 0 || request.HostNamespace == 0 || request.Namespace == 0 || request.HostNamespace == request.Namespace {
		return ErrBoundary
	}
	if brokerNamespaceFD(fds[3], unix.CLONE_NEWNS, request.Source.MountInode) != nil {
		return ErrBoundary
	}
	kinds := [4]int{unix.CLONE_NEWNS, unix.CLONE_NEWNET, unix.CLONE_NEWNET, unix.CLONE_NEWNS}
	inodes := [4]uint64{request.Target.MountInode, request.HostNamespace, request.Namespace, request.Source.MountInode}
	for index, fd := range fds {
		if fd < 0 || brokerNamespaceFD(fd, kinds[index], inodes[index]) != nil {
			return ErrBoundary
		}
		for previous := 0; previous < index; previous++ {
			if fds[previous] == fd {
				return ErrBoundary
			}
		}
	}
	return nil
}
