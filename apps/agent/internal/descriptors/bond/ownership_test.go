package bond_test

import (
	"testing"

	"ngfw/agent/internal/descriptors/bond"
	iface "ngfw/agent/internal/descriptors/interface"
)

type persistedClaims struct{ iface.ClaimStore }

func (persistedClaims) Persistent() bool { return true }

// TestOwnershipDeclarations (TD-11b rule, F-bonding fix round 1): every bond descriptor declares exactly one of
// RecordsNoOwnership (bond.bond, bond.member-weight) and CheckPersistent (bond.member, which records claims on untagged
// member NICs); bond.member's check refuses the in-memory claim store and accepts a persisted one.
func TestOwnershipDeclarations(t *testing.T) {
	type noOwnership interface{ RecordsNoOwnership() }
	type checker interface{ CheckPersistent() error }
	for _, d := range []any{bond.NewBond(nil, owner), bond.NewWeight(nil, owner)} {
		_, none := d.(noOwnership)
		_, checks := d.(checker)
		if !none || checks {
			t.Fatalf("%T: RecordsNoOwnership=%v CheckPersistent=%v", d, none, checks)
		}
	}
	md := bond.NewMember(nil, "wown")
	var d any = md
	if _, none := d.(noOwnership); none {
		t.Fatal("bond.member declares both")
	}
	iface.SetClaimStore("wown", nil)
	if err := md.CheckPersistent(); err == nil {
		t.Fatal("bond.member accepted the in-memory claim store")
	}
	iface.SetClaimStore("wown", persistedClaims{iface.NewMemoryClaimStore()})
	if err := md.CheckPersistent(); err != nil {
		t.Fatalf("persisted store refused: %v", err)
	}
	iface.SetClaimStore("wown", nil)
}
