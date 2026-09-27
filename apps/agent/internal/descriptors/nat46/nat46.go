// Package nat46 projects stateless NAT46 (RFC 7915 SIIT, 1:1: an IPv4 service address in front
// of an IPv6-only server) onto VPP's MAP plugin. VPP 26.06 has no NAT46 plugin; a MAP-T domain
// in 1:1 mode (IPv4 /32, IPv6 /128, ea_bits_len 0, no PSID) with ip6_src = the RFC 6052 /96 that
// embeds the IPv4 clients does the translation both ways (docs/agent/descriptors/nat46.md).
//
// The package owns no descriptor: it builds mapnat.DomainSpec / mapnat.InterfaceSpec values that
// the map.domain / map.interface descriptors (package mapnat, DF-3) apply, and assembles the
// NAT46 view back from what those descriptors retrieve. NAT46 domains carry the name prefix
// DomainPrefix so they never collide with nat.map domains in the same tag space.
package nat46

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"ngfw/agent/internal/descriptors/mapnat"
)

// DomainPrefix starts the map.domain name of every NAT46 mapping ("nat46-<mapping name>").
const DomainPrefix = "nat46-"

// WellKnownPrefix is RFC 6052's well-known prefix (64:ff9b::/96).
const WellKnownPrefix = "64:ff9b::/96"

// ErrInvalid wraps every validation error; Field names the offending leaf.
var ErrInvalid = errors.New("nat46: invalid")

// FieldError is a validation error with a path relative to the NAT46 object
// ("mappings/0/ipv4", "clientPrefix", "interfaces/1").
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return fmt.Sprintf("nat46: %s: %s", e.Field, e.Message) }

// Unwrap makes errors.Is(err, ErrInvalid) true.
func (e *FieldError) Unwrap() error { return ErrInvalid }

// Mapping is one 1:1 translation: IPv4 clients reach IPv4 (the service address) and are
// delivered to IPv6 (the IPv6-only server).
type Mapping struct {
	Name string `json:"name"`
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
	// MTU of the IPv6 side (0 = VPP default); IPv4 packets above MTU-20 are fragmented.
	MTU uint32 `json:"mtu,omitempty"`
}

// Config is the NAT46 object: the /96 that represents IPv4 clients on the IPv6 side, the
// interfaces MAP-T translation runs on (both the IPv4-facing and the IPv6-facing ones: VPP
// enables ip4-map-t and ip6-map-t together) and the mappings.
type Config struct {
	ClientPrefix string    `json:"clientPrefix"`
	Interfaces   []string  `json:"interfaces"`
	Mappings     []Mapping `json:"mappings"`
}

// Projection is what Project hands to the mapnat descriptors.
type Projection struct {
	Domains    []mapnat.DomainSpec
	Interfaces []mapnat.InterfaceSpec
}

// DomainName is the map.domain name of mapping name.
func DomainName(name string) string { return DomainPrefix + name }

// Validate checks c and returns every problem found (nil when valid).
func Validate(c Config) []*FieldError {
	var errs []*FieldError
	add := func(f, format string, a ...any) {
		errs = append(errs, &FieldError{Field: f, Message: fmt.Sprintf(format, a...)})
	}

	if len(c.Mappings) > 0 || len(c.Interfaces) > 0 {
		p, err := netip.ParsePrefix(c.ClientPrefix)
		switch {
		case err != nil:
			add("clientPrefix", "not an IPv6 prefix: %q", c.ClientPrefix)
		case !p.Addr().Is6() || p.Addr().Is4In6():
			add("clientPrefix", "must be an IPv6 prefix")
		case p.Bits() != 96:
			add("clientPrefix", "must be a /96 (RFC 6052; 1:1 SIIT embeds the whole IPv4 address in the last 32 bits)")
		case p.Masked() != p:
			add("clientPrefix", "host bits set (use %s)", p.Masked())
		}
	}
	if len(c.Mappings) > 0 && len(c.Interfaces) == 0 {
		add("interfaces", "at least one interface is required when mappings are configured")
	}
	seenIf := map[string]bool{}
	for i, n := range c.Interfaces {
		f := fmt.Sprintf("interfaces/%d", i)
		switch {
		case n == "":
			add(f, "empty interface name")
		case seenIf[n]:
			add(f, "duplicate interface %q", n)
		}
		seenIf[n] = true
	}
	names, v4s, v6s := map[string]bool{}, map[netip.Addr]bool{}, map[netip.Addr]bool{}
	for i, m := range c.Mappings {
		f := fmt.Sprintf("mappings/%d", i)
		switch {
		case m.Name == "":
			add(f+"/name", "required")
		case strings.ContainsAny(m.Name, "#/ :"):
			add(f+"/name", "must not contain '#', '/', ':' or spaces")
		case len(DomainName(m.Name)) > 60:
			add(f+"/name", "too long (the VPP tag holds 64 bytes with the owner)")
		case names[m.Name]:
			add(f+"/name", "duplicate mapping name %q", m.Name)
		}
		names[m.Name] = true
		if a, err := netip.ParseAddr(m.IPv4); err != nil || !a.Is4() {
			add(f+"/ipv4", "not an IPv4 address: %q", m.IPv4)
		} else if !a.IsGlobalUnicast() && !a.IsPrivate() {
			add(f+"/ipv4", "must be a unicast address")
		} else if v4s[a] {
			add(f+"/ipv4", "IPv4 service address %s used twice", a)
		} else {
			v4s[a] = true
		}
		if a, err := netip.ParseAddr(m.IPv6); err != nil || !a.Is6() || a.Is4In6() {
			add(f+"/ipv6", "not an IPv6 address: %q", m.IPv6)
		} else if !a.IsGlobalUnicast() {
			add(f+"/ipv6", "must be a unicast address")
		} else if v6s[a] {
			add(f+"/ipv6", "IPv6 server %s used twice", a)
		} else {
			v6s[a] = true
			if cp, err := netip.ParsePrefix(c.ClientPrefix); err == nil && cp.Contains(a) {
				add(f+"/ipv6", "server %s is inside the client prefix %s", a, cp)
			}
		}
		if m.MTU != 0 && (m.MTU < 1280 || m.MTU > 65535) {
			add(f+"/mtu", "must be 0 or 1280..65535")
		}
	}
	return errs
}

