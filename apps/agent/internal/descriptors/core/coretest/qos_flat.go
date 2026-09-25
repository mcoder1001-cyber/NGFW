package coretest

// F-qos-flat extension of the model: VPP's policer pool and interface policing (policer plugin) and the qos
// record / store / egress-map / mark tables (vnet qos), as VPP 26.06 behaves:
//
//   - policer_add refuses a duplicate name (VALUE_EXIST); policer_details carries no pool index, so policer_dump_v2
//     with an index answers that one policer (nothing for a free slot) and ~0 answers all; policer_update zeroes the
//     counters; policer_reset refills the buckets and keeps the counters.
//   - policer_input / policer_output (apply=1) enable the policer feature on EVERY call — a second apply stacks a
//     second instance of the node (D-076). apply=0 on an interface that never had a policer since VPP started is
//     VPP's out-of-bounds write (policer_op.c): the model counts it in UnseenUnapplies (tests assert it stays 0).
//   - qos record / store enables are reference-counted; disable at zero answers VALUE_EXIST; store implements the ip
//     source only (UNIMPLEMENTED otherwise). qos_mark_enable_disable replaces the map of (interface, source).
//
// QoSRestart models a VPP restart for these tables (everything gone, feature counts reset). Installed by New
// (one registration line; TD-23 turns it into RegisterExtension("qos-flat", …)).

import (
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/policer"
	"ngfw/agent/binapi/qos"
)

// QoSPolicer is one policer of the model.
type QoSPolicer struct {
	Index   uint32
	Details policer.PolicerDetails
	Resets  int
}

type qosKey struct {
	idx uint32
	src qos.QosSource
}

type qosModel struct {
	mu       sync.Mutex
	next     uint32
	policers map[uint32]*QoSPolicer
	// feature instances per "in|out/<sw_if_index>" and whether it ever had one since the (modelled) VPP start
	features map[string]int
	seen     map[string]bool
	unseen   int
	records  map[qosKey]int
	stores   map[qosKey]int
	storeVal map[qosKey]uint8
	maps     map[uint32]qos.QosEgressMap
	marks    map[qosKey]uint32
}

var qosModels sync.Map // *VPP → *qosModel

func (v *VPP) qosModel() *qosModel {
	m, _ := qosModels.LoadOrStore(v, newQoSModel())
	return m.(*qosModel)
}

func newQoSModel() *qosModel {
	return &qosModel{policers: map[uint32]*QoSPolicer{}, features: map[string]int{}, seen: map[string]bool{},
		records: map[qosKey]int{}, stores: map[qosKey]int{}, storeVal: map[qosKey]uint8{}, maps: map[uint32]qos.QosEgressMap{}, marks: map[qosKey]uint32{}}
}

// QoSRestart drops every policer, attachment, record, store, map and mark (a modelled VPP restart of these tables).
func (v *VPP) QoSRestart() {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next, m.unseen = 0, 0
	m.policers, m.features, m.seen = map[uint32]*QoSPolicer{}, map[string]int{}, map[string]bool{}
	m.records, m.stores, m.storeVal = map[qosKey]int{}, map[qosKey]int{}, map[qosKey]uint8{}
	m.maps, m.marks = map[uint32]qos.QosEgressMap{}, map[qosKey]uint32{}
}

// QoSPolicers returns the policers of the model by VPP name.
func (v *VPP) QoSPolicers() map[string]QoSPolicer {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]QoSPolicer{}
	for _, p := range m.policers {
		out[p.Details.Name] = *p
	}
	return out
}

// DeleteQoSPolicer removes a policer behind the agent's back (simulated loss); false when absent.
func (v *VPP) DeleteQoSPolicer(name string) bool {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.policers {
		if p.Details.Name == name {
			delete(m.policers, i)
			return true
		}
	}
	return false
}

// PolicerFeatures returns how many policer feature instances are enabled on sw_if_index in direction dir ("in" or
// "out"): 1 is correct, 2 or more is a stacked (double-applied) policer.
func (v *VPP) PolicerFeatures(dir string, swIfIndex uint32) int {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.features[dir+"/"+itoa(swIfIndex)]
}

// UnseenUnapplies counts apply=0 on an interface that had no policer since the modelled VPP start (VPP writes out
// of bounds there — must stay 0).
func (v *VPP) UnseenUnapplies() int {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unseen
}

// QoSMaps returns the egress map ids of the model, sorted.
func (v *VPP) QoSMaps() []uint32 {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]uint32, 0, len(m.maps))
	for id := range m.maps {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// DeleteQoSMap removes an egress map behind the agent's back (simulated loss).
func (v *VPP) DeleteQoSMap(id uint32) {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.maps, id)
}

