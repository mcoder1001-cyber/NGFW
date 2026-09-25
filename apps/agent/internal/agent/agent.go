// Package agent wires the vrx-agent process together: configuration, the VPP connection,
// the reconciler with the core descriptors, the persisted desired state (resync on start and on
// VPP reconnect, confirm timer), the vrx.v1.Dataplane gRPC server on a unix socket, and the
// Prometheus metrics endpoint.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// Config is read once from environment variables (systemd EnvironmentFile).
type Config struct {
	// Socket is the unix socket the gRPC server listens on (VRX_AGENT_SOCKET).
	Socket string
	// SocketGroup owns the socket (VRX_SOCKET_GROUP); missing group → primary group + warning.
	SocketGroup string
	// VPPAPISocket is VPP's binary API socket (VRX_AGENT_VPP_API_SOCKET).
	VPPAPISocket string
	// VPPStatsSocket is VPP's stats segment socket (VRX_AGENT_VPP_STATS_SOCKET).
	VPPStatsSocket string
	// StateDir holds desired.pb & co (VRX_AGENT_STATE_DIR).
	StateDir string
	// Owner stamped on every object (VRX_OWNER; tests: their VRX_TEST_PREFIX).
	Owner string
	// MetricsAddr is the Prometheus listen address (VRX_METRICS_ADDR, or 127.0.0.1:$VRX_METRICS_PORT);
	// "" or "off" disables it. /metrics is unauthenticated: a non-loopback address needs MetricsAllowRemote.
	MetricsAddr string
	// MetricsAllowRemote (VRX_METRICS_ALLOW_REMOTE=1) permits a non-loopback MetricsAddr (TD-9, review 1.5e).
	MetricsAllowRemote bool
	// LogLevel: debug, info, warn, error (VRX_LOG_LEVEL); anything else refuses to start (TD-9, review 1.5c).
	LogLevel string
	// VPPReplyTimeout bounds each VPP reply (VRX_AGENT_VPP_REPLY_TIMEOUT: seconds or a Go duration;
	// 0 = vpp.DefaultReplyTimeout, 30 s; TD-9, review 1.1).
	VPPReplyTimeout time.Duration
	// replyErr is a malformed VRX_AGENT_VPP_REPLY_TIMEOUT (Validate refuses to start).
	replyErr error
	// GlobalsOwner (VRX_GLOBALS_OWNER, D-071): true only for the product agent on a real box (the
	// default for the production owner "vrx"); test slots on the shared host are never the owner.
	GlobalsOwner bool
	// IDs is the VPP numeric id range the families may allocate (TD-8, subsystems.ResolveIDScope:
	// VRX_VPP_TABLE_BASE or VRX_VPP_ID_RANGE=all). ConfigFromEnv without either refuses to start
	// (TD-8b, D-129 Q3); the zero value of a Config built in code owns no id (fail closed), so a family
	// that allocates ids refuses to register.
	IDs subsystems.IDScope
	// idsErr is a missing, malformed or contradictory id range setting (Validate refuses to start).
	idsErr error
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv returns the configuration with production defaults.
func ConfigFromEnv() Config {
	metrics := env("VRX_METRICS_ADDR", "")
	if metrics == "" {
		metrics = "127.0.0.1:" + env("VRX_METRICS_PORT", "9101")
	}
	owner := env("VRX_OWNER", "vrx")
	globals := owner == "vrx"
	switch strings.ToLower(os.Getenv("VRX_GLOBALS_OWNER")) {
	case "1", "true", "yes":
		globals = true
	case "0", "false", "no":
		globals = false
	}
	ids, idsErr := subsystems.ResolveIDScope() // none set: ErrNoIDRange, start-up refused (TD-8b, D-129 Q3)
	reply, replyErr := parseTimeout(os.Getenv("VRX_AGENT_VPP_REPLY_TIMEOUT"))
	if replyErr != nil {
		replyErr = fmt.Errorf("invalid VRX_AGENT_VPP_REPLY_TIMEOUT: %w", replyErr)
	}
	return Config{
		IDs:                ids,
		idsErr:             idsErr,
		VPPReplyTimeout:    reply,
		replyErr:           replyErr,
		MetricsAllowRemote: os.Getenv("VRX_METRICS_ALLOW_REMOTE") == "1",
		GlobalsOwner:       globals,
		Socket:             env("VRX_AGENT_SOCKET", "/run/vrx/agent.sock"),
		SocketGroup:        env("VRX_SOCKET_GROUP", "vrx"),
		VPPAPISocket:       env("VRX_AGENT_VPP_API_SOCKET", "/run/vpp/api.sock"),
		VPPStatsSocket:     env("VRX_AGENT_VPP_STATS_SOCKET", "/run/vpp/stats.sock"),
		StateDir:           env("VRX_AGENT_STATE_DIR", "/var/lib/vrx/agent"),
		Owner:              owner,
		MetricsAddr:        metrics,
		LogLevel:           env("VRX_LOG_LEVEL", "info"),
	}
}

// Validate checks the configuration.
func (c Config) Validate() error {
	switch {
	case c.Owner == "" || strings.ContainsAny(c.Owner, ":/\\ \x00\r\n"):
		return fmt.Errorf("invalid VRX_OWNER %q", c.Owner)
	case c.Socket == "":
		return errors.New("VRX_AGENT_SOCKET is empty")
	case c.StateDir == "":
		return errors.New("VRX_AGENT_STATE_DIR is empty")
	case c.idsErr != nil:
		return c.idsErr
	case c.replyErr != nil:
		return c.replyErr
	case c.VPPReplyTimeout < 0 || (c.VPPReplyTimeout > 0 && c.VPPReplyTimeout < MinVPPReplyTimeout):
		return fmt.Errorf("invalid VRX_AGENT_VPP_REPLY_TIMEOUT %s: at least %s (govpp's health-check window)", c.VPPReplyTimeout, MinVPPReplyTimeout)
	}
	if _, err := ParseLogLevel(c.LogLevel); err != nil {
		return err
	}
	return c.checkMetricsAddr()
}

// MinVPPReplyTimeout is the least VRX_AGENT_VPP_REPLY_TIMEOUT (review L6): govpp's health check (a probe
// every 1 s, 2 s reply timeout, 5 misses — vpp/conn.go) reconnects a dead or hung VPP within about 15 s,
// which drops late replies. A shorter reply timeout would return a channel id to govpp's pool while VPP may
// still answer on it, and govpp's Invoke does not check which message a reply answers.
const MinVPPReplyTimeout = 15 * time.Second

// ParseLogLevel parses VRX_LOG_LEVEL (debug, info, warn, error; "" = info). An unknown level is an
// error: the agent refuses to start rather than run at a level nobody asked for (TD-9, review 1.5c).
func ParseLogLevel(s string) (slog.Level, error) {
	level := slog.LevelInfo
	if s == "" {
		return level, nil
	}
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo, fmt.Errorf("invalid VRX_LOG_LEVEL %q (debug, info, warn or error)", s)
	}
	return level, nil
}

