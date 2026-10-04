package subsystems

// The materializer is isolated under the private agent state directory. It consumes only
// operational PKI references delivered over the existing authorized socket channel; no
// system daemon is started or reloaded. Drift triggers the normal scheduler resync.

import (
	"context"
	"fmt"

	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/pki"
	"ngfw/agent/internal/scheduler"
)

// pkiFilesName is the descriptor name in Domains["vpn"] (appended here, the tunnels.go pattern, so subsystems.go needs
// only the register line).
const pkiFilesName = pki.Name

func init() { Domains[VPN] = append(Domains[VPN], pkiFilesName) }

// pkiWatchInterval is how often the runtime checks the files on disk (a var for the tests).
var pkiWatchInterval = 30 * time.Second

// pkiResolver adapts active literal references from the sealed cache, without exposing resolver errors.
type pkiResolver struct {
	mu     sync.RWMutex
	source func(string) ([]byte, error)
}

func (r *pkiResolver) Resolve(ctx context.Context, ref string) ([]byte, error) {
	r.mu.RLock()
	source := r.source
	r.mu.RUnlock()
	if source == nil {
		return nil, pki.ErrNoSource
	}
	material, err := source(ref)
	if err != nil {
		clear(material)
		return nil, pki.ErrNotFound
	}
	return material, nil
}
func SetPKISecrets(owner string, source func(string) ([]byte, error)) error {
	rt := PKIRuntimeFor(owner)
	if rt == nil || rt.source == nil {
		return pki.ErrNoSource
	}
	rt.source.mu.Lock()
	rt.source.source = source
	rt.source.mu.Unlock()
	return nil
}

// PKIRuntime is one agent's PKI file materialiser (nil materialiser: this agent materialises nothing, see Reason).
type PKIRuntime struct {
	source *pkiResolver
	owner  string
	m      *pki.Materialiser
	reason string

	watchMu sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

var (
	pkiRuntimes sync.Map                   // owner → *PKIRuntime
	activePKI   atomic.Pointer[PKIRuntime] // this process's agent (one agent per process)
)

// PKIRuntimeFor returns owner's runtime (nil when the materialiser is not wired into this agent).
func PKIRuntimeFor(owner string) *PKIRuntime {
	if v, ok := pkiRuntimes.Load(owner); ok {
		return v.(*PKIRuntime)
	}
	return nil
}

// Enabled reports whether this agent materialises PKI files.
func (rt *PKIRuntime) Enabled() bool { return rt != nil && rt.m != nil }

// Reason says why the runtime is disabled ("" when enabled).
func (rt *PKIRuntime) Reason() string {
	if rt == nil {
		return "the PKI file materialiser is not wired into this agent build"
	}
	return rt.reason
}

// Root returns the swanctl directory ("" when disabled).
func (rt *PKIRuntime) Root() string {
	if !rt.Enabled() {
		return ""
	}
	return rt.m.Root()
}

// State lists the materialised files (the PkiFileState RPC).
func (rt *PKIRuntime) State() ([]*ngfwv1.PkiFileStateFile, error) {
	if !rt.Enabled() {
		return nil, nil
	}
	return rt.m.State()
}

// PKIProjection returns the PKI builder's options from this process's runtime.
func PKIProjection() desired.PKIOptions {
	rt := activePKI.Load()
	if !rt.Enabled() {
		return desired.PKIOptions{Disabled: rt != nil}
	}
	return desired.PKIOptions{Plan: rt.m.Plan}
}

// registerPKI registers the pki.files descriptor inside the agent state directory.
func (w *Wiring) registerPKI(r scheduler.Registry) error {
	rt := &PKIRuntime{owner: w.env.Owner, source: &pkiResolver{}}
	keyer, err := w.VPNKeyer()
	if err != nil {
		return fmt.Errorf("pki: %w", err)
	}
	m, err := pki.New(pki.Config{
		Root:     filepath.Join(w.env.StateDir, "pki-"+w.env.Owner),
		Manifest: filepath.Join(w.env.StateDir, "pki-files-"+w.env.Owner+".json"),
		Keyer:    keyer, Source: rt.source, Log: w.env.Log,
	})
	if err != nil {
		return fmt.Errorf("pki: %w", err)
	}
	rt.m = m
	if old, ok := pkiRuntimes.Swap(w.env.Owner, rt); ok {
		old.(*PKIRuntime).Close()
	}
	activePKI.Store(rt)
	if !rt.Enabled() {
		w.env.Log.Info("PKI files not materialised by this agent", "reason", rt.reason)
		return nil
	}
	r.Register(pki.NewDescriptor(rt.m))
	rt.startWatch(w.RequestResync)
	w.OnClose(rt.Close)
	w.env.Log.Info("PKI file materialiser wired", "root", rt.m.Root(), "source", rt.m.HasSource())
	return nil
}

// startWatch runs the materialiser's drift watch until Close.
func (rt *PKIRuntime) startWatch(resync func()) {
	rt.watchMu.Lock()
	defer rt.watchMu.Unlock()
	if rt.cancel != nil || !rt.Enabled() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	rt.cancel, rt.done = cancel, done
	go func() {
		defer close(done)
		rt.m.Watch(ctx, pkiWatchInterval, resync)
	}()
}

// Close stops the watcher (Agent.Stop through Wiring.OnClose).
func (rt *PKIRuntime) Close() {
	if rt.Enabled() {
		defer rt.m.Close()
	}
	rt.watchMu.Lock()
	cancel, done := rt.cancel, rt.done
	rt.cancel, rt.done = nil, nil
	rt.watchMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// SetPKIRuntimeForTest installs a PKI runtime over m for owner (nil m: a disabled runtime with reason) and returns a
// restore function. Test support for packages outside subsystems (the PkiFileState rpc test); the product never calls
// it.
func SetPKIRuntimeForTest(owner string, m *pki.Materialiser, reason string) (restore func()) {
	rt := &PKIRuntime{owner: owner, m: m, reason: reason}
	old, had := pkiRuntimes.Swap(owner, rt)
	prevActive := activePKI.Swap(rt)
	return func() {
		activePKI.Store(prevActive)
		if had {
			pkiRuntimes.Store(owner, old)
		} else {
			pkiRuntimes.Delete(owner)
		}
	}
}
