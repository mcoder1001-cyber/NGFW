package pppoe

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestAllocateDelegationStableAndContained(t *testing.T) {
	targets := []DelegationTarget{{Interface: "lan-b", SubnetID: 255}, {Interface: "lan-a", SubnetID: 1}}
	lease := netip.MustParsePrefix("2001:db8:1234:5600::/56")
	got, err := AllocateDelegation(lease, targets)
	if err != nil {
		t.Fatal(err)
	}
	want := []DelegationAssignment{
		{Interface: "lan-a", Prefix: netip.MustParsePrefix("2001:db8:1234:5601::/64"), Router: netip.MustParsePrefix("2001:db8:1234:5601::1/64")},
		{Interface: "lan-b", Prefix: netip.MustParsePrefix("2001:db8:1234:56ff::/64"), Router: netip.MustParsePrefix("2001:db8:1234:56ff::1/64")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	targets[0], targets[1] = targets[1], targets[0]
	reordered, err := AllocateDelegation(lease, targets)
	if err != nil || !reflect.DeepEqual(got, reordered) {
		t.Fatalf("reordering changed allocation: %v %v", reordered, err)
	}
	for bits := 16; bits <= 64; bits++ {
		delegated := netip.PrefixFrom(lease.Addr(), bits).Masked()
		last := (uint64(1) << uint(64-bits)) - 1
		assignments, err := AllocateDelegation(delegated, []DelegationTarget{{Interface: "lan", SubnetID: last}})
		if err != nil || len(assignments) != 1 {
			t.Fatalf("/%d boundary allocation: %v %v", bits, assignments, err)
		}
		a := assignments[0]
		if a.Prefix.Bits() != 64 || !delegated.Contains(a.Prefix.Addr()) || !a.Prefix.Contains(a.Router.Addr()) || a.Router.Addr() == a.Prefix.Addr() {
			t.Fatalf("/%d escaped its delegation: %v", bits, a)
		}
		if invalid, err := AllocateDelegation(delegated, []DelegationTarget{{Interface: "lan", SubnetID: last + 1}}); err == nil || invalid != nil {
			t.Fatalf("/%d accepted overflow: %v %v", bits, invalid, err)
		}
	}
}

func TestAllocateDelegationRejectsWholeConflictingPlan(t *testing.T) {
	lease := netip.MustParsePrefix("2001:db8:100::/56")
	for _, targets := range [][]DelegationTarget{
		{{Interface: "lan", SubnetID: 1}, {Interface: "lan", SubnetID: 2}},
		{{Interface: "lan-a", SubnetID: 1}, {Interface: "lan-b", SubnetID: 1}},
		{{Interface: "lan", SubnetID: 0}, {Interface: "", SubnetID: 1}},
		{{Interface: "lan", SubnetID: 0}, {Interface: "overflow", SubnetID: 256}},
		make([]DelegationTarget, 1025),
	} {
		if got, err := AllocateDelegation(lease, targets); err == nil || got != nil {
			t.Fatalf("partial or accepted invalid plan: %v %v", got, err)
		}
	}
	for _, raw := range []string{"192.0.2.0/24", "::ffff:192.0.2.0/120", "fe80::/64", "ff00::/16", "2001:db8::/65", "2000::/15"} {
		if got, err := AllocateDelegation(netip.MustParsePrefix(raw), []DelegationTarget{{Interface: "lan"}}); err == nil || got != nil {
			t.Fatalf("accepted invalid lease %s: %v %v", raw, got, err)
		}
	}
	if got, err := AllocateDelegation(netip.Prefix{}, nil); err == nil || got != nil {
		t.Fatalf("accepted expired/absent lease: %v %v", got, err)
	}
}

func TestAllocateDelegationRenewalAndSingleSubnet(t *testing.T) {
	targets := []DelegationTarget{{Interface: "lan", SubnetID: 0}}
	for _, raw := range []string{"2001:db8:1::1234/64", "fd01:abcd:2::/64"} {
		lease := netip.MustParsePrefix(raw)
		got, err := AllocateDelegation(lease, targets)
		if err != nil || len(got) != 1 || got[0].Prefix != lease.Masked() {
			t.Fatalf("renewed /64 allocation %s: %v %v", raw, got, err)
		}
	}
}
