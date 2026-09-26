package desired

// F-nat44-ei-64-66-nptv6: `nat.nat66` onto DF-3's nat66 descriptors (docs/agent/descriptors/nat66.md) and back.
//
//	nat.nat66 (enabled: true)               → nat66.enable/global {outside VRF 0} (write-only, D-063; a slot requires it)
//	nat.nat66.inside[i] / outside[i]        → nat66.interface/<if> {side} (VPP keeps one side per interface)
//	nat.nat66.staticMappings[i]             → nat66.static-mapping/<local>/<table>
//
// The schema has no nat66 outside VRF, so the plugin is enabled with table 0. `enabled` defaults to false: a disabled
// block programs nothing.

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

func nat66Build(s Sink, n *vrxv1.Nat66Config, vrfID func(string) (uint32, bool)) {
	if n == nil || !n.GetEnabled() {
		return
	}
	natAdd(s, nat66.NameEnable, nat66.Singleton, &nat66.EnableSpec{}, Ptr("nat", "nat66"))
	for _, side := range []string{nat66.SideInside, nat66.SideOutside} {
		list := n.GetInside()
		if side == nat66.SideOutside {
			list = n.GetOutside()
		}
		for i, ifName := range list {
			natAdd(s, nat66.NameInterface, ifName, &nat66.InterfaceSpec{Interface: ifName, Side: side}, Ptr("nat", "nat66", side, strconv.Itoa(i)))
		}
	}
	for i, m := range n.GetStaticMappings() {
		pt := Ptr("nat", "nat66", "staticMappings", strconv.Itoa(i))
		local, err := netip.ParseAddr(m.GetLocal())
		if err != nil || !local.Is6() || local.Is4In6() {
			s.Errorf(pt+"/local", "nat.nat66-valid", "%q is not an IPv6 address", m.GetLocal())
			continue
		}
		ext, err := netip.ParseAddr(m.GetExternal())
		if err != nil || !ext.Is6() || ext.Is4In6() {
			s.Errorf(pt+"/external", "nat.nat66-valid", "%q is not an IPv6 address", m.GetExternal())
			continue
		}
		vrf, ok := natVRF(s, m.GetVrf(), vrfID, pt+"/vrf")
		if !ok {
			continue
		}
		spec := nat66.StaticMappingSpec{Local: local.String(), External: ext.String(), VRF: vrf}
		spec.Normalize()
		natAdd(s, nat66.NameStaticMapping, fmt.Sprintf("%s/%d", spec.Local, spec.VRF), &spec, pt)
	}
}

// assembleNat66 builds `nat.nat66`: `enabled` when any nat66 object exists (VPP holds none while disabled).
func assembleNat66(out *vrxv1.NatConfig, kvs []scheduler.KV, tableName func(uint32) string) {
	var (
		any66           bool
		inside, outside []string
		maps            []nat66.StaticMappingSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case nat66.NameEnable: // write-only: never retrieved, only present when a projection is assembled again
		case nat66.NameInterface:
			if v, err := natcommon.Decode[nat66.InterfaceSpec](kv.Value); err == nil {
				if v.Side == nat66.SideInside {
					inside = append(inside, v.Interface)
				} else {
					outside = append(outside, v.Interface)
				}
			}
		case nat66.NameStaticMapping:
			if v, err := natcommon.Decode[nat66.StaticMappingSpec](kv.Value); err == nil {
				maps = append(maps, v)
			}
		default:
			continue
		}
		any66 = true
	}
	if !any66 {
		return
	}
	n := &vrxv1.Nat66Config{Enabled: proto.Bool(true)}
	sort.Strings(inside)
	sort.Strings(outside)
	n.Inside, n.Outside = inside, outside
	sort.Slice(maps, func(a, b int) bool {
		if maps[a].VRF != maps[b].VRF {
			return maps[a].VRF < maps[b].VRF
		}
		return natAddrLess(maps[a].Local, maps[b].Local)
	})
	for _, m := range maps {
		n.StaticMappings = append(n.StaticMappings, &vrxv1.Nat66Config_StaticMapping{Local: proto.String(m.Local), External: proto.String(m.External), Vrf: natVRFName(m.VRF, tableName)})
	}
	out.Nat66 = n
}
