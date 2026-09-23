package acl

import (
	"errors"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// Descriptor names (scheduler.Descriptor.Name); the first segment of every key.
const (
	NameACL                   = "acl.acl"
	NameMacipACL              = "acl.macip-acl"
	NameInterfaceBinding      = "acl.interface-binding"
	NameEtypeWhitelist        = "acl.etype-whitelist"
	NameMacipInterfaceBinding = "acl.macip-interface-binding"
	NameStatsEnable           = "acl.stats-enable"
)

// Action is what an ACL rule does with a matching packet.
type Action string

// Rule actions. ActionReflect is permit + create a session so the return flow is permitted
// (stateful ACL); it is invalid in MACIP rules.
const (
	ActionDeny    Action = "deny"
	ActionPermit  Action = "permit"
	ActionReflect Action = "reflect"
)

// Wildcard prefixes for ACL rules ("match any address" of one family). A rule's src and dst
// must be of the same family; VPP rejects mixed rules.
const (
	AnyV4 = "0.0.0.0/0"
	AnyV6 = "::/0"
)

// Field names of the structpb documents. Desired values must be built with the Proto()
// methods below (every field present, canonical text) so that proto.Equal against Retrieve is
// a correct diff.
const (
	FieldName          = "name"
	FieldRules         = "rules"
	FieldAction        = "action"
	FieldSrc           = "src"
	FieldDst           = "dst"
	FieldProto         = "proto"
	FieldSrcPortFirst  = "src_port_first"
	FieldSrcPortLast   = "src_port_last"
	FieldDstPortFirst  = "dst_port_first"
	FieldDstPortLast   = "dst_port_last"
	FieldTCPFlagsMask  = "tcp_flags_mask"
	FieldTCPFlagsValue = "tcp_flags_value"
	FieldSrcMac        = "src_mac"
	FieldSrcMacMask    = "src_mac_mask"
	FieldInterface     = "interface"
	FieldInput         = "input"
	FieldOutput        = "output"
	FieldACL           = "acl"
	FieldEnabled       = "enabled"
)

// ErrSpec is wrapped by every validation error of the specs below.
var ErrSpec = errors.New("acl: invalid spec")

// Rule is one L3/L4 ACL rule, in the order VPP evaluates it (acl_types.acl_rule).
//
// Src and Dst are canonical CIDR prefixes (netip.Prefix.Masked().String()), both IPv4 or both
// IPv6; use AnyV4/AnyV6 for "any". Proto is the IP protocol number, 0 = any protocol (ports are
// then ignored by VPP). For TCP/UDP the port fields are inclusive ranges (0–65535 = any); for
// ICMP/ICMPv6 SrcPort* is the ICMP type range and DstPort* the ICMP code range (0–255 = any).
// TCPFlagsMask/Value match TCP flags (packet.flags & mask == value); 0/0 = any.
type Rule struct {
	Action        Action
	Src           string
	Dst           string
	Proto         uint8
	SrcPortFirst  uint16
	SrcPortLast   uint16
	DstPortFirst  uint16
	DstPortLast   uint16
	TCPFlagsMask  uint8
	TCPFlagsValue uint8
}

// ACL is the desired state of one acl.acl object: a named, ordered rule list. Name is the
// object id (key acl.acl/<name>) and, prefixed with the owner, the VPP tag.
type ACL struct {
	Name  string
	Rules []Rule
}

// MacipRule is one L2 MACIP rule (acl_types.macip_acl_rule): permit/deny frames whose source MAC
// matches SrcMac under SrcMacMask and whose source IP is inside SrcPrefix.
type MacipRule struct {
	Action     Action
	SrcMac     string // canonical lower-case "aa:bb:cc:dd:ee:ff"
	SrcMacMask string // same form; "ff:ff:ff:ff:ff:ff" = exact match
	SrcPrefix  string // canonical CIDR prefix
}

// MacipACL is the desired state of one acl.macip-acl object.
type MacipACL struct {
	Name  string
	Rules []MacipRule
}

// InterfaceBinding is the desired state of one acl.interface-binding object: the complete
// ordered ACL lists (by acl.acl name) applied to one interface, inbound and outbound. Both lists
// empty is equivalent to no binding; Delete sets empty lists.
type InterfaceBinding struct {
	Interface string // VPP interface name, e.g. "loop1040"
	Input     []string
	Output    []string
}

// EtypeWhitelist is the desired state of one acl.etype-whitelist object: the non-IP ethertypes
// the ACL plugin lets through on an interface with ACLs applied, inbound and outbound. Lists
// must be strictly ascending (canonical form).
type EtypeWhitelist struct {
	Interface string
	Input     []uint16
	Output    []uint16
}

// MacipBinding is the desired state of one acl.macip-interface-binding object: the single MACIP
// ACL (by acl.macip-acl name) applied inbound on one interface.
type MacipBinding struct {
	Interface string
	ACL       string
}

// StatsEnable is the desired state of the acl.stats-enable singleton: whether the data plane
// increments the per-rule hit counters (/acl/<index>/matches) in the stats segment.
type StatsEnable struct {
	Enabled bool
}

// StatsEnableID is the object id of the singleton (key acl.stats-enable/global).
const StatsEnableID = "global"

// ---- structpb encoding -----------------------------------------------------------------------

func mustStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(fmt.Sprintf("acl: structpb encoding: %v", err)) // only for unsupported Go types: a programming error
	}
	return s
}

