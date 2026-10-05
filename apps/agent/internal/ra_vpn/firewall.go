package ravpn

import (
	"fmt"
	"net/netip"
	"strings"
)

// Firewall returns an atomic, private-namespace nft transaction. The caller
// verifies namespace inode/capabilities first and existing table ownership before
// passing replace=true. It never flushes a namespace-wide or host ruleset.
func (plan *NetworkPlan) Firewall(replace bool) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	endpoint, _ := netip.ParseAddr(plan.LocalAddress)
	family := "ip"
	if !endpoint.Is4() {
		family = "ip6"
	}
	var output strings.Builder
	if replace {
		output.WriteString("delete table inet ngfw_ra\n")
	}
	fmt.Fprintf(&output, "table inet ngfw_ra {\n comment \"ngfw-ra:%s\"\n", plan.Instance)
	output.WriteString(" chain input { type filter hook input priority 0; policy drop;\n iifname \"lo\" accept\n iifname \"xfrm0\" drop\n")
	fmt.Fprintf(&output, " iifname \"outer0\" %s daddr %s udp dport { 500, 4500 } accept\n iifname \"outer0\" %s daddr %s meta l4proto esp accept\n", family, endpoint.String(), family, endpoint.String())
	output.WriteString(" iifname \"outer0\" ct state established,related accept\n iifname \"outer0\" ip protocol icmp icmp type { destination-unreachable, time-exceeded, parameter-problem } accept\n iifname \"outer0\" meta l4proto ipv6-icmp icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem } accept\n }\n chain forward { type filter hook forward priority 0; policy drop;\n")
	for _, text := range plan.Pools {
		prefix, _ := netip.ParsePrefix(text)
		kind := "ip"
		if !prefix.Addr().Is4() {
			kind = "ip6"
		}
		fmt.Fprintf(&output, " iifname \"xfrm0\" oifname \"inner0\" %s saddr %s accept\n iifname \"inner0\" oifname \"xfrm0\" %s daddr %s accept\n", kind, text, kind, text)
	}
	output.WriteString(" }\n chain output { type filter hook output priority 0; policy drop;\n oifname \"lo\" accept\n oifname \"outer0\" meta mark 1 accept\n")
	for _, server := range plan.Radius {
		fmt.Fprintf(&output, " oifname \"outer0\" %s daddr %s udp dport %d accept\n", family, server.Address, server.Port)
	}
	// Namespace-generated related ICMP errors to a client pool are bounded
	// control-plane replies, not an alternate route for protected application data.
	for _, text := range plan.Pools {
		prefix, _ := netip.ParsePrefix(text)
		kind, protocol := "ip", "icmp"
		if !prefix.Addr().Is4() {
			kind, protocol = "ip6", "ipv6-icmp"
		}
		fmt.Fprintf(&output, " oifname \"xfrm0\" %s daddr %s meta l4proto %s ct state related accept\n", kind, text, protocol)
	}
	output.WriteString(" }\n}\n")
	return []byte(output.String()), nil
}

// Sysctls is a closed list, written only in the verified isolated namespace.
// No caller-controlled sysctl names or values cross the helper boundary.
func (plan *NetworkPlan) Sysctls() (map[string]string, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return map[string]string{
		"net/ipv4/ip_forward":                    "1",
		"net/ipv4/conf/all/send_redirects":       "0",
		"net/ipv4/conf/default/send_redirects":   "0",
		"net/ipv4/conf/all/accept_redirects":     "0",
		"net/ipv4/conf/default/accept_redirects": "0",
		"net/ipv4/conf/all/rp_filter":            "0",
		"net/ipv4/conf/default/rp_filter":        "0",
		"net/ipv4/conf/outer0/rp_filter":         "0",
		"net/ipv4/conf/inner0/rp_filter":         "0",
		"net/ipv4/conf/xfrm0/rp_filter":          "0",
		"net/ipv4/conf/outer0/accept_redirects":  "0",
		"net/ipv4/conf/inner0/accept_redirects":  "0",
		"net/ipv4/conf/xfrm0/accept_redirects":   "0",
		"net/ipv4/conf/outer0/send_redirects":    "0",
		"net/ipv4/conf/inner0/send_redirects":    "0",
		"net/ipv4/conf/xfrm0/send_redirects":     "0",
		"net/ipv6/conf/all/forwarding":           "1",
		"net/ipv6/conf/all/accept_ra":            "0",
		"net/ipv6/conf/default/accept_ra":        "0",
	}, nil
}
