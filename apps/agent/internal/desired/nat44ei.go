package desired

// F-nat44-ei-64-66-nptv6: `nat` with `mode: "ei"` onto DF-3's nat44-ei descriptors (docs/agent/descriptors/nat44-ei.md,
// nat-common.md), and the assembler back to vrx.v1.NatConfig. Called from nat.go under this task's anchors.
//
//	nat (NAT44 on, D-062 Nat44Enabled)      → nat44-ei.enable/global {inside/outside VRF, static-mapping-only, connection-tracking}
//	nat.timeouts (≠ VPP defaults)           → nat44-ei.timeouts/global
//	nat.forwarding: true                    → nat44-ei.forwarding/global (presence = on)
//	nat.inside[i] / nat.outside[i]          → nat44-ei.interface-feature/<if>/inside|outside
//	nat.outputFeature[i]                    → nat44-ei.output-feature/<if>
//	nat.pools[i] {range, vrf?}              → nat44-ei.address-pool/<first>-<last>/<table>
//	nat.pools[i] {interface}                → nat44-ei.interface-address/<if>
//	nat.staticMappings[i]                   → nat44-ei.static-mapping/<name> (VPP tag "<owner>:<name>")
//	nat.identityMappings[i]                 → nat44-ei.identity-mapping/id-<hash of the tuple> (IdentityMappingName)
//
// ED-only leaves (twice-NAT pools and mappings, self-twice-NAT, out2in-only, load-balanced mappings) are errors at
// their pointer (the schema's nat.mode-ed-features rejects them first; the agent refuses them on its own too).
// `sessionLimit` is a warning: nat44-ei's API enable carries no session count (startup.conf / CLI only, DF-3).
// Enable, timeouts and forwarding are VPP globals (D-071): the globals owner sets them, a test slot only requires them.