// checkMetricsAddr refuses a /metrics address other than loopback — the endpoint has no
// authentication — unless VRX_METRICS_ALLOW_REMOTE=1 says it is meant (TD-9, review 1.5e).
func (c Config) checkMetricsAddr() error {
	if c.MetricsAddr == "" || c.MetricsAddr == "off" {
		return nil
	}
	host, _, err := net.SplitHostPort(c.MetricsAddr)
	if err != nil {
		return fmt.Errorf("invalid VRX_METRICS_ADDR %q: %w", c.MetricsAddr, err)
	}
	if c.MetricsAllowRemote || host == "localhost" {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("VRX_METRICS_ADDR %q is not a loopback address: /metrics is unauthenticated, set VRX_METRICS_ALLOW_REMOTE=1 to serve it there anyway", c.MetricsAddr)
}

// parseTimeout parses seconds ("30") or a Go duration ("1m30s"); "" = 0 (the default).
func parseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		if n == 0 {
			return 0, errors.New("must be positive")
		}
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, errors.New("must be positive")
	}
	return d, nil
}

// vppConn is what the agent needs from the VPP connection manager (vpp.Conn; fakes in tests).
type vppConn interface {
	vpp.Client
	States() <-chan vpp.ConnState
	Close()
}

// dialVPP opens the VPP connection manager (vpp.Dial); the unit tests of Start's seam wiring (TD-8)
// substitute a fake VPP.
var dialVPP = func(socket string, opts vpp.ConnOptions) vppConn { return vpp.Dial(socket, opts) }

