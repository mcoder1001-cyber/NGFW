package subsystems

// F-wireguard wiring: the DF-5 wireguard family (+ the agent-local wireguard.meta table) registered
// with the D-096 keyer, the D-071 globals flag and the agent's WireGuard secret store; the peer-event
// watcher that feeds TD-8's event sink (Env.Publish) and the WireguardState RPC's handshake times; the
// builder environment of the projection.
//
// State that the projection and the RPCs need is kept per process (project() carries no owner; the
// same pattern as F-rpf-adl-pbr's env): the last registered family is the projection's, the RPC looks
// its family up by owner. One product agent per process registers exactly one.

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// wgFamily is one agent's WireGuard family.
type wgFamily struct {
	owner   string
	peer    *wireguard.Peer
	secrets *WireguardSecrets
	obs     *WireguardObserver

	mu     sync.Mutex // guards the watcher
	cancel context.CancelFunc
	done   chan struct{}
}

var (
	wgMu       sync.Mutex
	wgByWiring = map[*Wiring]*wgFamily{}
	wgByOwner  = map[string]*wgFamily{}
	wgCurrent  *wgFamily
)

// wireguardFixture fills the secret store in test builds (wireguard_fixture.go and its build tag);
// nil in the product agent (no material without the secret channel, PENDING-secret-channel).
var wireguardFixture func(s *WireguardSecrets, log *slog.Logger) error

// wireguardDescriptors are the vpn domain's descriptors this build implements (Domains["vpn"];
// P11 and F-ikev2-native append theirs).
func wireguardDescriptors() []string { return append([]string(nil), desired.WireguardDescriptors...) }

// WireguardOptions are the options DF-5's wireguard.Register gets from the product agent (IPsecOptions
// pattern): the D-096 keyer, the D-071 globals flag and the agent's WireGuard secret store.
func (w *Wiring) WireguardOptions(secrets *WireguardSecrets) ([]wireguard.Option, error) {
	k, err := w.VPNKeyer()
	if err != nil {
		return nil, err
	}
	return []wireguard.Option{wireguard.WithKeyer(k), wireguard.WithGlobalsOwner(w.env.GlobalsOwner), wireguard.WithSecrets(secrets)}, nil
}

// registerWireguard registers wireguard.interface, wireguard.peer, wireguard.async-mode (setter only
// for the globals owner, D-071) and wireguard.meta (file store <state dir>/wireguard-meta-<owner>.json).
func (w *Wiring) registerWireguard(r scheduler.Registry) error {
	k, err := w.VPNKeyer()
	if err != nil {
		return fmt.Errorf("wireguard: %w", err)
	}
	secrets := NewWireguardSecrets(k)
	if wireguardFixture != nil {
		if err := wireguardFixture(secrets, w.env.Log); err != nil {
			return fmt.Errorf("wireguard: test secret fixture: %w", err)
		}
	}
	opts, err := w.WireguardOptions(secrets)
	if err != nil {
		return fmt.Errorf("wireguard: %w", err)
	}
	peer := wireguard.Register(wgRegistry{r}, w.env.Client, w.env.Owner, opts...)
	store, err := wireguard.NewFileMetaStore(filepath.Join(w.env.StateDir, "wireguard-meta-"+w.env.Owner+".json"))
	if err != nil {
		return err
	}
	r.Register(wireguard.NewMeta(store))
	fam := &wgFamily{owner: w.env.Owner, peer: peer, secrets: secrets, obs: newWireguardObserver()}
	wgMu.Lock()
	wgByWiring[w], wgByOwner[w.env.Owner], wgCurrent = fam, fam, fam
	wgMu.Unlock()
	return nil
}

// wgRegistry gives DF-5's non-owner requirement (vpn.Require, a read-only package for this row) the
// TD-11b ownership declaration it lacks: a requirement records nothing. Everything else is registered
// as it is.
type wgRegistry struct{ scheduler.Registry }

func (g wgRegistry) Register(d scheduler.Descriptor) {
	if req, ok := d.(*vpn.Require); ok {
		d = &wgRequirement{Require: req}
	}
	g.Registry.Register(d)
}

// wgRequirement is vpn.Require with the TD-11b declaration (every method is vpn.Require's).
type wgRequirement struct{ *vpn.Require }

// RecordsNoOwnership declares that a VPP-global requirement records no ownership (TD-11b).
func (*wgRequirement) RecordsNoOwnership() {}

func wireguardFamily(w *Wiring) *wgFamily {
	wgMu.Lock()
	defer wgMu.Unlock()
	return wgByWiring[w]
}

// WireguardEnv is the WireGuard builder's environment for the projection (projection.go): the secret
// reference mapping of the registered family, or none (every WireGuard interface then fails with
// agent.secret-unavailable).
func WireguardEnv() desired.WireguardEnv {
	wgMu.Lock()
	fam := wgCurrent
	wgMu.Unlock()
	if fam == nil {
		return desired.WireguardEnv{}
	}
	return desired.WireguardEnv{SecretRef: fam.secrets.Ref}
}

