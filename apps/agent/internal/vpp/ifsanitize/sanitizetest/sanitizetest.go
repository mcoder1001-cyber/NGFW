// Package sanitizetest models, on internal/vpp/fake, the per-sw_if_index state VPP keeps after
// an interface is deleted (V19/V21) and the messages ifsanitize.Sanitize uses, with VPP 26.06's
// semantics (vnet/classify/in_out_acl.c, policer_classify.c, flow_classify.c, ip4_forward.c,
// plugins/adl, plugins/vxlan): an unbind naming a table that is not the bound one, or a freed
// table, answers NO_SUCH_TABLE (-65).
//
// Descriptor tests whose creators now sanitize call Clean(f) to answer those messages for a VPP
// without inherited state; Model is the stateful version for tests that plant inherited state.
package sanitizetest

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.fd.io/govpp/api"

	adlapi "ngfw/agent/binapi/adl"
	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
	l2api "ngfw/agent/binapi/l2"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
)

const none = ifsanitize.NoIndex

// Iface is the per-sw_if_index state VPP keeps (index 0 ip4, 1 ip6, 2 l2 where applicable).
type Iface struct {
	IPTable     [2]uint32
	L2In, L2Out [3]uint32
	InACL       [3]uint32
	OutACL      [3]uint32
	Policer     [3]uint32
	Flow        [2]uint32
	// Features holds enabled "arc/feature" pairs.
	Features map[string]bool
	ADL      bool
	Vxlan    [2]bool
	// SPD is the bound SPD pool index (none: unbound).
	SPD uint32
	// L3Resets counts sw_interface_set_l2_bridge enable=0 calls.
	L3Resets int
}

// Clear returns a state without bindings.
func Clear() *Iface {
	return &Iface{
		IPTable: [2]uint32{none, none}, L2In: [3]uint32{none, none, none}, L2Out: [3]uint32{none, none, none},
		InACL: [3]uint32{none, none, none}, OutACL: [3]uint32{none, none, none}, Policer: [3]uint32{none, none, none},
		Flow: [2]uint32{none, none}, Features: map[string]bool{}, SPD: none,
	}
}

// Model is a stateful VPP for the sanitize messages.
type Model struct {
	mu     sync.Mutex
	Tables map[uint32]bool
	Ifs    map[uint32]*Iface
	// SPDs maps spd_id → SPD pool index.
	SPDs map[uint32]uint32
	// Free is the classify table pool's free list (LIFO, as vppinfra pool); Len the pool vector
	// length (grown to cover every table index).
	Free []uint32
	Len  uint32
	// Created counts classify_add_del_table adds (placeholders).
	Created int
	masks   map[uint32][]byte
}

// NewModel returns an empty model.
func NewModel() *Model {
	return &Model{Tables: map[uint32]bool{}, Ifs: map[uint32]*Iface{}, SPDs: map[uint32]uint32{}, masks: map[uint32][]byte{}}
}

// If returns (creating) the state of sw_if_index idx.
func (m *Model) If(idx uint32) *Iface {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ifLocked(idx)
}

func (m *Model) ifLocked(idx uint32) *Iface {
	s, ok := m.Ifs[idx]
	if !ok {
		s = Clear()
		m.Ifs[idx] = s
	}
	return s
}

const errNoSuchTable = ifsanitize.ErrNoSuchTable

// unbind models vnet_set_*_intfc(is_add=0): for each named table, it must exist and be the
// bound one; then the slot is cleared (and its feature disabled).
func (m *Model) unbind(slots []uint32, want []uint32, features []string, f map[string]bool) error {
	for i, t := range want {
		if t == none {
			continue
		}
		if !m.Tables[t] || slots[i] != t {
			return errNoSuchTable
		}
		slots[i] = none
		if features != nil && features[i] != "" {
			delete(f, features[i])
		}
	}
	return nil
}

