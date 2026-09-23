// Package l2tp holds the L2TPv3 descriptors: tunnels (cookies updated in place), per-interface
// decap enable and the global lookup key. See docs/agent/descriptors/l2tp.md.
package l2tp

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the l2tp descriptors (tunnel, interface-enable, lookup-key) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	o := df6.BuildOptions(owner, opts)
	r.Register(NewTunnel(c, owner))
	r.Register(NewInterfaceEnable(c, owner, opts...))
	r.Register(df6.Global(lookupKeySpec(), c, o)) // VPP-global: setter only on the globals owner (D-071)
}
