package coretest

// F-det44-map-dslite-cnat: stateful models of the map and cnat plugins (DF-3's descriptors/mapnat and cnat), after
// DF-3's per-package fakes and VPP 26.06's map_api.c / cnat_api.c: MAP domains get increasing indexes and keep their
// tag, rules only exist on a domain without EA bits, the parameters start at VPP's defaults (security check and TC
// copy on), MAP-E and MAP-T are two independent per-interface bitmaps answered through feature_is_enabled; cnat
// translations update in place on the same (vip, port, proto), n_paths = 0 and every default-SNAT-entry dereference
// without an entry are recorded as crashes (V10), excluded prefixes are refcounted per add (D-076).

import (
	"net/netip"
	"sync"

	"go.fd.io/govpp/api"

	cnatapi "ngfw/agent/binapi/cnat"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	maps "ngfw/agent/binapi/map"
)

// Map is the map plugin model.
type Map struct {
	v  *VPP
	mu sync.Mutex

	next    uint32
	Domains map[uint32]*maps.MapDomainDetails
	Rules   map[uint32]map[uint16]ip_types.IP6Address
	Params  maps.MapParamGetReply
	Encap   map[uint32]bool
	Trans   map[uint32]bool
}

// Lock guards the exported fields.
func (m *Map) Lock() { m.mu.Lock() }

// Unlock releases Lock.
func (m *Map) Unlock() { m.mu.Unlock() }

// Cnat is the cnat plugin model.
type Cnat struct {
	v  *VPP
	mu sync.Mutex

	nextID   uint32
	Trs      map[uint32]cnatapi.CnatTranslation
	Snat     *cnatapi.CnatGetSnatAddressesReply
	Policy   cnatapi.CnatSnatPolicies
	SnatIfs  map[[2]uint32]bool
	PfxRefs  map[string]int
	Feat     map[uint32]bool
	Crashes  []string // calls that would crash VPP 26.06 (V10); must stay empty
	Sessions []cnatapi.CnatSession
	Purges   int
}

// Lock guards the exported fields.
func (c *Cnat) Lock() { c.mu.Lock() }

// Unlock releases Lock.
func (c *Cnat) Unlock() { c.mu.Unlock() }

var mapModels, cnatModels sync.Map

func init() {
	RegisterExtension("map-cnat", func(v *VPP) {
		mapModels.Store(v, v.installMap())
		cnatModels.Store(v, v.installCnat())
	})
	RegisterFeatureIsEnabled("ip4-map", func(v *VPP, r *featureapi.FeatureIsEnabled) *featureapi.FeatureIsEnabledReply {
		m := v.Map()
		m.mu.Lock()
		defer m.mu.Unlock()
		return &featureapi.FeatureIsEnabledReply{IsEnabled: r.ArcName == "ip4-unicast" && m.Encap[uint32(r.SwIfIndex)]}
	})
	RegisterFeatureIsEnabled("ip4-map-t", func(v *VPP, r *featureapi.FeatureIsEnabled) *featureapi.FeatureIsEnabledReply {
		m := v.Map()
		m.mu.Lock()
		defer m.mu.Unlock()
		return &featureapi.FeatureIsEnabledReply{IsEnabled: r.ArcName == "ip4-unicast" && m.Trans[uint32(r.SwIfIndex)]}
	})
	RegisterFeatureIsEnabled("cnat-input-ip4", func(v *VPP, r *featureapi.FeatureIsEnabled) *featureapi.FeatureIsEnabledReply {
		c := v.Cnat()
		c.mu.Lock()
		defer c.mu.Unlock()
		return &featureapi.FeatureIsEnabledReply{IsEnabled: r.ArcName == "ip4-unicast" && c.Feat[uint32(r.SwIfIndex)]}
	})
}

// Map returns the map plugin model of v.
func (v *VPP) Map() *Map {
	m, ok := mapModels.Load(v)
	if !ok {
		m, _ = mapModels.LoadOrStore(v, v.installMap())
	}
	return m.(*Map)
}

