package coretest

// F-bridge-l2 extension of the model (wave-A-hotspots A6): bridge domains, members, L2 cross-connects,
// the L2 FIB, per-interface L2 feature flags, VLAN tag rewrite, L3 cross-connects, the mactime device
// table and its feature enable (feature_is_enabled), so the agent's projection, Retrieve and restart
// paths run in unit tests with the real DF-1 l2/l3xc and F-bridge-l2 mactime descriptors. Behaviour
// follows VPP 26.06 as DF-1's l2 tests model it: joining as BVI installs a static BVI FIB entry for the
// interface MAC, leaving L2 mode clears the output tag rewrite and the feature bitmap, mactime adds on
// an existing MAC append ranges, the mactime feature stacks on every enable, and feature_is_enabled
// answers true for an index the device-input arc never reached (VPP's int→bool cast).

import (
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	l3xcapi "ngfw/agent/binapi/l3xc"
	mactimeapi "ngfw/agent/binapi/mactime"
)

type vtrState struct{ op, push, tag1, tag2 uint32 }

type l3xcKey struct {
	idx uint32
	v6  bool
}

type fibKey struct {
	bd  uint32
	mac [6]uint8
}

// l2Model is the F-bridge-l2 state of one VPP model (guarded by VPP.mu).
type l2Model struct {
	bds     map[uint32]*l2api.BridgeDomainDetails
	member  map[uint32]uint32 // sw_if_index → bd id
	shg     map[uint32]uint8
	xc      map[uint32]uint32 // rx → tx
	fib     map[fibKey]*l2api.L2FibTableDetails
	feat    map[uint32]l2api.L2IntfFeatFlags
	vtr     map[uint32]vtrState
	l3xc    map[l3xcKey]l3xcapi.L3xc
	devices []*mactimeapi.MactimeDetails
	mtCount map[uint32]int
	reached map[uint32]bool
}

var (
	l2ModelsMu sync.Mutex
	l2Models   = map[*VPP]*l2Model{}
)

func (v *VPP) l2() *l2Model {
	l2ModelsMu.Lock()
	defer l2ModelsMu.Unlock()
	m, ok := l2Models[v]
	if !ok {
		m = &l2Model{bds: map[uint32]*l2api.BridgeDomainDetails{}, member: map[uint32]uint32{}, shg: map[uint32]uint8{},
			xc: map[uint32]uint32{}, fib: map[fibKey]*l2api.L2FibTableDetails{}, feat: map[uint32]l2api.L2IntfFeatFlags{},
			vtr: map[uint32]vtrState{}, l3xc: map[l3xcKey]l3xcapi.L3xc{}, mtCount: map[uint32]int{}, reached: map[uint32]bool{}}
		l2Models[v] = m
	}
	return m
}

const noIndex = ^interface_types.InterfaceIndex(0)

func joinFeat(bvi bool) l2api.L2IntfFeatFlags {
	f := l2api.L2_INTF_FEAT_LEARN | l2api.L2_INTF_FEAT_FWD | l2api.L2_INTF_FEAT_FLOOD | l2api.L2_INTF_FEAT_UU_FLOOD |
		l2api.L2_INTF_FEAT_ARP_TERM | l2api.L2_INTF_FEAT_ARP_UFWD
	if bvi {
		f &^= l2api.L2_INTF_FEAT_LEARN
	}
	return f
}

// leaveL2Locked is what VPP does when idx leaves a bridge or a cross-connect (set_int_l2_mode to L3).
func (v *VPP) leaveL2Locked(m *l2Model, idx uint32) {
	if bd, ok := m.member[idx]; ok {
		if b := m.bds[bd]; b != nil {
			if uint32(b.BviSwIfIndex) == idx {
				b.BviSwIfIndex = noIndex
				for k, e := range m.fib {
					if e.BviMac && uint32(e.SwIfIndex) == idx && k.bd == bd {
						delete(m.fib, k)
					}
				}
			}
			if uint32(b.UuFwdSwIfIndex) == idx {
				b.UuFwdSwIfIndex = noIndex
			}
		}
	}
	delete(m.member, idx)
	delete(m.shg, idx)
	delete(m.feat, idx)
	delete(m.vtr, idx)
}

