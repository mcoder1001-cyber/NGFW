package subsystems

import (
	"slices"
	"testing"

	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/core/coretest"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// F-bonding: the bond family is registered in the interfaces domain on the persisted claim store, never on an
// in-memory one (review 3.2).
func TestBondingWiring(t *testing.T) {
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "wb")
	if err != nil {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	w, err := Register(r, Env{Client: coretest.New(), Owner: "wb", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{bond.BondName, bond.MemberName, bond.WeightName} {
		if !slices.Contains(r.Names(), n) || DomainOf(n) != Interfaces {
			t.Fatalf("%s: registered %v, domain %q", n, r.Names(), DomainOf(n))
		}
	}
	if iface.Claims("wb") != iface.ClaimStore(w.IfaceClaims()) {
		t.Fatal("the bond family runs on another claim store")
	}
	// a Wiring without the persisted store refuses to register the family
	iface.SetClaimStore("wb", nil)
	if err := w.registerBonding(scheduler.NewRegistry()); err == nil {
		t.Fatal("registered on an in-memory claim store")
	}
	if err := (&Wiring{env: Env{Owner: "wc"}}).registerBonding(scheduler.NewRegistry()); err == nil {
		t.Fatal("registered without any claim store")
	}
}
