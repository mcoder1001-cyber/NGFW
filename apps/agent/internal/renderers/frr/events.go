package frr

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// FRR has no push channel the agent can subscribe to without linking FRR code (the
// northbound gRPC module is not built in the Ubuntu package: --disable-grpc), so events come
// from polling at 1 Hz: every poller turns a show command into a flat key → value snapshot,
// and a change of a value (or a key appearing/disappearing) is one Event.

// DefaultPollInterval is the event polling period.
const DefaultPollInterval = time.Second

// ShowFunc runs a constant JSON show command (Renderer.ShowJSON).
type ShowFunc func(ctx context.Context, cmd ShowCommand) (json.RawMessage, error)

// PollFunc produces one snapshot: key → value. Protocol pollers (P12: neighbour state per
// peer) register one with RegisterPoller.
type PollFunc func(ctx context.Context, show ShowFunc) (map[string]string, error)

// Event is one observed change.
type Event struct {
	// Poller is the poller's name ("routes", "interfaces", "bgp-neighbors" …).
	Poller string
	// Key identifies the object within the poller ("ipv4/default", "w12f0", a peer address).
	Key string
	// Old and New are the values before and after ("" = absent).
	Old, New string
}

// String renders the event as "<poller> <key>: <old> -> <new>" ("-" for absent).
func (e Event) String() string {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return fmt.Sprintf("%s %s: %s -> %s", e.Poller, e.Key, dash(e.Old), dash(e.New))
}

// ToProto maps the event to the agent's Event message: interface link changes become
// LINK_UP / LINK_DOWN; everything else is EVENT_KIND_UNSPECIFIED with the details in
// attributes (no FRR-specific EventKind exists yet: RF-1-questions.md Q3).
func (e Event) ToProto() *vrxv1.Event {
	ev := &vrxv1.Event{
		Kind:    vrxv1.EventKind_EVENT_KIND_UNSPECIFIED,
		Message: fmt.Sprintf("frr %s %s: %q -> %q", e.Poller, e.Key, e.Old, e.New),
		Attributes: map[string]string{
			"source": "frr", "poller": e.Poller, "key": e.Key, "old": e.Old, "new": e.New,
		},
	}
	if e.Poller == PollerInterfaces {
		switch e.New {
		case "up":
			ev.Kind = vrxv1.EventKind_EVENT_KIND_LINK_UP
		case "down":
			ev.Kind = vrxv1.EventKind_EVENT_KIND_LINK_DOWN
		}
		if ev.Kind != vrxv1.EventKind_EVENT_KIND_UNSPECIFIED {
			name := e.Key
			ev.Interface = &name
		}
	}
	return ev
}

// Framework poller names.
const (
	PollerRoutes     = "routes"
	PollerInterfaces = "interfaces"
)

var (
	pollersMu sync.Mutex
	pollers   = map[string]PollFunc{}
)

// RegisterPoller adds a protocol poller used by every Poller created from a Renderer. It
// panics on a nil function or an invalid/duplicate name (init()-time programming errors).
func RegisterPoller(name string, fn PollFunc) {
	if fn == nil || !sectionNameRe.MatchString(name) {
		panic(fmt.Sprintf("frr: RegisterPoller(%q): invalid name or nil function", name))
	}
	pollersMu.Lock()
	defer pollersMu.Unlock()
	if _, dup := pollers[name]; dup || name == PollerRoutes || name == PollerInterfaces {
		panic(fmt.Sprintf("frr: poller %q registered twice", name))
	}
	pollers[name] = fn
}

// pollRoutes reports RIB counts per family, VRF and protocol ("ipv4/default/static" → "3")
// from the summary commands — O(VRFs × protocols), independent of the table size (M3).
func pollRoutes(ctx context.Context, show ShowFunc) (map[string]string, error) {
	sum, err := ribSummary(ctx, show)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(sum))
	for k, n := range sum {
		out[k] = strconv.Itoa(n)
	}
	return out, nil
}

