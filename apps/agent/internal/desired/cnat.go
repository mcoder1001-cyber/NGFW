package desired

// F-det44-map-dslite-cnat: `nat.cnat` onto DF-3's cnat descriptors (docs/agent/descriptors/cnat.md) and back.
//
//	nat.cnat.translations[i]            → cnat.translation/<vip>/<proto>/<port> (backends = paths; the name and
//	                                       description are configuration-only labels, never retrieved)
//	nat.cnat.snat.addresses             → cnat.snat-addresses/global (VPP global, D-071; the default SNAT entry)
//	nat.cnat.snat.policy (≠ none)       → cnat.snat-policy/global (write-only, D-063; "interface" = VPP if-pfx)
//	nat.cnat.snat.interfaces[i]         → cnat.snat-interface/<if>/<table> (write-only) + cnat.interface-feature/<if>
//	nat.cnat.snat.excludePrefixes[i]    → cnat.snat-exclude-prefix/<prefix> (write-only, VPP global)
//
// V10: every snat-* object depends on the default SNAT entry; DF-3's descriptors keep the NULL-entry guards and the
// host-wide cnat lock. A policy without SNAT addresses is refused here (it would never apply).
// CNAT and NAT44-ED on the same interface are refused (open question: no supported feature ordering in 26.06).

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/cnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// Rule ids of this task's agent-side checks (mirrored by packages/schema semantic rules of the same name).
const (
	RuleCnatSnatAddress = "nat.det44-map-dslite-cnat-cnat-snat-address"
	RuleCnatNat44       = "nat.det44-map-dslite-cnat-cnat-nat44-interface"
	// RuleCnatFeature is the info notice naming the interfaces the cnat feature is derived for (questions Q4).
	RuleCnatFeature = "nat.det44-map-dslite-cnat-cnat-interface-feature"
)

// InfoSink is the optional info level of a Sink (ISSUE_SEVERITY_INFO): notices that never block an Apply.
type InfoSink interface {
	Infof(pointer, rule, format string, a ...any)
}

// infof reports an info notice when s supports it (the agent's projection does; minimal test sinks may not).
func infof(s Sink, pointer, rule, format string, a ...any) {
	if in, ok := s.(InfoSink); ok {
		in.Infof(pointer, rule, format, a...)
	}
}

var cnatPolicies = map[string]string{"none": cnat.PolicyNone, "interface": cnat.PolicyIfPfx, "k8s": cnat.PolicyK8s}

func cnatBuild(s Sink, n *ngfwv1.NatConfig) {
	c := n.GetCnat()
	if c == nil {
		return
	}
	for i, t := range c.GetTranslations() {
		cnatTranslation(s, t, i)
	}
	sn := c.GetSnat()
	if sn == nil {
		return
	}
	base := Ptr("nat", "cnat", "snat")
	polName := sn.GetPolicy()
	if polName == "" {
		polName = "none"
	}
	pol, ok := cnatPolicies[polName]
	if !ok {
		s.Errorf(base+"/policy", "nat.cnat-valid", "policy %q is not none, interface or k8s", polName)
		return
	}
	a := sn.GetAddresses()
	hasAddr := a != nil && (a.Ipv4 != nil || a.Ipv6 != nil || a.Interface != nil)
	if hasAddr {
		spec := cnat.SnatAddressesSpec{IP4: a.GetIpv4(), IP6: a.GetIpv6(), Interface: a.GetInterface()}
		if spec.Interface != "" && (spec.IP4 != "" || spec.IP6 != "") {
			s.Errorf(base+"/addresses", "nat.cnat-valid", "set either an interface or addresses, not both")
			return
		}
		if spec.IP4 != "" {
			if x, err := netip.ParseAddr(spec.IP4); err != nil || !x.Is4() {
				s.Errorf(base+"/addresses/ipv4", "nat.cnat-valid", "%q is not an IPv4 address", spec.IP4)
				return
			}
		}
		if spec.IP6 != "" {
			if x, err := netip.ParseAddr(spec.IP6); err != nil || !x.Is6() {
				s.Errorf(base+"/addresses/ipv6", "nat.cnat-valid", "%q is not an IPv6 address", spec.IP6)
				return
			}
		}
		spec.Normalize()
		natAdd(s, cnat.NameSnatAddresses, cnat.Singleton, &spec, base+"/addresses")
	}
	needsEntry := pol != cnat.PolicyNone || len(sn.GetInterfaces()) > 0 || len(sn.GetExcludePrefixes()) > 0
	if needsEntry && !hasAddr {
		s.Errorf(base+"/addresses", RuleCnatSnatAddress, "an SNAT policy, policy interfaces or excluded prefixes need SNAT addresses (the default SNAT entry, V10)")
		return
	}
	if pol != cnat.PolicyNone {
		natAdd(s, cnat.NameSnatPolicy, cnat.Singleton, &cnat.SnatPolicySpec{Policy: pol}, base+"/policy")
	}
	nat44If := map[string]bool{}
	if Nat44Enabled(n) && NatMode(n) == NatModeED {
		for _, l := range [][]string{n.GetInside(), n.GetOutside(), n.GetOutputFeature()} {
			for _, x := range l {
				nat44If[x] = true
			}
		}
	}
	feat := map[string]bool{}
	for i, b := range sn.GetInterfaces() {
		pt := base + "/interfaces/" + strconv.Itoa(i)
		if nat44If[b.GetInterface()] {
			s.Errorf(pt+"/interface", RuleCnatNat44, "interface %q is also a NAT44-ED interface: CNAT and NAT44-ED on one interface are not supported", b.GetInterface())
			continue
		}
		spec := cnat.SnatInterfaceSpec{Interface: b.GetInterface(), Table: b.GetTable()}
		natAdd(s, cnat.NameSnatInterface, spec.Interface+"/"+spec.Table, &spec, pt)
		if !feat[spec.Interface] {
			feat[spec.Interface] = true
			natAdd(s, cnat.NameInterfaceFeature, spec.Interface, &cnat.InterfaceFeatureSpec{Interface: spec.Interface}, pt)
		}
	}
	if len(feat) > 0 {
		names := make([]string, 0, len(feat))
		for n := range feat {
			names = append(names, n)
		}
		sort.Strings(names)
		infof(s, base+"/interfaces", RuleCnatFeature, "the cnat interface feature is enabled on %v (derived from snat.interfaces; no document leaf)", names)
	}
	for i, p := range sn.GetExcludePrefixes() {
		pt := base + "/excludePrefixes/" + strconv.Itoa(i)
		x, err := netip.ParsePrefix(p)
		if err != nil {
			s.Errorf(pt, "nat.cnat-valid", "%q is not a prefix", p)
			continue
		}
		if x.Masked() != x {
			s.Errorf(pt, "nat.prefixes-are-networks", "%s has host bits set (network %s)", x, x.Masked())
			continue
		}
		spec := cnat.SnatExcludePrefixSpec{Prefix: x.String()}
		spec.Normalize()
		natAdd(s, cnat.NameSnatExcludePfx, spec.Prefix, &spec, pt)
	}
}

