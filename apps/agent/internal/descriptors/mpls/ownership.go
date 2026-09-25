package mpls

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
)

// How each MPLS descriptor records ownership, for the product agent's guard (dfkit/persist,
// TD-11b; F-mpls-srmpls registers the family): the agent refuses to start when a registered
// descriptor declares neither, or records in a store that does not survive an agent restart.

// RecordsNoOwnership declares that an MPLS table is ours by its VPP name "<owner>:<id>"; table 0
// of an agent that is not the globals owner is only required (NewTableFor), never owned.
func (*TableDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares that MPLS enabled on an untagged interface is ours by a claim in the owner's DF-1
// claim store (Target.ClaimFirst); the product wiring installs the persisted IfaceClaims.
func (d *InterfaceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameInterface, d.Owner)
}

// CheckPersistent declares that a label route in the shared table 0 is ours by its D-080 boot record in the
// owner's DF-7 BootStore (review H2); the product wiring installs Wiring.BootStore() with
// df7.SetBootStore.
func (d *RouteDescriptor) CheckPersistent() error {
	return dfkit.CheckBoot(NameRoute, df7.BootStoreFor(d.Owner))
}

// RecordsNoOwnership declares that a label binding records nothing: it is write-only (D-063),
// re-applied idempotently on every resync and removed only when it leaves the desired state.
func (*IPBindDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a tunnel is ours by its mt_tag / interface tag "<owner>:<name>".
func (*TunnelDescriptor) RecordsNoOwnership() {}
