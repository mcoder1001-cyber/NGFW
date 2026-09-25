package coretest

// F-acl: the acl plugin in the agent-level fake VPP (acl.c semantics, as DF-4's descriptor-level
// model: index allocation, replace in place, in-use checks, one MACIP ACL per interface, count-0
// details for interfaces without lists) plus the read-only CLI that prints the counters flag and a
// fake stats segment with the per-ACL counter vectors. Interfaces are the model's own (v.Ifaces).

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vlib"
)

// VPP retvals of the acl plugin model.
const (
	RetvalInvalidValue       int32 = -7
	RetvalInvalidValue2      int32 = -8
	RetvalEntryAlreadyExists int32 = -12
	RetvalACLInUseInbound    int32 = -142
)

const aclWildcard = ^uint32(0)

// ACLModel is the acl plugin state of one fake VPP.
type ACLModel struct {
	mu        sync.Mutex
	acls      map[uint32]*vppacl.ACLDetails
	nextACL   uint32
	bindings  map[uint32]*vppacl.ACLInterfaceListDetails
	etypes    map[uint32]*vppacl.ACLInterfaceEtypeWhitelistDetails
	macips    map[uint32]*vppacl.MacipACLDetails
	nextMacip uint32
	macipBind map[uint32]uint32
	// CountersEnabled is the plugin's counters flag (acl_stats_intf_counters_enable).
	countersEnabled bool
	// Stats is the stats segment: /acl/<index>/matches vectors (tests set hits with SetHits).
	Stats *FakeStats
	// failNext: message name → retval returned once (fault injection, FailNext).
	failNext map[string]int32
}

// FailNext makes the next request named message (acl_add_replace, acl_interface_set_acl_list)
// answer retval instead of being applied (fault injection for rollback tests).
func (m *ACLModel) FailNext(message string, retval int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext == nil {
		m.failNext = map[string]int32{}
	}
	m.failNext[message] = retval
}

// takeFailLocked returns and clears the injected retval of message (0 = none).
func (m *ACLModel) takeFailLocked(message string) int32 {
	rv := m.failNext[message]
	delete(m.failNext, message)
	return rv
}

// SetRules replaces the rules of ACL idx behind the agent's back (a hand edit in VPP).
func (m *ACLModel) SetRules(idx uint32, rules ...acl_types.ACLRule) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.acls[idx]
	if ok {
		a.R = append([]acl_types.ACLRule(nil), rules...)
	}
	return ok
}

var (
	aclModelsMu sync.Mutex
	aclModels   = map[*VPP]*ACLModel{}
)

// ACL returns the acl plugin model of v.
func (v *VPP) ACL() *ACLModel {
	aclModelsMu.Lock()
	defer aclModelsMu.Unlock()
	return aclModels[v]
}

func (v *VPP) ifaceExists(idx uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.Ifaces[idx]
	return ok
}

