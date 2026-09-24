package subsystems

// Shared seams of the wave-A/B/C features (W-seed launch plan §5.2; wired by TD-8, D-119 M2): the
// agent's event sink and resync hook (A5), the agent's VPP id range (fail closed), the dynamic desired
// sources (S1) and the feature metrics collectors. Each is inert until a feature uses it.
//
// The agent stays declarative through all of them: a feature publishes events and hands the agent
// desired state (KVs of registered descriptors); only the scheduler, under the agent's transaction
// lock, ever writes VPP.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// ---- VPP id range (fail closed) ---------------------------------------------------------------

// EnvTableBase is the environment variable of a test slot's first VRF/table id
// (docs/lab/shared-host-rules.md §1: slot N owns N000–N999; §11: reserved ranges).
const EnvTableBase = "VRX_VPP_TABLE_BASE"

// EnvIDRange is the explicit opt-in to own every id: VRX_VPP_ID_RANGE=all (the product agent on a box
// of its own). No other value is accepted; a numbered range is VRX_VPP_TABLE_BASE.
const EnvIDRange = "VRX_VPP_ID_RANGE"

// IDRangeAll is the only accepted value of VRX_VPP_ID_RANGE.
const IDRangeAll = "all"

// SlotIDRangeSize is the number of table/numeric ids a slot owns.
const SlotIDRangeSize = 1000

// ErrNoIDRange means that neither VRX_VPP_TABLE_BASE nor VRX_VPP_ID_RANGE=all is set: the agent owns
// no numeric id (fail closed), never "every id" by default.
var ErrNoIDRange = errors.New("no VPP id range: set " + EnvTableBase + " (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §11) or " + EnvIDRange + "=" + IDRangeAll + " (the product agent on a box of its own)")

// IDRange is a closed range of numeric ids (FIB tables, SPD/SA ids, policy ids, map ids). Convert it
// to the family's own range type (df2.IDRange, df7.IDRange, vpn.IDRange) when registering.
type IDRange struct{ Lo, Hi uint32 }

// IDScope is the agent's resolved id range, handed to the wiring through Env.IDs and read by the
// families through Wiring.IDRange. The zero value owns no id (fail closed).
type IDScope struct {
	// Range is the ids this agent may allocate (VRX_VPP_TABLE_BASE: base..base+999).
	Range *IDRange
	// All is VRX_VPP_ID_RANGE=all: every id (what a nil df2/df7 range and the zero vpn.IDRange mean).
	All bool
}

// String describes the scope (start-up log).
func (s IDScope) String() string {
	switch {
	case s.Range != nil:
		return fmt.Sprintf("%d-%d", s.Range.Lo, s.Range.Hi)
	case s.All:
		return IDRangeAll
	}
	return "none (fail closed)"
}

// ResolveIDScope reads the agent's id range from the environment and fails closed:
//
//	VRX_VPP_TABLE_BASE=<base>   → base..base+999 (a test slot, or a reserved range on a shared host)
//	VRX_VPP_ID_RANGE=all        → every id (the product agent on a box of its own)
//	neither                     → the zero IDScope (no id) and ErrNoIDRange
//	both, a malformed base, or another VRX_VPP_ID_RANGE value → an error (never "every id")
func ResolveIDScope() (IDScope, error) {
	base, hasBase := os.LookupEnv(EnvTableBase)
	all, hasAll := os.LookupEnv(EnvIDRange)
	hasBase, hasAll = hasBase && base != "", hasAll && all != ""
	if hasAll && all != IDRangeAll {
		return IDScope{}, fmt.Errorf("%s=%q: the only accepted value is %q (a numbered range is %s)", EnvIDRange, all, IDRangeAll, EnvTableBase)
	}
	switch {
	case hasBase && hasAll:
		return IDScope{}, fmt.Errorf("%s and %s=%s are both set: set exactly one", EnvTableBase, EnvIDRange, IDRangeAll)
	case hasAll:
		return IDScope{All: true}, nil
	case !hasBase:
		return IDScope{}, ErrNoIDRange
	}
	n, err := strconv.ParseUint(base, 10, 32)
	if err != nil {
		return IDScope{}, fmt.Errorf("%s=%q is not a table id: %w", EnvTableBase, base, err)
	}
	if n == 0 || n > uint64(^uint32(0))-(SlotIDRangeSize-1) {
		return IDScope{}, fmt.Errorf("%s=%q is outside 1–%d (table 0 is VPP's default table; the range must fit in 32 bits)", EnvTableBase, base, uint64(^uint32(0))-(SlotIDRangeSize-1))
	}
	return IDScope{Range: &IDRange{Lo: uint32(n), Hi: uint32(n) + SlotIDRangeSize - 1}}, nil
}

