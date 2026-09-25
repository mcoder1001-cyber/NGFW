package flowprobe

import "ngfw/agent/internal/descriptors/dfkit"

// TD-11b declarations (dfkit/persist; gap found by F-ipfix-sflow: subsystems'
// TestRequirePersistentPerFamily failed once the family was registered).

// RecordsNoOwnership declares: flowprobe.params is a VPP-global singleton (D-071 role, no records).
func (*ParamsDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares: flowprobe on an untagged interface is ours through the owner's DF-1 claim store,
// which must survive an agent restart.
func (d *InterfaceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameInterface, d.owner)
}
