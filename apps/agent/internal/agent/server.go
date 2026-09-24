package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// server adapts Service to the generated vrx.v1.Dataplane gRPC service.
type server struct {
	vrxv1.UnimplementedDataplaneServer
	svc   *Service
	stats statsSource
	log   *slog.Logger
}

var _ vrxv1.DataplaneServer = (*server)(nil)

func (g *server) Apply(ctx context.Context, req *vrxv1.ApplyRequest) (*vrxv1.ApplyResponse, error) {
	return g.svc.Apply(ctx, req)
}

func (g *server) Retrieve(ctx context.Context, req *vrxv1.RetrieveRequest) (*vrxv1.RetrieveResponse, error) {
	return g.svc.Retrieve(ctx, req)
}

func (g *server) DryRun(ctx context.Context, req *vrxv1.DryRunRequest) (*vrxv1.ValidationReport, error) {
	return g.svc.DryRun(ctx, req)
}

func (g *server) Health(context.Context, *vrxv1.HealthRequest) (*vrxv1.HealthResponse, error) {
	return g.svc.Health(), nil
}

func (g *server) InterfaceState(ctx context.Context, req *vrxv1.InterfaceStateRequest) (*vrxv1.InterfaceStateResponse, error) {
	return g.svc.InterfaceState(ctx, req)
}

func (g *server) StreamStats(req *vrxv1.StreamStatsRequest, stream grpc.ServerStreamingServer[vrxv1.StatsBatch]) error {
	if iv := req.GetIntervalMs(); iv != 0 && (iv < 200 || iv > 60000) {
		return status.Errorf(codes.InvalidArgument, "interval_ms %d outside 200–60000", iv)
	}
	if g.stats == nil {
		return status.Error(codes.Unavailable, "stats segment reader not configured")
	}
	err := streamStats(stream.Context(), g.stats, req, stream.Send, g.log)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (g *server) StreamEvents(req *vrxv1.StreamEventsRequest, stream grpc.ServerStreamingServer[vrxv1.Event]) error {
	sub := g.svc.events().subscribe(req)
	defer g.svc.events().unsubscribe(sub)
	for {
		evs, err := sub.next(stream.Context())
		if err != nil {
			return nil // client went away
		}
		for _, ev := range evs {
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}

// Action dispatches on the requested action. Each feature adds its `case *vrxv1.ActionRequest_<Member>:`
// under its anchor and implements it in its own internal/agent/rpc_<slug>.go; every other action is
// Unimplemented (wave-A-hotspots A4).
func (g *server) Action(req *vrxv1.ActionRequest, _ grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	switch req.GetAction().(type) {
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-ikev2-native
	// wave-BC: F-ra-vpn
	// wave-BC: F-ha-state-sync
	// wave-BC: F-capture-trace
	// wave-BC: F-backup-restore
	// wave-A: F-vrf-static-ecmp
	// wave-A: F-neighbors-ra
	// wave-A: F-nat44-ed-sessions
	// wave-A: F-unbound-chrony-syslog
	default:
		return status.Error(codes.Unimplemented, "actions (ping, traceroute, capture) are implemented by P08/F-*")
	}
}

// listenUnix creates the agent socket: parent dir 0750, stale socket removed, socket 0660 with
// group VRX_SOCKET_GROUP (the process's primary group when that group does not exist — the agent
// never creates groups).
func listenUnix(path, group string, log *slog.Logger) (net.Listener, error) {
	if log == nil {
		log = slog.Default()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("socket dir: %w", err)
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		// Refuse to steal the socket of a running agent.
		if c, err := net.Dial("unix", path); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("%s is in use by another process", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	gid := os.Getgid()
	if group != "" {
		if g, err := user.LookupGroup(group); err == nil {
			if n, err := strconv.Atoi(g.Gid); err == nil {
				gid = n
			}
		} else {
			log.Warn("socket group does not exist; using the process's primary group", "group", group, "gid", gid)
		}
	}
	if err := os.Chown(path, -1, gid); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("chown socket: %w", err)
	}
	if err := os.Chmod(path, 0o660); err != nil { //nolint:gosec // contract: socket 0660 so the API's group can connect
		_ = l.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return l, nil
}
