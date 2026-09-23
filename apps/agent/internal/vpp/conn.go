package vpp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

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

// Invoke implements Client.
func (c *Conn) Invoke(ctx context.Context, req, reply api.Message) error {
	conn, err := c.current()
	if err != nil {
		return err
	}
	return conn.Invoke(ctx, req, reply)
}

// NewStream implements Client.
func (c *Conn) NewStream(ctx context.Context, opts ...api.StreamOption) (api.Stream, error) {
	conn, err := c.current()
	if err != nil {
		return nil, err
	}
	return conn.NewStream(ctx, opts...)
}

// WatchEvent implements Client.
func (c *Conn) WatchEvent(ctx context.Context, event api.Message) (api.Watcher, error) {
	conn, err := c.current()
	if err != nil {
		return nil, err
	}
	return conn.WatchEvent(ctx, event)
}
