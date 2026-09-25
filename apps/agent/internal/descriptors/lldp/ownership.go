package lldp

// Ownership declarations for the product agent's guard (TD-11b, dfkit/persist: every registered
// descriptor declares CheckPersistent or RecordsNoOwnership, else subsystems.Register refuses to
// start the agent). This branch predates TD-11b on its base, so the check follows persist's
// structural protocol here (a store that survives an agent restart has Persistent() bool == true);
// after the rebase onto TD-11b it can call dfkit.CheckClaims / dfkit.CheckBoot instead (questions Q6).

import (
	"fmt"
	"reflect"

	iface "ngfw/agent/internal/descriptors/interface"
)

// requirePersistent returns nil when every store survives an agent restart (persist.Require's rule).
func requirePersistent(what string, stores ...any) error {
	for _, s := range stores {
		p, ok := s.(interface{ Persistent() bool })
		if !ok || s == nil || (reflect.ValueOf(s).Kind() == reflect.Pointer && reflect.ValueOf(s).IsNil()) || !p.Persistent() {
			return fmt.Errorf("ownership store does not survive an agent restart (in memory): %s: %T", what, s)
		}
	}
	return nil
}

// claimsOf is the owner's DF-1 claim store (claims on untagged interfaces, dfkit.Target.Claim).
func claimsOf(owner string) any { return iface.Claims(owner) }

// RecordsNoOwnership: lldp.global is a VPP-global singleton (globals owner only, D-071); it records
// nothing.
func (*GlobalDescriptor) RecordsNoOwnership() {}

// CheckPersistent: LLDP on an untagged interface is ours through the owner's claim store.
func (d *InterfaceDescriptor) CheckPersistent() error {
	return requirePersistent(NameInterface+": claims on untagged interfaces", claimsOf(d.Owner))
}
