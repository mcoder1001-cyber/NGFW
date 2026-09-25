package dfkit

import (
	"net"
	"net/netip"
	"ngfw/agent/internal/descriptors/kit"

	"ngfw/agent/binapi/ip_types"
)

// ParseAddr parses and canonicalises an IP address (IPv4-mapped IPv6 is unmapped).
func ParseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, Specf("address %q: %v", s, err)
	}
	if a.Zone() != "" {
		return netip.Addr{}, Specf("address %q: zones are not supported", s)
	}
	return a.Unmap(), nil
}

// ParsePrefix applies kit.ParsePrefix (host bits rejected) as a spec error.
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := kit.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, Specf("%v", err)
	}
	return p, nil
}

// ToAPIAddress converts a netip.Addr to the binapi address type.
func ToAPIAddress(a netip.Addr) ip_types.Address {
	return ip_types.NewAddress(net.IP(a.AsSlice()))
}

// FromAPIAddress converts a binapi address to a canonical netip.Addr (invalid for AF garbage).
func FromAPIAddress(a ip_types.Address) netip.Addr {
	if a.Af == ip_types.ADDRESS_IP4 {
		ip4 := a.Un.GetIP4()
		return netip.AddrFrom4(ip4)
	}
	ip6 := a.Un.GetIP6()
	return netip.AddrFrom16(ip6)
}

// Family returns "ip4" or "ip6", the family segment used in DF-8 keys.
func Family(a netip.Addr) string {
	if a.Is4() {
		return "ip4"
	}
	return "ip6"
}
