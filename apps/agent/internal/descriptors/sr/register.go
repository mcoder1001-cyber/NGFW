// Package sr holds the SRv6 descriptors: local SIDs, policies, steering and the global encap
// source / hop limit. See docs/agent/descriptors/sr.md.
package sr

import (
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the SRv6 descriptors with r: local SIDs, policies and steering (ours by
// claim, D-071) and the VPP-global encap source / hop limit (setters only for the globals
// owner, df6.WithGlobalsOwner; require variants otherwise).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df6.Option) {
	o := df6.BuildOptions(owner, opts)
	r.Register(df6.Global(encapSourceSpec(), c, o))
	r.Register(df6.Global(encapHopLimitSpec(), c, o))
	r.Register(NewLocalSid(c, owner, opts...))
	r.Register(NewPolicy(c, owner, opts...))
	r.Register(NewSteering(c, owner, opts...))
}
