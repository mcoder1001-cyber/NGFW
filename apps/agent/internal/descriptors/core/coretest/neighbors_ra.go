package coretest

// F-neighbors-ra (wave-A-hotspots A6): the agent-level model of the DF-2 families the feature registers — neighbours
// (ip_neighbor_*), router advertisements, proxy ND and DAD (ip6_nd, ip6_dad) and proxy ARP (arp) — with VPP's
// semantics where the agent depends on them:
//   - RA state exists only while the interface has an IPv6 address (VPP's ip6-link lock; the last address removed
//     destroys it); sw_interface_ip6nd_ra_config/_prefix answer IP6_NOT_ENABLED otherwise; the toggle semantics of
//     ip6_ra_config (a zero field is "unchanged", is_no restores the default of every non-zero field);
//   - ip_neighbor_dump / want_ip_neighbor_events_v2 with sw_if_index ~0 (or 0 for the watcher) mean "every interface";
//     the model counts such calls (AllInterfaceCalls) so tests can assert the agent never makes them;
//   - ip_neighbor_add_del is_add=0 of a missing entry answers NO_SUCH_ENTRY; ip_neighbor_flush removes static entries
//     as well (ip_neighbor_del_all).
// Learn/Forget add or remove a learned entry and emit ip_neighbor_event_v2 to subscribed interfaces.

import (
	"net/netip"
	"sort"
	"strings"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/arp"
	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_dad"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
)

// VPP retvals of the neighbour/RA model (vnet/api_errno.h).
const (
	RetvalIP6NotEnabled int32 = -25
	RetvalInvalidValue  int32 = -52
)

type nbrKey struct {
	idx uint32
	ip  netip.Addr
}

type raState struct {
	name     string // interface name the state belongs to (a reused index starts fresh)
	det      ip6_nd.SwInterfaceIP6ndRaDetails
	prefixes []ip6_nd.IP6ndRaPrefix
}

// NeighborsRa is the F-neighbors-ra part of the model (NeighborsRaOf).
type NeighborsRa struct {
	mu        sync.Mutex
	nbrs      map[nbrKey]ip_neighbor.IPNeighbor
	age       map[nbrKey]float64
	config    map[ip_types.AddressFamily]ip_neighbor.IPNeighborConfig
	ra        map[uint32]*raState
	proxyNd   map[nbrKey]bool
	proxyNdOn map[uint32]bool
	dad       ip6_dad.IP6DadDetails
	ranges    []arp.ProxyArp
	proxyArp  map[uint32]bool
	watch     map[uint32]uint32 // sw_if_index → pid
	// AllInterfaceCalls counts dumps/flushes/subscriptions that named every interface (~0, or 0 for the watcher).
	AllInterfaceCalls int
}

var neighborsRaModels sync.Map // *VPP → *NeighborsRa

// NeighborsRaOf returns the F-neighbors-ra model of v.
func NeighborsRaOf(v *VPP) *NeighborsRa {
	m, _ := neighborsRaModels.Load(v)
	return m.(*NeighborsRa)
}

func (v *VPP) hasIPv6Locked(idx uint32) (string, bool) {
	i, ok := v.Ifaces[idx]
	if !ok {
		return "", false
	}
	for a := range i.Addrs {
		if strings.Contains(a, ":") {
			return i.Name, true
		}
	}
	return i.Name, false
}

func defaultRaDetails(idx uint32) ip6_nd.SwInterfaceIP6ndRaDetails {
	return ip6_nd.SwInterfaceIP6ndRaDetails{
		SwIfIndex: interface_types.InterfaceIndex(idx), SendRadv: false, AdvLinkLayerAddress: true, AdvRouterLifetime: 600,
		MaxRadvInterval: 200, MinRadvInterval: 150, InitialAdvertsCount: 3, InitialAdvertsInterval: 16, CurHopLimit: 64,
	}
}

// raLocked returns idx's RA state, creating the default one while IPv6 is enabled and dropping it when not (the VPP
// ip6-link lock): nil when the interface has no IPv6 address. Caller holds v.mu and m.mu.
func (m *NeighborsRa) raLocked(v *VPP, idx uint32) *raState {
	name, on := v.hasIPv6Locked(idx)
	st := m.ra[idx]
	if !on {
		delete(m.ra, idx)
		return nil
	}
	if st == nil || st.name != name {
		st = &raState{name: name, det: defaultRaDetails(idx)}
		m.ra[idx] = st
	}
	return st
}

