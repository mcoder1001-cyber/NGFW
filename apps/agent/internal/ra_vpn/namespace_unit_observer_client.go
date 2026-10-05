package ravpn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const unitObserverPacketLimit = 4096

type unitObserverRequest struct {
	Instance string
	Source   bootid.Identity
	Expected bootid.Identity
}

type unitObserverResponse struct {
	Instance     string
	Server       bootid.Identity
	Identity     UnitIdentity
	ControlGroup string
}

func decodeUnitObserverPacket(data []byte, destination any) error {
	if len(data) == 0 || len(data) > unitObserverPacketLimit {
		return ErrEngine
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrEngine
	}
	return nil
}

func boundUnitObserverSocket(ctx context.Context, fd int) error {
	if ctx.Err() != nil {
		return ErrEngine
	}
	duration := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		duration = min(duration, time.Until(deadline))
	}
	if duration <= 0 {
		return ErrEngine
	}
	timeout := unix.NsecToTimeval(max(duration.Nanoseconds(), int64(1000)))
	if unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil ||
		unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout) != nil {
		return ErrEngine
	}
	return nil
}

// receiveUnitObserverPacket owns every received right until the entire packet
// has been accepted. It closes all collected rights even when an unexpected
// ancillary message precedes them; no early return inside ancillary traversal.
func receiveUnitObserverPacket(fd, wantedRights int) ([]byte, []*os.File, error) {
	if wantedRights < 0 || wantedRights > 4 {
		return nil, nil, ErrEngine
	}
	var data [unitObserverPacketLimit + 1]byte
	control := make([]byte, unix.CmsgSpace(16*4))
	n, cn, flags, _, receiveErr := unix.Recvmsg(fd, data[:], control, unix.MSG_CMSG_CLOEXEC)
	messages, parseErr := unix.ParseSocketControlMessage(control[:cn])
	var descriptors []int
	ancillaryInvalid := false
	for _, message := range messages {
		if message.Header.Level != unix.SOL_SOCKET || message.Header.Type != unix.SCM_RIGHTS {
			ancillaryInvalid = true
			continue
		}
		rights, err := unix.ParseUnixRights(&message)
		if err != nil {
			ancillaryInvalid = true
			continue
		}
		descriptors = append(descriptors, rights...)
	}
	transferred := false
	defer func() {
		if !transferred {
			for _, descriptor := range descriptors {
				_ = unix.Close(descriptor)
			}
		}
	}()
	messageCount := 0
	if wantedRights != 0 {
		messageCount = 1
	}
	if receiveErr != nil || parseErr != nil || ancillaryInvalid ||
		flags & ^unix.MSG_CMSG_CLOEXEC != 0 || n == 0 || n > unitObserverPacketLimit ||
		len(messages) != messageCount || len(descriptors) != wantedRights {
		return nil, nil, ErrEngine
	}
	files := make([]*os.File, 0, len(descriptors))
	for _, descriptor := range descriptors {
		files = append(files, os.NewFile(uintptr(descriptor), "manager-held unit observation"))
	}
	transferred = true
	return data[:n], files, nil
}

func closeUnitObserverFiles(files []*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}

func unitObserverSnapshot(instance string, response unitObserverResponse, files []*os.File) (*UnitProcessSnapshot, error) {
	if !validUnitName("ngfw-ra@"+instance+".service") || response.Instance != instance ||
		!response.Identity.Valid() || !response.Server.Complete() || response.Identity.BootID != response.Server.BootID || !unitObserverControlGroup(instance, response.ControlGroup) || len(files) != 2 ||
		files[0] == nil || files[1] == nil {
		return nil, ErrEngine
	}
	if brokerNamespaceFD(int(files[0].Fd()), unix.CLONE_NEWNET, response.Identity.NamespaceInode) != nil {
		return nil, ErrEngine
	}
	var executable unix.Stat_t
	if unix.Fstat(int(files[1].Fd()), &executable) != nil ||
		executable.Mode&unix.S_IFMT != unix.S_IFREG || executable.Uid != 0 ||
		executable.Mode&0022 != 0 || executable.Mode&0111 == 0 || executable.Nlink != 1 {
		return nil, ErrEngine
	}
	return &UnitProcessSnapshot{Instance: instance, Identity: response.Identity,
		ControlGroup: response.ControlGroup, Network: files[0], Executable: files[1]}, nil
}

func unitObserverControlGroup(instance, group string) bool {
	return len(group) <= 512 && filepath.Clean(group) == group &&
		strings.HasPrefix(group, "/system.slice/") && strings.HasSuffix(group, "/ngfw-ra@"+instance+".service") &&
		!strings.ContainsAny(group, "\r\n\x00")
}

