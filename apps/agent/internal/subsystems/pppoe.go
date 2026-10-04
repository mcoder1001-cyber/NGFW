package subsystems

// F-pppoe-client / F-pppoe-client-host (D-168): the PPPoE *client* subsystem. It owns the pppd renderer/supervisor
// (renderers/pppoe) and the VPP mirror (descriptors/pppoe.ClientMirror) for one agent owner, and exposes them to the
// RPC layer through a per-owner runtime registry (PppoeOf), mirroring the kea/unbound wiring pattern.
//
// The companion singleton scheduler descriptor projects enabled interface configuration into renderer sessions,
// resolving password references only at the renderer boundary. The watcher learns negotiated address and route
// state from ip-up/ip-down hooks and serializes VPP mirroring with configuration transactions.
// Only the globals owner drives host pppd units and default routes. Test slots render private files without host
// supervision, refuse reconnect, and apply their bounded route policy. Exit observations expose failCount/lastError.
// Real ISP dialing and Linux/VPP interoperability remain laboratory acceptance.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/vpp"
)

// ErrNotGlobalsOwner means the pppoe client daemon is driven only by the globals owner; a slot agent neither runs
// the host's pppd units nor reconnects them.
var ErrNotGlobalsOwner = errors.New("pppoe client is driven only by the globals owner (this agent is a test slot)")

// PppoeRuntime is the per-owner PPPoE client runtime the RPC layer reaches.
type PppoeRuntime struct {
	renderer     *pppoe.Renderer
	runner       renderers.Runner
	vpp          vpp.Client
	owner        string
	globalsOwner bool
	allowRoute   pppoedesc.RouteTablePolicy
	log          *slog.Logger
	stateDir     string

	mu        sync.Mutex
	exclusive func(context.Context, func(context.Context) error) error
	mirrored  map[string]pppoedesc.Mirror
	failures  map[string]pppoeFailure
	applied   map[string]pppoe.Session // config interface name -> last applied session (reconnect + state)
}

var (
	pppoeMu  sync.Mutex
	pppoeReg = map[string]*PppoeRuntime{}
)

// PppoeOf returns the PPPoE client runtime registered for owner (nil before registerPppoe).
func PppoeOf(owner string) *PppoeRuntime {
	pppoeMu.Lock()
	defer pppoeMu.Unlock()
	return pppoeReg[owner]
}

// refuseRunner is the Runner a non-globals-owner agent gets: it records the attempt and refuses to exec, so a slot
// never drives the host's systemd (D-079/D-173).
type refuseRunner struct{ log *slog.Logger }

func (r refuseRunner) Run(_ context.Context, cmd renderers.Command) (renderers.Output, error) {
	r.log.Warn("pppoe: refusing to run on a non-globals-owner agent", "path", cmd.Path, "args", cmd.Args)
	return renderers.Output{}, fmt.Errorf("%w: would have run %s", ErrNotGlobalsOwner, cmd.Path)
}

// registerPppoe builds the pppd renderer for env (product paths on the globals owner, else the slot's own tree) and
// records the per-owner runtime for the state/reconnect RPC. It registers no scheduler descriptor (see the file
// comment). Calling pppoe.New here is what makes renderers/pppoe reachable (TD-11a).
func (w *Wiring) registerPppoe() {
	log := w.env.Log.With("feature", "pppoe-client")
	globals := w.env.GlobalsOwner
	var runner renderers.Runner
	if globals {
		runner = renderers.NewSystemRunner(renderers.NewAllowlist(pppoe.Binaries()...))
	} else {
		runner = refuseRunner{log: log}
	}
	rt := &PppoeRuntime{
		renderer:     newPppoeRenderer(w.env),
		runner:       runner,
		vpp:          w.env.Client,
		owner:        w.env.Owner,
		globalsOwner: globals,
		allowRoute:   w.pppoeRoutePolicy(),
		exclusive:    w.env.Exclusive,
		log:          log,
		stateDir:     pppoeStateDir(w.env),
		applied:      map[string]pppoe.Session{},
	}
	pppoeMu.Lock()
	pppoeReg[w.env.Owner] = rt
	pppoeMu.Unlock()
	log.Info("pppoe client wired", "globals_owner", globals, "product_paths", globals)
}

