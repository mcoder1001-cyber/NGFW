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
	"runtime/debug"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// newGRPCServer is the agent's gRPC server with panic recovery on every unary and streaming handler
// (TD-9, review 1.1d): a handler panic answers INTERNAL, is logged with its stack and counted in
// vrx_agent_panics_total{where="grpc"}, and never takes the agent down. A panic inside a transaction is
// contained by the service itself (it also reloads the state and owes a resync, Service.containLocked),
// and a descriptor's by the scheduler (ErrDescriptorPanic): the transaction rolls back.
func newGRPCServer(log *slog.Logger, m *metrics) *grpc.Server {
	recovered := func(method string, r any) error {
		log.Error("gRPC handler panicked", "method", method, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		m.panicked("grpc")
		return status.Errorf(codes.Internal, "agent bug: %s panicked (logged)", method)
	}
	return grpc.NewServer(
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
			defer func() {
				if r := recover(); r != nil {
					resp, err = nil, recovered(info.FullMethod, r)
				}
			}()
			return h(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = recovered(info.FullMethod, r)
				}
			}()
			return h(srv, ss)
		}),
	)
}

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
func (g *server) Action(req *vrxv1.ActionRequest, stream grpc.ServerStreamingServer[vrxv1.ActionOutput]) error {
	switch req.GetAction().(type) {
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-ikev2-native
	// wave-BC: F-ra-vpn
	// wave-BC: F-ha-state-sync
	// wave-BC: F-capture-trace
	// wave-BC: F-backup-restore
	// wave-A: F-vrf-static-ecmp
	case *vrxv1.ActionRequest_Ping:
		return g.actionPing(req.GetPing(), stream)
	case *vrxv1.ActionRequest_Traceroute:
		return g.actionTraceroute(req.GetTraceroute())
	// wave-A: F-neighbors-ra
	case *vrxv1.ActionRequest_ArpFlush:
		return g.arpFlush(req.GetArpFlush(), stream)
	// wave-A: F-nat44-ed-sessions
	case *vrxv1.ActionRequest_NatSessionKill:
		return g.natSessionKill(req, stream)
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
