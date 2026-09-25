package objects

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// IP protocol numbers of the service protocols.
const (
	ProtoAny   uint8 = 0
	ProtoICMP  uint8 = 1
	ProtoTCP   uint8 = 6
	ProtoUDP   uint8 = 17
	ProtoICMP6 uint8 = 58
	ProtoSCTP  uint8 = 132
)

// PortSpec is one protocol/port match in the shape of an ACL rule (descriptors/acl.Rule): Proto
// is the IP protocol number (0 = any; ports are then irrelevant). For TCP, UDP and SCTP the port
// fields are inclusive ranges (0–65535 = any); for ICMP/ICMPv6 SrcPort* is the ICMP type range and
// DstPort* the ICMP code range (0–255 = any). TCPFlagsMask/Value: packet.flags & mask == value
// (0/0 = any; only on TCP entries).
type PortSpec struct {
	Proto         uint8
	SrcPortFirst  uint16
	SrcPortLast   uint16
	DstPortFirst  uint16
	DstPortLast   uint16
	TCPFlagsMask  uint8
	TCPFlagsValue uint8
}

func (p PortSpec) String() string {
	return fmt.Sprintf("proto %d src %d-%d dst %d-%d flags %#02x/%#02x", p.Proto, p.SrcPortFirst, p.SrcPortLast, p.DstPortFirst, p.DstPortLast, p.TCPFlagsValue, p.TCPFlagsMask)
}

