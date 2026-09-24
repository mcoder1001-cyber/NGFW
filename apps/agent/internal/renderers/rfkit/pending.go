package rfkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ngfw/agent/internal/vpp/bootid"
)

// ActionRequired is returned by Apply when the files were written but the daemon applies a
// changed directive only at startup (listen sockets, engine id …): the caller — the commit
// engine, product: systemd — must restart the unit. It is not a failure to roll back (D-079;
// the same duck-typed shape as the RF-3 kea/unbound/chrony renderers: NeedsRestart()).
type ActionRequired struct {
	Daemon, Unit, Action, Reason string
}

func (e *ActionRequired) Error() string {
	return fmt.Sprintf("%s needs %s (unit %s): %s", e.Daemon, e.Action, e.Unit, e.Reason)
}

// NeedsRestart reports the unit and the action ("restart").
func (e *ActionRequired) NeedsRestart() (unit, action string) { return e.Unit, e.Action }

// Pending is a persisted restart request (D-079: "pending restart requests persist until acted
// on"). It is returned again by every Apply until the daemon's process started after the
// request: a later start time (clock ticks since boot, /proc/<pid>/stat field 22), or another
// boot (kernel boot_id, D-080), or — within the same tick — another PID.
type Pending struct {
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	BootID     string `json:"bootId"`
	SinceTicks uint64 `json:"sinceTicks"`
	PID        int    `json:"pid"`
}

// procReader is the /proc access (TD-1 helper); replaced in tests.
var procReader = bootid.Reader{}

// clockTicks is USER_HZ (100 on every Linux architecture the product targets).
const clockTicks = 100

func uptimeTicks() (uint64, error) {
	b, err := ReadFileLimit(filepath.Join(procRoot(), "uptime"), 256)
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("empty /proc/uptime")
	}
	sec, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, err
	}
	return uint64(sec * clockTicks), nil
}

func procRoot() string {
	if procReader.ProcRoot != "" {
		return procReader.ProcRoot
	}
	return "/proc"
}

// SetPending records a restart request made while process pid (0 = unknown) was running.
func SetPending(path, action, reason string, pid int) error {
	now, err := uptimeTicks()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(Pending{Action: action, Reason: reason, BootID: procReader.BootID(), SinceTicks: now, PID: pid})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// GetPending reads the pending request (nil = none or unreadable).
func GetPending(path string) *Pending {
	b, err := ReadFileLimit(path, 4096)
	if err != nil {
		return nil
	}
	var rec Pending
	if json.Unmarshal(b, &rec) != nil || rec.Action == "" {
		return nil
	}
	return &rec
}

// ClearPending removes the request.
func ClearPending(path string) { _ = os.Remove(path) }

// StartedAfter reports whether process pid started after the request was recorded.
func (p *Pending) StartedAfter(pid int) bool {
	if pid <= 0 {
		return false
	}
	if b := procReader.BootID(); b != "" && p.BootID != "" && b != p.BootID {
		return true
	}
	st := procReader.StartTime(pid)
	if st == 0 {
		return false
	}
	return st > p.SinceTicks || (st == p.SinceTicks && p.PID != 0 && pid != p.PID)
}
