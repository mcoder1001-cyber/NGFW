package sr_mpls //nolint:revive // see policy.go

import (
	"fmt"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/dfkit/persist"
)

// CheckPersistent is the product agent's guard (dfkit/persist, TD-11b; F-mpls-srmpls registers the
// family): the per-boot applied-once claims of endpoint/color assignments are keyed by binding SID,
// so they need a persisted store keyed by id — df6.WithClaims(Wiring.PairClaims("df6")) — not the
// in-memory default and not the interface-bound IfaceClaims (every Claim of that store fails).
// The policy and steering descriptors are df6.KeyedDescriptors, which declare the same check.
func (d *EndpointColorDescriptor) CheckPersistent() error {
	if err := persist.Require(EndpointColorName+": applied-once claims (pass df6.WithClaims(Wiring.PairClaims(\"df6\")))", d.claims); err != nil {
		return err
	}
	if b, ok := d.claims.(interface{ BindsInterfaceIndex() bool }); ok && b.BindsInterfaceIndex() {
		return fmt.Errorf("%w: %s: %T", df6.ErrClaimStoreKind, EndpointColorName, d.claims)
	}
	return nil
}
