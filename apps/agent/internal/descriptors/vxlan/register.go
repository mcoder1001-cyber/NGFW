// Package vxlan holds the VXLAN descriptors: tunnels and the ip4/ip6 bypass feature.
// See docs/agent/descriptors/vxlan.md.
package vxlan

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the vxlan descriptors (tunnel, bypass) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewBypass(c, owner))
}
