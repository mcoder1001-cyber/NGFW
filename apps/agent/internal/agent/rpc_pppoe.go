package agent

// F-pppoe-client / F-pppoe-client-host (D-168): the PppoeReconnect RPC handler. It is a top-level DataplaneServer
// method (not an Action case), so defining it on *server overrides the embedded UnimplementedDataplaneServer, which
// returned Unimplemented (501 upstream) before. It redials one PPPoE client session now, ignoring the hold-off, by
// restarting its pppd supervisor unit through the pppoe subsystem runtime.

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/subsystems"
)

// maxIfaceName bounds the interface name echoed into the reply/log (a config interface key is short).
const maxIfaceName = 64

// PppoeReconnect implements the PppoeReconnect RPC (the name is fixed by the generated DataplaneServer interface).
func (g *server) PppoeReconnect(ctx context.Context, req *ngfwv1.PppoeReconnectRequest) (*ngfwv1.PppoeReconnectResponse, error) { //nolint:revive // generated interface name
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	iface := req.GetInterface()
	if iface == "" || len(iface) > maxIfaceName || strings.IndexFunc(iface, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "interface is required, at most %d bytes, no control characters", maxIfaceName)
	}
	rt := subsystems.PppoeOf(g.svc.owner)
	if rt == nil {
		return nil, status.Error(codes.Unavailable, "the PPPoE client subsystem is not wired in this agent")
	}
	accepted, message, err := rt.Reconnect(ctx, iface)
	if err != nil {
		// A slot agent does not drive the host's pppd units (D-079/D-173).
		if errors.Is(err, subsystems.ErrNotGlobalsOwner) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	g.log.Info("rpc PppoeReconnect", "interface", iface, "accepted", accepted)
	return &ngfwv1.PppoeReconnectResponse{Accepted: accepted, Message: message}, nil
}
