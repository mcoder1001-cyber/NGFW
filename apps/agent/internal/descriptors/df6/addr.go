package df6

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
		return netip.Addr{}, fmt.Errorf("%w: address %q: %w", ErrBadValue, s, err)
	}
	return a.Unmap().WithZone(""), nil
}

// ParseAddr4 parses an IPv4 address.
func ParseAddr4(s string) (netip.Addr, error) {
	a, err := ParseAddr(s)
	if err != nil {
		return netip.Addr{}, err
	}
	if !a.Is4() {
		return netip.Addr{}, fmt.Errorf("%w: %s is not IPv4", ErrBadValue, s)
	}
	return a, nil
}

// ParseAddr6 parses an IPv6 address.
func ParseAddr6(s string) (netip.Addr, error) {
	a, err := ParseAddr(s)
	if err != nil {
		return netip.Addr{}, err
	}
	if !a.Is6() {
		return netip.Addr{}, fmt.Errorf("%w: %s is not IPv6", ErrBadValue, s)
	}
	return a, nil
}

// ParsePrefix is kit.MaskPrefix wrapped in ErrBadValue (host bits still masked; TD-16 open question).
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := kit.MaskPrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: %w", ErrBadValue, err)
	}
	return p, nil
}

// Canonical returns the canonical string of an address string ("" stays "").
func Canonical(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	a, err := ParseAddr(s)
	if err != nil {
		return "", err
	}
	return a.String(), nil
}

// CanonicalPrefix returns the canonical (masked) string of a prefix string.
func CanonicalPrefix(s string) (string, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return "", err
	}
	return p.String(), nil
}

// ToAddress converts a to ip_types.Address.
func ToAddress(a netip.Addr) ip_types.Address {
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(a.As4())}
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(a.As16())}
}

// AddressOf parses s and converts it; "" yields the zero (unspecified IPv4) address, which
// is what VPP expects for "no address" (p2mp tunnel destination, no mcast group, …).
func AddressOf(s string) (ip_types.Address, error) {
	if strings.TrimSpace(s) == "" {
		return ip_types.Address{}, nil
	}
	a, err := ParseAddr(s)
	if err != nil {
		return ip_types.Address{}, err
	}
	return ToAddress(a), nil
}

// FromAddress converts an ip_types.Address to netip.
func FromAddress(a ip_types.Address) netip.Addr {
	if a.Af == ip_types.ADDRESS_IP6 {
		return netip.AddrFrom16(a.Un.GetIP6())
	}
	return netip.AddrFrom4(a.Un.GetIP4())
}

// AddressString is the canonical string of a; the unspecified address (0.0.0.0 / ::) is "".
func AddressString(a ip_types.Address) string {
	n := FromAddress(a)
	if n.IsUnspecified() {
		return ""
	}
	return n.String()
}

// ToPrefix converts p to ip_types.Prefix.
func ToPrefix(p netip.Prefix) ip_types.Prefix {
	return ip_types.Prefix{Address: ToAddress(p.Addr()), Len: uint8(p.Bits())} //nolint:gosec // 0..128
}

// PrefixOf parses s and converts it.
func PrefixOf(s string) (ip_types.Prefix, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return ip_types.Prefix{}, err
	}
	return ToPrefix(p), nil
}

// FromPrefix converts an ip_types.Prefix to netip (masked).
func FromPrefix(p ip_types.Prefix) netip.Prefix {
	return netip.PrefixFrom(FromAddress(p.Address), int(p.Len)).Masked()
}

// PrefixString is the canonical string of p.
func PrefixString(p ip_types.Prefix) string { return FromPrefix(p).String() }

// ToIP4 converts an IPv4 address; it errors on IPv6.
func ToIP4(a netip.Addr) (ip_types.IP4Address, error) {
	if !a.Is4() {
		return ip_types.IP4Address{}, fmt.Errorf("%w: address %s is not IPv4", ErrBadValue, a)
	}
	return ip_types.IP4Address(a.As4()), nil
}

// ToIP6 converts an IPv6 address; it errors on IPv4.
func ToIP6(a netip.Addr) (ip_types.IP6Address, error) {
	if !a.Is6() {
		return ip_types.IP6Address{}, fmt.Errorf("%w: address %s is not IPv6", ErrBadValue, a)
	}
	return ip_types.IP6Address(a.As16()), nil
}

// IP6Of parses an IPv6 address string into the binapi type; "" yields ::.
func IP6Of(s string) (ip_types.IP6Address, error) {
	if strings.TrimSpace(s) == "" {
		return ip_types.IP6Address{}, nil
	}
	a, err := ParseAddr6(s)
	if err != nil {
		return ip_types.IP6Address{}, err
	}
	return ToIP6(a)
}

// IP6String is the canonical string of a; :: is "".
func IP6String(a ip_types.IP6Address) string {
	n := netip.AddrFrom16(a)
	if n.IsUnspecified() {
		return ""
	}
	return n.String()
}

// IP4Of parses an IPv4 address string into the binapi type; "" yields 0.0.0.0.
func IP4Of(s string) (ip_types.IP4Address, error) {
	if strings.TrimSpace(s) == "" {
		return ip_types.IP4Address{}, nil
	}
	a, err := ParseAddr4(s)
	if err != nil {
		return ip_types.IP4Address{}, err
	}
	return ToIP4(a)
}

// IP4String is the canonical string of a; 0.0.0.0 is "".
func IP4String(a ip_types.IP4Address) string {
	n := netip.AddrFrom4(a)
	if n.IsUnspecified() {
		return ""
	}
	return n.String()
}

// IP6PrefixOf parses an IPv6 prefix into the binapi type.
func IP6PrefixOf(s string) (ip_types.IP6Prefix, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return ip_types.IP6Prefix{}, err
	}
	a, err := ToIP6(p.Addr())
	if err != nil {
		return ip_types.IP6Prefix{}, err
	}
	return ip_types.IP6Prefix{Address: a, Len: uint8(p.Bits())}, nil //nolint:gosec // 0..128
}

// IP4PrefixOf parses an IPv4 prefix into the binapi type.
func IP4PrefixOf(s string) (ip_types.IP4Prefix, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return ip_types.IP4Prefix{}, err
	}
	a, err := ToIP4(p.Addr())
	if err != nil {
		return ip_types.IP4Prefix{}, err
	}
	return ip_types.IP4Prefix{Address: a, Len: uint8(p.Bits())}, nil //nolint:gosec // 0..32
}

// ParseMAC parses a MAC address (any net.ParseMAC form) into the binapi type.
func ParseMAC(s string) (ethernet_types.MacAddress, error) {
	m, err := ethernet_types.ParseMacAddress(strings.TrimSpace(s))
	if err != nil {
		return ethernet_types.MacAddress{}, fmt.Errorf("%w: mac %q: %w", ErrBadValue, s, err)
	}
	return m, nil
}

// MACString is the canonical lower-case colon form of m.
func MACString(m ethernet_types.MacAddress) string { return strings.ToLower(m.String()) }

// CanonicalMAC returns the canonical form of a MAC string.
func CanonicalMAC(s string) (string, error) {
	m, err := ParseMAC(s)
	if err != nil {
		return "", err
	}
	return MACString(m), nil
}
