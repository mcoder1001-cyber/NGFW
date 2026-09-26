package lcp

// Ownership declarations for the product agent's guard (TD-11b, dfkit/persist: every registered descriptor declares
// CheckPersistent or RecordsNoOwnership, else subsystems.Register refuses to start the agent).

import "ngfw/agent/internal/descriptors/dfkit"

// CheckPersistent declares that the pairs record ownership: a pair on an untagged interface (a DPDK NIC) is ours
// through DF-1's claim store (dfkit Target.Claim), which must survive an agent restart.
func (d *ItfPairDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameItfPair, d.owner) }

// RecordsNoOwnership declares that the default-namespace singleton keeps no claim or boot record: it is one
// VPP-global setting found by its fixed key.
func (*DefaultNetnsDescriptor) RecordsNoOwnership() {}
