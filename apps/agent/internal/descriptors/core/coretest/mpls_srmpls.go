package coretest

// F-mpls-srmpls extension of the model (wave-A-hotspots A6: own file): core VPP MPLS and the
// srmpls plugin, as the host VPP 26.06 behaves, so the agent's projection, Retrieve, restart and
// MplsState paths run in unit tests with DF-7's and DF-6's real descriptors:
//
//   - MPLS tables (mpls_table_add_del / mpls_table_dump; a new table gets VPP's reserved special
//     entries); no table 0 exists until someone creates it (the shared host has none, D-071);
//   - MPLS on an interface: a u8 counter per interface (sw_interface_set_mpls_enable, NO_SUCH_FIB
//     without table 0; mpls_interface_dump);
//   - label routes per table (mpls_route_add_del replaces the path set; NO_SUCH_FIB for a missing
//     table, NO_SUCH_ENTRY for a missing delete; mpls_route_dump reports non-EOS payload as MPLS);
//   - label ↔ IP bindings (mpls_ip_bind_unbind: both tables must exist; the bound label appears in
//     table 0 as an EOS entry);
//   - MPLS tunnels: an interface of device class "MPLS tunnel device" named mpls-tunnel<N>, deleted
//     when a delete carries all its paths (mpls_tunnel_add_del / mpls_tunnel_dump);
//   - SR-MPLS policies (sr_mpls_policy_add/mod/del, the BSID as an EOS entry of table 0 with
//     recursive MPLS paths; VPP needs table 0) and steering by BSID (sr_mpls_steering_add_del: an IP
//     route with a recursive MPLS path, reported by the base model's ip_route_v2_dump).
//
// installMplsSrmpls runs from New() (one line there until TD-23's RegisterExtension is on main; then
// it becomes RegisterExtension("mpls-srmpls", …) in this file). It hooks only mpls_* and sr_mpls_*
// messages, so it never collides with another feature's extension; the FIB source table that SR
// steering probes (fib_source_dump) is added per test by UseSRFibSource.

import (
	"fmt"
	"net/netip"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/mpls"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	"ngfw/agent/binapi/sr_types"
)

// MplsTunnelDevType is VPP 26.06's device class of MPLS tunnel interfaces (mpls_tunnel.c).
const MplsTunnelDevType = "MPLS tunnel device"

// SRFibSource is the id the FIB source "SR" has in UseSRFibSource's table.
const SRFibSource = 23

// MplsLabelKey is one MPLS FIB entry key: table, local label, end of stack.
type MplsLabelKey struct {
	Table uint32
	Label uint32
	EOS   bool
}

// MplsModel is the MPLS state of the fake. Guarded by VPP.mu; read it through the accessors.
type MplsModel struct {
	Tables   map[uint32]string // id → name
	Enabled  map[uint32]uint8  // sw_if_index → enable counter (a u8, like VPP)
	Routes   map[MplsLabelKey]mpls.MplsRoute
	Tunnels  map[uint32]*mpls.MplsTunnel // sw_if_index → tunnel
	Policies map[uint32][]SRSegmentList  // bsid → segment lists
	Binds    map[string]uint32           // "<vrf>|<prefix>" → label
	nextTun  uint32
}

// SRSegmentList is one segment list of a modelled SR-MPLS policy.
type SRSegmentList struct {
	Labels []uint32
	Weight uint32
}

// mplsModels holds each model's MPLS state (*VPP → *MplsModel): the VPP struct is fakevpp.go's.
var mplsModels sync.Map

// mplsLocked returns v's MPLS model (v.mu held for the model's contents).
func (v *VPP) mplsLocked() *MplsModel {
	if m, ok := mplsModels.Load(v); ok {
		return m.(*MplsModel)
	}
	m, _ := mplsModels.LoadOrStore(v, &MplsModel{Tables: map[uint32]string{}, Enabled: map[uint32]uint8{}, Routes: map[MplsLabelKey]mpls.MplsRoute{},
		Tunnels: map[uint32]*mpls.MplsTunnel{}, Policies: map[uint32][]SRSegmentList{}, Binds: map[string]uint32{}})
	return m.(*MplsModel)
}

