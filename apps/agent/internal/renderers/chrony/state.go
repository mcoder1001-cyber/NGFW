package chrony

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// chronyc -c prints CSV without a header; the column names below follow `chronyc` (4.8)
// human-readable output, in order.

// Source is one `chronyc -c sources` row.
type Source struct {
	Mode    string  `json:"mode"`  // "^" server, "=" peer, "#" refclock
	State   string  `json:"state"` // "*" selected, "+" combined, "-" not combined, "?" unusable, "x" falseticker, "~" variable
	Name    string  `json:"name"`
	Stratum int     `json:"stratum"`
	Poll    int     `json:"poll"`
	Reach   string  `json:"reach"` // octal
	LastRx  string  `json:"lastRx"`
	Offset  float64 `json:"offset"`   // adjusted offset of the last sample (s)
	Measure float64 `json:"measured"` // measured offset (s)
	Error   float64 `json:"error"`    // estimated error (s)
}

// SourceStats is one `chronyc -c sourcestats` row.
type SourceStats struct {
	Name      string  `json:"name"`
	NP        int     `json:"np"`
	NR        int     `json:"nr"`
	Span      int     `json:"span"`
	Frequency float64 `json:"frequency"`
	FreqSkew  float64 `json:"freqSkew"`
	Offset    float64 `json:"offset"`
	StdDev    float64 `json:"stdDev"`
}

// Tracking is `chronyc -c tracking`.
type Tracking struct {
	RefID          string  `json:"refId"`
	RefName        string  `json:"refName"`
	Stratum        int     `json:"stratum"`
	RefTime        float64 `json:"refTime"`
	SystemTime     float64 `json:"systemTime"`
	LastOffset     float64 `json:"lastOffset"`
	RMSOffset      float64 `json:"rmsOffset"`
	Frequency      float64 `json:"frequency"`
	ResidualFreq   float64 `json:"residualFreq"`
	Skew           float64 `json:"skew"`
	RootDelay      float64 `json:"rootDelay"`
	RootDispersion float64 `json:"rootDispersion"`
	UpdateInterval float64 `json:"updateInterval"`
	Leap           string  `json:"leap"`
}

// serverStatsNames are the `chronyc -c serverstats` columns (chrony 4.8); extra columns of
// newer versions are kept as "field<N>".
var serverStatsNames = []string{
	"ntpPacketsReceived", "ntpPacketsDropped", "commandPacketsReceived", "commandPacketsDropped",
	"clientLogRecordsDropped", "ntsKeConnectionsAccepted", "ntsKeConnectionsDropped",
	"authenticatedNtpPackets", "interleavedNtpPackets", "ntpTimestampsHeld", "ntpTimestampSpan",
	"ntpDaemonRxTimestamps", "ntpDaemonTxTimestamps", "ntpKernelRxTimestamps", "ntpKernelTxTimestamps",
	"ntpHardwareRxTimestamps", "ntpHardwareTxTimestamps",
}

// State is chronyd's actual state.
type State struct {
	Running     bool              `json:"running"`
	Sources     []Source          `json:"sources"`
	SourceStats []SourceStats     `json:"sourceStats"`
	Tracking    *Tracking         `json:"tracking,omitempty"`
	ServerStats map[string]string `json:"serverStats,omitempty"`
}

// State reads chronyd over its command socket; not running → Running false.
func (r *Renderer) State(ctx context.Context) (State, error) {
	st := State{Sources: []Source{}, SourceStats: []SourceStats{}}
	out, err := r.Chronyc(ctx, "-c", "tracking")
	if errors.Is(err, ErrNotRunning) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.Running = true
	if st.Tracking, err = ParseTracking(out); err != nil {
		return st, err
	}
	if out, err = r.Chronyc(ctx, "-c", "sources"); err != nil {
		return st, err
	}
	if st.Sources, err = ParseSources(out); err != nil {
		return st, err
	}
	if out, err = r.Chronyc(ctx, "-c", "sourcestats"); err != nil {
		return st, err
	}
	if st.SourceStats, err = ParseSourceStats(out); err != nil {
		return st, err
	}
	if out, err = r.Chronyc(ctx, "-c", "serverstats"); err != nil {
		return st, err
	}
	st.ServerStats, err = ParseServerStats(out)
	return st, err
}

func records(b []byte) ([][]string, error) {
	cr := csv.NewReader(bytes.NewReader(b))
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("chrony: csv: %w", err)
	}
	return rows, nil
}

type fieldParser struct {
	row []string
	err error
}

func (p *fieldParser) str(i int) string {
	if i >= len(p.row) {
		if p.err == nil {
			p.err = fmt.Errorf("chrony: csv row %q has %d fields, want > %d", strings.Join(p.row, ","), len(p.row), i)
		}
		return ""
	}
	return p.row[i]
}