func applyRa(d *ip6_nd.SwInterfaceIP6ndRaDetails, r *ip6_nd.SwInterfaceIP6ndRaConfig) int32 {
	no := r.IsNo
	set := func(flag uint8, cur *bool, on bool) {
		if flag != 0 {
			*cur = on != no
		}
	}
	maxI, minI, lt := d.MaxRadvInterval, d.MinRadvInterval, float64(d.AdvRouterLifetime)
	if r.MaxInterval != 0 {
		maxI = float64(r.MaxInterval)
		if no {
			maxI = 200
		}
	}
	if r.MinInterval != 0 || r.MaxInterval != 0 {
		minI = float64(r.MinInterval)
		if r.MinInterval == 0 {
			minI = 0.75 * float64(r.MaxInterval)
		}
		if no {
			minI = 150
		}
	}
	if r.DefaultRouter != 0 {
		lt = float64(r.Lifetime)
		if no {
			lt = 600
		}
	}
	if lt > 9000 {
		lt = 9000
	}
	if (lt != 0 && lt <= maxI) || minI > 0.75*maxI || minI < 3 {
		return RetvalInvalidValue
	}
	set(r.Suppress, &d.SendRadv, false)
	set(r.Managed, &d.AdvManagedFlag, true)
	set(r.Other, &d.AdvOtherFlag, true)
	set(r.LlOption, &d.AdvLinkLayerAddress, false)
	set(r.SendUnicast, &d.SendUnicast, true)
	set(r.Cease, &d.CeaseRadv, true)
	d.MaxRadvInterval, d.MinRadvInterval, d.AdvRouterLifetime = maxI, minI, uint16(lt)
	if r.InitialCount != 0 {
		d.InitialAdvertsCount = r.InitialCount
		if no {
			d.InitialAdvertsCount = 3
		}
	}
	if r.InitialInterval != 0 {
		d.InitialAdvertsInterval = float64(r.InitialInterval)
		if no {
			d.InitialAdvertsInterval = 16
		}
	}
	return 0
}

func addrOf(a ip_types.Address) netip.Addr {
	if a.Af == ip_types.ADDRESS_IP6 {
		return netip.AddrFrom16(a.Un.GetIP6())
	}
	return netip.AddrFrom4(a.Un.GetIP4())
}

