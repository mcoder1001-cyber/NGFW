package desired

// F-det44-map-dslite-cnat: `nat.map` onto DF-3's map descriptors (docs/agent/descriptors/map.md) and back.
//
//	nat.map.domains[i]                  → map.domain/<name> (tagged "<owner>:<name>")
//	nat.map.domains[i].rules[j]         → map.rule/<name>/<psid>  (LW4o6 = a MAP-E domain with ea bits 0 + per-PSID rules)
//	nat.map.parameters (≠ VPP defaults) → map.params/global (VPP global, D-071)
//	nat.map.interfaces[i] {if, mode}    → map.interface/<if>/map-e|map-t
//
// The domain `mode` is not a VPP property (map_add_domain carries no flags): map-e vs map-t is chosen per interface.
// Not applied (agent.unsupported-field warnings): parameters.tcpMss and parameters.preResolve — write-only in VPP
// 26.06 and out of this task's scope.

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// MAP domain modes.
const (
	MapModeE     = "map-e"
	MapModeT     = "map-t"
	MapModeLW4o6 = "lw4o6"
)

// mapParams turns the document's parameters into a spec; unset leaves take VPP's defaults.
func mapParams(p *vrxv1.MapParameters) mapnat.ParamsSpec {
	out := mapnat.DefaultParams
	if p == nil {
		return out
	}
	if f := p.GetFragmentation(); f != nil {
		if f.Inner != nil {
			out.FragInner = f.GetInner()
		}
		if f.IgnoreDf != nil {
			out.FragIgnoreDF = f.GetIgnoreDf()
		}
	}
	if p.IcmpSourceAddress != nil {
		out.ICMPRelaySrc = p.GetIcmpSourceAddress()
	}
	if p.Icmp6Unreachables != nil {
		out.ICMP6Unreachable = p.GetIcmp6Unreachables()
	}
	if sc := p.GetSecurityCheck(); sc != nil {
		if sc.Enabled != nil {
			out.SecurityCheck = sc.GetEnabled()
		}
		if sc.Fragments != nil {
			out.SecurityCheckFrags = sc.GetFragments()
		}
	}
	if tc := p.GetTrafficClass(); tc != nil {
		if tc.Copy != nil {
			out.TCCopy = tc.GetCopy()
		}
		if tc.Value != nil {
			out.TCClass = tc.GetValue()
		}
	}
	out.Normalize()
	return out
}

func mapBuild(s Sink, m *vrxv1.MapConfig) {
	if m == nil {
		return
	}
	base := Ptr("nat", "map")
	used := len(m.GetDomains()) > 0 || len(m.GetInterfaces()) > 0
	if pp := m.GetParameters(); pp != nil {
		if pp.TcpMss != nil {
			s.Warnf(base+"/parameters/tcpMss", ruleUnsupported, "nat.map.parameters.tcpMss is not applied by this agent build (write-only in VPP 26.06)")
		}
		if pr := pp.GetPreResolve(); pr != nil && (pr.Ipv4 != nil || pr.Ipv6 != nil) {
			s.Warnf(base+"/parameters/preResolve", ruleUnsupported, "nat.map.parameters.preResolve is not applied by this agent build (write-only in VPP 26.06)")
		}
	}
	if !used {
		return
	}
	if ps := mapParams(m.GetParameters()); ps != mapnat.DefaultParams {
		if ps.ICMPRelaySrc != "" {
			if a, err := netip.ParseAddr(ps.ICMPRelaySrc); err != nil || !a.Is4() {
				s.Errorf(base+"/parameters/icmpSourceAddress", "nat.map-valid", "%q is not an IPv4 address", ps.ICMPRelaySrc)
				return
			}
		}
		natAdd(s, mapnat.NameParams, mapnat.Singleton, &ps, base+"/parameters")
	}
	for i, d := range m.GetDomains() {
		mapDomain(s, d, i)
	}
	seen := map[string]bool{}
	for i, b := range m.GetInterfaces() {
		pt := Ptr("nat", "map", "interfaces", strconv.Itoa(i))
		mode := b.GetMode()
		if mode != MapModeE && mode != MapModeT {
			s.Errorf(pt+"/mode", "nat.map-valid", "mode %q is not \"map-e\" or \"map-t\"", mode)
			continue
		}
		if seen[b.GetInterface()] {
			s.Errorf(pt+"/interface", "nat.map-valid", "interface %q is already bound to MAP", b.GetInterface())
			continue
		}
		seen[b.GetInterface()] = true
		spec := mapnat.InterfaceSpec{Interface: b.GetInterface(), Translation: mode == MapModeT}
		natAdd(s, mapnat.NameInterface, spec.Interface+"/"+mode, &spec, pt)
	}
}