// Agent is a running agent (Start/Stop form, used by Run and by in-process tests).
type Agent struct {
	cfg     Config
	log     *slog.Logger
	conn    vppConn
	svc     *Service
	grpc    *grpc.Server
	metrics *metrics
	httpSrv *http.Server
	stats   *statsReader
	wiring  *subsystems.Wiring
	// resyncs carries Env.Resync requests (A5, TD-8) to watchVPP; nil = none (tests).
	resyncs chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// Start brings the agent up: state, VPP connection (background), gRPC and metrics listeners.
// The first resync runs as soon as VPP is connected.
func Start(ctx context.Context, cfg Config, version string, log *slog.Logger) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("owner", cfg.Owner)
	owned, err := ownertable.Open(cfg.StateDir, cfg.Owner)
	if err != nil {
		return nil, err
	}
	m := newMetrics()
	conn := dialVPP(cfg.VPPAPISocket, vpp.ConnOptions{Logger: log, ReplyTimeout: cfg.VPPReplyTimeout})
	reg := scheduler.NewRegistry()
	// A5 seams (TD-8): the features' events reach the service's bus, their resync requests watchVPP.
	events, resyncs := newBus(), make(chan struct{}, 1)
	wiring, err := subsystems.Register(reg, subsystems.Env{Client: conn, Owner: cfg.Owner, StateDir: cfg.StateDir, Owned: owned, GlobalsOwner: cfg.GlobalsOwner, Log: log.With("component", "subsystems"),
		Publish: events.publishFeature, Resync: func() { requestResync(resyncs) }, IDs: cfg.IDs})
	if err != nil {
		conn.Close()
		return nil, err
	}
	log.Info("subsystems wired", "domains", implementedDomains(), "wiring", wiring.String(), "vpp_ids", cfg.IDs.String())
	if cfg.IDs == (subsystems.IDScope{}) {
		log.Warn("no VPP id range: families that allocate numeric ids refuse to register", "why", subsystems.ErrNoIDRange)
	}
	m.collectors = wiring.MetricsCollectors // TD-8: feature metric families on /metrics
	sched := scheduler.New(reg, log.With("component", "scheduler"))
	svc, err := NewService(ServiceConfig{Owner: cfg.Owner, Version: version, Logger: log, VPP: conn, Scheduler: sched, StateDir: cfg.StateDir, Metrics: m, BeforeTxn: wiring.BeforeTxn, NetdevKind: wiring.NetdevKind(),
		Events: events, Sources: wiring.DynamicSources(),
		RequestResync: func() { requestResync(resyncs) }}) // TD-9: the owed resync takes the Env.Resync path
	if err != nil {
		conn.Close()
		return nil, err
	}
	a := &Agent{cfg: cfg, log: log, conn: conn, svc: svc, metrics: m, stats: newStatsReader(cfg.VPPStatsSocket, log), wiring: wiring, resyncs: resyncs}

	svc.claimsTxn = wiring.ClaimsTxn // TD-11c (review 3.2): keyed claim stores write once per transaction
	l, err := listenUnix(cfg.Socket, cfg.SocketGroup, log)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("listen %s: %w", cfg.Socket, err)
	}
	a.grpc = newGRPCServer(log, m) // TD-9: panic recovery interceptors
	vrxv1.RegisterDataplaneServer(a.grpc, &server{svc: svc, stats: a.stats, log: log})

	if cfg.MetricsAddr != "" && cfg.MetricsAddr != "off" {
		ml, err := net.Listen("tcp", cfg.MetricsAddr)
		if err != nil {
			_ = l.Close()
			conn.Close()
			return nil, fmt.Errorf("metrics listen %s: %w", cfg.MetricsAddr, err)
		}
		a.httpSrv = &http.Server{Handler: m.handler(), ReadHeaderTimeout: 5 * time.Second}
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.httpSrv.Serve(ml); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("metrics server", "err", err)
			}
		}()
	}

	rctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.wg.Add(3)
	go func() {
		defer a.wg.Done()
		if err := a.grpc.Serve(l); err != nil {
			log.Error("grpc server", "err", err)
		}
	}()
	go func() {
		defer a.wg.Done()
		a.watchVPP(rctx)
	}()
	go func() {
		defer a.wg.Done()
		a.watchDrift(rctx)
	}()
	log.Info("vrx-agent listening", "socket", cfg.Socket, "metrics", cfg.MetricsAddr, "state_dir", cfg.StateDir, "vpp_api", cfg.VPPAPISocket)
	return a, nil
}

