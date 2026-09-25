package afpacket

// RecordsNoOwnership declares, for the product agent's guard (dfkit/persist, TD-11b fix round 1),
// that a host-interface records no ownership in any store: it is ours by its interface tag
// (iface.AcquireAndTag; an untagged one is removed again).
func (*HostInterfaceDescriptor) RecordsNoOwnership() {}
