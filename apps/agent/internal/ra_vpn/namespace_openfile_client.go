package ravpn

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"math"
	"ngfw/agent/internal/vpp/bootid"
	"os"
)

func publishNumericThroughManager(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity) (result error) {
	trace := newNumericPublisherClientTiming()
	defer func() { trace.logFailure(result != nil, ctx.Err() == context.DeadlineExceeded) }()
	gaps := newNumericPublisherGapTiming(ctx, trace.started)
	defer func() { gaps.logFailure(result != nil) }()
	bounded, cancel := context.WithTimeout(ctx, NumericOpenFilePublicationBudget)
	defer cancel()
	proof, err := newNumericPublisherInstallationProof(bounded)
	if err != nil {
		return err
	}
	trace.markPreflight(1)
	defer func() {
		if proof.Close() != nil {
			result = ErrBoundary
		}
	}()
	pid, gid := os.Getpid(), os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || gid < 0 || gid > math.MaxUint32 {
		return numericPublisherFailure(bounded, 1)
	}
	source := (bootid.Reader{}).ForPID(pid)
	trust, peerErr := numericPublisherSourceTrust(bounded, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, source, proof)
	if peerErr != nil {
		return numericPublisherFailure(bounded, 2)
	}
	if readSourceAgentReference(source) != nil {
		return numericPublisherFailure(bounded, 3)
	}
	trace.markPreflight(2)
	if err := numericPublisherManagerFromTrust(bounded, bootid.Identity{}, proof, trust); err != nil {
		return err
	}
	trace.markPreflight(3)
	probe := numericPublisherRequest{Phase: "probe", Source: source}
	first, image, err := numericPublisherExchange(bounded, probe, nil, proof, trace, gaps, 0)
	if err != nil {
		return err
	}
	defer func() { _ = image.Close() }()
	if err := waitNumericPublisherExit(bounded, first.Server); err != nil {
		return err
	}
	gaps.mark(2)
	request := numericPublisherRequest{Phase: "publish", Source: source, Kind: kind, Instance: instance, Target: target, PreviousServer: first.Server}
	if validateNumericPublisherRequest(request) != nil {
		return numericPublisherFailure(bounded, 11)
	}
	second, fresh, err := numericPublisherExchange(bounded, request, image, proof, trace, gaps, 1)
	if err != nil {
		return err
	}
	defer func() { _ = fresh.Close() }()
	if second.Server.Equal(first.Server) || !second.Published || !sameNumericPublisherSource(image, fresh) || !(bootid.Reader{}).ForPID(pid).Equal(source) || readSourceAgentReference(source) != nil {
		return numericPublisherFailure(bounded, 24)
	}
	return nil
}

