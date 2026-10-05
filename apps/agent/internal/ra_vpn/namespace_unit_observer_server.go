package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// RunUnitObservationProvider serves one bounded request using descriptors opened
// by the manager before the observer drops every capability. It cannot activate
// a daemon, change namespaces, or accept caller-supplied paths or target PIDs.
func RunUnitObservationProvider(numericPID string) error {
	pid, err := strconv.Atoi(numericPID)
	if err != nil || pid <= 1 || strconv.Itoa(pid) != numericPID || os.Geteuid() != 0 {
		return ErrEngine
	}
	count, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	if err != nil || count != 4 || len(names) != 4 || os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return ErrEngine
	}
	roles := map[string]int{}
	for index, name := range names {
		if name != "unit-listener" && name != "unit-net" && name != "unit-exe" && name != sourceAgentExecutableRole {
			return ErrEngine
		}
		if _, exists := roles[name]; exists {
			return ErrEngine
		}
		fd := index + 3
		unix.CloseOnExec(fd)
		roles[name] = fd
	}
	files := map[string]*os.File{}
	for name, fd := range roles {
		files[name] = os.NewFile(uintptr(fd), "manager-opened observation role")
	}
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	if validateNamespaceBrokerProcess(os.Getpid(), 0) != nil || unitObserverInstallation() != nil {
		return ErrEngine
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener := roles["unit-listener"]
	kind, err := unix.GetsockoptInt(listener, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil || kind != unix.SOCK_SEQPACKET || boundUnitObserverSocket(ctx, listener) != nil {
		return ErrEngine
	}
	socket, _, err := unix.Accept4(listener, unix.SOCK_CLOEXEC)
	if err != nil {
		return ErrEngine
	}
	defer func() { _ = unix.Close(socket) }()
	if boundUnitObserverSocket(ctx, socket) != nil {
		return ErrEngine
	}
	peer, err := unix.GetsockoptUcred(socket, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return ErrEngine
	}
	data, unexpected, err := receiveUnitObserverPacket(socket, 0)
	closeUnitObserverFiles(unexpected)
	var request unitObserverRequest
	if err != nil || decodeUnitObserverPacket(data, &request) != nil || request.Expected.PID != pid || !request.Expected.Complete() || verifyFixedAgentPeer(ctx, peer, request.Source) != nil || readSourceAgentReference(request.Source) != nil || validateSourceAgentExecutable(files[sourceAgentExecutableRole]) != nil {
		return ErrEngine
	}
	expected, group, err := unitObserverTarget(ctx, request.Instance)
	if err != nil || !expected.Equal(request.Expected) {
		return ErrEngine
	}
	network := files["unit-net"]
	executable := files["unit-exe"]
	var ns unix.Stat_t
	if unix.Fstat(int(network.Fd()), &ns) != nil || brokerNamespaceFD(int(network.Fd()), unix.CLONE_NEWNET, ns.Ino) != nil {
		return ErrEngine
	}
	server := (bootid.Reader{}).ForPID(os.Getpid())
	response := unitObserverResponse{Instance: request.Instance, Server: server, ControlGroup: group, Identity: UnitIdentity{BootID: expected.BootID, PID: pid, StartTicks: expected.StartTime, NamespaceInode: ns.Ino}}
	if _, err = unitObserverSnapshot(request.Instance, response, []*os.File{network, executable}); err != nil {
		return ErrEngine
	}
	if !sameUnitExecutable(executable, "/opt/ngfw-ra/sbin/charon-systemd") && !sameUnitExecutable(executable, "/usr/lib/ngfw/ngfw-ra-daemon") {
		return ErrEngine
	}
	if unitObserverManager(ctx, request.Instance, expected, server) != nil {
		return ErrEngine
	}
	fresh, freshGroup, err := unitObserverTarget(ctx, request.Instance)
	if err != nil || !fresh.Equal(expected) || freshGroup != group || verifyFixedAgentPeer(ctx, peer, request.Source) != nil || readSourceAgentReference(request.Source) != nil || validateSourceAgentExecutable(files[sourceAgentExecutableRole]) != nil {
		return ErrEngine
	}
	output, err := json.Marshal(response)
	if err != nil || len(output) > unitObserverPacketLimit || unix.Sendmsg(socket, output, unix.UnixRights(int(network.Fd()), int(executable.Fd()), int(files[sourceAgentExecutableRole].Fd())), nil, 0) != nil {
		return ErrEngine
	}
	if boundUnitObserverSocket(ctx, socket) != nil {
		return ErrEngine
	}
	ack, unexpected, err := receiveUnitObserverPacket(socket, 0)
	closeUnitObserverFiles(unexpected)
	if err != nil || string(ack) != "OK" || ctx.Err() != nil || !(bootid.Reader{}).ForPID(pid).Equal(expected) || !(bootid.Reader{}).ForPID(request.Source.PID).Equal(request.Source) {
		return ErrEngine
	}
	return nil
}
