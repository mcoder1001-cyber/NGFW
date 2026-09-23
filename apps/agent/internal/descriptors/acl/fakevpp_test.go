package acl

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
)

// VPP retvals the fake returns (vnet/api_errno.h); the tests only rely on them being non-zero.
const (
	rvInvalidSwIfIndex   int32 = -2
	rvNoSuchEntry        int32 = -6
	rvInvalidValue       int32 = -7
	rvInvalidValue2      int32 = -8
	rvEntryAlreadyExists int32 = -12
	rvACLInUseInbound    int32 = -142
)

// Interfaces the fake VPP starts with (sw_if_index → name, tag).
const (
	ifLocal0   = 0 // untagged, never ours
	ifLoop1040 = 1 // ours (owner w10)
	ifLoop1041 = 2 // ours
	ifLoop300  = 3 // another owner's (w3)
	ifHostEth0 = 4 // untagged physical port
)

// fakeVPP models the acl plugin (and the interface dump) on top of internal/vpp/fake: state in
// maps, semantics copied from acl.c (index allocation, replace-in-place, in-use checks, one
// MACIP ACL per interface, count-0 details for interfaces without lists).
type fakeVPP struct {
	*fake.Client
	mu               sync.Mutex
	ifaces           map[uint32]*interfaces.SwInterfaceDetails
	acls             map[uint32]*vppacl.ACLDetails
	nextACL          uint32
	bindings         map[uint32]*vppacl.ACLInterfaceListDetails
	etypes           map[uint32]*vppacl.ACLInterfaceEtypeWhitelistDetails
	macips           map[uint32]*vppacl.MacipACLDetails
	nextMacip        uint32
	macipBind        map[uint32]uint32 // sw_if_index → macip acl index
	countersEnabled  bool
	countersCalls    int
	properStatsReply bool   // false = mirror VPP 26.06 (replies with acl_del_reply)
	vppPID           uint32 // vpe_pid reported by control_ping (the PID part of the VPP boot identity)
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{
		Client:    fake.New(),
		ifaces:    map[uint32]*interfaces.SwInterfaceDetails{},
		acls:      map[uint32]*vppacl.ACLDetails{},
		bindings:  map[uint32]*vppacl.ACLInterfaceListDetails{},
		etypes:    map[uint32]*vppacl.ACLInterfaceEtypeWhitelistDetails{},
		macips:    map[uint32]*vppacl.MacipACLDetails{},
		macipBind: map[uint32]uint32{},
		vppPID:    4242,
	}
	v.addInterface(ifLocal0, "local0", "")
	v.addInterface(ifLoop1040, "loop1040", "w10:loop1040")
	v.addInterface(ifLoop1041, "loop1041", "w10:loop1041")
	v.addInterface(ifLoop300, "loop300", "w3:loop300")
	v.addInterface(ifHostEth0, "host-eth0", "")

	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedKeys(v.ifaces) {
			d := *v.ifaces[idx]
			out = append(out, &d)
		}
		return out, nil
	})
	v.On("acl_add_replace", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLAddReplace)
		v.mu.Lock()
		defer v.mu.Unlock()
		if len(r.Tag) >= 64 {
			return reply(&vppacl.ACLAddReplaceReply{Retval: rvInvalidValue})
		}
		for _, rule := range r.R {
			if rule.SrcPrefix.Address.Af != rule.DstPrefix.Address.Af {
				return reply(&vppacl.ACLAddReplaceReply{Retval: rvInvalidValue})
			}
			if rule.SrcportOrIcmptypeFirst > rule.SrcportOrIcmptypeLast || rule.DstportOrIcmpcodeFirst > rule.DstportOrIcmpcodeLast {
				return reply(&vppacl.ACLAddReplaceReply{Retval: rvInvalidValue2})
			}
		}
		idx := r.ACLIndex
		if idx == noACL {
			idx = v.nextACL
			v.nextACL++
		} else if _, ok := v.acls[idx]; !ok {
			return reply(&vppacl.ACLAddReplaceReply{Retval: rvNoSuchEntry})
		}
		v.acls[idx] = &vppacl.ACLDetails{ACLIndex: idx, Tag: r.Tag, R: append([]acl_types.ACLRule{}, r.R...)}
		return reply(&vppacl.ACLAddReplaceReply{ACLIndex: idx})
	})
	v.On("acl_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.acls[r.ACLIndex]; !ok {
			return reply(&vppacl.ACLDelReply{Retval: rvNoSuchEntry})
		}
		for _, b := range v.bindings {
			for _, idx := range b.Acls {
				if idx == r.ACLIndex {
					return reply(&vppacl.ACLDelReply{Retval: rvACLInUseInbound})
				}
			}
		}
		delete(v.acls, r.ACLIndex)
		return reply(&vppacl.ACLDelReply{})
	})
	v.On("acl_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedKeys(v.acls) {
			if r.ACLIndex == noACL || r.ACLIndex == idx {
				d := *v.acls[idx]
				d.R = append([]acl_types.ACLRule{}, d.R...)
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("acl_interface_set_acl_list", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLInterfaceSetACLList)
		v.mu.Lock()
		defer v.mu.Unlock()
		swif := uint32(r.SwIfIndex)
		if _, ok := v.ifaces[swif]; !ok {
			return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: rvInvalidSwIfIndex})
		}
		if int(r.NInput) > len(r.Acls) {
			return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: rvInvalidValue})
		}
		seen := map[bool]map[uint32]bool{true: {}, false: {}}
		for i, idx := range r.Acls {
			if _, ok := v.acls[idx]; !ok {
				return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: rvNoSuchEntry})
			}
			in := i < int(r.NInput)
			if seen[in][idx] {
				return reply(&vppacl.ACLInterfaceSetACLListReply{Retval: rvEntryAlreadyExists})
			}
			seen[in][idx] = true
		}
		if len(r.Acls) == 0 {
			delete(v.bindings, swif)
		} else {
			v.bindings[swif] = &vppacl.ACLInterfaceListDetails{SwIfIndex: r.SwIfIndex, NInput: r.NInput, Acls: append([]uint32{}, r.Acls...)}
		}
		return reply(&vppacl.ACLInterfaceSetACLListReply{})
	})
	v.On("acl_interface_list_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLInterfaceListDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedKeys(v.ifaces) {
			if uint32(r.SwIfIndex) != allInterfaces && uint32(r.SwIfIndex) != idx {
				continue
			}
			if b, ok := v.bindings[idx]; ok {
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
		v.mu.Lock()
		defer v.mu.Unlock()
		swif := uint32(r.SwIfIndex)
		if _, ok := v.ifaces[swif]; !ok {
			return reply(&vppacl.ACLInterfaceSetEtypeWhitelistReply{Retval: rvInvalidSwIfIndex})
		}
		if len(r.Whitelist) == 0 {
			delete(v.etypes, swif)
		} else {
			v.etypes[swif] = &vppacl.ACLInterfaceEtypeWhitelistDetails{SwIfIndex: r.SwIfIndex, NInput: r.NInput, Whitelist: append([]uint16{}, r.Whitelist...)}
		}
		return reply(&vppacl.ACLInterfaceSetEtypeWhitelistReply{})
	})
	v.On("acl_interface_etype_whitelist_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedKeys(v.ifaces) {
			if w, ok := v.etypes[idx]; ok {
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
		v.mu.Lock()
		defer v.mu.Unlock()
		idx := r.ACLIndex
		if idx == noACL {
			idx = v.nextMacip
			v.nextMacip++
		} else if _, ok := v.macips[idx]; !ok {
			return reply(&vppacl.MacipACLAddReplaceReply{Retval: rvNoSuchEntry})
		}
		v.macips[idx] = &vppacl.MacipACLDetails{ACLIndex: idx, Tag: r.Tag, R: append([]acl_types.MacipACLRule{}, r.R...)}
		return reply(&vppacl.MacipACLAddReplaceReply{ACLIndex: idx})
	})
	v.On("macip_acl_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.macips[r.ACLIndex]; !ok {
			return reply(&vppacl.MacipACLDelReply{Retval: rvNoSuchEntry})
		}
		// unlike acl_del, macip_acl_del unapplies the ACL from every interface first
		// (acl.c macip_acl_del_list → macip_acl_interface_del_acl)
		for swif, bound := range v.macipBind {
			if bound == r.ACLIndex {
				delete(v.macipBind, swif)
			}
		}
		delete(v.macips, r.ACLIndex)
		return reply(&vppacl.MacipACLDelReply{})
	})
	v.On("macip_acl_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedKeys(v.macips) {
			if r.ACLIndex == noACL || r.ACLIndex == idx {
				d := *v.macips[idx]
				d.R = append([]acl_types.MacipACLRule{}, d.R...)
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("macip_acl_interface_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLInterfaceAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		swif := uint32(r.SwIfIndex)
		if _, ok := v.ifaces[swif]; !ok {
			return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: rvInvalidSwIfIndex})
		}
		if r.IsAdd {
			if _, ok := v.macips[r.ACLIndex]; !ok {
				return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: rvNoSuchEntry})
			}
			v.macipBind[swif] = r.ACLIndex // replaces an existing one, like acl.c
			return reply(&vppacl.MacipACLInterfaceAddDelReply{})
		}
		if _, ok := v.macipBind[swif]; !ok {
			return reply(&vppacl.MacipACLInterfaceAddDelReply{Retval: rvNoSuchEntry})
		}
		delete(v.macipBind, swif)
		return reply(&vppacl.MacipACLInterfaceAddDelReply{})
	})
	v.On("macip_acl_interface_list_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.MacipACLInterfaceListDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, swif := range sortedKeys(v.macipBind) {
			if uint32(r.SwIfIndex) != allInterfaces && uint32(r.SwIfIndex) != swif {
				continue
			}
			out = append(out, &vppacl.MacipACLInterfaceListDetails{SwIfIndex: interface_types.InterfaceIndex(swif), Acls: []uint32{v.macipBind[swif]}})
		}
		return out, nil
	})
	v.On("acl_stats_intf_counters_enable", func(req api.Message) ([]api.Message, error) {
		r := req.(*vppacl.ACLStatsIntfCountersEnable)
		v.mu.Lock()
		defer v.mu.Unlock()
		v.countersEnabled = r.Enable
		v.countersCalls++
		if v.properStatsReply {
			return reply(&vppacl.ACLStatsIntfCountersEnableReply{})
		}
		return reply(&vppacl.ACLDelReply{}) // what VPP 26.06 really sends (acl.c REPLY_MACRO (VL_API_ACL_DEL_REPLY))
	})
	v.On("control_ping", func(api.Message) ([]api.Message, error) { // dumps + VPP boot identity
		v.mu.Lock()
		defer v.mu.Unlock()
		return reply(&memclnt.ControlPingReply{VpePID: v.vppPID})
	})
	v.Reply("acl_plugin_get_version", &vppacl.ACLPluginGetVersionReply{Major: 1, Minor: 0})
	v.Reply("acl_plugin_get_conn_table_max_entries", &vppacl.ACLPluginGetConnTableMaxEntriesReply{ConnTableMaxEntries: 1 << 20})
	return v
}

