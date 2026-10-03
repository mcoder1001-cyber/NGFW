// Package ldp translates validated daemon observations into existing MPLS specs.
// It does not write VPP. The daemon JSON adapter and dynamic-source wiring are
// separate acceptance steps; callers must not confuse a failed read with empty.
package ldp

import (
	"fmt"
	"net/netip"
	"sort"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mpls"
)

// Binding is an in-use remote binding joined with its adjacency by the reader.
type Binding struct {
	FEC            string
	LocalLabel     uint32
	RemoteLabel    uint32
	NextHop        string
	LinuxInterface string
}

// Translate produces EOS IPv4 routes; implicit-null remote labels pop, local
// implicit-null has no entry. Unsupported reserved labels fail the whole read
// rather than silently withdrawing previously installed forwarding state.
func Translate(bindings []Binding, table uint32, reverse map[string]string) ([]mpls.Route, error) {
	routes := map[uint32]mpls.Route{}
	fecs := map[uint32]string{}
	for _, b := range bindings {
		fec, err := netip.ParsePrefix(b.FEC)
		if err != nil || !fec.Addr().Is4() || fec != fec.Masked() {
			return nil, fmt.Errorf("LDP FEC must be canonical IPv4 prefix")
		}
		if b.LocalLabel == 3 {
			continue
		}
		if b.LocalLabel < 16 || b.LocalLabel > df7.MaxLabel {
			return nil, fmt.Errorf("LDP local label outside unreserved range")
		}
		if b.RemoteLabel != 3 && (b.RemoteLabel < 16 || b.RemoteLabel > df7.MaxLabel) {
			return nil, fmt.Errorf("LDP unsupported remote label")
		}
		next, err := netip.ParseAddr(b.NextHop)
		if err != nil || !next.Is4() || next.IsUnspecified() || next.IsMulticast() {
			return nil, fmt.Errorf("LDP next hop must be IPv4 unicast")
		}
		logical, ok := reverse[b.LinuxInterface]
		if !ok || logical == "" {
			return nil, fmt.Errorf("LDP adjacency has no logical LCP mapping")
		}
		if previous, ok := fecs[b.LocalLabel]; ok && previous != b.FEC {
			return nil, fmt.Errorf("LDP label collision between FECs")
		}
		fecs[b.LocalLabel] = b.FEC
		route := routes[b.LocalLabel]
		route.Table, route.Label, route.EOS, route.EOSProto = table, b.LocalLabel, true, mpls.PayloadIP4
		path := df7.Path{Interface: logical, NextHop: next.String(), Weight: 1}
		if b.RemoteLabel != 3 {
			path.Labels = []df7.Label{{Label: b.RemoteLabel}}
		}
		route.Paths = append(route.Paths, path)
		routes[b.LocalLabel] = route
	}
	labels := make([]uint32, 0, len(routes))
	for label := range routes {
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool { return labels[i] < labels[j] })
	out := make([]mpls.Route, 0, len(labels))
	for _, label := range labels {
		r := routes[label]
		paths, err := df7.NormalizePaths(r.Paths)
		if err != nil {
			return nil, err
		}
		// VPP MrNPaths is uint8. Duplicate observations must not multiply
		// forwarding paths, and a larger distinct set must never wrap to zero.
		seen := map[string]bool{}
		r.Paths = nil
		for _, path := range paths {
			key := fmt.Sprintf("%#v", path)
			if seen[key] {
				continue
			}
			seen[key] = true
			r.Paths = append(r.Paths, path)
		}
		if len(r.Paths) > 255 {
			return nil, fmt.Errorf("LDP route exceeds VPP limit of 255 paths")
		}
		if err := r.Validate(); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