const (
	retvalNoSuchFib        = int32(api.NO_SUCH_FIB)
	retvalNoSuchEntry      = int32(api.NO_SUCH_ENTRY)
	retvalInvalidValue     = int32(api.INVALID_VALUE)
	retvalInvalidSwIfIdx   = int32(api.INVALID_SW_IF_INDEX)
	payloadMPLS            = 2
	noInterface            = ^uint32(0)
	srPolicyNotFoundRetval = -1
)

// specialEntries are the reserved labels VPP installs in every new MPLS table (ip4/ip6 explicit
// null, router alert — mpls_fib.c).
var specialEntries = []MplsLabelKey{{Label: 0, EOS: true}, {Label: 1, EOS: false}, {Label: 2, EOS: true}}

func (m *MplsModel) addTable(id uint32, name string) {
	if _, ok := m.Tables[id]; ok {
		return // VPP only adds a lock; the name stays
	}
	m.Tables[id] = name
	for _, k := range specialEntries {
		k.Table = id
		m.Routes[k] = mpls.MplsRoute{MrTableID: id, MrLabel: k.Label, MrEos: b2u(k.EOS), MrEosProto: 0, MrNPaths: 1,
			MrPaths: []fib_types.FibPath{{SwIfIndex: noInterface, Type: fib_types.FIB_API_PATH_TYPE_LOCAL}}}
	}
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func (v *VPP) installMplsSrmpls() {
	v.On("mpls_table_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*mpls.MplsTableAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if r.MtIsAdd {
			m.addTable(r.MtTable.MtTableID, r.MtTable.MtName)
		} else {
			delete(m.Tables, r.MtTable.MtTableID)
			for k := range m.Routes {
				if k.Table == r.MtTable.MtTableID {
					delete(m.Routes, k)
				}
			}
		}
		return reply(&mpls.MplsTableAddDelReply{})
	})
	v.On("mpls_table_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		ids := make([]uint32, 0, len(m.Tables))
		for id := range m.Tables {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		out := make([]api.Message, 0, len(ids))
		for _, id := range ids {
			out = append(out, &mpls.MplsTableDetails{MtTable: mpls.MplsTable{MtTableID: id, MtName: m.Tables[id]}})
		}
		return out, nil
	})
	v.On("sw_interface_set_mpls_enable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*mpls.SwInterfaceSetMplsEnable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if _, ok := v.Ifaces[uint32(r.SwIfIndex)]; !ok {
			return reply(&mpls.SwInterfaceSetMplsEnableReply{Retval: retvalInvalidSwIfIdx})
		}
		if _, ok := m.Tables[0]; !ok {
			return reply(&mpls.SwInterfaceSetMplsEnableReply{Retval: retvalNoSuchFib})
		}
		if r.Enable {
			m.Enabled[uint32(r.SwIfIndex)]++
		} else {
			m.Enabled[uint32(r.SwIfIndex)]-- // wraps at 0, like VPP
		}
		return reply(&mpls.SwInterfaceSetMplsEnableReply{})
	})
	v.On("mpls_interface_dump", func(msg api.Message) ([]api.Message, error) {
		want := uint32(msg.(*mpls.MplsInterfaceDump).SwIfIndex)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		var idx []uint32
		for i, n := range m.Enabled {
			if n > 0 && (want == noInterface || want == i) {
				idx = append(idx, i)
			}
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a] < idx[b] })
		out := make([]api.Message, 0, len(idx))
		for _, i := range idx {
			out = append(out, &mpls.MplsInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(i)})
		}
		return out, nil
	})
	v.On("mpls_route_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*mpls.MplsRouteAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		rt := r.MrRoute
		if _, ok := m.Tables[rt.MrTableID]; !ok {
			return reply(&mpls.MplsRouteAddDelReply{Retval: retvalNoSuchFib})
		}
		k := MplsLabelKey{rt.MrTableID, rt.MrLabel, rt.MrEos != 0}
		if !r.MrIsAdd {
			if _, ok := m.Routes[k]; !ok {
				return reply(&mpls.MplsRouteAddDelReply{Retval: retvalNoSuchEntry})
			}
			delete(m.Routes, k)
			return reply(&mpls.MplsRouteAddDelReply{})
		}
		for _, p := range rt.MrPaths {
			if p.SwIfIndex != noInterface {
				if _, ok := v.Ifaces[p.SwIfIndex]; !ok {
					return reply(&mpls.MplsRouteAddDelReply{Retval: retvalInvalidSwIfIdx})
				}
			}
			if p.TableID != 0 && p.SwIfIndex == noInterface && (p.Proto == fib_types.FIB_API_PATH_NH_PROTO_IP4 || p.Proto == fib_types.FIB_API_PATH_NH_PROTO_IP6) {
				if _, ok := v.Tables[tableKey{p.TableID, p.Proto == fib_types.FIB_API_PATH_NH_PROTO_IP6}]; !ok {
					return reply(&mpls.MplsRouteAddDelReply{Retval: retvalNoSuchFib})
				}
			}
		}
		if rt.MrEos == 0 {
			rt.MrEosProto = payloadMPLS // VPP reports the MPLS payload for non-EOS entries
		}
		rt.MrPaths = append([]fib_types.FibPath(nil), rt.MrPaths...)
		rt.MrNPaths = uint8(len(rt.MrPaths)) //nolint:gosec // ≤ 255 in a test model
		m.Routes[k] = rt
		return reply(&mpls.MplsRouteAddDelReply{})
	})
	v.On("mpls_route_dump", func(msg api.Message) ([]api.Message, error) {
		id := msg.(*mpls.MplsRouteDump).Table.MtTableID
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if _, ok := m.Tables[id]; !ok {
			return nil, nil
		}
		keys := make([]MplsLabelKey, 0)
		for k := range m.Routes {
			if k.Table == id {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(a, b int) bool {
			if keys[a].Label != keys[b].Label {
				return keys[a].Label < keys[b].Label
			}
			return keys[a].EOS && !keys[b].EOS
		})
		out := make([]api.Message, 0, len(keys))
		for _, k := range keys {
			out = append(out, &mpls.MplsRouteDetails{MrRoute: m.Routes[k]})
		}
		return out, nil
	})
	v.On("mpls_ip_bind_unbind", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*mpls.MplsIPBindUnbind)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if _, ok := m.Tables[r.MbMplsTableID]; !ok {
			return reply(&mpls.MplsIPBindUnbindReply{Retval: retvalNoSuchFib})
		}
		p := netip.MustParsePrefix(r.MbPrefix.String()).Masked()
		if _, ok := v.Tables[tableKey{r.MbIPTableID, p.Addr().Is6()}]; !ok {
			return reply(&mpls.MplsIPBindUnbindReply{Retval: retvalNoSuchFib})
		}
		key := fmt.Sprintf("%d|%s", r.MbIPTableID, p)
		if r.MbIsBind {
			if old, ok := m.Binds[key]; ok && old != r.MbLabel {
				delete(m.Routes, MplsLabelKey{0, old, true})
			}
			m.Binds[key] = r.MbLabel
			if _, ok := m.Tables[0]; ok {
				m.Routes[MplsLabelKey{0, r.MbLabel, true}] = mpls.MplsRoute{MrTableID: 0, MrLabel: r.MbLabel, MrEos: 1, MrNPaths: 1,
					MrPaths: []fib_types.FibPath{{SwIfIndex: noInterface, TableID: r.MbIPTableID, Type: fib_types.FIB_API_PATH_TYPE_NORMAL}}}
			}
		} else if old, ok := m.Binds[key]; ok && old == r.MbLabel { // VPP ignores an unbind with another label
			delete(m.Binds, key)
			delete(m.Routes, MplsLabelKey{0, old, true})
		}
		return reply(&mpls.MplsIPBindUnbindReply{})
	})
	v.On("mpls_tunnel_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*mpls.MplsTunnelAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if r.MtIsAdd {
			if uint32(r.MtTunnel.MtSwIfIndex) != noInterface {
				return reply(&mpls.MplsTunnelAddDelReply{Retval: retvalInvalidValue}) // path adds to an existing tunnel: not modelled
			}
			idx := v.next
			v.next++
			t := r.MtTunnel
			t.MtSwIfIndex = interface_types.InterfaceIndex(idx)
			t.MtTunnelIndex = m.nextTun
			t.MtPaths = append([]fib_types.FibPath(nil), t.MtPaths...)
			m.nextTun++
			m.Tunnels[idx] = &t
			v.Ifaces[idx] = &Iface{Index: idx, Name: fmt.Sprintf("mpls-tunnel%d", t.MtTunnelIndex), DevType: MplsTunnelDevType, Addrs: map[string]bool{}}
			return reply(&mpls.MplsTunnelAddDelReply{SwIfIndex: t.MtSwIfIndex, TunnelIndex: t.MtTunnelIndex})
		}
		t, ok := m.Tunnels[uint32(r.MtTunnel.MtSwIfIndex)]
		if !ok {
			return reply(&mpls.MplsTunnelAddDelReply{Retval: retvalInvalidSwIfIdx})
		}
		if len(r.MtTunnel.MtPaths) >= len(t.MtPaths) { // every path removed → the tunnel goes
			delete(m.Tunnels, uint32(t.MtSwIfIndex))
			if i, ok := v.Ifaces[uint32(t.MtSwIfIndex)]; ok {
				v.dropInterfaceLocked(i)
			}
		}
		return reply(&mpls.MplsTunnelAddDelReply{})
	})
	v.On("mpls_tunnel_dump", func(msg api.Message) ([]api.Message, error) {
		want := uint32(msg.(*mpls.MplsTunnelDump).SwIfIndex)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		var idx []uint32
		for i := range m.Tunnels {
			if want == noInterface || want == i {
				idx = append(idx, i)
			}
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a] < idx[b] })
		out := make([]api.Message, 0, len(idx))
		for _, i := range idx {
			out = append(out, &mpls.MplsTunnelDetails{MtTunnel: *m.Tunnels[i]})
		}
		return out, nil
	})
	v.On("sr_mpls_policy_add", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*srmplsapi.SrMplsPolicyAdd)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if _, ok := m.Tables[0]; !ok {
			return reply(&srmplsapi.SrMplsPolicyAddReply{Retval: retvalNoSuchFib})
		}
		if _, ok := m.Policies[r.Bsid]; ok || len(r.Segments) == 0 {
			return reply(&srmplsapi.SrMplsPolicyAddReply{Retval: srPolicyNotFoundRetval})
		}
		m.Policies[r.Bsid] = []SRSegmentList{{Labels: append([]uint32(nil), r.Segments...), Weight: r.Weight}}
		m.syncBSIDLocked(r.Bsid)
		return reply(&srmplsapi.SrMplsPolicyAddReply{})
	})
	v.On("sr_mpls_policy_mod", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*srmplsapi.SrMplsPolicyMod)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		lists, ok := m.Policies[r.Bsid]
		if !ok || r.Operation != sr_types.SR_POLICY_OP_API_ADD {
			return reply(&srmplsapi.SrMplsPolicyModReply{Retval: srPolicyNotFoundRetval})
		}
		m.Policies[r.Bsid] = append(lists, SRSegmentList{Labels: append([]uint32(nil), r.Segments...), Weight: r.Weight})
		m.syncBSIDLocked(r.Bsid)
		return reply(&srmplsapi.SrMplsPolicyModReply{})
	})
	v.On("sr_mpls_policy_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*srmplsapi.SrMplsPolicyDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		if _, ok := m.Policies[r.Bsid]; !ok {
			return reply(&srmplsapi.SrMplsPolicyDelReply{Retval: srPolicyNotFoundRetval})
		}
		delete(m.Policies, r.Bsid)
		delete(m.Routes, MplsLabelKey{0, r.Bsid, true})
		return reply(&srmplsapi.SrMplsPolicyDelReply{})
	})
	v.On("sr_mpls_steering_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*srmplsapi.SrMplsSteeringAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.mplsLocked()
		a := netip.AddrFrom4(r.Prefix.Address.Un.GetIP4())
		if r.Prefix.Address.Af == ip_types.ADDRESS_IP6 {
			a = netip.AddrFrom16(r.Prefix.Address.Un.GetIP6())
		}
		p := netip.PrefixFrom(a, int(r.MaskWidth)).Masked()
		if _, ok := v.Tables[tableKey{r.TableID, a.Is6()}]; !ok {
			return reply(&srmplsapi.SrMplsSteeringAddDelReply{Retval: retvalNoSuchFib})
		}
		rk := routeKey{r.TableID, p.String()}
		if r.IsDel {
			if _, ok := v.Routes[rk]; !ok {
				return reply(&srmplsapi.SrMplsSteeringAddDelReply{Retval: retvalNoSuchEntry})
			}
			delete(v.Routes, rk)
			return reply(&srmplsapi.SrMplsSteeringAddDelReply{})
		}
		if _, ok := m.Policies[r.Bsid]; !ok {
			return reply(&srmplsapi.SrMplsSteeringAddDelReply{Retval: srPolicyNotFoundRetval})
		}
		path := fib_types.FibPath{SwIfIndex: noInterface, Proto: fib_types.FIB_API_PATH_NH_PROTO_MPLS, Type: fib_types.FIB_API_PATH_TYPE_NORMAL, Weight: 1}
		if r.VPNLabel != ^uint32(0) {
			path.NLabels, path.LabelStack[0] = 1, fib_types.FibMplsLabel{Label: r.VPNLabel}
		}
		pfx, _ := ip_types.ParsePrefix(p.String())
		v.Routes[rk] = ip.IPRoute{TableID: r.TableID, Prefix: pfx, NPaths: 1, Paths: []fib_types.FibPath{path}}
		return reply(&srmplsapi.SrMplsSteeringAddDelReply{})
	})
}