// gcLocked forgets the L2 state of interfaces that were deleted (VPP removes it with the interface).
func (v *VPP) gcLocked(m *l2Model) {
	for idx := range m.member {
		if _, ok := v.Ifaces[idx]; !ok {
			v.leaveL2Locked(m, idx)
		}
	}
	for rx, tx := range m.xc {
		_, ok1 := v.Ifaces[rx]
		_, ok2 := v.Ifaces[tx]
		if !ok1 || !ok2 {
			delete(m.xc, rx)
		}
	}
	for k := range m.l3xc {
		if _, ok := v.Ifaces[k.idx]; !ok {
			delete(m.l3xc, k)
		}
	}
	for idx := range m.mtCount {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.mtCount, idx)
		}
	}
}

func (v *VPP) installBridgeL2() {
	// sw_interface_dump again, now with the tag-rewrite fields (detailsOf + the model's vtr)
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			d := detailsOf(v.Ifaces[idx])
			if t, ok := m.vtr[idx]; ok {
				d.VtrOp, d.VtrPushDot1q, d.VtrTag1, d.VtrTag2 = t.op, t.push, t.tag1, t.tag2
			}
			out = append(out, d)
		}
		return out, nil
	})
	v.On("bridge_domain_add_del_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainAddDelV2)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		if !r.IsAdd {
			if _, ok := m.bds[r.BdID]; !ok {
				return reply(&l2api.BridgeDomainAddDelV2Reply{Retval: RetvalNoSuchEntry})
			}
			for _, bd := range m.member {
				if bd == r.BdID {
					return reply(&l2api.BridgeDomainAddDelV2Reply{Retval: -119}) // BD_NOT_MODIFIABLE: members left
				}
			}
			delete(m.bds, r.BdID)
			for k := range m.fib {
				if k.bd == r.BdID {
					delete(m.fib, k)
				}
			}
			return reply(&l2api.BridgeDomainAddDelV2Reply{})
		}
		if _, dup := m.bds[r.BdID]; dup {
			return reply(&l2api.BridgeDomainAddDelV2Reply{Retval: -100})
		}
		m.bds[r.BdID] = &l2api.BridgeDomainDetails{BdID: r.BdID, Flood: r.Flood, UuFlood: r.UuFlood, Forward: r.Forward, Learn: r.Learn,
			ArpTerm: r.ArpTerm, ArpUfwd: r.ArpUfwd, MacAge: r.MacAge, BdTag: r.BdTag, BviSwIfIndex: noIndex, UuFwdSwIfIndex: noIndex}
		return reply(&l2api.BridgeDomainAddDelV2Reply{BdID: r.BdID})
	})
	v.On("bridge_domain_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		v.gcLocked(m)
		ids := make([]uint32, 0, len(m.bds))
		for id := range m.bds {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var out []api.Message
		for _, id := range ids {
			if r.BdID != ^uint32(0) && r.BdID != id {
				continue
			}
			bd := *m.bds[id]
			bd.SwIfDetails = nil
			for sw, b := range m.member {
				if b == id {
					bd.SwIfDetails = append(bd.SwIfDetails, l2api.BridgeDomainSwIf{SwIfIndex: interface_types.InterfaceIndex(sw), Shg: m.shg[sw]})
				}
			}
			sort.Slice(bd.SwIfDetails, func(i, j int) bool { return bd.SwIfDetails[i].SwIfIndex < bd.SwIfDetails[j].SwIfIndex })
			bd.NSwIfs = uint32(len(bd.SwIfDetails)) //nolint:gosec // small
			out = append(out, &bd)
		}
		return out, nil
	})
	v.On("bridge_flags", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeFlags)
		v.mu.Lock()
		defer v.mu.Unlock()
		bd, ok := v.l2().bds[r.BdID]
		if !ok {
			return reply(&l2api.BridgeFlagsReply{Retval: RetvalNoSuchEntry})
		}
		set := func(p *bool, bit l2api.BdFlags) {
			if r.Flags&bit != 0 {
				*p = r.IsSet
			}
		}
		set(&bd.Learn, l2api.BRIDGE_API_FLAG_LEARN)
		set(&bd.Forward, l2api.BRIDGE_API_FLAG_FWD)
		set(&bd.Flood, l2api.BRIDGE_API_FLAG_FLOOD)
		set(&bd.UuFlood, l2api.BRIDGE_API_FLAG_UU_FLOOD)
		set(&bd.ArpTerm, l2api.BRIDGE_API_FLAG_ARP_TERM)
		set(&bd.ArpUfwd, l2api.BRIDGE_API_FLAG_ARP_UFWD)
		return reply(&l2api.BridgeFlagsReply{})
	})
	v.On("bridge_domain_set_mac_age", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.BridgeDomainSetMacAge)
		v.mu.Lock()
		defer v.mu.Unlock()
		bd, ok := v.l2().bds[r.BdID]
		if !ok {
			return reply(&l2api.BridgeDomainSetMacAgeReply{Retval: RetvalNoSuchEntry})
		}
		bd.MacAge = r.MacAge
		return reply(&l2api.BridgeDomainSetMacAgeReply{})
	})
	v.On("sw_interface_set_l2_bridge", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.SwInterfaceSetL2Bridge)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		sw := uint32(r.RxSwIfIndex)
		i, ok := v.Ifaces[sw]
		if !ok {
			return reply(&l2api.SwInterfaceSetL2BridgeReply{Retval: RetvalInvalidSwIfIndex})
		}
		if !r.Enable {
			v.leaveL2Locked(m, sw)
			return reply(&l2api.SwInterfaceSetL2BridgeReply{})
		}
		bd, ok := m.bds[r.BdID]
		if !ok {
			return reply(&l2api.SwInterfaceSetL2BridgeReply{Retval: RetvalNoSuchEntry})
		}
		if r.PortType == l2api.L2_API_PORT_TYPE_BVI && bd.BviSwIfIndex != noIndex && uint32(bd.BviSwIfIndex) != sw {
			return reply(&l2api.SwInterfaceSetL2BridgeReply{Retval: -120}) // BD_ALREADY_HAS_BVI
		}
		delete(m.xc, sw)
		m.member[sw], m.shg[sw] = r.BdID, r.Shg
		m.feat[sw] = joinFeat(r.PortType == l2api.L2_API_PORT_TYPE_BVI)
		switch r.PortType {
		case l2api.L2_API_PORT_TYPE_BVI:
			bd.BviSwIfIndex = r.RxSwIfIndex
			m.fib[fibKey{r.BdID, i.L2}] = &l2api.L2FibTableDetails{BdID: r.BdID, Mac: i.L2, SwIfIndex: r.RxSwIfIndex, StaticMac: true, BviMac: true}
		case l2api.L2_API_PORT_TYPE_UU_FWD:
			bd.UuFwdSwIfIndex = r.RxSwIfIndex
		}
		return reply(&l2api.SwInterfaceSetL2BridgeReply{})
	})
	v.On("sw_interface_set_l2_xconnect", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.SwInterfaceSetL2Xconnect)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		rx := uint32(r.RxSwIfIndex)
		if _, ok := v.Ifaces[rx]; !ok {
			return reply(&l2api.SwInterfaceSetL2XconnectReply{Retval: RetvalInvalidSwIfIndex})
		}
		if !r.Enable {
			delete(m.xc, rx)
			delete(m.vtr, rx)
			return reply(&l2api.SwInterfaceSetL2XconnectReply{})
		}
		if _, ok := v.Ifaces[uint32(r.TxSwIfIndex)]; !ok {
			return reply(&l2api.SwInterfaceSetL2XconnectReply{Retval: RetvalInvalidSwIfIndex})
		}
		if _, ok := m.member[rx]; ok {
			v.leaveL2Locked(m, rx)
		}
		m.xc[rx] = uint32(r.TxSwIfIndex)
		return reply(&l2api.SwInterfaceSetL2XconnectReply{})
	})
	v.On("l2_xconnect_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		v.gcLocked(m)
		rxs := make([]uint32, 0, len(m.xc))
		for rx := range m.xc {
			rxs = append(rxs, rx)
		}
		sort.Slice(rxs, func(i, j int) bool { return rxs[i] < rxs[j] })
		var out []api.Message
		for _, rx := range rxs {
			out = append(out, &l2api.L2XconnectDetails{RxSwIfIndex: interface_types.InterfaceIndex(rx), TxSwIfIndex: interface_types.InterfaceIndex(m.xc[rx])})
		}
		return out, nil
	})
	v.On("l2fib_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2fibAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		if _, ok := m.bds[r.BdID]; !ok {
			return reply(&l2api.L2fibAddDelReply{Retval: RetvalNoSuchEntry})
		}
		k := fibKey{r.BdID, r.Mac}
		if r.IsAdd {
			m.fib[k] = &l2api.L2FibTableDetails{BdID: r.BdID, Mac: r.Mac, SwIfIndex: r.SwIfIndex, StaticMac: r.StaticMac || r.FilterMac, FilterMac: r.FilterMac, BviMac: r.BviMac}
		} else {
			delete(m.fib, k)
		}
		return reply(&l2api.L2fibAddDelReply{})
	})
	v.On("l2_fib_table_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2FibTableDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		keys := make([]fibKey, 0, len(m.fib))
		for k := range m.fib {
			if r.BdID == ^uint32(0) || r.BdID == k.bd {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].bd != keys[j].bd {
				return keys[i].bd < keys[j].bd
			}
			return string(keys[i].mac[:]) < string(keys[j].mac[:])
		})
		out := make([]api.Message, 0, len(keys))
		for _, k := range keys {
			e := *m.fib[k]
			out = append(out, &e)
		}
		return out, nil
	})
	v.On("l2_interface_feat_flags_get", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceFeatFlagsGet)
		v.mu.Lock()
		defer v.mu.Unlock()
		return reply(&l2api.L2InterfaceFeatFlagsGetReply{Flags: v.l2().feat[uint32(r.SwIfIndex)]})
	})
	v.On("l2_interface_feat_flags_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceFeatFlagsSet)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		if r.IsSet {
			m.feat[uint32(r.SwIfIndex)] |= r.Flags
		} else {
			m.feat[uint32(r.SwIfIndex)] &^= r.Flags
		}
		return reply(&l2api.L2InterfaceFeatFlagsSetReply{})
	})
	v.On("l2_interface_vlan_tag_rewrite", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.L2InterfaceVlanTagRewrite)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		if _, ok := v.Ifaces[uint32(r.SwIfIndex)]; !ok {
			return reply(&l2api.L2InterfaceVlanTagRewriteReply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.VtrOp == 0 {
			delete(m.vtr, uint32(r.SwIfIndex))
		} else {
			// VPP reports push_dot1q only for operations that push a tag (l2vtr_get)
			push := r.PushDot1q
			if r.VtrOp == 3 || r.VtrOp == 4 {
				push = 0
			}
			m.vtr[uint32(r.SwIfIndex)] = vtrState{op: r.VtrOp, push: push, tag1: r.Tag1, tag2: r.Tag2}
		}
		return reply(&l2api.L2InterfaceVlanTagRewriteReply{})
	})
	v.On("l3xc_update", func(req api.Message) ([]api.Message, error) {
		r := req.(*l3xcapi.L3xcUpdate)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := v.Ifaces[uint32(r.L3xc.SwIfIndex)]; !ok {
			return reply(&l3xcapi.L3xcUpdateReply{Retval: RetvalInvalidSwIfIndex})
		}
		x := r.L3xc
		for i := range x.Paths {
			if x.Paths[i].Weight == 0 {
				x.Paths[i].Weight = 1 // fib_api.c stores 0 as 1
			}
		}
		v.l2().l3xc[l3xcKey{uint32(x.SwIfIndex), x.IsIP6}] = x
		return reply(&l3xcapi.L3xcUpdateReply{})
	})
	v.On("l3xc_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*l3xcapi.L3xcDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		k := l3xcKey{uint32(r.SwIfIndex), r.IsIP6}
		if _, ok := v.l2().l3xc[k]; !ok {
			return reply(&l3xcapi.L3xcDelReply{Retval: RetvalNoSuchEntry})
		}
		delete(v.l2().l3xc, k)
		return reply(&l3xcapi.L3xcDelReply{})
	})
	v.On("l3xc_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		v.gcLocked(m)
		keys := make([]l3xcKey, 0, len(m.l3xc))
		for k := range m.l3xc {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].idx != keys[j].idx {
				return keys[i].idx < keys[j].idx
			}
			return !keys[i].v6
		})
		var out []api.Message
		for _, k := range keys {
			out = append(out, &l3xcapi.L3xcDetails{L3xc: m.l3xc[k]})
		}
		return out, nil
	})
	v.On("mactime_add_del_range", func(req api.Message) ([]api.Message, error) {
		r := req.(*mactimeapi.MactimeAddDelRange)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		for i, d := range m.devices {
			if d.MacAddress != r.MacAddress {
				continue
			}
			if !r.IsAdd {
				m.devices = append(m.devices[:i], m.devices[i+1:]...)
				return reply(&mactimeapi.MactimeAddDelRangeReply{})
			}
			for _, x := range r.Ranges { // VPP appends to an existing device
				d.Ranges = append(d.Ranges, mactimeapi.MactimeTimeRange{Start: x.Start, End: x.End})
			}
			d.Nranges = uint32(len(d.Ranges)) //nolint:gosec // small
			return reply(&mactimeapi.MactimeAddDelRangeReply{})
		}
		if !r.IsAdd {
			return reply(&mactimeapi.MactimeAddDelRangeReply{Retval: RetvalNoSuchEntry})
		}
		d := &mactimeapi.MactimeDetails{PoolIndex: uint32(len(m.devices)), MacAddress: r.MacAddress, DeviceName: r.DeviceName} //nolint:gosec // small
		for _, x := range r.Ranges {
			d.Ranges = append(d.Ranges, mactimeapi.MactimeTimeRange{Start: x.Start, End: x.End})
		}
		d.Nranges = uint32(len(d.Ranges)) //nolint:gosec // small
		switch {
		case len(r.Ranges) > 0 && r.Drop:
			d.Flags = 1 << 2
		case len(r.Ranges) > 0:
			d.Flags = 1 << 3
		case r.Drop:
			d.Flags = 1 << 0
		default:
			d.Flags = 1 << 1
		}
		m.devices = append(m.devices, d)
		return reply(&mactimeapi.MactimeAddDelRangeReply{})
	})
	v.On("mactime_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, d := range v.l2().devices {
			c := *d
			c.Ranges = append([]mactimeapi.MactimeTimeRange(nil), d.Ranges...)
			out = append(out, &c)
		}
		return append(out, &mactimeapi.MactimeDumpReply{TableEpoch: 1}), nil
	})
	v.On("mactime_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*mactimeapi.MactimeEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		idx := uint32(r.SwIfIndex)
		i, ok := v.Ifaces[idx]
		if !ok || i.IsSub {
			return reply(&mactimeapi.MactimeEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.reached[idx] = true
		if r.EnableDisable {
			m.mtCount[idx]++
		} else if m.mtCount[idx] > 0 {
			m.mtCount[idx]--
		}
		return reply(&mactimeapi.MactimeEnableDisableReply{})
	})
	v.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.l2()
		v.gcLocked(m)
		idx := uint32(r.SwIfIndex)
		if _, ok := v.Ifaces[idx]; !ok {
			return reply(&feature.FeatureIsEnabledReply{Retval: RetvalInvalidSwIfIndex})
		}
		return reply(&feature.FeatureIsEnabledReply{IsEnabled: m.mtCount[idx] > 0 || !m.reached[idx]})
	})
}