func reply(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }

func sortedKeys[V any](m map[uint32]V) []uint32 {
	keys := make([]uint32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func (v *fakeVPP) addInterface(idx uint32, name, tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ifaces[idx] = &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, Tag: tag}
}

// addACL plants an ACL directly (any owner's), returning its index.
func (v *fakeVPP) addACL(tag string, rules ...acl_types.ACLRule) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	idx := v.nextACL
	v.nextACL++
	v.acls[idx] = &vppacl.ACLDetails{ACLIndex: idx, Tag: tag, R: rules}
	return idx
}

// addMacipACL plants a MACIP ACL directly, returning its index.
func (v *fakeVPP) addMacipACL(tag string, rules ...acl_types.MacipACLRule) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	idx := v.nextMacip
	v.nextMacip++
	v.macips[idx] = &vppacl.MacipACLDetails{ACLIndex: idx, Tag: tag, R: rules}
	return idx
}

// bind plants an interface ACL list directly.
func (v *fakeVPP) bind(swif uint32, nInput uint8, acls ...uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.bindings[swif] = &vppacl.ACLInterfaceListDetails{SwIfIndex: interface_types.InterfaceIndex(swif), NInput: nInput, Acls: acls}
}

// restartVPP simulates a VPP restart for the global state the tests look at: a new VPP
// PID and the counters flag back to its default (off).
func (v *fakeVPP) restartVPP() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.vppPID += 100
	v.countersEnabled = false
}

