package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const targetSupplierExecutable = "/usr/lib/ngfw/ngfw-ra-namespace-broker"
const targetSupplierService = "/usr/lib/systemd/system/ngfw-ra-targets@.service"
const targetSupplierSocket = "/usr/lib/systemd/system/ngfw-ra-targets@.socket"

// SystemdNamespaceTargets obtains fresh roles through an existing fixed socket.
// It never provisions a listener or reads another process's namespace/executable.
type SystemdNamespaceTargets struct {
	ExpectedVPP   func(context.Context) (bootid.Identity, error)
	Executable    string
	requesterRole string
}

func (p *SystemdNamespaceTargets) expected(ctx context.Context) (bootid.Identity, error) {
	if p == nil || p.ExpectedVPP == nil {
		return bootid.Identity{}, ErrBoundary
	}
	identity, err := p.ExpectedVPP(ctx)
	if err != nil || !validNumericOpenFileTarget(identity) || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return bootid.Identity{}, ErrBoundary
	}
	return identity, nil
}
func (p *SystemdNamespaceTargets) executable() string {
	if p.Executable != "" {
		return p.Executable
	}
	return targetSupplierExecutable
}
func targetCanonicalSource(ctx context.Context) (bootid.Identity, error) {
	pid, gid := os.Getpid(), os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || os.Geteuid() != 0 || gid < 0 || uint64(gid) > math.MaxUint32 {
		return bootid.Identity{}, ErrBoundary
	}
	source := (bootid.Reader{}).ForPID(pid)
	if readSourceAgentReference(source) != nil || verifyFixedAgentPeer(ctx, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, source) != nil {
		return bootid.Identity{}, ErrBoundary
	}
	return source, nil
}

// Preflight observes the existing supplier without installing or starting units.
func (p *SystemdNamespaceTargets) Preflight(ctx context.Context) error {
	snapshot, err := p.Acquire(ctx)
	if err != nil {
		return ErrBoundary
	}
	return snapshot.Close()
}

type namespaceTargetCapture struct {
	Snapshot *NamespaceTargetSnapshot
	Server   bootid.Identity
}

