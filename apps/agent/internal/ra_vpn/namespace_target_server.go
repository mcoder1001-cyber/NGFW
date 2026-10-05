package ravpn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	Targets [2]MountTarget
}

// RunNamespaceTargetProvider serves one bounded read-only observation. The
// manager opens the listener and both typed role descriptors before cap drop.
func RunNamespaceTargetProvider(instance string) error {
	pid, err := strconv.Atoi(instance)
	if err != nil || pid <= 1 || strconv.Itoa(pid) != instance || os.Geteuid() != 0 {
		return ErrBoundary
	}
	if validateNamespaceBrokerProcess(os.Getpid(), uint64(1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_SYS_CHROOT)) != nil {
		return ErrBoundary
	}
	count, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || count != 3 || os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return ErrBoundary
	}
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	if len(names) != 3 {
		return ErrBoundary
	}
	descriptors := map[string]int{}
	for index, name := range names {
		if name != "targets-listener" && name != "vpp-mount" && name != "manager-mount" {
			return ErrBoundary
		}
		if _, exists := descriptors[name]; exists {
			return ErrBoundary
		}
		descriptors[name] = index + 3
	}
	listener := descriptors["targets-listener"]
	if kind, err := unix.GetsockoptInt(listener, unix.SOL_SOCKET, unix.SO_TYPE); err != nil || kind != unix.SOCK_SEQPACKET {
		return ErrBoundary
	}
	timeout := unix.NsecToTimeval((5 * time.Second).Nanoseconds())
	if unix.SetsockoptTimeval(listener, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil {
		return ErrBoundary
	}
	socket, _, err := unix.Accept4(listener, unix.SOCK_CLOEXEC)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(socket) }()
	if unix.SetsockoptTimeval(socket, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil || unix.SetsockoptTimeval(socket, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout) != nil {
		return ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(socket, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid <= 1 {
		return ErrBoundary
	}
	// Rights are not accepted in a read-only target observation request.
	var data [4097]byte
	control := make([]byte, unix.CmsgSpace(16*4))
	n, cn, flags, _, err := unix.Recvmsg(socket, data[:], control, unix.MSG_CMSG_CLOEXEC)
	messages, parseErr := unix.ParseSocketControlMessage(control[:cn])
	for _, message := range messages {
		if message.Header.Level == unix.SOL_SOCKET && message.Header.Type == unix.SCM_RIGHTS {
			rights, e := unix.ParseUnixRights(&message)
			if e == nil {
				for _, fd := range rights {
					_ = unix.Close(fd)
				}
			}
		}
	}
	if err != nil || parseErr != nil || cn != 0 || flags & ^unix.MSG_CMSG_CLOEXEC != 0 || n > 4096 {
		return ErrBoundary
	}
	var request namespaceTargetRequest
	decoder := json.NewDecoder(bytes.NewReader(data[:n]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Source.PID != int(peer.Pid) || !request.Source.Complete() || !(bootid.Reader{}).ForPID(request.Source.PID).Equal(request.Source) || request.ExpectedVPP.PID != pid || !request.ExpectedVPP.Complete() {
		return ErrBoundary
	}
	if validateNamespaceTargetRequester(request) != nil {
		return ErrBoundary
	}
	var response namespaceTargetResponse
	response.Server = (bootid.Reader{}).ForPID(os.Getpid())
	identities := [2]bootid.Identity{(bootid.Reader{}).ForPID(pid), (bootid.Reader{}).ForPID(1)}
	if !identities[0].Equal(request.ExpectedVPP) || !identities[1].Complete() || !response.Server.Complete() {
		return ErrBoundary
	}
	fds := [2]int{descriptors["vpp-mount"], descriptors["manager-mount"]}
	for index, fd := range fds {
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || brokerNamespaceFD(fd, unix.CLONE_NEWNS, stat.Ino) != nil {
			return ErrBoundary
		}
		response.Targets[index] = MountTarget{Boot: identities[index], MountInode: stat.Ino}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if verifyBrokerVPPUnitIdentity(ctx, response.Targets[0]) != nil {
		return ErrBoundary
	}
	output, err := json.Marshal(response)
	if err != nil || len(output) > 4096 {
		return ErrBoundary
	}
	if unix.Sendmsg(socket, output, unix.UnixRights(fds[:]...), nil, 0) != nil {
		return ErrBoundary
	}
	var ack [8]byte
	n, _, flags, _, err = unix.Recvmsg(socket, ack[:], nil, 0)
	if err != nil || flags != 0 || string(ack[:n]) != "OK" || !(bootid.Reader{}).ForPID(pid).Equal(identities[0]) || !(bootid.Reader{}).ForPID(1).Equal(identities[1]) {
		return ErrBoundary
	}
	return nil
}

// Broker observations are authenticated as the fixed running broker unit,
// rather than permitting every root process with the broker capability mask.
func validateNamespaceTargetRequester(request namespaceTargetRequest) error {
	switch request.Role {
	case "agent":
		return validateNamespaceBrokerProcess(request.Source.PID, uint64(1<<unix.CAP_NET_ADMIN|1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_IPC_LOCK))
	case "broker":
		if validateNamespaceBrokerProcess(request.Source.PID, uint64(1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_SYS_CHROOT)) != nil {
			return ErrBoundary
		}
	default:
		return ErrBoundary
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(request.Source.PID) + "/cgroup")
	if err != nil || len(data) > 16384 {
		return ErrBoundary
	}
	unit := ""
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "0::/system.slice/ngfw-ra-namespace-broker@") {
			continue
		}
		candidate := strings.TrimPrefix(line, "0::/system.slice/")
		if !strings.HasSuffix(candidate, ".service") || len(candidate) > 256 || strings.ContainsAny(candidate, "/\x00\r\n ") {
			return ErrBoundary
		}
		for _, character := range candidate {
			if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-_.@\\", character)) {
				return ErrBoundary
			}
		}
		if unit != "" {
			return ErrBoundary
		}
		unit = candidate
	}
	if unit == "" {
		return ErrBoundary
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fields, err := namespaceSystemdProperties(ctx, unit, "MainPID,ControlGroup,FragmentPath,DropInPaths,ExecStart")
	if err != nil || fields["MainPID"] != strconv.Itoa(request.Source.PID) || fields["ControlGroup"] != "/system.slice/"+unit || fields["FragmentPath"] != namespaceBrokerUnit || fields["DropInPaths"] != "" || !strings.Contains(fields["ExecStart"], "path=/usr/lib/ngfw/ngfw-ra-namespace-broker ; argv[]=/usr/lib/ngfw/ngfw-ra-namespace-broker ;") || !(bootid.Reader{}).ForPID(request.Source.PID).Equal(request.Source) {
		return ErrBoundary
	}
	return (&SystemdNamespaceBroker{Executable: "/usr/lib/ngfw/ngfw-ra-namespace-broker"}).Preflight(ctx)
}

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
