package rsyslog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers/rfkit"
)

// Counters is one impstats record: its origin and numeric fields.
type Counters struct {
	Origin string
	Values map[string]int64
}

// readStatsFrom parses impstats JSON lines ("<ctime>: {json}") written at or after offset
// (bounded to the newest maxStatsTail bytes) and stamped after since (zero: any), and returns
// the latest record per name.
func readStatsFrom(path string, offset int64, since time.Time) (map[string]Counters, error) {
	f, err := os.Open(path) //nolint:gosec // renderer-owned path
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if offset > size { // truncated since
		offset = 0
	}
	start := max(offset, size-maxStatsTail)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, maxStatsTail))
	if err != nil {
		return nil, err
	}
	if start > offset { // began mid-line
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return ParseStatsSince(b, since), nil
}

// ParseStats parses impstats JSON lines; malformed or partial lines are skipped. Only numeric
// fields are kept (names and origins are the only strings, and neither is user data).
func ParseStats(b []byte) map[string]Counters { return ParseStatsSince(b, time.Time{}) }

// statsTimeLayout is impstats' line prefix (ctime(3), local time, second resolution).
const statsTimeLayout = "Mon Jan _2 15:04:05 2006"

// ParseStatsSince is ParseStats restricted to records stamped in a later second than since
// (records of a process that stopped before since can never qualify).
func ParseStatsSince(b []byte, since time.Time) map[string]Counters {
	out := map[string]Counters{}
	for _, line := range bytes.Split(b, []byte("\n")) {
		i := bytes.Index(line, []byte(": {"))
		if i < 0 {
			continue
		}
		if !since.IsZero() {
			ts, err := time.ParseInLocation(statsTimeLayout, string(line[:i]), time.Local)
			if err != nil || !ts.After(since.Truncate(time.Second)) {
				continue
			}
		}
		var rec map[string]any
		if err := json.Unmarshal(line[i+2:], &rec); err != nil {
			continue
		}
		name, _ := rec["name"].(string)
		origin, _ := rec["origin"].(string)
		if name == "" {
			continue
		}
		c := Counters{Origin: origin, Values: map[string]int64{}}
		for k, v := range rec {
			if n, ok := v.(float64); ok {
				c.Values[k] = int64(n)
			}
		}
		out[name] = c
	}
	return out
}

// TargetState is one export target as impstats reports it.
type TargetState struct {
	Name, Target, Protocol string
	// Reported is false when impstats has no record of the action (rsyslog not running or not
	// yet reporting).
	Reported bool
	// Action counters (core.action).
	Processed, Failed, Suspended, SuspendedDuration, Resumed int64
	// Queue counters ("<name> queue", core.queue).
	QueueSize, Enqueued, Full, DiscardedFull, DiscardedNF, MaxQueueSize int64
}

// State is rsyslog's actual export state.
type State struct {
	Targets []TargetState
	// Inputs are the submitted counters per input (imuxsock, imtcp …).
	Inputs map[string]int64
	// Error is the (redacted) reason stats are unavailable.
	Error string
}

var liveTargetRe = regexp.MustCompile(`action\(type="omfwd" name="(vrx_export_[0-9]{1,2}_[0-9a-f]{8})" target="([^"]*)" port="([0-9]+)" protocol="(udp|tcp)"`)

