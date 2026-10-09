package multiwan

import "net/netip"

// CarrierProbeTarget is the bounded carrier broker's endpoint contract. There
// is no resolver exception, alternate port, IPv6 path or arbitrary device.
func CarrierProbeTarget(kind, target string) bool {
	if kind != "icmp" && kind != "http" && kind != "dns" {
		return false
	}
	address, err := netip.ParseAddr(target)
	return err == nil && address.Is4() && !address.IsUnspecified() && !address.IsMulticast() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && address.String() != "255.255.255.255"
}