// watchVPP reacts to connection changes: VPP_CONNECTED → version, resync, link events;
// VPP_DISCONNECTED → event. It also serves Env.Resync requests while VPP is connected and starts the
// dynamic sources' loops after the first resync (TD-8).
func (a *Agent) watchVPP(ctx context.Context) {
	var linkCancel context.CancelFunc
	stopLinks := func() {
		if linkCancel != nil {
			linkCancel()
			linkCancel = nil
		}
	}
	defer stopLinks()
	connected, sourcesStarted := false, false
	// Env.Resync storm guard (TD-8 review R8): a request within resyncMinInterval of the last requested
	// resync is deferred to the end of the interval, where every request made meanwhile coalesces.
	var lastRequested time.Time
	var deferred <-chan time.Time
	requested := func() {
		if !connected || !a.conn.Connected() {
			a.log.Debug("resync request dropped: VPP is not connected (the reconnect resyncs)")
			return
		}
		a.fullResync(ctx, "requested")
		lastRequested = time.Now()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-deferred:
			deferred = nil
			requested()
		case <-a.resyncs:
			if deferred != nil {
				continue // already deferred: coalesced
			}
			if wait := resyncMinInterval - time.Since(lastRequested); !lastRequested.IsZero() && wait > 0 {
				a.log.Warn("resync requested again right after a requested resync: deferred", "in", wait)
				deferred = time.After(wait)
				continue
			}
			requested()
		case st := <-a.conn.States():
			a.metrics.setVPP(st.Connected)
			connected = st.Connected
			if !st.Connected {
				stopLinks()
				msg := "VPP binary API disconnected"
				if st.Err != nil {
					msg += ": " + st.Err.Error()
				}
				a.svc.events().publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_VPP_DISCONNECTED, Message: msg})
				continue
			}
			vctx, vcancel := context.WithTimeout(ctx, 5*time.Second)
			v, err := vppVersion(vctx, a.conn)
			vcancel()
			if err != nil {
				a.log.Warn("show_version", "err", err)
			}
			a.svc.SetVPPVersion(v)
			a.svc.events().publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_VPP_CONNECTED, Message: "VPP " + v})
			if a.wiring != nil {
				// P08: D-080 boot identity for the stores, DF-8 Reconnected(); its ControlPing gets a
				// deadline of its own (TD-9)
				a.safely("wiring connect hook", func() {
					cctx, ccancel := context.WithTimeout(ctx, connectHookTimeout)
					defer ccancel()
					a.wiring.Connected(cctx)
				})
			}
			a.fullResync(ctx, "connect")
			if !sourcesStarted {
				sourcesStarted = true
				a.startSources(ctx)
			}
			stopLinks()
			lctx, cancelLinks := context.WithCancel(ctx)
			linkCancel = cancelLinks
			a.wg.Add(1)
			go func() {
				defer a.wg.Done()
				a.runLinks(lctx)
			}()
		}
	}
}

// fullResync re-applies the stored desired state (Service.Resync, with the dynamic sources) and runs
// the wiring's after-resync hook.
func (a *Agent) fullResync(ctx context.Context, why string) {
	resp := a.svc.Resync(ctx)
	if resp != nil {
		a.log.Info("resync finished", "why", why, "status", resp.GetStatus().String(), "summary", resp.GetSummary().String())
	}
	if a.wiring != nil {
		a.safely("wiring after-resync hook", func() { a.wiring.AfterResync(ctx) }) // TD-3 Q2: release clean quarantine holders (ifsanitize.Release)
	}
}

// connectHookTimeout bounds the wiring's connect hook (its boot-identity ControlPing, TD-9; a var for
// the tests).
var connectHookTimeout = 30 * time.Second