// QoSMarks returns (sw_if_index → map id) of the marks with output source src.
func (v *VPP) QoSMarks(src qos.QosSource) map[uint32]uint32 {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[uint32]uint32{}
	for k, id := range m.marks {
		if k.src == src {
			out[k.idx] = id
		}
	}
	return out
}

// QoSRecords returns the number of record enables of (sw_if_index, src).
func (v *VPP) QoSRecords(swIfIndex uint32, src qos.QosSource) int {
	m := v.qosModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records[qosKey{swIfIndex, src}]
}

func (v *VPP) hasIndex(idx uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.Ifaces[idx]
	return ok
}

func (v *VPP) installQoSFlat() {
	m := v.qosModel()
	fromCfg := func(name string, c policer.PolicerAdd) policer.PolicerDetails {
		i := c.Infos
		return policer.PolicerDetails{Name: name, Cir: i.Cir, Eir: i.Eir, Cb: i.Cb, Eb: i.Eb, RateType: i.RateType, RoundType: i.RoundType,
			Type: i.Type, ColorAware: i.ColorAware, ConformAction: i.ConformAction, ExceedAction: i.ExceedAction, ViolateAction: i.ViolateAction,
			CurrentLimit: uint32(min(i.Cb, 1<<31)), CurrentBucket: uint32(min(i.Cb, 1<<31)), //nolint:gosec // G115: bounded
			ExtendedLimit: uint32(min(i.Eb, 1<<31)), ExtendedBucket: uint32(min(i.Eb, 1<<31))} //nolint:gosec // G115: bounded
	}
	v.On("policer_add", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerAdd)
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, p := range m.policers {
			if p.Details.Name == r.Name {
				return reply(&policer.PolicerAddReply{Retval: int32(api.VALUE_EXIST)})
			}
		}
		idx := m.next
		m.next++
		m.policers[idx] = &QoSPolicer{Index: idx, Details: fromCfg(r.Name, *r)}
		return reply(&policer.PolicerAddReply{PolicerIndex: idx})
	})
	v.On("policer_update", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerUpdate)
		m.mu.Lock()
		defer m.mu.Unlock()
		p, ok := m.policers[r.PolicerIndex]
		if !ok {
			return reply(&policer.PolicerUpdateReply{Retval: RetvalNoSuchEntry})
		}
		p.Details = fromCfg(p.Details.Name, policer.PolicerAdd{Infos: r.Infos})
		return reply(&policer.PolicerUpdateReply{})
	})
	v.On("policer_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerDel)
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.policers[r.PolicerIndex]; !ok {
			return reply(&policer.PolicerDelReply{Retval: RetvalNoSuchEntry})
		}
		delete(m.policers, r.PolicerIndex)
		return reply(&policer.PolicerDelReply{})
	})
	v.On("policer_reset", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerReset)
		m.mu.Lock()
		defer m.mu.Unlock()
		p, ok := m.policers[r.PolicerIndex]
		if !ok {
			return reply(&policer.PolicerResetReply{Retval: RetvalNoSuchEntry})
		}
		p.Details.CurrentBucket, p.Details.ExtendedBucket = p.Details.CurrentLimit, p.Details.ExtendedLimit
		p.Resets++
		return reply(&policer.PolicerResetReply{})
	})
	v.On("policer_dump_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerDumpV2)
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.PolicerIndex != ^uint32(0) {
			if p, ok := m.policers[r.PolicerIndex]; ok {
				d := p.Details
				return []api.Message{&d}, nil
			}
			return nil, nil
		}
		idx := make([]uint32, 0, len(m.policers))
		for i := range m.policers {
			idx = append(idx, i)
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a] < idx[b] })
		out := make([]api.Message, 0, len(idx))
		for _, i := range idx {
			d := m.policers[i].Details
			out = append(out, &d)
		}
		return out, nil
	})
	apply := func(dir, name string, sw interface_types.InterfaceIndex, on bool) int32 {
		if !v.hasIndex(uint32(sw)) {
			return RetvalInvalidSwIfIndex
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		found := false
		for _, p := range m.policers {
			found = found || p.Details.Name == name
		}
		if !found {
			return RetvalNoSuchEntry
		}
		k := dir + "/" + itoa(uint32(sw))
		switch {
		case on:
			m.features[k]++
			m.seen[k] = true
		case !m.seen[k]:
			m.unseen++ // VPP: out-of-bounds write
		case m.features[k] > 0:
			m.features[k]--
		}
		return 0
	}
	v.On("policer_input", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerInput)
		return reply(&policer.PolicerInputReply{Retval: apply("in", r.Name, r.SwIfIndex, r.Apply)})
	})
	v.On("policer_output", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*policer.PolicerOutput)
		return reply(&policer.PolicerOutputReply{Retval: apply("out", r.Name, r.SwIfIndex, r.Apply)})
	})
	v.On("qos_record_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosRecordEnableDisable)
		k := qosKey{uint32(r.Record.SwIfIndex), r.Record.InputSource}
		if !v.hasIndex(k.idx) {
			return reply(&qos.QosRecordEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Enable {
			m.records[k]++
		} else if m.records[k] == 0 {
			return reply(&qos.QosRecordEnableDisableReply{Retval: int32(api.VALUE_EXIST)})
		} else {
			m.records[k]--
		}
		return reply(&qos.QosRecordEnableDisableReply{})
	})
	v.On("qos_record_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, k := range sortedQoSKeys(m.records) {
			if m.records[k] > 0 {
				out = append(out, &qos.QosRecordDetails{Record: qos.QosRecord{SwIfIndex: interface_types.InterfaceIndex(k.idx), InputSource: k.src}})
			}
		}
		return out, nil
	})
	v.On("qos_store_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosStoreEnableDisable)
		k := qosKey{uint32(r.Store.SwIfIndex), r.Store.InputSource}
		if k.src != qos.QOS_API_SOURCE_IP {
			return reply(&qos.QosStoreEnableDisableReply{Retval: int32(api.UNIMPLEMENTED)})
		}
		if !v.hasIndex(k.idx) {
			return reply(&qos.QosStoreEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Enable {
			if m.stores[k] == 0 {
				m.storeVal[k] = r.Store.Value
			}
			m.stores[k]++
		} else if m.stores[k] == 0 {
			return reply(&qos.QosStoreEnableDisableReply{Retval: int32(api.VALUE_EXIST)})
		} else {
			m.stores[k]--
		}
		return reply(&qos.QosStoreEnableDisableReply{})
	})
	v.On("qos_store_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, k := range sortedQoSKeys(m.stores) {
			if m.stores[k] > 0 {
				out = append(out, &qos.QosStoreDetails{Store: qos.QosStore{SwIfIndex: interface_types.InterfaceIndex(k.idx), InputSource: k.src, Value: m.storeVal[k]}})
			}
		}
		return out, nil
	})
	v.On("qos_egress_map_update", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosEgressMapUpdate)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.maps[r.Map.ID] = r.Map
		return reply(&qos.QosEgressMapUpdateReply{})
	})
	v.On("qos_egress_map_delete", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosEgressMapDelete)
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.maps, r.ID)
		return reply(&qos.QosEgressMapDeleteReply{})
	})
	v.On("qos_egress_map_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		ids := make([]uint32, 0, len(m.maps))
		for id := range m.maps {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		out := make([]api.Message, 0, len(ids))
		for _, id := range ids {
			out = append(out, &qos.QosEgressMapDetails{Map: m.maps[id]})
		}
		return out, nil
	})
	v.On("qos_mark_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosMarkEnableDisable)
		k := qosKey{r.Mark.SwIfIndex, r.Mark.OutputSource}
		if !v.hasIndex(k.idx) {
			return reply(&qos.QosMarkEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Enable {
			if _, ok := m.maps[r.Mark.MapID]; !ok {
				return reply(&qos.QosMarkEnableDisableReply{Retval: RetvalNoSuchEntry})
			}
			m.marks[k] = r.Mark.MapID
		} else if _, ok := m.marks[k]; !ok {
			return reply(&qos.QosMarkEnableDisableReply{Retval: int32(api.VALUE_EXIST)})
		} else {
			delete(m.marks, k)
		}
		return reply(&qos.QosMarkEnableDisableReply{})
	})
	v.On("qos_mark_dump", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*qos.QosMarkDump)
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, k := range sortedQoSKeys(m.marks) {
			if uint32(r.SwIfIndex) == ^uint32(0) || uint32(r.SwIfIndex) == k.idx {
				out = append(out, &qos.QosMarkDetails{Mark: qos.QosMark{SwIfIndex: k.idx, MapID: m.marks[k], OutputSource: k.src}})
			}
		}
		return out, nil
	})
}

func sortedQoSKeys[V any](m map[qosKey]V) []qosKey {
	out := make([]qosKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].idx != out[j].idx {
			return out[i].idx < out[j].idx
		}
		return out[i].src < out[j].src
	})
	return out
}
