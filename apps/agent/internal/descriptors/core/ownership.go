package core

// Ownership declarations for the product agent's guard (dfkit/persist, TD-11b fix round 1, review
// M1): every registered descriptor says how it records ownership.

import (
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/ownertable"
)

// RecordsNoOwnership declares that a VRF table is ours by its name "<owner>:<vrf name>" in VPP.
func (*VRFDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a loopback is ours by its interface tag "<owner>:<name>".
func (*LoopbackDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a table binding is ours when its interface's tag is (no store
// in this build; TD-11c's claim path on untagged interfaces replaces this with CheckPersistent over
// its claim store — ownership_test.go fails until it does).
func (*InterfaceTableDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that an address is ours when its interface's tag is (see
// InterfaceTableDescriptor).
func (*InterfaceAddrDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires a persisted route owner table: a route (no tag field in VPP) is ours by
// its record there — ownertable.Open in the state dir; ownertable.NewMemory is for tests.
func (d *RouteDescriptor) CheckPersistent() error {
	if _, ok := d.Owned.(*ownertable.File); ok {
		return nil
	}
	return persist.Require("core "+RouteName+": route owner table (pass ownertable.Open(state dir))", d.Owned)
}
