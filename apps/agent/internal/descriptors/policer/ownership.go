package policer

import (
	"errors"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
)

// Ownership declarations of the product agent's guard (TD-11b, dfkit/persist; F-qos-flat registers the family):
// the agent refuses to start with a descriptor that does not say how it records ownership, or that records it
// in a store that does not survive an agent restart.

// RecordsNoOwnership declares that a policer is ours by its owner-tagged VPP name ("<owner>:<name>") alone.
func (*Descriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a bind is addressed by the owner-tagged policer name and records nothing.
func (*BindDescriptor) RecordsNoOwnership() {}

// CheckPersistent: an attachment on an untagged interface is ours through the owner's DF-1 claim store, and
// its applied-once record (D-076/D-080: never a second policer_input/output apply, never an un-apply that VPP
// did not see applied) lives in the owner's DF-7 BootStore — both must survive an agent restart.
func (d *InterfaceDescriptor) CheckPersistent() error {
	return errors.Join(dfkit.CheckClaims(NameInterface, d.Owner), dfkit.CheckBoot(NameInterface, df7.BootStoreFor(d.Owner)))
}

// CheckPersistent: classifier policing on an untagged interface is ours through the owner's DF-1 claim store.
func (d *ClassifyDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameClassify, d.Owner) }
