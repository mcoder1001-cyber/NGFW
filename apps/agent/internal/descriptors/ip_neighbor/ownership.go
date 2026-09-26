package ipneighbor

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-neighbors-ra was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// CheckPersistent declares: a neighbour on an untagged interface is ours through the DF-2 claim store, which must
// survive an agent restart (df2.WithClaims(Wiring.KeyedClaims("acl"))).
func (d *NeighborDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(NeighborName) }

// RecordsNoOwnership declares: the neighbour limits are a VPP-global singleton (globals owner only, D-071).
func (*ConfigDescriptor) RecordsNoOwnership() {}
