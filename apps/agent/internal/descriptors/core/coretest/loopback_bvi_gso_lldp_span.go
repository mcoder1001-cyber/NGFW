package coretest

// F-loopback-bvi-gso-lldp-span extension of the model (wave-A-hotspots A6): GSO, SPAN, LLDP and nsim,
// so the agent's projection, Retrieve and restart paths run in unit tests with the real gso, DF-7
// span/lldp and nsim descriptors. Behaviour follows VPP 26.06:
//   - feature_gso_enable_disable stacks the feature on every enable (vnet_config_add_feature has no
//     duplicate check) and a disable removes one instance; feature_is_enabled("ip4-output", "gso-ip4")
//     answers true for an index the arc never reached (VPP's int→bool cast);
//   - sw_interface_span_enable_disable sets the (source, destination, level) direction bits in place
//     (idempotent), state 0 clears them, source == destination is refused;
//   - sw_interface_set_lldp enables / disables LLDP on the interface (the model keeps sw == hw index; the
//     V20 mismatch is DF-7's unit tests' business), lldp_dump lists the enabled interfaces;
//   - nsim: the cross-connect and the output feature fail with -76 (CANNOT_ENABLE_DISABLE_FEATURE)
//     before nsim_configure2, need hardware interfaces (no sub-interfaces), and the output feature
//     stacks like any feature; nsim_configure2 refuses a zero delay/bandwidth and packet sizes outside
//     64–9000 (-77, -78, -79 as VPP's INVALID_VALUE_2/…).
//
// feature_is_enabled goes through the TD-23 seam (gsoIsEnabled below): this file answers the gso-ip4 read-back and
// hands every other arc/feature to the F-bridge-l2 answer (mactime), which it replicates.

import (
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/feature"
	gsoapi "ngfw/agent/binapi/gso"
	"ngfw/agent/binapi/interface_types"
	lldpapi "ngfw/agent/binapi/lldp"
	nsimapi "ngfw/agent/binapi/nsim"
	spanapi "ngfw/agent/binapi/span"
	"ngfw/agent/binapi/vlib"
)

// VPP retvals of the nsim handlers (vnet/api_errno.h).
const (
	RetvalCannotEnableDisableFeature int32 = -76
	RetvalInvalidValue               int32 = -77
	RetvalInvalidValue2              int32 = -78
	RetvalInvalidValue3              int32 = -79
	RetvalSpanInvalidInterface       int32 = -11
)

type spanKey struct {
	from, to uint32
	l2       bool
}

// NsimModel is the nsim state of the model (tests read it with Nsim).
type NsimModel struct {
	Configured bool
	Config     nsimapi.NsimConfigure2
	// Configures counts nsim_configure2 calls (each one reallocates VPP's wheels).
	Configures int
	// CrossA / CrossB are the cross-connected sw_if_indexes (0 = none); CrossCount the stacked enables.
	CrossA, CrossB uint32
	CrossCount     int
	// Output is the stacked output-feature count per sw_if_index.
	Output map[uint32]int
}

// lbgsModel is the F-loopback-bvi-gso-lldp-span state of one VPP model (guarded by VPP.mu).
type lbgsModel struct {
	gso        map[uint32]int
	gsoReached map[uint32]bool
	span       map[spanKey]spanapi.SpanState
	lldp       map[uint32]lldpapi.SwInterfaceSetLldp
	lldpGlobal *lldpapi.LldpConfig
	nsim       NsimModel
}

var (
	lbgsModelsMu sync.Mutex
	lbgsModels   = map[*VPP]*lbgsModel{}
)

func (v *VPP) lbgs() *lbgsModel {
	lbgsModelsMu.Lock()
	defer lbgsModelsMu.Unlock()
	m, ok := lbgsModels[v]
	if !ok {
		m = &lbgsModel{gso: map[uint32]int{}, gsoReached: map[uint32]bool{}, span: map[spanKey]spanapi.SpanState{},
			lldp: map[uint32]lldpapi.SwInterfaceSetLldp{}, nsim: NsimModel{Output: map[uint32]int{}}}
		lbgsModels[v] = m
	}
	return m
}