func numericPublisherExchange(ctx context.Context, request numericPublisherRequest, previous *os.File, proof *numericPublisherInstallationProof, trace *numericPublisherClientTiming, gaps *numericPublisherGapTiming, phase int) (numericPublisherResponse, *os.File, error) {
	publicationContext := ctx
	bounded, cancel := context.WithTimeout(ctx, NumericPublisherValidationBudget)
	defer cancel()
	ctx = bounded
	var empty numericPublisherResponse
	if proof.Verify(ctx) != nil {
		return empty, nil, numericPublisherFailure(ctx, 4)
	}
	if validateNumericPublisherRequest(request) != nil || brokerProtectedParent(NamespaceIPCRoot) != nil {
		return empty, nil, numericPublisherFailure(ctx, 11)
	}
	var stat unix.Stat_t
	if unix.Lstat(numericPublisherSocketPath, &stat) != nil || stat.Mode != unix.S_IFSOCK|0600 || stat.Uid != 0 || stat.Gid != 0 {
		return empty, nil, numericPublisherFailure(ctx, 12)
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return empty, nil, numericPublisherFailure(ctx, 13)
	}
	defer func() { _ = unix.Close(fd) }()
	stopCancellation := watchNumericPublisherCancellation(publicationContext, fd)
	defer stopCancellation()
	trace.markExchange(phase, 0)
	if boundNumericPublisherValidationSocket(ctx, fd) != nil || unix.Connect(fd, &unix.SockaddrUnix{Name: numericPublisherSocketPath}) != nil {
		return empty, nil, numericPublisherFailure(ctx, 14)
	}
	trace.markExchange(phase, 1)
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid != 1 {
		return empty, nil, numericPublisherFailure(ctx, 15)
	}
	readyData, readyFiles, readyErr := receiveUnitObserverPacket(fd, 1)
	if readyErr != nil {
		return empty, nil, numericPublisherFailure(ctx, 19)
	}
	trace.markExchange(phase, 2)
	defer closeUnitObserverFiles(readyFiles)
	var ready numericPublisherReady
	if decodeUnitObserverPacket(readyData, &ready) != nil || validateNumericPublisherReady(ready, request.Source) != nil || validateSourceAgentExecutable(readyFiles[0]) != nil {
		return empty, nil, numericPublisherFailure(ctx, 20)
	}
	if err := numericPublisherManagerWithProof(ctx, ready.Server, proof); err != nil {
		return empty, nil, err
	}
	trace.markExchange(phase, 3)
	if unix.Sendmsg(fd, []byte("READY"), nil, nil, 0) != nil {
		return empty, nil, numericPublisherFailure(ctx, 17)
	}
	ipc, ipcCancel := context.WithTimeout(publicationContext, NumericPublisherIPCBudget)
	defer ipcCancel()
	ctx = ipc
	if boundUnitObserverSocket(ctx, fd) != nil {
		return empty, nil, numericPublisherFailure(ctx, 14)
	}
	content, err := json.Marshal(request)
	if err != nil || len(content) > numericPublisherPacketLimit {
		return empty, nil, numericPublisherFailure(ctx, 16)
	}
	if unix.Sendmsg(fd, content, nil, nil, 0) != nil {
		return empty, nil, numericPublisherFailure(ctx, 17)
	}
	trace.markExchange(phase, 4)
	if previous != nil && unix.Sendmsg(fd, []byte("SOURCE"), unix.UnixRights(int(previous.Fd())), nil, 0) != nil {
		return empty, nil, numericPublisherFailure(ctx, 18)
	}

	if request.Phase == "publish" {
		if ctx.Err() != nil {
			return empty, nil, numericPublisherFailure(ctx, 19)
		}
		work, workCancel := context.WithTimeout(publicationContext, NumericPublisherWorkBudget)
		defer workCancel()
		if boundNumericPublisherWorkSocket(work, fd) != nil {
			return empty, nil, numericPublisherFailure(work, 14)
		}
		completed, images, workErr := receiveUnitObserverPacket(fd, 1)
		if workErr != nil {
			return empty, nil, numericPublisherFailure(work, 19)
		}
		defer closeUnitObserverFiles(images)
		var frame numericPublisherWorkFrame
		if decodeUnitObserverPacket(completed, &frame) != nil || validateNumericPublisherWorkFrame(frame, "publication-complete", request.Source, ready.Server, frame.Token) != nil || previous == nil || !sameNumericPublisherSource(images[0], readyFiles[0]) || !sameNumericPublisherSource(images[0], previous) || validateSourceAgentExecutable(images[0]) != nil {
			return empty, nil, numericPublisherFailure(work, 20)
		}
		trust, err := numericPublisherManagerTrust(work, frame.Server, proof, request.Source)
		if err != nil {
			return empty, nil, err
		}
		pid, gid := request.Source.PID, os.Getegid()
		if pid <= 1 || pid > math.MaxInt32 || gid < 0 || gid > math.MaxUint32 || numericPublisherPeerFromTrust(work, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, request.Source, trust) != nil || readSourceAgentReference(request.Source) != nil || proof.Verify(work) != nil || sendNumericPublisherWorkFrame(fd, "publication-ack", request.Source, ready.Server, frame.Token, -1) != nil {
			return empty, nil, numericPublisherFailure(work, 22)
		}
		final, finalCancel := context.WithTimeout(publicationContext, NumericPublisherIPCBudget)
		defer finalCancel()
		ctx = final
		if boundUnitObserverSocket(ctx, fd) != nil {
			return empty, nil, numericPublisherFailure(ctx, 14)
		}
	}
	data, files, err := receiveUnitObserverPacket(fd, 1)
	if err != nil {
		return empty, nil, numericPublisherFailure(ctx, 19)
	}
	switch phase {
	case 0:
		gaps.mark(0)
	case 1:
		gaps.mark(3)
	}
	success := false
	defer func() {
		if !success {
			closeUnitObserverFiles(files)
		}
	}()
	var response numericPublisherResponse
	if len(data) > numericPublisherPacketLimit || decodeUnitObserverPacket(data, &response) != nil || response.Phase != request.Phase || !response.Source.Equal(request.Source) || !response.Server.Equal(ready.Server) || !sameNumericPublisherSource(readyFiles[0], files[0]) {
		return empty, nil, numericPublisherFailure(ctx, 20)
	}
	trust, err := numericPublisherManagerTrust(ctx, response.Server, proof, request.Source)
	if err != nil {
		return empty, nil, err
	}
	if validateSourceAgentExecutable(files[0]) != nil {
		return empty, nil, numericPublisherFailure(ctx, 21)
	}
	if request.Phase == "probe" && response.Published {
		return empty, nil, numericPublisherFailure(ctx, 20)
	}
	if request.Phase == "publish" && (!response.Published || previous == nil || !sameNumericPublisherSource(previous, files[0])) {
		return empty, nil, numericPublisherFailure(ctx, 20)
	}
	pid, gid := request.Source.PID, os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || gid < 0 || gid > math.MaxUint32 {
		return empty, nil, numericPublisherFailure(ctx, 1)
	}
	if numericPublisherPeerFromTrust(ctx, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, request.Source, trust) != nil || readSourceAgentReference(request.Source) != nil || unix.Sendmsg(fd, []byte("OK"), nil, nil, 0) != nil {
		return empty, nil, numericPublisherFailure(ctx, 22)
	}
	if proof.Verify(ctx) != nil {
		return empty, nil, numericPublisherFailure(ctx, 4)
	}
	switch phase {
	case 0:
		gaps.mark(1)
	case 1:
		gaps.mark(4)
	}
	success = true
	return response, files[0], nil
}
