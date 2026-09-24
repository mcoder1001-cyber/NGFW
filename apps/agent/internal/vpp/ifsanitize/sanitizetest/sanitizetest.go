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
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	adlapi "ngfw/agent/binapi/adl"
	classifyapi "ngfw/agent/binapi/classify"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
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
}

// NewModel returns an empty model.
func NewModel() *Model {
	return &Model{Tables: map[uint32]bool{}, Ifs: map[uint32]*Iface{}, SPDs: map[uint32]uint32{}}
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
	inFeatures      = []string{"ip4-unicast/ip4-inacl", "ip6-unicast/ip6-inacl", ""}
	outFeatures     = []string{"ip4-output/ip4-outacl", "ip6-output/ip6-outacl", ""}
	policerFeatures = []string{"ip4-unicast/ip4-policer-classify", "ip6-unicast/ip6-policer-classify", ""}
	flowFeatures    = []string{"ip4-unicast/ip4-flow-classify", "ip6-unicast/ip6-flow-classify"}
)

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
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*featureapi.FeatureIsEnabled)
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.ifLocked(uint32(r.SwIfIndex))
		on := s.Features[r.ArcName+"/"+r.FeatureName]
		if r.ArcName == "device-input" && r.FeatureName == "adl-input" {
			on = s.ADL
		}
		return one(&featureapi.FeatureIsEnabledReply{IsEnabled: on}), nil
	})
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
	s := m.ifLocked(idx)
	s.Features = map[string]bool{}
	s.ADL = false
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
	"policer_classify_set_interface", "flow_classify_set_interface", "feature_is_enabled",
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

// PoisonActive plants, on every index in idx, the crash vector Sanitize must refuse: an input
// ACL bound to a deleted table (99) with ip4-inacl enabled.
func (m *Model) PoisonActive(idx ...uint32) {
	for _, i := range idx {
		s := m.If(i)
		s.InACL[0] = 99
		s.Features["ip4-unicast/ip4-inacl"] = true
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