// lbgsGCLocked drops the state of deleted interfaces (as the F-bridge-l2 model does for its features).
func (v *VPP) lbgsGCLocked(m *lbgsModel) {
	for idx := range m.gso {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.gso, idx)
		}
	}
	for k := range m.span {
		_, ok1 := v.Ifaces[k.from]
		_, ok2 := v.Ifaces[k.to]
		if !ok1 || !ok2 {
			delete(m.span, k)
		}
	}
	for idx := range m.lldp {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.lldp, idx)
		}
	}
	for idx := range m.nsim.Output {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.nsim.Output, idx)
		}
	}
}

// hardwareLocked reports whether idx is an existing non-sub interface.
func (v *VPP) hardwareLocked(idx uint32) bool {
	i, ok := v.Ifaces[idx]
	return ok && !i.IsSub && idx != 0
}

// gsoIsEnabled answers feature_is_enabled for gso-ip4 on ip4-output through the TD-23 composition seam
// (RegisterFeatureIsEnabled); mactime's answer is F-bridge-l2's own registration (coretest/bridge_l2.go).
func gsoIsEnabled(v *VPP, r *feature.FeatureIsEnabled) *feature.FeatureIsEnabledReply {
	v.mu.Lock()
	defer v.mu.Unlock()
	idx := uint32(r.SwIfIndex)
	if _, ok := v.Ifaces[idx]; !ok {
		return &feature.FeatureIsEnabledReply{Retval: RetvalInvalidSwIfIndex}
	}
	if r.ArcName != "ip4-output" {
		return &feature.FeatureIsEnabledReply{IsEnabled: true} // VPP's own unknown-feature cast
	}
	m := v.lbgs()
	v.lbgsGCLocked(m)
	return &feature.FeatureIsEnabledReply{IsEnabled: m.gso[idx] > 0 || !m.gsoReached[idx]}
}

func init() {
	RegisterExtension("loopback-bvi-gso-lldp-span", (*VPP).installLoopbackBviGsoLldpSpan)
	RegisterFeatureIsEnabled("gso-ip4", gsoIsEnabled)
}

