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
	r.Register(&Global{Descriptor: df6.Global(encapSourceSpec(), c, o)})
	r.Register(&Global{Descriptor: df6.Global(encapHopLimitSpec(), c, o)})
	r.Register(NewLocalSid(c, owner, opts...))
	r.Register(NewPolicy(c, owner, opts...))
	r.Register(NewSteering(c, owner, opts...))
}

// Global is how Register registers the two VPP-global settings (the df6 setter for the globals
// owner, the df6 require variant otherwise): the same descriptor plus the product agent's TD-11b
// declaration that a VPP-wide setting records no ownership in any claim or boot store. The setters
// are idempotent (sr_set_encap_source / sr_set_encap_hop_limit overwrite one global variable), so
// D-076 needs no applied-once record either. F-srv6 (gap: df6's singletons declare nothing).
type Global struct{ scheduler.Descriptor }

// RecordsNoOwnership declares (dfkit/persist, TD-11b) that the global records no ownership.
func (*Global) RecordsNoOwnership() {}

// Unwrap returns the df6 descriptor (dfkit/persist walks it).
func (g *Global) Unwrap() scheduler.Descriptor { return g.Descriptor }

// DeleteOnAbsence forwards the df6 descriptor's absence rule (the require variant never deletes on
// absence): embedding the interface alone would hide it from the reconciler.
func (g *Global) DeleteOnAbsence() bool {
	if a, ok := g.Descriptor.(scheduler.AbsenceDeleter); ok {
		return a.DeleteOnAbsence()
	}
	return true
}
