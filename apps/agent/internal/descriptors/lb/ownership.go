package lb

// Ownership declarations for the product agent's guard (TD-11b, dfkit/persist: every registered descriptor declares
// CheckPersistent or RecordsNoOwnership, else subsystems.Register refuses to start the agent). F-lb gap: DF-7
// predates TD-11b (TestOwnershipDeclared).

import (
	"errors"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
)

// RecordsNoOwnership declares that lb.conf records nothing: it is a VPP-global singleton that only the globals owner
// registers (D-071).
func (*ConfDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires a persisted BootStore: a VIP is ours only through the D-080 ownership record written after
// our own add, so an in-memory store would lose every VIP on an agent restart (never deleted, never re-adopted).
func (d *VIPDescriptor) CheckPersistent() error {
	return dfkit.CheckBoot(NameVIP, df7.BootStoreFor(d.Owner))
}

// CheckPersistent requires a persisted BootStore (the AS ownership records, as for VIPs).
func (d *ASDescriptor) CheckPersistent() error {
	return dfkit.CheckBoot(NameAS, df7.BootStoreFor(d.Owner))
}

// CheckPersistent requires a persisted BootStore (the D-076 applied-once record of the stacking in2out feature) and a
// persisted claim store (the feature on an untagged interface is ours through the owner's claim).
func (d *IntfNatDescriptor) CheckPersistent() error {
	return errors.Join(dfkit.CheckBoot(NameIntfNat, df7.BootStoreFor(d.Owner)), dfkit.CheckClaims(NameIntfNat, d.Owner))
}
