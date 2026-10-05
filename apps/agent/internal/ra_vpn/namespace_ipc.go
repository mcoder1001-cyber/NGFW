package ravpn

import (
	"math"
	"strconv"
)

// NamespaceIPCRoot is the fixed root-only transport directory outside the
// original agent RuntimeDirectory=ngfw. Agent starts/restarts must not change
// the ownership of manager-created sockets. Instance/source files remain in
// their existing protected root:ngfw directory; no privilege is added.
const NamespaceIPCRoot = "/run/ngfw-ra-ipc"

// NamespaceTargetIPCRoot contains only canonical numeric VPP supplier sockets.
const NamespaceTargetIPCRoot = NamespaceIPCRoot + "/targets"

// NamespaceObserverIPCRoot contains only canonical numeric RA observer sockets.
const NamespaceObserverIPCRoot = NamespaceIPCRoot + "/observers"

// NamespacePublisherSocketPath is the one fixed authenticated publisher socket.
const NamespacePublisherSocketPath = NamespaceIPCRoot + "/openfile.sock"

// NamespaceBrokerSocketPath is the one fixed authenticated namespace FD socket.
const NamespaceBrokerSocketPath = NamespaceIPCRoot + "/namespace.sock"

// NamespaceTargetSocket derives a supplier socket from a validated kernel PID.
// It accepts no caller-selected path, unit, role or arbitrary string.
func NamespaceTargetSocket(pid int) (string, error) {
	if pid <= 1 || pid > math.MaxInt32 {
		return "", ErrBoundary
	}
	return NamespaceTargetIPCRoot + "/" + strconv.Itoa(pid) + ".sock", nil
}

// NamespaceObserverSocket derives an observer socket from a validated kernel PID.
func NamespaceObserverSocket(pid int) (string, error) {
	if pid <= 1 || pid > math.MaxInt32 {
		return "", ErrBoundary
	}
	return NamespaceObserverIPCRoot + "/" + strconv.Itoa(pid) + ".sock", nil
}
