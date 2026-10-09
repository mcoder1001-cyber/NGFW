package pppoe

import (
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
