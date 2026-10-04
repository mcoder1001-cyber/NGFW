package multiwan

import (
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

const NATOutputName = "nat44-ed.output-feature.wan"
const NATAddressName = "nat44-ed.interface-address.wan"

// NATObjects configures post-routing SNAT; it requires an explicitly enabled ED
// plugin rather than implicitly modifying a global singleton. Configuration
// writers take precedence as a pair, preventing half-owned feature/pool state.
func NATObjects(doc *ngfwv1.DesiredState) []scheduler.KV {
	n := doc.GetNat()
	if n == nil || !n.GetEnabled() || (n.GetMode() != "" && n.GetMode() != "ed") {
		return nil
	}
	occupied := map[string]bool{}
	for _, name := range append(append(append([]string{}, n.GetInside()...), n.GetOutside()...), n.GetOutputFeature()...) {
		occupied[name] = true
	}
	for _, pool := range n.GetPools() {
		if pool.GetInterface() != "" {
			occupied[pool.GetInterface()] = true
		}
	}
	seen := map[string]bool{}
	var out []scheduler.KV
	for _, g := range doc.GetRouting().GetWanGroups() {
		if len(g.GetMembers()) < 2 {
			continue
		}
		for _, m := range g.GetMembers() {
			name := m.GetInterface()
			a, err := netip.ParseAddr(m.GetGateway())
			if occupied[name] || seen[name] || doc.GetInterfaces()[name] == nil || err != nil || !a.Is4() {
				continue
			}
			seen[name] = true
			output, _ := natcommon.Encode(&nat44ed.OutputFeatureSpec{Interface: name})
			address, _ := natcommon.Encode(&nat44ed.InterfaceAddressSpec{Interface: name})
			out = append(out, scheduler.KV{Key: scheduler.Join(NATOutputName, name), Value: output}, scheduler.KV{Key: scheduler.Join(NATAddressName, name), Value: address})
		}
	}
	return out
}

// DeadAddresses is restricted to configured member IPv4 addresses and only
// transitions observed healthy -> unhealthy in the same probe generation.
func DeadAddresses(doc *ngfwv1.DesiredState, before, after []*ngfwv1.WanGroupState) map[string]bool {
	n := doc.GetNat()
	if n == nil || !n.GetEnabled() || (n.GetMode() != "" && n.GetMode() != "ed") {
		return nil
	}
	old := map[string]map[string]bool{}
	for _, g := range before {
		old[g.Name] = map[string]bool{}
		for _, m := range g.Members {
			old[g.Name][m.Interface] = m.Up
		}
	}
	now := map[string]map[string]bool{}
	for _, g := range after {
		now[g.Name] = map[string]bool{}
		for _, m := range g.Members {
			now[g.Name][m.Interface] = m.Up
		}
	}
	dead := map[string]bool{}
	for _, g := range doc.GetRouting().GetWanGroups() {
		if g.StickySessions != nil && !g.GetStickySessions() {
			continue
		}
		for _, m := range g.GetMembers() {
			up, observed := now[g.GetName()][m.GetInterface()]
			if !observed || up || !old[g.GetName()][m.GetInterface()] {
				continue
			}
			for _, raw := range doc.GetInterfaces()[m.GetInterface()].GetIpv4() {
				p, err := netip.ParsePrefix(raw)
				if err == nil && p.Addr().Is4() {
					dead[p.Addr().String()] = true
				}
			}
		}
	}
	return dead
}