func stringList(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func uint16List(in []uint16) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

func (r Rule) toMap() map[string]any {
	return map[string]any{
		FieldAction:        string(r.Action),
		FieldSrc:           r.Src,
		FieldDst:           r.Dst,
		FieldProto:         float64(r.Proto),
		FieldSrcPortFirst:  float64(r.SrcPortFirst),
		FieldSrcPortLast:   float64(r.SrcPortLast),
		FieldDstPortFirst:  float64(r.DstPortFirst),
		FieldDstPortLast:   float64(r.DstPortLast),
		FieldTCPFlagsMask:  float64(r.TCPFlagsMask),
		FieldTCPFlagsValue: float64(r.TCPFlagsValue),
	}
}

// Proto returns the canonical structpb document of a.
func (a ACL) Proto() *structpb.Struct {
	rules := make([]any, len(a.Rules))
	for i, r := range a.Rules {
		rules[i] = r.toMap()
	}
	return mustStruct(map[string]any{FieldName: a.Name, FieldRules: rules})
}

// Proto returns the canonical structpb document of a.
func (a MacipACL) Proto() *structpb.Struct {
	rules := make([]any, len(a.Rules))
	for i, r := range a.Rules {
		rules[i] = map[string]any{
			FieldAction:     string(r.Action),
			FieldSrcMac:     r.SrcMac,
			FieldSrcMacMask: r.SrcMacMask,
			FieldSrc:        r.SrcPrefix,
		}
	}
	return mustStruct(map[string]any{FieldName: a.Name, FieldRules: rules})
}

// Proto returns the canonical structpb document of b.
func (b InterfaceBinding) Proto() *structpb.Struct {
	return mustStruct(map[string]any{
		FieldInterface: b.Interface,
		FieldInput:     stringList(b.Input),
		FieldOutput:    stringList(b.Output),
	})
}

// Proto returns the canonical structpb document of w.
func (w EtypeWhitelist) Proto() *structpb.Struct {
	return mustStruct(map[string]any{
		FieldInterface: w.Interface,
		FieldInput:     uint16List(w.Input),
		FieldOutput:    uint16List(w.Output),
	})
}

// Proto returns the canonical structpb document of b.
func (b MacipBinding) Proto() *structpb.Struct {
	return mustStruct(map[string]any{FieldInterface: b.Interface, FieldACL: b.ACL})
}

// Proto returns the canonical structpb document of s.
func (s StatsEnable) Proto() *structpb.Struct {
	return mustStruct(map[string]any{FieldEnabled: s.Enabled})
}

// ---- structpb decoding -----------------------------------------------------------------------

// fields is a tolerant reader over a structpb.Struct: missing fields read as zero values, fields
// of the wrong kind are an error.
type fields struct {
	m   map[string]*structpb.Value
	err error
}

func newFields(msg proto.Message) (*fields, error) {
	s, ok := msg.(*structpb.Struct)
	if !ok || s == nil {
		return nil, fmt.Errorf("%w: value is %T, want *structpb.Struct", ErrSpec, msg)
	}
	return &fields{m: s.GetFields()}, nil
}

func (f *fields) fail(name, want string, v *structpb.Value) {
	if f.err == nil {
		f.err = fmt.Errorf("%w: field %q is %T, want %s", ErrSpec, name, v.GetKind(), want)
	}
}

func (f *fields) str(name string) string {
	v, ok := f.m[name]
	if !ok || v == nil {
		return ""
	}
	if _, isStr := v.GetKind().(*structpb.Value_StringValue); !isStr {
		f.fail(name, "string", v)
		return ""
	}
	return v.GetStringValue()
}

func (f *fields) boolean(name string) bool {
	v, ok := f.m[name]
	if !ok || v == nil {
		return false
	}
	if _, isBool := v.GetKind().(*structpb.Value_BoolValue); !isBool {
		f.fail(name, "bool", v)
		return false
	}
	return v.GetBoolValue()
}

// number reads an unsigned integer field bounded by max.
func (f *fields) number(name string, maxValue uint64) uint64 {
	v, ok := f.m[name]
	if !ok || v == nil {
		return 0
	}
	if _, isNum := v.GetKind().(*structpb.Value_NumberValue); !isNum {
		f.fail(name, "number", v)
		return 0
	}
	n := v.GetNumberValue()
	if n < 0 || n > float64(maxValue) || n != math.Trunc(n) {
		if f.err == nil {
			f.err = fmt.Errorf("%w: field %q = %v is not an integer in 0..%d", ErrSpec, name, n, maxValue)
		}
		return 0
	}
	return uint64(n)
}

// u8 and u16 read bounded integer fields; number has already rejected anything out of range.
func (f *fields) u8(name string) uint8   { return uint8(f.number(name, math.MaxUint8)) }   //nolint:gosec // bounded by number
func (f *fields) u16(name string) uint16 { return uint16(f.number(name, math.MaxUint16)) } //nolint:gosec // bounded by number

func (f *fields) list(name string) []*structpb.Value {
	v, ok := f.m[name]
	if !ok || v == nil {
		return nil
	}
	if _, isList := v.GetKind().(*structpb.Value_ListValue); !isList {
		f.fail(name, "list", v)
		return nil
	}
	return v.GetListValue().GetValues()
}

func (f *fields) strings(name string) []string {
	vals := f.list(name)
	out := make([]string, 0, len(vals))
	for i, v := range vals {
		if _, isStr := v.GetKind().(*structpb.Value_StringValue); !isStr {
			f.fail(fmt.Sprintf("%s[%d]", name, i), "string", v)
			return nil
		}
		out = append(out, v.GetStringValue())
	}
	return out
}

func (f *fields) uint16s(name string) []uint16 {
	vals := f.list(name)
	out := make([]uint16, 0, len(vals))
	for i, v := range vals {
		n, isNum := v.GetKind().(*structpb.Value_NumberValue)
		if !isNum || n.NumberValue < 0 || n.NumberValue > math.MaxUint16 || n.NumberValue != math.Trunc(n.NumberValue) {
			f.fail(fmt.Sprintf("%s[%d]", name, i), "integer 0..65535", v)
			return nil
		}
		out = append(out, uint16(n.NumberValue))
	}
	return out
}

func (f *fields) structs(name string) []*fields {
	vals := f.list(name)
	out := make([]*fields, 0, len(vals))
	for i, v := range vals {
		s, isStruct := v.GetKind().(*structpb.Value_StructValue)
		if !isStruct {
			f.fail(fmt.Sprintf("%s[%d]", name, i), "object", v)
			return nil
		}
		out = append(out, &fields{m: s.StructValue.GetFields()})
	}
	return out
}

// FromProto decodes an ACL document. It does not validate; call Validate.
func FromProto(msg proto.Message) (ACL, error) {
	f, err := newFields(msg)
	if err != nil {
		return ACL{}, err
	}
	a := ACL{Name: f.str(FieldName)}
	for _, rf := range f.structs(FieldRules) {
		a.Rules = append(a.Rules, Rule{
			Action:        Action(rf.str(FieldAction)),
			Src:           rf.str(FieldSrc),
			Dst:           rf.str(FieldDst),
			Proto:         rf.u8(FieldProto),
			SrcPortFirst:  rf.u16(FieldSrcPortFirst),
			SrcPortLast:   rf.u16(FieldSrcPortLast),
			DstPortFirst:  rf.u16(FieldDstPortFirst),
			DstPortLast:   rf.u16(FieldDstPortLast),
			TCPFlagsMask:  rf.u8(FieldTCPFlagsMask),
			TCPFlagsValue: rf.u8(FieldTCPFlagsValue),
		})
		if rf.err != nil {
			return ACL{}, rf.err
		}
	}
	return a, f.err
}

// MacipACLFromProto decodes a MacipACL document. It does not validate; call Validate.
func MacipACLFromProto(msg proto.Message) (MacipACL, error) {
	f, err := newFields(msg)
	if err != nil {
		return MacipACL{}, err
	}
	a := MacipACL{Name: f.str(FieldName)}
	for _, rf := range f.structs(FieldRules) {
		a.Rules = append(a.Rules, MacipRule{
			Action:     Action(rf.str(FieldAction)),
			SrcMac:     rf.str(FieldSrcMac),
			SrcMacMask: rf.str(FieldSrcMacMask),
			SrcPrefix:  rf.str(FieldSrc),
		})
		if rf.err != nil {
			return MacipACL{}, rf.err
		}
	}
	return a, f.err
}

// InterfaceBindingFromProto decodes an InterfaceBinding document.
func InterfaceBindingFromProto(msg proto.Message) (InterfaceBinding, error) {
	f, err := newFields(msg)
	if err != nil {
		return InterfaceBinding{}, err
	}
	b := InterfaceBinding{Interface: f.str(FieldInterface), Input: f.strings(FieldInput), Output: f.strings(FieldOutput)}
	return b, f.err
}

// EtypeWhitelistFromProto decodes an EtypeWhitelist document.
func EtypeWhitelistFromProto(msg proto.Message) (EtypeWhitelist, error) {
	f, err := newFields(msg)
	if err != nil {
		return EtypeWhitelist{}, err
	}
	w := EtypeWhitelist{Interface: f.str(FieldInterface), Input: f.uint16s(FieldInput), Output: f.uint16s(FieldOutput)}
	return w, f.err
}

// MacipBindingFromProto decodes a MacipBinding document.
func MacipBindingFromProto(msg proto.Message) (MacipBinding, error) {
	f, err := newFields(msg)
	if err != nil {
		return MacipBinding{}, err
	}
	b := MacipBinding{Interface: f.str(FieldInterface), ACL: f.str(FieldACL)}
	return b, f.err
}

// StatsEnableFromProto decodes a StatsEnable document.
func StatsEnableFromProto(msg proto.Message) (StatsEnable, error) {
	f, err := newFields(msg)
	if err != nil {
		return StatsEnable{}, err
	}
	return StatsEnable{Enabled: f.boolean(FieldEnabled)}, f.err
}

// ---- validation -------------------------------------------------------------------------------

// maxInterfaceList is the largest ACL list / ethertype list one interface takes (u8 count on the
// wire, acl_interface_set_acl_list.count and acl_interface_set_etype_whitelist.count).
const maxInterfaceList = math.MaxUint8

func validateName(what, name string) error {
	if name == "" {
		return fmt.Errorf("%w: %s name is empty", ErrSpec, what)
	}
	return nil
}

// Validate checks a for what VPP would reject (acl_add_replace): empty name, non-canonical or
// mixed-family prefixes, port ranges with first > last, unknown actions.
func (a ACL) Validate() error {
	if err := validateName("acl", a.Name); err != nil {
		return err
	}
	for i, r := range a.Rules {
		if err := r.validate(); err != nil {
			return fmt.Errorf("acl %q rule %d: %w", a.Name, i, err)
		}
	}
	return nil
}

func (r Rule) validate() error {
	if _, err := actionToAPI(r.Action); err != nil {
		return err
	}
	src, err := parsePrefix(r.Src)
	if err != nil {
		return fmt.Errorf("src: %w", err)
	}
	dst, err := parsePrefix(r.Dst)
	if err != nil {
		return fmt.Errorf("dst: %w", err)
	}
	if src.Addr().Is4() != dst.Addr().Is4() {
		return fmt.Errorf("%w: src %s and dst %s are not the same address family", ErrSpec, r.Src, r.Dst)
	}
	if r.SrcPortFirst > r.SrcPortLast {
		return fmt.Errorf("%w: src port/type range %d-%d has first > last", ErrSpec, r.SrcPortFirst, r.SrcPortLast)
	}
	if r.DstPortFirst > r.DstPortLast {
		return fmt.Errorf("%w: dst port/code range %d-%d has first > last", ErrSpec, r.DstPortFirst, r.DstPortLast)
	}
	return nil
}

// Validate checks a for what VPP would reject (macip_acl_add_replace) plus the reflect action,
// which has no meaning for L2 rules.
func (a MacipACL) Validate() error {
	if err := validateName("macip-acl", a.Name); err != nil {
		return err
	}
	for i, r := range a.Rules {
		if err := r.validate(); err != nil {
			return fmt.Errorf("macip-acl %q rule %d: %w", a.Name, i, err)
		}
	}
	return nil
}

func (r MacipRule) validate() error {
	if r.Action != ActionDeny && r.Action != ActionPermit {
		return fmt.Errorf("%w: macip action %q must be %q or %q", ErrSpec, r.Action, ActionDeny, ActionPermit)
	}
	if _, err := parseMAC(r.SrcMac); err != nil {
		return fmt.Errorf("src_mac: %w", err)
	}
	if _, err := parseMAC(r.SrcMacMask); err != nil {
		return fmt.Errorf("src_mac_mask: %w", err)
	}
	if _, err := parsePrefix(r.SrcPrefix); err != nil {
		return fmt.Errorf("src: %w", err)
	}
	return nil
}

func validateACLList(direction string, names []string) error {
	if len(names) > maxInterfaceList {
		return fmt.Errorf("%w: %s list has %d ACLs, VPP takes at most %d", ErrSpec, direction, len(names), maxInterfaceList)
	}
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n == "" {
			return fmt.Errorf("%w: %s list contains an empty ACL name", ErrSpec, direction)
		}
		if _, dup := seen[n]; dup {
			return fmt.Errorf("%w: ACL %q listed twice in %s (VPP: entry already exists)", ErrSpec, n, direction)
		}
		seen[n] = struct{}{}
	}
	return nil
}

