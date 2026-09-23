// Package rfkit holds the pieces the single-file daemon renderers (snmpd, keepalived, rsyslog;
// RF-4) share, the way descriptors/dfkit serves the descriptor factories (D-077):
//
//   - Controller: the daemon's control channel behind one interface, with a systemd
//     implementation (product: `systemctl reload|restart|kill <unit>`) and a child-process
//     implementation (tests: signal the PID the test spawned, restart through a hook);
//   - secret references (D-051), a resolver interface and a Redactor that masks every
//     resolved value in errors, state and events;
//   - ApplyFiles: snapshot → atomic write → reload → convergence check → restore + reload on
//     any failure (AD-4; RF-1 review H2: success is only reported after the daemon shows the
//     new configuration);
//   - bounded file reads (RF-1 review M3) and a 1 Hz change poller for event streams.
//
// No process is started here except through renderers.Runner (fixed argv, allow-listed).
package rfkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"ngfw/agent/internal/renderers"
)

// SystemctlBin is the product control channel (listed in internal/renderers/ALLOWLIST.md).
const SystemctlBin = "/usr/bin/systemctl"

// Controller is a daemon's control channel.
type Controller interface {
	// Reload makes the running daemon re-read its configuration (SIGHUP semantics).
	Reload(ctx context.Context) error
	// Restart stops and starts the daemon (for daemons that cannot reload: rsyslog).
	Restart(ctx context.Context) error
	// Signal delivers sig to the daemon's main process (keepalived state dumps).
	Signal(ctx context.Context, sig syscall.Signal) error
}

// ErrNotRunning is returned when the daemon has no process to signal.
var ErrNotRunning = errors.New("rfkit: daemon is not running")

// unitRe restricts systemd unit names to plain daemon units.
var unitRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// SystemdController drives a systemd unit through systemctl (product). It never starts,
// enables or disables a unit: reload, restart and kill only.
type SystemdController struct {
	Runner renderers.Runner
	Unit   string
}

var _ Controller = (*SystemdController)(nil)

func (c *SystemdController) run(ctx context.Context, args ...string) error {
	if c.Runner == nil || !unitRe.MatchString(c.Unit) {
		return fmt.Errorf("rfkit: systemd controller needs a runner and a plain unit name (got %q)", c.Unit)
	}
	_, err := c.Runner.Run(ctx, renderers.Command{Path: SystemctlBin, Args: append(args, c.Unit)})
	if err != nil {
		return fmt.Errorf("rfkit: systemctl %s %s: %w", strings.Join(args, " "), c.Unit, err)
	}
	return nil
}

// Reload implements Controller: `systemctl reload <unit>`.
func (c *SystemdController) Reload(ctx context.Context) error { return c.run(ctx, "reload") }

// Restart implements Controller: `systemctl restart <unit>`.
func (c *SystemdController) Restart(ctx context.Context) error { return c.run(ctx, "restart") }

// Signal implements Controller: `systemctl kill --kill-whom=main --signal=<n> <unit>`.
func (c *SystemdController) Signal(ctx context.Context, sig syscall.Signal) error {
	if sig <= 0 || sig > 64 {
		return fmt.Errorf("rfkit: signal %d out of range", sig)
	}
	return c.run(ctx, "kill", "--kill-whom=main", "--signal="+strconv.Itoa(int(sig)))
}

// MainPID is implemented by controllers that can name the daemon's main process
// (convergence checks on the process itself: listening sockets, start time).
type MainPID interface {
	MainPID(ctx context.Context) (int, error)
}

// MainPID implements MainPID: `systemctl show <unit> -p MainPID --value` (0 = not running).
func (c *SystemdController) MainPID(ctx context.Context) (int, error) {
	if c.Runner == nil || !unitRe.MatchString(c.Unit) {
		return 0, fmt.Errorf("rfkit: systemd controller needs a runner and a plain unit name (got %q)", c.Unit)
	}
	out, err := c.Runner.Run(ctx, renderers.Command{Path: SystemctlBin, Args: []string{"show", c.Unit, "-p", "MainPID", "--value"}})
	if err != nil {
		return 0, fmt.Errorf("rfkit: systemctl show %s: %w", c.Unit, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out.Stdout)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("%w: unit %s has no main process", ErrNotRunning, c.Unit)
	}
	return pid, nil
}

// ProcessController signals a daemon the caller spawned (integration tests; shared-host
// rules §5: only PIDs you started). Before every signal it checks that the PID still runs
// Binary (via /proc/<pid>/exe), so a recycled PID is never signalled (RF-1 review M4).
type ProcessController struct {
	// PID returns the daemon's main PID (from the test's exec.Cmd or a pidfile: PIDFile).
	PID func() (int, error)
	// Binary is the absolute path of the daemon executable.
	Binary string
	// OnRestart restarts the daemon (the test kills its child and starts a new one).
	OnRestart func(ctx context.Context) error
}

var _ Controller = (*ProcessController)(nil)

// PIDFile returns a PID function reading a pidfile (bounded, one decimal number).
func PIDFile(path string) func() (int, error) {
	return func() (int, error) {
		b, err := ReadFileLimit(path, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: pidfile %s: %v", ErrNotRunning, path, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 1 {
			return 0, fmt.Errorf("%w: pidfile %s holds %q", ErrNotRunning, path, strings.TrimSpace(string(b)))
		}
		return pid, nil
	}
}

func (c *ProcessController) pid() (int, error) {
	if c.PID == nil {
		return 0, fmt.Errorf("%w: no PID source", ErrNotRunning)
	}
	pid, err := c.PID()
	if err != nil {
		return 0, err
	}
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return 0, fmt.Errorf("%w: pid %d: %v", ErrNotRunning, pid, err)
	}
	if c.Binary != "" && exe != c.Binary {
		// The binary may be a symlink (coreutils multicall, alternatives).
		if resolved, err := filepath.EvalSymlinks(c.Binary); err != nil || resolved != exe {
			return 0, fmt.Errorf("%w: pid %d runs %s, not %s", ErrNotRunning, pid, exe, c.Binary)
		}
	}
	return pid, nil
}

// MainPID implements MainPID (the verified PID of the child).
func (c *ProcessController) MainPID(context.Context) (int, error) { return c.pid() }

// Signal implements Controller.
func (c *ProcessController) Signal(_ context.Context, sig syscall.Signal) error {
	pid, err := c.pid()
	if err != nil {
		return err
	}
	if err := syscall.Kill(pid, sig); err != nil {
		return fmt.Errorf("rfkit: signal %d to pid %d: %w", sig, pid, err)
	}
	return nil
}

// Reload implements Controller: SIGHUP to the child.
func (c *ProcessController) Reload(ctx context.Context) error { return c.Signal(ctx, syscall.SIGHUP) }

// Restart implements Controller through OnRestart.
func (c *ProcessController) Restart(ctx context.Context) error {
	if c.OnRestart == nil {
		return errors.New("rfkit: process controller has no restart hook")
	}
	return c.OnRestart(ctx)
}
