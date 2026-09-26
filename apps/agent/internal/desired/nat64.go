package desired

// F-nat44-ei-64-66-nptv6: `nat.nat64` onto DF-3's nat64 descriptors (docs/agent/descriptors/nat64.md) and back.
// NAT64 is also the PLAT side of 464XLAT (RFC 6877): the CLAT is the customer's, nothing extra is modelled.
//
//	nat.nat64 (enabled: true)               → nat64.enable/global (write-only, D-063; a slot only requires it, D-071)
//	nat.nat64.timeouts (≠ VPP defaults)     → nat64.timeouts/global
//	nat.nat64.inside[i] / outside[i]        → nat64.interface/<if>/inside|outside
//	nat.nat64.prefixes[i] {prefix, vrf?}    → nat64.prefix/<prefix>/<table>
//	nat.nat64.pools[i] {range, vrf?}        → nat64.pool/<first>-<last>/<table>
//	nat.nat64.staticBibs[i]                 → nat64.static-bib/<proto>/<inside ip>/<inside port>/<table>
//
// `enabled` defaults to false in the schema: a nat64 block with `enabled: false` keeps its configuration and programs
// nothing. Descriptions are configuration-only labels (never retrieved, proto.md §5).

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// ruleNat64TenantVRF warns that a NAT64 prefix or static BIB entry in a non-default VRF pins that VRF in VPP 26.06:
// nat64_add_del_prefix never unlocks the VRF's IPv6 table and nat64_add_del_static_bib_entry locks it on every add and
// delete (V-new c), so a later commit that deletes the VRF fails its verify and is rolled back, and a rollback or a
// confirmed-commit revert to a revision without the VRF fails the same way, until VPP restarts. (Pools lock and unlock
// correctly.)
const ruleNat64TenantVRF = "nat.nat64-tenant-vrf"

// nat64PrefixLengths are the RFC 6052 prefix lengths VPP accepts.
var nat64PrefixLengths = map[int]bool{32: true, 40: true, 48: true, 56: true, 64: true, 96: true}

