package agent

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/multiwan"
)

func (g *server) WanState(ctx context.Context, req *vrxv1.WanStateRequest) (*vrxv1.WanStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if g.wan == nil || !g.wan.Ready() {
		return nil, status.Error(codes.Unavailable, "WAN monitor runtime is not wired")
	}
	snapshot := g.wan.Snapshot()
	filter := len(req.GetGroups()) > 0
	selected := map[string]bool{}
	for _, name := range req.GetGroups() {
		selected[name] = true
	}
	response := &vrxv1.WanStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now())}
	for _, group := range snapshot {
		if !filter || selected[group.Name] {
			response.Groups = append(response.Groups, group)
			delete(selected, group.Name)
		}
	}
	if len(selected) > 0 {
		return nil, status.Error(codes.NotFound, "requested WAN group is not configured")
	}
	return response, nil
}

// watchWAN observes only durably stored desired state, including rollback and
// restart. Probe I/O runs outside the transaction lock; an invalid replacement
// leaves the last valid generation intact and is logged, never silently applied.
func (a *Agent) watchWAN(ctx context.Context, runtime *multiwan.Runtime) {
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
		defer cancel()
		if err := runtime.Close(closeCtx); err != nil {
			a.log.Error("WAN probes failed to drain")
		}
	}()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		if err := a.svc.lock(ctx); err != nil {
			return
		}
		saved := a.svc.st.wanSaved
		groups := saved.GetRouting().GetWanGroups()
		clone := make([]*vrxv1.WanGroup, 0, len(groups))
		for _, group := range groups {
			clone = append(clone, proto.Clone(group).(*vrxv1.WanGroup))
		}
		identityDoc := &vrxv1.DesiredState{Interfaces: saved.GetInterfaces()}
		encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(identityDoc)
		identity := sha256.Sum256(encoded)
		a.svc.unlock()
		if err != nil {
			a.log.Error("WAN interface identity encoding failed")
			return
		}
		if err := runtime.ReplaceWithProbe(ctx, clone, string(identity[:]), multiwan.DeviceProbe(func(member string) (string, error) {
			return wanDevice(saved, member)
		})); err != nil && ctx.Err() == nil {
			a.log.Error("WAN monitor configuration rejected", "reason", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func wanDevice(saved *vrxv1.DesiredState, member string) (string, error) {
	iface := saved.GetInterfaces()[member]
	if iface == nil || iface.GetLcp() == nil || (iface.GetVrf() != "" && iface.GetVrf() != "default") || iface.GetLcp().GetNetns() != "" {
		return "", multiwan.UnsupportedDevice()
	}
	device := iface.GetLcp().GetHostIfName()
	if device == "" {
		device = member
	}
	return device, nil
}

// wanSnapshot copies only monitor inputs, not unrelated routes or secrets. The
// resulting document is immutable and shared by one committed probe generation.
func wanSnapshot(ds *vrxv1.DesiredState) *vrxv1.DesiredState {
	return proto.Clone(&vrxv1.DesiredState{
		Interfaces: ds.GetInterfaces(),
		Routing:    &vrxv1.RoutingConfig{WanGroups: ds.GetRouting().GetWanGroups()},
	}).(*vrxv1.DesiredState)
}
