// Package vxlan holds the VXLAN descriptors: tunnels and the ip4/ip6 bypass feature.
// See docs/agent/descriptors/vxlan.md.
package vxlan

import (
	"ngfw/agent/internal/descriptors/df6"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the vxlan descriptors (tunnel, bypass) with r.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewBypass(c, owner, opts...))
}

// init maps the VPP 26.06 device class of the interfaces this package creates to their creator
// (iface.RegisterKind, TD-11c 3.1c): interface/<name> then orders the interface's attributes
// (admin state, MTU, addresses, VRF, bridge membership) before the tunnel on delete (F-tunnels).
func init() {
	iface.RegisterKind("VXLAN", TunnelName)
}