func sortedUint32Keys[V any](m map[uint32]V) []uint32 {
	keys := make([]uint32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// installACL registers the acl plugin handlers on v (called by New).
func (v *VPP) installACL() {
	m := &ACLModel{
		acls: map[uint32]*vppacl.ACLDetails{}, bindings: map[uint32]*vppacl.ACLInterfaceListDetails{},
		etypes: map[uint32]*vppacl.ACLInterfaceEtypeWhitelistDetails{}, macips: map[uint32]*vppacl.MacipACLDetails{},
		macipBind: map[uint32]uint32{}, Stats: NewFakeStats(),
	}
	aclModelsMu.Lock()
	aclModels[v] = m
	aclModelsMu.Unlock()

	v.On("acl_add_replace", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLAddReplace)
		m.mu.Lock()
		defer m.mu.Unlock()
		if rv := m.takeFailLocked("acl_add_replace"); rv != 0 {
			return reply(&vppacl.ACLAddReplaceReply{Retval: rv})
		}
		if len(r.Tag) >= 64 {
			return reply(&vppacl.ACLAddReplaceReply{Retval: RetvalInvalidValue})
		}
		for _, rule := range r.R {
			if rule.SrcPrefix.Address.Af != rule.DstPrefix.Address.Af {
				return reply(&vppacl.ACLAddReplaceReply{Retval: RetvalInvalidValue})
			}
			if rule.SrcportOrIcmptypeFirst > rule.SrcportOrIcmptypeLast || rule.DstportOrIcmpcodeFirst > rule.DstportOrIcmpcodeLast {
				return reply(&vppacl.ACLAddReplaceReply{Retval: RetvalInvalidValue2})
			}
		}
		idx := r.ACLIndex
		if idx == aclWildcard {
			idx = m.nextACL
			m.nextACL++
		} else if _, ok := m.acls[idx]; !ok {
			return reply(&vppacl.ACLAddReplaceReply{Retval: RetvalNoSuchEntry})
		}
		m.acls[idx] = &vppacl.ACLDetails{ACLIndex: idx, Tag: r.Tag, R: append([]acl_types.ACLRule{}, r.R...)}
		m.Stats.ensure(fmt.Sprintf("/acl/%d/matches", idx), len(r.R)+1) // validate_and_reset_acl_counters
		return reply(&vppacl.ACLAddReplaceReply{ACLIndex: idx})
	})
	v.On("acl_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.acls[r.ACLIndex]; !ok {
			return reply(&vppacl.ACLDelReply{Retval: RetvalNoSuchEntry})
		}
		for _, b := range m.bindings {
			for _, idx := range b.Acls {
				if idx == r.ACLIndex {
					return reply(&vppacl.ACLDelReply{Retval: RetvalACLInUseInbound})
				}
			}
		}
		delete(m.acls, r.ACLIndex)
		return reply(&vppacl.ACLDelReply{})
	})
	v.On("acl_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLDump)
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedUint32Keys(m.acls) {
			if r.ACLIndex == aclWildcard || r.ACLIndex == idx {
				d := *m.acls[idx]
				d.R = append([]acl_types.ACLRule{}, d.R...)
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("acl_interface_set_acl_list", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLInterfaceSetACLList)
		swif := uint32(r.SwIfIndex)
		if !v.ifaceExists(swif) {
			return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if rv := m.takeFailLocked("acl_interface_set_acl_list"); rv != 0 {
			return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: rv})
		}
		if int(r.NInput) > len(r.Acls) {
			return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: RetvalInvalidValue})
		}
		seen := map[bool]map[uint32]bool{true: {}, false: {}}
		for i, idx := range r.Acls {
			if _, ok := m.acls[idx]; !ok {
				return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: RetvalNoSuchEntry})
			}
			in := i < int(r.NInput)
			if seen[in][idx] {
				return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: RetvalEntryAlreadyExists})
			}
			seen[in][idx] = true
		}
		if len(r.Acls) == 0 {
			delete(m.bindings, swif)
		} else {
			m.bindings[swif] = &vppacl.ACLInterfaceListDetails{SwIfIndex: r.SwIfIndex, NInput: r.NInput, Acls: append([]uint32{}, r.Acls...)}
		}
		return reply(&vppacl.ACLInterfaceSetACLListReply{})
	})
	v.On("acl_interface_list_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLInterfaceListDump)
		v.mu.Lock()
		idxs := v.indexesLocked()
		v.mu.Unlock()
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, idx := range idxs {
			if uint32(r.SwIfIndex) != aclWildcard && uint32(r.SwIfIndex) != idx {
				continue
			}
			if b, ok := m.bindings[idx]; ok {
				d := *b
				d.Acls = append([]uint32{}, b.Acls...)
				out = append(out, &d)
			} else { // VPP sends a count-0 details for every interface
				out = append(out, &vppacl.ACLInterfaceListDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return out, nil
	})
	v.On("acl_interface_set_etype_whitelist", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLInterfaceSetEtypeWhitelist)
		swif := uint32(r.SwIfIndex)
		if !v.ifaceExists(swif) {
			return reply(&vppacl.ACLInterfaceSetEtypeWhitelistReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(r.Whitelist) == 0 {
			delete(m.etypes, swif)
		} else {
			m.etypes[swif] = &vppacl.ACLInterfaceEtypeWhitelistDetails{SwIfIndex: r.SwIfIndex, NInput: r.NInput, Whitelist: append([]uint16{}, r.Whitelist...)}
		}
		return reply(&vppacl.ACLInterfaceSetEtypeWhitelistReply{})
	})
	v.On("acl_interface_etype_whitelist_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		idxs := v.indexesLocked()
		v.mu.Unlock()
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, idx := range idxs {
			if w, ok := m.etypes[idx]; ok {
				d := *w
				d.Whitelist = append([]uint16{}, w.Whitelist...)
				out = append(out, &d)
			} else {
				out = append(out, &vppacl.ACLInterfaceEtypeWhitelistDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return out, nil
	})
	v.On("macip_acl_add_replace", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLAddReplace)
		m.mu.Lock()
		defer m.mu.Unlock()
		idx := r.ACLIndex
		if idx == aclWildcard {
			idx = m.nextMacip
			m.nextMacip++
		} else if _, ok := m.macips[idx]; !ok {
			return reply(&vppacl.MacipACLAddReplaceReply{Retval: RetvalNoSuchEntry})
		}
		m.macips[idx] = &vppacl.MacipACLDetails{ACLIndex: idx, Tag: r.Tag, R: append([]acl_types.MacipACLRule{}, r.R...)}
		return reply(&vppacl.MacipACLAddReplaceReply{ACLIndex: idx})
	})
	v.On("macip_acl_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.macips[r.ACLIndex]; !ok {
			return reply(&vppacl.MacipACLDelReply{Retval: RetvalNoSuchEntry})
		}
		for swif, bound := range m.macipBind { // macip_acl_del unapplies first (acl.c)
			if bound == r.ACLIndex {
				delete(m.macipBind, swif)
			}
		}
		delete(m.macips, r.ACLIndex)
		return reply(&vppacl.MacipACLDelReply{})
	})
	v.On("macip_acl_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLDump)
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedUint32Keys(m.macips) {
			if r.ACLIndex == aclWildcard || r.ACLIndex == idx {
				d := *m.macips[idx]
				d.R = append([]acl_types.MacipACLRule{}, d.R...)
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("macip_acl_interface_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLInterfaceAddDel)
		swif := uint32(r.SwIfIndex)
		if !v.ifaceExists(swif) {
			return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.IsAdd {
			if _, ok := m.macips[r.ACLIndex]; !ok {
				return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: RetvalNoSuchEntry})
			}
			m.macipBind[swif] = r.ACLIndex
			return reply(&vppacl.MacipACLInterfaceAddDelReply{})
		}
		if _, ok := m.macipBind[swif]; !ok {
			return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: RetvalNoSuchEntry})
		}
		delete(m.macipBind, swif)
		return reply(&vppacl.MacipACLInterfaceAddDelReply{})
	})
	v.On("macip_acl_interface_list_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLInterfaceListDump)
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, swif := range sortedUint32Keys(m.macipBind) {
			if uint32(r.SwIfIndex) != aclWildcard && uint32(r.SwIfIndex) != swif {
				continue
			}
			out = append(out, &vppacl.MacipACLInterfaceListDetails{SwIfIndex: interface_types.InterfaceIndex(swif), Acls: []uint32{m.macipBind[swif]}})
		}
		return out, nil
	})
	v.On("acl_stats_intf_counters_enable", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLStatsIntfCountersEnable)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.countersEnabled = r.Enable
		return reply(&vppacl.ACLDelReply{}) // what VPP 26.06 really sends (V7)
	})
	v.On("cli_inband", func(req api.Message) ([]api.Message, error) {
		r := req.(*vlib.CliInband)
		if r.Cmd != "show acl-plugin tables mask" {
			return reply(&vlib.CliInbandReply{Retval: RetvalInvalidValue, Reply: "fake vpp: only the acl counters flag is modelled"})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		on := 0
		if m.countersEnabled {
			on = 1
		}
		return reply(&vlib.CliInbandReply{Reply: fmt.Sprintf("Stats counters enabled for interface ACLs: %d\nUse hash-based lookup for ACLs: 1\nMask-type entries:\n", on)})
	})
	v.Reply("acl_plugin_get_version", &vppacl.ACLPluginGetVersionReply{Major: 1, Minor: 0})
}

