package subsystems

import (
	"context"
	"errors"
	binbfd "ngfw/agent/binapi/bfd"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/desired"
	_ "ngfw/agent/internal/renderers/frr/bfd"          // Registers BFD profiles and state readers.
	_ "ngfw/agent/internal/renderers/frr/redistribute" // Registers redistribution state reader.
	"ngfw/agent/internal/scheduler"
	"sync"
	"sync/atomic"
	"time"
)

func init() { Domains[Routing] = append(Domains[Routing], bfd.NameAuthKey, bfd.NameSession) }

// BfdIDSpan mirrors the assigned slot range; unscoped production uses the default descriptor range.
func BfdIDSpan() (uint32, uint32) {
	ids, e := SlotIDRange()
	if e != nil {
		return 1, 0
	}
	return ids.Lo, ids.Hi
}

var bfdSecrets sync.Map
var bfdSources sync.Map
var bfdRefs sync.Map
var bfdProjection atomic.Value

// BfdProjection returns the current owner-scoped projection environment.
func BfdProjection() desired.BfdEnvironment {
	if v := bfdProjection.Load(); v != nil {
		return v.(desired.BfdEnvironment)
	}
	first, last := BfdIDSpan()
	return desired.BfdEnvironment{First: first, Last: last}
}

// SetBfdSecretSource installs or removes an owner-scoped sealed-secret resolver.
func SetBfdSecretSource(owner string, source func(string) ([]byte, error)) {
	if source == nil {
		bfdSources.Delete(owner)
	} else {
		bfdSources.Store(owner, source)
	}
}

// SetBfdSecrets installs an owner-scoped ID resolver. The secure channel must resolve key refs before this seam is called.
func SetBfdSecrets(owner string, resolver bfd.Secrets) {
	if resolver == nil {
		bfdSecrets.Delete(owner)
	} else {
		bfdSecrets.Store(owner, resolver)
	}
}
func (w *Wiring) registerBfd(r scheduler.Registry) error {
	ids, e := w.IDRange()
	if e != nil && !errors.Is(e, ErrNoIDRange) {
		return e
	}
	opts := []df7.Option{df7.WithIDs(ids.DF7())}
	first, last := uint32(0), ^uint32(0)
	if ids != nil {
		first, last = ids.Lo, ids.Hi
	}
	claims, err := w.KeyedClaims("bfd-endpoints")
	if err != nil {
		return err
	}
	mh := &bfd.MultihopEnvironment{Claims: &bfdEndpointClaims{store: claims},
		Created:  func(key string) { createBfdObservation(w.env.Owner, key) },
		Deleted:  func(key string) { deleteBfdObservation(w.env.Owner, key) },
		Snapshot: func(keys []string) { pruneBfdObservations(w.env.Owner, keys) },
	}
	if w.env.GlobalsOwner {
		mh.Enable = func(ctx context.Context) error {
			_, err := binbfd.NewServiceClient(w.env.Client).BfdUDPEnableMultihop(ctx, &binbfd.BfdUDPEnableMultihop{})
			return err
		}
	}
	bfd.SetMultihopEnvironment(w.env.Owner, mh)
	refs := &sync.Map{}
	bfdRefs.Store(w.env.Owner, refs)
	bfdProjection.Store(desired.BfdEnvironment{First: first, Last: last, Multihop: w.env.GlobalsOwner, SetRef: func(id uint32, ref string) { refs.Store(id, ref) }, Ref: func(id uint32) string {
		if ref, ok := refs.Load(id); ok {
			return ref.(string)
		}
		return ""
	}})
	bfd.Register(r, w.env.Client, w.env.Owner, func(ctx context.Context, id uint32) ([]byte, error) {
		if source, ok := bfdSources.Load(w.env.Owner); ok {
			if ref, ok := refs.Load(id); ok {
				material, e := source.(func(string) ([]byte, error))(ref.(string))
				if e != nil {
					return nil, bfd.ErrNoSecret
				}
				return material, nil
			}
		}
		v, ok := bfdSecrets.Load(w.env.Owner)
		if !ok {
			return nil, bfd.ErrNoSecret
		}
		return v.(bfd.Secrets)(ctx, id)
	}, opts...)
	if w.env.GlobalsOwner {
		bfd.RegisterGlobals(r, w.env.Client, w.env.Owner, opts...)
	}
	w.OnClose(func() {
		defer func() {
			bfd.SetMultihopEnvironment(w.env.Owner, nil)
			bfdSources.Delete(w.env.Owner)
			bfdSecrets.Delete(w.env.Owner)
			bfdRefs.Delete(w.env.Owner)
			clearBfdObservations(w.env.Owner)
		}()
		if v, ok := bfdWatches.LoadAndDelete(w); ok {
			v.(*bfdWatch).stop()
		}
	})
	return nil
}

