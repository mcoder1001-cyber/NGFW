// Package ipip holds the IP-in-IP descriptors: point-to-point / multipoint tunnels and 6RD
// tunnels. See docs/agent/descriptors/ipip.md.
package ipip

import (
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
