package desired

// F-nat46: `nat.nat46` (stateless SIIT 1:1) onto DF-3's map descriptors through the descriptors/nat46 projection
// library (docs/agent/descriptors/nat46.md) and back.
//
//	nat.nat46.mappings[i]   → map.domain/nat46-<name> (IPv4 /32 ↔ IPv6 /128, ea_bits_len 0, ip6_src = clientPrefix)
//	nat.nat46.interfaces[i] → map.interface/<if>/map-t — shared with nat.map: when nat.map binds the same interface
//	                          map-t, nat.map's builder emits the one key and NAT46 does not (one object, two owners)
//
// Ownership of shared MAP-T interfaces on the way back: VPP keeps no owner for map_if_enable_disable, so the assembler
// uses the stored desired state (SetNat46Owners, called by the service after a successful apply/revert and at start,
// never from a projection): a retrieved map-t interface is reported under nat.nat46 when the stored nat46 lists it and
// under nat.map when the stored nat.map lists it or nat46 does not (foreign bindings stay visible as nat.map drift).

import (
	"net/netip"
	"sort"
	"strconv"
	"sync"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/nat46"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// RuleNat46 is the issue rule of every NAT46 refusal.
const RuleNat46 = "nat.nat46-valid"

var nat46Owners struct {
	sync.Mutex
	nat46, mapT map[string]bool
}

// SetNat46Owners records which map-t interfaces the STORED desired state gives to nat.nat46 and to nat.map.
func SetNat46Owners(nat *ngfwv1.NatConfig) {
	n46, mt := map[string]bool{}, map[string]bool{}
	for _, i := range nat.GetNat46().GetInterfaces() {
		n46[i] = true
	}
	for _, b := range nat.GetMap().GetInterfaces() {
		if b.GetMode() == MapModeT {
			mt[b.GetInterface()] = true
		}
	}
	nat46Owners.Lock()
	nat46Owners.nat46, nat46Owners.mapT = n46, mt
	nat46Owners.Unlock()
}

// nat46Config is the library view of the document object; an unset clientPrefix is RFC 6052's well-known prefix.
func nat46Config(n *ngfwv1.Nat46Config) nat46.Config {
	c := nat46.Config{ClientPrefix: n.GetClientPrefix(), Interfaces: n.GetInterfaces()}
	if n.ClientPrefix == nil {
		c.ClientPrefix = nat46.WellKnownPrefix
	}
	for _, m := range n.GetMappings() {
		c.Mappings = append(c.Mappings, nat46.Mapping{Name: m.GetName(), IPv4: m.GetIpv4(), IPv6: m.GetIpv6(), MTU: m.GetMtu()})
	}
	return c
}

func nat46Build(s Sink, nat *ngfwv1.NatConfig) {
	n := nat.GetNat46()
	if n == nil {
		return
	}
	base := Ptr("nat", "nat46")
	c := nat46Config(n)
	bad := false
	for _, e := range nat46.Validate(c) {
		s.Errorf(base+"/"+e.Field, RuleNat46, "%s", e.Message)
		bad = true
	}
	// Cross-checks with nat.map: one VPP domain table (a /32 inside a MAP rule prefix would be shadowed or steal
	// traffic by longest match) and one MAP mode per interface.
	for i, m := range c.Mappings {
		a, err := netip.ParseAddr(m.IPv4)
		if err != nil {
			continue
		}
		for _, d := range nat.GetMap().GetDomains() {
			if p, err := netip.ParsePrefix(d.GetIpv4Prefix()); err == nil && p.Contains(a) {
				s.Errorf(base+"/mappings/"+strconv.Itoa(i)+"/ipv4", RuleNat46, "%s is inside MAP domain %q (%s)", a, d.GetName(), p)
				bad = true
			}
		}
	}
	mapT := map[string]bool{}
	for _, b := range nat.GetMap().GetInterfaces() {
		if b.GetMode() == MapModeT {
			mapT[b.GetInterface()] = true
		}
	}
	for i, ifn := range c.Interfaces {
		for _, b := range nat.GetMap().GetInterfaces() {
			if b.GetInterface() == ifn && b.GetMode() == MapModeE {
				s.Errorf(base+"/interfaces/"+strconv.Itoa(i), RuleNat46, "interface %q is a MAP-E interface in nat.map; NAT46 needs translation (map-t)", ifn)
				bad = true
			}
		}
	}
	if bad {
		return
	}
	proj, err := nat46.Project(c)
	if err != nil {
		s.Errorf(base, RuleNat46, "%v", err)
		return
	}
	idx := map[string]int{}
	for i, m := range c.Mappings {
		idx[nat46.DomainName(m.Name)] = i
	}
	for _, d := range proj.Domains {
		d.Normalize()
		natAdd(s, mapnat.NameDomain, d.Name, &d, base+"/mappings/"+strconv.Itoa(idx[d.Name]))
	}
	pos := map[string]int{}
	for i, n := range c.Interfaces {
		pos[n] = i
	}
	for _, sp := range proj.Interfaces {
		if mapT[sp.Interface] {
			continue // nat.map emits the shared map.interface/<if>/map-t key
		}
		natAdd(s, mapnat.NameInterface, sp.Interface+"/"+MapModeT, &sp, base+"/interfaces/"+strconv.Itoa(pos[sp.Interface]))
	}
}

// assembleNat46 adds `nat.nat46` and moves NAT46-owned map-t interfaces out of the assembled nat.map (runs after
// assembleMap).
func assembleNat46(out *ngfwv1.NatConfig, kvs []scheduler.KV) {
	nat46Owners.Lock()
	own46, ownMap := nat46Owners.nat46, nat46Owners.mapT
	nat46Owners.Unlock()
	var (
		domains []mapnat.DomainSpec
		ifaces  []mapnat.InterfaceSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case mapnat.NameDomain:
			if v, err := natcommon.Decode[mapnat.DomainSpec](kv.Value); err == nil && nat46.IsNAT46Domain(v) {
				domains = append(domains, v)
			}
		case mapnat.NameInterface:
			if v, err := natcommon.Decode[mapnat.InterfaceSpec](kv.Value); err == nil && v.Translation && own46[v.Interface] {
				ifaces = append(ifaces, v)
			}
		}
	}
	if m := out.GetMap(); m != nil {
		keep := m.Interfaces[:0]
		for _, b := range m.GetInterfaces() {
			if b.GetMode() == MapModeT && own46[b.GetInterface()] && !ownMap[b.GetInterface()] {
				continue
			}
			keep = append(keep, b)
		}
		m.Interfaces = keep
		if len(m.Interfaces) == 0 {
			m.Interfaces = nil
		}
		if proto.Size(m) == 0 {
			out.Map = nil
		}
	}
	if len(domains) == 0 && len(ifaces) == 0 {
		return
	}
	c, err := nat46.Assemble(domains, ifaces)
	if err != nil {
		// Two client prefixes: something Project never builds. Report every domain under the first prefix's
		// config would hide it; leave the mappings out so the drift is visible.
		c = nat46.Config{}
		for _, i := range ifaces {
			c.Interfaces = append(c.Interfaces, i.Interface)
		}
		sort.Strings(c.Interfaces)
	}
	n := &ngfwv1.Nat46Config{Interfaces: c.Interfaces}
	if c.ClientPrefix != "" {
		n.ClientPrefix = proto.String(c.ClientPrefix)
	}
	for _, m := range c.Mappings {
		nm := &ngfwv1.Nat46Mapping{Name: proto.String(m.Name), Ipv4: proto.String(m.IPv4), Ipv6: proto.String(m.IPv6)}
		if m.MTU != 0 {
			nm.Mtu = proto.Uint32(m.MTU)
		}
		n.Mappings = append(n.Mappings, nm)
	}
	out.Nat46 = n
}
