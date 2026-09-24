package desired

// The `nat` domain builder (F-nat44-ed-sessions): NAT44 with `mode: "ed"` onto DF-3's nat44-ed descriptors
// (docs/agent/descriptors/nat44-ed.md, nat-common.md), and the assembler back to vrx.v1.NatConfig.
//
//	nat (NAT44 on, D-062 Nat44Enabled)      → nat44-ed.enable/global {sessions ← sessionLimit, inside/outside VRF}
//	nat.timeouts (≠ VPP defaults)           → nat44-ed.timeouts/global
//	nat.forwarding: true                    → nat44-ed.forwarding/global (presence = on)
//	nat.inside[i] / nat.outside[i]          → nat44-ed.interface-feature/<if>/inside|outside
//	nat.outputFeature[i]                    → nat44-ed.output-feature/<if>
//	nat.pools[i] {range, vrf?, twiceNat}    → nat44-ed.address-pool/<first>-<last>/<table>[/twice-nat]
//	nat.pools[i] {interface, twiceNat}      → nat44-ed.interface-address/<if>
//	nat.staticMappings[i]                   → nat44-ed.static-mapping/<name> (VPP tag "<owner>:<name>")
//	nat.identityMappings[i]                 → nat44-ed.identity-mapping/id-<hash of the tuple> (the schema has no name)
//	nat.loadBalancedMappings[i]             → nat44-ed.lb-static-mapping/<name>
//
// The enable, timeouts and forwarding objects are VPP globals (D-071): the product agent (globals owner) sets them,
// a test slot only requires them. Every interface reference goes through the `interface/<name>` alias (the DF-3
// descriptors depend on natcommon.InterfaceDep = "interface/<name>"), every VRF through its table id ("vrf/<id>").
// A pool without `vrf` is the default VRF (tenant table 0; the proto comment "unset = default").
//
// Not projected here (agent.unsupported-field warnings): `mode: "ei"` and the sibling translators — they are
// appended by F-nat44-ei-64-66-nptv6 and F-det44-map-dslite-cnat under their anchors below — and `nat.ipfix`
// (NAT IPFIX logging, owner undecided); `staticMappingOnly` / `connectionTracking` (VPP 26.06 answers UNSUPPORTED).
// Pool names and every `description` are configuration-only labels: VPP pools carry no tag, so Retrieve leaves them
// unset (docs/contracts/proto.md §5 "never invented").

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// NAT44 modes (`nat.mode`).
const (
	NatModeED = "ed"
	NatModeEI = "ei"
)

// ruleUnsupported is the DryRun warning rule of a leaf this build does not apply (the API's drift view skips it).
const ruleUnsupported = "agent.unsupported-field"

// MaxPoolAddresses is VPP's limit for one nat44_add_del_address_range.
const MaxPoolAddresses = 1024

// Nat44Enabled is packages/schema's isNat44Enabled() (D-062): the explicit `enabled` when set, otherwise true
// exactly when any NAT44 object (inside/outside/output-feature interface, pool, mapping) is configured. Callers
// never read `nat.enabled` raw.
func Nat44Enabled(n *vrxv1.NatConfig) bool {
	if n == nil {
		return false
	}
	if n.Enabled != nil {
		return n.GetEnabled()
	}
	return len(n.GetInside())+len(n.GetOutside())+len(n.GetOutputFeature())+len(n.GetPools())+
		len(n.GetStaticMappings())+len(n.GetIdentityMappings())+len(n.GetLoadBalancedMappings()) > 0
}

// NatMode is `nat.mode` with its schema default ("ed").
func NatMode(n *vrxv1.NatConfig) string {
	if n.GetMode() == "" {
		return NatModeED
	}
	return n.GetMode()
}