var (
	inFeatures      = []string{"ip4-unicast/ip4-inacl", "ip6-unicast/ip6-inacl", "l2:l2-input-acl"}
	outFeatures     = []string{"ip4-output/ip4-outacl", "ip6-output/ip6-outacl", "l2:l2-output-acl"}
	policerFeatures = []string{"ip4-unicast/ip4-policer-classify", "ip6-unicast/ip6-policer-classify", "l2:l2-policer"}
	flowFeatures    = []string{"ip4-unicast/ip4-flow-classify", "ip6-unicast/ip6-flow-classify"}
)

// DeleteTable frees table t as VPP does (index pushed on the pool's free list).
func (m *Model) DeleteTable(t uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.growLocked()
	if t+1 > m.Len {
		m.Len = t + 1
	}
	delete(m.Tables, t)
	delete(m.masks, t)
	m.Free = append(m.Free, t)
}

// growLocked makes the pool consistent with Tables: the vector covers every table, and every index
// below Len that is neither a table nor on the free list is a freed index (pushed ascending, so the
// highest pops first).
func (m *Model) growLocked() {
	for t := range m.Tables {
		if t+1 > m.Len {
			m.Len = t + 1
		}
	}
	onFree := map[uint32]bool{}
	for _, f := range m.Free {
		onFree[f] = true
	}
	for i := uint32(0); i < m.Len; i++ {
		if !m.Tables[i] && !onFree[i] {
			m.Free = append([]uint32{i}, m.Free...)
		}
	}
}

// Pool returns the classify table pool as vppinfra keeps it: the vector length (it never
// shrinks) and a copy of the free list, bottom first (the last entry pops first).
func (m *Model) Pool() (uint32, []uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.growLocked()
	return m.Len, append([]uint32(nil), m.Free...)
}

// Install registers the model's handlers on f (replacing earlier ones for these messages).
func (m *Model) Install(f *fake.Client) {
	for name, h := range m.Handlers() {
		f.On(name, h)
	}
}

