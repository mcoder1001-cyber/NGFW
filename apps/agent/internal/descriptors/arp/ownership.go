package arp

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-neighbors-ra was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// CheckPersistent declares: proxy ARP on an untagged interface is ours through the DF-2 claim store.
func (d *InterfaceDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(InterfaceName) }

// RecordsNoOwnership declares: a proxy-ARP range is ours through its table id (the agent id range, TD-8b).
func (*RangeDescriptor) RecordsNoOwnership() {}
