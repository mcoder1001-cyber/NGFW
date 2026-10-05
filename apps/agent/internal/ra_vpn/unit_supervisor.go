package ravpn

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UnitObservationProvider obtains a fresh manager-attested process observation
// for exactly ngfw-ra@<fullInstance>.service. Profiles never select a PID, path,
// command or another unit. Preflight is read-only and never starts a unit.
// Acquire binds MainPID, complete process identity and cgroup before and after
// the manager opens the process NETNS and executable descriptors before cap drop.
type UnitObservationProvider interface {
	Acquire(context.Context, string) (*UnitProcessSnapshot, error)
	Preflight(context.Context) error
}

// UnitProcessSnapshot contains public ownership metadata and held observation
// descriptors only. It never carries credentials, daemon config or proc environ.
// The consumer owns both descriptors and must Close even on validation failure.
// Network must be a held CLONE_NEWNET NSFS descriptor; Executable is the actual
// process executable, compared to fixed verified installation artifacts.
type UnitProcessSnapshot struct {
	Instance     string
	Identity     UnitIdentity
	ControlGroup string
	Network      *os.File
	Executable   *os.File
}

// Close releases only descriptors returned by this observation.
func (s *UnitProcessSnapshot) Close() error {
	if s == nil {
		return nil
	}
	var failure error
	for _, file := range []*os.File{s.Network, s.Executable} {
		if file != nil && file.Close() != nil {
			failure = ErrEngine
		}
	}
	s.Network = nil
	s.Executable = nil
	return failure
}

// UnitOperation restricts trusted fixture-manager dispatch to fixed private-unit
// operations. It never carries caller-provided arguments, paths or unit names.
type UnitOperation uint8

const (
	UnitOperationStart UnitOperation = iota + 1
	UnitOperationStop
	UnitOperationObserve
	UnitOperationInactive
)

// UnitManagerState is actual manager readback, not a readiness assertion.
// Both PIDs and the cgroup must come from the exact fixed-instance unit.
type UnitManagerState struct {
	MainPID      int
	ControlPID   int
	ActiveState  string
	ControlGroup string
}

// UnitManagerDispatch is a trusted constructor-only private manager seam.
// The identifier is always a validated full instance64, never a unit or path.
// Observe and Inactive must read actual owned process/control/cgroup state;
// errors and unknown state cannot be substituted with empty/inactive results.
type UnitManagerDispatch func(context.Context, UnitOperation, string) (UnitManagerState, error)

// SystemdUnits uses fixed unit names. Tests inject their disposable supervisor.
type SystemdUnits struct {
	Observation UnitObservationProvider
	manager     UnitManagerDispatch
}

// NewSystemdUnitsForManager binds a trusted disposable manager fixture to the
// same process-observation consumer used by the fixed production dispatcher.
func NewSystemdUnitsForManager(observation UnitObservationProvider, dispatch UnitManagerDispatch) (SystemdUnits, error) {
	if observation == nil || dispatch == nil {
		return SystemdUnits{}, ErrEngine
	}
	return SystemdUnits{Observation: observation, manager: dispatch}, nil
}

func (u SystemdUnits) execute(ctx context.Context, args ...string) ([]byte, error) {
	if u.manager == nil {
		return unitCommand(ctx, args...)
	}
	if len(args) < 2 || !validUnitName(args[len(args)-1]) {
		return nil, ErrEngine
	}
	name := args[len(args)-1]
	instance := strings.TrimSuffix(strings.TrimPrefix(name, "ngfw-ra@"), ".service")
	var operation UnitOperation
	switch {
	case len(args) == 2 && args[0] == "start":
		operation = UnitOperationStart
	case len(args) == 2 && args[0] == "stop":
		operation = UnitOperationStop
	case len(args) == 4 && args[0] == "show" && args[1] == "--property=MainPID" && args[2] == "--value":
		operation = UnitOperationObserve
	case len(args) == 3 && args[0] == "show" && args[1] == "--property=MainPID,ControlPID,ActiveState,ControlGroup":
		operation = UnitOperationInactive
	default:
		return nil, ErrEngine
	}
	state, err := u.manager(ctx, operation, instance)
	if err != nil || state.MainPID < 0 || state.MainPID > 2147483647 || state.ControlPID < 0 || state.ControlPID > 2147483647 || len(state.ControlGroup) > 512 || strings.ContainsAny(state.ControlGroup, "\r\n") || len(state.ActiveState) > 32 || strings.ContainsAny(state.ActiveState, "\r\n") {
		return nil, ErrEngine
	}
	switch operation {
	case UnitOperationObserve:
		return []byte(strconv.Itoa(state.MainPID)), nil
	case UnitOperationInactive:
		return []byte(fmt.Sprintf("MainPID=%d\nControlPID=%d\nActiveState=%s\nControlGroup=%s\n", state.MainPID, state.ControlPID, state.ActiveState, state.ControlGroup)), nil
	default:
		return nil, nil
	}
}

