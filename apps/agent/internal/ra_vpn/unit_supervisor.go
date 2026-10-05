package ravpn

import (
	"context"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SystemdUnits uses fixed unit names. Tests inject their disposable supervisor.
type SystemdUnits struct{}

func unitName(p *NetworkPlan) (string, error) {
	if p == nil || p.Validate() != nil || p.NamespaceInode == 0 {
		return "", ErrEngine
	}
	return "ngfw-ra@" + p.Instance + ".service", nil
}
func unitCommand(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/usr/bin/systemctl", args...).Output()
	if e != nil || len(b) > 16384 {
		return nil, ErrEngine
	}
	return b, nil
}
func unitPID(ctx context.Context, name string) (int, error) {
	b, e := unitCommand(ctx, "show", "--property=MainPID", "--value", name)
	if e != nil {
		return 0, ErrEngine
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil || pid < 0 {
		return 0, ErrEngine
	}
	return pid, nil
}
func observeProcess(p *NetworkPlan, pid int, allowHelper bool) (UnitIdentity, error) {
	if pid <= 1 {
		return UnitIdentity{}, ErrEngine
	}
	var ns, host unix.Stat_t
	if unix.Stat("/proc/"+strconv.Itoa(pid)+"/ns/net", &ns) != nil || unix.Stat("/proc/1/ns/net", &host) != nil || ns.Ino != p.NamespaceInode || ns.Ino == host.Ino {
		return UnitIdentity{}, ErrEngine
	}
	exe, e := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if e != nil || exe != "/opt/ngfw-ra/sbin/charon-systemd" && (!allowHelper || exe != "/usr/lib/ngfw/ngfw-ra-daemon") {
		return UnitIdentity{}, ErrEngine
	}
	cg, e := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	name, _ := unitName(p)
	if e != nil || len(cg) > 16384 || !strings.Contains(string(cg), "/"+name+"\n") {
		return UnitIdentity{}, ErrEngine
	}
	ticks := (bootid.Reader{}).StartTime(pid)
	if ticks == 0 {
		return UnitIdentity{}, ErrEngine
	}
	boot := (bootid.Reader{}).BootID()
	if boot == "" {
		return UnitIdentity{}, ErrEngine
	}
	return UnitIdentity{BootID: boot, PID: pid, StartTicks: ticks, NamespaceInode: ns.Ino}, nil
}
func (SystemdUnits) Observe(ctx context.Context, p *NetworkPlan) (UnitIdentity, error) {
	name, e := unitName(p)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	pid, e := unitPID(ctx, name)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	return observeProcess(p, pid, false)
}
func (u SystemdUnits) Start(ctx context.Context, p *NetworkPlan) (UnitIdentity, error) {
	name, e := unitName(p)
	if e != nil {
		return UnitIdentity{}, ErrEngine
	}
	prior, e := unitPID(ctx, name)
	if e != nil || prior != 0 {
		return UnitIdentity{}, ErrEngine
	}
	if _, e = unitCommand(ctx, "start", name); e != nil {
		return UnitIdentity{}, ErrEngine
	}
	captured := UnitIdentity{}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if pid, e := unitPID(ctx, name); e == nil && pid > 1 {
			if id, e := observeProcess(p, pid, true); e == nil {
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
func (u SystemdUnits) Stop(ctx context.Context, p *NetworkPlan, want UnitIdentity) error {
	name, e := unitName(p)
	if e != nil {
		return ErrEngine
	}
	pid, e := unitPID(ctx, name)
	if e != nil {
		return ErrEngine
	}
	if pid == 0 {
		return nil
	}
	if !want.Valid() {
		return ErrEngine
	}
	actual, e := observeProcess(p, pid, true)
	if e != nil || actual != want {
		return ErrEngine
	}
	if _, e = unitCommand(ctx, "stop", name); e != nil {
		return ErrEngine
	}
	pid, e = unitPID(ctx, name)
	if e != nil || pid != 0 || (bootid.Reader{}).StartTime(want.PID) == want.StartTicks {
		return ErrEngine
	}
	return nil
}

// Retired proves an old kernel boot has ended and its ephemeral instance is
// absent. It never touches a live/new fixed unit or adopts a recreated root.
func (SystemdUnits) Retired(ctx context.Context, s EngineSpec, u UnitIdentity) (bool, error) {
	current := (bootid.Reader{}).BootID()
	if !u.Valid() || current == "" {
		return false, ErrEngine
	}
	if current == u.BootID {
		return false, nil
	}
	if s.Validate() != nil {
		return false, ErrEngine
	}
	if _, e := os.Lstat(InstanceRoot + "/" + s.Instance); !os.IsNotExist(e) {
		return false, ErrEngine
	}
	pid, e := unitPID(ctx, "ngfw-ra@"+s.Instance+".service")
	if e != nil || pid != 0 {
		return false, ErrEngine
	}
	return true, nil
}