// Nat emits the objects of `nat`. vrfID maps a VRF name to its table id ("" and "default" → 0; false: unknown).
// Dispatch: NAT44-ED here; every other translator is reported as not applied, in the sibling groups below.
func Nat(s Sink, nat *vrxv1.NatConfig, vrfID func(string) (uint32, bool)) {
	if nat == nil {
		return
	}
	on, mode := Nat44Enabled(nat), NatMode(nat)
	switch {
	case mode != NatModeED && mode != NatModeEI:
		s.Errorf(Ptr("nat", "mode"), "nat.mode", "mode %q is not \"ed\" or \"ei\"", mode)
	case on && mode == NatModeED:
		nat44ED(s, nat, vrfID)
	}
	// NAT IPFIX logging: owner undecided (wave-BC-launch-queue.md M7), outside both sibling groups.
	natUnsupported(s, "ipfix", nat.GetIpfix(), "NAT IPFIX logging (F-ipfix-sflow)")
	// Sibling translators, one line each; a sibling replaces its lines by its builder call (anchors: two groups).
	// wave-A: F-nat44-ei-64-66-nptv6
	if on && mode == NatModeEI {
		s.Warnf(Ptr("nat", "mode"), ruleUnsupported, "NAT44-EI (nat.mode \"ei\") is not applied by this agent build (F-nat44-ei-64-66-nptv6); nothing of nat44 is programmed")
	}
	natUnsupported(s, "nat64", nat.GetNat64(), "F-nat44-ei-64-66-nptv6")
	natUnsupported(s, "nat66", nat.GetNat66(), "F-nat44-ei-64-66-nptv6")
	natUnsupported(s, "nptv6", nat.GetNptv6(), "F-nat44-ei-64-66-nptv6")
	// wave-BC: F-det44-map-dslite-cnat
	natUnsupported(s, "det44", nat.GetDet44(), "F-det44-map-dslite-cnat")
	natUnsupported(s, "dslite", nat.GetDslite(), "F-det44-map-dslite-cnat")
	natUnsupported(s, "map", nat.GetMap(), "F-det44-map-dslite-cnat")
	natUnsupported(s, "cnat", nat.GetCnat(), "F-det44-map-dslite-cnat")
	// pnat (NatConfig 27, F-det44-map-dslite-cnat): its builder call goes here.
}

// natUnsupported warns about a present, non-empty translator this build does not apply.
func natUnsupported(s Sink, key string, m proto.Message, owner string) {
	if m == nil || !m.ProtoReflect().IsValid() || proto.Size(m) == 0 {
		return
	}
	s.Warnf(Ptr("nat", key), ruleUnsupported, "nat.%s is not applied by this agent build (%s)", key, owner)
}

// natAdd encodes a DF-3 spec into its carrier and adds it.
func natAdd[T any](s Sink, d string, id string, spec *T, pointer string) {
	v, err := natcommon.Encode(spec)
	if err != nil {
		s.Errorf(pointer, "nat.encode", "%v", err)
		return
	}
	s.Add(scheduler.Join(d, id), v, pointer)
}

// natVRF resolves an optional VRF name at pointer ("" / "default" → 0).
func natVRF(s Sink, name string, vrfID func(string) (uint32, bool), pointer string) (uint32, bool) {
	id, ok := vrfID(name)
	if !ok {
		s.Errorf(pointer, "nat.vrfs-exist", "VRF %q does not exist", name)
	}
	return id, ok
}