func (v *VPP) installLoopbackBviGsoLldpSpan() {
	v.On("feature_gso_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*gsoapi.FeatureGsoEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		idx := uint32(r.SwIfIndex)
		if _, ok := v.Ifaces[idx]; !ok {
			return reply(&gsoapi.FeatureGsoEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.gsoReached[idx] = true
		if r.EnableDisable {
			m.gso[idx]++
		} else if m.gso[idx] > 0 {
			m.gso[idx]--
		}
		return reply(&gsoapi.FeatureGsoEnableDisableReply{})
	})
	v.On("sw_interface_span_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*spanapi.SwInterfaceSpanEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		from, to := uint32(r.SwIfIndexFrom), uint32(r.SwIfIndexTo)
		if r.State > spanapi.SPAN_STATE_API_RX_TX {
			return reply(&spanapi.SwInterfaceSpanEnableDisableReply{Retval: RetvalInvalidValue})
		}
		if from == to {
			return reply(&spanapi.SwInterfaceSpanEnableDisableReply{Retval: RetvalSpanInvalidInterface})
		}
		if _, ok := v.Ifaces[from]; !ok {
			return reply(&spanapi.SwInterfaceSpanEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		if _, ok := v.Ifaces[to]; !ok && r.State != spanapi.SPAN_STATE_API_DISABLED {
			return reply(&spanapi.SwInterfaceSpanEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		k := spanKey{from, to, r.IsL2}
		if r.State == spanapi.SPAN_STATE_API_DISABLED {
			delete(m.span, k)
		} else {
			m.span[k] = r.State
		}
		return reply(&spanapi.SwInterfaceSpanEnableDisableReply{})
	})
	v.On("sw_interface_span_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*spanapi.SwInterfaceSpanDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		v.lbgsGCLocked(m)
		keys := make([]spanKey, 0, len(m.span))
		for k := range m.span {
			if k.l2 == r.IsL2 {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].from != keys[j].from {
				return keys[i].from < keys[j].from
			}
			return keys[i].to < keys[j].to
		})
		var out []api.Message
		for _, k := range keys {
			out = append(out, &spanapi.SwInterfaceSpanDetails{SwIfIndexFrom: interface_types.InterfaceIndex(k.from),
				SwIfIndexTo: interface_types.InterfaceIndex(k.to), State: m.span[k], IsL2: k.l2})
		}
		return out, nil
	})
	v.On("sw_interface_set_lldp", func(req api.Message) ([]api.Message, error) {
		r := req.(*lldpapi.SwInterfaceSetLldp)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		idx := uint32(r.SwIfIndex)
		if !v.hardwareLocked(idx) {
			return reply(&lldpapi.SwInterfaceSetLldpReply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.Enable {
			if _, on := m.lldp[idx]; !on { // VPP ignores new parameters on an enabled interface
				m.lldp[idx] = *r
			}
		} else {
			delete(m.lldp, idx)
		}
		return reply(&lldpapi.SwInterfaceSetLldpReply{})
	})
	v.On("lldp_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		v.lbgsGCLocked(m)
		idxs := make([]uint32, 0, len(m.lldp))
		for idx := range m.lldp {
			idxs = append(idxs, idx)
		}
		sort.Slice(idxs, func(i, j int) bool { return idxs[i] < idxs[j] })
		var out []api.Message
		for _, idx := range idxs {
			out = append(out, &lldpapi.LldpDetails{SwIfIndex: interface_types.InterfaceIndex(idx), LastSent: 10,
				ChassisID: make([]byte, 64), PortID: make([]byte, 64)})
		}
		return append(out, &lldpapi.LldpDumpReply{}), nil
	})
	v.On("lldp_config", func(req api.Message) ([]api.Message, error) {
		r := req.(*lldpapi.LldpConfig)
		v.mu.Lock()
		defer v.mu.Unlock()
		c := *r
		v.lbgs().lldpGlobal = &c
		return reply(&lldpapi.LldpConfigReply{})
	})
	v.On("show_threads", func(api.Message) ([]api.Message, error) { // a VPP without workers (nsim's worker check)
		return reply(&vlib.ShowThreadsReply{Count: 1, ThreadData: []vlib.ThreadData{{ID: 0, Name: "vpp_main"}}})
	})
	v.On("nsim_configure2", func(req api.Message) ([]api.Message, error) {
		r := req.(*nsimapi.NsimConfigure2)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		switch {
		case r.BandwidthInBitsPerSecond == 0:
			return reply(&nsimapi.NsimConfigure2Reply{Retval: RetvalInvalidValue})
		case r.DelayInUsec == 0:
			return reply(&nsimapi.NsimConfigure2Reply{Retval: RetvalInvalidValue2})
		case r.AveragePacketSize < 64 || r.AveragePacketSize > 9000:
			return reply(&nsimapi.NsimConfigure2Reply{Retval: RetvalInvalidValue3})
		}
		m.nsim.Configured = true
		m.nsim.Config = *r
		m.nsim.Configures++
		return reply(&nsimapi.NsimConfigure2Reply{})
	})
	v.On("nsim_cross_connect_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nsimapi.NsimCrossConnectEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		a, b := uint32(r.SwIfIndex0), uint32(r.SwIfIndex1)
		switch {
		case !m.nsim.Configured:
			return reply(&nsimapi.NsimCrossConnectEnableDisableReply{Retval: RetvalCannotEnableDisableFeature})
		case !v.hardwareLocked(a) || !v.hardwareLocked(b):
			return reply(&nsimapi.NsimCrossConnectEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		m.nsim.CrossA, m.nsim.CrossB = a, b // VPP keeps one pair, overwritten by every call
		if r.EnableDisable {
			m.nsim.CrossCount++
		} else if m.nsim.CrossCount > 0 {
			m.nsim.CrossCount--
		}
		return reply(&nsimapi.NsimCrossConnectEnableDisableReply{})
	})
	v.On("nsim_output_feature_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nsimapi.NsimOutputFeatureEnableDisable)
		v.mu.Lock()
		defer v.mu.Unlock()
		m := v.lbgs()
		idx := uint32(r.SwIfIndex)
		switch {
		case !m.nsim.Configured:
			return reply(&nsimapi.NsimOutputFeatureEnableDisableReply{Retval: RetvalCannotEnableDisableFeature})
		case !v.hardwareLocked(idx):
			return reply(&nsimapi.NsimOutputFeatureEnableDisableReply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.EnableDisable {
			m.nsim.Output[idx]++
		} else if m.nsim.Output[idx] > 0 {
			m.nsim.Output[idx]--
		}
		return reply(&nsimapi.NsimOutputFeatureEnableDisableReply{})
	})
}

// GsoCount returns the stacked GSO enables per interface name (tests).
func (v *VPP) GsoCount() map[string]int {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.lbgs()
	v.lbgsGCLocked(m)
	out := map[string]int{}
	for idx, n := range m.gso {
		if n > 0 {
			out[v.Ifaces[idx].Name] = n
		}
	}
	return out
}

// Spans returns "<from>→<to>/<device|l2>" → state of every mirror session (tests).
func (v *VPP) Spans() map[string]spanapi.SpanState {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.lbgs()
	v.lbgsGCLocked(m)
	out := map[string]spanapi.SpanState{}
	for k, s := range m.span {
		level := "device"
		if k.l2 {
			level = "l2"
		}
		out[v.Ifaces[k.from].Name+"→"+v.Ifaces[k.to].Name+"/"+level] = s
	}
	return out
}

// Lldp returns the port description of every LLDP-enabled interface by name (tests).
func (v *VPP) Lldp() map[string]string {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.lbgs()
	v.lbgsGCLocked(m)
	out := map[string]string{}
	for idx, r := range m.lldp {
		out[v.Ifaces[idx].Name] = r.PortDesc
	}
	return out
}

// LldpGlobal returns the last lldp_config (nil = never configured; tests).
func (v *VPP) LldpGlobal() *lldpapi.LldpConfig {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.lbgs().lldpGlobal
}

// Nsim returns a copy of the nsim state (tests).
func (v *VPP) Nsim() NsimModel {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.lbgs()
	v.lbgsGCLocked(m)
	c := m.nsim
	c.Output = map[uint32]int{}
	for k, n := range m.nsim.Output {
		if n > 0 {
			c.Output[k] = n
		}
	}
	return c
}

// ClearLoopbackBviGsoLldpSpan drops every GSO, SPAN, LLDP and nsim-output state "behind the agent's
// back" (the restart simulation's loss of dependents, D-095c); the nsim model stays configured.
func (v *VPP) ClearLoopbackBviGsoLldpSpan() {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.lbgs()
	m.gso = map[uint32]int{}
	m.span = map[spanKey]spanapi.SpanState{}
	m.lldp = map[uint32]lldpapi.SwInterfaceSetLldp{}
	m.nsim.Output = map[uint32]int{}
	m.nsim.CrossCount = 0
}

// EnableLldp enables LLDP on idx behind the agent's back (tests: another owner's interface).
func (v *VPP) EnableLldp(idx uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lbgs().lldp[idx] = lldpapi.SwInterfaceSetLldp{SwIfIndex: interface_types.InterfaceIndex(idx), Enable: true}
}
