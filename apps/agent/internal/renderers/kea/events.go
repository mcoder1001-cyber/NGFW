package kea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Kea pushes no events to a client: the poller samples statistic-get-all at 1 Hz and emits
// one Event per watched statistic that changed (packets received, per-subnet pool usage)
// plus a "running" event when a server starts or stops answering.

// DefaultPollInterval is the event polling period.
const DefaultPollInterval = time.Second

// Event is one observed change.
type Event struct {
	Daemon   string // "kea-dhcp4" / "kea-dhcp6"
	Key      string // statistic name or "running"
	Old, New string // "" = absent
}

func (e Event) String() string {
	return fmt.Sprintf("%s %s: %q -> %q", e.Daemon, e.Key, e.Old, e.New)
}

// ToProto maps the event to the agent's Event message (no DHCP-specific EventKind exists:
// details go into attributes, same convention as the FRR renderer).
func (e Event) ToProto() *vrxv1.Event {
	return &vrxv1.Event{
		Kind:    vrxv1.EventKind_EVENT_KIND_UNSPECIFIED,
		Message: e.String(),
		Attributes: map[string]string{
			"source": "kea", "daemon": e.Daemon, "key": e.Key, "old": e.Old, "new": e.New,
		},
	}
}

// watched reports whether a statistic is worth an event.
func watched(name string) bool {
	switch {
	case name == "pkt4-received", name == "pkt6-received", name == "pkt4-ack-sent", name == "pkt6-reply-sent":
		return true
	case strings.HasPrefix(name, "subnet[") &&
		(strings.HasSuffix(name, "].assigned-addresses") || strings.HasSuffix(name, "].total-addresses") ||
			strings.HasSuffix(name, "].assigned-nas") || strings.HasSuffix(name, "].total-nas")):
		return true
	}
	return false
}

// Poller samples both servers and diffs consecutive samples.
type Poller struct {
	r    *Renderer
	last map[string]map[string]string // daemon → key → value
}

// NewPoller returns a poller over r's control channel.
func (r *Renderer) NewPoller() *Poller { return &Poller{r: r, last: map[string]map[string]string{}} }

// Poll takes one sample and returns the changes since the previous one (the first sample
// reports every watched value as new).
func (p *Poller) Poll(ctx context.Context) ([]Event, error) {
	var events []Event
	for _, fam := range []int{4, 6} {
		daemon := fmt.Sprintf("kea-dhcp%d", fam)
		cur := map[string]string{"running": "false"}
		stats, err := p.r.Statistics(ctx, fam)
		switch {
		case errors.Is(err, ErrNotRunning):
		case err != nil:
			return nil, err
		default:
			cur["running"] = "true"
			for k, v := range stats {
				if watched(k) {
					cur[k] = v
				}
			}
		}
		events = append(events, diff(daemon, p.last[daemon], cur)...)
		p.last[daemon] = cur
	}
	return events, nil
}

// Run polls every interval until ctx ends, sending events to emit. Poll errors are reported
// once per distinct message as an "error" event and polling continues.
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
			emit(Event{Daemon: "kea", Key: "error", Old: lastErr, New: err.Error()})
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

func diff(daemon string, old, cur map[string]string) []Event {
	var out []Event
	for _, k := range sortedKeys(cur) {
		if old[k] != cur[k] {
			out = append(out, Event{Daemon: daemon, Key: k, Old: old[k], New: cur[k]})
		}
	}
	for _, k := range sortedKeys(old) {
		if _, ok := cur[k]; !ok {
			out = append(out, Event{Daemon: daemon, Key: k, Old: old[k]})
		}
	}
	return out
}
