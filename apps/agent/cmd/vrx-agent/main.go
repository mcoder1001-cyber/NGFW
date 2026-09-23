// Command vrx-agent is the privileged dataplane agent: it owns the VPP binary API and the
// GPL daemons' configuration, and serves the vrx.v1.Dataplane gRPC API on a unix socket.
// Task P05 fills in the reconciler; this is the process skeleton.
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
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := agent.ConfigFromEnv()
	log.Info("vrx-agent starting", "version", version, "socket", cfg.Socket, "vpp_api", cfg.VPPAPISocket)

	if err := agent.Run(ctx, cfg, version); err != nil {
		log.Error("vrx-agent exited with error", "err", err)
		os.Exit(1)
	}
	log.Info("vrx-agent stopped")
}