func (p *SystemdNamespaceTargets) acquireOnce(ctx context.Context, expected, source bootid.Identity) (*namespaceTargetCapture, error) {
	if p.validateUnits(ctx, expected, bootid.Identity{}) != nil {
		return nil, ErrBoundary
	}
	path, pathErr := NamespaceTargetSocket(expected.PID)
	if pathErr != nil {
		return nil, ErrBoundary
	}
	if brokerProtectedParent(NamespaceTargetIPCRoot) != nil {
		return nil, ErrBoundary
	}
	var info unix.Stat_t
	if unix.Lstat(path, &info) != nil || info.Mode != unix.S_IFSOCK|0600 || info.Uid != 0 || info.Gid != 0 || info.Nlink != 1 {
		return nil, ErrBoundary
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() { _ = unix.Close(fd) }()
	if boundUnitObserverSocket(ctx, fd) != nil || unix.Connect(fd, &unix.SockaddrUnix{Name: path}) != nil {
		return nil, ErrBoundary
	}
	peer, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil || peer.Pid != 1 || peer.Uid != 0 || peer.Gid != 0 {
		return nil, ErrBoundary
	}
	request := namespaceTargetRequest{Source: source, ExpectedVPP: expected, Role: "agent"}
	data, err := json.Marshal(request)
	if err != nil || len(data) > unitObserverPacketLimit || unix.Sendmsg(fd, data, nil, nil, 0) != nil {
		return nil, ErrBoundary
	}
	data, files, err := receiveUnitObserverPacket(fd, 4)
	if err != nil {
		return nil, ErrBoundary
	}
	transferred := false
	defer func() {
		if !transferred {
			closeUnitObserverFiles(files)
		}
	}()
	var response namespaceTargetResponse
	if decodeUnitObserverPacket(data, &response) != nil || !response.Server.Complete() || !response.Source.Equal(source) || !response.Targets[0].Boot.Equal(expected) || response.Targets[1].Boot.PID != 1 {
		return nil, ErrBoundary
	}
	snapshot := &NamespaceTargetSnapshot{Targets: response.Targets, Source: response.Source, Files: [2]*os.File{files[0], files[1]}, Executables: [2]*os.File{files[2], files[3]}}
	if snapshot.Validate() != nil || validateTargetHeldImages(snapshot) != nil || p.validateUnits(ctx, expected, response.Server) != nil {
		return nil, ErrBoundary
	}
	fresh, err := p.expected(ctx)
	currentSource, sourceErr := targetCanonicalSource(ctx)
	if err != nil || sourceErr != nil || !fresh.Equal(expected) || !currentSource.Equal(source) || !(bootid.Reader{}).ForPID(1).Equal(snapshot.Targets[1].Boot) || ctx.Err() != nil || unix.Sendmsg(fd, []byte("OK"), nil, nil, 0) != nil {
		return nil, ErrBoundary
	}
	transferred = true
	return &namespaceTargetCapture{Snapshot: snapshot, Server: response.Server}, nil
}

// Acquire compares exactly two fresh manager activations and returns the second.
func (p *SystemdNamespaceTargets) Acquire(ctx context.Context) (*NamespaceTargetSnapshot, error) {
	if p == nil || (p.requesterRole != "" && p.requesterRole != "agent") {
		return nil, ErrBoundary
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	expected, err := p.expected(bounded)
	if err != nil {
		return nil, ErrBoundary
	}
	source, err := targetCanonicalSource(bounded)
	if err != nil {
		return nil, ErrBoundary
	}
	first, err := p.acquireOnce(bounded, expected, source)
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() { _ = first.Snapshot.Close() }()
	if waitTargetSupplierExit(bounded, first.Server, expected.PID) != nil {
		return nil, ErrBoundary
	}
	second, err := p.acquireOnce(bounded, expected, source)
	if err != nil {
		return nil, ErrBoundary
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = second.Snapshot.Close()
		}
	}()
	if first.Server.Equal(second.Server) || !first.Snapshot.SameTargets(second.Snapshot) {
		return nil, ErrBoundary
	}
	fresh, err := p.expected(bounded)
	current, sourceErr := targetCanonicalSource(bounded)
	if err != nil || sourceErr != nil || !fresh.Equal(expected) || !current.Equal(source) || bounded.Err() != nil {
		return nil, ErrBoundary
	}
	accepted = true
	return second.Snapshot, nil
}

func waitTargetSupplierExit(ctx context.Context, server bootid.Identity, targetPID int) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !(bootid.Reader{}).ForPID(server.PID).Equal(server) {
			fields, err := namespaceSystemdProperties(bounded, "ngfw-ra-targets@"+strconv.Itoa(targetPID)+".service", "MainPID,ActiveState,SubState")
			if err == nil && fields["MainPID"] == "0" && fields["ActiveState"] == "inactive" && fields["SubState"] == "dead" {
				return nil
			}
		}
		select {
		case <-bounded.Done():
			return ErrBoundary
		case <-ticker.C:
		}
	}
}

func validateTargetHeldImages(snapshot *NamespaceTargetSnapshot) error {
	if snapshot == nil || snapshot.Validate() != nil || brokerProtectedParent("/usr/bin") != nil || !sameUnitExecutable(snapshot.Executables[0], "/usr/bin/vpp") || validateSourceAgentExecutable(snapshot.Executables[1]) != nil {
		return ErrBoundary
	}
	return nil
}

func targetSupplierControlGroup(pid int, group string) bool {
	unit := "ngfw-ra-targets@" + strconv.Itoa(pid) + ".service"
	return group == "/system.slice/"+unit || group == "/system.slice/system-ngfw\\x2dra\\x2dtargets.slice/"+unit
}

