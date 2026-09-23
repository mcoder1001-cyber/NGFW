// Package pppoe holds the PPPoE descriptors: sessions and the (VPP-global) control-plane punt
// interface. See docs/agent/descriptors/pppoe.md.
package pppoe

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the pppoe descriptors with r: sessions, and pppoe.cp/global (setter only
// on the globals owner, df6.WithGlobalsOwner; require variant otherwise — D-071).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	o := df6.BuildOptions(owner, opts)
	r.Register(NewSession(c, owner))
	r.Register(df6.Global(cpSpec(owner, o.Claims), c, o))
}
