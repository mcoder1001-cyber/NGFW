package vpp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

// DefaultReplyTimeout bounds one VPP reply (TD-9, review 1.1): the time VPP may take to answer a request, or to send the
// next message of a dump. govpp's own default is 0, "wait forever": a VPP that crashes or hangs while a request is in
// flight then parks the caller — and with it the agent's transaction lock — for good. The agent overrides it with
// VRX_AGENT_VPP_REPLY_TIMEOUT (ConnOptions.ReplyTimeout).
const DefaultReplyTimeout = 30 * time.Second

// ErrTimeout is returned (wrapped) when VPP did not answer within the reply timeout. It wraps
// context.DeadlineExceeded, so errors.Is(err, context.DeadlineExceeded) holds and its text survives a "%v" in
// a descriptor's error: the request may have reached VPP, so its outcome is unknown (the scheduler reports the
// transaction DEGRADED and the agent owes a resync).
var ErrTimeout = fmt.Errorf("vpp: no reply in time: %w", context.DeadlineExceeded)

// IsTimeout reports whether err means that VPP did not answer in time: ErrTimeout, a deadline, or govpp's
// own reply timeout — matched by text too, for errors a descriptor formatted with %v. Such an outcome is
// never certain (the request may have been applied) and never stored under a txn_id (TD-9 fix round 1).
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, core.ErrReplyTimeout) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, context.DeadlineExceeded.Error()) || strings.Contains(msg, core.ErrReplyTimeout.Error())
}

// govpp's health-check defaults (250 ms reply timeout, 2 misses) disconnect on hypervisor jitter: on vrx-a an idle VPP
// answers a control ping in 0.4 ms median but spikes to ~300 ms with nothing else running (ESXi memory reclaim, P08 review
// I6, D-108), and every disconnect forces a full reconnect + resync. 2 s × 5 misses still reports a dead VPP within ~10 s.
// core.DefaultReplyTimeout is the backstop for any govpp path that does not go through Conn (TD-9).
func init() {
	core.HealthCheckReplyTimeout = 2 * time.Second
	core.HealthCheckThreshold = 5
	core.DefaultReplyTimeout = DefaultReplyTimeout
}

// ConnState is a change of the binary-API connection state reported by Conn.States.
type ConnState struct {
	Connected bool
	At        time.Time
	Err       error
}

// ConnOptions tunes the connection manager. Zero values mean the defaults.
type ConnOptions struct {
	// Attempts per govpp connect round before the manager backs off (default 5).
	Attempts int
	// Interval between attempts inside a round (default 1 s).
	Interval time.Duration
	// MinBackoff / MaxBackoff bound the exponential backoff between rounds (default 1 s / 30 s).
	MinBackoff, MaxBackoff time.Duration
	// Logger (default slog.Default()).
	Logger *slog.Logger
	// ReplyTimeout bounds each VPP reply (default DefaultReplyTimeout): an Invoke's whole round trip, and every
	// message of a stream (a stream's own core.WithReplyTimeout option overrides it for a known-slow dump).
	ReplyTimeout time.Duration
}