func (p PortSpec) less(q PortSpec) bool {
	a := [...]int{int(p.Proto), int(p.DstPortFirst), int(p.DstPortLast), int(p.SrcPortFirst), int(p.SrcPortLast), int(p.TCPFlagsMask), int(p.TCPFlagsValue)}
	b := [...]int{int(q.Proto), int(q.DstPortFirst), int(q.DstPortLast), int(q.SrcPortFirst), int(q.SrcPortLast), int(q.TCPFlagsMask), int(q.TCPFlagsValue)}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// canonPorts sorts and deduplicates.
func canonPorts(ps []PortSpec) []PortSpec {
	sort.Slice(ps, func(i, j int) bool { return ps[i].less(ps[j]) })
	out := ps[:0]
	for i, p := range ps {
		if i == 0 || p != out[len(out)-1] {
			out = append(out, p)
		}
	}
	return out
}

type span struct{ first, last uint16 }

var anyPort = []span{{0, 65535}}

func portSpans(list []string) ([]span, error) {
	if len(list) == 0 {
		return anyPort, nil
	}
	out := make([]span, 0, len(list))
	for _, s := range list {
		from, to, ok := strings.Cut(s, "-")
		if !ok {
			to = from
		}
		a, err1 := strconv.ParseUint(from, 10, 16)
		b, err2 := strconv.ParseUint(to, 10, 16)
		if err1 != nil || err2 != nil || a < 1 || a > b {
			return nil, fmt.Errorf("%w: port range %q", ErrInvalid, s)
		}
		out = append(out, span{uint16(a), uint16(b)})
	}
	return out, nil
}

// ExpandServiceSpec turns one protocol/port specification (an ACL rule's inline service, or a
// service object's fields) into PortSpecs: one per protocol × source range × destination range;
// tcp-udp yields TCP and UDP entries (TCP flags only on the TCP ones). Sorted, deduplicated.
func ExpandServiceSpec(spec *vrxv1.ServiceSpec) ([]PortSpec, error) {
	var out []PortSpec
	ports := func(proto uint8, flags bool) error {
		src, err := portSpans(spec.GetSourcePorts())
		if err != nil {
			return err
		}
		dst, err := portSpans(spec.GetDestinationPorts())
		if err != nil {
			return err
		}
		var mask, value uint8
		if flags && spec.GetTcpFlags() != nil {
			m, v := spec.GetTcpFlags().GetMask(), spec.GetTcpFlags().GetValue()
			if m > 255 || v > 255 || v&^m != 0 {
				return fmt.Errorf("%w: TCP flags value %#x outside mask %#x", ErrInvalid, v, m)
			}
			mask, value = uint8(m), uint8(v)
		}
		for _, s := range src {
			for _, d := range dst {
				out = append(out, PortSpec{Proto: proto, SrcPortFirst: s.first, SrcPortLast: s.last, DstPortFirst: d.first, DstPortLast: d.last, TCPFlagsMask: mask, TCPFlagsValue: value})
			}
		}
		return nil
	}
	icmp := func(proto uint8) error {
		t, c := span{0, 255}, span{0, 255}
		if spec.Type != nil {
			if spec.GetType() > 255 {
				return fmt.Errorf("%w: ICMP type %d", ErrInvalid, spec.GetType())
			}
			t = span{uint16(spec.GetType()), uint16(spec.GetType())} //nolint:gosec // ≤ 255, checked above
		}
		if spec.Code != nil {
			if spec.Type == nil || spec.GetCode() > 255 {
				return fmt.Errorf("%w: ICMP code %d (requires a type)", ErrInvalid, spec.GetCode())
			}
			c = span{uint16(spec.GetCode()), uint16(spec.GetCode())} //nolint:gosec // ≤ 255, checked above
		}
		out = append(out, PortSpec{Proto: proto, SrcPortFirst: t.first, SrcPortLast: t.last, DstPortFirst: c.first, DstPortLast: c.last})
		return nil
	}
	var err error
	switch spec.GetProtocol() {
	case "tcp":
		err = ports(ProtoTCP, true)
	case "udp":
		err = ports(ProtoUDP, false)
	case "sctp":
		err = ports(ProtoSCTP, false)
	case "tcp-udp":
		if err = ports(ProtoTCP, true); err == nil {
			err = ports(ProtoUDP, false)
		}
	case "icmp":
		err = icmp(ProtoICMP)
	case "icmp6":
		err = icmp(ProtoICMP6)
	case "any":
		out = append(out, PortSpec{Proto: ProtoAny, SrcPortLast: 65535, DstPortLast: 65535})
	case "other":
		if spec.Number == nil || spec.GetNumber() > 255 {
			return nil, fmt.Errorf("%w: protocol \"other\" needs a number 0–255", ErrInvalid)
		}
		out = append(out, PortSpec{Proto: uint8(spec.GetNumber()), SrcPortLast: 65535, DstPortLast: 65535}) //nolint:gosec // ≤ 255, checked above
	default:
		return nil, fmt.Errorf("%w: unknown protocol %q", ErrInvalid, spec.GetProtocol())
	}
	if err != nil {
		return nil, err
	}
	return canonPorts(out), nil
}

// ServiceSpecOf is the protocol/port part of a service object.
func ServiceSpecOf(o *vrxv1.ServiceObject) *vrxv1.ServiceSpec {
	return &vrxv1.ServiceSpec{
		Protocol: o.Protocol, DestinationPorts: o.GetDestinationPorts(), SourcePorts: o.GetSourcePorts(),
		TcpFlags: o.GetTcpFlags(), Type: o.Type, Code: o.Code, Number: o.Number,
	}
}

// ExpandService turns the service object or service group ref of doc into PortSpecs (the union of
// a group's members, recursively; sorted, deduplicated). Errors as for Expand.
func ExpandService(doc *vrxv1.ObjectsConfig, ref string, opts ...Option) ([]PortSpec, error) {
	e := &svcExpander{doc: doc, o: optionsOf(opts), memo: map[string][]PortSpec{}, visiting: map[string]bool{}}
	return e.expand(ref)
}

type svcExpander struct {
	doc      *vrxv1.ObjectsConfig
	o        options
	memo     map[string][]PortSpec
	visiting map[string]bool
	stack    []string
}

func (e *svcExpander) expand(ref string) ([]PortSpec, error) {
	if s, ok := e.doc.GetServices()[ref]; ok {
		ps, err := ExpandServiceSpec(ServiceSpecOf(s))
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", ref, err)
		}
		if len(ps) > e.o.limit {
			return nil, &LimitError{Ref: ref, Count: len(ps), Limit: e.o.limit}
		}
		return ps, nil
	}
	g, ok := e.doc.GetServiceGroups()[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %q is neither a service nor a service group", ErrUnknownObject, ref)
	}
	if m, ok := e.memo[ref]; ok {
		return m, nil
	}
	if e.visiting[ref] {
		return nil, fmt.Errorf("%w: %s", ErrCycle, cyclePath(e.stack, ref))
	}
	e.visiting[ref] = true
	e.stack = append(e.stack, ref)
	var out []PortSpec
	for _, m := range g.GetMembers() {
		ps, err := e.expand(m)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	out = canonPorts(out)
	if len(out) > e.o.limit {
		return nil, &LimitError{Ref: ref, Count: len(out), Limit: e.o.limit}
	}
	e.stack = e.stack[:len(e.stack)-1]
	delete(e.visiting, ref)
	e.memo[ref] = out
	return out, nil
}
