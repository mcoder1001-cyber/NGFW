package subsystems

// F-nat44-ed-sessions: the `nat` domain's NAT44-ED family (DF-3 descriptors/nat44ed) with the persisted NAT claim
// store and the D-071 globals flag. subsystems.go carries only the registration lines under the feature's anchors.

import (
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// nat44EDDescriptors are the nat44-ed descriptors of the `nat` domain. nat44-ed.vrf-table is registered but in no
// domain: the configuration has no leaf for it (out of scope), so it is never planned.
var nat44EDDescriptors = []string{
	nat44ed.NameEnable,
	nat44ed.NameTimeouts,
	nat44ed.NameForwarding,
	nat44ed.NameInterfaceFeature,
	nat44ed.NameOutputFeature,
	nat44ed.NameAddressPool,
	nat44ed.NameInterfaceAddress,
	nat44ed.NameStaticMapping,
	nat44ed.NameIdentityMapping,
	nat44ed.NameLBStaticMapping,
}

// natDomain concatenates the descriptor groups of the `nat` domain: NAT44-ED here, the sibling families
// (F-nat44-ei-64-66-nptv6, F-det44-map-dslite-cnat) one group each under their anchors in Domains.
func natDomain(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// registerNat44ED registers the nat44-ed family with the persisted claims of the "nat" family (D-080: no in-memory
// default in the product agent) and the D-071 globals flag (a test slot only requires enable/timeouts/forwarding).
func (w *Wiring) registerNat44ED(r scheduler.Registry) error {
	claims, err := w.KeyedClaims("nat")
	if err != nil {
		return err
	}
	nat44ed.Register(r, w.env.Client, w.env.Owner, natcommon.WithGlobalsOwner(w.env.GlobalsOwner), natcommon.WithClaims(claims))
	return nil
}
