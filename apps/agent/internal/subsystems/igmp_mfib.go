package subsystems

import (
	"context"
	"errors"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"sync"
	"time"
)

func init() {
	Domains[Routing] = append(Domains[Routing], igmp.NameInterface, igmp.NameListen, igmp.NameProxyDevice, igmp.NameDownstream, igmp.NameGroupPrefix, mfib.Name)
}
func (w *Wiring) registerIgmpMfib(r scheduler.Registry) error {
	ids, e := w.IDRange()
	if e != nil && !errors.Is(e, ErrNoIDRange) {
		return e
	}
	opts := []df7.Option{df7.WithIDs(ids.DF7())}
	igmp.Register(r, w.env.Client, w.env.Owner, opts...)
	if w.env.GlobalsOwner {
		igmp.RegisterGlobals(r, w.env.Client, w.env.Owner, opts...)
	}
	desired.ConfigureIgmpGlobals(w.env.GlobalsOwner)
	r.Register(mfib.New(w.env.Client, w.env.Owner, w.BootStore(), opts...))
	w.OnClose(func() {
		if v, ok := igmpWatches.LoadAndDelete(w); ok {
			v.(*igmpWatch).stop()
		}
	})
	return nil
}

type igmpWatch struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var igmpWatches sync.Map

func (i *igmpWatch) stop() { i.mu.Lock(); defer i.mu.Unlock(); i.stopLocked() }
func (i *igmpWatch) stopLocked() {
	if i.cancel != nil {
		i.cancel()
		select {
		case <-i.done:
		case <-time.After(5 * time.Second):
			return
		}
		i.cancel = nil
	}
}

// Start only after resync: the owner interface claims must exist before memberships can be decoded.
func (w *Wiring) igmpMfibAfterResync(ctx context.Context) {
	if w.env.Publish == nil {
		return
	}
	v, _ := igmpWatches.LoadOrStore(w, &igmpWatch{})
	watch := v.(*igmpWatch)
	watch.mu.Lock()
	defer watch.mu.Unlock()
	if watch.cancel != nil {
		select {
		case <-watch.done:
			watch.cancel = nil
		default:
			return
		}
	}
	run, cancel := context.WithCancel(context.WithoutCancel(ctx))
	watch.cancel = cancel
	done := make(chan struct{})
	watch.done = done
	go func() {
		defer close(done)
		for run.Err() == nil {
			events, e := igmp.WatchEvents(run, w.env.Client, w.env.Owner)
			if e == nil {
				for ev := range events {
					if run.Err() == nil {
						w.Publish(IgmpMembershipEvent(ev))
					}
				}
			} else {
				w.env.Log.Debug("IGMP event subscription unavailable", "err", e)
			}
			select {
			case <-run.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
func (w *Wiring) igmpMfibConnected() {
	if v, ok := igmpWatches.Load(w); ok {
		v.(*igmpWatch).stop()
	}
}

// IgmpMembershipEvent encodes a VPP change using the multicast event contract.
func IgmpMembershipEvent(ev igmp.Event) *ngfwv1.Event {
	return &ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_IGMP_GROUP_CHANGED, Interface: &ev.Interface, Attributes: map[string]string{"source": "igmp", "interface": ev.Interface, "group": ev.Group, "sources": ev.Source, "filter": ev.Filter}}
}
