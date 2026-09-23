package rfkit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// DefaultPollInterval is the event polling period for daemons without a push channel.
const DefaultPollInterval = time.Second

// Event is one observed change of a daemon's state.
type Event struct {
	// Source is the renderer name ("snmpd", "keepalived", "rsyslog").
	Source string
	// Key identifies the object ("vi1", "reachable", "vrx_export_0/failed").
	Key string
	// Old and New are the values before and after ("" = absent).
	Old, New string
	// Error is set (and Key/Old/New empty) when a poll failed; it is already redacted.
	Error string
}

// String renders the event as "<source> <key>: <old> -> <new>".
func (e Event) String() string {
	if e.Error != "" {
		return fmt.Sprintf("%s poll error: %s", e.Source, e.Error)
	}
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return fmt.Sprintf("%s %s: %s -> %s", e.Source, e.Key, dash(e.Old), dash(e.New))
}

// ToProto maps the event to the agent's Event message: poll errors are EVENT_KIND_ERROR,
// changes EVENT_KIND_UNSPECIFIED with the details in attributes (no daemon-specific kind
// exists yet; the same choice as the FRR renderer).
func (e Event) ToProto() *vrxv1.Event {
	if e.Error != "" {
		return &vrxv1.Event{
			Kind:       vrxv1.EventKind_EVENT_KIND_ERROR,
			Message:    e.String(),
			Attributes: map[string]string{"source": e.Source},
		}
	}
	return &vrxv1.Event{
		Kind:       vrxv1.EventKind_EVENT_KIND_UNSPECIFIED,
		Message:    e.String(),
		Attributes: map[string]string{"source": e.Source, "key": e.Key, "old": e.Old, "new": e.New},
	}
}

// SnapshotFunc returns the current flat state (key → value) of a daemon.
type SnapshotFunc func(ctx context.Context) (map[string]string, error)

// Poller turns successive snapshots into change events.
type Poller struct {
	Source string
	Snap   SnapshotFunc
	// Redact masks secrets in error texts (may be nil).
	Redact func(string) string

	prev    map[string]string
	started bool
	lastErr string
}

// Step takes one snapshot and returns the changes since the previous one. The first
// successful snapshot is the baseline and yields no events. A failing snapshot yields one
// error event (repeated failures with the same text are reported once).
func (p *Poller) Step(ctx context.Context) []Event {
	cur, err := p.Snap(ctx)
	if err != nil {
		msg := err.Error()
		if p.Redact != nil {
			msg = p.Redact(msg)
		}
		if msg == p.lastErr {
			return nil
		}
		p.lastErr = msg
		return []Event{{Source: p.Source, Error: msg}}
	}
	p.lastErr = ""
	if !p.started {
		p.prev, p.started = cur, true
		return nil
	}
	var out []Event
	keys := slices.Sorted(maps.Keys(cur))
	for _, k := range keys {
		if old := p.prev[k]; old != cur[k] {
			out = append(out, Event{Source: p.Source, Key: k, Old: old, New: cur[k]})
		}
	}
	for _, k := range slices.Sorted(maps.Keys(p.prev)) {
		if _, ok := cur[k]; !ok {
			out = append(out, Event{Source: p.Source, Key: k, Old: p.prev[k]})
		}
	}
	p.prev = cur
	return out
}

// Run polls every interval (DefaultPollInterval when zero) and sends events on ch until ctx
// ends. It never blocks longer than ctx on a full channel.
func (p *Poller) Run(ctx context.Context, interval time.Duration, ch chan<- Event) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		for _, ev := range p.Step(ctx) {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