// SlotIDRange is ResolveIDScope as a range: base..base+999, nil only with VRX_VPP_ID_RANGE=all
// (every id), ErrNoIDRange when neither variable is set. Families read the range through
// Wiring.IDRange (Env), never from the environment; this is for tools and tests.
func SlotIDRange() (*IDRange, error) {
	s, err := ResolveIDScope()
	if err != nil {
		return nil, err
	}
	return s.Range, nil
}

// IDRange returns the id range of this agent (Env.IDs): the slot's or reserved range, nil = every id
// (only with VRX_VPP_ID_RANGE=all), or ErrNoIDRange (fail closed). A family that allocates numeric ids
// calls it on its Register line and fails the registration on error:
//
//	ids, err := w.IDRange()
//	if err != nil {
//		return nil, fmt.Errorf("<family>: %w", err)
//	}
func (w *Wiring) IDRange() (*IDRange, error) {
	switch {
	case w.env.IDs.Range != nil:
		r := *w.env.IDs.Range
		return &r, nil
	case w.env.IDs.All:
		return nil, nil
	}
	return nil, ErrNoIDRange
}

// ---- events and resync (A5) -------------------------------------------------------------------

// Publish hands ev to the agent's event bus (Env.Publish): every StreamEvents subscriber gets a copy
// (the caller may reuse ev), numbered by its stream, with ts set by the bus when unset. The agent drops
// an EVENT_KIND_UNSPECIFIED event. Without a sink, and for a nil event, it does nothing.
func (w *Wiring) Publish(ev *vrxv1.Event) {
	if ev != nil && w.env.Publish != nil {
		w.env.Publish(ev)
	}
}

// RequestResync asks the agent for a full resync of its stored desired state (Env.Resync). It never
// blocks (safe inside a descriptor call, under the transaction lock): requests coalesce, and one made
// while VPP is disconnected is dropped because the reconnect resyncs anyway. Without a hook it does
// nothing.
func (w *Wiring) RequestResync() {
	if w.env.Resync != nil {
		w.env.Resync()
	}
}

// ---- dynamic desired sources (S1) ------------------------------------------------------------

// SyncFunc runs one transaction for a dynamic source: its current Desired, scoped to its descriptors,
// under the agent's transaction lock. It returns nil when the transaction ended APPLIED, and an error
// when VPP is disconnected (the reconnect resync includes the source), ctx is done, or the
// transaction failed or rolled back.
type SyncFunc func(ctx context.Context) error

// DynamicSource is a feature-owned source of desired state the configuration document does not
// carry (seam S1, wave-BC-numbers.md): F-mpls-ldp's FRR→VPP label sync, F-igmp-mfib's PIM→mFIB sync.
//
// The agent merges Desired into the projection of every transaction under its transaction lock —
// Apply, resync (start, VPP reconnect, Env.Resync), confirm revert, DryRun — with Descriptors in the
// transaction's scope, and the source's own SyncFunc runs a transaction scoped to Descriptors only. So
// an object the source stops producing is deleted, a VPP restart is repaired by the reconnect resync,
// a config change that removes what a dynamic object depends on deletes both in one transaction, and
// nothing but the scheduler writes VPP.
type DynamicSource struct {
	// Name identifies the source in logs and events: the feature slug ("mpls-ldp").
	Name string
	// Descriptors are the registered descriptors this source owns exclusively: descriptor instances
	// that belong to no configuration domain (e.g. "mpls-route.ldp"). The configuration never
	// produces their keys; every key Desired returns must belong to one of them.
	Descriptors []string
	// Desired returns the source's objects for doc, the stored configuration document as it will be
	// after the transaction (read-only). It runs under the transaction lock, so it must be fast and
	// do no VPP or daemon I/O: it combines the state the Run loop cached (FRR's labels, PIM's
	// routes) with doc, and leaves out objects whose configuration dependencies doc no longer has.
	// It must be safe to call concurrently with Run.
	Desired func(doc *vrxv1.DesiredState) []scheduler.KV
	// Run is the feature's loop (poll or subscribe to the daemon), optional. The agent starts it once,
	// after its first resync, and cancels ctx when it stops; Run must return then. It calls sync after
	// its cached state changed.
	Run func(ctx context.Context, sync SyncFunc)
}

