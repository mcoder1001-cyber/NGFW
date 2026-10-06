package ravpn

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"log"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// RunNumericOpenFilePublisher serves one authenticated canonical-agent request.
// The manager preopens both named descriptors before the service drops all caps.
func RunNumericOpenFilePublisher() (result error) {
	started := time.Now()
	wholeContext, wholeCancel := context.WithTimeout(context.Background(), NumericPublisherServerWholeBudget)
	defer wholeCancel()
	stage := NumericPublisherServerRoles
	checkpoint16 := uint8(0)
	var diagnosticState numericPublisherDiagnosticState
	diagnosticContext := context.Background()
	setStage := func(value NumericPublisherServerStage) {
		stage = value
		log.Printf("remote-access publisher-server entry_stage=%d deadline_exceeded=%t elapsed_ms=%d", stage, diagnosticContext.Err() == context.DeadlineExceeded, time.Since(started).Milliseconds())
	}
	setStage(NumericPublisherServerRoles)
	defer func() {
		if result != nil {
			if stage == NumericPublisherServerPostTarget {
				if line := diagnosticState.line(2, checkpoint16, time.Since(started), true); line != "" {
					log.Print(line)
				}
			}
			innerStage := uint8(0)
			var failure *NumericPublisherFailure
			if errors.As(result, &failure) && failure.Stage >= 1 && failure.Stage <= 24 {
				innerStage = failure.Stage
			}
			log.Printf("remote-access publisher-server stage=%d deadline_exceeded=%t elapsed_ms=%d inner_stage=%d", stage, diagnosticContext.Err() == context.DeadlineExceeded, time.Since(started).Milliseconds(), innerStage)
		}
	}()

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
	setStage(NumericPublisherServerCaps)
	if validateNamespaceBrokerProcess(os.Getpid(), 0) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerInstallation)
	validationContext, validationCancel := context.WithTimeout(wholeContext, NumericPublisherValidationBudget)
	defer func() { diagnosticState.captureBeforeCancel(diagnosticContext, result != nil); validationCancel() }()
	diagnosticContext = validationContext
	proof, err := newNumericPublisherInstallationProof(validationContext)
	if err != nil {
		return err
	}
	defer func() {
		if proof.Close() != nil {
			result = ErrBoundary
		}
	}()
	ctx := validationContext
	diagnosticContext = ctx
	setStage(NumericPublisherServerListener)
	listener := int(roles[numericPublisherListenerRole].Fd())
	kind, err := unix.GetsockoptInt(listener, unix.SOL_SOCKET, unix.SO_TYPE)
	address, addrErr := unix.Getsockname(listener)
	path, ok := address.(*unix.SockaddrUnix)
	if err != nil || kind != unix.SOCK_SEQPACKET || addrErr != nil || !ok || path.Name != numericPublisherSocketPath || boundUnitObserverSocket(ctx, listener) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerAccept)
	socket, _, err := unix.Accept4(listener, unix.SOCK_CLOEXEC)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(socket) }()
	stopPeer := watchNumericPublisherPeer(wholeContext, socket, wholeCancel)
	defer stopPeer()
	if boundUnitObserverSocket(ctx, socket) != nil {
		return ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(socket, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return ErrBoundary
	}
	// Authenticate the actual canonical peer before accepting any caller rights.
	source := (bootid.Reader{}).ForPID(int(peer.Pid))
	setStage(NumericPublisherServerPeer)
	trust, peerErr := numericPublisherSourceTrust(ctx, peer, source, proof)
	if peerErr != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerReference)
	if readSourceAgentReference(source) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerSourceImage)
	if validateSourceAgentExecutable(roles[sourceAgentExecutableRole]) != nil {
		return ErrBoundary
	}
	server := (bootid.Reader{}).ForPID(os.Getpid())
	setStage(NumericPublisherServerProof)
	if proof.Verify(ctx) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerManager)
	if err := numericPublisherManagerFromTrust(ctx, server, proof, trust); err != nil {
		return err
	}
	ready, readyErr := json.Marshal(numericPublisherReady{Version: 1, Phase: "validation-ready", Source: source, Server: server})
	if readyErr != nil || unix.Sendmsg(socket, ready, unix.UnixRights(int(roles[sourceAgentExecutableRole].Fd())), nil, 0) != nil {
		return ErrBoundary
	}
	if boundNumericPublisherValidationSocket(validationContext, socket) != nil {
		return ErrBoundary
	}
	accepted, acceptedRights, acceptedErr := receiveUnitObserverPacket(socket, 0)
	closeUnitObserverFiles(acceptedRights)
	if acceptedErr != nil || string(accepted) != "READY" || validationContext.Err() != nil || proof.Verify(validationContext) != nil {
		return ErrBoundary
	}
	ipcContext, ipcCancel := context.WithTimeout(wholeContext, NumericPublisherIPCBudget)
	defer func() { diagnosticState.captureBeforeCancel(diagnosticContext, result != nil); ipcCancel() }()
	ctx = ipcContext
	diagnosticContext = ctx
	if boundUnitObserverSocket(ctx, socket) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerRequest)
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
		setStage(NumericPublisherServerPrevious)
		previousData, previousFiles, err := receiveUnitObserverPacket(socket, 1)
		if err != nil {
			return ErrBoundary
		}
		defer closeUnitObserverFiles(previousFiles)
		server := (bootid.Reader{}).ForPID(os.Getpid())
		if string(previousData) != "SOURCE" || request.PreviousServer.Equal(server) || (bootid.Reader{}).ForPID(request.PreviousServer.PID).Equal(request.PreviousServer) || validateSourceAgentExecutable(previousFiles[0]) != nil || !sameNumericPublisherSource(previousFiles[0], roles[sourceAgentExecutableRole]) {
			return ErrBoundary
		}
		if ctx.Err() != nil {
			return ErrBoundary
		}
		workContext, workCancel := context.WithTimeout(wholeContext, NumericPublisherWorkBudget)
		defer func() { diagnosticState.captureBeforeCancel(diagnosticContext, result != nil); workCancel() }()
		ctx = workContext
		diagnosticContext = ctx
		if boundNumericPublisherWorkSocket(ctx, socket) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerProof)
		if proof.Verify(ctx) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerPublish)
		if (FixedNumericOpenFilePublisher{}).PublishNumericOpenFile(ctx, request.Kind, request.Instance, request.Target) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerPostPeer)
		if verifyFixedAgentPeer(ctx, peer, source) != nil || readSourceAgentReference(source) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerTarget)
		if verifyNumericOpenFileTarget(ctx, request.Kind, request.Instance, request.Target) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerActivate)
		if activateNumericSupplier(ctx, request.Kind, request.Target) != nil {
			return ErrBoundary
		}
		setStage(NumericPublisherServerPostTarget)
		checkpoint16 = 1
		if verifyNumericOpenFileTarget(ctx, request.Kind, request.Instance, request.Target) != nil {
			return ErrBoundary
		}
		checkpoint16 = 2
		workTrust, trustErr := numericPublisherManagerTrust(ctx, server, proof, source)
		if trustErr != nil {
			return ErrBoundary
		}
		checkpoint16 = 3
		if numericPublisherPeerFromTrust(ctx, peer, source, workTrust) != nil {
			return ErrBoundary
		}
		checkpoint16 = 4
		if readSourceAgentReference(source) != nil {
			return ErrBoundary
		}
		checkpoint16 = 5
		if validateSourceAgentExecutable(roles[sourceAgentExecutableRole]) != nil {
			return ErrBoundary
		}
		checkpoint16 = 6
		if proof.Verify(ctx) != nil {
			return ErrBoundary
		}
		checkpoint16 = 7
		token, tokenErr := newNumericPublisherWorkToken()
		if tokenErr != nil {
			return ErrBoundary
		}
		checkpoint16 = 8
		if sendNumericPublisherWorkFrame(socket, "publication-complete", source, server, token, int(roles[sourceAgentExecutableRole].Fd())) != nil {
			return ErrBoundary
		}
		checkpoint16 = 9
		ackData, ackRights, ackErr := receiveUnitObserverPacket(socket, 0)
		closeUnitObserverFiles(ackRights)
		var ack numericPublisherWorkFrame
		if ackErr != nil {
			return ErrBoundary
		}
		checkpoint16 = 10
		if decodeUnitObserverPacket(ackData, &ack) != nil {
			return ErrBoundary
		}
		checkpoint16 = 11
		if validateNumericPublisherWorkFrame(ack, "publication-ack", source, server, token) != nil {
			return ErrBoundary
		}
		checkpoint16 = 12
		if ctx.Err() != nil {
			return ErrBoundary
		}
		checkpoint16 = 13
		if noNumericPublisherQueuedInput(socket) != nil {
			return ErrBoundary
		}
		checkpoint16 = 14
		if proof.Verify(ctx) != nil {
			return ErrBoundary
		}
		checkpoint16 = 15
		if verifyFixedAgentPeer(ctx, peer, source) != nil {
			return ErrBoundary
		}
		checkpoint16 = 16
		if readSourceAgentReference(source) != nil {
			return ErrBoundary
		}
		checkpoint16 = 17
		finalContext, finalCancel := context.WithTimeout(wholeContext, NumericPublisherIPCBudget)
		defer func() { diagnosticState.captureBeforeCancel(diagnosticContext, result != nil); finalCancel() }()
		ctx = finalContext
		diagnosticContext = ctx
		checkpoint16 = 18
		if boundUnitObserverSocket(ctx, socket) != nil {
			return ErrBoundary
		}
		published = true
	}

	return serveNumericPublisherReply(ctx, socket, peer, request, roles[sourceAgentExecutableRole], published, proof, setStage, stopPeer)
}

