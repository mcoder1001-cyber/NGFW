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
	"os"
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
	// "" or "off" disables it.
	MetricsAddr string
	// LogLevel: debug, info, warn, error (VRX_LOG_LEVEL).
	LogLevel string
	// GlobalsOwner (VRX_GLOBALS_OWNER, D-071): true only for the product agent on a real box (the
	// default for the production owner "vrx"); test slots on the shared host are never the owner.
	GlobalsOwner bool
	// IDs is the VPP numeric id range the families may allocate (TD-8, subsystems.ResolveIDScope:
	// VRX_VPP_TABLE_BASE or VRX_VPP_ID_RANGE=all). It fails closed: the zero value (neither variable
	// set, or a Config built in code) owns no id, so a family that allocates ids refuses to register.
	IDs subsystems.IDScope
	// idsErr is a malformed or contradictory id range setting (Validate refuses to start).
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
	ids, idsErr := subsystems.ResolveIDScope()
	if errors.Is(idsErr, subsystems.ErrNoIDRange) {
		idsErr = nil // fail closed without refusing to start: the zero scope owns no id (Start warns)
	}
	return Config{
		IDs:            ids,
		idsErr:         idsErr,
		GlobalsOwner:   globals,
		Socket:         env("VRX_AGENT_SOCKET", "/run/vrx/agent.sock"),
		SocketGroup:    env("VRX_SOCKET_GROUP", "vrx"),
		VPPAPISocket:   env("VRX_AGENT_VPP_API_SOCKET", "/run/vpp/api.sock"),
		VPPStatsSocket: env("VRX_AGENT_VPP_STATS_SOCKET", "/run/vpp/stats.sock"),
		StateDir:       env("VRX_AGENT_STATE_DIR", "/var/lib/vrx/agent"),
		Owner:          owner,
		MetricsAddr:    metrics,
		LogLevel:       env("VRX_LOG_LEVEL", "info"),
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
	}
	return nil
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
	conn := dialVPP(cfg.VPPAPISocket, vpp.ConnOptions{Logger: log})
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
		Events: events, Sources: wiring.DynamicSources()})
	if err != nil {
		conn.Close()
		return nil, err
	}
	a := &Agent{cfg: cfg, log: log, conn: conn, svc: svc, metrics: m, stats: newStatsReader(cfg.VPPStatsSocket, log), wiring: wiring, resyncs: resyncs}

	l, err := listenUnix(cfg.Socket, cfg.SocketGroup, log)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("listen %s: %w", cfg.Socket, err)
	}
	a.grpc = grpc.NewServer()
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
	a.wg.Add(2)
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.resyncs:
			if !connected {
				a.log.Debug("resync request dropped: VPP is not connected (the reconnect resyncs)")
				continue
			}
			a.fullResync(ctx, "requested")
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
				a.wiring.Connected(ctx) // P08: D-080 boot identity for the stores, DF-8 Reconnected()
			}
			a.fullResync(ctx, "connect")
			if !sourcesStarted {
				sourcesStarted = true
				a.startSources(ctx)
			}
			stopLinks()
			lctx, cancelLinks := context.WithCancel(ctx)
			linkCancel = cancelLinks
			go func() {
				if err := watchLinks(lctx, a.conn, a.svc.events(), a.log); err != nil && lctx.Err() == nil {
					a.log.Warn("link events stopped", "err", err)
				}
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
		a.wiring.AfterResync(ctx) // TD-3 Q2: release clean quarantine holders (ifsanitize.Release)
	}
}

// requestResync is the Env.Resync hook (A5, TD-8): it never blocks, and requests coalesce.
func requestResync(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// startSources starts the loops of the dynamic desired sources (S1, TD-8) once, after the first
// resync; they stop when ctx is cancelled (Stop waits for them).
func (a *Agent) startSources(ctx context.Context) {
	for _, src := range a.svc.sources {
		if src.Run == nil {
			continue
		}
		a.log.Info("dynamic source started", "source", src.Name, "descriptors", src.Descriptors)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			src.Run(ctx, a.svc.sourceSync(src.Name))
			a.log.Info("dynamic source stopped", "source", src.Name)
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