// seamName is the shape of a dynamic source or metrics collector name.
var seamName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)

// seamRegistry holds what the features register through the seams (Wiring.seams).
type seamRegistry struct {
	mu         sync.Mutex
	sources    []DynamicSource
	collectors []MetricsCollector
}

// AddDynamicSource registers a dynamic desired source; call it on the feature's line in Register. It
// refuses a malformed or duplicate name, a missing Desired, no descriptors, and a descriptor that
// belongs to a configuration domain or to another source (the agent also refuses an unregistered one).
func (w *Wiring) AddDynamicSource(src DynamicSource) error {
	if !seamName.MatchString(src.Name) {
		return fmt.Errorf("dynamic source %q: the name must match %s", src.Name, seamName)
	}
	if src.Desired == nil || len(src.Descriptors) == 0 {
		return fmt.Errorf("dynamic source %s: Desired and at least one descriptor are required", src.Name)
	}
	w.seams.mu.Lock()
	defer w.seams.mu.Unlock()
	owner := map[string]string{}
	for _, s := range w.seams.sources {
		if s.Name == src.Name {
			return fmt.Errorf("dynamic source %s: registered twice", src.Name)
		}
		for _, d := range s.Descriptors {
			owner[d] = s.Name
		}
	}
	seen := map[string]bool{}
	for _, d := range src.Descriptors {
		switch {
		case !scheduler.ValidName(d):
			return fmt.Errorf("dynamic source %s: %q is not a descriptor name", src.Name, d)
		case seen[d]:
			return fmt.Errorf("dynamic source %s: descriptor %s is listed twice", src.Name, d)
		case DomainOf(d) != "":
			return fmt.Errorf("dynamic source %s: descriptor %s belongs to the %s domain (a source owns descriptor instances of no domain)", src.Name, d, DomainOf(d))
		case owner[d] != "":
			return fmt.Errorf("dynamic source %s: descriptor %s is owned by dynamic source %s", src.Name, d, owner[d])
		}
		seen[d] = true
	}
	src.Descriptors = append([]string(nil), src.Descriptors...)
	w.seams.sources = append(w.seams.sources, src)
	return nil
}

// DynamicSources returns the registered dynamic sources in registration order.
func (w *Wiring) DynamicSources() []DynamicSource {
	w.seams.mu.Lock()
	defer w.seams.mu.Unlock()
	return append([]DynamicSource(nil), w.seams.sources...)
}

// ---- feature metrics ------------------------------------------------------------------------

// MetricsCollector adds a feature's metric families to the agent's Prometheus endpoint (P05
// /metrics; F-dashboard-prom-alarms and any feature with counters of its own).
type MetricsCollector struct {
	// Name identifies the collector (the collector label of vrx_agent_metrics_collector_errors_total).
	Name string
	// Collect writes complete families in the text exposition format 0.0.4 (# HELP, # TYPE, samples;
	// names "vrx_<feature>_…", never "vrx_agent_…", no secrets in label values). It runs on every
	// scrape, concurrently with transactions and outside every agent lock, with a deadline on ctx:
	// read cached values or the stats segment, never block on the VPP binary API. When it returns an
	// error, nothing it wrote is served and the error counter goes up.
	Collect func(ctx context.Context, w io.Writer) error
}

// AddMetricsCollector registers a metrics collector; call it on the feature's line in Register. It
// refuses a malformed or duplicate name and a missing Collect.
func (w *Wiring) AddMetricsCollector(c MetricsCollector) error {
	if !seamName.MatchString(c.Name) || c.Collect == nil {
		return fmt.Errorf("metrics collector %q: a name matching %s and Collect are required", c.Name, seamName)
	}
	w.seams.mu.Lock()
	defer w.seams.mu.Unlock()
	for _, x := range w.seams.collectors {
		if x.Name == c.Name {
			return fmt.Errorf("metrics collector %s: registered twice", c.Name)
		}
	}
	w.seams.collectors = append(w.seams.collectors, c)
	return nil
}

// MetricsCollectors returns the registered collectors sorted by name (the agent calls it on every
// scrape).
func (w *Wiring) MetricsCollectors() []MetricsCollector {
	w.seams.mu.Lock()
	out := append([]MetricsCollector(nil), w.seams.collectors...)
	w.seams.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
