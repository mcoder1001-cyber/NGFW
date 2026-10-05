package ravpn

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// RunNumericOpenFilePublisher serves one authenticated canonical-agent request.
// The manager preopens both named descriptors before the service drops all caps.
func RunNumericOpenFilePublisher() error {
	count, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	if os.Geteuid() != 0 || err != nil || count != 2 || len(names) != 2 || os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return ErrBoundary
	}
	roles := map[string]*os.File{}
	for index, name := range names {
		if name != numericPublisherListenerRole && name != sourceAgentExecutableRole {
			return ErrBoundary
		}
		if _, exists := roles[name]; exists {
			return ErrBoundary
		}
		fd := index + 3
		unix.CloseOnExec(fd)
		roles[name] = os.NewFile(uintptr(fd), "manager-opened publisher role")
	}
	defer func() {
		for _, file := range roles {
			_ = file.Close()
		}
	}()
	if validateNamespaceBrokerProcess(os.Getpid(), 0) != nil || numericPublisherInstallation() != nil {
		return ErrBoundary
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener := int(roles[numericPublisherListenerRole].Fd())
	kind, err := unix.GetsockoptInt(listener, unix.SOL_SOCKET, unix.SO_TYPE)
	address, addrErr := unix.Getsockname(listener)
	path, ok := address.(*unix.SockaddrUnix)
	if err != nil || kind != unix.SOCK_SEQPACKET || addrErr != nil || !ok || path.Name != numericPublisherSocketPath || boundUnitObserverSocket(ctx, listener) != nil {
		return ErrBoundary
	}
	socket, _, err := unix.Accept4(listener, unix.SOCK_CLOEXEC)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(socket) }()
	if boundUnitObserverSocket(ctx, socket) != nil {
		return ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(socket, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return ErrBoundary
	}
	// Authenticate the actual canonical peer before accepting any caller rights.
	source := (bootid.Reader{}).ForPID(int(peer.Pid))
	if verifyFixedAgentPeer(ctx, peer, source) != nil || readSourceAgentReference(source) != nil || validateSourceAgentExecutable(roles[sourceAgentExecutableRole]) != nil {
		return ErrBoundary
	}
	data, previous, err := receiveUnitObserverPacket(socket, 0)
	if err != nil {
		return ErrBoundary
	}
	closeUnitObserverFiles(previous)
	var request numericPublisherRequest
	if len(data) > numericPublisherPacketLimit || decodeUnitObserverPacket(data, &request) != nil || validateNumericPublisherRequest(request) != nil || !request.Source.Equal(source) {
		return ErrBoundary
	}
	published := false
	if request.Phase == "publish" {
		previousData, previousFiles, err := receiveUnitObserverPacket(socket, 1)
		if err != nil {
			return ErrBoundary
		}
		defer closeUnitObserverFiles(previousFiles)
		server := (bootid.Reader{}).ForPID(os.Getpid())
		if string(previousData) != "SOURCE" || request.PreviousServer.Equal(server) || (bootid.Reader{}).ForPID(request.PreviousServer.PID).Equal(request.PreviousServer) || validateSourceAgentExecutable(previousFiles[0]) != nil || !sameNumericPublisherSource(previousFiles[0], roles[sourceAgentExecutableRole]) {
			return ErrBoundary
		}
		if (FixedNumericOpenFilePublisher{}).PublishNumericOpenFile(ctx, request.Kind, request.Instance, request.Target) != nil {
			return ErrBoundary
		}
		if verifyFixedAgentPeer(ctx, peer, source) != nil || readSourceAgentReference(source) != nil || verifyNumericOpenFileTarget(ctx, request.Kind, request.Instance, request.Target) != nil || activateNumericSupplier(ctx, request.Kind, request.Target) != nil || verifyNumericOpenFileTarget(ctx, request.Kind, request.Instance, request.Target) != nil {
			return ErrBoundary
		}
		published = true
	}

	return serveNumericPublisherReply(ctx, socket, peer, request, roles[sourceAgentExecutableRole], published)
}

func serveNumericPublisherReply(ctx context.Context, socket int, peer *unix.Ucred, request numericPublisherRequest, image *os.File, published bool) error {
	server := (bootid.Reader{}).ForPID(os.Getpid())
	if numericPublisherManager(ctx, server) != nil || verifyFixedAgentPeer(ctx, peer, request.Source) != nil || readSourceAgentReference(request.Source) != nil || validateSourceAgentExecutable(image) != nil {
		return ErrBoundary
	}
	output, err := json.Marshal(numericPublisherResponse{Phase: request.Phase, Source: request.Source, Server: server, Published: published})
	if err != nil || len(output) > numericPublisherPacketLimit || unix.Sendmsg(socket, output, unix.UnixRights(int(image.Fd())), nil, 0) != nil {
		return ErrBoundary
	}
	ack, rights, err := receiveUnitObserverPacket(socket, 0)
	closeUnitObserverFiles(rights)
	if err != nil || string(ack) != "OK" || ctx.Err() != nil {
		return ErrBoundary
	}
	return nil
}

func activateNumericSupplier(ctx context.Context, kind NumericOpenFileKind, target bootid.Identity) error {
	name := ""
	switch kind {
	case NumericOpenFileTargets:
		name = "ngfw-ra-targets@" + strconv.Itoa(target.PID) + ".socket"
	case NumericOpenFileObserver:
		name = "ngfw-ra-observer@" + strconv.Itoa(target.PID) + ".socket"
	default:
		return ErrBoundary
	}
	if !validNumericOpenFileTarget(target) {
		return ErrBoundary
	}
	if exec.CommandContext(ctx, "/usr/bin/systemctl", "daemon-reload").Run() != nil {
		return ErrBoundary
	}
	// #nosec G204 -- Only the two literal supplier families plus verified canonical PID can reach this fixed command.
	if exec.CommandContext(ctx, "/usr/bin/systemctl", "start", name).Run() != nil {
		return ErrBoundary
	}
	return nil
}