// WireguardObserverFor returns the peer-event observer of owner's family (nil when not registered).
func WireguardObserverFor(owner string) *WireguardObserver {
	wgMu.Lock()
	defer wgMu.Unlock()
	if fam := wgByOwner[owner]; fam != nil {
		return fam.obs
	}
	return nil
}

// WireguardSecretsFor returns owner's WireGuard secret store (tests fill it; nil when not registered).
func WireguardSecretsFor(owner string) *WireguardSecrets {
	wgMu.Lock()
	defer wgMu.Unlock()
	if fam := wgByOwner[owner]; fam != nil {
		return fam.secrets
	}
	return nil
}

// ---- peer events (DF-5 Q8) -----------------------------------------------------------------------

// wireguardConnected (re)starts the peer-event watcher on a new binary-API connection (called from
// Connected, before the resync): the previous connection's VPP-side registration died with it. The
// watcher runs until the next connect or until ctx (the agent's) ends. Without an event sink
// (Env.Publish nil) nothing is started.
func (w *Wiring) wireguardConnected(ctx context.Context) {
	fam := wireguardFamily(w)
	if fam == nil || w.env.Publish == nil {
		return // no event sink (tests without an agent bus): nothing to publish, no subscription
	}
	fam.mu.Lock()
	defer fam.mu.Unlock()
	if fam.cancel != nil {
		fam.cancel()
		<-fam.done
	}
	wctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	fam.cancel, fam.done = cancel, done
	go func() {
		defer close(done)
		fam.watch(wctx, w.Publish, w.env.Log)
	}()
}

// watch subscribes to peer.Events and publishes every event; it re-subscribes with backoff when the
// subscription fails or ends before ctx.
func (f *wgFamily) watch(ctx context.Context, publish func(*vrxv1.Event), log *slog.Logger) {
	backoff, warned := time.Second, false
	for ctx.Err() == nil {
		events, err := f.peer.Events(ctx)
		if err != nil {
			lvl := slog.LevelWarn
			if warned {
				lvl = slog.LevelDebug // once per outage at WARN
			}
			warned = true
			log.Log(ctx, lvl, "wireguard peer events: subscribe failed, retrying", "err", err, "in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(2*backoff, time.Minute)
			continue
		}
		backoff, warned = time.Second, false
		f.obs.setActive(true)
		for ev := range events {
			f.obs.record(ev, time.Now())
			publish(WireguardEvent(ev))
		}
		f.obs.setActive(false)
		if ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
		}
	}
}

// WireguardEvent is the StreamEvents form of a peer event (EVENT_KIND_WIREGUARD_PEER_CHANGED,
// docs/contracts/proto.md §11): no key material, the public key is the peer's identity.
func WireguardEvent(ev wireguard.PeerEvent) *vrxv1.Event {
	state := "down"
	switch {
	case ev.Established:
		state = "established"
	case ev.Dead:
		state = "dead"
	}
	short := ev.PublicKey
	if len(short) > 8 {
		short = short[:8] + "…"
	}
	return &vrxv1.Event{
		Kind: vrxv1.EventKind_EVENT_KIND_WIREGUARD_PEER_CHANGED, Interface: proto.String(ev.Interface),
		Message: fmt.Sprintf("WireGuard peer %s on %s: %s", short, ev.Interface, state),
		Attributes: map[string]string{
			"public_key": ev.PublicKey, "peer_index": strconv.FormatUint(uint64(ev.PeerIndex), 10),
			"established": strconv.FormatBool(ev.Established), "dead": strconv.FormatBool(ev.Dead),
		},
	}
}

// WireguardObserver keeps what the peer events tell and VPP's dumps do not: when this agent last saw
// each peer become established (VPP 26.06 has no handshake timestamp), and whether it is watching.
type WireguardObserver struct {
	mu     sync.Mutex
	active bool
	last   map[string]time.Time // "<wg>|<public key>" → last established event
}

func newWireguardObserver() *WireguardObserver {
	return &WireguardObserver{last: map[string]time.Time{}}
}

func (o *WireguardObserver) setActive(v bool) {
	o.mu.Lock()
	o.active = v
	o.mu.Unlock()
}

func (o *WireguardObserver) record(ev wireguard.PeerEvent, at time.Time) {
	if !ev.Established {
		return
	}
	o.mu.Lock()
	o.last[ev.Interface+"|"+ev.PublicKey] = at
	o.mu.Unlock()
}

// Active reports whether the peer-event subscription is running.
func (o *WireguardObserver) Active() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active
}

// LastEstablished returns when the peer on interface wg was last seen becoming established.
func (o *WireguardObserver) LastEstablished(wg, publicKey string) (time.Time, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	t, ok := o.last[wg+"|"+publicKey]
	return t, ok
}
