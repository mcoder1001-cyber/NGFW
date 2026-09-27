// Package ipip holds the IP-in-IP descriptors: point-to-point / multipoint tunnels and 6RD
// tunnels. See docs/agent/descriptors/ipip.md.
package ipip

import (
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/kit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the ipip descriptors (tunnel, 6rd) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	kit.Register(r, kit.Env{Client: c, Owner: owner},
		func(e kit.Env) scheduler.Descriptor { return NewTunnel(e.Client, e.Owner) },
		func(e kit.Env) scheduler.Descriptor { return NewSixrd(e.Client, e.Owner) },
	)
}

// init maps the VPP 26.06 device class of the interfaces this package creates to their creator
// (iface.RegisterKind, TD-11c 3.1c): interface/<name> then orders the interface's attributes
// (admin state, MTU, addresses, VRF, bridge membership) before the tunnel on delete (F-tunnels).
func init() {
	iface.RegisterKind("IPIP tunnel device", TunnelName)
	iface.RegisterKind("ip6ip-6rd", SixrdName)
}
