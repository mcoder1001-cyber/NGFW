package strongswan

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/strongswan/govici/vici"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Events. charon pushes ike-updown, child-updown, ike-rekey and child-rekey over VICI
// (event registration); Watch subscribes and turns each into an Event. When the event session
// breaks (charon restarted or stopped) Watch reports it, polls list-sas at 1 Hz and turns
// state changes into events until it can subscribe again (reconnect with exponential backoff,
// 1 s … 30 s). The event payloads carry identities, addresses and SPIs, never keys; every
// attribute is redacted and clipped.

// EventNames are the VICI events Watch subscribes to.
var EventNames = []string{"ike-updown", "child-updown", "ike-rekey", "child-rekey"}

// Event kinds besides the VICI event names.
const (
	// KindDaemon: the event channel to charon went down or came back (State "down"/"up").
	KindDaemon = "daemon"
	// KindPoll: a state change seen by polling (1 Hz while the event channel is down, and the
	// periodic / overflow resync while subscribed).
	KindPoll = "poll"
	// KindResync: the event buffer overflowed; a resync (KindPoll events) follows.
	KindResync = "resync"
)

// Event is one observed change.
type Event struct {
	Time time.Time
	// Kind is a VICI event name, KindDaemon or KindPoll.
	Kind string
	// Conn is the connection (IKE_SA config) name; Child the CHILD_SA name, if any.
	Conn, Child string
	// Up is set for up events (updown kinds; daemon "up").
	Up bool
	// State is the SA state after the event (ESTABLISHED, INSTALLED, DELETING, "absent", …).
	State string
	// Attrs are small, flat details (unique ids, hosts, SPIs).
	Attrs map[string]string
}

// String renders the event for logs.
func (e Event) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "strongswan %s", e.Kind)
	if e.Conn != "" {
		fmt.Fprintf(&b, " %s", e.Conn)
	}
	if e.Child != "" {
		fmt.Fprintf(&b, "/%s", e.Child)
	}
	if e.State != "" {
		fmt.Fprintf(&b, ": %s", e.State)
	}
	return b.String()
}

// ToProto maps the event to the agent's Event message. There is no IPsec EventKind yet
// (same gap as RF-1 Q3): SA events are EVENT_KIND_UNSPECIFIED with the details in attributes;
// a lost daemon channel is EVENT_KIND_ERROR.
func (e Event) ToProto() *vrxv1.Event {
	attrs := map[string]string{"source": "strongswan", "event": e.Kind}
	maps.Copy(attrs, e.Attrs)
	if e.Conn != "" {
		attrs["conn"], attrs["tunnel"] = e.Conn, TunnelName(e.Conn)
	}
	if e.Child != "" {
		attrs["child"] = e.Child
	}
	if e.State != "" {
		attrs["state"] = e.State
	}
	attrs["up"] = yesNo(e.Up)
	kind := vrxv1.EventKind_EVENT_KIND_UNSPECIFIED
	if e.Kind == KindDaemon && !e.Up {
		kind = vrxv1.EventKind_EVENT_KIND_ERROR
	}
	return &vrxv1.Event{Kind: kind, Message: e.String(), Attributes: attrs}
}

// event attributes copied from VICI SA sections.
var saAttrs = []string{"uniqueid", "state", "local-host", "remote-host", "local-id", "remote-id", "version"}
var childAttrs = []string{"uniqueid", "state", "spi-in", "spi-out", "mode", "protocol"}

const maxAttr = 256

func (r *Renderer) clean(s string) string {
	if len(s) > maxAttr {
		s = s[:maxAttr]
	}
	return s
}

// fromVICI converts one VICI event (possibly several SAs) into Events.
func (r *Renderer) fromVICI(ev vici.Event) []Event {
	up := str(ev.Message, "up") == "yes"
	var out []Event
	for _, k := range ev.Message.Keys() {
		sa := sub(ev.Message, k)
		if sa == nil {
			continue
		}
		if _, err := SectionName(k); err != nil {
			continue // not a connection name the renderer could have loaded
		}
		base := Event{Time: ev.Timestamp, Kind: ev.Name, Conn: k, Up: up, Attrs: map[string]string{}}
		if ev.Name == "ike-rekey" {
			sa = sub(sa, "new")
		}
		for _, a := range saAttrs {
			if v := str(sa, a); v != "" {
				base.Attrs["ike-"+a] = r.clean(v)
			}
		}
		base.State = r.clean(str(sa, "state"))
		children := sub(sa, "child-sas")
		if strings.HasPrefix(ev.Name, "child-") && children != nil {
			for _, ck := range children.Keys() {
				c := sub(children, ck)
				if ev.Name == "child-rekey" {
					c = sub(c, "new")
				}
				e := base
				e.Attrs = maps.Clone(base.Attrs)
				e.Child = r.clean(str(c, "name"))
				for _, a := range childAttrs {
					if v := str(c, a); v != "" {
						e.Attrs["child-"+a] = r.clean(v)
					}
				}
				e.State = r.clean(str(c, "state"))
				if !up && ev.Name == "child-updown" && e.State == "" {
					e.State = "absent"
				}
				out = append(out, e)
			}
			continue
		}
		out = append(out, base)
	}
	return out
}

// Backoff bounds for re-subscribing.
const (
	minBackoff   = time.Second
	maxBackoff   = 30 * time.Second
	pollInterval = time.Second
)

