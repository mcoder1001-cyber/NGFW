package subsystems

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mpls"
	ldp "ngfw/agent/internal/frrsync/ldp"
	"ngfw/agent/internal/lcpmap"
	_ "ngfw/agent/internal/renderers/frr/ldp"
	"ngfw/agent/internal/scheduler"
)

var ldpCaches sync.Map
var ldpStateReaders sync.Map // owner -> named descriptor Retrieve

// MplsLdpState returns a detached snapshot of the live source.
func MplsLdpState(ctx context.Context, owner string) (*ngfwv1.MplsLdpStateResponse, error) {
	c, ok := ldpCaches.Load(owner)
	if !ok {
		return nil, nil
	}
	reader, ok := ldpStateReaders.Load(owner)
	if !ok {
		return nil, fmt.Errorf("LDP ownership reader unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	routes, err := reader.(func(context.Context) ([]scheduler.KV, error))(ctx)
	if err != nil {
		return nil, fmt.Errorf("LDP installed state unavailable")
	}
	out := c.(*ldp.Cache).State()
	out.Sync.Installed = uint32(len(routes))
	return out, nil
}
func registerMplsLdp(r scheduler.Registry, w *Wiring) error { return registerMplsLdpFor(r, w, 0) }

func registerMplsLdpFor(r scheduler.Registry, w *Wiring, table uint32) error {
	rt := FRRRuntime(w.env.Owner)
	if !rt.Enabled() {
		return nil
	}
	ids, err := w.IDRange()
	if err != nil {
		return err
	}
	var opts []df7.Option
	if ids != nil {
		opts = append(opts, df7.WithIDRange(ids.Lo, ids.Hi))
	}
	descriptor := mpls.NewNamedRoute(ldp.RouteName, w.env.Client, w.env.Owner, opts...)
	r.Register(descriptor)
	cache := &ldp.Cache{}
	ldpCaches.Store(w.env.Owner, cache)
	ldpStateReaders.Store(w.env.Owner, descriptor.Retrieve)
	return w.AddDynamicSource(DynamicSource{Name: "mpls-ldp", Descriptors: []string{ldp.RouteName}, Desired: func(doc *ngfwv1.DesiredState) []scheduler.KV {
		cfg := doc.GetRouting().GetMpls().GetLdp()
		if cfg == nil {
			return nil
		}
		allowed := map[string]bool{}
		for _, name := range cfg.GetInterfaces() {
			allowed[name] = doc.GetInterfaces()[name] != nil
		}
		var out []scheduler.KV
		reverse := map[string]string{}
		for logical, host := range lcpmap.FromDesired(doc) {
			reverse[host] = logical
		}
		for _, route := range cache.RoutesFor(reverse) {
			if labels := cfg.GetLabelRange(); labels != nil && (route.Label < labels.GetMin() || route.Label > labels.GetMax()) {
				continue
			}
			paths := route.Paths
			route.Paths = nil
			for _, p := range paths {
				if allowed[p.Interface] {
					route.Paths = append(route.Paths, p)
				}
			}
			if len(route.Paths) > 0 {
				out = append(out, df7.KV(descriptor.KeyOf(df7.Encode(route)), route, nil))
			}
		}
		return out
	}, Run: func(ctx context.Context, apply SyncFunc) {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		previous := map[string]string{}
		degraded := false
		for {
			rt.mu.Lock()
			doc := rt.last
			if doc != nil {
				doc = proto.Clone(doc).(*ngfwv1.DesiredState)
			}
			rt.mu.Unlock()
			var observation ldp.Observation
			var routes []mpls.Route
			var readErr error
			if doc.GetRouting().GetMpls().GetLdp() != nil {
				observation, readErr = ldp.Read(ctx, rt.r.ShowJSON)
				if readErr == nil {
					reverse := map[string]string{}
					for logical, host := range lcpmap.FromDesired(doc) {
						reverse[host] = logical
					}
					routes, readErr = ldp.Translate(observation.Bindings, table, reverse)
				}
			}
			if readErr == nil {
				current := map[string]string{}
				for _, n := range observation.Neighbors {
					current[n.LsrId] = n.State
				}
				if !reflect.DeepEqual(previous, current) {
					for peer, state := range current {
						if previous[peer] != state {
							w.Publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_LDP_NEIGHBOR_CHANGED, Attributes: map[string]string{"source": "ldp", "peer": peer, "old": previous[peer], "new": state}})
						}
					}
					for peer, state := range previous {
						if _, ok := current[peer]; !ok {
							w.Publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_LDP_NEIGHBOR_CHANGED, Attributes: map[string]string{"source": "ldp", "peer": peer, "old": state, "new": "DOWN"}})
						}
					}
					previous = current
				}
			}
			if cache.Update(time.Now(), observation, routes, readErr) {
				syncErr := apply(ctx)
				cache.Applied(time.Now(), syncErr)
				if syncErr != nil {
					w.Publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_ERROR, Message: "LDP scheduler sync failed", Attributes: map[string]string{"source": "ldp"}})
				}
			}
			if readErr != nil && !degraded {
				w.Publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_ERROR, Message: "LDP observation unavailable; retaining routes during hold-down", Attributes: map[string]string{"source": "ldp"}})
			}
			degraded = readErr != nil
			if readErr != nil {
				w.env.Log.Debug("LDP observation failed", "error", fmt.Sprint(readErr))
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}})
}