func unitName(p *NetworkPlan) (string, error) {
	if p == nil || p.Validate() != nil || p.NamespaceInode == 0 {
		return "", ErrEngine
	}
	return "ngfw-ra@" + p.Instance + ".service", nil
}
func validUnitName(name string) bool {
	return strings.HasPrefix(name, "ngfw-ra@") && strings.HasSuffix(name, ".service") && ValidInstance(strings.TrimSuffix(strings.TrimPrefix(name, "ngfw-ra@"), ".service"))
}
func unitCommand(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if len(args) != 2 && len(args) != 3 && len(args) != 4 {
		return nil, ErrEngine
	}
	name := args[len(args)-1]
	if !validUnitName(name) {
		return nil, ErrEngine
	}
	var command *exec.Cmd
	if len(args) == 4 {
		if args[0] != "show" || args[1] != "--property=MainPID" || args[2] != "--value" {
			return nil, ErrEngine
		}
		//nolint:gosec // Fixed executable/operation; name is exactly ngfw-ra@<64 lowercase hex>.service, validated above. No shell or caller-selected options.
		command = exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID", "--value", name)
	} else if len(args) == 3 {
		if args[0] != "show" || args[1] != "--property=MainPID,ControlPID,ActiveState,ControlGroup" {
			return nil, ErrEngine
		}
		//nolint:gosec // Fixed bounded property query and validated private unit identity, no shell/caller-selected operation.
		command = exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID,ControlPID,ActiveState,ControlGroup", name)
	} else {
		switch args[0] {
		case "start":
			//nolint:gosec // Fixed executable/operation and validated private unit identity; no shell or arbitrary arguments.
			command = exec.CommandContext(ctx, "/usr/bin/systemctl", "start", name)
		case "stop":
			//nolint:gosec // Fixed executable/operation and validated private unit identity; ownership is verified before this call.
			command = exec.CommandContext(ctx, "/usr/bin/systemctl", "stop", name)
		default:
			return nil, ErrEngine
		}
	}
	b, e := command.Output()
	if e != nil || len(b) > 16384 {
		return nil, ErrEngine
	}
	return b, nil
}
func (u SystemdUnits) pid(ctx context.Context, name string) (int, error) {
	b, e := u.execute(ctx, "show", "--property=MainPID", "--value", name)
	if e != nil {
		return 0, ErrEngine
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil || pid < 0 {
		return 0, ErrEngine
	}
	return pid, nil
}

// Preflight verifies the installed manager observation boundary without starting units.
func (u SystemdUnits) Preflight(ctx context.Context) error {
	if u.Observation == nil || u.Observation.Preflight(ctx) != nil {
		return ErrEngine
	}
	return nil
}

func verifyUnitSnapshot(p *NetworkPlan, snapshot *UnitProcessSnapshot, allowHelper bool) (UnitIdentity, error) {
	if p == nil || p.Validate() != nil || p.NamespaceInode == 0 || p.HostNamespaceInode == 0 || p.NamespaceInode == p.HostNamespaceInode || snapshot == nil || snapshot.Instance != p.Instance || !snapshot.Identity.Valid() || snapshot.Network == nil || snapshot.Executable == nil {
		return UnitIdentity{}, ErrEngine
	}
	id := snapshot.Identity
	name, err := unitName(p)
	if err != nil || len(snapshot.ControlGroup) > 512 || filepath.Clean(snapshot.ControlGroup) != snapshot.ControlGroup || !strings.HasPrefix(snapshot.ControlGroup, "/system.slice/") || !strings.HasSuffix(snapshot.ControlGroup, "/"+name) {
		return UnitIdentity{}, ErrEngine
	}
	var ns unix.Stat_t
	var fs unix.Statfs_t
	fd := int(snapshot.Network.Fd())
	kind, err := unix.IoctlRetInt(fd, unix.NS_GET_NSTYPE)
	if err != nil || kind != unix.CLONE_NEWNET || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || unix.Fstat(fd, &ns) != nil || ns.Ino != p.NamespaceInode || ns.Ino == p.HostNamespaceInode || ns.Ino != id.NamespaceInode {
		return UnitIdentity{}, ErrEngine
	}
	paths := []string{"/opt/ngfw-ra/sbin/charon-systemd"}
	if allowHelper {
		paths = append(paths, "/usr/lib/ngfw/ngfw-ra-daemon")
	}
	for _, path := range paths {
		if sameUnitExecutable(snapshot.Executable, path) {
			return id, nil
		}
	}
	return UnitIdentity{}, ErrEngine
}

func sameUnitExecutable(actual *os.File, path string) bool {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	var wanted, got unix.Stat_t
	return unix.Fstat(fd, &wanted) == nil && unix.Fstat(int(actual.Fd()), &got) == nil && wanted.Mode&unix.S_IFMT == unix.S_IFREG && wanted.Uid == 0 && wanted.Mode&0022 == 0 && wanted.Mode&0111 != 0 && wanted.Nlink == 1 && wanted.Dev == got.Dev && wanted.Ino == got.Ino && wanted.Mode == got.Mode && wanted.Uid == got.Uid && got.Nlink == 1
}

func (u SystemdUnits) observe(ctx context.Context, p *NetworkPlan, pid int, allowHelper bool) (UnitIdentity, error) {
	if u.Observation == nil || pid <= 1 || p == nil {
		return UnitIdentity{}, ErrEngine
	}
	snapshot, err := u.Observation.Acquire(ctx, p.Instance)
	if snapshot != nil {
		defer func() { _ = snapshot.Close() }()
	}
	if err != nil {
		return UnitIdentity{}, ErrEngine
	}
	id, err := verifyUnitSnapshot(p, snapshot, allowHelper)
	if err != nil || id.PID != pid {
		return UnitIdentity{}, ErrEngine
	}
	return id, nil
}

// Observe verifies the exact private daemon process and namespace.
func (u SystemdUnits) Observe(ctx context.Context, p *NetworkPlan) (UnitIdentity, error) {
	name, e := unitName(p)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	pid, e := u.pid(ctx, name)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	return u.observe(ctx, p, pid, false)
}

// Start launches only an inactive validated fixed private unit.
func (u SystemdUnits) Start(ctx context.Context, p *NetworkPlan) (UnitIdentity, error) {
	if u.Preflight(ctx) != nil {
		return UnitIdentity{}, ErrEngine
	}
	name, e := unitName(p)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	prior, e := u.pid(ctx, name)
	if e != nil || prior != 0 {
		return UnitIdentity{}, ErrEngine
	}
	if _, e = u.execute(ctx, "start", name); e != nil {
		// A failed/timed-out start may still have launched the helper. Capture
		// only an exact private process proof so rollback can stop our generation.
		readback, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if pid, err := u.pid(readback, name); err == nil && pid > 1 {
			if id, err := u.observe(readback, p, pid, true); err == nil {
				return id, ErrEngine
			}
		}
		return UnitIdentity{}, ErrEngine
	}
	captured := UnitIdentity{}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if pid, e := u.pid(ctx, name); e == nil && pid > 1 {
			if id, e := u.observe(ctx, p, pid, true); e == nil {
				if captured.Valid() && captured != id {
					return captured, ErrEngine
				}
				captured = id
			}
		}
		id, e := u.Observe(ctx, p)
		if e == nil {
			return id, nil
		}
		select {
		case <-ctx.Done():
			return captured, ErrEngine
		case <-deadline.C:
			return captured, ErrEngine
		case <-ticker.C:
		}
	}
}