func mapDomain(s Sink, d *vrxv1.MapDomain, i int) {
	pt := Ptr("nat", "map", "domains", strconv.Itoa(i))
	v4, e1 := netip.ParsePrefix(d.GetIpv4Prefix())
	v6, e2 := netip.ParsePrefix(d.GetIpv6Prefix())
	src, e3 := netip.ParsePrefix(d.GetIpv6Source())
	mode := d.GetMode()
	switch {
	case mode != MapModeE && mode != MapModeT && mode != MapModeLW4o6:
		s.Errorf(pt+"/mode", "nat.map-valid", "mode %q is not map-e, map-t or lw4o6", mode)
		return
	case e1 != nil || !v4.Addr().Is4():
		s.Errorf(pt+"/ipv4Prefix", "nat.map-valid", "%q is not an IPv4 prefix", d.GetIpv4Prefix())
		return
	case e2 != nil || !v6.Addr().Is6():
		s.Errorf(pt+"/ipv6Prefix", "nat.map-valid", "%q is not an IPv6 prefix", d.GetIpv6Prefix())
		return
	case e3 != nil || !src.Addr().Is6():
		s.Errorf(pt+"/ipv6Source", "nat.map-valid", "%q is not an IPv6 prefix", d.GetIpv6Source())
		return
	case v4.Masked() != v4:
		s.Errorf(pt+"/ipv4Prefix", "nat.prefixes-are-networks", "%s has host bits set", v4)
		return
	case v6.Masked() != v6:
		s.Errorf(pt+"/ipv6Prefix", "nat.prefixes-are-networks", "%s has host bits set", v6)
		return
	case mode == MapModeT && src.Bits() != 64 && src.Bits() != 96:
		s.Errorf(pt+"/ipv6Source", "nat.map-valid", "MAP-T needs the DMR prefix as ipv6Source, length 64 or 96")
		return
	case mode != MapModeT && src.Bits() != 128:
		s.Errorf(pt+"/ipv6Source", "nat.map-valid", "%s needs the BR address (/128) as ipv6Source", mode)
		return
	case d.GetPsidOffset()+d.GetPsidLength() > 16:
		s.Errorf(pt+"/psidLength", "nat.map-valid", "PSID offset + length must be ≤ 16")
		return
	case v6.Bits()+int(d.GetEaBitsLength()) > 64:
		s.Errorf(pt+"/eaBitsLength", "nat.map-valid", "ipv6Prefix length + EA bits must be ≤ 64")
		return
	case mode == MapModeLW4o6 && d.GetEaBitsLength() != 0:
		s.Errorf(pt+"/eaBitsLength", "nat.map-valid", "lw4o6 uses per-PSID rules: EA bits must be 0")
		return
	}
	spec := mapnat.DomainSpec{Name: d.GetName(), IP4Prefix: v4.String(), IP6Prefix: v6.String(), IP6Src: src.String(),
		EABitsLen: d.GetEaBitsLength(), PSIDOffset: d.GetPsidOffset(), PSIDLength: d.GetPsidLength(), MTU: d.GetMtu()}
	spec.Normalize()
	natAdd(s, mapnat.NameDomain, spec.Name, &spec, pt)
	psids := map[uint32]bool{}
	for j, r := range d.GetRules() {
		rp := pt + "/rules/" + strconv.Itoa(j)
		dst, err := netip.ParseAddr(r.GetIpv6Destination())
		switch {
		case err != nil || !dst.Is6() || dst.Is4In6():
			s.Errorf(rp+"/ipv6Destination", "nat.map-valid", "%q is not an IPv6 address", r.GetIpv6Destination())
			continue
		case d.GetPsidLength() < 16 && r.GetPsid() >= 1<<d.GetPsidLength():
			s.Errorf(rp+"/psid", "nat.map-valid", "PSID %d does not fit in %d bits", r.GetPsid(), d.GetPsidLength())
			continue
		case psids[r.GetPsid()]:
			s.Errorf(rp+"/psid", "nat.map-valid", "PSID %d is used twice", r.GetPsid())
			continue
		}
		psids[r.GetPsid()] = true
		rs := mapnat.RuleSpec{Domain: spec.Name, PSID: r.GetPsid(), IP6Dst: dst.String()}
		rs.Normalize()
		natAdd(s, mapnat.NameRule, spec.Name+"/"+strconv.FormatUint(uint64(rs.PSID), 10), &rs, rp)
	}
}

