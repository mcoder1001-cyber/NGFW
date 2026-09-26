package coretest

// F-rpf-adl-pbr extension of the model (wave-A-hotspots A6): what urpf, adl, abf, the ACL dump and
// auto_sdl need so the agent's projection, Retrieve and restart paths run in unit tests with DF-2's real
// descriptors. Behaviour follows VPP 26.06: urpf_update_v2 replaces the check (mode OFF removes it);
// adl-input stacks on repeated enables; feature_is_enabled answers true for an unknown feature (V23 a);
// abf_policy_add_del is additive (is_add appends paths, !is_add removes them, the last path removed
// deletes the policy); attachments are keyed by (policy, sw_if_index, family).

import (
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	abfapi "ngfw/agent/binapi/abf"
	adlapi "ngfw/agent/binapi/adl"
	autosdlapi "ngfw/agent/binapi/auto_sdl"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	urpfapi "ngfw/agent/binapi/urpf"
)

func adlInputIsEnabled(v *VPP, req *featureapi.FeatureIsEnabled) *featureapi.FeatureIsEnabledReply {
	v.mu.Lock()
	defer v.mu.Unlock()
	return &featureapi.FeatureIsEnabledReply{IsEnabled: v.rpf().AdlInput[uint32(req.SwIfIndex)] > 0}
}

// ethernetInputDisabled models device-input's end node: never in a config's feature list, so
// feature_is_enabled(ethernet-input) reads false (adl.go's control query; V23 a). Any other unknown
// feature reads true (the dispatcher's default), VPP's own unknown-feature cast.
func ethernetInputDisabled(*VPP, *featureapi.FeatureIsEnabled) *featureapi.FeatureIsEnabledReply {
	return &featureapi.FeatureIsEnabledReply{}
}

func init() {
	RegisterExtension("rpf-adl-pbr", (*VPP).installRpfAdlPbr)
	// adl-input: this feature's read-back (the adl plugin has no dump). Registered through the TD-23 composition seam.
	RegisterFeatureIsEnabled("adl-input", adlInputIsEnabled)
	RegisterFeatureIsEnabled("ethernet-input", ethernetInputDisabled)
}

// UrpfKey identifies one uRPF check.
type UrpfKey struct {
	SwIfIndex uint32
	IPv6      bool
	Input     bool
}

// UrpfCheck is the configured mode and lookup table.
type UrpfCheck struct {
	Mode    urpfapi.UrpfMode
	TableID uint32
}

// AbfAttachKey identifies one ABF attachment.
type AbfAttachKey struct {
	PolicyID  uint32
	SwIfIndex uint32
	IPv6      bool
}

// RpfAdlPbrModel is the F-rpf-adl-pbr part of the model (guarded by VPP.mu).
type RpfAdlPbrModel struct {
	Urpf      map[UrpfKey]UrpfCheck
	AdlInput  map[uint32]int // adl-input instances per sw_if_index
	Allowlist int            // adl_allowlist_enable_disable calls (write-only: no dump)
	// ACLs are acl-plugin ACLs (index → tag); tests add them with AddACL (F-acl creates them in the product).
	Policies map[uint32]*abfapi.AbfPolicy
	Attach   map[AbfAttachKey]uint32 // → priority
	AutoSdl  []autosdlapi.AutoSdlConfig
}

// rpfModels holds each VPP model's F-rpf-adl-pbr part (the VPP struct itself is P05/P08's).
var rpfModels sync.Map // *VPP → *RpfAdlPbrModel

// rpf returns v's F-rpf-adl-pbr model.
func (v *VPP) rpf() *RpfAdlPbrModel {
	m, _ := rpfModels.Load(v)
	return m.(*RpfAdlPbrModel)
}