func nat64Build(s Sink, n *vrxv1.Nat64Config, vrfID func(string) (uint32, bool)) {
	if n == nil || !n.GetEnabled() {
		return
	}
	natAdd(s, nat64.NameEnable, nat64.Singleton, &nat64.EnableSpec{}, Ptr("nat", "nat64"))
	if t := natTimeouts(n.GetTimeouts()); t.UDP != nat64.DefaultTimeouts.UDP || t.TCPEstablished != nat64.DefaultTimeouts.TCPEstablished ||
		t.TCPTransitory != nat64.DefaultTimeouts.TCPTransitory || t.ICMP != nat64.DefaultTimeouts.ICMP {
		spec := nat64.TimeoutsSpec{UDP: t.UDP, TCPEstablished: t.TCPEstablished, TCPTransitory: t.TCPTransitory, ICMP: t.ICMP}
		natAdd(s, nat64.NameTimeouts, nat64.Singleton, &spec, Ptr("nat", "nat64", "timeouts"))
	}
	for _, side := range []string{nat64.SideInside, nat64.SideOutside} {
		list := n.GetInside()
		if side == nat64.SideOutside {
			list = n.GetOutside()
		}
		for i, ifName := range list {
			natAdd(s, nat64.NameInterface, ifName+"/"+side, &nat64.InterfaceSpec{Interface: ifName, Side: side}, Ptr("nat", "nat64", side, strconv.Itoa(i)))
		}
	}
	prefixVRF := map[uint32]string{} // VPP keeps one NAT64 prefix per VRF and overwrites (nat64.c nat64_add_del_prefix)
	for i, p := range n.GetPrefixes() {
		pt := Ptr("nat", "nat64", "prefixes", strconv.Itoa(i))
		pfx, err := netip.ParsePrefix(p.GetPrefix())
		switch {
		case err != nil || !pfx.Addr().Is6() || pfx.Addr().Is4In6():
			s.Errorf(pt+"/prefix", "nat.nat64-valid", "%q is not an IPv6 prefix", p.GetPrefix())
			continue
		case !nat64PrefixLengths[pfx.Bits()]:
			s.Errorf(pt+"/prefix", "nat.nat64-valid", "NAT64 prefix length must be 32, 40, 48, 56, 64 or 96 (RFC 6052), got /%d", pfx.Bits())
			continue
		case pfx.Masked() != pfx:
			s.Errorf(pt+"/prefix", "nat.prefixes-are-networks", "%s has host bits set (network %s)", p.GetPrefix(), pfx.Masked())
			continue
		}
		vrf, ok := natVRF(s, p.GetVrf(), vrfID, pt+"/vrf")
		if !ok {
			continue
		}
		if prev, dup := prefixVRF[vrf]; dup {
			s.Errorf(pt+"/vrf", "nat.nat64-valid", "one NAT64 prefix per VRF: %s already uses this VRF (VPP would overwrite it)", prev)
			continue
		}
		prefixVRF[vrf] = pfx.String()
		nat64TenantVRFWarn(s, vrf, p.GetVrf(), pt+"/vrf")
		spec := nat64.PrefixSpec{Prefix: pfx.String(), VRF: vrf}
		spec.Normalize()
		natAdd(s, nat64.NamePrefix, fmt.Sprintf("%s/%d", spec.Prefix, spec.VRF), &spec, pt)
	}
	for i, p := range n.GetPools() {
		pt := Ptr("nat", "nat64", "pools", strconv.Itoa(i))
		first, last, err := ParseNatRange(p.GetRange())
		if err != nil {
			s.Errorf(pt+"/range", "nat.pools-valid", "%v", err)
			continue
		}
		if sz := natRangeSize(first, last); sz > MaxPoolAddresses {
			s.Errorf(pt+"/range", "nat.pool-size", "pool range %q has %d addresses; the agent adds at most %d per range (split it)", p.GetRange(), sz, MaxPoolAddresses)
			continue
		}
		vrf, ok := natVRF(s, p.GetVrf(), vrfID, pt+"/vrf")
		if !ok {
			continue
		}
		spec := nat64.PoolSpec{First: first.String(), Last: last.String(), VRF: vrf}
		spec.Normalize()
		natAdd(s, nat64.NamePool, fmt.Sprintf("%s-%s/%d", spec.First, spec.Last, spec.VRF), &spec, pt)
	}
	for i, b := range n.GetStaticBibs() {
		pt := Ptr("nat", "nat64", "staticBibs", strconv.Itoa(i))
		in, err := netip.ParseAddr(b.GetInside().GetIp())
		if err != nil || !in.Is6() || in.Is4In6() {
			s.Errorf(pt+"/inside/ip", "nat.nat64-valid", "%q is not an IPv6 address", b.GetInside().GetIp())
			continue
		}
		out, err := netip.ParseAddr(b.GetOutside().GetIp())
		if err != nil || !out.Is4() {
			s.Errorf(pt+"/outside/ip", "nat.nat64-valid", "%q is not an IPv4 address", b.GetOutside().GetIp())
			continue
		}
		if b.Protocol == nil {
			s.Errorf(pt+"/protocol", "nat.nat64-valid", "a static BIB entry needs a protocol")
			continue
		}
		vrf, ok := natVRF(s, b.GetVrf(), vrfID, pt+"/vrf")
		if !ok {
			continue
		}
		nat64TenantVRFWarn(s, vrf, b.GetVrf(), pt+"/vrf")
		spec := nat64.StaticBIBSpec{InsideIP: in.String(), InsidePort: b.GetInside().GetPort(), OutsideIP: out.String(), OutsidePort: b.GetOutside().GetPort(), Protocol: b.GetProtocol(), VRF: vrf}
		spec.Normalize()
		natAdd(s, nat64.NameStaticBIB, fmt.Sprintf("%s/%s/%d/%d", spec.Protocol, spec.InsideIP, spec.InsidePort, spec.VRF), &spec, pt)
	}
}

// nat64TenantVRFWarn warns at ptr when a NAT64 prefix or static BIB entry is bound to a non-default VRF (V-new c).
func nat64TenantVRFWarn(s Sink, vrf uint32, name, ptr string) {
	if vrf == 0 {
		return
	}
	s.Warnf(ptr, ruleNat64TenantVRF, "NAT64 in VRF %q: VPP 26.06 keeps the VRF's IPv6 table locked, so this VRF cannot be deleted until VPP restarts (V-new c); a commit, rollback or confirmed-commit revert that deletes it fails and is rolled back", name)
}

