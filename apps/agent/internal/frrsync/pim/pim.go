// Package pim translates bounded FRR observations into declarative mFIB objects.
package pim

import (
	"context"
	"encoding/json"
	"fmt"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers/frr"
	renderpim "ngfw/agent/internal/renderers/frr/pim"
	"ngfw/agent/internal/scheduler"
	"slices"
	"sync"
	"time"
)

const Descriptor = "mfib.route.pim"

// MaxRoutes bounds parser input independently of the supported runtime snapshot.
const MaxRoutes = 10000

// MaxDynamicRoutes limits feature-induced per-route ownership-check churn.
// Larger snapshots are rejected without replacing the last valid cache.
const MaxDynamicRoutes = 256

// Observation follows FRR 10.x pim_cmd_common.c show ip mroute JSON.
type Observation struct {
	Source    string            `json:"source"`
	Group     string            `json:"group"`
	IIF       string            `json:"iif"`
	Installed *int              `json:"installed"`
	Oil       map[string]Output `json:"oil"`
}
type Output struct {
	Inbound  string `json:"inboundInterface"`
	Outbound string `json:"outboundInterface"`
	TTL      *int   `json:"ttl"`
}

// Parse accepts only the default-VRF group/source object tree. A failed or
// unsupported read is never an empty snapshot (and never triggers withdrawal).
func Parse(raw []byte) ([]Observation, error) {
	if len(raw) == 0 || len(raw) >= 4<<20 {
		return nil, fmt.Errorf("PIM observation outside output bound")
	}
	var groups map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil || groups == nil {
		return nil, fmt.Errorf("invalid PIM group tree")
	}
	var out []Observation
	for group, sources := range groups {
		if sources == nil {
			return nil, fmt.Errorf("invalid PIM source tree")
		}
		for source, row := range sources {
			var o Observation
			if err := json.Unmarshal(row, &o); err != nil || o.Group != group || o.Source != source || o.Installed == nil || o.IIF == "" {
				return nil, fmt.Errorf("invalid PIM route observation")
			}
			if *o.Installed != 0 && *o.Installed != 1 {
				return nil, fmt.Errorf("invalid PIM installed state")
			}
			for host, p := range o.Oil {
				if host != p.Outbound || p.Inbound != o.IIF || p.TTL == nil || *p.TTL < 1 || *p.TTL > 255 {
					return nil, fmt.Errorf("invalid PIM OIL observation")
				}
			}
			probe := mfib.Route{Group: o.Group, Source: o.Source, Paths: []mfib.Path{{Interface: "probe", Flags: "accept"}}}
			if probe.Source == "0.0.0.0" || probe.Source == "*" {
				probe.Source = ""
			}
			if e := probe.Validate(); e != nil {
				return nil, e
			}
			out = append(out, o)
			if len(out) > MaxRoutes {
				return nil, fmt.Errorf("PIM route bound exceeded")
			}
		}
	}
	slices.SortFunc(out, func(a, b Observation) int {
		if a.Group < b.Group {
			return -1
		}
		if a.Group > b.Group {
			return 1
		}
		if a.Source < b.Source {
			return -1
		}
		if a.Source > b.Source {
			return 1
		}
		return 0
	})
	return out, nil
}

// Translate uses only enabled PIM interfaces and unique LCP mappings. Removed
// dependencies suppress the entire route, allowing the same transaction to withdraw it.
func Translate(observations []Observation, doc *ngfwv1.DesiredState) []scheduler.KV {
	reverse := map[string]string{}
	ambiguous := map[string]bool{}
	mapping := lcpmap.FromDesired(doc)
	for _, logical := range doc.GetRouting().GetMulticast().GetPim().GetInterfaces() {
		host, ok := mapping[logical]
		if !ok {
			continue
		}
		if previous, ok := reverse[host]; ok && previous != logical {
			ambiguous[host] = true
		}
		reverse[host] = logical
	}
	var out []scheduler.KV
	for _, o := range observations {
		if o.Installed == nil || *o.Installed != 1 {
			continue
		}
		logical, ok := reverse[o.IIF]
		if !ok || ambiguous[o.IIF] {
			continue
		}
		r := mfib.Route{Group: o.Group, Source: o.Source, Paths: []mfib.Path{{Interface: logical, Flags: "accept"}}}
		if r.Source == "0.0.0.0" || r.Source == "*" {
			r.Source = ""
		}
		valid := true
		hosts := make([]string, 0, len(o.Oil))
		for host := range o.Oil {
			hosts = append(hosts, host)
		}
		slices.Sort(hosts)
		for _, host := range hosts {
			logical, ok := reverse[host]
			if !ok || ambiguous[host] || host == o.IIF {
				valid = false
				break
			}
			r.Paths = append(r.Paths, mfib.Path{Interface: logical, Flags: "forward"})
		}
		if valid && r.Validate() == nil {
			out = append(out, df7.KV(mfib.NamedKey(Descriptor, r), r, nil))
		}
	}
	return out
}

// Source caches only fully validated successful reads. Its Run callback must be
// installed in seam S1; it never writes VPP or holds a cache lock while syncing.
type Source struct {
	Read     func(context.Context) ([]byte, error)
	Interval time.Duration
	// OnError receives every completed poll outcome, nil after successful read + sync.
	// Callers can publish transition-based failure and recovery diagnostics.
	OnError func(error)
	mu      sync.RWMutex
	cache   []Observation
}

func (s *Source) Desired(doc *ngfwv1.DesiredState) []scheduler.KV {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Translate(s.cache, doc)
}

// Refresh preserves forwarding on daemon/read/parser failure; {} is a real withdrawal.
func (s *Source) Refresh(ctx context.Context) error {
	if s.Read == nil {
		return fmt.Errorf("PIM reader unavailable")
	}
	raw, e := s.Read(ctx)
	if e != nil {
		return e
	}
	snapshot, e := Parse(raw)
	if e != nil {
		return e
	}
	if len(snapshot) > MaxDynamicRoutes {
		return fmt.Errorf("PIM snapshot exceeds supported limit of %d routes", MaxDynamicRoutes)
	}
	s.mu.Lock()
	s.cache = snapshot
	s.mu.Unlock()
	return nil
}
func (s *Source) Run(ctx context.Context, syncState func(context.Context) error) {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		e := s.Refresh(readCtx)
		cancel()
		if e == nil {
			e = syncState(ctx)
		}
		if ctx.Err() != nil {
			return
		}
		if s.OnError != nil {
			s.OnError(e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Reader returns the fixed, shell-free FRR command used by the source.
func Reader(show func(context.Context, frr.ShowCommand) ([]byte, error)) func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) { return show(ctx, renderpim.ShowMroute) }
}