func (v *VPP) installRpfAdlPbr() {
	m := &RpfAdlPbrModel{
		Urpf: map[UrpfKey]UrpfCheck{}, AdlInput: map[uint32]int{},
		Policies: map[uint32]*abfapi.AbfPolicy{}, Attach: map[AbfAttachKey]uint32{},
	}
	rpfModels.Store(v, m)
	v.On("urpf_update_v2", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*urpfapi.UrpfUpdateV2)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.Ifaces[uint32(req.SwIfIndex)]; !ok {
			return reply(&urpfapi.UrpfUpdateV2Reply{Retval: RetvalInvalidSwIfIndex})
		}
		k := UrpfKey{SwIfIndex: uint32(req.SwIfIndex), IPv6: req.Af == ip_types.ADDRESS_IP6, Input: req.IsInput}
		if req.Mode == urpfapi.URPF_API_MODE_OFF {
			delete(m.Urpf, k)
		} else {
			m.Urpf[k] = UrpfCheck{Mode: req.Mode, TableID: req.TableID}
		}
		return reply(&urpfapi.UrpfUpdateV2Reply{})
	})
	v.On("urpf_interface_dump", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*urpfapi.UrpfInterfaceDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]UrpfKey, 0, len(m.Urpf))
		for k := range m.Urpf {
			if uint32(req.SwIfIndex) == ^uint32(0) || uint32(req.SwIfIndex) == k.SwIfIndex {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].SwIfIndex < keys[j].SwIfIndex })
		out := make([]api.Message, 0, len(keys))
		for _, k := range keys {
			af := ip_types.ADDRESS_IP4
			if k.IPv6 {
				af = ip_types.ADDRESS_IP6
			}
			c := m.Urpf[k]
			out = append(out, &urpfapi.UrpfInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(k.SwIfIndex), IsInput: k.Input, Mode: c.Mode, Af: af, TableID: c.TableID})
		}
		return out, nil
	})
	v.On("adl_interface_enable_disable", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*adlapi.AdlInterfaceEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		idx := uint32(req.SwIfIndex)
		if req.EnableDisable {
			m.AdlInput[idx]++
		} else if m.AdlInput[idx] > 0 {
			m.AdlInput[idx]--
		}
		return reply(&adlapi.AdlInterfaceEnableDisableReply{})
	})
	v.On("adl_allowlist_enable_disable", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m.Allowlist++
		return reply(&adlapi.AdlAllowlistEnableDisableReply{})
	})
	v.On("abf_policy_add_del", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*abfapi.AbfPolicyAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		p := req.Policy
		cur, exists := m.Policies[p.PolicyID]
		if req.IsAdd {
			if !v.ACL().Has(p.ACLIndex) {
				return reply(&abfapi.AbfPolicyAddDelReply{Retval: RetvalNoSuchEntry})
			}
			if !exists {
				cur = &abfapi.AbfPolicy{PolicyID: p.PolicyID, ACLIndex: p.ACLIndex}
				m.Policies[p.PolicyID] = cur
			}
			cur.Paths = append(cur.Paths, p.Paths...)
			cur.NPaths = uint8(len(cur.Paths)) //nolint:gosec // ≤ 255 in tests
			return reply(&abfapi.AbfPolicyAddDelReply{})
		}
		if !exists {
			return reply(&abfapi.AbfPolicyAddDelReply{Retval: RetvalNoSuchEntry})
		}
		var keep []fib_types.FibPath
		for _, have := range cur.Paths {
			removed := false
			for _, del := range p.Paths {
				if samePath(have, del) {
					removed = true
					break
				}
			}
			if !removed {
				keep = append(keep, have)
			}
		}
		if len(keep) == 0 {
			delete(m.Policies, p.PolicyID)
		} else {
			cur.Paths, cur.NPaths = keep, uint8(len(keep)) //nolint:gosec // ≤ 255 in tests
		}
		return reply(&abfapi.AbfPolicyAddDelReply{})
	})
	v.On("abf_policy_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		ids := make([]uint32, 0, len(m.Policies))
		for id := range m.Policies {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		out := make([]api.Message, 0, len(ids))
		for _, id := range ids {
			p := *m.Policies[id]
			p.Paths = append([]fib_types.FibPath(nil), p.Paths...)
			out = append(out, &abfapi.AbfPolicyDetails{Policy: p})
		}
		return out, nil
	})
	v.On("abf_itf_attach_add_del", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*abfapi.AbfItfAttachAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		a := req.Attach
		k := AbfAttachKey{PolicyID: a.PolicyID, SwIfIndex: uint32(a.SwIfIndex), IPv6: a.IsIPv6}
		if req.IsAdd {
			if _, ok := m.Policies[a.PolicyID]; !ok {
				return reply(&abfapi.AbfItfAttachAddDelReply{Retval: RetvalNoSuchEntry})
			}
			if _, ok := v.Ifaces[k.SwIfIndex]; !ok {
				return reply(&abfapi.AbfItfAttachAddDelReply{Retval: RetvalInvalidSwIfIndex})
			}
			m.Attach[k] = a.Priority
		} else {
			delete(m.Attach, k)
		}
		return reply(&abfapi.AbfItfAttachAddDelReply{})
	})
	v.On("abf_itf_attach_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]AbfAttachKey, 0, len(m.Attach))
		for k := range m.Attach {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].PolicyID != keys[j].PolicyID {
				return keys[i].PolicyID < keys[j].PolicyID
			}
			return keys[i].SwIfIndex < keys[j].SwIfIndex
		})
		out := make([]api.Message, 0, len(keys))
		for _, k := range keys {
			out = append(out, &abfapi.AbfItfAttachDetails{Attach: abfapi.AbfItfAttach{PolicyID: k.PolicyID, SwIfIndex: interface_types.InterfaceIndex(k.SwIfIndex), Priority: m.Attach[k], IsIPv6: k.IPv6}})
		}
		return out, nil
	})
	v.On("auto_sdl_config", func(msg api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m.AutoSdl = append(m.AutoSdl, *msg.(*autosdlapi.AutoSdlConfig))
		return reply(&autosdlapi.AutoSdlConfigReply{})
	})
}

