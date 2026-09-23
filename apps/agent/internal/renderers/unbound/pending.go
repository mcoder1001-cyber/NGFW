package unbound

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Pending restart requests (D-079, review M2). A restart request is written to
// Paths.PendingFile together with the system uptime (clock ticks) at which it was made; it is
// returned again by every Apply until the daemon's process start time (/proc/<pid>/stat) is
// later than the request, i.e. until the daemon runs the configuration that needed it.

type pendingRecord struct {
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	SinceTicks uint64 `json:"sinceTicks"`
	// PID is the daemon process that was running when the request was made (0 = unknown).
	PID int `json:"pid"`
}

// clockTicks is USER_HZ, 100 on every Linux architecture the product targets.
const clockTicks = 100

func uptimeTicks() (uint64, error) {
	b, err := os.ReadFile("/proc/uptime")
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

// procStartTicks is field 22 (starttime) of /proc/<pid>/stat.
func procStartTicks(pid int) (uint64, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, errors.New("malformed stat")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return 0, errors.New("short stat")
	}
	return strconv.ParseUint(f[19], 10, 64) // fields after ")" start at field 3
}

func setPending(path, action, reason string, pid int) error {
	now, err := uptimeTicks()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(pendingRecord{Action: action, Reason: reason, SinceTicks: now, PID: pid})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func getPending(path string) *pendingRecord {
	b, err := os.ReadFile(path) //nolint:gosec // renderer-owned state file
	if err != nil {
		return nil
	}
	var rec pendingRecord
	if json.Unmarshal(b, &rec) != nil || rec.Action == "" {
		return nil
	}
	return &rec
}

func clearPending(path string) { _ = os.Remove(path) }

// startedAfter reports whether process pid started after the request was recorded: a later
// start tick, or — within the same 10 ms tick — a different process than the one running when
// the request was made.
func (rec *pendingRecord) startedAfter(pid int) bool {
	st, err := procStartTicks(pid)
	if err != nil {
		return false
	}
	return st > rec.SinceTicks || (st == rec.SinceTicks && rec.PID != 0 && pid != rec.PID)
}