// SetCountersEnabled sets the plugin's counters flag directly (as if another owner switched it).
func (m *ACLModel) SetCountersEnabled(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countersEnabled = on
}

// CountersEnabled reports the counters flag.
func (m *ACLModel) CountersEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.countersEnabled
}

// AddACL plants an ACL directly (any owner's, e.g. a foreign one), returning its index.
func (m *ACLModel) AddACL(tag string, rules ...acl_types.ACLRule) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.nextACL
	m.nextACL++
	m.acls[idx] = &vppacl.ACLDetails{ACLIndex: idx, Tag: tag, R: rules}
	m.Stats.ensure(fmt.Sprintf("/acl/%d/matches", idx), len(rules)+1)
	return idx
}

// DeleteACL removes an ACL behind the agent's back (restart-safety simulations); false if absent.
func (m *ACLModel) DeleteACL(idx uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.acls[idx]
	delete(m.acls, idx)
	return ok
}

// Bind plants an interface ACL list directly (input first, nInput of them).
func (m *ACLModel) Bind(swif uint32, nInput uint8, acls ...uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(acls) == 0 {
		delete(m.bindings, swif)
		return
	}
	m.bindings[swif] = &vppacl.ACLInterfaceListDetails{SwIfIndex: interface_types.InterfaceIndex(swif), NInput: nInput, Acls: acls}
}

