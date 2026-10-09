package multiwan

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/scheduler"
)

// RouteName identifies WAN-owned default routes.
const RouteName = "ip.route.wan"

// RouteIssue associates a validation failure with its configuration pointer.
type RouteIssue struct{ Pointer, Message string }

// Routes validates exclusive ownership even while all links are down. Unknown
// and locally unavailable observations never grant a forwarding path.
func Routes(doc *ngfwv1.DesiredState, health []*ngfwv1.WanGroupState) ([]scheduler.KV, []RouteIssue) {
	var out []scheduler.KV
	var issues []RouteIssue
	fail := func(p, m string) { issues = append(issues, RouteIssue{p, m}) }
	up := map[string]map[string]bool{}
	for _, g := range health {
		up[g.GetName()] = map[string]bool{}
		for _, m := range g.GetMembers() {
			up[g.GetName()][m.GetInterface()] = m.GetUp()
		}
	}
	claims := map[string]string{}
	tableOf := func(name string) (uint32, bool) {
		if name == "" || name == "default" {
			return 0, true
		}
		v, ok := doc.GetVrfs()[name]
		return v.GetId(), ok
	}
	for i, g := range doc.GetRouting().GetWanGroups() {
		pt := "/routing/wanGroups/" + strconv.Itoa(i)
		if g.GetMode() != "" && g.GetMode() != "failover" && g.GetMode() != "balance" {
			fail(pt+"/mode", "unsupported WAN mode")
			continue
		}
		var members []Member
		gateways := map[string]netip.Addr{}
		var table uint32
		tableSet := false
		valid := true
		families := map[bool]bool{}
		for j, m := range g.GetMembers() {
			mp := pt + "/members/" + strconv.Itoa(j)
			iface, ok := doc.GetInterfaces()[m.GetInterface()]
			if !ok {
				fail(mp+"/interface", "WAN interface does not exist")
				valid = false
				continue
			}
			t, ok := tableOf(iface.GetVrf())
			if !ok || (tableSet && table != t) {
				fail(mp+"/interface", "WAN members must share one existing VRF")
				valid = false
				continue
			}
			table, tableSet = t, true
			if m.GetNextHop() != "gateway" {
				// Dynamic clients are IPv4 and reserve their route even before binding.
				if m.GetNextHop() == "dhcp" || m.GetNextHop() == "pppoe" {
					families[false] = true
				}
				continue
			}
			a, err := netip.ParseAddr(m.GetGateway())
			if err != nil || a.IsUnspecified() || a.IsMulticast() {
				fail(mp+"/gateway", "WAN gateway must be a unicast IP address")
				valid = false
				continue
			}
			a = a.Unmap()
			gateways[m.GetInterface()] = a
			families[a.Is6()] = true
			w := m.GetWeight()
			if w == 0 {
				w = 1
			}
			if w > 255 {
				fail(mp+"/weight", "WAN weight exceeds 255")
				valid = false
			}
			priority := uint32(100)
			if m.Priority != nil {
				priority = m.GetPriority()
			}
			members = append(members, Member{Interface: m.GetInterface(), Priority: int(priority), Weight: int(w), Up: up[g.GetName()][m.GetInterface()]})
		}
		if len(families) > 1 {
			fail(pt+"/members", "WAN group gateways must use one address family")
			valid = false
		}
		for v6 := range families {
			prefix := "0.0.0.0/0"
			if v6 {
				prefix = "::/0"
			}
			key := core.RouteKey(table, prefix).ID()
			if previous, ok := claims[key]; ok {
				fail(pt, "WAN default route conflicts with "+previous)
				valid = false
			}
			claims[key] = pt
			for j, r := range doc.GetRouting().GetStatic() {
				t, ok := tableOf(r.GetVrf())
				p, e := netip.ParsePrefix(r.GetPrefix())
				if ok && e == nil && t == table && p.Bits() == 0 && p.Addr().Is6() == v6 {
					fail(fmt.Sprintf("/routing/static/%d", j), "static default conflicts with WAN group")
					valid = false
				}
			}
			if !valid {
				continue
			}
			selected := BalancePaths(members)
			if g.GetMode() != "balance" {
				active := FailoverActive(members)
				selected = nil
				for _, m := range members {
					if m.Interface == active {
						selected = append(selected, m)
					}
				}
			}
			route := &core.Route{TableId: table, Prefix: prefix}
			for _, m := range selected {
				if m.Weight < 1 || m.Weight > 255 {
					fail(pt+"/members", "selected WAN weight is outside 1..255")
					continue
				}
				route.Paths = append(route.Paths, &core.RoutePath{Interface: m.Interface, Address: gateways[m.Interface].String(), Weight: uint32(m.Weight)})
			}
			if len(route.Paths) > 0 {
				out = append(out, scheduler.KV{Key: scheduler.Join(RouteName, key), Value: route})
			}
		}
	}
	// Refuse a partially valid forwarding program.
	if len(issues) > 0 {
		return nil, issues
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
