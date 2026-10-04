package subsystems

import (
	"context"
	"go.fd.io/govpp/api"
	"log/slog"
	binigmp "ngfw/agent/binapi/igmp"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
	"sync"
	"testing"
	"time"
)

func TestIgmpMfibRegisteredWithPersistedStores(t *testing.T) {
	reg := scheduler.NewRegistry()
	dir := t.TempDir()
	owned, e := ownertable.Open(dir, "w1")
	if e != nil {
		t.Fatal(e)
	}
	w, e := Register(reg, Env{Client: coretest.New(), Owner: "w1", StateDir: dir, Owned: owned})
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	for _, name := range []string{igmp.NameInterface, igmp.NameListen, igmp.NameProxyDevice, igmp.NameDownstream, mfib.Name} {
		if _, ok := reg.Get(name); !ok {
			t.Fatal("missing", name)
		}
		if DomainOf(name) != Routing {
			t.Fatal("wrong domain", name)
		}
	}
	if _, ok := reg.Get(igmp.NameGroupPrefix); ok {
		t.Fatal("non-owner registered global ranges")
	}
}
func TestIgmpMembershipEventContract(t *testing.T) {
	e := IgmpMembershipEvent(igmp.Event{Interface: "in", Group: "232.1.2.3", Source: "10.1.0.1", Filter: "include"})
	if e.Kind != ngfwv1.EventKind_EVENT_KIND_IGMP_GROUP_CHANGED || e.GetInterface() != "in" || e.Attributes["source"] != "igmp" || e.Attributes["sources"] != "10.1.0.1" || e.Attributes["filter"] != "include" {
		t.Fatalf("event %+v", e)
	}
}

func TestIgmpReconnectWaitsForOldUnsubscribe(t *testing.T) {
	f := fake.New()
	registered := make(chan struct{}, 2)
	cleanupStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var w *Wiring
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		if w != nil {
			if v, ok := igmpWatches.LoadAndDelete(w); ok {
				v.(*igmpWatch).stop()
			}
		}
	})
	f.On("want_igmp_events", func(m api.Message) ([]api.Message, error) {
		req := m.(*binigmp.WantIgmpEvents)
		if req.Enable != 0 {
			registered <- struct{}{}
		} else {
			cleanupStarted <- struct{}{}
			<-release
		}
		return []api.Message{&binigmp.WantIgmpEventsReply{}}, nil
	})
	w = &Wiring{env: Env{Client: f, Owner: "w1", Publish: func(*ngfwv1.Event) {}, Log: slog.Default()}}
	w.igmpMfibAfterResync(context.Background())
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("initial subscribe missing")
	}
	stopped := make(chan struct{})
	go func() { w.igmpMfibConnected(); close(stopped) }()
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup missing")
	}
	select {
	case <-stopped:
		t.Fatal("stop completed before old unsubscribe")
	case <-time.After(20 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not complete")
	}
	w.igmpMfibAfterResync(context.Background())
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("new subscribe missing")
	}
	if v, ok := igmpWatches.LoadAndDelete(w); ok {
		v.(*igmpWatch).stop()
	}
}