// Project validates c and builds the MAP-T objects.
func Project(c Config) (Projection, error) {
	if errs := Validate(c); len(errs) > 0 {
		return Projection{}, errs[0]
	}
	var out Projection
	if len(c.Mappings) == 0 && len(c.Interfaces) == 0 {
		return out, nil
	}
	cp := netip.MustParsePrefix(c.ClientPrefix).String()
	for _, m := range c.Mappings {
		v4 := netip.MustParseAddr(m.IPv4)
		v6 := netip.MustParseAddr(m.IPv6)
		out.Domains = append(out.Domains, mapnat.DomainSpec{
			Name:      DomainName(m.Name),
			IP4Prefix: netip.PrefixFrom(v4, 32).String(),
			IP6Prefix: netip.PrefixFrom(v6, 128).String(),
			IP6Src:    cp,
			MTU:       m.MTU,
		})
	}
	for _, n := range c.Interfaces {
		out.Interfaces = append(out.Interfaces, mapnat.InterfaceSpec{Interface: n, Translation: true})
	}
	sort.Slice(out.Domains, func(i, j int) bool { return out.Domains[i].Name < out.Domains[j].Name })
	sort.Slice(out.Interfaces, func(i, j int) bool { return out.Interfaces[i].Interface < out.Interfaces[j].Interface })
	return out, nil
}

// IsNAT46Domain reports whether a retrieved map.domain is a NAT46 mapping: the name prefix and
// the 1:1 shape (/32, /128, no EA bits, no PSID). A nat46-* domain of another shape is not
// assembled (it is reported by Assemble as foreign so the caller can surface drift).
func IsNAT46Domain(d mapnat.DomainSpec) bool {
	if !strings.HasPrefix(d.Name, DomainPrefix) || d.EABitsLen != 0 || d.PSIDLength != 0 || d.PSIDOffset != 0 {
		return false
	}
	p4, e4 := netip.ParsePrefix(d.IP4Prefix)
	p6, e6 := netip.ParsePrefix(d.IP6Prefix)
	s6, es := netip.ParsePrefix(d.IP6Src)
	return e4 == nil && e6 == nil && es == nil && p4.Bits() == 32 && p6.Bits() == 128 && s6.Bits() == 96
}

// Assemble builds the NAT46 view from retrieved map objects. interfaces is the set of MAP-T
// interfaces NAT46 owns (the caller decides; MAP-T interfaces are shared with nat.map when both
// are configured). Domains that are not NAT46 are ignored; a NAT46 domain set with more than one
// client prefix returns an error (drift: VPP holds something Project never builds).
func Assemble(domains []mapnat.DomainSpec, interfaces []mapnat.InterfaceSpec) (Config, error) {
	var c Config
	for _, d := range domains {
		if !IsNAT46Domain(d) {
			continue
		}
		src := netip.MustParsePrefix(d.IP6Src).Masked().String()
		if c.ClientPrefix == "" {
			c.ClientPrefix = src
		} else if c.ClientPrefix != src {
			return Config{}, fmt.Errorf("nat46: domains carry two client prefixes (%s, %s)", c.ClientPrefix, src)
		}
		c.Mappings = append(c.Mappings, Mapping{
			Name: strings.TrimPrefix(d.Name, DomainPrefix),
			IPv4: netip.MustParsePrefix(d.IP4Prefix).Addr().String(),
			IPv6: netip.MustParsePrefix(d.IP6Prefix).Addr().String(),
			MTU:  d.MTU,
		})
	}
	for _, i := range interfaces {
		if i.Translation {
			c.Interfaces = append(c.Interfaces, i.Interface)
		}
	}
	sort.Slice(c.Mappings, func(i, j int) bool { return c.Mappings[i].Name < c.Mappings[j].Name })
	sort.Strings(c.Interfaces)
	return c, nil
}

// ClientAddress is the IPv6 source an IPv4 client appears with on the IPv6 side
// (RFC 6052 /96: the IPv4 address in the last 32 bits).
func ClientAddress(clientPrefix, ipv4 string) (string, error) {
	p, err := netip.ParsePrefix(clientPrefix)
	if err != nil || p.Bits() != 96 || !p.Addr().Is6() {
		return "", fmt.Errorf("%w: client prefix %q is not an IPv6 /96", ErrInvalid, clientPrefix)
	}
	a, err := netip.ParseAddr(ipv4)
	if err != nil || !a.Is4() {
		return "", fmt.Errorf("%w: %q is not an IPv4 address", ErrInvalid, ipv4)
	}
	b := p.Masked().Addr().As16()
	v := a.As4()
	copy(b[12:], v[:])
	return netip.AddrFrom16(b).String(), nil
}