// Binding returns the ACL list of an interface (nil when none).
func (m *ACLModel) Binding(swif uint32) (nInput uint8, acls []uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bindings[swif]
	if !ok {
		return 0, nil
	}
	return b.NInput, append([]uint32(nil), b.Acls...)
}

// ACLs returns index → tag of every ACL.
func (m *ACLModel) ACLs() map[uint32]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[uint32]string{}
	for i, a := range m.acls {
		out[i] = a.Tag
	}
	return out
}

// Rules returns the rules of ACL idx.
func (m *ACLModel) Rules(idx uint32) []acl_types.ACLRule {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.acls[idx]; ok {
		return append([]acl_types.ACLRule(nil), a.R...)
	}
	return nil
}

// MacipBinding returns the MACIP ACL bound on an interface.
func (m *ACLModel) MacipBinding(swif uint32) (uint32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.macipBind[swif]
	return i, ok
}

// SetHits sets rule rule's counter of ACL idx on worker 0.
func (m *ACLModel) SetHits(idx uint32, rule int, packets, bytes uint64) {
	m.Stats.set(fmt.Sprintf("/acl/%d/matches", idx), rule, packets, bytes)
}

// FakeStats is a StatsSource (ListStats/DumpStats) over combined counter vectors.
type FakeStats struct {
	mu      sync.Mutex
	vectors map[string]adapter.CombinedCounterStat
	// Fail makes every call fail.
	Fail error
}

// NewFakeStats returns an empty stats segment.
func NewFakeStats() *FakeStats { return &FakeStats{vectors: map[string]adapter.CombinedCounterStat{}} }

func (s *FakeStats) ensure(path string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.vectors[path]
	if len(w) == 0 {
		w = adapter.CombinedCounterStat{nil}
	}
	for len(w[0]) < n {
		w[0] = append(w[0], adapter.CombinedCounter{0, 0})
	}
	for i := range w[0] { // reset like validate_and_reset_acl_counters
		w[0][i] = adapter.CombinedCounter{0, 0}
	}
	s.vectors[path] = w
}

func (s *FakeStats) set(path string, i int, packets, bytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.vectors[path]
	if len(w) == 0 {
		w = adapter.CombinedCounterStat{nil}
	}
	for len(w[0]) <= i {
		w[0] = append(w[0], adapter.CombinedCounter{0, 0})
	}
	w[0][i] = adapter.CombinedCounter{packets, bytes}
	s.vectors[path] = w
}

func (s *FakeStats) matching(patterns []string) ([]string, error) {
	var res []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", p, err)
		}
		res = append(res, re)
	}
	var out []string
	for path := range s.vectors {
		for _, re := range res {
			if re.MatchString(path) {
				out = append(out, path)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// ListStats implements descriptors/acl.StatsSource.
func (s *FakeStats) ListStats(patterns ...string) ([]adapter.StatIdentifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail != nil {
		return nil, s.Fail
	}
	paths, err := s.matching(patterns)
	if err != nil {
		return nil, err
	}
	out := make([]adapter.StatIdentifier, 0, len(paths))
	for i, p := range paths {
		out = append(out, adapter.StatIdentifier{Index: uint32(i), Name: []byte(p)}) //nolint:gosec // test sizes
	}
	return out, nil
}

// DumpStats implements descriptors/acl.StatsSource.
func (s *FakeStats) DumpStats(patterns ...string) ([]adapter.StatEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail != nil {
		return nil, s.Fail
	}
	paths, err := s.matching(patterns)
	if err != nil {
		return nil, err
	}
	out := make([]adapter.StatEntry, 0, len(paths))
	for i, p := range paths {
		v := make(adapter.CombinedCounterStat, len(s.vectors[p]))
		for w := range s.vectors[p] {
			v[w] = append([]adapter.CombinedCounter(nil), s.vectors[p][w]...)
		}
		out = append(out, adapter.StatEntry{
			StatIdentifier: adapter.StatIdentifier{Index: uint32(i), Name: []byte(p)}, //nolint:gosec // test sizes
			Type:           adapter.CombinedCounterVector,
			Data:           v,
		})
	}
	return out, nil
}