// pppoeRoutePolicy decides whether the default route may be written in a FIB table: the globals owner may write any;
// a slot may write only inside its own id range; anything else fails closed.
func (w *Wiring) pppoeRoutePolicy() pppoedesc.RouteTablePolicy {
	if w.env.GlobalsOwner {
		return func(uint32) bool { return true }
	}
	idr, err := w.IDRange()
	return func(table uint32) bool {
		if err != nil {
			return false // no id range: fail closed
		}
		if idr == nil {
			return true // NGFW_VPP_ID_RANGE=all
		}
		return !idr.Empty() && table >= idr.Lo && table <= idr.Hi
	}
}

// newPppoeRenderer returns the pppd renderer for env: product paths on the globals owner, else the slot's own tree
// under /run/ngfw-test/<owner>/pppoe (loopback-safe, never a system unit).
func newPppoeRenderer(env Env) *pppoe.Renderer {
	if env.GlobalsOwner {
		return pppoe.New()
	}
	return pppoe.New(pppoe.WithPaths(pppoe.PathsUnder(filepath.Join(slotRunDir(env.Owner), "pppoe"))))
}

// Reconnect restarts the pppd unit of the config interface now (ignores the hold-off). Only the globals owner drives
// the host's pppd units; a slot returns ErrNotGlobalsOwner. Accepted is false when no session has been applied for
// that interface (nothing to redial).
func (rt *PppoeRuntime) Reconnect(ctx context.Context, iface string) (accepted bool, message string, err error) {
	if !rt.globalsOwner {
		return false, "", ErrNotGlobalsOwner
	}
	rt.mu.Lock()
	s, ok := rt.applied[iface]
	rt.mu.Unlock()
	if !ok {
		return false, "no active PPPoE session for interface " + iface, nil
	}
	unit := "ngfw-pppoe-" + s.HostIf + ".service"
	if _, err := rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"restart", unit}}); err != nil {
		return false, "", fmt.Errorf("pppoe reconnect %s: %w", iface, err)
	}
	rt.log.Info("pppoe reconnect", "interface", iface, "unit", unit)
	return true, "redialing " + iface, nil
}

