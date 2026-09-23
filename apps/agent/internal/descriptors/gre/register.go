// Package gre holds the GRE tunnel descriptor (gre_tunnel_add_del_v2 / gre_tunnel_v2_dump):
// L3, TEB and ERSPAN tunnels, point-to-point or multipoint. See docs/agent/descriptors/gre.md.
package gre

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the gre descriptors (tunnel) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewTunnel(c, owner))
}