// Handlers returns the model's handler per message name (Messages).
func (m *Model) Handlers() map[string]fake.Handler {
	hs := map[string]fake.Handler{}
	f := handlerSet(hs)
	one := func(r api.Message) []api.Message { return []api.Message{r} }
	f.On("classify_table_ids", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		rep := &classifyapi.ClassifyTableIdsReply{}
		for id := range m.Tables {
			rep.Ids = append(rep.Ids, id)
		}
		rep.Count = uint32(len(rep.Ids)) //nolint:gosec // test sizes
		return one(rep), nil
	})
	f.On("classify_add_del_table", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyAddDelTable)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.growLocked()
		if !r.IsAdd {
			if !m.Tables[r.TableIndex] {
				return nil, api.VPPApiError(-6)
			}
			delete(m.Tables, r.TableIndex)
			delete(m.masks, r.TableIndex)
			m.Free = append(m.Free, r.TableIndex)
			return one(&classifyapi.ClassifyAddDelTableReply{}), nil
		}
		var idx uint32
		if n := len(m.Free); n > 0 {
			idx, m.Free = m.Free[n-1], m.Free[:n-1]
		} else {
			idx = m.Len
			m.Len++
		}
		m.Tables[idx] = true
		m.masks[idx] = append([]byte(nil), r.Mask...)
		m.Created++
		return one(&classifyapi.ClassifyAddDelTableReply{NewTableIndex: idx, MatchNVectors: r.MatchNVectors, SkipNVectors: r.SkipNVectors}), nil
	})
	f.On("classify_table_info", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyTableInfo)
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.Tables[r.TableID] {
			return nil, api.VPPApiError(-6)
		}
		mask := m.masks[r.TableID]
		if mask == nil {
			mask = make([]byte, 16)
		}
		return one(&classifyapi.ClassifyTableInfoReply{TableID: r.TableID, MatchNVectors: 1, NextTableIndex: none, MaskLength: uint32(len(mask)), Mask: mask}), nil //nolint:gosec // 16
	})
	f.On("sw_interface_set_l2_bridge", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2api.SwInterfaceSetL2Bridge)
		m.mu.Lock()
		defer m.mu.Unlock()
		if !r.Enable {
			st := m.ifLocked(uint32(r.RxSwIfIndex))
			for k := range st.Features {
				if strings.HasPrefix(k, "l2:") {
					delete(st.Features, k)
				}
			}
			st.L3Resets++
		}
		return one(&l2api.SwInterfaceSetL2BridgeReply{}), nil
	})
	f.On("classify_set_interface_ip_table", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifySetInterfaceIPTable)
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.TableIndex != none && !m.Tables[r.TableIndex] {
			return nil, api.VPPApiError(-6) // NO_SUCH_ENTRY
		}
		i := 0
		if r.IsIPv6 {
			i = 1
		}
		m.ifLocked(uint32(r.SwIfIndex)).IPTable[i] = r.TableIndex
		return one(&classifyapi.ClassifySetInterfaceIPTableReply{}), nil
	})
	f.On("classify_set_interface_l2_tables", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifySetInterfaceL2Tables)
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, t := range []uint32{r.IP4TableIndex, r.IP6TableIndex, r.OtherTableIndex} {
			if t != none && !m.Tables[t] {
				return nil, errNoSuchTable
			}
		}
		s := m.ifLocked(uint32(r.SwIfIndex))
		v := [3]uint32{r.IP4TableIndex, r.IP6TableIndex, r.OtherTableIndex}
		if r.IsInput {
			s.L2In = v
		} else {
			s.L2Out = v
		}
		return one(&classifyapi.ClassifySetInterfaceL2TablesReply{}), nil
	})
	f.On("classify_table_by_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyTableByInterface)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		return one(&classifyapi.ClassifyTableByInterfaceReply{SwIfIndex: r.SwIfIndex, IP4TableID: s.InACL[0], IP6TableID: s.InACL[1], L2TableID: s.InACL[2]}), nil
	})
	f.On("input_acl_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.InputACLSetInterface)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		if err := m.set(s.InACL[:], []uint32{r.IP4TableIndex, r.IP6TableIndex, r.L2TableIndex}, r.IsAdd, inFeatures, s.Features); err != nil {
			return nil, err
		}
		return one(&classifyapi.InputACLSetInterfaceReply{}), nil
	})
	f.On("output_acl_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.OutputACLSetInterface)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		if err := m.set(s.OutACL[:], []uint32{r.IP4TableIndex, r.IP6TableIndex, r.L2TableIndex}, r.IsAdd, outFeatures, s.Features); err != nil {
			return nil, err
		}
		return one(&classifyapi.OutputACLSetInterfaceReply{}), nil
	})
	f.On("policer_classify_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.PolicerClassifySetInterface)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		if err := m.set(s.Policer[:], []uint32{r.IP4TableIndex, r.IP6TableIndex, r.L2TableIndex}, r.IsAdd, policerFeatures, s.Features); err != nil {
			return nil, err
		}
		return one(&classifyapi.PolicerClassifySetInterfaceReply{}), nil
	})
	f.On("flow_classify_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.FlowClassifySetInterface)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		if err := m.set(s.Flow[:], []uint32{r.IP4TableIndex, r.IP6TableIndex}, r.IsAdd, flowFeatures, s.Features); err != nil {
			return nil, err
		}
		return one(&classifyapi.FlowClassifySetInterfaceReply{}), nil
	})
	// policer/flow_classify_dump as VPP 26.06 answers them (classify_api.c): the handler returns
	// nothing unless sw_if_index < vec_len(vector), and walks vec_len(&vector[sw_if_index]) — for
	// sw_if_index 0 that is the vector itself (every binding of every index, deleted ones
	// included), for any other index a length read out of bounds (the model fails the test).
	dump := func(name string, slots func(*Iface) []uint32, typ func(api.Message) (int, uint32), details func(idx, table uint32) api.Message) fake.Handler {
		return func(req api.Message) ([]api.Message, error) {
			i, filter := typ(req)
			m.mu.Lock()
			defer m.mu.Unlock()
			switch filter {
			case none:
				return nil, nil
			case 0:
			default:
				return nil, fmt.Errorf("sanitizetest: %s with sw_if_index %d reads out of bounds in VPP 26.06 (vec_len on a pointer into the vector)", name, filter)
			}
			var out []api.Message
			for _, idx := range sortedIfs(m.Ifs) {
				if t := slots(m.Ifs[idx])[i]; t != none {
					out = append(out, details(idx, t))
				}
			}
			return out, nil
		}
	}
	f.On("policer_classify_dump", dump("policer_classify_dump", func(s *Iface) []uint32 { return s.Policer[:] },
		func(req api.Message) (int, uint32) {
			r := req.(*classifyapi.PolicerClassifyDump)
			return int(r.Type), uint32(r.SwIfIndex)
		},
		func(idx, t uint32) api.Message {
			return &classifyapi.PolicerClassifyDetails{SwIfIndex: interface_types.InterfaceIndex(idx), TableIndex: t}
		}))
	f.On("flow_classify_dump", dump("flow_classify_dump", func(s *Iface) []uint32 { return s.Flow[:] },
		func(req api.Message) (int, uint32) {
			r := req.(*classifyapi.FlowClassifyDump)
			return int(r.Type), uint32(r.SwIfIndex)
		},
		func(idx, t uint32) api.Message {
			return &classifyapi.FlowClassifyDetails{SwIfIndex: interface_types.InterfaceIndex(idx), TableIndex: t}
		}))
	f.On("adl_interface_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*adlapi.AdlInterfaceEnableDisable)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.ifLocked(uint32(r.SwIfIndex)).ADL = r.EnableDisable
		return one(&adlapi.AdlInterfaceEnableDisableReply{}), nil
	})
	f.On("sw_interface_set_vxlan_bypass", func(req api.Message) ([]api.Message, error) {
		r := req.(*vxlanapi.SwInterfaceSetVxlanBypass)
		m.mu.Lock()
		defer m.mu.Unlock()
		i := 0
		if r.IsIPv6 {
			i = 1
		}
		m.ifLocked(uint32(r.SwIfIndex)).Vxlan[i] = r.Enable
		return one(&vxlanapi.SwInterfaceSetVxlanBypassReply{}), nil
	})
	f.On("ipsec_spd_interface_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIfs(m.Ifs) {
			if spd := m.Ifs[idx].SPD; spd != none {
				out = append(out, &ipsecapi.IpsecSpdInterfaceDetails{SpdIndex: spd, SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return out, nil
	})
	f.On("ipsec_spds_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, id := range sortedIfs(m.SPDs) {
			out = append(out, &ipsecapi.IpsecSpdsDetails{SpdID: id})
		}
		return out, nil
	})
	f.On("ipsec_interface_add_del_spd", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsecapi.IpsecInterfaceAddDelSpd)
		m.mu.Lock()
		defer m.mu.Unlock()
		pool, ok := m.SPDs[r.SpdID]
		if !ok {
			return nil, api.VPPApiError(-11) // SYSCALL_ERROR_1: no such spd-id
		}
		s := m.ifLocked(uint32(r.SwIfIndex))
		switch {
		case r.IsAdd && s.SPD != none:
			return nil, api.VPPApiError(-12) // SYSCALL_ERROR_2: spd already assigned
		case r.IsAdd:
			s.SPD = pool
		default:
			s.SPD = none // VPP does not check that the named SPD is the bound one
		}
		return one(&ipsecapi.IpsecInterfaceAddDelSpdReply{}), nil
	})
	return hs
}

