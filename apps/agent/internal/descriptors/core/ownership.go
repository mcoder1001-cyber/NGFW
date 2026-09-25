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

// CheckPersistent requires a persisted claim store: a table binding on an untagged interface is ours
// by its claim in Env.Claims (TD-11c claim path, D-071/D-080; the product passes
// subsystems.IfaceClaims). A tagged interface's binding is ours by the tag.
func (d *InterfaceTableDescriptor) CheckPersistent() error {
	return persist.Require("core "+InterfaceTableName+": claims", d.Claims)
}

// CheckPersistent requires a persisted claim store: an address on an untagged interface is ours by
// its claim in Env.Claims (see InterfaceTableDescriptor).
func (d *InterfaceAddrDescriptor) CheckPersistent() error {
	return persist.Require("core "+InterfaceAddrName+": claims", d.Claims)
}

// CheckPersistent requires a persisted route owner table: a route (no tag field in VPP) is ours by
// its record there — ownertable.Open in the state dir; ownertable.NewMemory is for tests.
func (d *RouteDescriptor) CheckPersistent() error {
	if _, ok := d.Owned.(*ownertable.File); ok {
		return nil
	}
	return persist.Require("core "+RouteName+": route owner table (pass ownertable.Open(state dir))", d.Owned)
}