// Cnat returns the cnat plugin model of v.
func (v *VPP) Cnat() *Cnat {
	m, ok := cnatModels.Load(v)
	if !ok {
		m, _ = cnatModels.LoadOrStore(v, v.installCnat())
	}
	return m.(*Cnat)
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func (v *VPP) installMap() *Map {
	m := &Map{v: v, Domains: map[uint32]*maps.MapDomainDetails{}, Rules: map[uint32]map[uint16]ip_types.IP6Address{},
		Params: maps.MapParamGetReply{SecCheckEnable: true, TcCopy: true}, Encap: map[uint32]bool{}, Trans: map[uint32]bool{}}
	one := func(x api.Message) ([]api.Message, error) { return []api.Message{x}, nil }
	v.On("map_add_domain", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapAddDomain)
		m.mu.Lock()
		defer m.mu.Unlock()
		idx := m.next
		m.next++
		m.Domains[idx] = &maps.MapDomainDetails{DomainIndex: idx, IP4Prefix: r.IP4Prefix, IP6Prefix: r.IP6Prefix, IP6Src: r.IP6Src,
			EaBitsLen: r.EaBitsLen, PsidOffset: r.PsidOffset, PsidLength: r.PsidLength, Mtu: r.Mtu, Tag: r.Tag}
		return one(&maps.MapAddDomainReply{Index: idx})
	})
	v.On("map_del_domain", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapDelDomain)
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.Domains[r.Index]; !ok {
			return one(&maps.MapDelDomainReply{Retval: int32(api.NO_SUCH_ENTRY)})
		}
		delete(m.Domains, r.Index)
		delete(m.Rules, r.Index)
		return one(&maps.MapDelDomainReply{})
	})
	v.On("map_domain_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for i := uint32(0); i < m.next; i++ {
			if d, ok := m.Domains[i]; ok {
				c := *d
				out = append(out, &c)
			}
		}
		return out, nil
	})
	v.On("map_add_del_rule", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapAddDelRule)
		m.mu.Lock()
		defer m.mu.Unlock()
		d, ok := m.Domains[r.Index]
		if !ok || d.EaBitsLen > 0 || int(r.Psid) >= 1<<d.PsidLength {
			return one(&maps.MapAddDelRuleReply{Retval: -1})
		}
		if m.Rules[r.Index] == nil {
			m.Rules[r.Index] = map[uint16]ip_types.IP6Address{}
		}
		if r.IsAdd {
			m.Rules[r.Index][r.Psid] = r.IP6Dst
		} else {
			delete(m.Rules[r.Index], r.Psid)
		}
		return one(&maps.MapAddDelRuleReply{})
	})
	v.On("map_rule_dump", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapRuleDump)
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for psid := 0; psid < 1<<16; psid++ {
			if dst, ok := m.Rules[r.DomainIndex][uint16(psid)]; ok {
				out = append(out, &maps.MapRuleDetails{Psid: uint16(psid), IP6Dst: dst})
			}
		}
		return out, nil
	})
	v.On("map_param_get", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		p := m.Params
		return one(&p)
	})
	v.On("map_param_set_fragmentation", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapParamSetFragmentation)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Params.FragInner, m.Params.FragIgnoreDf = b2u8(r.Inner), b2u8(r.IgnoreDf)
		return one(&maps.MapParamSetFragmentationReply{})
	})
	v.On("map_param_set_icmp", func(x api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Params.ICMPIP4ErrRelaySrc = x.(*maps.MapParamSetICMP).IP4ErrRelaySrc
		return one(&maps.MapParamSetICMPReply{})
	})
	v.On("map_param_set_icmp6", func(x api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Params.ICMP6EnableUnreachable = x.(*maps.MapParamSetICMP6).EnableUnreachable
		return one(&maps.MapParamSetICMP6Reply{})
	})
	v.On("map_param_set_security_check", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapParamSetSecurityCheck)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Params.SecCheckEnable, m.Params.SecCheckFragments = r.Enable, r.Fragments
		return one(&maps.MapParamSetSecurityCheckReply{})
	})
	v.On("map_param_set_traffic_class", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapParamSetTrafficClass)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Params.TcCopy, m.Params.TcClass = r.Copy, r.TcClass
		return one(&maps.MapParamSetTrafficClassReply{})
	})
	v.On("map_if_enable_disable", func(x api.Message) ([]api.Message, error) {
		r := x.(*maps.MapIfEnableDisable)
		if !v.ifExists(uint32(r.SwIfIndex)) {
			return one(&maps.MapIfEnableDisableReply{Retval: int32(api.INVALID_SW_IF_INDEX)})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		t := m.Encap
		if r.IsTranslation {
			t = m.Trans
		}
		if r.IsEnable {
			t[uint32(r.SwIfIndex)] = true
		} else {
			delete(t, uint32(r.SwIfIndex))
		}
		return one(&maps.MapIfEnableDisableReply{})
	})
	return m
}

