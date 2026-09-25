// Package gtpu holds the GTP-U descriptors: tunnels (with in-place tteid update), forwarding
// entries and the ip4/ip6 bypass feature. See docs/agent/descriptors/gtpu.md.
package gtpu

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the gtpu descriptors (tunnel, forward, bypass) with r.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewForward(c, owner))
	r.Register(NewBypass(c, owner, opts...))
}
