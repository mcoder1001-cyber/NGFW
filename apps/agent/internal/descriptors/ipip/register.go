// Package ipip holds the IP-in-IP descriptors: point-to-point / multipoint tunnels and 6RD
// tunnels. See docs/agent/descriptors/ipip.md.
package ipip

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the ipip descriptors (tunnel, 6rd) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewSixrd(c, owner))
}
