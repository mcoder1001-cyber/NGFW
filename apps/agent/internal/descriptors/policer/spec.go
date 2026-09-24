// Package policer holds the reconciler descriptors for the VPP policer plugin (task DF-7, WBS
// D7.8): named policers (policer_add / policer_update / policer_del, retrieved with
// policer_dump_v2), their attachment to an interface's input or output path
// (policer_input / policer_output), their binding to a worker thread (policer_bind) and the
// classifier-driven policing of an interface (classify policer_classify_set_interface).
//
// Message names and fields come only from apps/agent/binapi/{policer,policer_types,classify}.
// Desired values are *structpb.Struct documents of the typed specs below (D-055 stand-in).
// docs/agent/descriptors/policer.md is the object ↔ message table.
//
// Ownership: a policer's VPP name is vpp.OwnerTag(owner, name) ("w10:gold"); the attachments
// are owned through the policer and the interface they reference.
package policer

import (
	"ngfw/agent/binapi/policer_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NamePolicer   = "policer.policer"
	NameInterface = "policer.interface"
	NameBind      = "policer.bind"
	NameClassify  = "policer.classify"
)

// Rate types.
const (
	RateKbps = "kbps"
	RatePps  = "pps"
)

// Round types.
const (
	RoundClosest = "closest"
	RoundUp      = "up"
	RoundDown    = "down"
)

// Policer types (RFC algorithms).
const (
	Type1R2C        = "1r2c"
	Type1R3C        = "1r3c-2697"
	Type2R3C2698    = "2r3c-2698"
	Type2R3C4115    = "2r3c-4115"
	Type2R3CMef5CF1 = "2r3c-mef5cf1"
)

// Action types.
const (
	ActDrop     = "drop"
	ActTransmit = "transmit"
	ActMark     = "mark-and-transmit"
)

var (
	rateTypes = map[string]policer_types.Sse2QosRateType{
		RateKbps: policer_types.SSE2_QOS_RATE_API_KBPS,
		RatePps:  policer_types.SSE2_QOS_RATE_API_PPS,
	}
	roundTypes = map[string]policer_types.Sse2QosRoundType{
		RoundClosest: policer_types.SSE2_QOS_ROUND_API_TO_CLOSEST,
		RoundUp:      policer_types.SSE2_QOS_ROUND_API_TO_UP,
		RoundDown:    policer_types.SSE2_QOS_ROUND_API_TO_DOWN,
	}
	policerTypes = map[string]policer_types.Sse2QosPolicerType{
		Type1R2C:        policer_types.SSE2_QOS_POLICER_TYPE_API_1R2C,
		Type1R3C:        policer_types.SSE2_QOS_POLICER_TYPE_API_1R3C_RFC_2697,
		Type2R3C2698:    policer_types.SSE2_QOS_POLICER_TYPE_API_2R3C_RFC_2698,
		Type2R3C4115:    policer_types.SSE2_QOS_POLICER_TYPE_API_2R3C_RFC_4115,
		Type2R3CMef5CF1: policer_types.SSE2_QOS_POLICER_TYPE_API_2R3C_RFC_MEF5CF1,
	}
	actionTypes = map[string]policer_types.Sse2QosActionType{
		ActDrop:     policer_types.SSE2_QOS_ACTION_API_DROP,
		ActTransmit: policer_types.SSE2_QOS_ACTION_API_TRANSMIT,
		ActMark:     policer_types.SSE2_QOS_ACTION_API_MARK_AND_TRANSMIT,
	}
)

func nameOf[K comparable](m map[string]K, v K) string {
	for n, x := range m {
		if x == v {
			return n
		}
	}
	return ""
}

// Action is what the policer does with packets of one colour. DSCP is the value written by
// mark-and-transmit (0–63) and must be 0 for the other actions.
type Action struct {
	Type string `json:"type,omitempty"`
	DSCP uint8  `json:"dscp,omitempty"`
}

// Policer is the desired state of one policer.policer object. Rates are kbit/s (RateKbps) or
// packets/s (RatePps); bursts are bytes. Every enum field is required (the API layer fills
// defaults).
type Policer struct {
	Name       string `json:"name,omitempty"`
	CIR        uint32 `json:"cir,omitempty"`
	EIR        uint32 `json:"eir,omitempty"`
	CB         uint64 `json:"cb,omitempty"`
	EB         uint64 `json:"eb,omitempty"`
	RateType   string `json:"rate_type,omitempty"`
	RoundType  string `json:"round_type,omitempty"`
	Type       string `json:"type,omitempty"`
	ColorAware bool   `json:"color_aware,omitempty"`
	Conform    Action `json:"conform,omitempty"`
	Exceed     Action `json:"exceed,omitempty"`
	Violate    Action `json:"violate,omitempty"`
}

// maxBurst keeps u64 bursts exact through the JSON/structpb encoding.
const maxBurst = 1 << 53