// assembleMap builds `nat.map` from retrieved objects. VPP does not keep the domain mode: a domain without EA bits
// that carries rules is reported as lw4o6, a domain whose ipv6Source is not a /128 as map-t, otherwise map-e.
// Descriptions are never invented; parameters only where the globals owner retrieved non-default values.
func assembleMap(out *vrxv1.NatConfig, kvs []scheduler.KV) {
	var (
		anyMap  bool
		params  *mapnat.ParamsSpec
		domains []mapnat.DomainSpec
		rules   = map[string][]mapnat.RuleSpec{}
		ifaces  []mapnat.InterfaceSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case mapnat.NameParams:
			if v, err := natcommon.Decode[mapnat.ParamsSpec](kv.Value); err == nil {
				params = &v
			}
		case mapnat.NameDomain:
			if v, err := natcommon.Decode[mapnat.DomainSpec](kv.Value); err == nil {
				domains = append(domains, v)
			}
		case mapnat.NameRule:
			if v, err := natcommon.Decode[mapnat.RuleSpec](kv.Value); err == nil {
				rules[v.Domain] = append(rules[v.Domain], v)
			}
		case mapnat.NameInterface:
			if v, err := natcommon.Decode[mapnat.InterfaceSpec](kv.Value); err == nil {
				ifaces = append(ifaces, v)
			}
		default:
			continue
		}
		anyMap = true
	}
	if !anyMap {
		return
	}
	m := &vrxv1.MapConfig{}
	sort.Slice(domains, func(a, b int) bool { return domains[a].Name < domains[b].Name })
	for _, d := range domains {
		rs := rules[d.Name]
		mode := MapModeE
		switch {
		case d.EABitsLen == 0 && len(rs) > 0:
			mode = MapModeLW4o6
		case !mapIsHost(d.IP6Src):
			mode = MapModeT
		}
		md := &vrxv1.MapDomain{Name: proto.String(d.Name), Mode: proto.String(mode), Ipv4Prefix: proto.String(d.IP4Prefix), Ipv6Prefix: proto.String(d.IP6Prefix),
			Ipv6Source: proto.String(d.IP6Src), EaBitsLength: proto.Uint32(d.EABitsLen), PsidOffset: proto.Uint32(d.PSIDOffset), PsidLength: proto.Uint32(d.PSIDLength)}
		if d.MTU != 0 {
			md.Mtu = proto.Uint32(d.MTU)
		}
		sort.Slice(rs, func(a, b int) bool { return rs[a].PSID < rs[b].PSID })
		for _, r := range rs {
			md.Rules = append(md.Rules, &vrxv1.MapDomain_Rule{Psid: proto.Uint32(r.PSID), Ipv6Destination: proto.String(r.IP6Dst)})
		}
		m.Domains = append(m.Domains, md)
	}
	sort.Slice(ifaces, func(a, b int) bool {
		if ifaces[a].Interface != ifaces[b].Interface {
			return ifaces[a].Interface < ifaces[b].Interface
		}
		return !ifaces[a].Translation && ifaces[b].Translation
	})
	for _, i := range ifaces {
		mode := MapModeE
		if i.Translation {
			mode = MapModeT
		}
		m.Interfaces = append(m.Interfaces, &vrxv1.MapInterface{Interface: proto.String(i.Interface), Mode: proto.String(mode)})
	}
	if params != nil {
		p := *params
		mp := &vrxv1.MapParameters{
			Fragmentation:     &vrxv1.MapParameters_Fragmentation{Inner: proto.Bool(p.FragInner), IgnoreDf: proto.Bool(p.FragIgnoreDF)},
			Icmp6Unreachables: proto.Bool(p.ICMP6Unreachable),
			SecurityCheck:     &vrxv1.MapParameters_SecurityCheck{Enabled: proto.Bool(p.SecurityCheck), Fragments: proto.Bool(p.SecurityCheckFrags)},
			TrafficClass:      &vrxv1.MapParameters_TrafficClass{Copy: proto.Bool(p.TCCopy), Value: proto.Uint32(p.TCClass)},
		}
		if p.ICMPRelaySrc != "" {
			mp.IcmpSourceAddress = proto.String(p.ICMPRelaySrc)
		}
		m.Parameters = mp
	}
	out.Map = m
}

func mapIsHost(p string) bool {
	x, err := netip.ParsePrefix(p)
	return err == nil && x.Bits() == 128
}
