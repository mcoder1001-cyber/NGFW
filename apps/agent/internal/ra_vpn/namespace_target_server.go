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

type namespaceTargetRequest struct {
	Source      bootid.Identity
	ExpectedVPP bootid.Identity
	Role        string
}
type namespaceTargetResponse struct {
	Server  bootid.Identity
	Source  bootid.Identity
	Targets [2]MountTarget
}

// RunNamespaceTargetProvider serves exactly one read-only bounded observation.
// PID1 preopens five fixed roles before dropping all capabilities.
func RunNamespaceTargetProvider(instance string) error {
	pid, err := strconv.ParseInt(instance, 10, 32)
	if err != nil || pid <= 1 || strconv.FormatInt(pid, 10) != instance || os.Geteuid() != 0 || validateNamespaceBrokerProcess(os.Getpid(), 0) != nil {
		return ErrBoundary
	}
	count, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || count != 5 || os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return ErrBoundary
	}
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	if len(names) != 5 {
		return ErrBoundary
	}
	var inherited [5]*os.File
	for i := range inherited {
		fd := i + 3
		unix.CloseOnExec(fd)
		inherited[i] = os.NewFile(uintptr(fd), "manager-opened target role")
	}
	defer func() {
		for _, file := range inherited {
			_ = file.Close()
		}
	}()
	files := map[string]*os.File{}
	for i, name := range names {
		if name != "targets-listener" && name != "vpp-mount" && name != "manager-mount" && name != "vpp-exe" && name != sourceAgentExecutableRole {
			return ErrBoundary
		}
		if _, exists := files[name]; exists {
			return ErrBoundary
		}
		files[name] = inherited[i]
	}
	listener := int(files["targets-listener"].Fd())
	if kind, err := unix.GetsockoptInt(listener, unix.SOL_SOCKET, unix.SO_TYPE); err != nil || kind != unix.SOCK_SEQPACKET {
		return ErrBoundary
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if boundUnitObserverSocket(ctx, listener) != nil {
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
	if err != nil || peer.Uid != 0 || peer.Pid <= 1 {
		return ErrBoundary
	}
	data, _, err := receiveUnitObserverPacket(socket, 0)
	if err != nil {
		return ErrBoundary
	}
	var request namespaceTargetRequest
	if decodeUnitObserverPacket(data, &request) != nil || request.Role != "agent" || request.Source.PID != int(peer.Pid) || request.ExpectedVPP.PID != int(pid) || !validNumericOpenFileTarget(request.ExpectedVPP) || verifyFixedAgentPeer(ctx, peer, request.Source) != nil || readSourceAgentReference(request.Source) != nil {
		return ErrBoundary
	}
	expected := (bootid.Reader{}).ForPID(int(pid))
	manager := (bootid.Reader{}).ForPID(1)
	server := (bootid.Reader{}).ForPID(os.Getpid())
	if !expected.Equal(request.ExpectedVPP) || !manager.Complete() || !server.Complete() {
		return ErrBoundary
	}
	snapshot := &NamespaceTargetSnapshot{Source: request.Source, Files: [2]*os.File{files["vpp-mount"], files["manager-mount"]}, Executables: [2]*os.File{files["vpp-exe"], files[sourceAgentExecutableRole]}}
	for i, file := range snapshot.Files {
		var stat unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || brokerNamespaceFD(int(file.Fd()), unix.CLONE_NEWNS, stat.Ino) != nil {
			return ErrBoundary
		}
		snapshot.Targets[i] = MountTarget{Boot: []bootid.Identity{expected, manager}[i], MountInode: stat.Ino}
	}
	if validateTargetHeldImages(snapshot) != nil {
		return ErrBoundary
	}
	probe := &SystemdNamespaceTargets{}
	if probe.validateUnits(ctx, expected, server) != nil {
		return ErrBoundary
	}
	response := namespaceTargetResponse{Server: server, Source: request.Source, Targets: snapshot.Targets}
	output, err := json.Marshal(response)
	if err != nil || len(output) > unitObserverPacketLimit {
		return ErrBoundary
	}
	rights := []int{int(snapshot.Files[0].Fd()), int(snapshot.Files[1].Fd()), int(snapshot.Executables[0].Fd()), int(snapshot.Executables[1].Fd())}
	if unix.Sendmsg(socket, output, unix.UnixRights(rights...), nil, 0) != nil {
		return ErrBoundary
	}
	ack, _, err := receiveUnitObserverPacket(socket, 0)
	if err != nil || string(ack) != "OK" || ctx.Err() != nil || verifyFixedAgentPeer(ctx, peer, request.Source) != nil || readSourceAgentReference(request.Source) != nil || !(bootid.Reader{}).ForPID(int(pid)).Equal(expected) || !(bootid.Reader{}).ForPID(1).Equal(manager) || !(bootid.Reader{}).ForPID(os.Getpid()).Equal(server) || validateTargetHeldImages(snapshot) != nil || probe.validateUnits(ctx, expected, server) != nil {
		return ErrBoundary
	}
	return nil
}

// Only the canonical agent role is supported until a broker's actual executable
// can be independently manager-attested. Unit metadata alone is insufficient.
func validateNamespaceBrokerProcess(pid int, expected uint64) error {
	status, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil || len(status) > 16384 {
		return ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	uids := strings.Fields(fields["Uid"])
	if len(uids) != 4 || strings.Join(uids, ",") != "0,0,0,0" || fields["NoNewPrivs"] != "1" {
		return ErrBoundary
	}
	for _, field := range []string{"CapEff", "CapPrm", "CapBnd", "CapInh", "CapAmb"} {
		value, err := strconv.ParseUint(fields[field], 16, 64)
		required := expected
		if field == "CapInh" || field == "CapAmb" {
			required = 0
		}
		if err != nil || value != required {
			return ErrBoundary
		}
	}
	return nil
}
