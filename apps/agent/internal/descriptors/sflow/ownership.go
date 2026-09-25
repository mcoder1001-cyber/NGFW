package sflow

import "ngfw/agent/internal/descriptors/dfkit"

// TD-11b declarations (dfkit/persist; gap found by F-ipfix-sflow: subsystems'
// TestRequirePersistentPerFamily failed once the family was registered).

// RecordsNoOwnership declares: sflow.global is a VPP-global singleton (D-071 role, no records).
func (*GlobalDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares: sFlow on an untagged interface is ours through the owner's DF-1 claim store,
// which must survive an agent restart (the learned hw→sw map is not ownership: it is re-learned in
// Create, V17).
func (d *InterfaceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameInterface, d.owner)
}