func (v *VPP) installNeighborsRa() {
	m := &NeighborsRa{
		nbrs: map[nbrKey]ip_neighbor.IPNeighbor{}, age: map[nbrKey]float64{},
		config: map[ip_types.AddressFamily]ip_neighbor.IPNeighborConfig{}, ra: map[uint32]*raState{},
		proxyNd: map[nbrKey]bool{}, proxyNdOn: map[uint32]bool{}, proxyArp: map[uint32]bool{}, watch: map[uint32]uint32{},
		dad: ip6_dad.IP6DadDetails{DadTransmits: 1, DadRetransmitDelay: 1},
	}
	neighborsRaModels.Store(v, m)
	lock := func() func() {
		v.mu.Lock()
		m.mu.Lock()
		return func() { m.mu.Unlock(); v.mu.Unlock() }
	}
	exists := func(idx uint32) bool { _, ok := v.Ifaces[idx]; return ok }

	// ---- ip_neighbor
	v.On("ip_neighbor_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborAddDel)
		defer lock()()
		idx := uint32(r.Neighbor.SwIfIndex)
		if !exists(idx) {
			return reply(&ip_neighbor.IPNeighborAddDelReply{Retval: RetvalInvalidSwIfIndex})
		}
		k := nbrKey{idx, addrOf(r.Neighbor.IPAddress)}
		_, had := m.nbrs[k]
		if !r.IsAdd {
			if !had {
				return reply(&ip_neighbor.IPNeighborAddDelReply{Retval: RetvalNoSuchEntry})
			}
			delete(m.nbrs, k)
			delete(m.age, k)
			return reply(&ip_neighbor.IPNeighborAddDelReply{})
		}
		m.nbrs[k] = r.Neighbor
		if _, ok := m.age[k]; !ok {
			m.age[k] = 3 // VPP reports an age for every entry, static ones included
		}
		return reply(&ip_neighbor.IPNeighborAddDelReply{})
	})
	v.On("ip_neighbor_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborDump)
		defer lock()()
		all := uint32(r.SwIfIndex) == ^uint32(0)
		if all {
			m.AllInterfaceCalls++
		}
		var keys []nbrKey
		for k, n := range m.nbrs {
			if n.IPAddress.Af == r.Af && (all || k.idx == uint32(r.SwIfIndex)) && exists(k.idx) {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].idx != keys[j].idx {
				return keys[i].idx < keys[j].idx
			}
			return keys[i].ip.Less(keys[j].ip)
		})
		out := make([]api.Message, 0, len(keys))
		for _, k := range keys {
			out = append(out, &ip_neighbor.IPNeighborDetails{Age: m.age[k], Neighbor: m.nbrs[k]})
		}
		return out, nil
	})
	v.On("ip_neighbor_flush", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborFlush)
		defer lock()()
		all := uint32(r.SwIfIndex) == ^uint32(0)
		if all {
			m.AllInterfaceCalls++
		}
		for k, n := range m.nbrs { // ip_neighbor_del_all: static entries too
			if n.IPAddress.Af == r.Af && (all || k.idx == uint32(r.SwIfIndex)) {
				delete(m.nbrs, k)
			}
		}
		return reply(&ip_neighbor.IPNeighborFlushReply{})
	})
	v.On("ip_neighbor_config", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.IPNeighborConfig)
		defer lock()()
		m.config[r.Af] = *r
		return reply(&ip_neighbor.IPNeighborConfigReply{})
	})
	v.On("ip_neighbor_config_get", func(req api.Message) ([]api.Message, error) {
		af := req.(*ip_neighbor.IPNeighborConfigGet).Af
		defer lock()()
		c, ok := m.config[af]
		if !ok {
			c = ip_neighbor.IPNeighborConfig{Af: af, MaxNumber: 50000}
		}
		return reply(&ip_neighbor.IPNeighborConfigGetReply{Af: af, MaxNumber: c.MaxNumber, MaxAge: c.MaxAge, Recycle: c.Recycle})
	})
	v.On("want_ip_neighbor_events_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip_neighbor.WantIPNeighborEventsV2)
		defer lock()()
		idx := uint32(r.SwIfIndex)
		if idx == 0 || idx == ^uint32(0) {
			m.AllInterfaceCalls++
			return reply(&ip_neighbor.WantIPNeighborEventsV2Reply{})
		}
		if !exists(idx) {
			return reply(&ip_neighbor.WantIPNeighborEventsV2Reply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.Enable {
			m.watch[idx] = r.PID
		} else {
			delete(m.watch, idx)
		}
		return reply(&ip_neighbor.WantIPNeighborEventsV2Reply{})
	})

	// ---- ip6_nd: router advertisements
	v.On("sw_interface_ip6nd_ra_config", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_nd.SwInterfaceIP6ndRaConfig)
		defer lock()()
		st := m.raLocked(v, uint32(r.SwIfIndex))
		if st == nil {
			return reply(&ip6_nd.SwInterfaceIP6ndRaConfigReply{Retval: RetvalIP6NotEnabled})
		}
		return reply(&ip6_nd.SwInterfaceIP6ndRaConfigReply{Retval: applyRa(&st.det, r)})
	})
	v.On("sw_interface_ip6nd_ra_prefix", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_nd.SwInterfaceIP6ndRaPrefix)
		defer lock()()
		st := m.raLocked(v, uint32(r.SwIfIndex))
		if st == nil {
			return reply(&ip6_nd.SwInterfaceIP6ndRaPrefixReply{Retval: RetvalIP6NotEnabled})
		}
		p := ip6_nd.IP6ndRaPrefix{Prefix: r.Prefix, OnlinkFlag: !r.OffLink, AutonomousFlag: !r.NoAutoconfig, ValLifetime: r.ValLifetime, PrefLifetime: r.PrefLifetime, NoAdvertise: r.NoAdvertise}
		for i, q := range st.prefixes {
			if q.Prefix == r.Prefix {
				if r.IsNo {
					st.prefixes = append(st.prefixes[:i], st.prefixes[i+1:]...)
				} else {
					st.prefixes[i] = p
				}
				return reply(&ip6_nd.SwInterfaceIP6ndRaPrefixReply{})
			}
		}
		if r.IsNo {
			return reply(&ip6_nd.SwInterfaceIP6ndRaPrefixReply{Retval: RetvalNoSuchEntry})
		}
		st.prefixes = append(st.prefixes, p)
		return reply(&ip6_nd.SwInterfaceIP6ndRaPrefixReply{})
	})
	v.On("sw_interface_ip6nd_ra_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_nd.SwInterfaceIP6ndRaDump)
		defer lock()()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			if uint32(r.SwIfIndex) != ^uint32(0) && uint32(r.SwIfIndex) != idx {
				continue
			}
			st := m.raLocked(v, idx)
			if st == nil {
				continue
			}
			det := st.det
			det.Prefixes = append([]ip6_nd.IP6ndRaPrefix(nil), st.prefixes...)
			det.NPrefixes = uint32(len(det.Prefixes)) //nolint:gosec // G115: model sizes are small
			out = append(out, &det)
		}
		return out, nil
	})

	// ---- ip6_nd: proxy ND (opt-in descriptor)
	v.On("ip6nd_proxy_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_nd.IP6ndProxyEnableDisable)
		defer lock()()
		m.proxyNdOn[uint32(r.SwIfIndex)] = r.IsEnable
		return reply(&ip6_nd.IP6ndProxyEnableDisableReply{})
	})
	v.On("ip6nd_proxy_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_nd.IP6ndProxyAddDel)
		defer lock()()
		k := nbrKey{uint32(r.SwIfIndex), netip.AddrFrom16(r.IP)}
		if r.IsAdd {
			m.proxyNd[k] = true
		} else {
			delete(m.proxyNd, k)
		}
		return reply(&ip6_nd.IP6ndProxyAddDelReply{})
	})
	v.On("ip6nd_proxy_dump", func(api.Message) ([]api.Message, error) {
		defer lock()()
		var out []api.Message
		for k := range m.proxyNd {
			if exists(k.idx) {
				out = append(out, &ip6_nd.IP6ndProxyDetails{SwIfIndex: interface_types.InterfaceIndex(k.idx), IP: k.ip.As16()})
			}
		}
		return out, nil
	})

	// ---- ip6_dad (VPP-wide)
	v.On("ip6_dad_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip6_dad.IP6DadEnableDisable)
		defer lock()()
		m.dad = ip6_dad.IP6DadDetails{Enabled: r.Enable, DadTransmits: r.DadTransmits, DadRetransmitDelay: r.DadRetransmitDelay}
		return reply(&ip6_dad.IP6DadEnableDisableReply{})
	})
	v.On("ip6_dad_dump", func(api.Message) ([]api.Message, error) {
		defer lock()()
		d := m.dad
		return reply(&d)
	})

	// ---- arp: proxy ARP
	v.On("proxy_arp_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*arp.ProxyArpAddDel)
		defer lock()()
		for i, p := range m.ranges {
			if p == r.Proxy {
				if !r.IsAdd {
					m.ranges = append(m.ranges[:i], m.ranges[i+1:]...)
				}
				return reply(&arp.ProxyArpAddDelReply{})
			}
		}
		if !r.IsAdd {
			return reply(&arp.ProxyArpAddDelReply{Retval: RetvalNoSuchEntry})
		}
		if _, ok := v.Tables[tableKey{r.Proxy.TableID, false}]; !ok {
			return reply(&arp.ProxyArpAddDelReply{Retval: RetvalNoSuchFib})
		}
		m.ranges = append(m.ranges, r.Proxy)
		return reply(&arp.ProxyArpAddDelReply{})
	})
	v.On("proxy_arp_dump", func(api.Message) ([]api.Message, error) {
		defer lock()()
		out := make([]api.Message, 0, len(m.ranges))
		for _, p := range m.ranges {
			out = append(out, &arp.ProxyArpDetails{Proxy: p})
		}
		return out, nil
	})
	v.On("proxy_arp_intfc_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*arp.ProxyArpIntfcEnableDisable)
		defer lock()()
		if !exists(uint32(r.SwIfIndex)) {
			return reply(&arp.ProxyArpIntfcEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.Enable {
			m.proxyArp[uint32(r.SwIfIndex)] = true
		} else {
			delete(m.proxyArp, uint32(r.SwIfIndex))
		}
		return reply(&arp.ProxyArpIntfcEnableDisableReply{})
	})
	v.On("proxy_arp_intfc_dump", func(api.Message) ([]api.Message, error) {
		defer lock()()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			if m.proxyArp[idx] {
				out = append(out, &arp.ProxyArpIntfcDetails{SwIfIndex: idx})
			}
		}
		return out, nil
	})
}

