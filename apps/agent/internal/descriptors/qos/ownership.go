package qos

import "ngfw/agent/internal/descriptors/dfkit"

// Ownership declarations of the product agent's guard (TD-11b, dfkit/persist; F-qos-flat registers the family):
// the agent refuses to start with a descriptor that does not say how it records ownership, or that records it in
// a store that does not survive an agent restart.

// CheckPersistent is the TD-11b check: a record on an untagged interface is ours through the owner's DF-1 claim store.
func (d *RecordDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameRecord, d.Owner) }

// CheckPersistent is the TD-11b check: a store on an untagged interface is ours through the owner's DF-1 claim store.
func (d *StoreDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameStore, d.Owner) }

// CheckPersistent is the TD-11b check: a mark on an untagged interface is ours through the owner's DF-1 claim store.
func (d *MarkDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameMark, d.Owner) }

// RecordsNoOwnership declares that an egress map is ours by its id alone (df7.WithIDRange: the id range this agent
// owns on a shared VPP; production owns every id, D-071).
func (*EgressMapDescriptor) RecordsNoOwnership() {}
