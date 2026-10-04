package multiwan

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"reflect"
	"sort"
)

// ExpandPBR converts group references on a detached document. All-down groups
// retain an ordinary lookup in their VRF; no failed member remains pinned.
func ExpandPBR(doc *ngfwv1.DesiredState, health []*ngfwv1.WanGroupState) []RouteIssue {
	routes, issues := Routes(doc, health)
	if len(issues) > 0 {
		return issues
	}
	groups := map[string]*ngfwv1.WanGroup{}
	for _, g := range doc.GetRouting().GetWanGroups() {
		groups[g.GetName()] = g
	}
	for name, policy := range doc.GetRouting().GetPbr().GetPolicies() {
		var paths []*ngfwv1.PbrPath
		for i, path := range policy.GetPaths() {
			group := path.GetWanGroup()
			if group == "" {
				paths = append(paths, path)
				continue
			}
			pointer := fmt.Sprintf("/routing/pbr/policies/%s/paths/%d/wanGroup", name, i)
			g, ok := groups[group]
			if !ok {
				issues = append(issues, RouteIssue{pointer, "WAN group does not exist"})
				continue
			}
			if path.Address != nil || path.Interface != nil || (path.Vrf != nil && path.GetVrf() != "default") {
				issues = append(issues, RouteIssue{pointer, "wanGroup is exclusive with address, interface and VRF"})
				continue
			}
			vrf := "default"
			if len(g.GetMembers()) > 0 {
				vrf = doc.GetInterfaces()[g.GetMembers()[0].GetInterface()].GetVrf()
				if vrf == "" {
					vrf = "default"
				}
			}
			var selected *core.Route
			// A member can occur in both IPv4 and IPv6 groups. Match the
			// group's forwarding identity, rather than an overlapping interface.
			table := uint32(0)
			if vrf != "default" {
				table = doc.GetVrfs()[vrf].GetId()
			}
			for _, member := range g.GetMembers() {
				if member.GetNextHop() != "gateway" {
					continue
				}
				gateway, err := netip.ParseAddr(member.GetGateway())
				if err != nil {
					continue // Routes already validates static gateways.
				}
				prefix := "0.0.0.0/0"
				if gateway.Unmap().Is6() {
					prefix = "::/0"
				}
				for _, kv := range routes {
					if kv.Key.ID() == core.RouteKey(table, prefix).ID() {
						selected = kv.Value.(*core.Route)
						break
					}
				}
				break
			}
			if selected == nil {
				paths = append(paths, &ngfwv1.PbrPath{Vrf: &vrf, Weight: path.Weight})
				continue
			}
			for _, rp := range selected.Paths {
				address, iface, weight := rp.Address, rp.Interface, rp.Weight
				paths = append(paths, &ngfwv1.PbrPath{Address: &address, Interface: &iface, Weight: &weight})
			}
		}
		policy.Paths = paths
	}
	return issues
}

// RestorePBRReferences preserves configuration references only when retrieved
// forwarding paths equal their current expansion. Divergence remains visible.
func RestorePBRReferences(retrieved, saved *ngfwv1.DesiredState, health []*ngfwv1.WanGroupState) {
	if saved == nil {
		return
	}
	expanded := proto.Clone(saved).(*ngfwv1.DesiredState)
	if len(ExpandPBR(expanded, health)) > 0 {
		return
	}
	normalize := func(paths []*ngfwv1.PbrPath) []string {
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			vrf := p.GetVrf()
			if vrf == "" {
				vrf = "default"
			}
			weight := p.GetWeight()
			if weight == 0 {
				weight = 1
			}
			out = append(out, fmt.Sprintf("%s|%s|%s|%d", p.GetAddress(), p.GetInterface(), vrf, weight))
		}
		sort.Strings(out)
		return out
	}
	for name, pol := range retrieved.GetRouting().GetPbr().GetPolicies() {
		original := saved.GetRouting().GetPbr().GetPolicies()[name]
		hasGroup := false
		for _, p := range original.GetPaths() {
			hasGroup = hasGroup || p.GetWanGroup() != ""
		}
		if !hasGroup {
			continue
		}
		expected := expanded.GetRouting().GetPbr().GetPolicies()[name]
		if reflect.DeepEqual(normalize(pol.Paths), normalize(expected.GetPaths())) {
			pol.Paths = proto.Clone(original).(*ngfwv1.PbrPolicy).Paths
		}
	}
}