// assembleNat64 builds `nat.nat64` from retrieved objects in canonical form. `enabled` is reported when any nat64
// object exists (VPP keeps none while the plugin is disabled: the objects prove it); the timeouts only when the
// globals owner retrieved non-default values; descriptions are never invented.
func assembleNat64(out *vrxv1.NatConfig, kvs []scheduler.KV, tableName func(uint32) string) {
	var (
		any64           bool
		timeouts        *nat64.TimeoutsSpec
		inside, outside []string
		prefixes        []nat64.PrefixSpec
		pools           []nat64.PoolSpec
		bibs            []nat64.StaticBIBSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case nat64.NameEnable: // write-only: never retrieved, only present when a projection is assembled again
		case nat64.NameTimeouts:
			if v, err := natcommon.Decode[nat64.TimeoutsSpec](kv.Value); err == nil {
				timeouts = &v
			}
		case nat64.NameInterface:
			if v, err := natcommon.Decode[nat64.InterfaceSpec](kv.Value); err == nil {
				if v.Side == nat64.SideInside {
					inside = append(inside, v.Interface)
				} else {
					outside = append(outside, v.Interface)
				}
			}
		case nat64.NamePrefix:
			if v, err := natcommon.Decode[nat64.PrefixSpec](kv.Value); err == nil {
				prefixes = append(prefixes, v)
			}
		case nat64.NamePool:
			if v, err := natcommon.Decode[nat64.PoolSpec](kv.Value); err == nil {
				pools = append(pools, v)
			}
		case nat64.NameStaticBIB:
			if v, err := natcommon.Decode[nat64.StaticBIBSpec](kv.Value); err == nil {
				bibs = append(bibs, v)
			}
		default:
			continue
		}
		any64 = true
	}
	if !any64 {
		return
	}
	n := &vrxv1.Nat64Config{Enabled: proto.Bool(true)}
	sort.Strings(inside)
	sort.Strings(outside)
	n.Inside, n.Outside = inside, outside
	sort.Slice(prefixes, func(a, b int) bool {
		if prefixes[a].VRF != prefixes[b].VRF {
			return prefixes[a].VRF < prefixes[b].VRF
		}
		return prefixes[a].Prefix < prefixes[b].Prefix
	})
	for _, p := range prefixes {
		n.Prefixes = append(n.Prefixes, &vrxv1.Nat64Config_Prefix{Prefix: proto.String(p.Prefix), Vrf: natVRFName(p.VRF, tableName)})
	}
	sort.Slice(pools, func(a, b int) bool {
		if pools[a].VRF != pools[b].VRF {
			return pools[a].VRF < pools[b].VRF
		}
		return natAddrLess(pools[a].First, pools[b].First)
	})
	for _, p := range pools {
		r := p.First
		if p.Last != p.First {
			r += "-" + p.Last
		}
		n.Pools = append(n.Pools, &vrxv1.Nat64Config_Pool{Range: proto.String(r), Vrf: natVRFName(p.VRF, tableName)})
	}
	sort.Slice(bibs, func(a, b int) bool {
		x, y := bibs[a], bibs[b]
		if x.VRF != y.VRF {
			return x.VRF < y.VRF
		}
		if x.InsideIP != y.InsideIP {
			return natAddrLess(x.InsideIP, y.InsideIP)
		}
		if x.Protocol != y.Protocol {
			return x.Protocol < y.Protocol
		}
		return x.InsidePort < y.InsidePort
	})
	for _, b := range bibs {
		n.StaticBibs = append(n.StaticBibs, &vrxv1.Nat64Config_StaticBib{
			Protocol: proto.String(b.Protocol),
			Inside:   &vrxv1.Nat64Config_StaticBib_Endpoint{Ip: proto.String(b.InsideIP), Port: proto.Uint32(b.InsidePort)},
			Outside:  &vrxv1.Nat64Config_StaticBib_Endpoint{Ip: proto.String(b.OutsideIP), Port: proto.Uint32(b.OutsidePort)},
			Vrf:      natVRFName(b.VRF, tableName),
		})
	}
	if timeouts != nil { // the globals owner's non-default timeouts (defaults are "no object", a slot sees none)
		t := *timeouts
		n.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(t.UDP), TcpEstablished: proto.Uint32(t.TCPEstablished), TcpTransitory: proto.Uint32(t.TCPTransitory), Icmp: proto.Uint32(t.ICMP)}
	}
	out.Nat64 = n
}