// Stop refuses replacements and waits for the exact owned process to exit.
func (u SystemdUnits) Stop(ctx context.Context, p *NetworkPlan, want UnitIdentity) error {
	name, e := unitName(p)
	if e != nil {
		return ErrEngine
	}
	pid, e := u.pid(ctx, name)
	if e != nil {
		return ErrEngine
	}
	if pid == 0 {
		if !want.Valid() {
			return u.Inactive(ctx, p)
		}
		if !retiredUnitProcess(want) || u.inactiveState(ctx, name) != nil {
			return ErrEngine
		}
		return nil
	}
	if !want.Valid() {
		return ErrEngine
	}
	actual, e := u.observe(ctx, p, pid, true)
	if e != nil || actual != want {
		return ErrEngine
	}
	if _, e = u.execute(ctx, "stop", name); e != nil {
		return ErrEngine
	}
	pid, e = u.pid(ctx, name)
	if e != nil || pid != 0 || !retiredUnitProcess(want) || u.inactiveState(ctx, name) != nil {
		return ErrEngine
	}
	return nil
}

// Retired proves an old kernel boot has ended and its ephemeral instance is
// absent. It never touches a live/new fixed unit or adopts a recreated root.
func (u SystemdUnits) Retired(ctx context.Context, s EngineSpec, previous UnitIdentity) (bool, error) {
	current := (bootid.Reader{}).BootID()
	if !previous.Valid() || current == "" {
		return false, ErrEngine
	}
	if current == previous.BootID {
		return false, nil
	}
	if s.Validate() != nil {
		return false, ErrEngine
	}
	if _, e := os.Lstat(InstanceRoot + "/" + s.Instance); !os.IsNotExist(e) {
		return false, ErrEngine
	}
	pid, e := u.pid(ctx, "ngfw-ra@"+s.Instance+".service")
	if e != nil || pid != 0 {
		return false, ErrEngine
	}
	return true, nil
}