func serveNumericPublisherReply(ctx context.Context, socket int, peer *unix.Ucred, request numericPublisherRequest, image *os.File, published bool, proof *numericPublisherInstallationProof, setStage func(NumericPublisherServerStage), stopPeer numericPublisherPeerStop) error {
	server := (bootid.Reader{}).ForPID(os.Getpid())
	setStage(NumericPublisherServerManager)
	trust, trustErr := numericPublisherManagerTrust(ctx, server, proof, request.Source)
	if trustErr != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerReplyPeer)
	if numericPublisherPeerFromTrust(ctx, peer, request.Source, trust) != nil || readSourceAgentReference(request.Source) != nil || validateSourceAgentExecutable(image) != nil {
		return ErrBoundary
	}
	if proof.Verify(ctx) != nil || enterNumericPublisherTerminal(ctx, socket, stopPeer) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerSend)
	if published && noNumericPublisherQueuedInput(socket) != nil {
		return ErrBoundary
	}
	output, err := json.Marshal(numericPublisherResponse{Phase: request.Phase, Source: request.Source, Server: server, Published: published})
	if err != nil || len(output) > numericPublisherPacketLimit || unix.Sendmsg(socket, output, unix.UnixRights(int(image.Fd())), nil, 0) != nil {
		return ErrBoundary
	}
	setStage(NumericPublisherServerAck)
	if receiveNumericPublisherTerminalACK(ctx, socket) != nil || proof.Verify(ctx) != nil {
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
	log.Print("remote-access publisher-server activation_step=1")
	reload := exec.CommandContext(ctx, "/usr/bin/systemctl", "daemon-reload")
	reload.WaitDelay = time.Second
	if reload.Run() != nil {
		return ErrBoundary
	}
	// #nosec G204 -- Only the two literal supplier families plus verified canonical PID can reach this fixed command.
	start := exec.CommandContext(ctx, "/usr/bin/systemctl", "start", name)
	start.WaitDelay = time.Second
	log.Print("remote-access publisher-server activation_step=2")
	if start.Run() != nil {
		return ErrBoundary
	}
	return nil
}
