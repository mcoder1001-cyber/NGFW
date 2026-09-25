package coretest

// F-srv6 extension of the model: VPP core `sr` (SRv6 local SIDs with their packet counters, policies with
// segment lists, steering entries, the two write-only globals), so the agent's projection, Retrieve,
// restart and rollback paths run in unit tests with DF-6's real sr descriptors. It follows VPP 26.06
// (src/vnet/srv6): an add on an existing SID/BSID fails, a steering add on an existing key re-points it,
// a policy delete while steering points at it leaves the steering dangling, and every handler that
// resolves a table with fib_table_find() uses ~0 unchecked — the model counts those calls as crashes
// (SR().Crashes must stay 0), and an SR object whose table was deleted under it counts as a leaked
// FIB entry (SR().Leaks, docs/vpp-code-track.md V15).
//
// Wiring: installSRv6 runs from New() (one line; after TD-23 it registers with RegisterExtension).

import (
	"fmt"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
)

// SRCounters are the counters sr_localsids_with_packet_stats_dump reports for one local SID.
type SRCounters struct{ GoodPackets, GoodBytes, BadPackets, BadBytes uint64 }

type srSteerKey struct {
	typ   sr_types.SrSteer
	table uint32
	pfx   ip_types.Prefix
	sw    uint32
}

// SRModel is the SRv6 state of one model (SR()). Fields are guarded by mu.
type SRModel struct {
	mu       sync.Mutex
	v        *VPP
	localsid map[ip_types.IP6Address]*srapi.SrLocalsidsDetails
	counters map[ip_types.IP6Address]SRCounters
	policies []*srapi.SrPoliciesV2Details
	steer    map[srSteerKey]*srapi.SrSteeringPolDetails
	encapSrc ip_types.IP6Address
	hopLimit uint8
	crashes  []string
}

var (
	srMu     sync.Mutex
	srModels = map[*VPP]*SRModel{}
)

// SR returns the SRv6 part of the model.
func (v *VPP) SR() *SRModel {
	srMu.Lock()
	defer srMu.Unlock()
	return srModels[v]
}

func (m *SRModel) hasTable(id uint32, v6 bool) bool {
	if id == 0 {
		return true
	}
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	_, ok := m.v.Tables[tableKey{id, v6}]
	return ok
}

func (m *SRModel) crash(format string, a ...any) {
	m.crashes = append(m.crashes, fmt.Sprintf(format, a...))
}

func (m *SRModel) policy(bsid ip_types.IP6Address) (int, *srapi.SrPoliciesV2Details) {
	for i, p := range m.policies {
		if p.Bsid == bsid {
			return i, p
		}
	}
	return -1, nil
}

// Crashes lists the calls that would have crashed VPP 26.06 (unchecked fib_table_find, dangling steering).
func (m *SRModel) Crashes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.crashes...)
}

// Leaks lists SR objects whose FIB table no longer exists (VPP leaks their FIB entries, V15).
func (m *SRModel) Leaks() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, l := range m.localsid {
		if !m.hasTable(l.FibTable, true) {
			out = append(out, fmt.Sprintf("localsid %s in ip6 table %d", ip6(l.Addr), l.FibTable))
		}
	}
	for _, p := range m.policies {
		if !m.hasTable(p.FibTable, true) {
			out = append(out, fmt.Sprintf("policy %s in ip6 table %d", ip6(p.Bsid), p.FibTable))
		}
	}
	for k := range m.steer {
		if k.typ != sr_types.SR_STEER_API_L2 && !m.hasTable(k.table, k.typ == sr_types.SR_STEER_API_IPV6) {
			out = append(out, fmt.Sprintf("steering in table %d", k.table))
		}
	}
	sort.Strings(out)
	return out
}

// Counts returns the number of local SIDs, policies and steering entries.
func (m *SRModel) Counts() (sids, policies, steering int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.localsid), len(m.policies), len(m.steer)
}

// SetCounters sets the packet counters of local SID sid (canonical IPv6 text).
func (m *SRModel) SetCounters(sid string, c SRCounters) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[mustIP6(sid)] = c
}

// Globals returns the encap source (canonical text, "::" when unset) and the hop limit.
func (m *SRModel) Globals() (string, uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ip6(m.encapSrc), m.hopLimit
}

// DeleteAll removes every SR object behind the agent's back (restart simulation: "VPP lost them"),
// steering first so no steering entry dangles.
func (m *SRModel) DeleteAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steer = map[srSteerKey]*srapi.SrSteeringPolDetails{}
	m.policies = nil
	m.localsid = map[ip_types.IP6Address]*srapi.SrLocalsidsDetails{}
}