// syncBSIDLocked installs a policy's BSID in table 0 with one recursive MPLS path per segment list.
func (m *MplsModel) syncBSIDLocked(bsid uint32) {
	var paths []fib_types.FibPath
	for _, sl := range m.Policies[bsid] {
		paths = append(paths, fib_types.FibPath{SwIfIndex: noInterface, Proto: fib_types.FIB_API_PATH_NH_PROTO_MPLS,
			Type: fib_types.FIB_API_PATH_TYPE_NORMAL, Weight: uint8(sl.Weight)}) //nolint:gosec // 1–255
	}
	m.Routes[MplsLabelKey{0, bsid, true}] = mpls.MplsRoute{MrTableID: 0, MrLabel: bsid, MrEos: 1, MrEosProto: payloadMPLS,
		MrNPaths: uint8(len(paths)), MrPaths: paths} //nolint:gosec // test model
}

// UseSRFibSource answers fib_source_dump with VPP's sources including "SR" (id SRFibSource), which
// SR-MPLS steering probes. Tests call it on their model (it is not part of New(): another feature's
// test model may answer fib_source_dump with its own table).
func (v *VPP) UseSRFibSource() {
	v.On("fib_source_dump", func(api.Message) ([]api.Message, error) {
		names := map[uint8]string{0: "special", 4: "interface", 8: "API", 15: "adjacency", 18: "recursive-resolution", SRFibSource: "SR"}
		ids := make([]int, 0, len(names))
		for id := range names {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		out := make([]api.Message, 0, len(ids))
		for _, id := range ids {
			out = append(out, &fib.FibSourceDetails{Src: fib.FibSource{ID: uint8(id), Name: names[uint8(id)]}}) //nolint:gosec // < 256
		}
		return out, nil
	})
}

// AddMplsTable creates MPLS table id named name behind the agent's back (e.g. the globals owner's
// table 0).
func (v *VPP) AddMplsTable(id uint32, name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.mplsLocked().addTable(id, name)
}

// DeleteMplsTable removes MPLS table id and its entries behind the agent's back.
func (v *VPP) DeleteMplsTable(id uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.mplsLocked()
	delete(m.Tables, id)
	for k := range m.Routes {
		if k.Table == id {
			delete(m.Routes, k)
		}
	}
}

// MplsTableNames returns the modelled MPLS tables (id → name).
func (v *VPP) MplsTableNames() map[uint32]string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[uint32]string{}
	for id, n := range v.mplsLocked().Tables {
		out[id] = n
	}
	return out
}