func nat44ED(s Sink, nat *vrxv1.NatConfig, vrfID func(string) (uint32, bool)) {
	// enable (VPP global, D-071): the plugin's session limit and inside/outside VRFs
	en := nat44ed.EnableSpec{Sessions: nat.GetSessionLimit()}
	okIn, okOut := true, true
	if nat.InsideVrf != nil {
		en.InsideVRF, okIn = natVRF(s, nat.GetInsideVrf(), vrfID, Ptr("nat", "insideVrf"))
	}
	if nat.OutsideVrf != nil {
		en.OutsideVRF, okOut = natVRF(s, nat.GetOutsideVrf(), vrfID, Ptr("nat", "outsideVrf"))
	}
	if okIn && okOut {
		natAdd(s, nat44ed.NameEnable, nat44ed.Singleton, &en, Ptr("nat"))
	}
	if nat.GetStaticMappingOnly() {
		s.Warnf(Ptr("nat", "staticMappingOnly"), ruleUnsupported, "static-mapping-only is UNSUPPORTED by nat44-ed in VPP 26.06 and is not applied")
	}
	if nat.GetConnectionTracking() {
		s.Warnf(Ptr("nat", "connectionTracking"), ruleUnsupported, "connection-tracking is UNSUPPORTED by nat44-ed in VPP 26.06 and is not applied")
	}
	if t := natTimeouts(nat.GetTimeouts()); t != nat44ed.DefaultTimeouts {
		natAdd(s, nat44ed.NameTimeouts, nat44ed.Singleton, &t, Ptr("nat", "timeouts"))
	}
	if nat.GetForwarding() {
		natAdd(s, nat44ed.NameForwarding, nat44ed.Singleton, &nat44ed.ForwardingSpec{}, Ptr("nat", "forwarding"))
	}

	for _, side := range []string{nat44ed.SideInside, nat44ed.SideOutside} {
		list := nat.GetInside()
		if side == nat44ed.SideOutside {
			list = nat.GetOutside()
		}
		for i, ifName := range list {
			spec := nat44ed.InterfaceFeatureSpec{Interface: ifName, Side: side}
			natAdd(s, nat44ed.NameInterfaceFeature, ifName+"/"+side, &spec, Ptr("nat", side, strconv.Itoa(i)))
		}
	}
	for i, ifName := range nat.GetOutputFeature() {
		natAdd(s, nat44ed.NameOutputFeature, ifName, &nat44ed.OutputFeatureSpec{Interface: ifName}, Ptr("nat", "outputFeature", strconv.Itoa(i)))
	}

	pools := natPools(s, nat.GetPools(), vrfID)
	for i, m := range nat.GetStaticMappings() {
		natStatic(s, m, i, pools, vrfID)
	}
	for i, m := range nat.GetIdentityMappings() {
		natIdentity(s, m, i, vrfID)
	}
	for i, m := range nat.GetLoadBalancedMappings() {
		natLB(s, m, i, vrfID)
	}
}

// natTimeouts fills unset timeouts with VPP's defaults (the Zod defaults are the same values).
func natTimeouts(t *vrxv1.NatTimeouts) nat44ed.TimeoutsSpec {
	out := nat44ed.DefaultTimeouts
	if t == nil {
		return out
	}
	if t.Udp != nil {
		out.UDP = t.GetUdp()
	}
	if t.TcpEstablished != nil {
		out.TCPEstablished = t.GetTcpEstablished()
	}
	if t.TcpTransitory != nil {
		out.TCPTransitory = t.GetTcpTransitory()
	}
	if t.Icmp != nil {
		out.ICMP = t.GetIcmp()
	}
	return out
}

// ParseNatRange splits a pool range "a.b.c.d-a.b.c.e" (or one address) into canonical IPv4 ends.
func ParseNatRange(r string) (first, last netip.Addr, err error) {
	lo, hi, found := strings.Cut(strings.TrimSpace(r), "-")
	if !found {
		hi = lo
	}
	if first, err = netip.ParseAddr(strings.TrimSpace(lo)); err != nil || !first.Is4() {
		return first, last, fmt.Errorf("pool range %q: %q is not an IPv4 address", r, lo)
	}
	if last, err = netip.ParseAddr(strings.TrimSpace(hi)); err != nil || !last.Is4() {
		return first, last, fmt.Errorf("pool range %q: %q is not an IPv4 address", r, hi)
	}
	if last.Less(first) {
		return first, last, fmt.Errorf("pool range %q ends before it starts", r)
	}
	return first, last, nil
}

// natPoolRef is what a static mapping's `external.pool` stands for: a range pool's start address or an
// interface pool's interface.
type natPoolRef struct{ ip, iface string }