// setEtypes plants an ethertype whitelist directly.
func (v *fakeVPP) setEtypes(swif uint32, nInput uint8, list ...uint16) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.etypes[swif] = &vppacl.ACLInterfaceEtypeWhitelistDetails{SwIfIndex: interface_types.InterfaceIndex(swif), NInput: nInput, Whitelist: list}
}

func (v *fakeVPP) aclCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.acls)
}

func (v *fakeVPP) hasACL(idx uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.acls[idx]
	return ok
}

// fakeStats is a StatsSource over a map path → combined counter vector.
type fakeStats struct {
	mu      sync.Mutex
	vectors map[string]adapter.CombinedCounterStat
	fail    error
}

func newFakeStats() *fakeStats { return &fakeStats{vectors: map[string]adapter.CombinedCounterStat{}} }

func (s *fakeStats) set(path string, v adapter.CombinedCounterStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectors[path] = v
}

func (s *fakeStats) matching(patterns []string) ([]string, error) {
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

func (s *fakeStats) ListStats(patterns ...string) ([]adapter.StatIdentifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
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

func (s *fakeStats) DumpStats(patterns ...string) ([]adapter.StatEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
	}
	paths, err := s.matching(patterns)
	if err != nil {
		return nil, err
	}
	out := make([]adapter.StatEntry, 0, len(paths))
	for i, p := range paths {
		out = append(out, adapter.StatEntry{
			StatIdentifier: adapter.StatIdentifier{Index: uint32(i), Name: []byte(p)}, //nolint:gosec // test sizes
			Type:           adapter.CombinedCounterVector,
			Data:           s.vectors[p],
		})
	}
	return out, nil
}
