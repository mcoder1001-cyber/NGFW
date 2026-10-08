package agent

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/scheduler"
)

func (g *server) WanState(ctx context.Context, req *ngfwv1.WanStateRequest) (*ngfwv1.WanStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if g.wan == nil || !g.wan.Ready() {
		if g.svc.st != nil {
			if err := g.svc.lock(ctx); err != nil {
				return nil, err
			}
			empty := len(g.svc.st.wanSaved.GetRouting().GetWanGroups()) == 0
			g.svc.unlock()
			if empty {
				if len(req.GetGroups()) > 0 {
					return nil, status.Error(codes.NotFound, "requested WAN group is not configured")
				}
				return &ngfwv1.WanStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now())}, nil
			}
		}
		return nil, status.Error(codes.Unavailable, "WAN monitor runtime is not wired")
	}
	snapshot, unavailable := g.wan.SnapshotWithAvailability()
	if snapshot == nil {
		return nil, status.Error(codes.Unavailable, "WAN monitor runtime is not ready")
	}
	active := map[string]string{}
	if g.svc.wan != nil {
		if err := g.svc.lock(ctx); err != nil {
			return nil, err
		}
		saved := g.svc.st.wanSaved
		installed, err := g.svc.sched.Retrieve(ctx, scheduler.Only(multiwan.RouteName))
		if err != nil {
			g.svc.unlock()
			return nil, status.Error(codes.Unavailable, "WAN installed routes unavailable")
		}
		active = installedWANActive(g.svc.wan.ResolveGateways(saved, wanIdentity(saved), false), installed)
		g.svc.unlock()
	}
	for _, group := range snapshot {
		group.Active = active[group.Name]
	}
	filter := len(req.GetGroups()) > 0
	selected := map[string]bool{}
	for _, name := range req.GetGroups() {
		selected[name] = true
	}
	response := &ngfwv1.WanStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now())}
	for _, group := range snapshot {
		if !filter || selected[group.Name] {
			if unavailable[group.Name] {
				return nil, status.Error(codes.Unavailable, "configured WAN probe is unavailable under host policy")
			}
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
	var previousHealth []*ngfwv1.WanGroupState
	var previousResolved *ngfwv1.DesiredState
	previousIdentity := ""
	pendingDead := map[string]bool{}
	cleanupProgress := &multiwan.CleanupProgress{}
	cleaner := nat44ed.New(a.svc.vpp, a.svc.owner)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		if err := a.svc.lock(ctx); err != nil {
			return
		}
		saved := a.svc.st.wanSaved
		groups := saved.GetRouting().GetWanGroups()
		clone := make([]*ngfwv1.WanGroup, 0, len(groups))
		for _, group := range groups {
			clone = append(clone, proto.Clone(group).(*ngfwv1.WanGroup))
		}
		identity := wanIdentity(saved)
		a.svc.unlock()
		if err := runtime.ReplaceWithProbe(ctx, clone, identity, multiwan.DeviceProbe(func(member string) (string, error) {
			return wanDevice(saved, member)
		})); err != nil && ctx.Err() == nil {
			a.log.Error("WAN monitor configuration rejected", "reason", err.Error())
		}
		observationCtx, observationCancel := context.WithTimeout(ctx, 2*time.Second)
		observed := a.readWANGateways(observationCtx, saved)
		observationCancel()
		if err := a.svc.lock(ctx); err != nil {
			return
		}
		if a.svc.st.wanSaved == saved && runtime.SetGateways(clone, identity, observed) {
			requestResync(a.resyncs)
		}
		a.svc.unlock()
		health := runtime.HealthFor(clone, identity)
		generationBytes, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(saved.GetRouting())
		generation := identity + string(generationBytes)
		if generation != previousIdentity {
			previousHealth = nil
			previousResolved = nil
			pendingDead = map[string]bool{}
			previousIdentity = generation
			*cleanupProgress = multiwan.CleanupProgress{}
		}
		cleanupHealth, unavailableGroups := runtime.SnapshotWithAvailability()
		usable := cleanupHealth[:0]
		for _, g := range cleanupHealth {
			if !unavailableGroups[g.Name] {
				usable = append(usable, g)
			}
		}
		resolved := runtime.ResolveGateways(saved, identity, true)
		for addr := range multiwan.DeadAddresses(previousResolved, previousHealth, usable) {
			if !pendingDead[addr] {
				*cleanupProgress = multiwan.CleanupProgress{}
			}
			pendingDead[addr] = true
		}
		for addr := range multiwan.RetiredAddresses(previousResolved, resolved) {
			pendingDead[addr] = true
			*cleanupProgress = multiwan.CleanupProgress{}
		}
		previousResolved = resolved
		if len(pendingDead) > 0 {
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			current, err := a.svc.withCurrentWAN(cleanupCtx, saved, func(c context.Context) error {
				tables := map[uint32]bool{}
				for _, g := range saved.GetRouting().GetWanGroups() {
					for _, m := range g.GetMembers() {
						iface := saved.GetInterfaces()[m.GetInterface()]
						vrf := iface.GetVrf()
						table := uint32(0)
						if vrf != "" && vrf != "default" {
							v, ok := saved.GetVrfs()[vrf]
							if !ok {
								continue
							}
							table = v.GetId()
						}
						tables[table] = true
					}
				}
				count, complete, e := multiwan.ClearDeadSessions(c, cleaner, pendingDead, tables, cleanupProgress)
				if count > 0 {
					a.log.Info("WAN dead-link NAT sessions cleared", "count", count)
				}
				if complete {
					pendingDead = map[string]bool{}
				}
				return e
			})
			cancel()
			if !current && err == nil {
				pendingDead = map[string]bool{}
				*cleanupProgress = multiwan.CleanupProgress{}
			}
			if err != nil {
				a.log.Warn("WAN dead-link NAT session cleanup pending", "reason", err.Error())
			}
		}
		// Probe measurements do not trigger a full resync; only forwarding selection changes do.
		for _, g := range health {
			for _, m := range g.Members {
				m.Since = nil
				m.LossPct = 0
				m.LatencyMs = 0
			}
		}
		changed := len(health) != len(previousHealth)
		for i, g := range health {
			if changed || !proto.Equal(g, previousHealth[i]) {
				changed = true
				break
			}
		}
		if changed {
			requestResync(a.resyncs)
			previousHealth = health
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func wanDevice(saved *ngfwv1.DesiredState, member string) (string, error) {
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
func wanSnapshot(ds *ngfwv1.DesiredState) *ngfwv1.DesiredState {
	return proto.Clone(&ngfwv1.DesiredState{
		Interfaces: ds.GetInterfaces(),
		Vrfs:       ds.GetVrfs(),
		Nat:        ds.GetNat(),
		Routing:    &ngfwv1.RoutingConfig{WanGroups: ds.GetRouting().GetWanGroups(), Pbr: ds.GetRouting().GetPbr()},
	}).(*ngfwv1.DesiredState)
}
