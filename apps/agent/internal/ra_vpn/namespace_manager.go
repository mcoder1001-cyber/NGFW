package ravpn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// NamespaceBrokerFDDispatch transfers only already-opened, typed namespace
// descriptors. The manager must authenticate the root peer before accepting
// their use; profile data never supplies descriptor numbers or paths.
type NamespaceBrokerFDDispatch interface {
	Preflight(context.Context) error
	RunFDs(context.Context, NamespaceBrokerMessage, [3]int) error
}

// NamespaceBrokerMessage contains ownership metadata, never credentials or
// caller-selected filesystem paths. Descriptor order is mount, host, private.
type NamespaceBrokerMessage struct {
	Operation     string
	Instance      string
	Source        MountTarget
	Targets       []MountTarget
	Target        MountTarget
	HostNamespace uint64
	Namespace     uint64
}

// SystemdNamespaceBroker invokes only the fixed root-owned manager unit.
type SystemdNamespaceBroker struct{ Executable string }

const namespaceBrokerUnit = "/usr/lib/systemd/system/ngfw-ra-namespace-broker@.service"
const expectedNamespaceBrokerUnit = "62bfab7a525207c8bf89598f687e6a8440fab7c864ae7b76ac7c402788dc7d91"

// Preflight verifies the protected broker executable and fixed manager template.
func (b *SystemdNamespaceBroker) Preflight(ctx context.Context) error {
	if validateNamespaceBrokerExecutable(b.Executable) != nil {
		return ErrBoundary
	}
	unit, e := trustedInstallationFile(namespaceBrokerUnit, 16384, false)
	digest := sha256.Sum256(unit)
	if e != nil || hex.EncodeToString(digest[:]) != expectedNamespaceBrokerUnit {
		return ErrBoundary
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, "/usr/bin/systemctl", "show", "--property=FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart", "ngfw-ra-namespace-broker@preflight.service")
	output, e := command.Output()
	if e != nil || len(output) > 16384 {
		return ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	caps := strings.Fields(fields["CapabilityBoundingSet"])
	if len(caps) != 2 || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_admin ") || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_chroot ") || fields["NoNewPrivileges"] != "yes" || fields["User"] != "root" || fields["Group"] != "ngfw" || fields["FragmentPath"] != namespaceBrokerUnit || fields["DropInPaths"] != "" || !strings.Contains(fields["ExecStart"], "path="+b.Executable+" ;") {
		return ErrBoundary
	}
	socketUnit, err := trustedInstallationFile("/usr/lib/systemd/system/ngfw-ra-namespace-broker.socket", 16384, false)
	if err != nil {
		return ErrBoundary
	}
	socketDigest := sha256.Sum256(socketUnit)
	if hex.EncodeToString(socketDigest[:]) != "f8b139a683fbcf447304672c085d031cc4fd7d8d0fc36ed137db257b80787308" {
		return ErrBoundary
	}
	command = exec.CommandContext(bounded, "/usr/bin/systemctl", "show", "--property=FragmentPath,DropInPaths,ActiveState,SubState,Listen", "ngfw-ra-namespace-broker.socket")
	output, err = command.Output()
	if err != nil || len(output) > 16384 {
		return ErrBoundary
	}
	fields = map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	if fields["FragmentPath"] != "/usr/lib/systemd/system/ngfw-ra-namespace-broker.socket" || fields["DropInPaths"] != "" || fields["ActiveState"] != "active" || fields["SubState"] != "listening" || fields["Listen"] != namespaceBrokerSocket+" (SequentialPacket)" {
		return ErrBoundary
	}
	var socketStat unix.Stat_t
	if brokerProtectedParent(NamespaceIPCRoot) != nil || unix.Lstat(namespaceBrokerSocket, &socketStat) != nil || socketStat.Uid != 0 || socketStat.Gid != 0 || socketStat.Mode != unix.S_IFSOCK|0600 {
		return ErrBoundary
	}
	return nil
}

const namespaceBrokerSocket = NamespaceBrokerSocketPath

