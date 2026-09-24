// Package fake is an in-memory vpp.Client for descriptor unit tests. It records every request
// and serves canned replies (or runs test-supplied handlers) keyed by VPP message name, and
// replays the multipart dump pattern the generated service clients use, so a descriptor can
// be tested unchanged against fake.New(...) and a real connection.
//
//	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
//	f.Reply("create_loopback_instance", &interfaces.CreateLoopbackInstanceReply{SwIfIndex: 5})
//	f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
//		return []api.Message{&interfaces.SwInterfaceDetails{SwIfIndex: 5, Tag: "w2:loop200"}}, nil
//	})
//	d := loopback.New(f, "w2")
//	meta, err := d.Create(ctx, desired)
//	f.CallsNamed("create_loopback_instance") // → the exact request the descriptor sent
//
// Dumps: the generated SwInterfaceDump(ctx, req) sends req and then memclnt.ControlPing on
// one stream and reads until memclnt.ControlPingReply. The fake queues the handler's replies
// for req, then the reply registered for "control_ping" — that reply must be the
// *memclnt.ControlPingReply type of the same binapi package the generated client uses, which
// is why it is passed in (WithControlPingReply) rather than hard-coded.
//
// Unknown requests fail with ErrNoHandler rather than silently succeeding. Handlers may keep
// state in closures to model VPP (see internal/scheduler/example_descriptor_test.go).
package fake

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/vpp"
)

// Errors returned by the fake.
var (
	ErrNoHandler    = errors.New("fake vpp: no handler registered for message")
	ErrNoReply      = errors.New("fake vpp: no queued reply on stream")
	ErrStreamClosed = errors.New("fake vpp: stream closed")
	ErrReplyType    = errors.New("fake vpp: handler reply type does not match")
)

// ControlPing is the VPP message name that ends a multipart dump.
const ControlPing = "control_ping"

// Handler produces the replies for one request. For a request/reply message it returns
// exactly one reply; for a dump request it returns the details messages (possibly none).
type Handler func(req api.Message) ([]api.Message, error)

// Client is the fake vpp.Client. Zero value is not usable; use New.
type Client struct {
	mu        sync.Mutex
	connected bool
	handlers  map[string]Handler
	calls     []api.Message
	watchers  map[string][]*watcher
}

var _ vpp.Client = (*Client)(nil)

// Option configures New.
type Option func(*Client)

// WithControlPingReply registers the reply for "control_ping": pass the
// *memclnt.ControlPingReply of the binapi package your descriptor uses.
func WithControlPingReply(reply api.Message) Option {
	return func(c *Client) { c.Reply(ControlPing, reply) }
}

// WithDisconnected starts the fake in the disconnected state (Connected() == false, every
// call returns vpp.ErrDisconnected) until SetConnected(true).
func WithDisconnected() Option {
	return func(c *Client) { c.connected = false }
}

// New returns a connected fake with no handlers.
func New(opts ...Option) *Client {
	c := &Client{
		connected: true,
		handlers:  make(map[string]Handler),
		watchers:  make(map[string][]*watcher),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// On registers h for requests named name (api.Message.GetMessageName), replacing any earlier
// handler. It returns c for chaining.
func (c *Client) On(name string, h Handler) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[name] = h
	return c
}

// Handles reports whether a handler is registered for requests named name.
func (c *Client) Handles(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.handlers[name]
	return ok
}

// Reply registers fixed replies for name: one message for request/reply calls, the details
// list (zero or more) for dumps. The same messages are returned on every call.
func (c *Client) Reply(name string, replies ...api.Message) *Client {
	return c.On(name, func(api.Message) ([]api.Message, error) { return replies, nil })
}

// Fail makes every request named name return err.
func (c *Client) Fail(name string, err error) *Client {
	return c.On(name, func(api.Message) ([]api.Message, error) { return nil, err })
}

// Calls returns every request received so far (Invoke and stream SendMsg), in order.
func (c *Client) Calls() []api.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]api.Message, len(c.calls))
	copy(out, c.calls)
	return out
}

// CallsNamed returns the requests named name, in order.
func (c *Client) CallsNamed(name string) []api.Message {
	var out []api.Message
	for _, m := range c.Calls() {
		if m.GetMessageName() == name {
			out = append(out, m)
		}
	}
	return out
}

// Reset forgets the recorded calls; handlers and connection state are kept.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = nil
}

// SetConnected flips the connection state.
func (c *Client) SetConnected(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = connected
}