// safely runs a wiring hook or the link watcher on a goroutine of the agent's own: a panic there is
// logged and counted instead of taking the agent down (TD-9, review 1.1d).
func (a *Agent) safely(what string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			a.log.Error("panic recovered", "in", what, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			if a.metrics != nil {
				a.metrics.panicked("hook") // review L2: not a transaction panic
			}
		}
	}()
	fn()
}

// Link-event watcher restart backoff (TD-9, review 1.1c; vars for the tests).
var (
	linkRetryFloor = time.Second
	linkRetryMax   = 30 * time.Second
)

// runLinks keeps the link-event watcher running while VPP stays connected (review 1.1c): after an
// error it restarts with backoff (linkRetryFloor doubling to linkRetryMax, reset after a watch that
// lasted longer than linkRetryMax). It ends with ctx (a disconnect or the agent's stop).
func (a *Agent) runLinks(ctx context.Context) {
	delay := linkRetryFloor
	for {
		start := time.Now()
		var err error
		a.safely("link events", func() { err = watchLinks(ctx, a.conn, a.svc.events(), a.log) })
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > linkRetryMax {
			delay = linkRetryFloor
		}
		a.log.Warn("link events stopped; restarting", "err", err, "in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay *= 2; delay > linkRetryMax {
			delay = linkRetryMax
		}
	}
}

// driftInterval is the period of the drift check (TD-9, review 1.1b; a var for the tests).
var driftInterval = 5 * time.Minute

// watchDrift runs the Plan-only drift check every driftInterval until ctx ends.
func (a *Agent) watchDrift(ctx context.Context) {
	t := time.NewTicker(driftInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.svc.CheckDrift(ctx)
		}
	}
}

// resyncMinInterval is the least time between two requested resyncs (a var for the tests).
var resyncMinInterval = 5 * time.Second

// requestResync is the Env.Resync hook (A5, TD-8): it never blocks, and requests coalesce.
func requestResync(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// startSources starts the loops of the dynamic desired sources (S1, TD-8) once, after the first
// resync; they stop when ctx is cancelled (Stop waits for them). A source without Run gets one sync
// here instead (the agent retries it while it fails). A Run that panics or returns before ctx is done
// stops its source until the agent restarts: out of sync, its objects left as they are (review R3).
func (a *Agent) startSources(ctx context.Context) {
	for _, ds := range a.svc.sources {
		if ds.Run == nil {
			if err := a.svc.syncSource(ctx, ds.Name); err != nil {
				a.log.Warn("dynamic source: first sync failed (retried)", "source", ds.Name, "err", err)
			}
			continue
		}
		a.log.Info("dynamic source started", "source", ds.Name, "descriptors", ds.Descriptors)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					a.log.Error("dynamic source: Run panicked", "source", ds.Name, "panic", r, "stack", string(debug.Stack()))
					a.svc.sourceStopped(ds, srcPanic, fmt.Sprintf("Run panicked: %v", r))
				}
			}()
			ds.Run(ctx, a.svc.sourceSync(ds.Name))
			if ctx.Err() == nil {
				a.svc.sourceStopped(ds, srcStopped, "Run returned before the agent stopped")
				return
			}
			a.log.Info("dynamic source stopped", "source", ds.Name)
		}()
	}
}

// Service returns the agent's service (tests).
func (a *Agent) Service() *Service { return a.svc }

// Stop shuts the agent down gracefully (the persisted state and any pending confirm deadline
// survive; the next start resumes them).
func (a *Agent) Stop() {
	a.cancel()
	// Streams (StreamEvents/StreamStats) only end when the client cancels; give unary calls a
	// moment to finish, then cut the streams (clients see UNAVAILABLE).
	done := make(chan struct{})
	go func() { a.grpc.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		a.grpc.Stop()
		<-done
	}
	if a.httpSrv != nil {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.httpSrv.Shutdown(sctx)
		cancel()
	}
	a.wg.Wait()
	a.svc.Close()
	if a.wiring != nil {
		a.wiring.Close() // families' background work (subsystems close seam, Wiring.OnClose)
	}
	a.stats.close()
	a.conn.Close()
	_ = os.Remove(a.cfg.Socket)
	a.log.Info("vrx-agent stopped")
}

// Run starts the agent and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config, version string) error {
	a, err := Start(ctx, cfg, version, slog.Default())
	if err != nil {
		return err
	}
	<-ctx.Done()
	a.Stop()
	return nil
}
