package desired

// F-det44-map-dslite-cnat: `nat.det44` onto DF-3's det44 descriptors (docs/agent/descriptors/det44.md) and back.
//
//	nat.det44 (enabled: true)              → det44.enable/global {inside_vrf, outside_vrf} (write-only, D-063; never
//	                                          disabled, V9 / D-068: a rollback leaves det44 enabled and idle)
//	nat.det44.timeouts (≠ VPP defaults)    → det44.timeouts/global (VPP global, D-071)
//	nat.det44.inside[i] / outside[i]       → det44.interface/<if>/inside|outside
//	nat.det44.mappings[i] {inside,outside} → det44.map/<inside>/<outside>
//
// The ports-per-host of a mapping follow from the prefix ratio: 2^(outside bits - inside bits) hosts share one
// outside address, VPP splits the 64512 non-reserved ports evenly (det44.c snat_det_add_map), at most 15 bits.

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// Det44MaxSharingBits is VPP's limit on the inside/outside prefix length difference (det44_add_del_map: ≤ 15 bits).
const Det44MaxSharingBits = 15

// Det44PortsPerHost is the number of outside ports each inside host gets for an inside /in → outside /out mapping.
func Det44PortsPerHost(in, out int) uint32 {
	d := out - in
	if d < 0 || d > Det44MaxSharingBits {
		return 0
	}
	return (65535 - 1023) / (uint32(1) << d)
}

func det44Build(s Sink, d *ngfwv1.Det44Config, vrfID func(string) (uint32, bool)) {
	if d == nil || !d.GetEnabled() {
		return
	}
	base := Ptr("nat", "det44")
	inVRF, ok1 := natVRF(s, d.GetInsideVrf(), vrfID, base+"/insideVrf")
	outVRF, ok2 := natVRF(s, d.GetOutsideVrf(), vrfID, base+"/outsideVrf")
	if !ok1 || !ok2 {
		return
	}
	natAdd(s, det44.NameEnable, det44.Singleton, &det44.EnableSpec{InsideVRF: inVRF, OutsideVRF: outVRF}, base)
	if t := natTimeouts(d.GetTimeouts()); t.UDP != det44.DefaultTimeouts.UDP || t.TCPEstablished != det44.DefaultTimeouts.TCPEstablished ||
		t.TCPTransitory != det44.DefaultTimeouts.TCPTransitory || t.ICMP != det44.DefaultTimeouts.ICMP {
		spec := det44.TimeoutsSpec{UDP: t.UDP, TCPEstablished: t.TCPEstablished, TCPTransitory: t.TCPTransitory, ICMP: t.ICMP}
		natAdd(s, det44.NameTimeouts, det44.Singleton, &spec, base+"/timeouts")
	}
	for _, side := range []string{det44.SideInside, det44.SideOutside} {
		list := d.GetInside()
		if side == det44.SideOutside {
			list = d.GetOutside()
		}
		for i, ifName := range list {
			natAdd(s, det44.NameInterface, ifName+"/"+side, &det44.InterfaceSpec{Interface: ifName, Side: side}, Ptr("nat", "det44", side, strconv.Itoa(i)))
		}
	}
	var seenIn []netip.Prefix
	for i, m := range d.GetMappings() {
		pt := Ptr("nat", "det44", "mappings", strconv.Itoa(i))
		in, err1 := netip.ParsePrefix(m.GetInside())
		out, err2 := netip.ParsePrefix(m.GetOutside())
		switch {
		case err1 != nil || !in.Addr().Is4():
			s.Errorf(pt+"/inside", "nat.det44-valid", "%q is not an IPv4 prefix", m.GetInside())
			continue
		case err2 != nil || !out.Addr().Is4():
			s.Errorf(pt+"/outside", "nat.det44-valid", "%q is not an IPv4 prefix", m.GetOutside())
			continue
		case in.Masked() != in:
			s.Errorf(pt+"/inside", "nat.prefixes-are-networks", "%s has host bits set (network %s)", in, in.Masked())
			continue
		case out.Masked() != out:
			s.Errorf(pt+"/outside", "nat.prefixes-are-networks", "%s has host bits set (network %s)", out, out.Masked())
			continue
		case out.Bits() < in.Bits():
			s.Errorf(pt+"/outside", "nat.det44-valid", "outside %s is larger than inside %s: DET44 maps many inside hosts onto one outside address, never the reverse", out, in)
			continue
		case out.Bits()-in.Bits() > Det44MaxSharingBits:
			s.Errorf(pt+"/outside", "nat.det44-valid", "sharing ratio 2^%d exceeds VPP's 2^%d (outside /%d for inside /%d)", out.Bits()-in.Bits(), Det44MaxSharingBits, out.Bits(), in.Bits())
			continue
		}
		overlap := false
		for _, p := range seenIn {
			if p.Overlaps(in) {
				overlap = true
			}
		}
		if overlap {
			s.Errorf(pt+"/inside", "nat.det44-valid", "inside prefix %s overlaps another mapping", in)
			continue
		}
		seenIn = append(seenIn, in)
		spec := det44.MapSpec{Inside: in.String(), Outside: out.String()}
		spec.Normalize()
		natAdd(s, det44.NameMap, spec.Inside+"/"+spec.Outside, &spec, pt)
	}
}

