// Package vxlan_gpe holds the descriptors of the vxlan-gpe plugin (tunnel, bypass).
package vxlan_gpe //nolint:revive // package name follows the binapi / VPP plugin name

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the vxlan-gpe descriptors (tunnel, bypass) with r.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewBypass(c, owner, opts...))
}