// RunFDs hands already-held namespace objects to the fixed activated broker.
// RunFDs preserves the archived interface but refuses unauthenticated legacy
// three-role production requests. All current operations require four roles.
func (*SystemdNamespaceBroker) RunFDs(context.Context, NamespaceBrokerMessage, [3]int) error {
	return ErrBoundary
}

// RunAttestedFDs transfers the authenticated caller's internally held source MNT.
func (*SystemdNamespaceBroker) RunAttestedFDs(ctx context.Context, request NamespaceBrokerMessage, fds [4]int) error {
	if ctx.Err() != nil || !ValidInstance(request.Instance) || brokerProtectedParent(NamespaceIPCRoot) != nil {
		return ErrBoundary
	}
	info, err := os.Lstat(namespaceBrokerSocket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 {
		return ErrBoundary
	}
	var stat unix.Stat_t
	if unix.Lstat(namespaceBrokerSocket, &stat) != nil || stat.Uid != 0 || stat.Gid != 0 {
		return ErrBoundary
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > 16384 {
		return ErrBoundary
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	timeout := unix.NsecToTimeval((5 * time.Second).Nanoseconds())
	if unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil || unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout) != nil {
		return ErrBoundary
	}
	if unix.Connect(fd, &unix.SockaddrUnix{Name: namespaceBrokerSocket}) != nil {
		return ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid != 1 {
		return ErrBoundary
	}
	if unix.Sendmsg(fd, data, unix.UnixRights(fds[:]...), nil, 0) != nil {
		return ErrBoundary
	}
	var response [8]byte
	n, _, flags, _, err := unix.Recvmsg(fd, response[:], nil, 0)
	if err != nil || flags != 0 || string(response[:n]) != "OK" || ctx.Err() != nil {
		return ErrBoundary
	}
	return nil
}

// RunManagedNamespaceBroker authenticates the protected root request and pins
// all four typed descriptors before any mount namespace change. The unit
// supplies only a fixed full instance identifier as its argument.
func RunManagedNamespaceBroker(socketFD int) error {
	if os.Geteuid() != 0 || socketFD < 0 {
		return ErrBoundary
	}
	roleContext, cancelRole := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRole()
	if normalizeCanonicalBroker(roleContext) != nil {
		return ErrBoundary
	}
	count, countErr := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if countErr != nil || count != 1 || os.Getenv("LISTEN_FDNAMES") != sourceAgentExecutableRole || os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return ErrBoundary
	}
	sourceImage := os.NewFile(3, "manager-opened canonical source executable")
	unix.CloseOnExec(3)
	defer func() { _ = sourceImage.Close() }()
	peer, e := unix.GetsockoptUcred(socketFD, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if e != nil || peer.Uid != 0 || peer.Pid <= 1 {
		return ErrBoundary
	}
	sourceBoot := (bootid.Reader{}).ForPID(int(peer.Pid))
	if verifyFixedAgentPeer(roleContext, peer, sourceBoot) != nil || readSourceAgentReference(sourceBoot) != nil || validateSourceAgentExecutable(sourceImage) != nil {
		return ErrBoundary
	}
	timeout := unix.NsecToTimeval((5 * time.Second).Nanoseconds())
	if unix.SetsockoptTimeval(socketFD, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil {
		return ErrBoundary
	}
	data, files, receiveErr := receiveUnitObserverPacket(socketFD, 4)
	if receiveErr != nil {
		return ErrBoundary
	}
	defer closeUnitObserverFiles(files)
	held := []int{int(files[0].Fd()), int(files[1].Fd()), int(files[2].Fd()), int(files[3].Fd())}
	var request NamespaceBrokerMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !ValidInstance(request.Instance) || request.Source.Boot.PID != int(peer.Pid) || len(request.Targets) != 2 || request.Namespace == 0 || request.HostNamespace == 0 || request.Namespace == request.HostNamespace {
		return ErrBoundary
	}
	instance := request.Instance
	if request.Operation != "export" && request.Operation != "verify" && request.Operation != "remove" {
		return ErrBoundary
	}
	if !request.Source.Boot.Complete() || request.Source.Boot.PID <= 1 || !(bootid.Reader{}).ForPID(request.Source.Boot.PID).Equal(request.Source.Boot) {
		return ErrBoundary
	}
	if !request.Source.Boot.Equal(sourceBoot) || verifyFixedAgentPeer(roleContext, peer, sourceBoot) != nil || readSourceAgentReference(sourceBoot) != nil || validateSourceAgentExecutable(sourceImage) != nil {
		return ErrBoundary
	}
	if validateAttestedBrokerFDs(request, [4]int{held[0], held[1], held[2], held[3]}) != nil {
		return ErrBoundary
	}
	// The manager role is PID1, and VPP must be the installed executable. Targets
	// are additionally bound to the authenticated agent's pending export receipt.
	if request.Targets[1].Boot.PID != 1 {
		return ErrBoundary
	}
	if verifyBrokerVPPUnitIdentity(roleContext, request.Targets[0]) != nil {
		return ErrBoundary
	}
	matched := false
	for _, target := range request.Targets {
		if !target.Boot.Complete() || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
			return ErrBoundary
		}
		if target.Boot.Equal(request.Target.Boot) && target.MountInode == request.Target.MountInode {
			matched = true
		}
	}
	if !matched {
		return ErrBoundary
	}
	if !(bootid.Reader{}).ForPID(request.Source.Boot.PID).Equal(request.Source.Boot) {
		return ErrBoundary
	}
	arguments := []string{request.Operation, instance, request.Target.Boot.String(), strconv.FormatUint(request.Target.MountInode, 10), strconv.FormatUint(request.HostNamespace, 10), strconv.FormatUint(request.Namespace, 10)}
	if runAttestedNamespaceBrokerFDs(arguments, held, request.Source.MountInode) != nil {
		return ErrBoundary
	}
	if verifyFixedAgentPeer(roleContext, peer, sourceBoot) != nil || readSourceAgentReference(sourceBoot) != nil || validateSourceAgentExecutable(sourceImage) != nil {
		return ErrBoundary
	}
	return unix.Sendmsg(socketFD, []byte("OK"), nil, nil, 0)
}

func verifyBrokerVPPUnitIdentity(ctx context.Context, target MountTarget) error {
	fragment, err := trustedInstallationFile("/usr/lib/systemd/system/vpp.service", 16384, false)
	if err != nil {
		return ErrBoundary
	}
	digest := sha256.Sum256(fragment)
	if hex.EncodeToString(digest[:]) != "6b004cdaa5b541c5d836eb45b65204d3b38a3081ce11717c9d3be6b4268c1716" {
		return ErrBoundary
	}
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID,FragmentPath,DropInPaths,ExecStart", "vpp.service")
	output, err := command.Output()
	if err != nil || len(output) > 16384 {
		return ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	pid, err := strconv.Atoi(fields["MainPID"])
	if err != nil || pid != target.Boot.PID || fields["FragmentPath"] != "/usr/lib/systemd/system/vpp.service" || !strings.Contains(fields["ExecStart"], "path=/usr/bin/vpp ; argv[]=/usr/bin/vpp -c /etc/vpp/startup.conf ;") {
		return ErrBoundary
	}
	if fields["DropInPaths"] != "/usr/lib/systemd/system/vpp.service.d/vpp-firstboot.conf" {
		return ErrBoundary
	}
	for _, path := range strings.Fields(fields["DropInPaths"]) {
		if path != "/usr/lib/systemd/system/vpp.service.d/vpp-firstboot.conf" {
			return ErrBoundary
		}
		content, err := trustedInstallationFile(path, 16384, false)
		if err != nil {
			return ErrBoundary
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != "a0ccadfaaa4c6d8217ddd7294cf33bb3b1d837e09a083b8d51d9969e0112ad82" {
			return ErrBoundary
		}
	}
	if !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
		return ErrBoundary
	}
	return nil
}