func sortedIfs[V any](m map[uint32]V) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// handlerSet collects handlers with the fake's On signature.
type handlerSet map[string]fake.Handler

func (h handlerSet) On(name string, fn fake.Handler) { h[name] = fn }

// set models vnet_set_*_intfc: an add is a silent no-op while the slot is bound (V19: also
// when the bound table is gone), a delete must name the bound, existing table.
func (m *Model) set(slots []uint32, want []uint32, isAdd bool, features []string, f map[string]bool) error {
	if !isAdd {
		return m.unbind(slots, want, features, f)
	}
	for i, t := range want {
		if t == none {
			continue
		}
		if !m.Tables[t] {
			return errNoSuchTable
		}
		if slots[i] != none {
			return nil
		}
		slots[i] = t
		if features[i] != "" {
			f[features[i]] = true
		}
	}
	return nil
}

// DeleteInterface models VPP's interface delete: every feature arc is cleared, the per-index
// table vectors, the vxlan bypass bitmap and the ADL config are not.
func (m *Model) DeleteInterface(idx uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.ifLocked(idx)
	for k := range st.Features {
		if !strings.HasPrefix(k, "l2:") { // l2 feature bits are not arcs: kept (TD-3 review H1)
			delete(st.Features, k)
		}
	}
	st.ADL = false
}

