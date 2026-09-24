package subsystems

// F-neighbors-ra wiring (registration helper, the slot id range of proxy-ARP tables, the neighbour-event watcher).
// subsystems.go carries only registration lines under the feature's wave-A anchors; everything else is here.

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"ngfw/agent/binapi/ip_neighbor"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	neighborsra "ngfw/agent/internal/actions/neighbors-ra"
	"ngfw/agent/internal/descriptors/arp"
	"ngfw/agent/internal/descriptors/df2"
	iface "ngfw/agent/internal/descriptors/interface"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	ipneighbor "ngfw/agent/internal/descriptors/ip_neighbor"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names of the F-neighbors-ra families, one per Domains line (subsystems.go): RA and proxy per interface →
// interfaces, proxy-ARP ranges → vrfs, static neighbours / limits / DAD → routing. The conditional ones
// (ip-neighbor.config and ip6-nd.dad for the globals owner, ip6-nd.proxy with VRX_DF2_PROXY_ND=1) are listed
// unconditionally: a scope entry without a registered descriptor manages nothing.
const (
	neighborsRaRaConfig   = ip6nd.RaConfigName
	neighborsRaRaPrefix   = ip6nd.RaPrefixName
	neighborsRaProxyNd    = ip6nd.ProxyNdName
	neighborsRaProxyArpIf = arp.InterfaceName
	neighborsRaProxyRange = arp.RangeName
	neighborsRaNeighbor   = ipneighbor.NeighborName
	neighborsRaConfig     = ipneighbor.ConfigName
	neighborsRaDad        = ip6nd.DadName
)

// neighborsRaIDRange is the proxy-ARP table range this agent owns: the slot range N000–N999 from VRX_VPP_TABLE_BASE
// on the shared host, nil (= every table) for the product agent (W-seed's SlotIDRange).
func neighborsRaIDRange() (*df2.IDRange, error) {
	r, err := SlotIDRange()
	if err != nil || r == nil {
		return nil, err
	}
	return &df2.IDRange{Lo: r.Lo, Hi: r.Hi}, nil
}

// registerNeighborsRa registers DF-2's ip_neighbor, ip6_nd and arp families for this owner with the persisted claim
// store (Wiring.KeyedClaims("acl"), D-080 — never the in-memory default), the slot table range for proxy-ARP ranges,
// the VPP-wide ones (neighbour limits, DAD) only for the globals owner (D-071) and proxy ND only on opt-in (D-064/V12).
// It tells the projection what it registered.
func (w *Wiring) registerNeighborsRa(r scheduler.Registry) error {
	claims, err := w.KeyedClaims("acl")
	if err != nil {
		return err
	}
	tables, err := neighborsRaIDRange()
	if err != nil {
		return err
	}
	c, owner, opt := w.env.Client, w.env.Owner, df2.WithClaims(claims)
	ipneighbor.Register(r, c, owner, opt)
	ip6nd.Register(r, c, owner, opt)
	arp.Register(r, c, owner, tables, opt)
	proxyNd := desired.ProxyNdEnabled()
	if proxyNd {
		w.env.Log.Warn("proxy ND is enabled (experimental, VPP V12: the shared VPP aborted after ip6nd_proxy_add_del on 2026-09-23)", "env", desired.EnvProxyNd)
		ip6nd.RegisterProxyNd(r, c, owner, opt)
	}
	if w.env.GlobalsOwner {
		ipneighbor.RegisterGlobals(r, c)
		ip6nd.RegisterGlobals(r, c)
	}
	desired.ConfigureNeighborsRa(desired.NeighborsRaOptions{GlobalsOwner: w.env.GlobalsOwner, ProxyNd: proxyNd})
	return nil
}

// neighborWatch is one owner's running neighbour-event watcher (restarted on every VPP connect).
type neighborWatch struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var neighborWatches sync.Map // *Wiring → *neighborWatch