// BfdStateEvent exposes a decoded owned session transition without authentication material.
func BfdStateEvent(e bfd.SessionEvent) *ngfwv1.Event {
	return &ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_BFD_STATE_CHANGED, Interface: &e.Interface, Attributes: map[string]string{"source": "bfd", "engine": "vpp", "key": e.Key, "interface": e.Interface, "localAddress": e.Local, "peerAddress": e.Peer, "state": e.State}}
}

type bfdWatch struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var bfdWatches sync.Map

func (i *bfdWatch) stop() { i.mu.Lock(); defer i.mu.Unlock(); i.stopLocked() }
func (i *bfdWatch) stopLocked() {
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
func (w *Wiring) bfdAfterResync(ctx context.Context) {
	if w.env.Publish == nil {
		return
	}
	v, _ := bfdWatches.LoadOrStore(w, &bfdWatch{})
	watch := v.(*bfdWatch)
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
			events, e := bfd.WatchEvents(run, w.env.Client, w.env.Owner)
			if e == nil {
				for ev := range events {
					if run.Err() == nil {
						recordBfdState(w.env.Owner, ev.Key, ev.State)
						w.Publish(BfdStateEvent(ev))
					}
				}
			} else {
				w.env.Log.Debug("BFD event subscription unavailable", "err", e)
			}
			resetBfdObservationHistory(w.env.Owner)
			select {
			case <-run.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
func (w *Wiring) bfdConnected() {
	clearBfdObservations(w.env.Owner)
	if v, ok := bfdWatches.Load(w); ok {
		v.(*bfdWatch).stop()
	}
}

type bfdObservation struct {
	state    string
	lastFlap time.Time
}

var bfdObservations sync.Map
var bfdObservationMu sync.Mutex
var bfdLiveKeys = map[string]map[string]struct{}{}

func recordBfdState(owner, key, state string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	if _, live := bfdLiveKeys[owner][key]; !live {
		return
	}
	id := owner + "\x00" + key
	next := bfdObservation{state: state}
	if value, ok := bfdObservations.Load(id); ok {
		old := value.(bfdObservation)
		next.lastFlap = old.lastFlap
		if old.state != state {
			next.lastFlap = time.Now().UTC()
		}
	}
	bfdObservations.Store(id, next)
}

// BfdLastFlap returns an observed transition time only; initial/restart state is unknown.
func BfdLastFlap(owner, key string) time.Time {
	if value, ok := bfdObservations.Load(owner + "\x00" + key); ok {
		return value.(bfdObservation).lastFlap
	}
	return time.Time{}
}

func createBfdObservation(owner, key string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	if bfdLiveKeys[owner] == nil {
		bfdLiveKeys[owner] = map[string]struct{}{}
	}
	bfdLiveKeys[owner][key] = struct{}{}
	bfdObservations.Delete(owner + "\x00" + key)
}
func deleteBfdObservation(owner, key string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	delete(bfdLiveKeys[owner], key)
	bfdObservations.Delete(owner + "\x00" + key)
}
func pruneBfdObservations(owner string, keys []string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	next := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		next[key] = struct{}{}
	}
	for key := range bfdLiveKeys[owner] {
		if _, found := next[key]; !found {
			bfdObservations.Delete(owner + "\x00" + key)
		}
	}
	bfdLiveKeys[owner] = next
}
func clearBfdObservations(owner string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	for key := range bfdLiveKeys[owner] {
		bfdObservations.Delete(owner + "\x00" + key)
	}
	delete(bfdLiveKeys, owner)
}

func resetBfdObservationHistory(owner string) {
	bfdObservationMu.Lock()
	defer bfdObservationMu.Unlock()
	for key := range bfdLiveKeys[owner] {
		bfdObservations.Delete(owner + "\x00" + key)
	}
}