func natPools(s Sink, pools []*vrxv1.NatPool, vrfID func(string) (uint32, bool)) map[string]natPoolRef {
	refs := map[string]natPoolRef{}
	for i, p := range pools {
		pt := Ptr("nat", "pools", strconv.Itoa(i))
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
			spec := nat44ed.AddressPoolSpec{First: first.String(), Last: last.String(), VRF: vrf, TwiceNAT: p.GetTwiceNat()}
			spec.Normalize()
			natAdd(s, nat44ed.NameAddressPool, nat44ed.PoolID(spec), &spec, pt)
			if _, dup := refs[p.GetName()]; !dup {
				refs[p.GetName()] = natPoolRef{ip: first.String()}
			}
		case p.Interface != nil && p.Range == nil:
			spec := nat44ed.InterfaceAddressSpec{Interface: p.GetInterface(), TwiceNAT: p.GetTwiceNat()}
			natAdd(s, nat44ed.NameInterfaceAddress, p.GetInterface(), &spec, pt)
			if _, dup := refs[p.GetName()]; !dup {
				refs[p.GetName()] = natPoolRef{iface: p.GetInterface()}
			}
		default:
			s.Errorf(pt, "nat.pools-valid", "a pool is exactly one of range / interface")
		}
	}
	return refs
}

func natRangeSize(first, last netip.Addr) uint64 {
	a, b := first.As4(), last.As4()
	x := uint64(a[0])<<24 | uint64(a[1])<<16 | uint64(a[2])<<8 | uint64(a[3])
	y := uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
	return y - x + 1
}

func natStatic(s Sink, m *vrxv1.NatStaticMapping, i int, pools map[string]natPoolRef, vrfID func(string) (uint32, bool)) {
	pt := Ptr("nat", "staticMappings", strconv.Itoa(i))
	ext := m.GetExternal()
	spec := nat44ed.StaticMappingSpec{
		Name:     m.GetName(),
		Local:    nat44ed.Endpoint{IP: m.GetLocal().GetIp(), Port: m.GetLocal().GetPort()},
		External: nat44ed.Endpoint{Port: ext.GetPort()},
		TwiceNAT: m.GetTwiceNat(), SelfTwiceNAT: m.GetSelfTwiceNat(), Out2InOnly: m.GetOut2InOnly(),
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
		spec.AddrOnly = true // 1:1: every protocol and port of the address
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
	natAdd(s, nat44ed.NameStaticMapping, spec.Name, &spec, pt)
}

// IdentityMappingName is the stable object id of an identity mapping (the schema has no name; the VPP tag needs
// one): "id-" + 16 hex digits of the SHA-256 of its tuple, so a reordered list keeps its objects.
func IdentityMappingName(spec nat44ed.IdentityMappingSpec) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d|%v", spec.IP, spec.Interface, spec.Protocol, spec.Port, spec.VRF, spec.AddrOnly)))
	return "id-" + hex.EncodeToString(sum[:8])
}

func natIdentity(s Sink, m *vrxv1.NatIdentityMapping, i int, vrfID func(string) (uint32, bool)) {
	pt := Ptr("nat", "identityMappings", strconv.Itoa(i))
	if (m.Ip == nil) == (m.Interface == nil) {
		s.Errorf(pt, "nat.identity-mappings", "exactly one of ip or interface is required")
		return
	}
	spec := nat44ed.IdentityMappingSpec{IP: m.GetIp(), Interface: m.GetInterface()}
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
	spec.Name = IdentityMappingName(spec)
	natAdd(s, nat44ed.NameIdentityMapping, spec.Name, &spec, pt)
}

