package ip6nd

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-neighbors-ra was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// CheckPersistent declares: RA settings on an untagged interface are ours through the DF-2 claim store.
func (d *RaConfigDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(RaConfigName) }

// CheckPersistent declares: an RA prefix on an untagged interface is ours through the DF-2 claim store.
func (d *RaPrefixDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(RaPrefixName) }

// CheckPersistent declares: a proxy-ND entry on an untagged interface is ours through the DF-2 claim store
// (registered only on opt-in, D-064).
func (d *ProxyNdDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(ProxyNdName) }

// RecordsNoOwnership declares: DAD is a VPP-global setting (globals owner only, D-071).
func (*DadDescriptor) RecordsNoOwnership() {}