// Acquire observes an existing canonical unit. It never starts a socket or service.
func (p *SystemdUnitObservation) acquireOnce(ctx context.Context, instance string) (*unitObserverCapture, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p.Preflight(ctx) != nil {
		return nil, ErrEngine
	}
	expected, group, err := unitObserverTarget(ctx, instance)
	if err != nil || unitObserverManager(ctx, instance, expected, bootid.Identity{}) != nil {
		return nil, ErrEngine
	}
	path := "/run/ngfw/ra/observers/" + strconv.Itoa(expected.PID) + ".sock"
	var info unix.Stat_t
	if brokerProtectedParent("/run/ngfw/ra/observers") != nil || unix.Lstat(path, &info) != nil || info.Mode != unix.S_IFSOCK|0600 || info.Uid != 0 || info.Gid != 0 {
		return nil, ErrEngine
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, ErrEngine
	}
	defer func() { _ = unix.Close(fd) }()
	if boundUnitObserverSocket(ctx, fd) != nil || unix.Connect(fd, &unix.SockaddrUnix{Name: path}) != nil {
		return nil, ErrEngine
	}
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid != 1 {
		return nil, ErrEngine
	}
	source := (bootid.Reader{}).ForPID(os.Getpid())
	request, err := json.Marshal(unitObserverRequest{Instance: instance, Source: source, Expected: expected})
	if err != nil || len(request) > unitObserverPacketLimit || unix.Sendmsg(fd, request, nil, nil, 0) != nil {
		return nil, ErrEngine
	}
	data, files, err := receiveUnitObserverPacket(fd, 3)
	if err != nil {
		return nil, ErrEngine
	}
	success := false
	defer func() {
		if !success {
			closeUnitObserverFiles(files)
		}
	}()
	var response unitObserverResponse
	if decodeUnitObserverPacket(data, &response) != nil || response.Identity.PID != expected.PID || response.Identity.BootID != expected.BootID || response.Identity.StartTicks != expected.StartTime || response.ControlGroup != group || unitObserverManager(ctx, instance, expected, response.Server) != nil {
		return nil, ErrEngine
	}
	snapshot, err := unitObserverSnapshot(instance, response, files[:2])
	if err != nil || validateSourceAgentExecutable(files[2]) != nil {
		return nil, ErrEngine
	}
	fresh, freshGroup, err := unitObserverTarget(ctx, instance)
	if err != nil || !fresh.Equal(expected) || freshGroup != group || ctx.Err() != nil || readSourceAgentReference(source) != nil || !(bootid.Reader{}).ForPID(os.Getpid()).Equal(source) || unix.Sendmsg(fd, []byte("OK"), nil, nil, 0) != nil {
		return nil, ErrEngine
	}
	success = true
	return &unitObserverCapture{Snapshot: snapshot, SourceExecutable: files[2], Server: response.Server, Source: source}, nil
}

type unitObserverCapture struct {
	Snapshot         *UnitProcessSnapshot
	SourceExecutable *os.File
	Server           bootid.Identity
	Source           bootid.Identity
}

func (capture *unitObserverCapture) close() {
	if capture != nil {
		if capture.Snapshot != nil {
			_ = capture.Snapshot.Close()
		}
		if capture.SourceExecutable != nil {
			_ = capture.SourceExecutable.Close()
		}
	}
}

// Acquire compares two independent manager captures before returning the second
// snapshot. A complete first-server exit prevents reusing its held descriptors.
// This establishes before/after linkage, not continual process immutability.
func (p *SystemdUnitObservation) Acquire(ctx context.Context, instance string) (*UnitProcessSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	first, err := p.acquireOnce(ctx, instance)
	if err != nil {
		return nil, ErrEngine
	}
	defer first.close()
	if waitUnitObserverExit(ctx, first.Server, first.Snapshot.Identity.PID) != nil {
		return nil, ErrEngine
	}
	second, err := p.acquireOnce(ctx, instance)
	if err != nil {
		return nil, ErrEngine
	}
	accepted := false
	defer func() {
		if !accepted {
			second.close()
		}
	}()
	if unitObserverCapturesMatch(first, second) != nil || ctx.Err() != nil {
		return nil, ErrEngine
	}
	if second.SourceExecutable.Close() != nil {
		return nil, ErrEngine
	}
	second.SourceExecutable = nil
	accepted = true
	return second.Snapshot, nil
}

func unitObserverCapturesMatch(first, second *unitObserverCapture) error {
	if first == nil || second == nil || first.Snapshot == nil || second.Snapshot == nil || !first.Server.Complete() || !second.Server.Complete() || first.Server.Equal(second.Server) || !first.Source.Equal(second.Source) ||
		first.Snapshot.Instance != second.Snapshot.Instance || first.Snapshot.Identity != second.Snapshot.Identity || first.Snapshot.ControlGroup != second.Snapshot.ControlGroup {
		return ErrEngine
	}
	for _, pair := range [][2]*os.File{{first.Snapshot.Network, second.Snapshot.Network}, {first.Snapshot.Executable, second.Snapshot.Executable}, {first.SourceExecutable, second.SourceExecutable}} {
		if pair[0] == nil || pair[1] == nil {
			return ErrEngine
		}
		var left, right unix.Stat_t
		if unix.Fstat(int(pair[0].Fd()), &left) != nil || unix.Fstat(int(pair[1].Fd()), &right) != nil || left.Dev != right.Dev || left.Ino != right.Ino || left.Mode != right.Mode || left.Uid != right.Uid || left.Nlink != right.Nlink {
			return ErrEngine
		}
	}
	return nil
}

func waitUnitObserverExit(ctx context.Context, server bootid.Identity, targetPID int) error {
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !(bootid.Reader{}).ForPID(server.PID).Equal(server) {
			fields, err := namespaceSystemdProperties(wait, "ngfw-ra-observer@"+strconv.Itoa(targetPID)+".service", "MainPID,ActiveState,SubState")
			if err == nil && fields["MainPID"] == "0" && fields["ActiveState"] == "inactive" && fields["SubState"] == "dead" {
				return nil
			}
		}
		select {
		case <-wait.Done():
			return ErrEngine
		case <-ticker.C:
		}
	}
}