func (v *VPP) indexByNameLocked(name string) (uint32, bool) {
	for idx, i := range v.Ifaces {
		if i.Name == name {
			return idx, true
		}
	}
	return 0, false
}

func toAddress(a netip.Addr) ip_types.Address {
	if a.Is4() {
		return ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(a.As4())}
	}
	return ip_types.Address{Af: ip_types.ADDRESS_IP6, Un: ip_types.AddressUnionIP6(a.As16())}
}

// Learn adds (or refreshes) a learned entry on the named interface, as the data plane would after ARP/ND, and emits an
// ip_neighbor_event_v2 when the interface is subscribed. False when the interface does not exist.
func (m *NeighborsRa) Learn(v *VPP, ifName, ip, mac string, age float64) bool {
	unlock := func() func() { v.mu.Lock(); m.mu.Lock(); return func() { m.mu.Unlock(); v.mu.Unlock() } }()
	idx, ok := v.indexByNameLocked(ifName)
	if !ok {
		unlock()
		return false
	}
	a := netip.MustParseAddr(ip)
	hw, err := ethernet_types.ParseMacAddress(mac)
	if err != nil {
		unlock()
		return false
	}
	k := nbrKey{idx, a}
	nb := ip_neighbor.IPNeighbor{SwIfIndex: interface_types.InterfaceIndex(idx), MacAddress: hw, IPAddress: toAddress(a)}
	m.nbrs[k] = nb
	m.age[k] = age
	pid, watched := m.watch[idx]
	unlock()
	if watched {
		v.Emit(&ip_neighbor.IPNeighborEventV2{PID: pid, Flags: ip_neighbor.IP_NEIGHBOR_API_EVENT_FLAG_ADDED, Neighbor: nb})
	}
	return true
}