func (a Action) validate(which string) error {
	if _, ok := actionTypes[a.Type]; !ok {
		return df7.Specf("%s action %q: want drop, transmit or mark-and-transmit", which, a.Type)
	}
	if a.DSCP > 63 {
		return df7.Specf("%s action dscp %d exceeds 63", which, a.DSCP)
	}
	if a.Type != ActMark && a.DSCP != 0 {
		return df7.Specf("%s action %q takes no dscp", which, a.Type)
	}
	return nil
}

// Validate checks p for what the scheduler and VPP need: a name that fits the owner tag,
// known enums, bursts below 2^53, EIR ≥ CIR for two-rate policers.
func (p Policer) Validate() error {
	if p.Name == "" {
		return df7.Specf("policer name is empty")
	}
	if _, err := vpp.OwnerTag("w00000", p.Name); err != nil { // longest owner prefix budget
		return df7.Specf("policer name %q: %v", p.Name, err)
	}
	if _, ok := rateTypes[p.RateType]; !ok {
		return df7.Specf("rate_type %q: want kbps or pps", p.RateType)
	}
	if _, ok := roundTypes[p.RoundType]; !ok {
		return df7.Specf("round_type %q: want closest, up or down", p.RoundType)
	}
	if _, ok := policerTypes[p.Type]; !ok {
		return df7.Specf("type %q: want 1r2c, 1r3c-2697, 2r3c-2698, 2r3c-4115 or 2r3c-mef5cf1", p.Type)
	}
	if p.CB >= maxBurst || p.EB >= maxBurst {
		return df7.Specf("burst sizes must be below 2^53")
	}
	if p.CIR == 0 {
		return df7.Specf("cir must be > 0")
	}
	if (p.Type == Type2R3C2698 || p.Type == Type2R3C4115 || p.Type == Type2R3CMef5CF1) && p.EIR < p.CIR {
		return df7.Specf("two-rate policer needs eir (%d) ≥ cir (%d)", p.EIR, p.CIR)
	}
	for _, a := range []struct {
		n string
		a Action
	}{{"conform", p.Conform}, {"exceed", p.Exceed}, {"violate", p.Violate}} {
		if err := a.a.validate(a.n); err != nil {
			return err
		}
	}
	return nil
}

func (p Policer) config() policer_types.PolicerConfig {
	act := func(a Action) policer_types.Sse2QosAction {
		return policer_types.Sse2QosAction{Type: actionTypes[a.Type], Dscp: a.DSCP}
	}
	return policer_types.PolicerConfig{
		Cir: p.CIR, Eir: p.EIR, Cb: p.CB, Eb: p.EB,
		RateType:      rateTypes[p.RateType],
		RoundType:     roundTypes[p.RoundType],
		Type:          policerTypes[p.Type],
		ColorAware:    p.ColorAware,
		ConformAction: act(p.Conform),
		ExceedAction:  act(p.Exceed),
		ViolateAction: act(p.Violate),
	}
}

// Directions of a policer attachment.
const (
	DirInput  = "input"
	DirOutput = "output"
)

// Attachment is the desired state of one policer.interface object: the policer that polices
// one direction of an interface (one policer per interface and direction).
type Attachment struct {
	Interface string `json:"interface,omitempty"`
	Direction string `json:"direction,omitempty"`
	Policer   string `json:"policer,omitempty"`
}

// Validate checks a.
func (a Attachment) Validate() error {
	if a.Interface == "" || a.Policer == "" {
		return df7.Specf("policer attachment needs interface and policer")
	}
	if a.Direction != DirInput && a.Direction != DirOutput {
		return df7.Specf("direction %q: want input or output", a.Direction)
	}
	return nil
}

// Bind is the desired state of one policer.bind object: the worker thread that handles all
// packets of a policer (VPP hands them off to it).
type Bind struct {
	Policer string `json:"policer,omitempty"`
	Worker  uint32 `json:"worker,omitempty"`
}

// Validate checks b.
func (b Bind) Validate() error {
	if b.Policer == "" {
		return df7.Specf("policer bind needs a policer")
	}
	return nil
}

// Classify is the desired state of one policer.classify object: the classify tables (DF-2
// classify.table names) whose sessions select the policer for packets of an interface.
type Classify struct {
	Interface string `json:"interface,omitempty"`
	IP4Table  string `json:"ip4_table,omitempty"`
	IP6Table  string `json:"ip6_table,omitempty"`
	L2Table   string `json:"l2_table,omitempty"`
}

// Validate checks c.
func (c Classify) Validate() error {
	if c.Interface == "" {
		return df7.Specf("policer classify needs an interface")
	}
	if c.IP4Table == "" && c.IP6Table == "" && c.L2Table == "" {
		return df7.Specf("policer classify on %q names no table; omit the object instead", c.Interface)
	}
	return nil
}