func natLB(s Sink, m *vrxv1.NatLoadBalancedMapping, i int, vrfID func(string) (uint32, bool)) {
	pt := Ptr("nat", "loadBalancedMappings", strconv.Itoa(i))
	spec := nat44ed.LBStaticMappingSpec{
		Name:     m.GetName(),
		External: nat44ed.Endpoint{IP: m.GetExternal().GetIp(), Port: m.GetExternal().GetPort()},
		Protocol: m.GetProtocol(), Affinity: m.GetAffinity(),
		TwiceNAT: m.GetTwiceNat(), SelfTwiceNAT: m.GetSelfTwiceNat(), Out2InOnly: m.GetOut2InOnly(),
	}
	for j, l := range m.GetLocals() {
		vrf, ok := natVRF(s, l.GetVrf(), vrfID, Ptr("nat", "loadBalancedMappings", strconv.Itoa(i), "locals", strconv.Itoa(j), "vrf"))
		if !ok {
			return
		}
		prob := l.GetProbability()
		if l.Probability == nil {
			prob = 1 // the schema default
		}
		spec.Locals = append(spec.Locals, nat44ed.LBLocal{IP: l.GetIp(), Port: l.GetPort(), Probability: prob, VRF: vrf})
	}
	natAdd(s, nat44ed.NameLBStaticMapping, spec.Name, &spec, pt)
}

// ---- assembler ------------------------------------------------------------------------------------------------

// AssembleNat builds `nat` from retrieved objects in canonical form (docs/contracts/proto.md §5): sorted lists,
// canonical addresses, explicit flags, VRF names through tableName (table 0 → unset). The VPP globals appear only
// where this agent can retrieve them (globals owner); pool names and descriptions are never invented.
func AssembleNat(kvs []scheduler.KV, tableName func(uint32) string) *vrxv1.NatConfig {
	out := &vrxv1.NatConfig{}
	assembleNat44ED(out, kvs, tableName)
	// Sibling assemblers add their leaves below their anchors.
	// wave-A: F-nat44-ei-64-66-nptv6
	// wave-BC: F-det44-map-dslite-cnat
	return out
}

// natVRFName is the document spelling of a table id: unset for the default table.
func natVRFName(id uint32, tableName func(uint32) string) *string {
	if id == 0 {
		return nil
	}
	return proto.String(tableName(id))
}