func (p *fieldParser) int(i int) int {
	v, err := strconv.Atoi(p.str(i))
	if err != nil && p.err == nil {
		p.err = fmt.Errorf("chrony: field %d %q: %w", i, p.str(i), err)
	}
	return v
}

func (p *fieldParser) float(i int) float64 {
	v, err := strconv.ParseFloat(p.str(i), 64)
	if err != nil && p.err == nil {
		p.err = fmt.Errorf("chrony: field %d %q: %w", i, p.str(i), err)
	}
	return v
}

// ParseSources decodes `chronyc -c sources`.
func ParseSources(b []byte) ([]Source, error) {
	rows, err := records(b)
	if err != nil {
		return nil, err
	}
	out := []Source{}
	for _, row := range rows {
		p := &fieldParser{row: row}
		s := Source{Mode: p.str(0), State: p.str(1), Name: p.str(2), Stratum: p.int(3), Poll: p.int(4), Reach: p.str(5),
			LastRx: p.str(6), Offset: p.float(7), Measure: p.float(8), Error: p.float(9)}
		if p.err != nil {
			return nil, p.err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParseSourceStats decodes `chronyc -c sourcestats`.
func ParseSourceStats(b []byte) ([]SourceStats, error) {
	rows, err := records(b)
	if err != nil {
		return nil, err
	}
	out := []SourceStats{}
	for _, row := range rows {
		p := &fieldParser{row: row}
		s := SourceStats{Name: p.str(0), NP: p.int(1), NR: p.int(2), Span: p.int(3), Frequency: p.float(4),
			FreqSkew: p.float(5), Offset: p.float(6), StdDev: p.float(7)}
		if p.err != nil {
			return nil, p.err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParseTracking decodes `chronyc -c tracking`.
func ParseTracking(b []byte) (*Tracking, error) {
	rows, err := records(b)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("chrony: tracking: want one row, got %d", len(rows))
	}
	p := &fieldParser{row: rows[0]}
	t := &Tracking{RefID: p.str(0), RefName: p.str(1), Stratum: p.int(2), RefTime: p.float(3), SystemTime: p.float(4),
		LastOffset: p.float(5), RMSOffset: p.float(6), Frequency: p.float(7), ResidualFreq: p.float(8), Skew: p.float(9),
		RootDelay: p.float(10), RootDispersion: p.float(11), UpdateInterval: p.float(12), Leap: p.str(13)}
	return t, p.err
}

// ParseServerStats decodes `chronyc -c serverstats` into name → value.
func ParseServerStats(b []byte) (map[string]string, error) {
	rows, err := records(b)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("chrony: serverstats: want one row, got %d", len(rows))
	}
	m := map[string]string{}
	for i, v := range rows[0] {
		name := fmt.Sprintf("field%d", i)
		if i < len(serverStatsNames) {
			name = serverStatsNames[i]
		}
		m[name] = v
	}
	return m, nil
}

func toStruct(v any) (*structpb.Struct, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("chrony: state: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("chrony: state: %w", err)
	}
	return structpb.NewStruct(m)
}

// ----- events -----------------------------------------------------------------------------

// DefaultPollInterval is the event polling period (chrony pushes nothing).
const DefaultPollInterval = time.Second

// Event is one observed change.
type Event struct{ Key, Old, New string }

func (e Event) String() string { return fmt.Sprintf("chrony %s: %q -> %q", e.Key, e.Old, e.New) }

// ToProto maps the event to the agent's Event message (details in attributes).
func (e Event) ToProto() *vrxv1.Event {
	return &vrxv1.Event{
		Kind:       vrxv1.EventKind_EVENT_KIND_UNSPECIFIED,
		Message:    e.String(),
		Attributes: map[string]string{"source": "chrony", "key": e.Key, "old": e.Old, "new": e.New},
	}
}

// Poller samples tracking (stratum, leap, reference) and the source states.
type Poller struct {
	r    *Renderer
	last map[string]string
}

// NewPoller returns a poller over r.
func (r *Renderer) NewPoller() *Poller { return &Poller{r: r} }

// Poll takes one sample and returns the changes since the previous one.
func (p *Poller) Poll(ctx context.Context) ([]Event, error) {
	cur := map[string]string{"running": "false"}
	out, err := p.r.Chronyc(ctx, "-c", "tracking")
	switch {
	case errors.Is(err, ErrNotRunning):
	case err != nil:
		return nil, err
	default:
		cur["running"] = "true"
		t, err := ParseTracking(out)
		if err != nil {
			return nil, err
		}
		cur["stratum"], cur["leap"], cur["reference"] = strconv.Itoa(t.Stratum), t.Leap, t.RefName
		out, err = p.r.Chronyc(ctx, "-c", "sources")
		if err != nil {
			return nil, err
		}
		srcs, err := ParseSources(out)
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			cur["source/"+s.Name] = s.State
		}
	}
	keys := make([]string, 0, len(cur))
	for k := range cur {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var evs []Event
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
