package agent

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/scheduler"
)

// Native IKE owns protected IPIP runtime admin state: down before establishment and after
// SA deletion. A normal interface desired object must never restore admin-up on resync.
func suppressNativeIPsecAdmin(p *projected, ds *ngfwv1.DesiredState) {
	protected := map[scheduler.Key]bool{}
	for _, tunnel := range ds.GetVpn().GetIpsec().GetTunnels() {
		if tunnel.GetEngine() != "vpp-ikev2" || (tunnel.Enabled != nil && !tunnel.GetEnabled()) {
			continue
		}
		name := tunnel.GetRouteBased().GetIpipInterface()
		ip := ds.GetTunnels().GetIpip()[name]
		if ip == nil || ip.Instance == nil {
			continue
		}
		protected[scheduler.Join(iface.AdminStateName, ipip.InterfaceName(ip.GetInstance()))] = true
		p.warnf(ptr("tunnels", "ipip", name, "enabled"), "agent.write-only-field",
			"protected IPIP admin state is controlled by native IKE SA establishment and deletion")
	}
	filtered := p.kvs[:0]
	for _, kv := range p.kvs {
		if !protected[kv.Key] {
			filtered = append(filtered, kv)
		} else {
			delete(p.pointers, kv.Key)
		}
	}
	p.kvs = filtered
}
