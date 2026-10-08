package agent

import (
	"context"
	"net/netip"

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
	carrier, _ := any(subsystems.PppoeOf(a.svc.owner)).(wanPPPoEForwarding)
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
				if iface.GetPppoe() == nil || carrier == nil {
					continue
				}
				if value, ok := wanPPPGateway(carrier, name); ok {
					observed[name] = value
				}
			}
		}
	}
	return observed
}

// wanPPPoEForwarding admits only the carrier's verified logical VPP interface.
// A legacy runtime without this contract remains unavailable.
type wanPPPoEForwarding interface {
	ForwardingGateway(string) (string, string, string, bool)
	ProbeForwarding(context.Context, string, *ngfwv1.WanMonitor) multiwan.CheckResult
}

func wanPPPGateway(carrier wanPPPoEForwarding, name string) (multiwan.LearnedGateway, bool) {
	local, gateway, generation, ready := carrier.ForwardingGateway(name)
	if !ready || generation == "" {
		return multiwan.LearnedGateway{}, false
	}
	address, err := netip.ParsePrefix(local)
	if err != nil {
		return multiwan.LearnedGateway{}, false
	}
	nextHop, err := netip.ParseAddr(gateway)
	if err != nil {
		return multiwan.LearnedGateway{}, false
	}
	return multiwan.LearnedGateway{Source: "pppoe", Generation: generation, Address: address, Gateway: nextHop}, true
}

func wanProbe(saved *ngfwv1.DesiredState, carrier wanPPPoEForwarding) multiwan.Probe {
	ordinary := multiwan.DeviceProbe(func(member string) (string, error) { return wanDevice(saved, member) })
	return func(ctx context.Context, member string, monitor *ngfwv1.WanMonitor) multiwan.CheckResult {
		iface := saved.GetInterfaces()[member]
		if iface.GetPppoe() == nil {
			return ordinary(ctx, member, monitor)
		}
		unavailable := multiwan.CheckResult{Sent: 1, Unavailable: true}
		if carrier == nil || !multiwan.CarrierProbeTarget(monitor.GetType(), monitor.GetTarget()) {
			return unavailable
		}
		_, _, generation, ready := carrier.ForwardingGateway(member)
		if !ready || generation == "" {
			return unavailable
		}
		observed := carrier.ProbeForwarding(ctx, member, monitor)
		_, _, current, ready := carrier.ForwardingGateway(member)
		if ctx.Err() != nil || !ready || current != generation {
			return unavailable
		}
		return observed
	}
}
