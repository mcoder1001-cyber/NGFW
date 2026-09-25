package subsystems

// F-lb: the VPP load-balancer families (DF-7's descriptors/lb; D-104) and the globals owner's lb garbage collection
// (D-090 (2)).
//
//	services  lb.vip, lb.as, lb.intf-nat (every agent); lb.conf (the globals owner only, D-071)
//
// VPP 26.06 frees a deleted VIP or AS ("removed", V20) only in its garbage collection, which the binary API never
// runs for a removed VIP. In the globals owner a successful lb.vip or lb.as Delete — including the delete half of a
// change (ErrRecreate) and a rollback — (re)arms one timer; lb.GCDelay after the last such delete the constant
// lb.GCCommand runs once through cli_inband (lb.GarbageCollect, ALLOWLIST.md). Slot agents (VRX_GLOBALS_OWNER=0) never
// send it (TestLbSlotAgentNeverCollects).

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// lbGCDelay is lb.GCDelay; tests shorten it.
var lbGCDelay = lb.GCDelay

// lbGCRun is lb.GarbageCollect; tests replace it.
var lbGCRun = lb.GarbageCollect

// lbGlobalsOwner is the globals-owner flag of the last registerLb (the projection reads it through LbEnv, like
// LoopbackBviGsoLldpSpanEnv): one product agent per process.
var lbGlobalsOwner atomic.Bool

// LbEnv is the lb projection's view of the wiring (desired.Lb).
func LbEnv() desired.LbEnv { return desired.LbEnv{GlobalsOwner: lbGlobalsOwner.Load()} }

// registerLb registers the lb families through one lb.Register call (+ lb.RegisterGlobals in the globals owner).
// Their records live in the persisted DF-7 BootStore that register installs (df7.SetBootStore, D-076/D-080).
func (w *Wiring) registerLb(r scheduler.Registry) {
	c, owner := w.env.Client, w.env.Owner
	lbGlobalsOwner.Store(w.env.GlobalsOwner)
	if !w.env.GlobalsOwner {
		lb.Register(r, c, owner)
		return
	}
	gc := &lbGC{client: c, log: w.env.Log, delay: lbGCDelay, run: lbGCRun}
	lb.Register(lbGCRegistry{Registry: r, gc: gc}, c, owner)
	lb.RegisterGlobals(r, c, owner)
}

// lbGCRegistry wraps the lb.vip and lb.as descriptors so that their deletes schedule the garbage collection.
type lbGCRegistry struct {
	scheduler.Registry
	gc *lbGC
}

// Register implements scheduler.Registry.
func (g lbGCRegistry) Register(d scheduler.Descriptor) {
	if n := d.Name(); n == lb.NameVIP || n == lb.NameAS {
		d = &lbGCOnDelete{Descriptor: d, gc: g.gc}
	}
	g.Registry.Register(d)
}

// lbGCOnDelete is an lb.vip / lb.as descriptor whose successful Delete schedules the garbage collection. The embedded
// descriptor carries the TD-11b ownership declaration (dfkit/persist unwraps it).
type lbGCOnDelete struct {
	scheduler.Descriptor
	gc *lbGC
}

// Delete implements scheduler.Descriptor.
func (d *lbGCOnDelete) Delete(ctx context.Context, obj proto.Message, meta any) error {
	if err := d.Descriptor.Delete(ctx, obj, meta); err != nil {
		return err
	}
	d.gc.schedule()
	return nil
}

// Unwrap returns the lb descriptor (dfkit/persist).
func (d *lbGCOnDelete) Unwrap() scheduler.Descriptor { return d.Descriptor }

// lbGC runs lb.GarbageCollect once, delay after the last schedule call (debounced: a burst of deletes — one
// transaction, a rollback — collects once).
type lbGC struct {
	client vpp.Client
	log    *slog.Logger
	delay  time.Duration
	run    func(context.Context, vpp.Client) error

	mu    sync.Mutex
	timer *time.Timer
}

func (g *lbGC) schedule() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
	}
	g.timer = time.AfterFunc(g.delay, g.fire)
}

func (g *lbGC) fire() {
	log := g.log
	if log == nil {
		log = slog.Default()
	}
	if !g.client.Connected() {
		log.Warn("lb garbage collection skipped: VPP not connected (removed VIPs stay until the next lb delete or a VPP restart, V20)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch err := g.run(ctx, g.client); {
	case errors.Is(err, lb.ErrGCUnsafe):
		log.Warn("lb garbage collection skipped (V20)", "reason", err)
	case err != nil:
		log.Error("lb garbage collection failed", "command", lb.GCCommand, "err", err)
	default:
		log.Info("lb garbage collection ran (D-090)", "command", lb.GCCommand)
	}
}