import (
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// ruleModeEDFeatures is the schema rule id of ED-only leaves in mode "ei" (packages/schema semantic/nat.ts).
const ruleModeEDFeatures = "nat.mode-ed-features"

func nat44EI(s Sink, nat *vrxv1.NatConfig, vrfID func(string) (uint32, bool)) {
	en := nat44ei.EnableSpec{StaticMappingOnly: nat.GetStaticMappingOnly(), ConnectionTracking: nat.GetConnectionTracking()}
	okIn, okOut := true, true
	if nat.InsideVrf != nil {
		en.InsideVRF, okIn = natVRF(s, nat.GetInsideVrf(), vrfID, Ptr("nat", "insideVrf"))
	}
	if nat.OutsideVrf != nil {
		en.OutsideVRF, okOut = natVRF(s, nat.GetOutsideVrf(), vrfID, Ptr("nat", "outsideVrf"))
	}
	if okIn && okOut {
		natAdd(s, nat44ei.NameEnable, nat44ei.Singleton, &en, Ptr("nat"))
	}
	if nat.SessionLimit != nil {
		s.Warnf(Ptr("nat", "sessionLimit"), ruleUnsupported, "nat44-ei takes its session limit from startup.conf (the API enable has none); sessionLimit is not applied")
	}
	if t := natTimeouts(nat.GetTimeouts()); t != nat44ed.DefaultTimeouts {
		spec := nat44ei.TimeoutsSpec{UDP: t.UDP, TCPEstablished: t.TCPEstablished, TCPTransitory: t.TCPTransitory, ICMP: t.ICMP}
		natAdd(s, nat44ei.NameTimeouts, nat44ei.Singleton, &spec, Ptr("nat", "timeouts"))
	}
	if nat.GetForwarding() {
		natAdd(s, nat44ei.NameForwarding, nat44ei.Singleton, &nat44ei.ForwardingSpec{}, Ptr("nat", "forwarding"))
	}
	for _, side := range []string{nat44ei.SideInside, nat44ei.SideOutside} {
		list := nat.GetInside()
		if side == nat44ei.SideOutside {
			list = nat.GetOutside()
		}
		for i, ifName := range list {
			natAdd(s, nat44ei.NameInterfaceFeature, ifName+"/"+side, &nat44ei.InterfaceFeatureSpec{Interface: ifName, Side: side}, Ptr("nat", side, strconv.Itoa(i)))
		}
	}
	for i, ifName := range nat.GetOutputFeature() {
		natAdd(s, nat44ei.NameOutputFeature, ifName, &nat44ei.OutputFeatureSpec{Interface: ifName}, Ptr("nat", "outputFeature", strconv.Itoa(i)))
	}
	pools := nat44EIPools(s, nat.GetPools(), vrfID)
	for i, m := range nat.GetStaticMappings() {
		nat44EIStatic(s, m, i, pools, vrfID)
	}
	for i, m := range nat.GetIdentityMappings() {
		nat44EIIdentity(s, m, i, vrfID)
	}
	for i := range nat.GetLoadBalancedMappings() {
		s.Errorf(Ptr("nat", "loadBalancedMappings", strconv.Itoa(i)), ruleModeEDFeatures, "load balancing requires mode 'ed' (nat44-ed)")
	}
}

func nat44EIPools(s Sink, pools []*vrxv1.NatPool, vrfID func(string) (uint32, bool)) map[string]natPoolRef {
	refs := map[string]natPoolRef{}
	for i, p := range pools {
		pt := Ptr("nat", "pools", strconv.Itoa(i))
		if p.GetTwiceNat() {
			s.Errorf(pt+"/twiceNat", ruleModeEDFeatures, "twice-NAT requires mode 'ed' (nat44-ed)")
			continue
		}
		switch {
		case p.Range != nil && p.Interface == nil:
			first, last, err := ParseNatRange(p.GetRange())
			if err != nil {
				s.Errorf(pt+"/range", "nat.pools-valid", "%v", err)
				continue
			}
			if n := natRangeSize(first, last); n > MaxPoolAddresses {
				s.Errorf(pt+"/range", "nat.pool-size", "pool range %q has %d addresses; VPP adds at most %d per range (split it)", p.GetRange(), n, MaxPoolAddresses)
				continue
			}
			vrf, ok := natVRF(s, p.GetVrf(), vrfID, pt+"/vrf")
			if !ok {
				continue
			}
			spec := nat44ei.AddressPoolSpec{First: first.String(), Last: last.String(), VRF: vrf}
			spec.Normalize()
			natAdd(s, nat44ei.NameAddressPool, spec.First+"-"+spec.Last+"/"+strconv.FormatUint(uint64(spec.VRF), 10), &spec, pt)
			if _, dup := refs[p.GetName()]; !dup {
				refs[p.GetName()] = natPoolRef{ip: first.String()}
			}
		case p.Interface != nil && p.Range == nil:
			natAdd(s, nat44ei.NameInterfaceAddress, p.GetInterface(), &nat44ei.InterfaceAddressSpec{Interface: p.GetInterface()}, pt)
			if _, dup := refs[p.GetName()]; !dup {
				refs[p.GetName()] = natPoolRef{iface: p.GetInterface()}
			}
		default:
			s.Errorf(pt, "nat.pools-valid", "a pool is exactly one of range / interface")
		}
	}
	return refs
}

func nat44EIStatic(s Sink, m *vrxv1.NatStaticMapping, i int, pools map[string]natPoolRef, vrfID func(string) (uint32, bool)) {
	pt := Ptr("nat", "staticMappings", strconv.Itoa(i))
	bad := false
	for _, f := range []struct {
		on   bool
		leaf string
	}{{m.GetTwiceNat(), "twiceNat"}, {m.GetSelfTwiceNat(), "selfTwiceNat"}, {m.GetOut2InOnly(), "out2inOnly"}} {
		if f.on {
			s.Errorf(pt+"/"+f.leaf, ruleModeEDFeatures, "%s requires mode 'ed' (nat44-ed)", f.leaf)
			bad = true
		}
	}
	if bad {
		return
	}
	ext := m.GetExternal()
	spec := nat44ei.StaticMappingSpec{
		Name:     m.GetName(),
		Local:    nat44ei.Endpoint{IP: m.GetLocal().GetIp(), Port: m.GetLocal().GetPort()},
		External: nat44ei.Endpoint{Port: ext.GetPort()},
	}
	switch {
	case ext.Ip != nil && ext.Pool == nil && ext.Interface == nil:
		spec.External.IP = ext.GetIp()
	case ext.Interface != nil && ext.Ip == nil && ext.Pool == nil:
		spec.External.Interface = ext.GetInterface()
	case ext.Pool != nil && ext.Ip == nil && ext.Interface == nil:
		ref, ok := pools[ext.GetPool()]
		if !ok {
			s.Errorf(pt+"/external/pool", "nat.static-mappings", "pool %q does not exist in nat.pools", ext.GetPool())
			return
		}
		spec.External.IP, spec.External.Interface = ref.ip, ref.iface
	default:
		s.Errorf(pt+"/external", "nat.static-mappings", "exactly one of external.ip, external.pool or external.interface is required")
		return
	}
	hasPorts := m.GetLocal().Port != nil || ext.Port != nil
	switch {
	case !hasPorts:
		spec.AddrOnly = true
		if m.Protocol != nil {
			s.Warnf(pt+"/protocol", ruleUnsupported, "a 1:1 mapping without ports covers every protocol; protocol %q is not applied", m.GetProtocol())
		}
	case m.GetLocal().Port == nil || ext.Port == nil:
		s.Errorf(pt, "nat.static-mappings", "local.port and external.port must be set together (omit both for a 1:1 mapping)")
		return
	case m.Protocol == nil:
		s.Errorf(pt+"/protocol", "nat.static-mappings", "protocol is required when ports are set")
		return
	default:
		spec.Protocol = m.GetProtocol()
	}
	vrf, ok := natVRF(s, m.GetVrf(), vrfID, pt+"/vrf")
	if !ok {
		return
	}
	spec.VRF = vrf
	spec.Normalize()
	natAdd(s, nat44ei.NameStaticMapping, spec.Name, &spec, pt)
}

func nat44EIIdentity(s Sink, m *vrxv1.NatIdentityMapping, i int, vrfID func(string) (uint32, bool)) {
	pt := Ptr("nat", "identityMappings", strconv.Itoa(i))
	if (m.Ip == nil) == (m.Interface == nil) {
		s.Errorf(pt, "nat.identity-mappings", "exactly one of ip or interface is required")
		return
	}
	spec := nat44ei.IdentityMappingSpec{IP: m.GetIp(), Interface: m.GetInterface()}
	switch {
	case m.Port == nil:
		spec.AddrOnly = true
		if m.Protocol != nil {
			s.Warnf(pt+"/protocol", ruleUnsupported, "an identity mapping without port covers every protocol; protocol %q is not applied", m.GetProtocol())
		}
	case m.Protocol == nil:
		s.Errorf(pt+"/protocol", "nat.identity-mappings", "protocol is required when port is set")
		return
	default:
		spec.Protocol, spec.Port = m.GetProtocol(), m.GetPort()
	}
	vrf, ok := natVRF(s, m.GetVrf(), vrfID, pt+"/vrf")
	if !ok {
		return
	}
	spec.VRF = vrf
	spec.Normalize()
	// the same stable id as NAT44-ED (a mode switch keeps the VPP tags)
	spec.Name = IdentityMappingName(nat44ed.IdentityMappingSpec{IP: spec.IP, Interface: spec.Interface, Protocol: spec.Protocol, Port: spec.Port, VRF: spec.VRF, AddrOnly: spec.AddrOnly})
	natAdd(s, nat44ei.NameIdentityMapping, spec.Name, &spec, pt)
}

// ---- assembler ------------------------------------------------------------------------------------------------

func assembleNat44EI(out *vrxv1.NatConfig, kvs []scheduler.KV, tableName func(uint32) string) {
	var (
		any44                  bool
		enable                 *nat44ei.EnableSpec
		timeouts               *nat44ei.TimeoutsSpec
		forwarding             bool
		rangePools             []nat44ei.AddressPoolSpec
		ifPools                []string
		statics                []nat44ei.StaticMappingSpec
		idents                 []nat44ei.IdentityMappingSpec
		inside, outside, outFe []string
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case nat44ei.NameEnable:
			if v, err := natcommon.Decode[nat44ei.EnableSpec](kv.Value); err == nil {
				enable = &v
			}
		case nat44ei.NameTimeouts:
			if v, err := natcommon.Decode[nat44ei.TimeoutsSpec](kv.Value); err == nil {
				timeouts = &v
			}
		case nat44ei.NameForwarding:
			forwarding = true
		case nat44ei.NameInterfaceFeature:
			if v, err := natcommon.Decode[nat44ei.InterfaceFeatureSpec](kv.Value); err == nil {
				if v.Side == nat44ei.SideInside {
					inside = append(inside, v.Interface)
				} else {
					outside = append(outside, v.Interface)
				}
			}
		case nat44ei.NameOutputFeature:
			if v, err := natcommon.Decode[nat44ei.OutputFeatureSpec](kv.Value); err == nil {
				outFe = append(outFe, v.Interface)
			}
		case nat44ei.NameAddressPool:
			if v, err := natcommon.Decode[nat44ei.AddressPoolSpec](kv.Value); err == nil {
				rangePools = append(rangePools, v)
			}
		case nat44ei.NameInterfaceAddress:
			if v, err := natcommon.Decode[nat44ei.InterfaceAddressSpec](kv.Value); err == nil {
				ifPools = append(ifPools, v.Interface)
			}
		case nat44ei.NameStaticMapping:
			if v, err := natcommon.Decode[nat44ei.StaticMappingSpec](kv.Value); err == nil {
				statics = append(statics, v)
			}
		case nat44ei.NameIdentityMapping:
			if v, err := natcommon.Decode[nat44ei.IdentityMappingSpec](kv.Value); err == nil {
				idents = append(idents, v)
			}
		default:
			continue
		}
		any44 = true
	}
	if !any44 {
		return
	}
	out.Mode = proto.String(NatModeEI)
	if enable != nil { // globals owner only (D-071)
		out.Enabled = proto.Bool(true)
		out.InsideVrf = natVRFName(enable.InsideVRF, tableName)
		out.OutsideVrf = natVRFName(enable.OutsideVRF, tableName)
		out.StaticMappingOnly = proto.Bool(enable.StaticMappingOnly)
		out.ConnectionTracking = proto.Bool(enable.ConnectionTracking)
		out.Forwarding = proto.Bool(forwarding)
		t := nat44ei.DefaultTimeouts
		if timeouts != nil {
			t = *timeouts
		}
		out.Timeouts = &vrxv1.NatTimeouts{Udp: proto.Uint32(t.UDP), TcpEstablished: proto.Uint32(t.TCPEstablished), TcpTransitory: proto.Uint32(t.TCPTransitory), Icmp: proto.Uint32(t.ICMP)}
	} else if forwarding {
		out.Forwarding = proto.Bool(true)
	}
	sort.Strings(inside)
	sort.Strings(outside)
	sort.Strings(outFe)
	out.Inside, out.Outside, out.OutputFeature = inside, outside, outFe

	sort.Slice(rangePools, func(a, b int) bool {
		x, y := rangePools[a], rangePools[b]
		if x.VRF != y.VRF {
			return x.VRF < y.VRF
		}
		return natAddrLess(x.First, y.First)
	})
	for _, p := range rangePools {
		r := p.First
		if p.Last != p.First {
			r += "-" + p.Last
		}
		out.Pools = append(out.Pools, &vrxv1.NatPool{Range: proto.String(r), Vrf: natVRFName(p.VRF, tableName), TwiceNat: proto.Bool(false)})
	}
	sort.Strings(ifPools)
	for _, ifName := range ifPools {
		out.Pools = append(out.Pools, &vrxv1.NatPool{Interface: proto.String(ifName), TwiceNat: proto.Bool(false)})
	}

	sort.Slice(statics, func(a, b int) bool { return statics[a].Name < statics[b].Name })
	for _, m := range statics {
		sm := &vrxv1.NatStaticMapping{
			Name:     proto.String(m.Name),
			Local:    &vrxv1.NatStaticMapping_Local{Ip: proto.String(m.Local.IP)},
			External: &vrxv1.NatStaticMapping_External{},
			Vrf:      natVRFName(m.VRF, tableName),
			TwiceNat: proto.Bool(false), SelfTwiceNat: proto.Bool(false), Out2InOnly: proto.Bool(false),
		}
		if m.External.Interface != "" {
			sm.External.Interface = proto.String(m.External.Interface)
		} else {
			sm.External.Ip = proto.String(m.External.IP)
		}
		if !m.AddrOnly {
			sm.Protocol = proto.String(m.Protocol)
			sm.Local.Port = proto.Uint32(m.Local.Port)
			sm.External.Port = proto.Uint32(m.External.Port)
		}
		out.StaticMappings = append(out.StaticMappings, sm)
	}

	key := func(m nat44ei.IdentityMappingSpec) string {
		return natIdentityKey(nat44ed.IdentityMappingSpec{IP: m.IP, Interface: m.Interface, Protocol: m.Protocol, Port: m.Port, VRF: m.VRF})
	}
	sort.Slice(idents, func(a, b int) bool { return key(idents[a]) < key(idents[b]) })
	for _, m := range idents {
		im := &vrxv1.NatIdentityMapping{Vrf: natVRFName(m.VRF, tableName)}
		if m.Interface != "" {
			im.Interface = proto.String(m.Interface)
		} else {
			im.Ip = proto.String(m.IP)
		}
		if !m.AddrOnly {
			im.Protocol = proto.String(m.Protocol)
			im.Port = proto.Uint32(m.Port)
		}
		out.IdentityMappings = append(out.IdentityMappings, im)
	}
}
