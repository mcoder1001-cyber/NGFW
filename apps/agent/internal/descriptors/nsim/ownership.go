package nsim

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

// CheckPersistent: the model's applied-once record lives in the owner's BootStore.
func (d *ConfigDescriptor) CheckPersistent() error {
	return requirePersistent(ConfigName+": applied-once record", d.store)
}

// CheckPersistent: claims on untagged interfaces and the applied-once record.
func (d *CrossConnectDescriptor) CheckPersistent() error {
	return requirePersistent(CrossConnectName+": claims on untagged interfaces and applied-once record", claimsOf(d.owner), d.store)
}

// CheckPersistent: claims on untagged interfaces and the applied-once records.
func (d *OutputDescriptor) CheckPersistent() error {
	return requirePersistent(OutputName+": claims on untagged interfaces and applied-once records", claimsOf(d.owner), d.store)
}
