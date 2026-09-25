package df7

import (
	"net/netip"
	"ngfw/agent/internal/descriptors/kit"
	"sort"

	"ngfw/agent/binapi/ip_types"
)

// ParseAddr parses an IP address (ErrSpec on failure), unmapping IPv4-in-IPv6.
func ParseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, Specf("address %q: %v", s, err)
	}
	return a.Unmap(), nil
}

// CanonAddr returns the canonical text of an address ("010.1.1.1" is rejected by netip).
func CanonAddr(s string) (string, error) {
	a, err := ParseAddr(s)
	if err != nil {
		return "", err
	}
	return a.String(), nil
}

// ParsePrefix parses a CIDR prefix and requires it to be canonical (host bits zero).
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := kit.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, Specf("%v", err)
	}
	return p, nil
}

// ParseIfPrefix parses an interface address with prefix length ("10.1.1.1/24"; host bits kept).
func ParseIfPrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, Specf("address/len %q: %v", s, err)
	}
	return p, nil
}

// ToAddress converts a netip address to the binapi address.
func ToAddress(a netip.Addr) ip_types.Address {
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(a.As4())}
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(a.As16())}
}

// FromAddress converts a binapi address to netip.
func FromAddress(a ip_types.Address) netip.Addr {
	if a.Af == ip_types.ADDRESS_IP6 {
		return netip.AddrFrom16(a.Un.GetIP6())
	}
	return netip.AddrFrom4(a.Un.GetIP4())
}

// ToPrefix converts a netip prefix to the binapi prefix.
func ToPrefix(p netip.Prefix) ip_types.Prefix {
	return ip_types.Prefix{Address: ToAddress(p.Addr()), Len: uint8(p.Bits())} //nolint:gosec // 0..128
}

// FromPrefix converts a binapi prefix to netip (not masked).
func FromPrefix(p ip_types.Prefix) netip.Prefix {
	return netip.PrefixFrom(FromAddress(p.Address), int(p.Len))
}

// ToAddressWithPrefix converts to the binapi address_with_prefix alias.
func ToAddressWithPrefix(p netip.Prefix) ip_types.AddressWithPrefix {
	return ip_types.AddressWithPrefix(ToPrefix(p))
}

// FromAddressWithPrefix converts a binapi address_with_prefix to netip (not masked).
func FromAddressWithPrefix(p ip_types.AddressWithPrefix) netip.Prefix {
	return FromPrefix(ip_types.Prefix(p))
}

// SortedAddrs parses, canonicalises and sorts a list of addresses (ErrSpec on a bad one or a
// duplicate).
func SortedAddrs(in []string) ([]string, error) {
	addrs := make([]netip.Addr, 0, len(in))
	for _, s := range in {
		a, err := ParseAddr(s)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	out := make([]string, 0, len(addrs))
	for i, a := range addrs {
		if i > 0 && a == addrs[i-1] {
			return nil, Specf("address %s listed twice", a)
		}
		out = append(out, a.String())
	}
	return out, nil
}

// SortAddrStrings sorts canonical address strings by address order (for Retrieve output).
func SortAddrStrings(in []string) {
	sort.Slice(in, func(i, j int) bool {
		a, errA := netip.ParseAddr(in[i])
		b, errB := netip.ParseAddr(in[j])
		if errA != nil || errB != nil {
			return in[i] < in[j]
		}
		return a.Less(b)
	})
}