func samePath(a, b fib_types.FibPath) bool {
	return a.SwIfIndex == b.SwIfIndex && a.TableID == b.TableID && a.Weight == b.Weight && a.Preference == b.Preference &&
		a.Type == b.Type && a.Proto == b.Proto && a.Nh.Address == b.Nh.Address
}

// AddACL adds an acl-plugin ACL with tag (e.g. "w3:lan-b") and returns its index.
// AddACL adds a tag-only ACL to the shared acl.go model (F-acl owns acl_dump since both were ported onto main).
func (v *VPP) AddACL(tag string) uint32 { return v.ACL().AddTagged(tag) }

// RpfAdlPbrState returns a copy of the uRPF checks, adl-input counts, ABF policies (id → path count)
// and attachments.
func (v *VPP) RpfAdlPbrState() (urpf map[UrpfKey]UrpfCheck, adl map[uint32]int, policies map[uint32]int, attach map[AbfAttachKey]uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.rpf()
	urpf, adl, policies, attach = map[UrpfKey]UrpfCheck{}, map[uint32]int{}, map[uint32]int{}, map[AbfAttachKey]uint32{}
	for k, c := range m.Urpf {
		urpf[k] = c
	}
	for k, n := range m.AdlInput {
		if n > 0 {
			adl[k] = n
		}
	}
	for id, p := range m.Policies {
		policies[id] = len(p.Paths)
	}
	for k, p := range m.Attach {
		attach[k] = p
	}
	return urpf, adl, policies, attach
}

// DeleteRpfAdlPbr removes every uRPF check, adl-input feature and ABF object behind the agent's back
// (the restart simulation's "loss"), attachments before policies (D-095 c).
func (v *VPP) DeleteRpfAdlPbr() {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.rpf()
	m.Attach = map[AbfAttachKey]uint32{}
	m.Policies = map[uint32]*abfapi.AbfPolicy{}
	m.AdlInput = map[uint32]int{}
	m.Urpf = map[UrpfKey]UrpfCheck{}
}
