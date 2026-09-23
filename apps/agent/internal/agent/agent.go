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
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
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
	return Config{
		Socket:         env("VRX_AGENT_SOCKET", "/run/vrx/agent.sock"),
		SocketGroup:    env("VRX_SOCKET_GROUP", "vrx"),
		VPPAPISocket:   env("VRX_AGENT_VPP_API_SOCKET", "/run/vpp/api.sock"),
		VPPStatsSocket: env("VRX_AGENT_VPP_STATS_SOCKET", "/run/vpp/stats.sock"),
		StateDir:       env("VRX_AGENT_STATE_DIR", "/var/lib/vrx/agent"),
		Owner:          env("VRX_OWNER", "vrx"),
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
	}
	return nil
}

// vppConn is what the agent needs from the VPP connection manager (vpp.Conn; fakes in tests).
type vppConn interface {
	vpp.Client
	States() <-chan vpp.ConnState
	Close()
}

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
	conn := vpp.Dial(cfg.VPPAPISocket, vpp.ConnOptions{Logger: log})
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: conn, Owner: cfg.Owner, Owned: owned})
	sched := scheduler.New(reg, log.With("component", "scheduler"))
	svc, err := NewService(ServiceConfig{Owner: cfg.Owner, Version: version, Logger: log, VPP: conn, Scheduler: sched, StateDir: cfg.StateDir, Metrics: m})
	if err != nil {
		conn.Close()
		return nil, err
	}
	a := &Agent{cfg: cfg, log: log, conn: conn, svc: svc, metrics: m, stats: newStatsReader(cfg.VPPStatsSocket, log)}

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
// VPP_DISCONNECTED → event.
func (a *Agent) watchVPP(ctx context.Context) {
	var linkCancel context.CancelFunc
	stopLinks := func() {
		if linkCancel != nil {
			linkCancel()
			linkCancel = nil
		}
	}
	defer stopLinks()
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-a.conn.States():
			a.metrics.setVPP(st.Connected)
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
			resp := a.svc.Resync(ctx)
			if resp != nil {
				a.log.Info("resync finished", "status", resp.GetStatus().String(), "summary", resp.GetSummary().String())
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
