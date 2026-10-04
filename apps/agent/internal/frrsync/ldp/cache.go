package ldp

import (
	"reflect"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/mpls"
)

// HoldDown is the grace period before a failed observation withdraws cached routes.
const HoldDown = 60 * time.Second

// RouteName identifies the descriptor scope owned exclusively by the LDP source.
const RouteName = "mpls-route.ldp"

// Cache retains the last valid observation and tracks pending scheduler application.
type Cache struct {
	mu          sync.Mutex
	observation Observation
	routes      []mpls.Route
	failedAt    time.Time
	status      ngfwv1.LdpSyncState
	initialized bool
	dirty       bool
}

// Update replaces successful observations, holds failed reads until expiry, and reports pending application.
func (c *Cache) Update(now time.Time, o Observation, routes []mpls.Route, err error) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.status.LastError = "LDP observation unavailable"
		if c.failedAt.IsZero() {
			c.failedAt = now
		}
		if now.Sub(c.failedAt) < HoldDown {
			return false
		}
		routes = nil
	} else {
		c.failedAt = time.Time{}
		c.observation = o
		c.status.LastError = ""
	}
	changed := c.dirty || !c.initialized || !reflect.DeepEqual(c.routes, routes)
	c.dirty = changed
	c.initialized = true
	c.routes = routes
	return changed
}

// Routes returns a copy of the desired route slice, including routes held during observation failures.
func (c *Cache) Routes() []mpls.Route {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]mpls.Route(nil), c.routes...)
}

// Applied records scheduler success or leaves the cache dirty for another attempt on failure.
func (c *Cache) Applied(now time.Time, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Source = "ldp-bindings"
	c.dirty = err != nil
	if err != nil {
		c.status.LastError = "LDP scheduler sync failed"
		return
	}
	c.status.LastSyncAt = timestamppb.New(now)
}

// State returns a detached cached observation; the caller retrieves the actual installed route count.
func (c *Cache) State() *ngfwv1.MplsLdpStateResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return proto.Clone(&ngfwv1.MplsLdpStateResponse{Neighbors: c.observation.Neighbors, Bindings: c.observation.LIB, Sync: &c.status}).(*ngfwv1.MplsLdpStateResponse)
}

// RoutesFor removes paths whose current LCP identity differs from the observed link.
func (c *Cache) RoutesFor(reverse map[string]string) []mpls.Route {
	c.mu.Lock()
	defer c.mu.Unlock()
	type pathKey struct {
		label         uint32
		next, logical string
	}
	permitted := map[pathKey]bool{}
	for _, b := range c.observation.Bindings {
		if logical := reverse[b.LinuxInterface]; logical != "" {
			permitted[pathKey{b.LocalLabel, b.NextHop, logical}] = true
		}
	}
	var out []mpls.Route
	for _, r := range c.routes {
		paths := r.Paths
		r.Paths = nil
		for _, p := range paths {
			if permitted[pathKey{r.Label, p.NextHop, p.Interface}] {
				r.Paths = append(r.Paths, p)
			}
		}
		if len(r.Paths) > 0 {
			out = append(out, r)
		}
	}
	return out
}