// State reads the live config for the rendered actions and impstats (bounded tail) for their
// counters. Missing stats are reported in Error, not as an error.
func (r *Renderer) State(context.Context) (*State, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	conf, err := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
	if err != nil {
		return nil, fmt.Errorf("rsyslog: read %s: %w", r.paths.ConfFile, err)
	}
	st := &State{Inputs: map[string]int64{}}
	stats, err := readStatsFrom(r.paths.StatsFile, 0, time.Time{})
	switch {
	case errors.Is(err, os.ErrNotExist):
		st.Error = "no impstats yet"
		stats = map[string]Counters{}
	case err != nil:
		st.Error = r.red.Redact(err.Error())
		stats = map[string]Counters{}
	}
	if statsSize(r.paths.StatsFile) > statsTruncateAt && r.statsMu.TryLock() {
		_ = os.Truncate(r.paths.StatsFile, 0)
		r.statsMu.Unlock()
	}
	for _, m := range liveTargetRe.FindAllSubmatch(conf, -1) {
		name := string(m[1])
		ts := TargetState{Name: name, Target: string(m[2]) + ":" + string(m[3]), Protocol: string(m[4])}
		if a, ok := stats[name]; ok && a.Origin == "core.action" {
			ts.Reported = true
			ts.Processed, ts.Failed, ts.Suspended = a.Values["processed"], a.Values["failed"], a.Values["suspended"]
			ts.SuspendedDuration, ts.Resumed = a.Values["suspended.duration"], a.Values["resumed"]
		}
		if q, ok := stats[name+" queue"]; ok {
			ts.QueueSize, ts.Enqueued, ts.Full = q.Values["size"], q.Values["enqueued"], q.Values["full"]
			ts.DiscardedFull, ts.DiscardedNF, ts.MaxQueueSize = q.Values["discarded.full"], q.Values["discarded.nf"], q.Values["maxqsize"]
		}
		st.Targets = append(st.Targets, ts)
	}
	for name, c := range stats {
		if strings.HasPrefix(c.Origin, "im") {
			if v, ok := c.Values["submitted"]; ok {
				st.Inputs[name] = v
			}
		}
	}
	return st, nil
}

// Retrieve implements renderers.Renderer: State as a *structpb.Struct.
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, r.red.Error(err)
	}
	return st.Struct()
}

// Struct converts the state to a structpb.Struct.
func (st *State) Struct() (*structpb.Struct, error) {
	targets := make([]any, 0, len(st.Targets))
	for _, t := range st.Targets {
		targets = append(targets, map[string]any{
			"name": t.Name, "target": t.Target, "protocol": t.Protocol, "reported": t.Reported,
			"processed": float64(t.Processed), "failed": float64(t.Failed), "suspended": float64(t.Suspended),
			"suspendedDuration": float64(t.SuspendedDuration), "resumed": float64(t.Resumed),
			"queueSize": float64(t.QueueSize), "enqueued": float64(t.Enqueued), "full": float64(t.Full),
			"discardedFull": float64(t.DiscardedFull), "discardedNf": float64(t.DiscardedNF), "maxQueueSize": float64(t.MaxQueueSize),
		})
	}
	inputs := map[string]any{}
	for _, k := range slices.Sorted(maps.Keys(st.Inputs)) {
		inputs[k] = float64(st.Inputs[k])
	}
	return structpb.NewStruct(map[string]any{"targets": targets, "inputs": inputs, "error": st.Error})
}

// Poller returns the 1 Hz event source: per action whether impstats reports it, its failed
// and suspended counters, queue discards and whether its queue holds a backlog (a target that
// stopped accepting: rsyslog keeps retrying and queues, action.resumeRetryCount=-1).
// Processed counts change with every message and are not events.
func (r *Renderer) Poller() *rfkit.Poller {
	return &rfkit.Poller{
		Source: "rsyslog",
		Redact: r.red.Redact,
		Snap: func(ctx context.Context) (map[string]string, error) {
			st, err := r.State(ctx)
			if err != nil {
				return nil, err
			}
			out := map[string]string{}
			for _, t := range st.Targets {
				out[t.Name+"/reported"] = strconv.FormatBool(t.Reported)
				out[t.Name+"/failed"] = strconv.FormatInt(t.Failed, 10)
				out[t.Name+"/suspended"] = strconv.FormatInt(t.Suspended, 10)
				out[t.Name+"/discarded"] = strconv.FormatInt(t.DiscardedFull+t.DiscardedNF, 10)
				out[t.Name+"/queue"] = "empty"
				if t.QueueSize > 0 {
					out[t.Name+"/queue"] = "backlog"
				}
			}
			return out, nil
		},
	}
}
