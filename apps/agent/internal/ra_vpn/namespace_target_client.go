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

// SystemdNamespaceTargets probes only the already-provisioned numeric VPP PID
// socket. It never starts a unit, creates a path or opens another process's NS.
type SystemdNamespaceTargets struct {
	ExpectedVPP func(context.Context) (bootid.Identity, error)
	Executable  string
}

func (p *SystemdNamespaceTargets) expected(ctx context.Context) (bootid.Identity, error) {
	if p == nil || p.ExpectedVPP == nil {
		return bootid.Identity{}, ErrBoundary
	}
	identity, err := p.ExpectedVPP(ctx)
	if err != nil || !identity.Complete() || identity.PID <= 1 || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return bootid.Identity{}, ErrBoundary
	}
	return identity, nil
}
func (p *SystemdNamespaceTargets) executable() string {
	if p.Executable != "" {
		return p.Executable
	}
	return "/usr/lib/ngfw/ngfw-ra-namespace-broker"
}
func (p *SystemdNamespaceTargets) Preflight(ctx context.Context) error {
	snapshot, err := p.Acquire(ctx)
	if err != nil {
		return ErrBoundary
	}
	return snapshot.Close()
}
func (p *SystemdNamespaceTargets) Acquire(ctx context.Context) (*NamespaceTargetSnapshot, error) {
	expected, err := p.expected(ctx)
	if err != nil {
		return nil, ErrBoundary
	}
	if p.validateUnits(ctx, expected.PID) != nil {
		return nil, ErrBoundary
	}
	path := "/run/ngfw/ra/targets/" + strconv.Itoa(expected.PID) + ".sock"
	if brokerProtectedParent("/run/ngfw/ra/targets") != nil {
		return nil, ErrBoundary
	}
	var info unix.Stat_t
	if unix.Lstat(path, &info) != nil || info.Mode != unix.S_IFSOCK|0600 || info.Uid != 0 || info.Gid != 0 {
		return nil, ErrBoundary
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	timeout := unix.NsecToTimeval((5 * time.Second).Nanoseconds())
	if unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil || unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout) != nil || unix.Connect(fd, &unix.SockaddrUnix{Name: path}) != nil {
		return nil, ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Uid != 0 || peer.Gid != 0 || peer.Pid != 1 {
		return nil, ErrBoundary
	}
	request := namespaceTargetRequest{Source: (bootid.Reader{}).ForPID(os.Getpid()), ExpectedVPP: expected}
	if !request.Source.Complete() {
		return nil, ErrBoundary
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > 4096 {
		return nil, ErrBoundary
	}
	if unix.Sendmsg(fd, data, nil, nil, 0) != nil {
		return nil, ErrBoundary
	}
	var responseData [4097]byte
	control := make([]byte, unix.CmsgSpace(2*4))
	n, cn, flags, _, receiveErr := unix.Recvmsg(fd, responseData[:], control, unix.MSG_CMSG_CLOEXEC)
	messages, parseErr := unix.ParseSocketControlMessage(control[:cn])
	var received []int
	transferred := false
	defer func() {
		if !transferred {
			for _, descriptor := range received {
				_ = unix.Close(descriptor)
			}
		}
	}()
	for _, message := range messages {
		if message.Header.Level != unix.SOL_SOCKET || message.Header.Type != unix.SCM_RIGHTS {
			return nil, ErrBoundary
		}
		rights, err := unix.ParseUnixRights(&message)
		if err != nil {
			return nil, ErrBoundary
		}
		received = append(received, rights...)
	}
	if receiveErr != nil || parseErr != nil || flags & ^unix.MSG_CMSG_CLOEXEC != 0 || n > 4096 || len(messages) != 1 || len(received) != 2 {
		return nil, ErrBoundary
	}
	var response namespaceTargetResponse
	decoder := json.NewDecoder(bytes.NewReader(responseData[:n]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil || decoder.Decode(new(any)) != io.EOF || !response.Targets[0].Boot.Equal(expected) || response.Targets[1].Boot.PID != 1 || !response.Server.Complete() {
		return nil, ErrBoundary
	}
	if !(bootid.Reader{}).ForPID(response.Server.PID).Equal(response.Server) || validateNamespaceBrokerProcess(response.Server.PID, uint64(1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_SYS_CHROOT)) != nil {
		return nil, ErrBoundary
	}
	properties, err := namespaceSystemdProperties(ctx, "ngfw-ra-targets@"+strconv.Itoa(expected.PID)+".service", "MainPID")
	if err != nil || properties["MainPID"] != strconv.Itoa(response.Server.PID) {
		return nil, ErrBoundary
	}
	snapshot := &NamespaceTargetSnapshot{Targets: response.Targets}
	for i, descriptor := range received {
		snapshot.Files[i] = os.NewFile(uintptr(descriptor), "manager-attested mount namespace")
	}
	transferred = true // os.File now owns each descriptor; never close its raw FD.
	success := false
	defer func() {
		if !success {
			_ = snapshot.Close()
		}
	}()
	if snapshot.Validate() != nil {
		return nil, ErrBoundary
	}
	fresh, err := p.expected(ctx)
	if err != nil || !fresh.Equal(expected) || !(bootid.Reader{}).ForPID(1).Equal(snapshot.Targets[1].Boot) || ctx.Err() != nil {
		return nil, ErrBoundary
	}
	if unix.Sendmsg(fd, []byte("OK"), nil, nil, 0) != nil {
		return nil, ErrBoundary
	}
	success = true
	return snapshot, nil
}

func namespaceSystemdProperties(ctx context.Context, unit, properties string) (map[string]string, error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(bounded, "/usr/bin/systemctl", "show", "--property="+properties, unit).Output()
	if err != nil || len(output) > 16384 {
		return nil, ErrBoundary
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	return fields, nil
}

func (p *SystemdNamespaceTargets) validateUnits(ctx context.Context, pid int) error {
	if validateNamespaceBrokerExecutable(p.executable()) != nil {
		return ErrBoundary
	}
	paths := []string{"/usr/lib/systemd/system/ngfw-ra-targets@.socket", "/usr/lib/systemd/system/ngfw-ra-targets@.service"}
	hashes := []string{"7deee678c583e33dc84e094bd252a1ae421164e2aba6a2334fd175842d81417e", "765694ce9c419989f805a1042b5e556ea5dd35fd324e66e6483a7b26305d549c"}
	for index, path := range paths {
		content, err := trustedInstallationFile(path, 16384, false)
		if err != nil {
			return ErrBoundary
		}
		hash := sha256.Sum256(content)
		if hex.EncodeToString(hash[:]) != hashes[index] {
			return ErrBoundary
		}
	}
	numeric := strconv.Itoa(pid)
	fields, err := namespaceSystemdProperties(ctx, "ngfw-ra-targets@"+numeric+".socket", "FragmentPath,DropInPaths,ActiveState,SubState,Listen")
	if err != nil || fields["FragmentPath"] != paths[0] || fields["DropInPaths"] != "" || fields["ActiveState"] != "active" || fields["SubState"] != "listening" || fields["Listen"] != "/run/ngfw/ra/targets/"+numeric+".sock (SequentialPacket)" {
		return ErrBoundary
	}
	fields, err = namespaceSystemdProperties(ctx, "ngfw-ra-targets@"+numeric+".service", "FragmentPath,DropInPaths,User,CapabilityBoundingSet,NoNewPrivileges,ExecStart")
	caps := strings.Fields(fields["CapabilityBoundingSet"])
	if err != nil || fields["FragmentPath"] != paths[1] || fields["DropInPaths"] != "" || fields["User"] != "root" || fields["NoNewPrivileges"] != "yes" || len(caps) != 2 || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_admin ") || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_chroot ") || !strings.Contains(fields["ExecStart"], "path="+p.executable()+" ; argv[]="+p.executable()+" --targets "+numeric+" ;") {
		return ErrBoundary
	}
	return nil
}
