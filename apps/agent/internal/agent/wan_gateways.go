package agent

import (
	"context"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/subsystems"
)

// readWANGateways polls authoritative owned client state. Event-only caches lose
// renewals across agent reconnect/restart; a complete dump also observes removal.
// Errors intentionally become absence, so stale gateways are withdrawn and the
// next watch tick retries. No configuration or secret is written by this reader.
func (a *Agent) readWANGateways(ctx context.Context, saved *ngfwv1.DesiredState) map[string]multiwan.LearnedGateway {
	observed := map[string]multiwan.LearnedGateway{}
	needDHCP := false
	for _, g := range saved.GetRouting().GetWanGroups() {
		for _, m := range g.GetMembers() {
			needDHCP = needDHCP || m.GetNextHop() == "dhcp"
		}
	}
	var leases map[string]dhcp.Lease
	if needDHCP {
		if rt := subsystems.DHCPFor(a.svc.st.dir, a.svc.owner); rt != nil && rt.Client != nil {
			var err error
			leases, err = rt.Client.Leases(ctx)
			if err != nil {
				a.log.Warn("WAN DHCP gateway observation unavailable")
			}
		}
	}
	for _, g := range saved.GetRouting().GetWanGroups() {
		for _, m := range g.GetMembers() {
			name := m.GetInterface()
			iface := saved.GetInterfaces()[name]
			if iface == nil {
				continue
			}
			switch m.GetNextHop() {
			case "dhcp":
				if iface.GetDhcpClient() == nil {
					continue
				}
				if lease, ok := leases[name]; ok && lease.State == "BOUND" {
					observed[name] = multiwan.LearnedGateway{Source: "dhcp", Address: lease.Address, Gateway: lease.Router}
				}
			case "pppoe":
				// Negotiated peer state alone does not establish a forwarding
				// carrier. Await a verified transit interface snapshot; never
				// route ordinary IP directly through the PPPoE physical WAN.
				continue
			}
		}
	}
	return observed
}
