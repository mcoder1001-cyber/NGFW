package unbound

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// State is Unbound's actual state, decoded from unbound-control's line output.
type State struct {
	Running    bool              `json:"running"`
	Status     map[string]string `json:"status,omitempty"`
	Stats      map[string]string `json:"stats,omitempty"`
	Forwards   []Zone            `json:"forwards"`
	Stubs      []Zone            `json:"stubs"`
	LocalZones []LocalZone       `json:"localZones"`
	LocalData  []string          `json:"localData"`
	// LocalDataTruncated: list_local_data exceeded the capture limit (large blocklists);
	// LocalData is then empty rather than silently partial (review L1).
	LocalDataTruncated bool `json:"localDataTruncated,omitempty"`
}

// Zone is one list_forwards / list_stubs line: "<zone> IN <forward|stub> [+flags] <addr>…".
type Zone struct {
	Zone  string   `json:"zone"`
	Kind  string   `json:"kind"`
	Flags []string `json:"flags,omitempty"`
	Addrs []string `json:"addrs"`
}

// LocalZone is one list_local_zones line: "<zone> <type>".
type LocalZone struct {
	Zone string `json:"zone"`
	Type string `json:"type"`
}

// State reads the daemon; a daemon that is not running is Running false.
func (r *Renderer) State(ctx context.Context) (State, error) {
	st := State{Forwards: []Zone{}, Stubs: []Zone{}, LocalZones: []LocalZone{}, LocalData: []string{}}
	out, err := r.Control(ctx, "status")
	if errors.Is(err, ErrNotRunning) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.Running, st.Status = true, ParseStatus(out)
	if out, err = r.Control(ctx, "stats_noreset"); err != nil {
		return st, err
	}
	st.Stats = ParseStats(out)
	if out, err = r.Control(ctx, "list_forwards"); err != nil {
		return st, err
	}
	st.Forwards = ParseZones(out)
	if out, err = r.Control(ctx, "list_stubs"); err != nil {
		return st, err
	}
	st.Stubs = ParseZones(out)
	if out, err = r.Control(ctx, "list_local_zones"); err != nil {
		return st, err
	}
	st.LocalZones = ParseLocalZones(out)
	switch out, err = r.Control(ctx, "list_local_data"); {
	case errors.Is(err, ErrOutputTruncated):
		st.LocalDataTruncated = true
	case err != nil:
		return st, err
	default:
		st.LocalData = ParseLocalData(out)
	}
	return st, nil
}

func lines(b []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ParseStatus decodes `status` ("version: 1.24.2", "is running..." …) into key → value.
func ParseStatus(b []byte) map[string]string {
	m := map[string]string{}
	for _, l := range lines(b) {
		if k, v, ok := strings.Cut(l, ":"); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			m["state"] = l
		}
	}
	return m
}

// ParseStats decodes `stats_noreset` ("total.num.queries=12") into name → value.
func ParseStats(b []byte) map[string]string {
	m := map[string]string{}
	for _, l := range lines(b) {
		if k, v, ok := strings.Cut(l, "="); ok {
			m[k] = v
		}
	}
	return m
}

// ParseZones decodes list_forwards / list_stubs.
func ParseZones(b []byte) []Zone {
	out := []Zone{}
	for _, l := range lines(b) {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		z := Zone{Zone: f[0], Kind: f[2], Addrs: []string{}}
		for _, x := range f[3:] {
			if strings.HasPrefix(x, "+") {
				z.Flags = append(z.Flags, x)
			} else {
				z.Addrs = append(z.Addrs, x)
			}
		}
		out = append(out, z)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Zone < out[j].Zone })
	return out
}

// ParseLocalZones decodes list_local_zones.
func ParseLocalZones(b []byte) []LocalZone {
	out := []LocalZone{}
	for _, l := range lines(b) {
		if f := strings.Fields(l); len(f) >= 2 {
			out = append(out, LocalZone{Zone: f[0], Type: f[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Zone < out[j].Zone })
	return out
}

// ParseLocalData decodes list_local_data (one RR per line, tabs normalised to spaces).
func ParseLocalData(b []byte) []string {
	out := []string{}
	for _, l := range lines(b) {
		out = append(out, strings.Join(strings.Fields(l), " "))
	}
	sort.Strings(out)
	return out
}

func toStruct(v any) (*structpb.Struct, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("unbound: state: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unbound: state: %w", err)
	}
	return structpb.NewStruct(m)
}

// ----- events -----------------------------------------------------------------------------

// DefaultPollInterval is the event polling period (Unbound pushes nothing).
const DefaultPollInterval = time.Second

// watchedStats are the counters that produce events.
var watchedStats = []string{"total.num.queries", "total.num.cachehits", "total.num.cachemiss", "total.requestlist.current.all"}

// Event is one observed change.
type Event struct {
	Key, Old, New string
}

func (e Event) String() string { return fmt.Sprintf("unbound %s: %q -> %q", e.Key, e.Old, e.New) }

// ToProto maps the event to the agent's Event message (details in attributes).
func (e Event) ToProto() *vrxv1.Event {
	return &vrxv1.Event{
		Kind:       vrxv1.EventKind_EVENT_KIND_UNSPECIFIED,
		Message:    e.String(),
		Attributes: map[string]string{"source": "unbound", "key": e.Key, "old": e.Old, "new": e.New},
	}
}

// Poller samples `stats_noreset` and reports changes.
type Poller struct {
	r    *Renderer
	last map[string]string
}

// NewPoller returns a poller over r.
func (r *Renderer) NewPoller() *Poller { return &Poller{r: r} }

// Poll takes one sample and returns the changes since the previous one.
func (p *Poller) Poll(ctx context.Context) ([]Event, error) {
	cur := map[string]string{"running": "false"}
	out, err := p.r.Control(ctx, "stats_noreset")
	switch {
	case errors.Is(err, ErrNotRunning):
	case err != nil:
		return nil, err
	default:
		cur["running"] = "true"
		stats := ParseStats(out)
		for _, k := range watchedStats {
			if v, ok := stats[k]; ok {
				cur[k] = v
			}
		}
	}
	var evs []Event
	keys := make([]string, 0, len(cur))
	for k := range cur {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if p.last[k] != cur[k] {
			evs = append(evs, Event{Key: k, Old: p.last[k], New: cur[k]})
		}
	}
	for k, v := range p.last {
		if _, ok := cur[k]; !ok {
			evs = append(evs, Event{Key: k, Old: v})
		}
	}
	p.last = cur
	return evs, nil
}

// Run polls every interval until ctx ends.
func (p *Poller) Run(ctx context.Context, interval time.Duration, emit func(Event)) error {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	lastErr := ""
	for {
		evs, err := p.Poll(ctx)
		if err != nil && err.Error() != lastErr && ctx.Err() == nil {
			emit(Event{Key: "error", Old: lastErr, New: err.Error()})
			lastErr = err.Error()
		}
		for _, e := range evs {
			emit(e)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