// BridgeMembers returns the bridge-domain id of every member by interface name (tests).
func (v *VPP) BridgeMembers() map[string]uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.l2()
	v.gcLocked(m)
	out := map[string]uint32{}
	for idx, bd := range m.member {
		out[v.Ifaces[idx].Name] = bd
	}
	return out
}

// BridgeDomainIDs returns the ids of every bridge domain in the model (tests).
func (v *VPP) BridgeDomainIDs() []uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []uint32
	for id := range v.l2().bds {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// MactimeCount is how often the mactime feature is stacked on interface name (tests).
func (v *VPP) MactimeCount(name string) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	for idx, i := range v.Ifaces {
		if i.Name == name {
			return v.l2().mtCount[idx]
		}
	}
	return 0
}

// DropBridgeL2 deletes every bridge domain, cross-connect, l3xc, tag rewrite and mactime device and
// enable behind the agent's back (restart-simulation tests, D-095c: dependents first).
func (v *VPP) DropBridgeL2() {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.l2()
	for idx := range m.member {
		v.leaveL2Locked(m, idx)
	}
	for rx := range m.xc {
		delete(m.vtr, rx)
	}
	m.xc = map[uint32]uint32{}
	m.l3xc = map[l3xcKey]l3xcapi.L3xc{}
	m.mtCount = map[uint32]int{}
	m.devices = nil
	m.bds = map[uint32]*l2api.BridgeDomainDetails{}
	m.fib = map[fibKey]*l2api.L2FibTableDetails{}
}
