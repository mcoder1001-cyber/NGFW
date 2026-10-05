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
	roles := [...]struct {
		fd, kind int
		inode    uint64
	}{
		{fds[0], unix.CLONE_NEWNS, request.Target.MountInode},
		{fds[1], unix.CLONE_NEWNET, request.HostNamespace},
		{fds[2], unix.CLONE_NEWNET, request.Namespace},
		{fds[3], unix.CLONE_NEWNS, request.Source.MountInode},
	}
	seen := make(map[int]bool, len(roles))
	for _, role := range roles {
		if role.fd < 0 || seen[role.fd] || brokerNamespaceFD(role.fd, role.kind, role.inode) != nil {
			return ErrBoundary
		}
		seen[role.fd] = true
	}
	return nil
}
