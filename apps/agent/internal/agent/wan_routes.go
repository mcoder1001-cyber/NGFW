package agent

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

func wanIdentity(doc *ngfwv1.DesiredState) string {
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&ngfwv1.DesiredState{Interfaces: doc.GetInterfaces(), Vrfs: doc.GetVrfs(), Nat: doc.GetNat()})
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(b)
	return string(hash[:])
}

func registerWANRoutes(reg scheduler.Registry, wiring *subsystems.Wiring, client vpp.Client, owner string, owned ownertable.Set, runtime *multiwan.Runtime) error {
	d := &multiwan.RouteDescriptor{RouteDescriptor: &core.RouteDescriptor{Env: core.Env{Client: client, Owner: owner, Owned: owned, IfRef: core.AliasInterfaceRef, RouteInstance: multiwan.RouteName}}}
	reg.Register(d)
	claims, err := wiring.KeyedClaims("nat")
	if err != nil {
		return err
	}
	plugin := nat44ed.New(client, owner, natcommon.WithClaims(claims))
	reg.Register(plugin.OutputFeature.Instance(multiwan.NATOutputName))
	reg.Register(plugin.InterfaceAddress.Instance(multiwan.NATAddressName))
	reg.Register(plugin.WANPool(multiwan.NATStaticAddressName, multiwan.NATOutputName))
	return wiring.AddDynamicSource(subsystems.DynamicSource{Name: "multiwan", Descriptors: []string{multiwan.RouteName, multiwan.NATOutputName, multiwan.NATAddressName, multiwan.NATStaticAddressName}, Desired: func(doc *ngfwv1.DesiredState) []scheduler.KV {
		health := runtime.HealthFor(doc.GetRouting().GetWanGroups(), wanIdentity(doc))
		resolved := runtime.ResolveGateways(doc, wanIdentity(doc), true)
		routes, _ := multiwan.Routes(resolved, health)
		if health == nil {
			return routes
		}
		return append(routes, multiwan.NATObjects(resolved)...)
	}, Run: func(ctx context.Context, sync subsystems.SyncFunc) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		runWANRoutes(ctx, ticker.C, runtime.HasConfiguredGroups, sync)
	}})
}

// runWANRoutes retains periodic repair for every configured group, including
// all-down groups. A successfully empty projection needs no repeated retrieval
// of unrelated dependencies; startup and the last withdrawal still reconcile.
func runWANRoutes(ctx context.Context, ticks <-chan time.Time, configured func() bool, apply subsystems.SyncFunc) {
	emptyApplied := false
	for {
		if ctx.Err() != nil {
			return
		}
		active := configured()
		if active || !emptyApplied {
			err := apply(ctx)
			emptyApplied = !active && err == nil
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// installedWANActive only reports a member after an owned FIB Retrieve confirms
// a single matching path. Probe-selected and installed states remain distinct.
func installedWANActive(doc *ngfwv1.DesiredState, kvs []scheduler.KV) map[string]string {
	active := map[string]string{}
	for _, g := range doc.GetRouting().GetWanGroups() {
		if g.GetMode() == "balance" {
			continue
		}
		for _, kv := range kvs {
			r, ok := kv.Value.(*core.Route)
			if !ok || len(r.Paths) != 1 {
				continue
			}
			for _, m := range g.GetMembers() {
				iface := doc.GetInterfaces()[m.GetInterface()]
				table := uint32(0)
				if iface.GetVrf() != "" && iface.GetVrf() != "default" {
					table = doc.GetVrfs()[iface.GetVrf()].GetId()
				}
				if table == r.GetTableId() && r.Paths[0].Interface == m.GetInterface() && r.Paths[0].Address == m.GetGateway() {
					active[g.GetName()] = m.GetInterface()
				}
			}
		}
	}
	return active
}

// withCurrentWAN closes the observation-to-action race with commit/rollback.
func (s *Service) withCurrentWAN(ctx context.Context, saved *ngfwv1.DesiredState, fn func(context.Context) error) (current bool, err error) {
	err = s.exclusive(ctx, func(c context.Context) error {
		if s.st.wanSaved != saved {
			return nil
		}
		current = true
		return fn(c)
	})
	return current, err
}