// pollInterfaces reports each interface's operational state ("up"/"down"; the
// administrative state when FRR reports no operational one).
func pollInterfaces(ctx context.Context, show ShowFunc) (map[string]string, error) {
	raw, err := show(ctx, ShowInterfaceAll)
	if err != nil {
		return nil, err
	}
	ifs, err := decodeInterfaces(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, st := range ifs {
		// FRR omits operationalStatus for an administratively down interface.
		status := st.OperationalStatus
		if status == "" {
			status = st.AdministrativeStatus
		}
		out[name] = strings.ToLower(status)
	}
	return out, nil
}

// interfaceState is the subset of `show interface json` the poller reads.
type interfaceState struct {
	OperationalStatus    string `json:"operationalStatus"`
	AdministrativeStatus string `json:"administrativeStatus"`
	Description          string `json:"description"`
	VRFName              string `json:"vrfName"`
}

// decodeInterfaces accepts `show interface json` (name → state) and `… vrf all json`
// (vrf → name → state).
func decodeInterfaces(raw json.RawMessage) (map[string]interfaceState, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("frr: decode interfaces: %w", err)
	}
	out := map[string]interfaceState{}
	for k, v := range top {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(v, &probe); err != nil {
			return nil, fmt.Errorf("frr: decode interface %q: %w", k, err)
		}
		_, hasOper := probe["operationalStatus"]
		_, hasAdmin := probe["administrativeStatus"]
		if hasOper || hasAdmin {
			var st interfaceState
			if err := json.Unmarshal(v, &st); err != nil {
				return nil, err
			}
			out[k] = st
			continue
		}
		for name, iv := range probe { // k is a VRF name
			var st interfaceState
			if err := json.Unmarshal(iv, &st); err != nil {
				return nil, fmt.Errorf("frr: decode interface %q: %w", name, err)
			}
			out[name] = st
		}
	}
	return out, nil
}

// Poller turns successive snapshots into Events. It is not safe for concurrent Step calls.
type Poller struct {
	show  ShowFunc
	funcs map[string]PollFunc
	last  map[string]map[string]string
}

// NewPoller returns a poller with the framework pollers plus the registered ones.
func (r *Renderer) NewPoller() *Poller {
	return newPoller(r.ShowJSON)
}

func newPoller(show ShowFunc) *Poller {
	fs := map[string]PollFunc{PollerRoutes: pollRoutes, PollerInterfaces: pollInterfaces}
	pollersMu.Lock()
	maps.Copy(fs, pollers)
	pollersMu.Unlock()
	return &Poller{show: show, funcs: fs, last: map[string]map[string]string{}}
}

// Step takes one snapshot per poller and returns the changes since the previous Step, sorted
// by (poller, key). The first successful snapshot of a poller is the baseline (no events). A
// failing poller keeps its previous baseline; the errors are joined into the return value.
func (p *Poller) Step(ctx context.Context) ([]Event, error) {
	var events []Event
	var errs []string
	for _, name := range slices.Sorted(maps.Keys(p.funcs)) {
		snap, err := p.funcs[name](ctx, p.show)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		prev, seen := p.last[name]
		p.last[name] = snap
		if !seen {
			continue
		}
		keys := map[string]bool{}
		for k := range prev {
			keys[k] = true
		}
		for k := range snap {
			keys[k] = true
		}
		for _, k := range slices.Sorted(maps.Keys(keys)) {
			if prev[k] != snap[k] {
				events = append(events, Event{Poller: name, Key: k, Old: prev[k], New: snap[k]})
			}
		}
	}
	if len(errs) > 0 {
		return events, fmt.Errorf("%w: poll: %s", ErrDaemon, strings.Join(errs, "; "))
	}
	return events, nil
}

// Watch polls every interval until ctx is done, calling emit for each Event and onErr (may
// be nil) for each failed poll. It returns ctx.Err().
func (p *Poller) Watch(ctx context.Context, interval time.Duration, emit func(Event), onErr func(error)) error {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		events, err := p.Step(ctx)
		for _, e := range events {
			emit(e)
		}
		if err != nil && onErr != nil && ctx.Err() == nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