func (v *VPP) installCnat() *Cnat {
	c := &Cnat{v: v, Trs: map[uint32]cnatapi.CnatTranslation{}, SnatIfs: map[[2]uint32]bool{}, PfxRefs: map[string]int{}, Feat: map[uint32]bool{}}
	one := func(x api.Message) ([]api.Message, error) { return []api.Message{x}, nil }
	noIf := ^interface_types.InterfaceIndex(0)
	v.On("cnat_translation_update", func(x api.Message) ([]api.Message, error) {
		tr := x.(*cnatapi.CnatTranslationUpdate).Translation
		c.mu.Lock()
		defer c.mu.Unlock()
		if tr.NPaths == 0 {
			c.Crashes = append(c.Crashes, "cnat_translation_update n_paths=0")
			return one(&cnatapi.CnatTranslationUpdateReply{Retval: int32(api.UNSUPPORTED)})
		}
		tr.Flags, tr.IsRealIP, tr.FlowHashConfig = 0, 0, 0
		tr.Paths = append([]cnatapi.CnatEndpointTuple(nil), tr.Paths...)
		for id, cur := range c.Trs {
			if cur.Vip.Addr == tr.Vip.Addr && cur.Vip.Port == tr.Vip.Port && cur.IPProto == tr.IPProto {
				tr.ID = id
				c.Trs[id] = tr
				return one(&cnatapi.CnatTranslationUpdateReply{ID: id})
			}
		}
		tr.ID = c.nextID
		c.nextID++
		c.Trs[tr.ID] = tr
		return one(&cnatapi.CnatTranslationUpdateReply{ID: tr.ID})
	})
	v.On("cnat_translation_del", func(x api.Message) ([]api.Message, error) {
		id := x.(*cnatapi.CnatTranslationDel).ID
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.Trs[id]; !ok {
			return one(&cnatapi.CnatTranslationDelReply{Retval: int32(api.NO_SUCH_ENTRY)})
		}
		delete(c.Trs, id)
		return one(&cnatapi.CnatTranslationDelReply{})
	})
	v.On("cnat_translation_dump", func(api.Message) ([]api.Message, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		var out []api.Message
		for id := uint32(0); id < c.nextID; id++ {
			if tr, ok := c.Trs[id]; ok {
				out = append(out, &cnatapi.CnatTranslationDetails{Translation: tr})
			}
		}
		return out, nil
	})
	v.On("cnat_get_snat_addresses", func(api.Message) ([]api.Message, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.Snat == nil {
			return one(&cnatapi.CnatGetSnatAddressesReply{Retval: int32(api.FEATURE_DISABLED)})
		}
		r := *c.Snat
		return one(&r)
	})
	v.On("cnat_set_snat_addresses", func(x api.Message) ([]api.Message, error) {
		r := x.(*cnatapi.CnatSetSnatAddresses)
		c.mu.Lock()
		defer c.mu.Unlock()
		if r.SnatIP4 == (ip_types.IP4Address{}) && r.SnatIP6 == (ip_types.IP6Address{}) && r.SwIfIndex == noIf {
			if c.Snat == nil {
				return one(&cnatapi.CnatSetSnatAddressesReply{Retval: int32(api.FEATURE_DISABLED)})
			}
			c.Snat, c.Policy, c.SnatIfs, c.PfxRefs = nil, 0, map[[2]uint32]bool{}, map[string]int{}
			return one(&cnatapi.CnatSetSnatAddressesReply{})
		}
		if c.Snat == nil {
			c.Snat = &cnatapi.CnatGetSnatAddressesReply{}
		}
		c.Snat.SnatIP4, c.Snat.SnatIP6 = r.SnatIP4, r.SnatIP6
		if r.SnatIP6 != (ip_types.IP6Address{}) || r.SwIfIndex != noIf {
			c.Snat.SwIfIndex = r.SwIfIndex
		}
		return one(&cnatapi.CnatSetSnatAddressesReply{})
	})
	v.On("cnat_set_snat_policy", func(x api.Message) ([]api.Message, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.Snat == nil {
			c.Crashes = append(c.Crashes, "cnat_set_snat_policy without a default SNAT entry")
			return one(&cnatapi.CnatSetSnatPolicyReply{Retval: int32(api.UNSUPPORTED)})
		}
		c.Policy = x.(*cnatapi.CnatSetSnatPolicy).Policy
		return one(&cnatapi.CnatSetSnatPolicyReply{})
	})
	v.On("cnat_snat_policy_add_del_exclude_pfx", func(x api.Message) ([]api.Message, error) {
		r := x.(*cnatapi.CnatSnatPolicyAddDelExcludePfx)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.Snat == nil {
			c.Crashes = append(c.Crashes, "cnat_snat_policy_add_del_exclude_pfx without a default SNAT entry")
			return one(&cnatapi.CnatSnatPolicyAddDelExcludePfxReply{Retval: int32(api.UNSUPPORTED)})
		}
		k := netip.PrefixFrom(netip.AddrFrom16(r.Prefix.Address.Un.GetIP6()), int(r.Prefix.Len)).String()
		if r.Prefix.Address.Af == ip_types.ADDRESS_IP4 {
			k = netip.PrefixFrom(netip.AddrFrom4(r.Prefix.Address.Un.GetIP4()), int(r.Prefix.Len)).String()
		}
		if r.IsAdd == 1 {
			c.PfxRefs[k]++
		} else if c.PfxRefs[k] > 0 {
			c.PfxRefs[k]--
			if c.PfxRefs[k] == 0 {
				delete(c.PfxRefs, k)
			}
		}
		return one(&cnatapi.CnatSnatPolicyAddDelExcludePfxReply{})
	})
	v.On("cnat_snat_policy_add_del_if", func(x api.Message) ([]api.Message, error) {
		r := x.(*cnatapi.CnatSnatPolicyAddDelIf)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.Snat == nil {
			return one(&cnatapi.CnatSnatPolicyAddDelIfReply{Retval: int32(api.FEATURE_DISABLED)})
		}
		k := [2]uint32{uint32(r.SwIfIndex), uint32(r.Table)}
		if r.IsAdd == 1 {
			c.SnatIfs[k] = true
		} else {
			delete(c.SnatIfs, k)
		}
		return one(&cnatapi.CnatSnatPolicyAddDelIfReply{})
	})
	v.On("feature_cnat_enable_disable", func(x api.Message) ([]api.Message, error) {
		r := x.(*cnatapi.FeatureCnatEnableDisable)
		if !v.ifExists(uint32(r.SwIfIndex)) {
			return one(&cnatapi.FeatureCnatEnableDisableReply{Retval: int32(api.INVALID_SW_IF_INDEX)})
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if r.EnableDisable {
			c.Feat[uint32(r.SwIfIndex)] = true
		} else {
			delete(c.Feat, uint32(r.SwIfIndex))
		}
		return one(&cnatapi.FeatureCnatEnableDisableReply{})
	})
	v.On("cnat_session_dump", func(api.Message) ([]api.Message, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		out := make([]api.Message, 0, len(c.Sessions))
		for _, s := range c.Sessions {
			out = append(out, &cnatapi.CnatSessionDetails{Session: s})
		}
		return out, nil
	})
	v.On("cnat_session_purge", func(api.Message) ([]api.Message, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.Purges++
		c.Sessions = nil
		return one(&cnatapi.CnatSessionPurgeReply{})
	})
	return c
}
