package coretest

// F-vrf-static-ecmp additions to the model (wave-A-hotspots A6: a new file, the existing ones stay read-only):
// the svs plugin (svs_route_add_del, svs_enable_disable, svs_dump, svs_table_add_del), fib_source_dump and the ping
// plugin (want_ping_finished_events → one ping_finished_event). Install them with InstallVrfStaticEcmp.
//
// Duplicate-add behaviour is modelled as VPP has it (D-076): a second svs_route_add_del add of a prefix only bumps a
// reference count and keeps the first selected table; a second svs_enable_disable enable stacks the feature (counted
// in SvsEnables). svs entries are FIB entries of the "svs" source (id SvsSource) in the model's Internal table, so
// ip_route_v2_dump reports them like VPP does (prefix + source, no selected table).

import (
	"net/netip"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ping"
	"ngfw/agent/binapi/svs"
)

// SvsSource is the FIB source id the svs plugin got on the host VPP (fib_source_dump, 2026-09-24).
const SvsSource uint8 = 39

// hostSources is fib_source_dump as the host VPP 26.06 answers it (ids 0–21 fixed, plugin sources after).
var hostSources = []string{
	"invalid", "special", "classify", "proxy", "interface", "SR", "BIER", "6RD", "API", "CLI", "LISP", "MAP", "DHCP",
	"IPv6-proxy-nd", "IPv6-nd", "adjacency", "mpls", "attached_export", "recursive-resolution", "urpf-exempt",
	"default-route", "interpose", "path-mtu", "cnat", "det44-hi", "det44-low", "dslite-hi", "ila", "lb", "lcp-rt",
	"lcp-rt-dynamic", "nat44-ei-low", "nat44-ei-hi", "nat64-hi", "nat64-low", "nat66-hi", "nat-low", "nat-hi", "pppoe",
	"svs",
}

// SvsEntry is one modelled svs route.
type SvsEntry struct {
	SourceTable uint32
	Refs        int
}

type svsIfKey struct {
	swIfIndex uint32
	v6        bool
}

// SvsState is the model's svs plugin state (guarded by VPP.mu).
type SvsState struct {
	Routes map[routeKey]*SvsEntry
	// Enabled maps (sw_if_index, family) to the svs table; SvsEnables counts enables per key (stacking).
	Enabled    map[svsIfKey]uint32
	SvsEnables map[svsIfKey]int
	// PingResult answers want_ping_finished_events (nil: every request answered).
	PingResult func(addr netip.Addr, repeat uint32) (requests, replies uint32)
	Pings      []ping.WantPingFinishedEvents
}

var svsStates = map[*VPP]*SvsState{}

// Svs returns the svs/ping model state of v (after InstallVrfStaticEcmp).
func (v *VPP) Svs() *SvsState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return svsStates[v]
}

// SvsRoute returns the modelled svs route of (table, prefix).
func (v *VPP) SvsRoute(table uint32, prefix string) (SvsEntry, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	st := svsStates[v]
	if st == nil {
		return SvsEntry{}, false
	}
	e, ok := st.Routes[routeKey{table, netip.MustParsePrefix(prefix).Masked().String()}]
	if !ok {
		return SvsEntry{}, false
	}
	return *e, true
}