// MplsRoute returns the modelled entry of (table, label, eos).
func (v *VPP) MplsRoute(table, label uint32, eos bool) (mpls.MplsRoute, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	r, ok := v.mplsLocked().Routes[MplsLabelKey{table, label, eos}]
	return r, ok
}

// AddMplsRoute installs an entry behind the agent's back (another feature's label).
func (v *VPP) AddMplsRoute(r mpls.MplsRoute) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.mplsLocked().Routes[MplsLabelKey{r.MrTableID, r.MrLabel, r.MrEos != 0}] = r
}

// DeleteMplsRoute removes an entry behind the agent's back (simulated loss).
func (v *VPP) DeleteMplsRoute(table, label uint32, eos bool) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.mplsLocked()
	k := MplsLabelKey{table, label, eos}
	_, ok := m.Routes[k]
	delete(m.Routes, k)
	return ok
}

// MplsLabels returns the labels ≥ 16 of MPLS table id as "label/eos|neos", sorted.
func (v *VPP) MplsLabels(id uint32) []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []string
	for k := range v.mplsLocked().Routes {
		if k.Table == id && k.Label >= 16 {
			e := "neos"
			if k.EOS {
				e = "eos"
			}
			out = append(out, fmt.Sprintf("%d/%s", k.Label, e))
		}
	}
	sort.Strings(out)
	return out
}

