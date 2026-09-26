package subsystems

// F-bonding wiring: DF-1's bond family (bond.bond, bond.member) plus F-bonding's bond.member-weight, all in the
// interfaces domain (a bond is an interface).

import (
	"fmt"

	"ngfw/agent/internal/descriptors/bond"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names of the bond family in Domains[Interfaces].
const (
	bondingBond   = bond.BondName
	bondingMember = bond.MemberName
	bondingWeight = bond.WeightName
)

// registerBonding registers the bond family. bond.member claims an untagged member NIC through iface.Claims(owner),
// the process-wide store of the owner; Register has installed the persisted IfaceClaims there (D-075, D-080), and
// this check makes that dependency explicit — product wiring never runs the family on an in-memory claim store
// (wave-A-hotspots §0.5, review 3.2).
func (w *Wiring) registerBonding(r scheduler.Registry) error {
	if w.ifaceClaim == nil || iface.Claims(w.env.Owner) != iface.ClaimStore(w.ifaceClaim) {
		return fmt.Errorf("bonding: the persisted interface claim store of owner %q is not installed", w.env.Owner)
	}
	bond.Register(r, w.env.Client, w.env.Owner)           // DF-1: bond.bond, bond.member
	r.Register(bond.NewWeight(w.env.Client, w.env.Owner)) // F-bonding: bond.member-weight, after the membership
	return nil
}
