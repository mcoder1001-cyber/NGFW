package pppoe

import (
	"fmt"
	"net/netip"
	"testing"
)

func TestCarrierSpecIsDistinctStableAndBounded(t *testing.T) {
	a, err := NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	if err != nil || a.Validate() != nil {
		t.Fatalf("spec: %v %v", a, err)
	}
	b, _ := NewCarrierSpec("ngfw", "pppwan", "wan", 1492)
	if a != b || len(a.Token()) != 16 {
		t.Fatalf("unstable identity: %v %v", a, b)
	}
	for _, pair := range [][2]string{{a.Host4, a.VPP4()}, {a.Host6, a.VPP6()}} {
		host, peer := netip.MustParsePrefix(pair[0]), netip.MustParsePrefix(pair[1])
		if host.Masked() != peer.Masked() || host.Addr() == peer.Addr() {
			t.Fatalf("invalid transit pair: %v", pair)
		}
	}
	if !netip.MustParsePrefix("169.254.0.0/16").Contains(netip.MustParsePrefix(a.Host4).Addr()) {
		t.Fatal("IPv4 outside owned link-local allocation pool")
	}
	if _, err := NewCarrierSpec("ngfw", "wan", "wan", 1492); err == nil {
		t.Fatal("accepted raw/logical alias")
	}
	a.Host4 = "192.0.2.2/30"
	if a.Validate() == nil {
		t.Fatal("accepted modified address derivation")
	}
}

func TestCarrierSpecRefusesOverlapsAndParentReuse(t *testing.T) {
	a, _ := NewCarrierSpec("ngfw", "ppp-a", "wan-a", 1492)
	b, _ := NewCarrierSpec("ngfw", "ppp-b", "wan-b", 1492)
	if err := CheckCarrierPrefixes([]CarrierSpec{a, b}, nil); err != nil {
		t.Fatal(err)
	}
	if err := CheckCarrierPrefixes([]CarrierSpec{a}, []netip.Prefix{netip.MustParsePrefix(a.Host4)}); err == nil {
		t.Fatal("accepted live/configured address overlap")
	}
	b.Parent = a.Parent
	if err := CheckCarrierPrefixes([]CarrierSpec{a, b}, nil); err == nil {
		t.Fatal("accepted shared raw parent")
	}
	b.Parent = a.Logical
	if err := CheckCarrierPrefixes([]CarrierSpec{a, b}, nil); err == nil {
		t.Fatal("accepted carrier chain")
	}
}

func TestCarrierTapCandidatesFitVPPAndRemainStable(t *testing.T) {
	seen := map[uint32]bool{}
	for i := 0; i < 100000; i++ {
		spec, err := NewCarrierSpec("ngfw", fmt.Sprintf("ppp%d", i), "wan", 1492)
		if err != nil {
			t.Fatal(err)
		}
		raw, transit := spec.TapIDs()
		againRaw, againTransit := spec.TapIDs()
		if raw < 2 || raw > 8190 || raw%2 != 0 || transit != raw+1 || transit > 8191 || raw != againRaw || transit != againTransit {
			t.Fatalf("unsupported/unstable candidate %d/%d", raw, transit)
		}
		seen[raw] = true
	}
	if !seen[2] || !seen[8190] {
		t.Fatal("did not exercise both supported pair boundaries")
	}
}