// InstallVrfStaticEcmp adds the svs, fib_source_dump and ping handlers to v.
func (v *VPP) InstallVrfStaticEcmp() *VPP {
	v.mu.Lock()
	st := &SvsState{Routes: map[routeKey]*SvsEntry{}, Enabled: map[svsIfKey]uint32{}, SvsEnables: map[svsIfKey]int{}}
	svsStates[v] = st
	v.mu.Unlock()
	v.On("fib_source_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(hostSources))
		for i, n := range hostSources {
			out = append(out, &fib.FibSourceDetails{Src: fib.FibSource{ID: uint8(i), Name: n}}) //nolint:gosec // < 256 entries
		}
		return out, nil
	})
	v.On("svs_table_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*svs.SvsTableAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		k := tableKey{req.TableID, req.Af == ip_types.ADDRESS_IP6}
		if req.IsAdd {
			if _, ok := v.Tables[k]; !ok {
				v.Tables[k] = map[bool]string{false: "ipv4-VRF:", true: "ipv6-VRF:"}[k.v6] + strconv.FormatUint(uint64(req.TableID), 10)
			}
			return reply(&svs.SvsTableAddDelReply{})
		}
		if _, ok := v.Tables[k]; !ok {
			return reply(&svs.SvsTableAddDelReply{Retval: RetvalNoSuchFib})
		}
		return reply(&svs.SvsTableAddDelReply{})
	})
	v.On("svs_route_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*svs.SvsRouteAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		p := netip.MustParsePrefix(req.Prefix.String()).Masked()
		if _, ok := v.Tables[tableKey{req.TableID, p.Addr().Is6()}]; !ok {
			return reply(&svs.SvsRouteAddDelReply{Retval: RetvalNoSuchFib})
		}
		rk := routeKey{req.TableID, p.String()}
		e := st.Routes[rk]
		if !req.IsAdd {
			if e != nil {
				if e.Refs--; e.Refs <= 0 {
					delete(st.Routes, rk)
					delete(v.Internal, rk)
				}
			}
			return reply(&svs.SvsRouteAddDelReply{})
		}
		if _, ok := v.Tables[tableKey{req.SourceTableID, p.Addr().Is6()}]; !ok {
			return reply(&svs.SvsRouteAddDelReply{Retval: RetvalNoSuchFib})
		}
		if e != nil {
			e.Refs++ // VPP: the source is added once; a repeated add keeps the first DPO
			return reply(&svs.SvsRouteAddDelReply{})
		}
		st.Routes[rk] = &SvsEntry{SourceTable: req.SourceTableID, Refs: 1}
		v.Internal[rk] = SvsSource
		return reply(&svs.SvsRouteAddDelReply{})
	})
	v.On("svs_enable_disable", func(m api.Message) ([]api.Message, error) {
		req := m.(*svs.SvsEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		v6 := req.Af == ip_types.ADDRESS_IP6
		if _, ok := v.Tables[tableKey{req.TableID, v6}]; !ok {
			return reply(&svs.SvsEnableDisableReply{Retval: RetvalNoSuchFib})
		}
		if _, ok := v.Ifaces[uint32(req.SwIfIndex)]; !ok {
			return reply(&svs.SvsEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		k := svsIfKey{uint32(req.SwIfIndex), v6}
		def := routeKey{req.TableID, map[bool]string{false: "0.0.0.0/0", true: "::/0"}[v6]}
		if req.IsEnable {
			st.Enabled[k] = req.TableID
			st.SvsEnables[k]++
			v.Internal[def] = SvsSource
			return reply(&svs.SvsEnableDisableReply{})
		}
		delete(st.Enabled, k)
		delete(st.SvsEnables, k)
		delete(v.Internal, def)
		return reply(&svs.SvsEnableDisableReply{})
	})
	v.On("svs_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]svsIfKey, 0, len(st.Enabled))
		for k := range st.Enabled {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].v6 != keys[j].v6 {
				return !keys[i].v6
			}
			return keys[i].swIfIndex < keys[j].swIfIndex
		})
		var out []api.Message
		for _, k := range keys {
			out = append(out, &svs.SvsDetails{TableID: st.Enabled[k], SwIfIndex: interface_types.InterfaceIndex(k.swIfIndex), Af: afOf(k.v6)})
		}
		return out, nil
	})
	v.On("want_ping_finished_events", func(m api.Message) ([]api.Message, error) {
		req := m.(*ping.WantPingFinishedEvents)
		v.mu.Lock()
		st.Pings = append(st.Pings, *req)
		fn := st.PingResult
		v.mu.Unlock()
		a, _ := netip.AddrFromSlice(addrBytes(req.Address))
		reqs, reps := req.Repeat, req.Repeat
		if fn != nil {
			reqs, reps = fn(a, req.Repeat)
		}
		// VPP answers the request first and sends the event when the pings are done
		go v.Emit(&ping.PingFinishedEvent{RequestCount: reqs, ReplyCount: reps})
		return reply(&ping.WantPingFinishedEventsReply{})
	})
	return v
}

func addrBytes(a ip_types.Address) []byte {
	if a.Af == ip_types.ADDRESS_IP6 {
		b := a.Un.GetIP6()
		return b[:]
	}
	b := a.Un.GetIP4()
	return b[:]
}