func cnatTranslation(s Sink, t *ngfwv1.CnatTranslation, i int) {
	pt := Ptr("nat", "cnat", "translations", strconv.Itoa(i))
	vip, err := netip.ParseAddr(t.GetVip().GetIp())
	if err != nil {
		s.Errorf(pt+"/vip/ip", "nat.cnat-valid", "%q is not an IP address", t.GetVip().GetIp())
		return
	}
	if len(t.GetBackends()) == 0 {
		s.Errorf(pt+"/backends", "nat.cnat-valid", "a translation needs at least one backend (V10)")
		return
	}
	spec := cnat.TranslationSpec{VIP: vip.String(), Port: t.GetVip().GetPort(), Proto: t.GetProtocol(), LBType: t.GetLbType()}
	for j, b := range t.GetBackends() {
		ba, err := netip.ParseAddr(b.GetIp())
		if err != nil || ba.Is4() != vip.Is4() {
			s.Errorf(pt+"/backends/"+strconv.Itoa(j)+"/ip", "nat.cnat-valid", "backend %q is not an address of the VIP's family", b.GetIp())
			return
		}
		spec.Paths = append(spec.Paths, cnat.PathSpec{Dst: ba.String(), DstPort: b.GetPort()})
	}
	spec.Normalize()
	natAdd(s, cnat.NameTranslation, spec.VIP+"/"+spec.Proto+"/"+strconv.FormatUint(uint64(spec.Port), 10), &spec, pt)
}

// assembleCnat builds `nat.cnat` from retrieved objects: translations (names are configuration-only and never
// invented) and, for the globals owner, the SNAT addresses. The policy, policy interfaces and excluded prefixes are
// write-only (D-063) and never retrieved.
func assembleCnat(out *ngfwv1.NatConfig, kvs []scheduler.KV) {
	var (
		trs  []cnat.TranslationSpec
		addr *cnat.SnatAddressesSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case cnat.NameTranslation:
			if v, err := natcommon.Decode[cnat.TranslationSpec](kv.Value); err == nil {
				trs = append(trs, v)
			}
		case cnat.NameSnatAddresses:
			if v, err := natcommon.Decode[cnat.SnatAddressesSpec](kv.Value); err == nil {
				addr = &v
			}
		}
	}
	if len(trs) == 0 && addr == nil {
		return
	}
	c := &ngfwv1.CnatConfig{}
	sort.Slice(trs, func(a, b int) bool {
		if trs[a].VIP != trs[b].VIP {
			return natAddrLess(trs[a].VIP, trs[b].VIP)
		}
		if trs[a].Proto != trs[b].Proto {
			return trs[a].Proto < trs[b].Proto
		}
		return trs[a].Port < trs[b].Port
	})
	for _, t := range trs {
		ct := &ngfwv1.CnatTranslation{Protocol: proto.String(t.Proto), Vip: &ngfwv1.CnatEndpoint{Ip: proto.String(t.VIP), Port: proto.Uint32(t.Port)}, LbType: proto.String(t.LBType)}
		for _, p := range t.Paths {
			ct.Backends = append(ct.Backends, &ngfwv1.CnatEndpoint{Ip: proto.String(p.Dst), Port: proto.Uint32(p.DstPort)})
		}
		c.Translations = append(c.Translations, ct)
	}
	if addr != nil {
		a := &ngfwv1.CnatConfig_Snat_Addresses{}
		if addr.IP4 != "" {
			a.Ipv4 = proto.String(addr.IP4)
		}
		if addr.IP6 != "" {
			a.Ipv6 = proto.String(addr.IP6)
		}
		if addr.Interface != "" {
			a.Interface = proto.String(addr.Interface)
		}
		c.Snat = &ngfwv1.CnatConfig_Snat{Addresses: a}
	}
	out.Cnat = c
}
