package acl

import "net/netip"

// parsePrefixLoose canonicalises any CIDR text (tests build sweeps from an address + length).
func parsePrefixLoose(s string) (string, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", err
	}
	return p.Masked().String(), nil
}
