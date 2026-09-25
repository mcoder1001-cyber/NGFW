package rsyslog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

// DeferredController is the control channel of an rsyslog instance the agent must not restart itself
// (F-unbound-chrony-syslog): a test slot's instance (shared-host rules: never the host's rsyslog, kill only PIDs you
// started). Restart records a pending restart request (D-079, rfkit.SetPending) and returns *rfkit.ActionRequired;
// Apply then keeps the written files (no convergence check, no rollback) and every later Apply returns the request
// again until the instance's process — read from PIDFile (`rsyslogd -i <file>`) — started after it. The slot
// harness (the product: systemd) performs the restart.
type DeferredController struct {
	// PendingFile persists the request.
	PendingFile string
	// PIDFile is the instance's pidfile ("" = unknown: the instance is treated as not running).
	PIDFile string
	// Binary is the executable the pidfile's process must run ("" = RsyslogdBin): a recycled pid is never taken
	// for the instance.
	Binary string
}

var _ rfkit.Controller = (*DeferredController)(nil)

// PID returns the running instance's pid (0 = not running), verified to be rsyslogd.
func (c *DeferredController) PID() int {
	if c.PIDFile == "" {
		return 0
	}
	pid, err := rfkit.PIDFile(c.PIDFile)()
	if err != nil {
		return 0
	}
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return 0
	}
	bin := c.Binary
	if bin == "" {
		bin = RsyslogdBin
	}
	if exe != bin {
		if resolved, err := filepath.EvalSymlinks(bin); err != nil || resolved != exe {
			return 0
		}
	}
	return pid
}

// request is the action the written configuration needs: restart a running instance, start a stopped one.
func (c *DeferredController) request(reason string) error {
	pid := c.PID()
	if pid == 0 {
		rfkit.ClearPending(c.PendingFile) // the next start reads the new file
		return &rfkit.ActionRequired{Daemon: "rsyslogd", Unit: "rsyslog", Action: "start", Reason: reason + "; rsyslogd is not running"}
	}
	if err := rfkit.SetPending(c.PendingFile, "restart", reason, pid); err != nil {
		return fmt.Errorf("rsyslog: record pending restart: %w", err)
	}
	return &rfkit.ActionRequired{Daemon: "rsyslogd", Unit: "rsyslog", Action: "restart", Reason: reason}
}

// Pending returns the restart request the running instance has not acted on (nil when none; a request the
// instance has acted on is cleared).
func (c *DeferredController) Pending() *rfkit.Pending {
	rec := rfkit.GetPending(c.PendingFile)
	if rec == nil {
		return nil
	}
	if pid := c.PID(); pid != 0 && rec.StartedAfter(pid) {
		rfkit.ClearPending(c.PendingFile)
		return nil
	}
	return rec
}

// Reload implements rfkit.Controller (rsyslog cannot reload a configuration): a restart request.
func (c *DeferredController) Reload(context.Context) error {
	return c.request("the export configuration changed (rsyslog applies a configuration only at start)")
}

// Restart implements rfkit.Controller: a restart request, never a restart.
func (c *DeferredController) Restart(context.Context) error {
	return c.request("the export configuration changed (rsyslog applies a configuration only at start)")
}

// Signal implements rfkit.Controller: refused (the agent signals only processes it started).
func (c *DeferredController) Signal(context.Context, syscall.Signal) error {
	return fmt.Errorf("%w: the agent does not signal an rsyslog instance it did not start", rfkit.ErrNotRunning)
}

// applyDeferred is Apply for a DeferredController: snapshot → atomic write → restart request (the files stay; there
// is nothing to verify until the instance restarts). Unchanged files → the still-pending request, if any.
func (r *Renderer) applyDeferred(files renderers.Files, dc *DeferredController) error {
	if unchanged(files) {
		r.pruneTLS(files)
		if rec := dc.Pending(); rec != nil {
			return &rfkit.ActionRequired{Daemon: "rsyslogd", Unit: "rsyslog", Action: rec.Action, Reason: rec.Reason + " (still pending: rsyslogd has not restarted since)"}
		}
		return nil
	}
	snap, err := renderers.TakeSnapshot(files.Paths()...)
	if err != nil {
		return err
	}
	if err := renderers.WriteFiles(files); err != nil {
		return r.red.Error(errors.Join(err, snap.Restore()))
	}
	r.pruneTLS(files)
	if dc.PID() == 0 && !bytes.Contains(files[r.paths.ConfFile].Content, []byte("action(")) {
		rfkit.ClearPending(dc.PendingFile) // an empty export needs no instance at all
		return nil
	}
	return dc.Restart(context.Background())
}
