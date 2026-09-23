package natcommon

import (
	"fmt"
	"net/netip"
	"sort"

	"ngfw/agent/binapi/ip_types"
)

// IP4 parses a canonical IPv4 address string into the binapi type.
func IP4(s string) (ip_types.IP4Address, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return ip_types.IP4Address{}, fmt.Errorf("natcommon: %q is not an IPv4 address", s)
	}
	return ip_types.IP4Address(a.As4()), nil
}

// IP6 parses a canonical IPv6 address string into the binapi type.
func IP6(s string) (ip_types.IP6Address, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is6() || a.Is4In6() {
		return ip_types.IP6Address{}, fmt.Errorf("natcommon: %q is not an IPv6 address", s)
	}
	return ip_types.IP6Address(a.As16()), nil
}

// Addr parses either family into the binapi union type.
func Addr(s string) (ip_types.Address, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return ip_types.Address{}, fmt.Errorf("natcommon: %q is not an IP address", s)
	}
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(ip_types.IP4Address(a.As4()))}, nil
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(ip_types.IP6Address(a.As16()))}, nil
}

// AddrString renders the binapi union type canonically ("" for an unset v4 0.0.0.0 is NOT
// special-cased: callers decide what zero means).
func AddrString(a ip_types.Address) string {
	if a.Af == ip_types.ADDRESS_IP6 {
		return IP6String(a.Un.GetIP6())
	}
	return IP4String(a.Un.GetIP4())
}

// IP4String renders an IPv4 address canonically.
func IP4String(a ip_types.IP4Address) string { return netip.AddrFrom4(a).String() }

// IP6String renders an IPv6 address canonically.
func IP6String(a ip_types.IP6Address) string { return netip.AddrFrom16(a).String() }

// Prefix4 parses "a.b.c.d/len" into the binapi type (address bits beyond len are masked).
func Prefix4(s string) (ip_types.IP4Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return ip_types.IP4Prefix{}, fmt.Errorf("natcommon: %q is not an IPv4 prefix", s)
	}
	p = p.Masked()
	return ip_types.IP4Prefix{Address: ip_types.IP4Address(p.Addr().As4()), Len: uint8(p.Bits())}, nil //nolint:gosec // ≤ 32
}

// Prefix6 parses "x::/len" into the binapi type (masked).
func Prefix6(s string) (ip_types.IP6Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is6() || p.Addr().Is4In6() {
		return ip_types.IP6Prefix{}, fmt.Errorf("natcommon: %q is not an IPv6 prefix", s)
	}
	p = p.Masked()
	return ip_types.IP6Prefix{Address: ip_types.IP6Address(p.Addr().As16()), Len: uint8(p.Bits())}, nil //nolint:gosec // ≤ 128
}

// Prefix parses either family into the binapi union prefix type (masked).
func Prefix(s string) (ip_types.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return ip_types.Prefix{}, fmt.Errorf("natcommon: %q is not an IP prefix", s)
	}
	p = p.Masked()
	a, err := Addr(p.Addr().String())
	if err != nil {
		return ip_types.Prefix{}, err
	}
	return ip_types.Prefix{Address: a, Len: uint8(p.Bits())}, nil //nolint:gosec // ≤ 128
}

// Prefix4String / Prefix6String / PrefixString render binapi prefixes canonically.
func Prefix4String(p ip_types.IP4Prefix) string {
	return netip.PrefixFrom(netip.AddrFrom4(p.Address), int(p.Len)).Masked().String()
}

// Prefix6String renders an IPv6 binapi prefix canonically.
func Prefix6String(p ip_types.IP6Prefix) string {
	return netip.PrefixFrom(netip.AddrFrom16(p.Address), int(p.Len)).Masked().String()
}

// PrefixString renders a union binapi prefix canonically.
func PrefixString(p ip_types.Prefix) string {
	a, err := netip.ParseAddr(AddrString(p.Address))
	if err != nil {
		return ""
	}
	return netip.PrefixFrom(a, int(p.Len)).Masked().String()
}

// CanonAddr canonicalises an address string ("010.9.0.1" → "10.9.0.1"); invalid input is
// returned unchanged so that validation fails later with a clear message.
func CanonAddr(s string) string {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap().String()
	}
	return s
}

// CanonPrefix canonicalises a prefix string (masked); invalid input is returned unchanged.
func CanonPrefix(s string) string {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked().String()
	}
	return s
}

// SortStrings sorts in place and returns s (for Normalize one-liners).
func SortStrings(s []string) []string {
	sort.Strings(s)
	return s
}

// ProtoName maps an IP protocol number to the names the NAT APIs and users share; unknown
// numbers render as the decimal number.
func ProtoName(n uint8) string {
	switch n {
	case 0:
		return "any"
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 58:
		return "icmp6"
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ProtoNumber is the inverse of ProtoName; "" and "any" are 0.
func ProtoNumber(s string) (uint8, error) {
	switch s {
	case "", "any":
		return 0, nil
	case "icmp":
		return 1, nil
	case "tcp":
		return 6, nil
	case "udp":
		return 17, nil
	case "icmp6":
		return 58, nil
	}
	var n uint8
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("natcommon: unknown protocol %q", s)
	}
	return n, nil
}

// CanonProto canonicalises a protocol string (numbers for the well-known protocols become
// their names, unknown input is returned unchanged).
func CanonProto(s string) string {
	n, err := ProtoNumber(s)
	if err != nil {
		return s
	}
	return ProtoName(n)
}
