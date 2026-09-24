package subsystems

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

func TestNeighborsRaIDRange(t *testing.T) {
	t.Setenv(EnvTableBase, "")
	if r, err := neighborsRaIDRange(); r != nil || err != nil {
		t.Fatalf("product agent: %v %v (nil = every table)", r, err)
	}
	t.Setenv(EnvTableBase, "9000")
	if r, err := neighborsRaIDRange(); err != nil || r == nil || r.Lo != 9000 || r.Hi != 9999 {
		t.Fatalf("slot 9: %v %v", r, err)
	}
	t.Setenv(EnvTableBase, "x")
	if _, err := neighborsRaIDRange(); err == nil {
		t.Fatal("malformed base accepted")
	}
}

func register(t *testing.T, v *coretest.VPP, globals bool, publish func(*vrxv1.Event)) (*scheduler.MapRegistry, *Wiring) {
	t.Helper()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w9")
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: "w9", StateDir: dir, Owned: owned, GlobalsOwner: globals, Publish: publish})
	if err != nil {
		t.Fatal(err)
	}
	return reg, w
}

// D-071 / D-064: the VPP-wide descriptors only for the globals owner, proxy ND only on opt-in; the projection learns
// the same facts; every registered F-neighbors-ra descriptor has a Domains entry.
func TestRegisterNeighborsRa(t *testing.T) {
	t.Setenv(EnvTableBase, "9000")
	t.Setenv(desired.EnvProxyNd, "")
	reg, _ := register(t, coretest.New(), false, nil)
	names := reg.Names()
	for _, n := range []string{neighborsRaRaConfig, neighborsRaRaPrefix, neighborsRaProxyArpIf, neighborsRaProxyRange, neighborsRaNeighbor} {
		if !slices.Contains(names, n) || DomainOf(n) == "" {
			t.Errorf("%s: registered %v, domain %q", n, slices.Contains(names, n), DomainOf(n))
		}
	}
	for _, n := range []string{neighborsRaConfig, neighborsRaDad, neighborsRaProxyNd} {
		if slices.Contains(names, n) {
			t.Errorf("%s registered for a slot agent without opt-in", n)
		}
	}
	if o := desired.NeighborsRaSettings(); o.GlobalsOwner || o.ProxyNd {
		t.Fatalf("projection options %+v", o)
	}
	t.Setenv(desired.EnvProxyNd, "1")
	reg, _ = register(t, coretest.New(), true, nil)
	names = reg.Names()
	for _, n := range []string{neighborsRaConfig, neighborsRaDad, neighborsRaProxyNd} {
		if !slices.Contains(names, n) || DomainOf(n) == "" {
			t.Errorf("%s missing for the globals owner / opt-in", n)
		}
	}
	if o := desired.NeighborsRaSettings(); !o.GlobalsOwner || !o.ProxyNd {
		t.Fatalf("projection options %+v", o)
	}
	t.Setenv(desired.EnvProxyNd, "")
	register(t, coretest.New(), false, nil) // leave the process-wide options at the default for other tests
}

type events struct {
	mu  sync.Mutex
	evs []*vrxv1.Event
}

func (e *events) publish(ev *vrxv1.Event) { e.mu.Lock(); e.evs = append(e.evs, ev); e.mu.Unlock() }
func (e *events) snapshot() []*vrxv1.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*vrxv1.Event(nil), e.evs...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunNeighborWatch(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	own := v.AddInterface("loop901", "Loopback", "w9:loop901")
	v.AddInterface("loop301", "Loopback", "w3:loop301") // another slot's: never subscribed
	var got events
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunNeighborWatch(ctx, NeighborWatchConfig{Client: v, Owner: "w9", Publish: got.publish, Tick: 30 * time.Millisecond, Rescan: 40 * time.Millisecond, PID: 99})
	}()
	waitFor(t, "subscription", func() bool { return slices.Equal(m.Watched(), []uint32{own}) })
	for i := 0; i < 20; i++ { // a burst on one interface
		m.Learn(v, "loop901", "10.9.1."+string(rune('1'+i%9)), "02:00:00:00:09:01", 0)
	}
	m.Learn(v, "loop301", "10.3.1.1", "02:00:00:00:03:01", 0) // not subscribed: VPP sends nothing to us
	m.Forget(v, "loop901", "10.9.1.1")
	waitFor(t, "coalesced event", func() bool { return len(got.snapshot()) > 0 })
	time.Sleep(80 * time.Millisecond)
	evs := got.snapshot()
	var added, removed int
	for _, ev := range evs {
		if ev.GetKind() != vrxv1.EventKind_EVENT_KIND_NEIGHBOR_CHANGED || ev.GetInterface() != "loop901" {
			t.Fatalf("event %v", ev)
		}
		added += atoi(ev.GetAttributes()["added"])
		removed += atoi(ev.GetAttributes()["removed"])
	}
	if added != 20 || removed != 1 || len(evs) > 3 {
		t.Fatalf("%d events, added %d removed %d: %v", len(evs), added, removed, evs)
	}
	// an interface created later is picked up by the rescan
	later := v.AddInterface("loop902", "Loopback", "w9:loop902")
	waitFor(t, "rescan subscription", func() bool { return slices.Equal(m.Watched(), []uint32{own, later}) })
	if m.AllCalls() != 0 {
		t.Fatal("a subscription named every interface")
	}
	cancel()
	<-done
}

// Review L2: the interfaces the resync creates right after Connected are subscribed by an early rescan, not 30 s later.
func TestRunNeighborWatchEarlyRescan(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunNeighborWatch(ctx, NeighborWatchConfig{Client: v, Owner: "w9", Publish: func(*vrxv1.Event) {}, Rescan: time.Hour, Early: []time.Duration{100 * time.Millisecond}, PID: 1})
	}()
	time.Sleep(20 * time.Millisecond) // the first scan found nothing
	idx := v.AddInterface("loop903", "Loopback", "w9:loop903")
	waitFor(t, "early rescan", func() bool { return slices.Equal(m.Watched(), []uint32{idx}) })
	cancel()
	<-done
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Connected starts the watcher only when the A5 event sink is wired (TD-8), and restarts it on every connect.
func TestNeighborsRaConnectedNeedsASink(t *testing.T) {
	v := coretest.New()
	m := coretest.NeighborsRaOf(v)
	v.AddInterface("loop901", "Loopback", "w9:loop901")
	_, w := register(t, v, false, nil)
	w.Connected(context.Background())
	time.Sleep(50 * time.Millisecond)
	if len(v.CallsNamed("want_ip_neighbor_events_v2")) != 0 {
		t.Fatal("subscribed without an event sink")
	}
	var got events
	_, w = register(t, v, false, got.publish)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Connected(ctx)
	waitFor(t, "subscription", func() bool { return len(m.Watched()) == 1 })
	w.Connected(ctx) // reconnect: the old watcher stops before the new one subscribes
	waitFor(t, "resubscription", func() bool { return len(v.CallsNamed("want_ip_neighbor_events_v2")) >= 2 })
	m.Learn(v, "loop901", "10.9.1.7", "02:00:00:00:09:07", 0)
	waitFor(t, "event through the sink", func() bool { return len(got.snapshot()) == 1 })
	cancel()
}
