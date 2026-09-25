package vpn

import (
	"fmt"
	"net"
	"net/netip"
	"ngfw/agent/internal/descriptors/kit"

	"ngfw/agent/binapi/ip_types"
)

// ParseAddress parses an IP address text into the binapi address type (IPv4-mapped IPv6 is
// unmapped, so "::ffff:10.4.0.1" and "10.4.0.1" are the same address).
func ParseAddress(s string) (ip_types.Address, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return ip_types.Address{}, fmt.Errorf("vpn: address %q: %w", s, err)
	}
	a = a.Unmap()
	return ip_types.NewAddress(net.IP(a.AsSlice())), nil
}

// AddressString returns the canonical text of a binapi address ("" if it cannot be decoded).
func AddressString(a ip_types.Address) string {
	n, ok := netip.AddrFromSlice(a.ToIP())
	if !ok {
		return ""
	}
	return n.Unmap().String()
}

// IsUnspecified reports whether a is 0.0.0.0 or ::.
func IsUnspecified(a ip_types.Address) bool {
	n, ok := netip.AddrFromSlice(a.ToIP())
	return !ok || n.Unmap().IsUnspecified()
}

// CanonicalAddress returns the canonical text of s, or an error when s is not an address.
func CanonicalAddress(s string) (string, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("vpn: address %q: %w", s, err)
	}
	return a.Unmap().String(), nil
}

// ParsePrefix parses a CIDR prefix into the binapi prefix type. Host bits must be zero
// ("10.4.1.0/24", not "10.4.1.7/24") so that desired and retrieved values compare equal.
func ParsePrefix(s string) (ip_types.Prefix, error) {
	p, err := kit.ParsePrefix(s)
	if err != nil {
		return ip_types.Prefix{}, fmt.Errorf("vpn: %w", err)
	}
	addr, err := ParseAddress(p.Addr().String())
	if err != nil {
		return ip_types.Prefix{}, err
	}
	return ip_types.Prefix{Address: addr, Len: uint8(p.Bits())}, nil //nolint:gosec // 0..128
}

// PrefixString returns the canonical text of a binapi prefix.
func PrefixString(p ip_types.Prefix) string {
	n, ok := netip.AddrFromSlice(p.Address.ToIP())
	if !ok {
		return ""
	}
	return netip.PrefixFrom(n.Unmap(), int(p.Len)).Masked().String()
}

// CanonicalPrefix returns the canonical masked text of s.
func CanonicalPrefix(s string) (string, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return "", err
	}
	return PrefixString(p), nil
}