// MplsEnabled returns the MPLS enable counter of sw_if_index.
func (v *VPP) MplsEnabled(swIfIndex uint32) uint8 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.mplsLocked().Enabled[swIfIndex]
}

// MplsTunnelsByTag returns the modelled tunnels' tags → sw_if_index.
func (v *VPP) MplsTunnelsByTag() map[string]uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[string]uint32{}
	for i, t := range v.mplsLocked().Tunnels {
		out[t.MtTag] = i
	}
	return out
}

// SRPolicies returns the modelled SR-MPLS policies (bsid → segment lists).
func (v *VPP) SRPolicies() map[uint32][]SRSegmentList {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[uint32][]SRSegmentList{}
	for b, l := range v.mplsLocked().Policies {
		out[b] = append([]SRSegmentList(nil), l...)
	}
	return out
}

// DeleteSRPolicy removes a policy and its BSID behind the agent's back (simulated loss).
func (v *VPP) DeleteSRPolicy(bsid uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.mplsLocked()
	_, ok := m.Policies[bsid]
	delete(m.Policies, bsid)
	delete(m.Routes, MplsLabelKey{0, bsid, true})
	return ok
}

// MplsBindings returns the modelled label bindings ("<ip table>|<prefix>" → label).
func (v *VPP) MplsBindings() map[string]uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[string]uint32{}
	for k, l := range v.mplsLocked().Binds {
		out[k] = l
	}
	return out
}
