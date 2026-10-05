package ravpn

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"math"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"time"
)

func publishNumericThroughManager(ctx context.Context, kind NumericOpenFileKind, instance string, target bootid.Identity) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pid, gid := os.Getpid(), os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || gid < 0 || gid > math.MaxUint32 {
		return ErrBoundary
	}
	source := (bootid.Reader{}).ForPID(pid)
	if verifyFixedAgentPeer(bounded, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, source) != nil || readSourceAgentReference(source) != nil || numericPublisherManager(bounded, bootid.Identity{}) != nil {
		return ErrBoundary
	}
	probe := numericPublisherRequest{Phase: "probe", Source: source}
	first, image, err := numericPublisherExchange(bounded, probe, nil)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = image.Close() }()
	if waitNumericPublisherExit(bounded, first.Server) != nil {
		return ErrBoundary
	}
	request := numericPublisherRequest{Phase: "publish", Source: source, Kind: kind, Instance: instance, Target: target, PreviousServer: first.Server}
	if validateNumericPublisherRequest(request) != nil {
		return ErrBoundary
	}
	second, fresh, err := numericPublisherExchange(bounded, request, image)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = fresh.Close() }()
	if second.Server.Equal(first.Server) || !second.Published || !sameNumericPublisherSource(image, fresh) || !(bootid.Reader{}).ForPID(pid).Equal(source) || readSourceAgentReference(source) != nil {
		return ErrBoundary
	}
	return nil
}

func numericPublisherExchange(ctx context.Context, request numericPublisherRequest, previous *os.File) (numericPublisherResponse, *os.File, error) {
	var empty numericPublisherResponse
	if validateNumericPublisherRequest(request) != nil || brokerProtectedParent("/run/ngfw/ra") != nil {
		return empty, nil, ErrBoundary
	}
	var stat unix.Stat_t
	if unix.Lstat(numericPublisherSocketPath, &stat) != nil || stat.Mode != unix.S_IFSOCK|0600 || stat.Uid != 0 || stat.Gid != 0 {
		return empty, nil, ErrBoundary
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return empty, nil, ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	if boundUnitObserverSocket(ctx, fd) != nil || unix.Connect(fd, &unix.SockaddrUnix{Name: numericPublisherSocketPath}) != nil {
		return empty, nil, ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid != 1 {
		return empty, nil, ErrBoundary
	}
	content, err := json.Marshal(request)
	if err != nil || len(content) > numericPublisherPacketLimit {
		return empty, nil, ErrBoundary
	}
	if unix.Sendmsg(fd, content, nil, nil, 0) != nil {
		return empty, nil, ErrBoundary
	}
	if previous != nil && unix.Sendmsg(fd, []byte("SOURCE"), unix.UnixRights(int(previous.Fd())), nil, 0) != nil {
		return empty, nil, ErrBoundary
	}
	data, files, err := receiveUnitObserverPacket(fd, 1)
	if err != nil {
		return empty, nil, ErrBoundary
	}
	success := false
	defer func() {
		if !success {
			closeUnitObserverFiles(files)
		}
	}()
	var response numericPublisherResponse
	if len(data) > numericPublisherPacketLimit || decodeUnitObserverPacket(data, &response) != nil || response.Phase != request.Phase || !response.Source.Equal(request.Source) || !response.Server.Complete() || numericPublisherManager(ctx, response.Server) != nil || validateSourceAgentExecutable(files[0]) != nil {
		return empty, nil, ErrBoundary
	}
	if request.Phase == "probe" && response.Published {
		return empty, nil, ErrBoundary
	}
	if request.Phase == "publish" && (!response.Published || previous == nil || !sameNumericPublisherSource(previous, files[0])) {
		return empty, nil, ErrBoundary
	}
	pid, gid := request.Source.PID, os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || gid < 0 || gid > math.MaxUint32 {
		return empty, nil, ErrBoundary
	}
	if verifyFixedAgentPeer(ctx, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, request.Source) != nil || readSourceAgentReference(request.Source) != nil || unix.Sendmsg(fd, []byte("OK"), nil, nil, 0) != nil {
		return empty, nil, ErrBoundary
	}
	success = true
	return response, files[0], nil
}
