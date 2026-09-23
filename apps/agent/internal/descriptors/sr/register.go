// Package sr holds the SRv6 descriptors: local SIDs, policies, steering and the global encap
// source / hop limit. See docs/agent/descriptors/sr.md.
package sr

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the SRv6 descriptors (localsid, policy, steering, encap source, encap
// hop limit) with r. scope attributes untagged SR objects on a shared VPP (nil = all).
func Register(r scheduler.Registry, c vpp.Client, scope *df6.Scope) {
	r.Register(NewEncapSource(c))
	r.Register(NewEncapHopLimit(c))
	r.Register(NewLocalSid(c, scope))
	r.Register(NewPolicy(c, scope))
	r.Register(NewSteering(c, scope))
}