// neighborsRaConnected (re)starts the neighbour-event watcher on a new binary-API connection: the previous connection's
// VPP-side subscriptions died with it (want_ip_neighbor_events_reaper) and the govpp watcher may belong to a dropped
// connection. Without an event sink (Env.Publish nil: the A5 seam is not wired yet, TD-8) nothing is started.
func (w *Wiring) neighborsRaConnected(ctx context.Context) {
	if w.env.Publish == nil {
		return
	}
	v, _ := neighborWatches.LoadOrStore(w, &neighborWatch{})
	nw := v.(*neighborWatch)
	nw.mu.Lock()
	defer nw.mu.Unlock()
	nw.stopLocked()
	wctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	nw.cancel, nw.done = cancel, done
	cfg := NeighborWatchConfig{Client: w.env.Client, Owner: w.env.Owner, Publish: w.Publish, Log: w.env.Log}
	go func() {
		defer close(done)
		RunNeighborWatch(wctx, cfg)
	}()
}

// stopLocked stops the running watcher and waits for it (so its subscriptions and the new one never interleave).
func (nw *neighborWatch) stopLocked() {
	if nw.cancel == nil {
		return
	}
	nw.cancel()
	select {
	case <-nw.done:
	case <-time.After(5 * time.Second):
	}
	nw.cancel, nw.done = nil, nil
}

// NeighborWatchConfig parameterises RunNeighborWatch (zero durations take the defaults).
type NeighborWatchConfig struct {
	Client  vpp.Client
	Owner   string
	Publish func(*vrxv1.Event)
	Log     *slog.Logger
	// Tick is the coalescing period (default 1 s: at most one event per interface per second).
	Tick time.Duration
	// Rescan is how often new or vanished interfaces are picked up (default 30 s).
	Rescan time.Duration
	// PID is the pid field of the subscriptions (default os.Getpid()).
	PID uint32
}

// RunNeighborWatch subscribes to want_ip_neighbor_events_v2 on every interface the owner can name — one interface per
// request, never ~0 (every slot's interfaces) or 0 (VPP maps it to ~0) — re-scans the interface table every Rescan,
// drops events of interfaces it cannot name, and publishes coalesced EVENT_KIND_NEIGHBOR_CHANGED events every Tick
// until ctx is done.
func RunNeighborWatch(ctx context.Context, cfg NeighborWatchConfig) {
	if cfg.Tick <= 0 {
		cfg.Tick = time.Second
	}
	if cfg.Rescan <= 0 {
		cfg.Rescan = 30 * time.Second
	}
	if cfg.PID == 0 {
		cfg.PID = uint32(os.Getpid()) //nolint:gosec // G115: a Linux pid fits in 32 bits
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	watcher, err := cfg.Client.WatchEvent(ctx, &ip_neighbor.IPNeighborEventV2{})
	if err != nil {
		cfg.Log.Warn("neighbour events: watch ip_neighbor_event_v2", "err", err)
		return
	}
	defer watcher.Close()
	subscribed := map[uint32]string{} // sw_if_index → logical name
	rescan := func() {
		sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		t, err := iface.Dump(sctx, cfg.Client, cfg.Owner)
		if err != nil {
			cfg.Log.Warn("neighbour events: interface dump", "err", err)
			return
		}
		want := map[uint32]string{}
		for _, idx := range t.Indexes() {
			if name, ok := t.Logical(idx); ok && idx != 0 && idx != df2.NoInterface {
				want[idx] = name
			}
		}
		for idx, name := range want {
			if subscribed[idx] == name {
				continue
			}
			if err := neighborsra.Subscribe(sctx, cfg.Client, idx, cfg.PID, true); err != nil {
				cfg.Log.Warn("neighbour events: subscribe", "interface", name, "err", err)
				continue
			}
			subscribed[idx] = name
		}
		for idx, name := range subscribed {
			if want[idx] == name {
				continue
			}
			delete(subscribed, idx)
			if _, exists := t.Details(idx); exists { // gone interfaces took their watcher with them
				_ = neighborsra.Subscribe(sctx, cfg.Client, idx, cfg.PID, false)
			}
		}
	}
	rescan()
	tick := time.NewTicker(cfg.Tick)
	defer tick.Stop()
	again := time.NewTicker(cfg.Rescan)
	defer again.Stop()
	var co neighborsra.Coalescer
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-watcher.Events():
			if !ok {
				return
			}
			ch, ok := neighborsra.DecodeEvent(m)
			if !ok {
				continue
			}
			if name, ours := subscribed[ch.SwIfIndex]; ours {
				co.Add(name, ch)
			}
		case <-tick.C:
			for _, ev := range co.Flush() {
				cfg.Publish(ev)
			}
		case <-again.C:
			rescan()
		}
	}
}
