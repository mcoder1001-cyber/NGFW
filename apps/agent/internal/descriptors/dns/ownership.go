package dns

// TD-11b ownership declarations (dfkit/persist): the VPP DNS cache is a VPP-global singleton (D-071) with no
// ownership record of any kind — the globals owner applies it, every other agent only requires it — so neither
// descriptor records claims or applied-once entries.

// RecordsNoOwnership declares that dns.name-server records no ownership (VPP-global, write-only).
func (*NameServerDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that dns.enable records no ownership (VPP-global, write-only).
func (*EnableDescriptor) RecordsNoOwnership() {}
