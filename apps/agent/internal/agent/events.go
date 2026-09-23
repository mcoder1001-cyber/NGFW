package agent

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// eventBufferSize bounds each subscriber's queue; on overflow the oldest events are dropped and
// one ERROR event "dropped N events" is delivered before the next one (contract §7).
const eventBufferSize = 256

// bus fans events out to StreamEvents subscribers. Publishing never blocks.
type bus struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

func newBus() *bus { return &bus{subs: map[*subscriber]struct{}{}} }

type subscriber struct {
	kinds      map[vrxv1.EventKind]bool
	interfaces map[string]bool
	size       int

	mu      sync.Mutex
	queue   []*vrxv1.Event
	dropped int
	notify  chan struct{}
	seq     uint64
}

// subscribe registers a subscriber with the request's filters (applied before buffering).
func (b *bus) subscribe(req *vrxv1.StreamEventsRequest) *subscriber {
	s := &subscriber{kinds: map[vrxv1.EventKind]bool{}, interfaces: map[string]bool{}, size: eventBufferSize, notify: make(chan struct{}, 1)}
	for _, k := range req.GetKinds() {
		s.kinds[k] = true
	}
	for _, i := range req.GetInterfaces() {
		s.interfaces[i] = true
	}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

func (b *bus) unsubscribe(s *subscriber) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
}

// publish delivers ev (seq is assigned per stream on delivery).
func (b *bus) publish(ev *vrxv1.Event) {
	if ev.Ts == nil {
		ev.Ts = timestamppb.Now()
	}
	b.mu.Lock()
	subs := make([]*subscriber, 0, len(b.subs))
	for s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		s.offer(ev)
	}
}

func (s *subscriber) wants(ev *vrxv1.Event) bool {
	if len(s.kinds) > 0 && !s.kinds[ev.GetKind()] {
		return false
	}
	if ev.Interface != nil && len(s.interfaces) > 0 && !s.interfaces[ev.GetInterface()] {
		return false
	}
	return true
}

func (s *subscriber) offer(ev *vrxv1.Event) {
	if !s.wants(ev) {
		return
	}
	s.mu.Lock()
	if len(s.queue) >= s.size {
		s.queue = s.queue[1:]
		s.dropped++
	}
	s.queue = append(s.queue, ev)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// next blocks until events are available (or ctx is done) and returns them with seq assigned,
// preceded by an ERROR "dropped N events" when the buffer overflowed.
func (s *subscriber) next(ctx context.Context) ([]*vrxv1.Event, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 || s.dropped > 0 {
			var out []*vrxv1.Event
			if s.dropped > 0 {
				out = append(out, &vrxv1.Event{Ts: timestamppb.Now(), Kind: vrxv1.EventKind_EVENT_KIND_ERROR, Message: fmt.Sprintf("dropped %d events", s.dropped)})
				s.dropped = 0
			}
			for _, ev := range s.queue {
				out = append(out, proto.Clone(ev).(*vrxv1.Event))
			}
			s.queue = nil
			for _, ev := range out {
				s.seq++
				ev.Seq = s.seq
			}
			s.mu.Unlock()
			return out, nil
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.notify:
		}
	}
}

func summaryPB(s summaryCounts) *vrxv1.ApplySummary {
	return &vrxv1.ApplySummary{
		Created: uint32(s.Created), Updated: uint32(s.Updated), Deleted: uint32(s.Deleted), //nolint:gosec // small counts
		Unchanged: uint32(s.Unchanged), Failed: uint32(s.Failed), Reverted: uint32(s.Reverted), //nolint:gosec // small counts
	}
}