// assembleDet44 builds `nat.det44` from retrieved objects (enable is write-only: `enabled` is reported when any
// det44 object exists; the VRFs are not retrievable and stay unset; timeouts only from the globals owner).
func assembleDet44(out *ngfwv1.NatConfig, kvs []scheduler.KV) {
	var (
		any44           bool
		timeouts        *det44.TimeoutsSpec
		inside, outside []string
		maps            []det44.MapSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case det44.NameEnable:
		case det44.NameTimeouts:
			if v, err := natcommon.Decode[det44.TimeoutsSpec](kv.Value); err == nil {
				timeouts = &v
			}
		case det44.NameInterface:
			if v, err := natcommon.Decode[det44.InterfaceSpec](kv.Value); err == nil {
				if strings.HasSuffix(v.Interface, det44.LeftoverSuffix) {
					continue // F-det44-cnat-fix: a leftover arc node is drift to repair, never a document entry
				}
				if v.Side == det44.SideInside {
					inside = append(inside, v.Interface)
				} else {
					outside = append(outside, v.Interface)
				}
			}
		case det44.NameMap:
			if v, err := natcommon.Decode[det44.MapSpec](kv.Value); err == nil {
				maps = append(maps, v)
			}
		default:
			continue
		}
		any44 = true
	}
	if !any44 {
		return
	}
	d := &ngfwv1.Det44Config{Enabled: proto.Bool(true)}
	sort.Strings(inside)
	sort.Strings(outside)
	d.Inside, d.Outside = inside, outside
	sort.Slice(maps, func(a, b int) bool { return natPrefixLess(maps[a].Inside, maps[b].Inside) })
	for _, m := range maps {
		d.Mappings = append(d.Mappings, &ngfwv1.Det44Config_Mapping{Inside: proto.String(m.Inside), Outside: proto.String(m.Outside)})
	}
	if timeouts != nil {
		t := *timeouts
		d.Timeouts = &ngfwv1.NatTimeouts{Udp: proto.Uint32(t.UDP), TcpEstablished: proto.Uint32(t.TCPEstablished), TcpTransitory: proto.Uint32(t.TCPTransitory), Icmp: proto.Uint32(t.ICMP)}
	}
	out.Det44 = d
}

// natPrefixLess orders prefixes by address, then length (unparsable ones by string).
func natPrefixLess(a, b string) bool {
	x, e1 := netip.ParsePrefix(a)
	y, e2 := netip.ParsePrefix(b)
	if e1 != nil || e2 != nil {
		return a < b
	}
	if x.Addr() != y.Addr() {
		return x.Addr().Less(y.Addr())
	}
	return x.Bits() < y.Bits()
}