// AddForeignLocalSid adds a local SID nobody claimed (another owner's or an operator's).
func (m *SRModel) AddForeignLocalSid(sid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := mustIP6(sid)
	m.localsid[a] = &srapi.SrLocalsidsDetails{Addr: a, Behavior: sr_types.SR_BEHAVIOR_API_END}
}

func mustIP6(s string) ip_types.IP6Address {
	a, err := ip_types.ParseIP6Address(s)
	if err != nil {
		panic(err)
	}
	return a
}

func ip6(a ip_types.IP6Address) string { return a.String() }

func (v *VPP) installSRv6() {
	m := &SRModel{
		v: v, localsid: map[ip_types.IP6Address]*srapi.SrLocalsidsDetails{}, counters: map[ip_types.IP6Address]SRCounters{},
		steer: map[srSteerKey]*srapi.SrSteeringPolDetails{}, hopLimit: 64,
	}
	srMu.Lock()
	srModels[v] = m
	srMu.Unlock()

	v.On("sr_localsid_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrLocalsidAddDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.hasTable(r.FibTable, true) {
			if r.IsDel {
				m.crash("sr_localsid_add_del(del) %s: no ip6 table %d", ip6(r.Localsid), r.FibTable)
			}
			return reply(&srapi.SrLocalsidAddDelReply{Retval: RetvalNoSuchFib})
		}
		if r.IsDel {
			if _, ok := m.localsid[r.Localsid]; !ok {
				return reply(&srapi.SrLocalsidAddDelReply{Retval: RetvalNoSuchEntry})
			}
			delete(m.localsid, r.Localsid)
			delete(m.counters, r.Localsid)
			return reply(&srapi.SrLocalsidAddDelReply{})
		}
		if _, ok := m.localsid[r.Localsid]; ok {
			return reply(&srapi.SrLocalsidAddDelReply{Retval: -1})
		}
		d := &srapi.SrLocalsidsDetails{Addr: r.Localsid, EndPsp: r.EndPsp, Behavior: r.Behavior, FibTable: r.FibTable, XconnectIfaceOrVrfTable: uint32(r.SwIfIndex)}
		switch r.Behavior {
		case sr_types.SR_BEHAVIOR_API_X, sr_types.SR_BEHAVIOR_API_DX4, sr_types.SR_BEHAVIOR_API_DX6:
			d.XconnectNhAddr = r.NhAddr
		case sr_types.SR_BEHAVIOR_API_END:
			d.XconnectIfaceOrVrfTable = 0
		case sr_types.SR_BEHAVIOR_API_DT4:
			if !m.hasTable(uint32(r.SwIfIndex), false) {
				m.crash("sr_localsid_add_del end.dt4 %s: no ip4 table %d", ip6(r.Localsid), r.SwIfIndex)
			}
		case sr_types.SR_BEHAVIOR_API_T, sr_types.SR_BEHAVIOR_API_DT6:
			if !m.hasTable(uint32(r.SwIfIndex), true) {
				m.crash("sr_localsid_add_del end.t/dt6 %s: no ip6 table %d", ip6(r.Localsid), r.SwIfIndex)
			}
		}
		m.localsid[r.Localsid] = d
		return reply(&srapi.SrLocalsidAddDelReply{})
	})
	sids := func() []ip_types.IP6Address {
		keys := make([]ip_types.IP6Address, 0, len(m.localsid))
		for k := range m.localsid {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return ip6(keys[i]) < ip6(keys[j]) })
		return keys
	}
	v.On("sr_localsids_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, k := range sids() {
			c := *m.localsid[k]
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("sr_localsids_with_packet_stats_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, k := range sids() {
			l, c := m.localsid[k], m.counters[k]
			out = append(out, &srapi.SrLocalsidsWithPacketStatsDetails{
				Addr: l.Addr, EndPsp: l.EndPsp, Behavior: l.Behavior, FibTable: l.FibTable, VlanIndex: l.VlanIndex,
				XconnectNhAddr: l.XconnectNhAddr, XconnectIfaceOrVrfTable: l.XconnectIfaceOrVrfTable,
				GoodTrafficBytes: c.GoodBytes, GoodTrafficPktCount: c.GoodPackets, BadTrafficBytes: c.BadBytes, BadTrafficPktCount: c.BadPackets,
			})
		}
		return out, nil
	})
	v.On("sr_policy_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyAddV2)
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.hasTable(r.FibTable, true) {
			m.crash("sr_policy_add_v2 %s: no ip6 table %d", ip6(r.BsidAddr), r.FibTable)
			return reply(&srapi.SrPolicyAddV2Reply{Retval: -1})
		}
		if _, p := m.policy(r.BsidAddr); p != nil {
			return reply(&srapi.SrPolicyAddV2Reply{Retval: -12})
		}
		src := r.EncapSrc
		if src == (ip_types.IP6Address{}) {
			src = m.encapSrc
		}
		sl := r.Sids
		sl.Weight = r.Weight
		m.policies = append(m.policies, &srapi.SrPoliciesV2Details{Bsid: r.BsidAddr, EncapSrc: src, Type: r.Type, IsEncap: r.IsEncap, FibTable: r.FibTable, SidLists: []srapi.Srv6SidList{sl}})
		return reply(&srapi.SrPolicyAddV2Reply{})
	})
	v.On("sr_policy_mod_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyModV2)
		m.mu.Lock()
		defer m.mu.Unlock()
		_, p := m.policy(r.BsidAddr)
		if p == nil || r.Operation != sr_types.SR_POLICY_OP_API_ADD {
			return reply(&srapi.SrPolicyModV2Reply{Retval: -1})
		}
		sl := r.Sids
		sl.Weight = r.Weight
		p.SidLists = append(p.SidLists, sl)
		return reply(&srapi.SrPolicyModV2Reply{})
	})
	v.On("sr_policy_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrPolicyDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		i, p := m.policy(r.BsidAddr)
		if p == nil {
			return reply(&srapi.SrPolicyDelReply{Retval: -1})
		}
		for _, s := range m.steer {
			if s.Bsid == r.BsidAddr {
				m.crash("sr_policy_del %s: steering still points at it (dangling)", ip6(r.BsidAddr))
			}
		}
		m.policies = append(m.policies[:i], m.policies[i+1:]...)
		return reply(&srapi.SrPolicyDelReply{})
	})
	v.On("sr_policies_v2_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, p := range m.policies {
			c := *p
			c.SidLists = append([]srapi.Srv6SidList(nil), p.SidLists...)
			c.NumSidLists = uint8(len(c.SidLists)) //nolint:gosec // small
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("sr_steering_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srapi.SrSteeringAddDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		k := srSteerKey{typ: r.TrafficType}
		if r.TrafficType == sr_types.SR_STEER_API_L2 {
			k.sw = uint32(r.SwIfIndex)
		} else {
			k.table, k.pfx = r.TableID, r.Prefix
			if !m.hasTable(r.TableID, r.TrafficType == sr_types.SR_STEER_API_IPV6) {
				m.crash("sr_steering_add_del: no table %d", r.TableID)
				return reply(&srapi.SrSteeringAddDelReply{Retval: -1})
			}
		}
		if r.IsDel {
			if _, ok := m.steer[k]; !ok {
				return reply(&srapi.SrSteeringAddDelReply{Retval: -4})
			}
			delete(m.steer, k)
			return reply(&srapi.SrSteeringAddDelReply{})
		}
		_, p := m.policy(r.BsidAddr)
		if p == nil {
			return reply(&srapi.SrSteeringAddDelReply{Retval: -2})
		}
		if !p.IsEncap && (r.TrafficType == sr_types.SR_STEER_API_L2 || r.TrafficType == sr_types.SR_STEER_API_IPV4) {
			return reply(&srapi.SrSteeringAddDelReply{Retval: -5}) // L2 / IPv4 steering need an encap policy
		}
		d := &srapi.SrSteeringPolDetails{TrafficType: r.TrafficType, Bsid: r.BsidAddr, SwIfIndex: r.SwIfIndex}
		if r.TrafficType != sr_types.SR_STEER_API_L2 {
			d.FibTable, d.Prefix, d.SwIfIndex = r.TableID, r.Prefix, 0
		}
		m.steer[k] = d // add on an existing key re-points it
		return reply(&srapi.SrSteeringAddDelReply{})
	})
	v.On("sr_steering_pol_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		keys := make([]srSteerKey, 0, len(m.steer))
		for k := range m.steer {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		var out []api.Message
		for _, k := range keys {
			c := *m.steer[k]
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("sr_set_encap_source", func(req api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.encapSrc = req.(*srapi.SrSetEncapSource).EncapsSource
		return reply(&srapi.SrSetEncapSourceReply{})
	})
	v.On("sr_set_encap_hop_limit", func(req api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.hopLimit = req.(*srapi.SrSetEncapHopLimit).HopLimit
		return reply(&srapi.SrSetEncapHopLimitReply{})
	})
}