// Connected implements vpp.Client.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// dispatch records req and runs its handler.
func (c *Client) dispatch(ctx context.Context, req api.Message) ([]api.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errors.New("fake vpp: nil request")
	}
	name := req.GetMessageName()
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil, vpp.ErrDisconnected
	}
	c.calls = append(c.calls, req)
	h, ok := c.handlers[name]
	c.mu.Unlock()
	if !ok {
		if name == ControlPing {
			return nil, fmt.Errorf("%w %q (dumps need fake.WithControlPingReply(&memclnt.ControlPingReply{}))", ErrNoHandler, name)
		}
		return nil, fmt.Errorf("%w %q", ErrNoHandler, name)
	}
	return h(req)
}

// Invoke implements vpp.Client: the handler must return exactly one reply of reply's type;
// its value is copied into reply.
func (c *Client) Invoke(ctx context.Context, req, reply api.Message) error {
	replies, err := c.dispatch(ctx, req)
	if err != nil {
		return err
	}
	if len(replies) != 1 {
		return fmt.Errorf("%w: handler for %q returned %d messages, Invoke needs exactly one",
			ErrReplyType, req.GetMessageName(), len(replies))
	}
	return copyMessage(reply, replies[0])
}

// copyMessage copies *src into *dst; both must be pointers to the same struct type.
func copyMessage(dst, src api.Message) error {
	dv, sv := reflect.ValueOf(dst), reflect.ValueOf(src)
	if dv.Kind() != reflect.Pointer || dv.IsNil() || sv.Kind() != reflect.Pointer || sv.IsNil() {
		return fmt.Errorf("%w: reply and canned reply must be non-nil pointers", ErrReplyType)
	}
	if dv.Type() != sv.Type() {
		return fmt.Errorf("%w: canned %T, Invoke expects %T", ErrReplyType, src, dst)
	}
	dv.Elem().Set(sv.Elem())
	return nil
}

// NewStream implements vpp.Client. SendMsg dispatches the request and queues its replies;
// RecvMsg pops them in order. Options are accepted and ignored.
func (c *Client) NewStream(ctx context.Context, _ ...api.StreamOption) (api.Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.Connected() {
		return nil, vpp.ErrDisconnected
	}
	return &stream{ctx: ctx, client: c}, nil
}

type stream struct {
	mu     sync.Mutex
	ctx    context.Context
	client *Client
	queue  []api.Message
	closed bool
}

func (s *stream) Context() context.Context { return s.ctx }

func (s *stream) SendMsg(msg api.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}
	replies, err := s.client.dispatch(s.ctx, msg)
	if err != nil {
		return err
	}
	s.queue = append(s.queue, replies...)
	return nil
}

func (s *stream) RecvMsg() (api.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrStreamClosed
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if len(s.queue) == 0 {
		return nil, ErrNoReply
	}
	msg := s.queue[0]
	s.queue = s.queue[1:]
	return msg, nil
}

func (s *stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.queue = nil
	return nil
}

// WatchEvent implements vpp.Client. Events pushed with Emit whose message name equals
// event.GetMessageName() are delivered to the watcher (buffer 64, dropped when full, like
// govpp). Cancelling ctx or calling Close closes the events channel.
func (c *Client) WatchEvent(ctx context.Context, event api.Message) (api.Watcher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if event == nil {
		return nil, errors.New("fake vpp: nil event type")
	}
	if !c.Connected() {
		return nil, vpp.ErrDisconnected
	}
	w := &watcher{ch: make(chan api.Message, 64), name: event.GetMessageName(), client: c}
	c.mu.Lock()
	c.watchers[w.name] = append(c.watchers[w.name], w)
	c.mu.Unlock()
	context.AfterFunc(ctx, w.Close)
	return w, nil
}

// Emit delivers event to every watcher subscribed to its message name and returns how many
// watchers received it.
func (c *Client) Emit(event api.Message) int {
	c.mu.Lock()
	ws := append([]*watcher(nil), c.watchers[event.GetMessageName()]...)
	c.mu.Unlock()
	n := 0
	for _, w := range ws {
		if w.deliver(event) {
			n++
		}
	}
	return n
}

type watcher struct {
	once   sync.Once
	mu     sync.Mutex
	closed bool
	ch     chan api.Message
	name   string
	client *Client
}

func (w *watcher) Events() <-chan api.Message { return w.ch }

func (w *watcher) deliver(m api.Message) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	select {
	case w.ch <- m:
		return true
	default:
		return false
	}
}

func (w *watcher) Close() {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.ch)
		w.mu.Unlock()
		c := w.client
		c.mu.Lock()
		ws := c.watchers[w.name]
		for i, x := range ws {
			if x == w {
				c.watchers[w.name] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
	})
}