// Conn is the production vpp.Client: a govpp connection over the binary-API socket with
// asynchronous connect, health checking (govpp's control-ping probe), reconnect with exponential
// backoff, and state notifications. While disconnected every call fails fast with
// ErrDisconnected instead of blocking.
type Conn struct {
	path string
	opts ConnOptions
	log  *slog.Logger

	mu        sync.RWMutex
	conn      *core.Connection
	connected atomic.Bool

	states chan ConnState
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

var _ Client = (*Conn)(nil)

// Dial starts connecting to the VPP binary API socket at path in the background and returns
// immediately. Watch States (or poll Connected) to learn when the connection is up.
func Dial(path string, opts ConnOptions) *Conn {
	if opts.Attempts <= 0 {
		opts.Attempts = 5
	}
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.MinBackoff <= 0 {
		opts.MinBackoff = time.Second
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 30 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ReplyTimeout <= 0 {
		opts.ReplyTimeout = DefaultReplyTimeout
	}
	c := &Conn{
		path:   path,
		opts:   opts,
		log:    opts.Logger.With("component", "vpp"),
		states: make(chan ConnState, 16),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go c.loop()
	return c
}

// States delivers every connection state change (buffered; the oldest change is dropped when the
// consumer lags — Connected() is always current).
func (c *Conn) States() <-chan ConnState { return c.states }

func (c *Conn) emit(s ConnState) {
	for {
		select {
		case c.states <- s:
			return
		default:
			select {
			case <-c.states:
			default:
			}
		}
	}
}

func (c *Conn) setConnected(v bool, err error) {
	if c.connected.Swap(v) == v {
		return
	}
	if v {
		c.log.Info("VPP binary API connected", "socket", c.path)
	} else {
		c.log.Warn("VPP binary API disconnected", "socket", c.path, "err", err)
	}
	c.emit(ConnState{Connected: v, At: time.Now(), Err: err})
}

// loop runs govpp connect rounds forever: AsyncConnect retries Attempts times Interval apart and
// then reports Failed; the manager then backs off exponentially and starts a new round.
func (c *Conn) loop() {
	defer close(c.done)
	backoff := c.opts.MinBackoff
	for {
		adapter := socketclient.NewVppClient(c.path)
		adapter.SetClientName("vrx-agent")
		conn, events, err := core.AsyncConnect(adapter, c.opts.Attempts, c.opts.Interval)
		if err != nil {
			c.log.Warn("govpp async connect", "err", err)
		} else {
			c.mu.Lock()
			c.conn = conn
			c.mu.Unlock()
			failed := c.watch(events)
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			if !failed {
				// stopped: govpp's Disconnect waits for its connect loop, which may sit in
				// WaitReady for seconds when the socket is missing — do not block Close on it.
				c.connected.Store(false)
				go conn.Disconnect()
				return
			}
			c.setConnected(false, errors.New("connect round failed"))
			conn.Disconnect()
		}
		select {
		case <-c.stop:
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > c.opts.MaxBackoff {
			backoff = c.opts.MaxBackoff
		}
	}
}

// watch follows one govpp connection's events. It returns true when govpp gave up (Failed) and
// false when Close was called.
func (c *Conn) watch(events <-chan core.ConnectionEvent) bool {
	for {
		select {
		case <-c.stop:
			return false
		case ev, ok := <-events:
			if !ok {
				return true
			}
			switch ev.State {
			case core.Connected:
				c.setConnected(true, nil)
			case core.Disconnected, core.NotResponding:
				c.setConnected(false, ev.Error)
			case core.Failed:
				c.log.Warn("VPP connect round failed; backing off", "err", ev.Error)
				return true
			}
		}
	}
}

// Close stops reconnecting and disconnects.
func (c *Conn) Close() {
	c.once.Do(func() { close(c.stop) })
	<-c.done
}

// WaitConnected blocks until connected or ctx is done.
func (c *Conn) WaitConnected(ctx context.Context) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for !c.Connected() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for VPP at %s: %w", c.path, ctx.Err())
		case <-t.C:
		}
	}
	return nil
}

// Connected implements Client.
func (c *Conn) Connected() bool { return c.connected.Load() }

func (c *Conn) current() (*core.Connection, error) {
	if !c.connected.Load() {
		return nil, ErrDisconnected
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return nil, ErrDisconnected
	}
	return c.conn, nil
}

// Invoke implements Client. The round trip is bounded by ReplyTimeout (TD-9): a caller's earlier deadline
// wins, a ctx without one gets it. A missed deadline is ErrTimeout. It is govpp's Connection.Invoke on a
// stream of this Conn's own reply timeout: govpp's Invoke uses the process-wide core.DefaultReplyTimeout,
// which would cut a larger VRX_AGENT_VPP_REPLY_TIMEOUT short (TD-9 review M1).
func (c *Conn) Invoke(ctx context.Context, req, reply api.Message) error {
	conn, err := c.current()
	if err != nil {
		return err
	}
	return invokeWithin(ctx, c.opts.ReplyTimeout, req, reply, func(ctx context.Context, req, reply api.Message) error {
		return invokeStream(ctx, conn, c.opts.ReplyTimeout, req, reply)
	})
}

// invokeStream is one request/reply round trip on a stream whose reply timeout is timeout.
func invokeStream(ctx context.Context, conn *core.Connection, timeout time.Duration, req, reply api.Message) error {
	st, err := conn.NewStream(ctx, core.WithReplyTimeout(timeout))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.SendMsg(req); err != nil {
		return err
	}
	m, err := st.RecvMsg()
	if err != nil {
		return err
	}
	dv, sv := reflect.ValueOf(reply), reflect.ValueOf(m)
	if dv.Kind() != reflect.Pointer || dv.IsNil() || dv.Type() != sv.Type() {
		return fmt.Errorf("vpp: %s: VPP answered %T, want %T", req.GetMessageName(), m, reply)
	}
	dv.Elem().Set(sv.Elem())
	return nil
}

// invokeWithin runs one request/reply round trip with its ctx bounded by timeout and turns the bound's own
// expiry — or govpp's own reply timeout — into ErrTimeout (a caller's cancel or deadline stays ctx.Err()).
func invokeWithin(ctx context.Context, timeout time.Duration, req, reply api.Message, invoke func(context.Context, api.Message, api.Message) error) error {
	if timeout <= 0 {
		return invoke(ctx, req, reply)
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := invoke(tctx, req, reply)
	switch {
	case err == nil || ctx.Err() != nil:
		return err
	case tctx.Err() != nil:
		return fmt.Errorf("%w: %s: %s", ErrTimeout, req.GetMessageName(), timeout)
	case errors.Is(err, core.ErrReplyTimeout):
		return fmt.Errorf("%w: %s: %w", ErrTimeout, req.GetMessageName(), err)
	}
	return err
}

// NewStream implements Client. Every message of the stream is bounded by ReplyTimeout
// (core.WithReplyTimeout, prepended: an option of the caller overrides it); a missed reply is ErrTimeout.
func (c *Conn) NewStream(ctx context.Context, opts ...api.StreamOption) (api.Stream, error) {
	conn, err := c.current()
	if err != nil {
		return nil, err
	}
	st, err := conn.NewStream(ctx, append([]api.StreamOption{core.WithReplyTimeout(c.opts.ReplyTimeout)}, opts...)...)
	if err != nil {
		return nil, err
	}
	return timeoutStream{st}, nil
}

// timeoutStream reports govpp's per-reply timeout as ErrTimeout.
type timeoutStream struct{ api.Stream }

func (s timeoutStream) RecvMsg() (api.Message, error) {
	m, err := s.Stream.RecvMsg()
	if errors.Is(err, core.ErrReplyTimeout) {
		err = fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return m, err
}

// Bounded returns c with every Invoke bounded by timeout exactly as Conn bounds its own (TD-9): for a Client
// that is not a Conn — the unit tests' fake VPPs prove the agent's behaviour when VPP never replies with it.
// Streams pass through unchanged (a Conn bounds each of their messages with govpp's per-reply timer).
func Bounded(c Client, timeout time.Duration) Client { return bounded{Client: c, timeout: timeout} }

type bounded struct {
	Client
	timeout time.Duration
}

func (b bounded) Invoke(ctx context.Context, req, reply api.Message) error {
	return invokeWithin(ctx, b.timeout, req, reply, b.Client.Invoke)
}

// WatchEvent implements Client.
func (c *Conn) WatchEvent(ctx context.Context, event api.Message) (api.Watcher, error) {
	conn, err := c.current()
	if err != nil {
		return nil, err
	}
	return conn.WatchEvent(ctx, event)
}
