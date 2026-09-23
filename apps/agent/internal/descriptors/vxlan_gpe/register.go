// Package vxlan_gpe holds the descriptors of the vxlan-gpe plugin (tunnel, bypass).
package vxlan_gpe //nolint:revive // package name follows the binapi / VPP plugin name

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the vxlan-gpe descriptors (tunnel, bypass) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewTunnel(c, owner))
	r.Register(NewBypass(c, owner))
}
