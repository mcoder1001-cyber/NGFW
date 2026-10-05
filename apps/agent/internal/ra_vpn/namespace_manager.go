package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// NamespaceBrokerDispatch is a trusted manager control seam. Profile input
// never selects a command, path, unit, target PID, FD or implementation.
type NamespaceBrokerDispatch interface {
	Preflight(context.Context) error
	Run(context.Context, string) error
}

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

type namespaceBrokerRequest struct {
	Operation                  string
	Instance                   string
	Source                     MountTarget
	Targets                    []MountTarget
	Target                     MountTarget
	MountFD, HostFD, PrivateFD int
	HostNamespace, Namespace   uint64
}

// SystemdNamespaceBroker invokes only the fixed root-owned manager unit.
type SystemdNamespaceBroker struct{ Executable string }

const namespaceBrokerUnit = "/usr/lib/systemd/system/ngfw-ra-namespace@.service"
const expectedNamespaceBrokerUnit = "0dab475a2527cb9391a3b29bfd280558ce400f0e92da147484d576582f7c6a9e"

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
	command := exec.CommandContext(bounded, "/usr/bin/systemctl", "show", "--property=FragmentPath,DropInPaths,User,CapabilityBoundingSet,NoNewPrivileges,ExecStart", "ngfw-ra-namespace@"+strings.Repeat("0", 64)+".service")
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
	if len(caps) != 2 || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_admin ") || !strings.Contains(" "+fields["CapabilityBoundingSet"]+" ", " cap_sys_chroot ") || fields["NoNewPrivileges"] != "yes" || fields["User"] != "root" || fields["FragmentPath"] != namespaceBrokerUnit || fields["DropInPaths"] != "" || !strings.Contains(fields["ExecStart"], "path="+b.Executable+" ;") {
		return ErrBoundary
	}
	return nil
}
func (*SystemdNamespaceBroker) Run(ctx context.Context, instance string) error {
	if !ValidInstance(instance) {
		return ErrBoundary
	}
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", "start", "ngfw-ra-namespace@"+instance+".service")
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if command.Run() != nil {
		return ErrBoundary
	}
	return nil
}
func writeNamespaceBrokerRequest(request namespaceBrokerRequest) error {
	if !ValidInstance(request.Instance) || !request.Source.Boot.Complete() {
		return ErrBoundary
	}
	root := filepath.Join(InstanceRoot, request.Instance)
	if brokerProtectedParent(root) != nil {
		return ErrBoundary
	}
	data, e := json.Marshal(request)
	if e != nil || len(data) > 16384 {
		return ErrBoundary
	}
	fd, e := unix.Open(filepath.Join(root, "namespace-request.json"), unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "protected broker request")
	_, e = file.Write(data)
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e != nil || ce != nil {
		return ErrBoundary
	}
	return nil
}

// RunManagedNamespaceBroker authenticates the protected root request and pins
// all three typed descriptors before any mount namespace change. The unit
// supplies only a fixed full instance identifier as its argument.
func RunManagedNamespaceBroker(instance string) error {
	if os.Geteuid() != 0 || !ValidInstance(instance) {
		return ErrBoundary
	}
	path := filepath.Join(InstanceRoot, instance, "namespace-request.json")
	if ValidatePrivateFile(path, 16384) != nil {
		return ErrBoundary
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "protected broker request")
	var request namespaceBrokerRequest
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	de := decoder.Decode(&request)
	if de == nil && decoder.Decode(new(any)) != io.EOF {
		de = ErrBoundary
	}
	ce := file.Close()
	if de != nil || ce != nil || request.Instance != instance || len(request.Targets) != 2 || request.Namespace == 0 || request.HostNamespace == 0 || request.Namespace == request.HostNamespace {
		return ErrBoundary
	}
	if request.Operation != "export" && request.Operation != "verify" && request.Operation != "remove" {
		return ErrBoundary
	}
	if !request.Source.Boot.Complete() || request.Source.Boot.PID <= 1 || !(bootid.Reader{}).ForPID(request.Source.Boot.PID).Equal(request.Source.Boot) {
		return ErrBoundary
	}
	sourceRoot := "/proc/" + strconv.Itoa(request.Source.Boot.PID)
	status, e := os.ReadFile(sourceRoot + "/status")
	if e != nil || len(status) > 16384 {
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
	capabilities, e := strconv.ParseUint(fields["CapEff"], 16, 64)
	allowed := uint64(1<<unix.CAP_NET_ADMIN | 1<<unix.CAP_SYS_ADMIN | 1<<unix.CAP_IPC_LOCK)
	if len(uids) != 4 || uids[0] != "0" || uids[1] != "0" || uids[2] != "0" || uids[3] != "0" || e != nil || capabilities != allowed || fields["NoNewPrivs"] != "1" {
		return ErrBoundary
	}
	var sourceMount unix.Stat_t
	if unix.Stat(sourceRoot+"/ns/mnt", &sourceMount) != nil || sourceMount.Ino != request.Source.MountInode {
		return ErrBoundary
	}
	// The manager role is PID1, and VPP must be the installed executable. Targets
	// are additionally bound to the authenticated agent's pending export receipt.
	if request.Targets[1].Boot.PID != 1 {
		return ErrBoundary
	}
	executable, e := os.Readlink("/proc/" + strconv.Itoa(request.Targets[0].Boot.PID) + "/exe")
	if e != nil || executable != "/usr/bin/vpp" {
		return ErrBoundary
	}
	matched := false
	for _, target := range request.Targets {
		if !target.Boot.Complete() || !(bootid.Reader{}).ForPID(target.Boot.PID).Equal(target.Boot) {
			return ErrBoundary
		}
		var current unix.Stat_t
		if unix.Stat("/proc/"+strconv.Itoa(target.Boot.PID)+"/ns/mnt", &current) != nil || current.Ino != target.MountInode {
			return ErrBoundary
		}
		if target.Boot.Equal(request.Target.Boot) && target.MountInode == request.Target.MountInode {
			matched = true
		}
	}
	if !matched {
		return ErrBoundary
	}
	numbers := []int{request.MountFD, request.HostFD, request.PrivateFD}
	kinds := []int{unix.CLONE_NEWNS, unix.CLONE_NEWNET, unix.CLONE_NEWNET}
	inodes := []uint64{request.Target.MountInode, request.HostNamespace, request.Namespace}
	held := make([]int, 0, 3)
	defer func() {
		for _, fd := range held {
			_ = unix.Close(fd)
		}
	}()
	for i, number := range numbers {
		if number < 3 || number > 1<<20 {
			return ErrBoundary
		}
		fd, e := unix.Open(sourceRoot+"/fd/"+strconv.Itoa(number), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if e != nil {
			return ErrBoundary
		}
		held = append(held, fd)
		if brokerNamespaceFD(fd, kinds[i], inodes[i]) != nil {
			return ErrBoundary
		}
	}
	if !(bootid.Reader{}).ForPID(request.Source.Boot.PID).Equal(request.Source.Boot) {
		return ErrBoundary
	}
	arguments := []string{request.Operation, instance, request.Target.Boot.String(), strconv.FormatUint(request.Target.MountInode, 10), strconv.FormatUint(request.HostNamespace, 10), strconv.FormatUint(request.Namespace, 10)}
	return runNamespaceBrokerFDs(arguments, held)
}
