// Package gre holds the GRE tunnel descriptor (gre_tunnel_add_del_v2 / gre_tunnel_v2_dump):
// L3, TEB and ERSPAN tunnels, point-to-point or multipoint. See docs/agent/descriptors/gre.md.
package gre

import (
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/kit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the gre descriptors (tunnel) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	kit.Register(r, kit.Env{Client: c, Owner: owner},
		func(e kit.Env) scheduler.Descriptor { return NewTunnel(e.Client, e.Owner) },
	)
}

// init maps the VPP 26.06 device class of the interfaces this package creates to their creator
// (iface.RegisterKind, TD-11c 3.1c): interface/<name> then orders the interface's attributes
// (admin state, MTU, addresses, VRF, bridge membership) before the tunnel on delete (F-tunnels).
func init() {
	iface.RegisterKind("GRE tunnel device", TunnelName)
}