// Validate checks b: interface name present, no duplicate ACL per direction, ≤ 255 per direction.
func (b InterfaceBinding) Validate() error {
	if err := validateName("interface", b.Interface); err != nil {
		return err
	}
	if err := validateACLList("input", b.Input); err != nil {
		return err
	}
	if err := validateACLList("output", b.Output); err != nil {
		return err
	}
	if len(b.Input)+len(b.Output) > maxInterfaceList {
		return fmt.Errorf("%w: %d ACLs on one interface, VPP takes at most %d", ErrSpec, len(b.Input)+len(b.Output), maxInterfaceList)
	}
	return nil
}

func validateEtypes(direction string, list []uint16) error {
	for i := 1; i < len(list); i++ {
		if list[i] <= list[i-1] {
			return fmt.Errorf("%w: %s ethertypes must be strictly ascending (canonical form), got %#04x after %#04x",
				ErrSpec, direction, list[i], list[i-1])
		}
	}
	return nil
}

// Validate checks w: interface name present, lists strictly ascending, ≤ 255 in total.
func (w EtypeWhitelist) Validate() error {
	if err := validateName("interface", w.Interface); err != nil {
		return err
	}
	if err := validateEtypes("input", w.Input); err != nil {
		return err
	}
	if err := validateEtypes("output", w.Output); err != nil {
		return err
	}
	if len(w.Input)+len(w.Output) > maxInterfaceList {
		return fmt.Errorf("%w: %d ethertypes on one interface, VPP takes at most %d", ErrSpec, len(w.Input)+len(w.Output), maxInterfaceList)
	}
	return nil
}

// Validate checks b: interface and ACL names present.
func (b MacipBinding) Validate() error {
	if err := validateName("interface", b.Interface); err != nil {
		return err
	}
	return validateName("macip-acl", b.ACL)
}