func retiredUnitProcess(want UnitIdentity) bool {
	reader := bootid.Reader{}
	current := reader.BootID()
	if current == "" {
		return false
	}
	if current != want.BootID {
		return true
	}
	ticks := reader.StartTime(want.PID)
	if ticks != 0 {
		return ticks != want.StartTicks
	}
	_, err := os.Stat("/proc/" + strconv.Itoa(want.PID))
	return os.IsNotExist(err)
}
func parseInactiveUnit(data []byte) (string, error) {
	if len(data) > 16384 {
		return "", ErrEngine
	}
	properties := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return "", ErrEngine
		}
		if _, exists := properties[parts[0]]; exists {
			return "", ErrEngine
		}
		properties[parts[0]] = parts[1]
	}
	if len(properties) != 4 || properties["MainPID"] != "0" || properties["ControlPID"] != "0" || (properties["ActiveState"] != "inactive" && properties["ActiveState"] != "failed") {
		return "", ErrEngine
	}
	group, exists := properties["ControlGroup"]
	if !exists {
		return "", ErrEngine
	}
	return group, nil
}
func (u SystemdUnits) inactiveState(ctx context.Context, name string) error {
	data, err := u.execute(ctx, "show", "--property=MainPID,ControlPID,ActiveState,ControlGroup", name)
	if err != nil {
		return ErrEngine
	}
	group, err := parseInactiveUnit(data)
	if err != nil {
		return err
	}
	if group == "" {
		return nil
	}
	if len(group) > 512 || filepath.Clean(group) != group || !strings.HasPrefix(group, "/system.slice/") || !strings.HasSuffix(group, "/"+name) {
		return ErrEngine
	}
	var fs unix.Statfs_t
	if unix.Statfs("/sys/fs/cgroup", &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrEngine
	}
	fd, err := unix.Open("/sys/fs/cgroup"+group+"/cgroup.procs", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil
	}
	if err != nil {
		return ErrEngine
	}
	file := os.NewFile(uintptr(fd), "owned-engine-cgroup")
	defer func() { _ = file.Close() }()
	processes, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(processes) > 16384 || strings.TrimSpace(string(processes)) != "" {
		return ErrEngine
	}
	return nil
}

// Inactive proves no fixed-unit processes or untracked private socket exist.
func (u SystemdUnits) Inactive(ctx context.Context, plan *NetworkPlan) error {
	if plan == nil || plan.Validate() != nil {
		return ErrEngine
	}
	name := "ngfw-ra@" + plan.Instance + ".service"
	if !validUnitName(name) || u.inactiveState(ctx, name) != nil {
		return ErrEngine
	}
	root, err := unix.Open(InstanceRoot+"/"+plan.Instance, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil
	}
	if err != nil {
		return ErrEngine
	}
	defer unix.Close(root)
	var info unix.Stat_t
	if unix.Fstat(root, &info) != nil || info.Uid != 0 || info.Mode&0077 != 0 {
		return ErrEngine
	}
	daemon, err := unix.Openat(root, "daemon", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil
	}
	if err != nil {
		return ErrEngine
	}
	defer unix.Close(daemon)
	if unix.Fstat(daemon, &info) != nil || info.Uid != 0 || info.Mode&0077 != 0 {
		return ErrEngine
	}
	if unix.Fstatat(daemon, "vici.sock", &info, unix.AT_SYMLINK_NOFOLLOW) != unix.ENOENT {
		return ErrEngine
	}
	return nil
}