func (p *SystemdNamespaceTargets) validateUnits(ctx context.Context, target, server bootid.Identity) error {
	if p == nil || p.executable() != targetSupplierExecutable || validateNamespaceBrokerExecutable(p.executable()) != nil || readTargetsOpenFile(target) != nil {
		return ErrBoundary
	}
	for _, item := range []struct{ path, digest string }{{targetSupplierSocket, targetSupplierSocketDigest}, {targetSupplierService, targetSupplierServiceDigest}} {
		content, err := trustedInstallationFile(item.path, 16384, false)
		digest := sha256.Sum256(content)
		if err != nil || hex.EncodeToString(digest[:]) != item.digest {
			return ErrBoundary
		}
	}
	numeric := strconv.Itoa(target.PID)
	socketPath, pathErr := NamespaceTargetSocket(target.PID)
	if pathErr != nil {
		return ErrBoundary
	}
	dropIn, err := TargetsOpenFilePath(target.PID)
	if err != nil {
		return ErrBoundary
	}
	fields, err := namespaceSystemdProperties(ctx, "ngfw-ra-targets@"+numeric+".socket", "FragmentPath,DropInPaths,ActiveState,SubState,Listen")
	if err != nil || fields["FragmentPath"] != targetSupplierSocket || fields["DropInPaths"] != "" || fields["ActiveState"] != "active" || fields["SubState"] != "listening" || fields["Listen"] != socketPath+" (SequentialPacket)" {
		return ErrBoundary
	}
	fields, err = namespaceSystemdProperties(ctx, "ngfw-ra-targets@"+numeric+".service", "MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart")
	if err != nil || fields["FragmentPath"] != targetSupplierService || fields["DropInPaths"] != dropIn || fields["User"] != "root" || fields["Group"] != "ngfw" || fields["CapabilityBoundingSet"] != "" || fields["NoNewPrivileges"] != "yes" || !strings.Contains(fields["ExecStart"], "path="+targetSupplierExecutable+" ; argv[]="+targetSupplierExecutable+" --targets "+numeric+" ;") {
		return ErrBoundary
	}
	if server.Complete() {
		if validateTargetSupplierGroup(server.PID) != nil {
			return ErrBoundary
		}
		if fields["MainPID"] != strconv.Itoa(server.PID) || !targetSupplierControlGroup(target.PID, fields["ControlGroup"]) || !(bootid.Reader{}).ForPID(server.PID).Equal(server) || validateNamespaceBrokerProcess(server.PID, 0) != nil {
			return ErrBoundary
		}
		group, err := os.ReadFile("/proc/" + strconv.Itoa(server.PID) + "/cgroup")
		if err != nil || len(group) > 16384 || strings.TrimSpace(string(group)) != "0::"+fields["ControlGroup"] {
			return ErrBoundary
		}
		image, err := os.Open("/proc/" + strconv.Itoa(server.PID) + "/exe")
		if err != nil {
			return ErrBoundary
		}
		valid := sameUnitExecutable(image, targetSupplierExecutable)
		closeErr := image.Close()
		if !valid || closeErr != nil {
			return ErrBoundary
		}
	}
	if verifyBrokerVPPUnitIdentity(ctx, MountTarget{Boot: target}) != nil {
		return ErrBoundary
	}
	return readTargetsOpenFile(target)
}
func namespaceSystemdProperties(ctx context.Context, unit, properties string) (map[string]string, error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// #nosec G204 -- fixed read-only executable; internal callers derive canonical unit names and constant property lists, never profile input or shell commands.
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

func validateTargetSupplierGroup(pid int) error {
	group, err := user.LookupGroup("ngfw")
	if err != nil {
		return ErrBoundary
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil {
		return ErrBoundary
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil || len(data) > 16384 {
		return ErrBoundary
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Gid:") {
			continue
		}
		values := strings.Fields(strings.TrimPrefix(line, "Gid:"))
		if len(values) != 4 {
			return ErrBoundary
		}
		for _, value := range values {
			actual, err := strconv.ParseUint(value, 10, 32)
			if err != nil || actual != gid {
				return ErrBoundary
			}
		}
		return nil
	}
	return ErrBoundary
}