// Apply renders and supervises the resolved sessions and remembers them for Reconnect/State. The caller resolves the
// sessions (parent tap + password) — see the file comment for the owed projection.
func (rt *PppoeRuntime) Apply(ctx context.Context, sessions []pppoe.Session) error {
	// Renderer compares peer/unit content, so credential-only rotation needs an
	// explicit restart here. Never retain an earlier negotiated hook after edit.
	files, err := rt.renderer.Render(sessions)
	if err != nil {
		return err
	}

	// Compare each fixed secret path; no secret text is returned or logged.
	secretsChanged := false
	for path, f := range files {
		if f.Secret {
			old, e := os.ReadFile(path) //nolint:gosec // fixed paths supplied by this owner renderer
			if e != nil || !bytes.Equal(old, f.Content) {
				secretsChanged = true
			}
		}
	}
	want := map[string]pppoe.Session{}
	for _, s := range sessions {
		record := s
		record.Password = ""
		want[s.Iface] = record
	}
	rt.mu.Lock()
	restarts := []string{}
	for name, oldSession := range rt.applied {
		next, exists := want[name]
		changed := !exists || !reflect.DeepEqual(oldSession, next) || secretsChanged
		if !changed {
			continue
		}
		if old, ok := rt.mirrored[name]; ok {
			if err := rt.Mirror(ctx, old, false); err != nil {
				rt.mu.Unlock()
				return err
			}
			delete(rt.mirrored, name)
		}

		// Clamp-only edits affect the VPP mirror, not the dialed session.
		oldDial, nextDial := oldSession, next
		oldDial.MSSClamp = false
		nextDial.MSSClamp = false
		if exists && !secretsChanged && reflect.DeepEqual(oldDial, nextDial) {
			continue
		}
		// Derive the state filename from the renderer's fixed slot/product paths.
		if err := os.Remove(filepath.Join(rt.stateDir, oldSession.HostIf+".state")); err != nil && !os.IsNotExist(err) {
			rt.mu.Unlock()
			return err
		}

		restartHost := ""
		if exists {
			restartHost = next.HostIf
		} else {
			for _, replacement := range want {
				if replacement.HostIf == oldSession.HostIf {
					restartHost = replacement.HostIf
					break
				}
			}
		}
		if restartHost != "" {
			automatic := false
			for path, file := range files {
				base := filepath.Base(path)
				if (base == "ngfw-"+restartHost && filepath.Base(filepath.Dir(path)) == "peers") || base == "ngfw-pppoe-"+restartHost+".service" {
					existing, e := os.ReadFile(path) //nolint:gosec // renderer-owned fixed peer/unit paths
					if e != nil || !bytes.Equal(existing, file.Content) {
						automatic = true
					}
				}
			}
			if !automatic {
				restarts = append(restarts, restartHost)
			}
		}
	}
	rt.mu.Unlock()

	if rt.globalsOwner {
		if err := rt.renderer.Apply(ctx, rt.runner, sessions); err != nil {
			return err
		}
		for _, host := range restarts {
			if _, err := rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"restart", "ngfw-pppoe-" + host + ".service"}}); err != nil {
				return errors.New("PPPoE credential rotation restart failed")
			}
		}

	} else {
		files, err := rt.renderer.Render(sessions)
		if err != nil {
			return err
		}
		paths := pppoe.PathsUnder(filepath.Join(slotRunDir(rt.owner), "pppoe"))
		// Explicitly remove only files created by this owner's previous sessions.
		rt.mu.Lock()
		old := make([]pppoe.Session, 0, len(rt.applied))
		for _, s := range rt.applied {
			old = append(old, s)
		}
		rt.mu.Unlock()
		previous, err := rt.renderer.Render(old)
		if err != nil {
			return err
		}
		for path := range previous {
			if _, keep := files[path]; !keep {
				if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
					return e
				}
			}
		}
		for _, dir := range []string{paths.PeersDir, paths.IPUpDir, paths.IPDownDir, paths.UnitDir, paths.StateDir} {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
		}
		if err = renderers.WriteFiles(files); err != nil {
			return err
		}
		rt.log.Warn("pppoe.not-supervised", "owner", rt.owner)
	}
	rt.mu.Lock()
	rt.applied = make(map[string]pppoe.Session, len(sessions))
	for _, s := range sessions {
		s.Password = "" // credentials belong only in secret files, not operational memory
		rt.applied[s.Iface] = s
	}
	rt.mu.Unlock()
	return nil
}

// Mirror reflects a negotiated session into VPP (up=true) or withdraws it (up=false), applying this agent's
// route-table policy (only the globals owner or the agent's own tables get a default route).
func (rt *PppoeRuntime) Mirror(ctx context.Context, m pppoedesc.Mirror, up bool) error {
	mirror := pppoedesc.NewClientMirror(rt.vpp, rt.owner, rt.log, pppoedesc.WithRouteTablePolicy(rt.allowRoute))
	return mirror.Apply(ctx, m, up)
}

// State reads the live PPPoE session state for a config interface. failCount/lastErr come from the agent's pppd
// exit tracking (owed); a session that was never applied is "down".
func (rt *PppoeRuntime) State(iface string, failCount uint32, lastErr string) (*ngfwv1.PppoeSessionState, error) {
	rt.mu.Lock()
	s, ok := rt.applied[iface]
	failure := rt.failures[iface]
	rt.mu.Unlock()
	if failCount == 0 {
		failCount = failure.count
		lastErr = failure.message
	}
	if !ok {
		return &ngfwv1.PppoeSessionState{Phase: "down", FailCount: failCount, LastError: lastErr}, nil
	}
	return rt.renderer.ReadState(s.HostIf, failCount, lastErr)
}

// Renderer exposes the pppd renderer (RPC state; tests).
func (rt *PppoeRuntime) Renderer() *pppoe.Renderer { return rt.renderer }

// Remember hydrates process-local observations from a verified applied manifest
// after restart. No files, secrets or VPP objects are written by retrieval.
func (rt *PppoeRuntime) Remember(sessions []pppoe.Session) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.applied = map[string]pppoe.Session{}
	for _, s := range sessions {
		s.Password = ""
		rt.applied[s.Iface] = s
	}
}

func pppoeStateDir(env Env) string {
	if env.GlobalsOwner {
		return pppoe.ProductPaths().StateDir
	}
	return pppoe.PathsUnder(filepath.Join(slotRunDir(env.Owner), "pppoe")).StateDir
}