func assembleNat44ED(out *vrxv1.NatConfig, kvs []scheduler.KV, tableName func(uint32) string) {
	var (
		any44                  bool
		enable                 *nat44ed.EnableSpec
		timeouts               *nat44ed.TimeoutsSpec
		forwarding             bool
		rangePools             []nat44ed.AddressPoolSpec
		ifPools                []nat44ed.InterfaceAddressSpec
		statics                []nat44ed.StaticMappingSpec
		idents                 []nat44ed.IdentityMappingSpec
		lbs                    []nat44ed.LBStaticMappingSpec
		inside, outside, outFe []string
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case nat44ed.NameEnable:
			if v, err := natcommon.Decode[nat44ed.EnableSpec](kv.Value); err == nil {
				enable = &v
			}
		case nat44ed.NameTimeouts:
			if v, err := natcommon.Decode[nat44ed.TimeoutsSpec](kv.Value); err == nil {
				timeouts = &v
			}
		case nat44ed.NameForwarding:
			forwarding = true
		case nat44ed.NameInterfaceFeature:
			if v, err := natcommon.Decode[nat44ed.InterfaceFeatureSpec](kv.Value); err == nil {
				if v.Side == nat44ed.SideInside {
					inside = append(inside, v.Interface)
				} else {
					outside = append(outside, v.Interface)
				}
			}
		case nat44ed.NameOutputFeature:
			if v, err := natcommon.Decode[nat44ed.OutputFeatureSpec](kv.Value); err == nil {
				outFe = append(outFe, v.Interface)
			}
		case nat44ed.NameAddressPool:
			if v, err := natcommon.Decode[nat44ed.AddressPoolSpec](kv.Value); err == nil {
				rangePools = append(rangePools, v)
			}
		case nat44ed.NameInterfaceAddress:
			if v, err := natcommon.Decode[nat44ed.InterfaceAddressSpec](kv.Value); err == nil {
				ifPools = append(ifPools, v)
			}
		case nat44ed.NameStaticMapping:
			if v, err := natcommon.Decode[nat44ed.StaticMappingSpec](kv.Value); err == nil {
				statics = append(statics, v)
			}
		case nat44ed.NameIdentityMapping:
			if v, err := natcommon.Decode[nat44ed.IdentityMappingSpec](kv.Value); err == nil {
				idents = append(idents, v)
			}
		case nat44ed.NameLBStaticMapping:
			if v, err := natcommon.Decode[nat44ed.LBStaticMappingSpec](kv.Value); err == nil {
				lbs = append(lbs, v)
			}
		default:
			continue
		}
		any44 = true
	}
	if !any44 {
		return
	}
	out.Mode = proto.String(NatModeED)
	if enable != nil { // globals owner only (a slot cannot retrieve VPP globals, D-071)
		out.Enabled = proto.Bool(true)
		out.InsideVrf = natVRFName(enable.InsideVRF, tableName)
		out.OutsideVrf = natVRFName(enable.OutsideVRF, tableName)
		if enable.Sessions != nat44ed.DefaultSessions {
			out.SessionLimit = proto.Uint32(enable.Sessions)
		}
		out.Forwarding = proto.Bool(forwarding)
		t := nat44ed.DefaultTimeouts
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
		if x.TwiceNAT != y.TwiceNAT {
			return !x.TwiceNAT
		}
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
		out.Pools = append(out.Pools, &vrxv1.NatPool{Range: proto.String(r), Vrf: natVRFName(p.VRF, tableName), TwiceNat: proto.Bool(p.TwiceNAT)})
	}
	sort.Slice(ifPools, func(a, b int) bool { return ifPools[a].Interface < ifPools[b].Interface })
	for _, p := range ifPools {
		out.Pools = append(out.Pools, &vrxv1.NatPool{Interface: proto.String(p.Interface), TwiceNat: proto.Bool(p.TwiceNAT)})
	}

	sort.Slice(statics, func(a, b int) bool { return statics[a].Name < statics[b].Name })
	for _, m := range statics {
		sm := &vrxv1.NatStaticMapping{
			Name:     proto.String(m.Name),
			Local:    &vrxv1.NatStaticMapping_Local{Ip: proto.String(m.Local.IP)},
			External: &vrxv1.NatStaticMapping_External{},
			Vrf:      natVRFName(m.VRF, tableName),
			TwiceNat: proto.Bool(m.TwiceNAT), SelfTwiceNat: proto.Bool(m.SelfTwiceNAT), Out2InOnly: proto.Bool(m.Out2InOnly),
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

	sort.Slice(idents, func(a, b int) bool { return natIdentityKey(idents[a]) < natIdentityKey(idents[b]) })
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

	sort.Slice(lbs, func(a, b int) bool { return lbs[a].Name < lbs[b].Name })
	for _, m := range lbs {
		lm := &vrxv1.NatLoadBalancedMapping{
			Name: proto.String(m.Name), Protocol: proto.String(m.Protocol),
			External: &vrxv1.NatLoadBalancedMapping_External{Ip: proto.String(m.External.IP), Port: proto.Uint32(m.External.Port)},
			Affinity: proto.Uint32(m.Affinity),
			TwiceNat: proto.Bool(m.TwiceNAT), SelfTwiceNat: proto.Bool(m.SelfTwiceNAT), Out2InOnly: proto.Bool(m.Out2InOnly),
		}
		for _, l := range m.Locals { // DF-3 keeps them sorted (vrf, ip, port)
			lm.Locals = append(lm.Locals, &vrxv1.NatLoadBalancedMapping_Local{Ip: proto.String(l.IP), Port: proto.Uint32(l.Port), Probability: proto.Uint32(l.Probability), Vrf: natVRFName(l.VRF, tableName)})
		}
		out.LoadBalancedMappings = append(out.LoadBalancedMappings, lm)
	}
}

func natIdentityKey(m nat44ed.IdentityMappingSpec) string {
	who := m.IP
	if m.Interface != "" {
		who = "if:" + m.Interface
	}
	return fmt.Sprintf("%s|%010d|%s|%05d", who, m.VRF, m.Protocol, m.Port)
}

func natAddrLess(a, b string) bool {
	x, e1 := netip.ParseAddr(a)
	y, e2 := netip.ParseAddr(b)
	if e1 != nil || e2 != nil {
		return a < b
	}
	return x.Less(y)
}
