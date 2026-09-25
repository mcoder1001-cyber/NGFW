package df2

import (
	"fmt"
	"net/netip"
	"ngfw/agent/internal/descriptors/kit"
	"strings"

	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/ip_types"
)

// ParseAddr parses an IP address into its canonical form (IPv4-mapped IPv6 unmapped, zone
// dropped) so that one address has exactly one key.
func ParseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("address %q: %w", s, err)
	}
	return a.Unmap().WithZone(""), nil
}

// ParsePrefix is kit.MaskPrefix: host bits are still masked here because key canonicalisation
// in dependent families relies on it (TD-16 open question); kit.ParsePrefix is the target policy.
func ParsePrefix(s string) (netip.Prefix, error) {
	return kit.MaskPrefix(s)
}

// FamilyOf returns the desired-state address family of a.
func FamilyOf(a netip.Addr) AddressFamily {
	if a.Is4() {
		return AddressFamily_IPV4
	}
	return AddressFamily_IPV6
}

// ToIPTypesAF converts the desired-state family to the binapi one.
func ToIPTypesAF(af AddressFamily) ip_types.AddressFamily {
	if af == AddressFamily_IPV6 {
		return ip_types.ADDRESS_IP6
	}
	return ip_types.ADDRESS_IP4
}

// FromIPTypesAF converts the binapi family to the desired-state one.
func FromIPTypesAF(af ip_types.AddressFamily) AddressFamily {
	if af == ip_types.ADDRESS_IP6 {
		return AddressFamily_IPV6
	}
	return AddressFamily_IPV4
}

// ToAddress converts a to ip_types.Address.
func ToAddress(a netip.Addr) ip_types.Address {
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(a.As4())}
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(a.As16())}
}

// FromAddress converts an ip_types.Address to netip.
func FromAddress(a ip_types.Address) netip.Addr {
	if a.Af == ip_types.ADDRESS_IP6 {
		return netip.AddrFrom16(a.Un.GetIP6())
	}
	return netip.AddrFrom4(a.Un.GetIP4())
}

// ToPrefix converts p to ip_types.Prefix.
func ToPrefix(p netip.Prefix) ip_types.Prefix {
	return ip_types.Prefix{Address: ToAddress(p.Addr()), Len: uint8(p.Bits())} //nolint:gosec // 0..128
}

// FromPrefix converts an ip_types.Prefix to netip (masked).
func FromPrefix(p ip_types.Prefix) netip.Prefix {
	return netip.PrefixFrom(FromAddress(p.Address), int(p.Len)).Masked()
}

// ToIP4 converts an IPv4 address; it errors on IPv6.
func ToIP4(a netip.Addr) (ip_types.IP4Address, error) {
	if !a.Is4() {
		return ip_types.IP4Address{}, fmt.Errorf("address %s is not IPv4", a)
	}
	return ip_types.IP4Address(a.As4()), nil
}

// ToIP6 converts an IPv6 address; it errors on IPv4.
func ToIP6(a netip.Addr) (ip_types.IP6Address, error) {
	if !a.Is6() {
		return ip_types.IP6Address{}, fmt.Errorf("address %s is not IPv6", a)
	}
	return ip_types.IP6Address(a.As16()), nil
}

// ParseMAC parses a MAC address (any net.ParseMAC form) into the binapi type.
func ParseMAC(s string) (ethernet_types.MacAddress, error) {
	m, err := ethernet_types.ParseMacAddress(strings.TrimSpace(s))
	if err != nil {
		return ethernet_types.MacAddress{}, fmt.Errorf("mac %q: %w", s, err)
	}
	return m, nil
}

// MACString is the canonical lower-case colon form of m.
func MACString(m ethernet_types.MacAddress) string { return strings.ToLower(m.String()) }
