// Command ngfw-agent is the privileged dataplane agent: it owns the VPP binary API and the
// GPL daemons' configuration, and serves the ngfw.v1.Dataplane gRPC API on a unix socket.
// Configuration comes from the environment (internal/agent.ConfigFromEnv):
//
//	NGFW_AGENT_SOCKET            gRPC unix socket            (/run/ngfw/agent.sock)
//	NGFW_SOCKET_GROUP            socket group                (ngfw; primary group if missing)
//	NGFW_OWNER                   owner tag of every object   (ngfw; tests: their NGFW_TEST_PREFIX)
//	NGFW_AGENT_STATE_DIR         desired.pb, owner table     (/var/lib/ngfw/agent)
//	NGFW_AGENT_VPP_API_SOCKET    VPP binary API              (/run/vpp/api.sock)
//	NGFW_AGENT_VPP_STATS_SOCKET  VPP stats segment           (/run/vpp/stats.sock)
//	NGFW_METRICS_ADDR / _PORT    Prometheus                  (127.0.0.1:9101; "off" disables)
//	NGFW_METRICS_ALLOW_REMOTE    1 = a non-loopback NGFW_METRICS_ADDR is meant (/metrics is unauthenticated)
//	NGFW_LOG_LEVEL               debug|info|warn|error       (info; anything else refuses to start)
//	NGFW_AGENT_VPP_REPLY_TIMEOUT bound of one VPP reply      (30 s; seconds or a Go duration; at least 15 s)
//
// An invalid setting refuses to start (exit status 1) instead of running with a default nobody asked for.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ngfw/agent/internal/agent"
)

var version = "dev"

func main() {
	cfg := agent.ConfigFromEnv()
	level, _ := agent.ParseLogLevel(cfg.LogLevel) // an invalid level is refused by Validate below (TD-9)
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	if err := cfg.Validate(); err != nil {
		log.Error("ngfw-agent: invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("ngfw-agent starting", "version", version, "pid", os.Getpid(), "owner", cfg.Owner, "socket", cfg.Socket, "vpp_api", cfg.VPPAPISocket)
	if err := agent.Run(ctx, cfg, version); err != nil {
		log.Error("ngfw-agent exited with error", "err", err)
		os.Exit(1)
	}
}