// Forget removes an entry (ageing) and emits the REMOVED event to a subscribed interface.
func (m *NeighborsRa) Forget(v *VPP, ifName, ip string) bool {
	unlock := func() func() { v.mu.Lock(); m.mu.Lock(); return func() { m.mu.Unlock(); v.mu.Unlock() } }()
	idx, ok := v.indexByNameLocked(ifName)
	if !ok {
		unlock()
		return false
	}
	k := nbrKey{idx, netip.MustParseAddr(ip)}
	nb, had := m.nbrs[k]
	delete(m.nbrs, k)
	delete(m.age, k)
	pid, watched := m.watch[idx]
	unlock()
	if had && watched {
		v.Emit(&ip_neighbor.IPNeighborEventV2{PID: pid, Flags: ip_neighbor.IP_NEIGHBOR_API_EVENT_FLAG_REMOVED, Neighbor: nb})
	}
	return had
}

// Neighbors lists the entries of the named interface as "ip mac static|dynamic", sorted.
func (m *NeighborsRa) Neighbors(v *VPP, ifName string) []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := v.indexByNameLocked(ifName)
	if !ok {
		return nil
	}
	var out []string
	for k, n := range m.nbrs {
		if k.idx != idx {
			continue
		}
		state := "dynamic"
		if n.Flags&ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC != 0 {
			state = "static"
		}
		out = append(out, k.ip.String()+" "+strings.ToLower(n.MacAddress.String())+" "+state)
	}
	sort.Strings(out)
	return out
}

// Watched returns the subscribed sw_if_indexes, sorted.
func (m *NeighborsRa) Watched() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]uint32, 0, len(m.watch))
	for idx := range m.watch {
		out = append(out, idx)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AllCalls returns AllInterfaceCalls under the lock.
func (m *NeighborsRa) AllCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.AllInterfaceCalls
}

// RaSuppressed reports the RA state of the named interface: ok=false when IPv6 is not enabled on it.
func (m *NeighborsRa) RaSuppressed(v *VPP, ifName string) (suppressed bool, prefixes int, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, found := v.indexByNameLocked(ifName)
	if !found {
		return false, 0, false
	}
	st := m.raLocked(v, idx)
	if st == nil {
		return false, 0, false
	}
	return !st.det.SendRadv, len(st.prefixes), true
}

// ProxyArpOn reports whether proxy ARP is enabled on the named interface.
func (m *NeighborsRa) ProxyArpOn(v *VPP, ifName string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := v.indexByNameLocked(ifName)
	return ok && m.proxyArp[idx]
}

// Ranges returns the proxy-ARP ranges as "table low-high", sorted.
func (m *NeighborsRa) Ranges() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, r := range m.ranges {
		out = append(out, netip.AddrFrom4(r.Low).String()+"-"+netip.AddrFrom4(r.Hi).String()+" t"+itoa(r.TableID))
	}
	sort.Strings(out)
	return out
}