// Watch streams events into out until ctx is done (then it returns ctx.Err()). It never
// closes out. Sends are non-blocking-with-context: a full channel blocks Watch, not charon.
func (r *Renderer) Watch(ctx context.Context, out chan<- Event) error {
	backoff := minBackoff
	var snapshot map[string]string // "conn#ikeid[/child#childid]" → state
	downReported, wasDown := false, false
	send := func(e Event) bool {
		select {
		case out <- e:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		evs, closeFn, err := r.subscribe(ctx)
		if err == nil {
			downReported, backoff = false, minBackoff
			snapshot = r.pollOnce(ctx, nil, send) // baseline for the resyncs below
			if up := r.upEvent(ctx, wasDown); up != nil && !send(*up) {
				closeFn()
				return ctx.Err()
			}
			wasDown = false
			if !r.watchSubscribed(ctx, evs, &snapshot, send) {
				closeFn()
				return ctx.Err()
			}
			closeFn()
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		if !downReported {
			msg := "event channel closed"
			if err != nil {
				msg = r.clean(err.Error())
			}
			if !send(Event{Time: r.now(), Kind: KindDaemon, State: "down", Attrs: map[string]string{"reason": msg}}) {
				return ctx.Err()
			}
			downReported, wasDown = true, true
		}
		// Poll at 1 Hz until the next subscription attempt is due.
		deadline := r.now().Add(backoff)
		for r.now().Before(deadline) {
			snapshot = r.pollOnce(ctx, snapshot, send)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// watchSubscribed forwards VICI events until the channel closes. Because govici drops events
// silently when the channel is full (RF-2 review M4), a full channel triggers an immediate
// resync (a KindResync event, then list-sas diffed against the last snapshot as KindPoll
// events), and the same resync runs every resyncInterval anyway, so a lost event is repaired
// within that interval at the latest. It returns false when ctx ended.
func (r *Renderer) watchSubscribed(ctx context.Context, evs <-chan vici.Event, snapshot *map[string]string, send func(Event) bool) bool {
	ticker := time.NewTicker(r.resyncEvery())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case ev, ok := <-evs:
			if !ok {
				return true
			}
			overflow := len(evs) >= cap(evs)-1
			for _, e := range r.fromVICI(ev) {
				if !send(e) {
					return false
				}
			}
			if overflow {
				if !send(Event{Time: r.now(), Kind: KindResync, Attrs: map[string]string{"reason": "event buffer full: events may have been dropped"}}) {
					return false
				}
				*snapshot = r.pollOnce(ctx, *snapshot, send)
			}
		case <-ticker.C:
			*snapshot = r.pollOnce(ctx, *snapshot, send)
		}
	}
}

// DefaultResyncInterval is how often Watch re-lists SAs while subscribed.
const DefaultResyncInterval = 30 * time.Second

// WithResyncInterval overrides DefaultResyncInterval (tests).
func WithResyncInterval(d time.Duration) Option { return func(r *Renderer) { r.resync = d } }

func (r *Renderer) resyncEvery() time.Duration {
	if r.resync > 0 {
		return r.resync
	}
	return DefaultResyncInterval
}

// upEvent is sent after a (re-)subscription: "restarted" when charon's start time differs
// from the acknowledged one (review M3; attrs started/acked), "up" after a reported outage,
// nothing otherwise.
func (r *Renderer) upEvent(ctx context.Context, wasDown bool) *Event {
	since := ""
	if s, err := r.open(ctx); err == nil {
		if st, err := s.call(ctx, "stats", nil); err == nil {
			since = str(sub(st, "uptime"), "since")
		}
		s.Close()
	}
	acked, restarted := r.restartState(since)
	if restarted {
		return &Event{Time: r.now(), Kind: KindDaemon, Up: true, State: "restarted",
			Attrs: map[string]string{"restarted": "yes", "started": r.clean(since), "acked": r.clean(acked)}}
	}
	if wasDown {
		return &Event{Time: r.now(), Kind: KindDaemon, Up: true, State: "up"}
	}
	return nil
}

// subscribe opens an event session; the returned channel is closed when charon goes away.
func (r *Renderer) subscribe(ctx context.Context) (<-chan vici.Event, func(), error) {
	c, err := r.dial(ctx, r.paths.ViciSocket)
	if err != nil {
		return nil, nil, err
	}
	if err := c.Subscribe(EventNames...); err != nil {
		_ = c.Close()
		return nil, nil, fmt.Errorf("%w: subscribe: %v", ErrDaemon, err)
	}
	ch := make(chan vici.Event, 256)
	c.NotifyEvents(ch)
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-stop:
		}
	}()
	return ch, func() { close(stop); _ = c.Close() }, nil
}

// pollOnce lists the SAs and emits a KindPoll event for every key whose state changed.
func (r *Renderer) pollOnce(ctx context.Context, prev map[string]string, send func(Event) bool) map[string]string {
	st, err := r.State(ctx)
	if err != nil {
		return prev // charon down as well: nothing to compare
	}
	cur := map[string]string{}
	for _, sa := range st.SAs {
		cur[sa.Name+"#"+sa.UniqueID] = sa.State
		for _, c := range sa.Children {
			cur[sa.Name+"#"+sa.UniqueID+"/"+c.Name+"#"+c.UniqueID] = c.State
		}
	}
	if prev != nil {
		keys := slices.Sorted(maps.Keys(cur))
		for k := range prev {
			if _, ok := cur[k]; !ok {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			if prev[k] == cur[k] {
				continue
			}
			ike, child, _ := strings.Cut(k, "/")
			conn, ikeID, _ := strings.Cut(ike, "#")
			e := Event{Time: r.now(), Kind: KindPoll, Conn: conn, State: cur[k], Up: cur[k] != "",
				Attrs: map[string]string{"ike-uniqueid": ikeID, "previous": prev[k]}}
			if e.State == "" {
				e.State = "absent"
			}
			if child != "" {
				name, id, _ := strings.Cut(child, "#")
				e.Child, e.Attrs["child-uniqueid"] = name, id
			}
			if !send(e) {
				return cur
			}
		}
	}
	return cur
}
