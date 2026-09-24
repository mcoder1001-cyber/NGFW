package objects

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Config opens a Runtime.
type Config struct {
	// StateDir is the agent state dir (VRX_AGENT_STATE_DIR); Owner the agent owner (VRX_OWNER).
	StateDir string
	Owner    string
	Log      *slog.Logger
	// Lookup resolves FQDN objects; nil = NetLookup(nil), the system resolver.
	Lookup Lookup
	// Refresh is the fixed refresh interval (0 = DefaultRefresh), clamped to [MinRefresh, MaxRefresh].
	Refresh time.Duration
	// Now is the clock (nil = time.Now); tests pass a fake one.
	Now func() time.Time
}

// Runtime is the objects domain of one running agent: the store of the applied objects document
// and the FQDN resolver over it. subsystems/object_model.go opens it when the family registers.
type Runtime struct {
	key   string
	store *Store
	res   *resolver
	log   *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var (
	runtimesMu sync.Mutex
	runtimes   = map[string]*Runtime{}
)

func runtimeKey(stateDir, owner string) string { return filepath.Clean(stateDir) + "\x00" + owner }

// Open loads the store (<dir>/objects-<owner>.json) and the FQDN state (<dir>/objects-fqdn-<owner>.json)
// and registers the runtime for RuntimeFor. A runtime already open for the same state dir and owner
// (an agent restarted inside one process) is closed first. The resolver does not run until Start.
func Open(cfg Config) (*Runtime, error) {
	if cfg.StateDir == "" || cfg.Owner == "" {
		return nil, errors.New("objects: state dir and owner are required")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Lookup == nil {
		cfg.Lookup = NetLookup(nil)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	store, err := OpenStore(filepath.Join(cfg.StateDir, "objects-"+cfg.Owner+".json"))
	if err != nil {
		return nil, err
	}
	res := newResolver(filepath.Join(cfg.StateDir, "objects-fqdn-"+cfg.Owner+".json"), cfg.Lookup, cfg.Refresh, cfg.Now, cfg.Log)
	res.load()
	rt := &Runtime{key: runtimeKey(cfg.StateDir, cfg.Owner), store: store, res: res, log: cfg.Log}
	store.onChange = rt.syncFQDN
	res.sync(fqdnObjects(store.Snapshot()), true)

	runtimesMu.Lock()
	prev := runtimes[rt.key]
	runtimes[rt.key] = rt
	runtimesMu.Unlock()
	if prev != nil {
		prev.stop()
	}
	return rt, nil
}

// RuntimeFor returns the runtime open for stateDir and owner (nil if none).
func RuntimeFor(stateDir, owner string) *Runtime {
	runtimesMu.Lock()
	defer runtimesMu.Unlock()
	return runtimes[runtimeKey(stateDir, owner)]
}

// Start runs the resolver loop in the background (idempotent).
func (rt *Runtime) Start() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.cancel, rt.done = cancel, make(chan struct{})
	go func(done chan struct{}) {
		defer close(done)
		rt.res.run(ctx)
	}(rt.done)
}

// Close stops the resolver loop and unregisters the runtime. The store stays readable.
func (rt *Runtime) Close() {
	rt.stop()
	runtimesMu.Lock()
	if runtimes[rt.key] == rt {
		delete(runtimes, rt.key)
	}
	runtimesMu.Unlock()
}

func (rt *Runtime) stop() {
	rt.mu.Lock()
	cancel, done := rt.cancel, rt.done
	rt.cancel, rt.done = nil, nil
	rt.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// Store is the runtime's objects store.
func (rt *Runtime) Store() *Store { return rt.store }

// Snapshot is a deep copy of the applied objects document (what Retrieve returns).
func (rt *Runtime) Snapshot() *vrxv1.ObjectsConfig { return rt.store.Snapshot() }

// FQDN is the FQDNLookup of this runtime (use with WithFQDN): the addresses FQDN address object
// name resolves to now; false when it has none (never resolved) or is not an FQDN object.
func (rt *Runtime) FQDN(name string) ([]netip.Addr, bool) { return rt.res.addresses(name) }

// FQDNStates is the resolver state of the FQDN objects named (all when none), sorted by name.
func (rt *Runtime) FQDNStates(names ...string) []FQDNState { return rt.res.states(names) }

// Subscribe calls f (on the resolver goroutine; keep it short) after every change of the
// addresses an FQDN resolves to — the in-agent notification that lets a consumer (F-acl)
// re-project what depends on it. The returned function unsubscribes.
func (rt *Runtime) Subscribe(f func(Change)) (unsubscribe func()) { return rt.res.subscribe(f) }

// ResolveDue resolves every FQDN whose refresh is due now (synchronously; tests with a fake
// clock, and the loop). It returns the number of names resolved.
func (rt *Runtime) ResolveDue(ctx context.Context) int { return rt.res.resolveDue(ctx, 0) }

// Queries is the number of resolutions started since Open (A+AAAA of one name count once).
func (rt *Runtime) Queries() int {
	rt.res.mu.Lock()
	defer rt.res.mu.Unlock()
	return rt.res.queries
}

func (rt *Runtime) syncFQDN() { rt.res.sync(fqdnObjects(rt.store.Snapshot()), false) }

// fqdnObjects maps every FQDN address object of doc to its host name.
func fqdnObjects(doc *vrxv1.ObjectsConfig) map[string]string {
	out := map[string]string{}
	for name, a := range doc.GetAddresses() {
		if a.GetType() == "fqdn" && a.GetFqdn() != "" {
			out[name] = a.GetFqdn()
		}
	}
	return out
}