// Clean answers the sanitize messages f has no handler for as a VPP without classify tables
// and without inherited state would (a stateful Model underneath, so repeated sanitizes and
// explicit bindings behave like VPP). Handlers a test registered itself are kept.
func Clean(f *fake.Client) *Model {
	m := NewModel()
	for name, h := range m.Handlers() {
		if !f.Handles(name) {
			f.On(name, h)
		}
	}
	return m
}

// Messages are the VPP messages ifsanitize.Sanitize sends.
var Messages = []string{
	"classify_table_ids", "classify_set_interface_ip_table", "classify_set_interface_l2_tables",
	"classify_table_by_interface", "input_acl_set_interface", "output_acl_set_interface",
	"policer_classify_dump", "flow_classify_dump",
	"classify_add_del_table", "classify_table_info", "sw_interface_set_l2_bridge",
	"policer_classify_set_interface", "flow_classify_set_interface",
	"adl_interface_enable_disable", "sw_interface_set_vxlan_bypass",
	"ipsec_spd_interface_dump", "ipsec_spds_dump", "ipsec_interface_add_del_spd",
}

// PoisonTable is the live classify table Poison binds.
const PoisonTable = 3

// Poison plants, on every index in idx, what a deleted interface leaves behind in VPP (V19,
// V21, DF-5 M3): ip4 classify, input ACL ip4 and output ACL ip6 bound to the live table
// PoisonTable, vxlan bypass set, an SPD binding (spd_id 10). Features are off, as after a VPP
// interface delete.
func (m *Model) Poison(idx ...uint32) {
	m.mu.Lock()
	m.Tables[PoisonTable] = true
	m.SPDs[10] = 0
	m.mu.Unlock()
	for _, i := range idx {
		s := m.If(i)
		s.IPTable[0] = PoisonTable
		s.InACL[0] = PoisonTable
		s.OutACL[1] = PoisonTable
		s.Vxlan = [2]bool{true, true}
		s.SPD = 0
	}
}

// Dirty reports what is still inherited on idx ("" when nothing).
func (m *Model) Dirty(idx uint32) string {
	s := m.If(idx)
	c := Clear()
	switch {
	case s.IPTable != c.IPTable:
		return "ip classify"
	case s.L2In != c.L2In || s.L2Out != c.L2Out:
		return "l2 classify"
	case s.InACL != c.InACL:
		return "input acl"
	case s.OutACL != c.OutACL:
		return "output acl"
	case s.Policer != c.Policer:
		return "policer classify"
	case s.Flow != c.Flow:
		return "flow classify"
	case s.Vxlan != c.Vxlan:
		return "vxlan bypass"
	case s.ADL:
		return "adl"
	case s.SPD != none:
		return "ipsec spd"
	}
	for k := range s.Features {
		if strings.HasPrefix(k, "l2:") {
			return "l2 feature bit " + k
		}
	}
	return ""
}

// Order returns the position of the first request named first and of the first request named
// then in f's call log (-1 when absent): tests assert the sanitize ran before the interface was
// tagged / reported created.
func Order(f *fake.Client, first, then string) (int, int) {
	a, b := -1, -1
	for i, c := range f.Calls() {
		n := c.GetMessageName()
		if n == first && a < 0 {
			a = i
		}
		if n == then && b < 0 {
			b = i
		}
	}
	return a, b
}
