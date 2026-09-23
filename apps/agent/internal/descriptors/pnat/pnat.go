// Package pnat holds the descriptors of VPP's policy 1:1 NAT plugin (binapi/pnat,
// pnat_plugin.so): bindings (match tuple → rewrite) and their attachment to an interface's
// input or output path. Object <-> message table: docs/agent/descriptors/pnat.md.
//
// VPP 26.06 specifics handled here (verified in src/plugins/nat/pnat):
//   - pnat_bindings_details carries no binding index. The index (needed to attach, detach
//     and delete) is recovered from pnat_bindings_get's cursor semantics: a get with cursor k
//     returns the details of every binding with index ≥ k, in index order.
//   - the flow hash is a bihash_16_8 without lazy instantiation: pnat_flow_lookup and
//     pnat_binding_detach on a VPP where no binding was ever attached dereference an
//     uninitialised table and crash VPP. The hash is initialised by the first attach and never
//     freed by a successful detach, so both messages are sent only while pnat_interfaces_get
//     lists at least one interface.
//   - pnat_binding_detach disables the whole attachment point of the interface, even when
//     other bindings remain attached there (enabled[] is not refcounted). Attachment Retrieve
//     therefore probes pnat_flow_lookup on both points and ignores enabled[].
package pnat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	pnatapi "ngfw/agent/binapi/pnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameBinding    = "pnat.binding"
	NameAttachment = "pnat.attachment"
)

// Attachment points.
const (
	PointInput  = "input"
	PointOutput = "output"
)

var (
	// ErrNoBinding is returned when an attachment's binding is not on the VPP.
	ErrNoBinding = errors.New("pnat: binding not found")
	// ErrFlowHashUninitialised is returned instead of sending a message that would crash
	// VPP 26.06 (no pnat interface exists, so the flow hash was never initialised).
	ErrFlowHashUninitialised = errors.New("pnat: flow hash not initialised on this VPP (no attachment exists); refusing a lookup/detach that would crash VPP 26.06")
)

// MatchSpec is the match tuple; empty addresses / zero ports / empty proto are wildcards
// (the mask is derived from what is set).
type MatchSpec struct {
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Proto   string `json:"proto"`
	SrcPort uint32 `json:"src_port"`
	DstPort uint32 `json:"dst_port"`
}

// RewriteSpec is the rewrite tuple; the mask is derived from what is set. CopyByte copies
// the byte at FromOffset to ToOffset, ClearByte clears the byte at ClearOffset.
type RewriteSpec struct {
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	SrcPort     uint32 `json:"src_port"`
	DstPort     uint32 `json:"dst_port"`
	CopyByte    bool   `json:"copy_byte"`
	FromOffset  uint32 `json:"from_offset"`
	ToOffset    uint32 `json:"to_offset"`
	ClearByte   bool   `json:"clear_byte"`
	ClearOffset uint32 `json:"clear_offset"`
}

// BindingSpec is one pnat binding (pnat_binding_add_v2). Its id is the canonical match tuple.
type BindingSpec struct {
	Match   MatchSpec   `json:"match"`
	Rewrite RewriteSpec `json:"rewrite"`
}

// Normalize canonicalises addresses and protocol.
func (s *BindingSpec) Normalize() {
	s.Match.Src, s.Match.Dst = natcommon.CanonAddr(s.Match.Src), natcommon.CanonAddr(s.Match.Dst)
	if s.Match.Proto != "" {
		s.Match.Proto = natcommon.CanonProto(s.Match.Proto)
	}
	s.Rewrite.Src, s.Rewrite.Dst = natcommon.CanonAddr(s.Rewrite.Src), natcommon.CanonAddr(s.Rewrite.Dst)
	if !s.Rewrite.CopyByte {
		s.Rewrite.FromOffset, s.Rewrite.ToOffset = 0, 0
	}
	if !s.Rewrite.ClearByte {
		s.Rewrite.ClearOffset = 0
	}
}

// ID renders the match tuple as the stable binding id:
// "<proto>/<src>/<sport>/<dst>/<dport>", "any" for wildcards.
func (m MatchSpec) ID() string {
	part := func(s string) string {
		if s == "" {
			return "any"
		}
		return s
	}
	port := func(p uint32) string {
		if p == 0 {
			return "any"
		}
		return strconv.FormatUint(uint64(p), 10)
	}
	return part(m.Proto) + "/" + part(m.Src) + "/" + port(m.SrcPort) + "/" + part(m.Dst) + "/" + port(m.DstPort)
}

// AttachmentSpec attaches the binding with id Binding (MatchSpec.ID) to an interface's input
// or output path (pnat_binding_attach).
type AttachmentSpec struct {
	Interface string `json:"interface"`
	Point     string `json:"point"`
	Binding   string `json:"binding"`
}

// BindingMeta is the binding index VPP assigned.
type BindingMeta struct{ Index uint32 }

// AttachMeta is the Meta of attachments.
type AttachMeta struct {
	SwIfIndex    uint32
	BindingIndex uint32
}

// BindingKey is the key of the binding with id.
func BindingKey(id string) scheduler.Key { return scheduler.Join(NameBinding, id) }

// Plugin bundles the client, the owner scope and the descriptors.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	svc    pnatapi.RPCService

	Binding    *natcommon.Descriptor[BindingSpec]
	Attachment *natcommon.Descriptor[AttachmentSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), svc: pnatapi.NewServiceClient(client)}
	p.Binding = p.newBinding()
	p.Attachment = p.newAttachment()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Binding, p.Attachment}
}

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string) *Plugin {
	p := New(client, owner)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

// ---- encoding -----------------------------------------------------------------------------

func optIP4(s string) (ip_types.IP4Address, bool, error) {
	if s == "" {
		return ip_types.IP4Address{}, false, nil
	}
	a, err := natcommon.IP4(s)
	return a, true, err
}

func port16(v uint32, what string) (uint16, error) {
	if v > 65535 {
		return 0, fmt.Errorf("pnat: %s %d out of range", what, v)
	}
	return uint16(v), nil
}

func u8(v uint32, what string) (uint8, error) {
	if v > 255 {
		return 0, fmt.Errorf("pnat: %s %d out of range", what, v)
	}
	return uint8(v), nil
}

func encodeMatch(m MatchSpec) (pnatapi.PnatMatchTuple, error) {
	var t pnatapi.PnatMatchTuple
	var err error
	var set bool
	if t.Src, set, err = optIP4(m.Src); err != nil {
		return t, fmt.Errorf("match.src: %w", err)
	} else if set {
		t.Mask |= pnatapi.PNAT_SA
	}
	if t.Dst, set, err = optIP4(m.Dst); err != nil {
		return t, fmt.Errorf("match.dst: %w", err)
	} else if set {
		t.Mask |= pnatapi.PNAT_DA
	}
	if m.Proto != "" {
		n, err := natcommon.ProtoNumber(m.Proto)
		if err != nil {
			return t, err
		}
		t.Proto, t.Mask = ip_types.IPProto(n), t.Mask|pnatapi.PNAT_PROTO
	}
	if m.SrcPort != 0 {
		if t.Sport, err = port16(m.SrcPort, "match.src_port"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_SPORT
	}
	if m.DstPort != 0 {
		if t.Dport, err = port16(m.DstPort, "match.dst_port"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_DPORT
	}
	if (m.SrcPort != 0 || m.DstPort != 0) && t.Proto != ip_types.IP_API_PROTO_TCP && t.Proto != ip_types.IP_API_PROTO_UDP {
		return t, fmt.Errorf("pnat: match ports need proto tcp or udp")
	}
	if t.Mask == 0 {
		return t, fmt.Errorf("pnat: empty match")
	}
	return t, nil
}

func encodeRewrite(r RewriteSpec) (pnatapi.PnatRewriteTuple, error) {
	var t pnatapi.PnatRewriteTuple
	var err error
	var set bool
	if t.Src, set, err = optIP4(r.Src); err != nil {
		return t, fmt.Errorf("rewrite.src: %w", err)
	} else if set {
		t.Mask |= pnatapi.PNAT_SA
	}
	if t.Dst, set, err = optIP4(r.Dst); err != nil {
		return t, fmt.Errorf("rewrite.dst: %w", err)
	} else if set {
		t.Mask |= pnatapi.PNAT_DA
	}
	if r.SrcPort != 0 {
		if t.Sport, err = port16(r.SrcPort, "rewrite.src_port"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_SPORT
	}
	if r.DstPort != 0 {
		if t.Dport, err = port16(r.DstPort, "rewrite.dst_port"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_DPORT
	}
	if r.CopyByte {
		if t.FromOffset, err = u8(r.FromOffset, "rewrite.from_offset"); err != nil {
			return t, err
		}
		if t.ToOffset, err = u8(r.ToOffset, "rewrite.to_offset"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_COPY_BYTE
	}
	if r.ClearByte {
		if t.ClearOffset, err = u8(r.ClearOffset, "rewrite.clear_offset"); err != nil {
			return t, err
		}
		t.Mask |= pnatapi.PNAT_CLEAR_BYTE
	}
	if t.Mask == 0 {
		return t, fmt.Errorf("pnat: empty rewrite")
	}
	return t, nil
}

func decode(d *pnatapi.PnatBindingsDetails) BindingSpec {
	var s BindingSpec
	m, r := d.Match, d.Rewrite
	if m.Mask&pnatapi.PNAT_SA != 0 {
		s.Match.Src = natcommon.IP4String(m.Src)
	}
	if m.Mask&pnatapi.PNAT_DA != 0 {
		s.Match.Dst = natcommon.IP4String(m.Dst)
	}
	if m.Mask&pnatapi.PNAT_PROTO != 0 {
		s.Match.Proto = natcommon.ProtoName(uint8(m.Proto))
	}
	if m.Mask&pnatapi.PNAT_SPORT != 0 {
		s.Match.SrcPort = uint32(m.Sport)
	}
	if m.Mask&pnatapi.PNAT_DPORT != 0 {
		s.Match.DstPort = uint32(m.Dport)
	}
	if r.Mask&pnatapi.PNAT_SA != 0 {
		s.Rewrite.Src = natcommon.IP4String(r.Src)
	}
	if r.Mask&pnatapi.PNAT_DA != 0 {
		s.Rewrite.Dst = natcommon.IP4String(r.Dst)
	}
	if r.Mask&pnatapi.PNAT_SPORT != 0 {
		s.Rewrite.SrcPort = uint32(r.Sport)
	}
	if r.Mask&pnatapi.PNAT_DPORT != 0 {
		s.Rewrite.DstPort = uint32(r.Dport)
	}
	if r.Mask&pnatapi.PNAT_COPY_BYTE != 0 {
		s.Rewrite.CopyByte, s.Rewrite.FromOffset, s.Rewrite.ToOffset = true, uint32(r.FromOffset), uint32(r.ToOffset)
	}
	if r.Mask&pnatapi.PNAT_CLEAR_BYTE != 0 {
		s.Rewrite.ClearByte, s.Rewrite.ClearOffset = true, uint32(r.ClearOffset)
	}
	s.Normalize()
	return s
}

func (p *Plugin) owns(s BindingSpec) bool {
	for _, a := range []string{s.Match.Src, s.Match.Dst, s.Rewrite.Src, s.Rewrite.Dst} {
		if a != "" && p.scope.OwnsAddrString(a) {
			return true
		}
	}
	return p.scope.All
}

// ---- binding enumeration (index recovery) -------------------------------------------------

// getFrom returns the details of every binding with index ≥ cursor, following EAGAIN
// continuations. cursor must stay below ^uint32(0) (VPP computes cursor+1 in u32).
func (p *Plugin) getFrom(ctx context.Context, cursor uint32) ([]*pnatapi.PnatBindingsDetails, error) {
	var out []*pnatapi.PnatBindingsDetails
	for {
		stream, err := p.svc.PnatBindingsGet(ctx, &pnatapi.PnatBindingsGet{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("pnat_bindings_get: %w", err)
		}
		next, done := uint32(0), false
		for !done {
			d, rep, err := stream.Recv()
			switch {
			case d != nil:
				out = append(out, d)
			case errors.Is(err, io.EOF):
				return out, nil
			case rep != nil && isRetval(err, api.INVALID_VALUE): // cursor past the last binding
				return out, nil
			case rep != nil && isRetval(err, api.EAGAIN):
				next, done = rep.Cursor, true
			case err != nil:
				return nil, fmt.Errorf("pnat_bindings_get: %w", err)
			}
		}
		if next == ^uint32(0) {
			return out, nil
		}
		cursor = next
	}
}

func isRetval(err error, want api.VPPApiError) bool {
	rv, ok := natcommon.Retval(err)
	return ok && rv == want
}

type binding struct {
	spec  BindingSpec
	index uint32
}

// bindings dumps every binding with its recovered pool index (unfiltered).
func (p *Plugin) bindings(ctx context.Context) ([]binding, error) {
	all, err := p.getFrom(ctx, 0)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	n := uint32(len(all)) //nolint:gosec // bounded by VPP's pool
	count := func(k uint32) (uint32, error) {
		d, err := p.getFrom(ctx, k)
		return uint32(len(d)), err //nolint:gosec // bounded by VPP's pool
	}
	idx := make([]uint32, n)
	// fast path: no holes in the pool → indices 0..n-1
	if c, err := count(n); err != nil {
		return nil, err
	} else if c == 0 {
		for i := range idx {
			idx[i] = uint32(i) //nolint:gosec // i < n
		}
	} else {
		// a_i = max{k : count(k) ≥ n-i}; count is non-increasing in k.
		lo := uint32(0)
		for i := uint32(0); i < n; i++ {
			need := n - i
			hi := lo + 1
			for {
				c, err := count(hi)
				if err != nil {
					return nil, err
				}
				if c < need {
					break
				}
				lo, hi = hi, hi*2+1
				if hi >= 1<<31 {
					return nil, fmt.Errorf("pnat: binding index recovery out of range")
				}
			}
			for hi-lo > 1 { // invariant: count(lo) ≥ need > count(hi)
				mid := lo + (hi-lo)/2
				c, err := count(mid)
				if err != nil {
					return nil, err
				}
				if c >= need {
					lo = mid
				} else {
					hi = mid
				}
			}
			idx[i] = lo
			lo++
		}
	}
	out := make([]binding, n)
	for i, d := range all {
		out[i] = binding{spec: decode(d), index: idx[i]}
	}
	return out, nil
}

func (p *Plugin) ownedBindings(ctx context.Context) ([]binding, error) {
	all, err := p.bindings(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, b := range all {
		if p.owns(b.spec) {
			out = append(out, b)
		}
	}
	return out, nil
}

// ---- binding descriptor -------------------------------------------------------------------

func (p *Plugin) newBinding() *natcommon.Descriptor[BindingSpec] {
	return natcommon.New(natcommon.Ops[BindingSpec]{
		Name: NameBinding,
		ID:   func(s BindingSpec) string { return s.Match.ID() },
		Create: func(ctx context.Context, s BindingSpec) (any, error) {
			m, err := encodeMatch(s.Match)
			if err != nil {
				return nil, err
			}
			r, err := encodeRewrite(s.Rewrite)
			if err != nil {
				return nil, err
			}
			rep, err := p.svc.PnatBindingAddV2(ctx, &pnatapi.PnatBindingAddV2{Match: m, Rewrite: r})
			if err != nil {
				return nil, fmt.Errorf("pnat_binding_add_v2: %w", err)
			}
			return BindingMeta{Index: rep.BindingIndex}, nil
		},
		Delete: func(ctx context.Context, _ BindingSpec, meta any) error {
			m, ok := meta.(BindingMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameBinding, meta)
			}
			if _, err := p.svc.PnatBindingDel(ctx, &pnatapi.PnatBindingDel{BindingIndex: m.Index}); err != nil {
				if rv, ok := natcommon.Retval(err); ok && rv == -1 { // already gone
					return nil
				}
				return fmt.Errorf("pnat_binding_del: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[BindingSpec], error) {
			bs, err := p.ownedBindings(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]natcommon.Item[BindingSpec], 0, len(bs))
			for _, b := range bs {
				out = append(out, natcommon.Item[BindingSpec]{Spec: b.spec, Meta: BindingMeta{Index: b.index}})
			}
			return out, nil
		},
	})
}

// ---- attachment descriptor ----------------------------------------------------------------

func pointValue(s string) (pnatapi.PnatAttachmentPoint, error) {
	switch s {
	case PointInput:
		return pnatapi.PNAT_IP4_INPUT, nil
	case PointOutput:
		return pnatapi.PNAT_IP4_OUTPUT, nil
	}
	return 0, fmt.Errorf("pnat: point must be %q or %q, got %q", PointInput, PointOutput, s)
}

// pnatInterfaces returns the sw_if_indexes pnat_interfaces_get lists; a non-empty result
// proves the flow hash is initialised.
func (p *Plugin) pnatInterfaces(ctx context.Context) ([]uint32, error) {
	stream, err := p.svc.PnatInterfacesGet(ctx, &pnatapi.PnatInterfacesGet{})
	if err != nil {
		return nil, fmt.Errorf("pnat_interfaces_get: %w", err)
	}
	var out []uint32
	for {
		d, rep, err := stream.Recv()
		switch {
		case d != nil:
			out = append(out, uint32(d.SwIfIndex))
		case errors.Is(err, io.EOF), rep != nil && isRetval(err, api.INVALID_VALUE):
			return out, nil
		case rep != nil && isRetval(err, api.EAGAIN):
			return nil, fmt.Errorf("pnat_interfaces_get: more than one reply batch of interfaces is not supported")
		case err != nil:
			return nil, fmt.Errorf("pnat_interfaces_get: %w", err)
		}
	}
}

func attachID(s AttachmentSpec) string { return s.Interface + "/" + s.Point + "/" + s.Binding }

func (p *Plugin) newAttachment() *natcommon.Descriptor[AttachmentSpec] {
	return natcommon.New(natcommon.Ops[AttachmentSpec]{
		Name: NameAttachment,
		ID:   attachID,
		Deps: func(s AttachmentSpec) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.Dep(BindingKey(s.Binding)), natcommon.InterfaceDep(s.Interface)}
		},
		Create: func(ctx context.Context, s AttachmentSpec) (any, error) {
			pt, err := pointValue(s.Point)
			if err != nil {
				return nil, err
			}
			idx, err := natcommon.ResolveInterface(ctx, p.client, s.Interface)
			if err != nil {
				return nil, err
			}
			bs, err := p.ownedBindings(ctx)
			if err != nil {
				return nil, err
			}
			for _, b := range bs {
				if b.spec.Match.ID() != s.Binding {
					continue
				}
				if _, err := p.svc.PnatBindingAttach(ctx, &pnatapi.PnatBindingAttach{SwIfIndex: idx, Attachment: pt, BindingIndex: b.index}); err != nil {
					return nil, fmt.Errorf("pnat_binding_attach: %w", err)
				}
				return AttachMeta{SwIfIndex: uint32(idx), BindingIndex: b.index}, nil
			}
			return nil, fmt.Errorf("%w: %q", ErrNoBinding, s.Binding)
		},
		Delete: func(ctx context.Context, s AttachmentSpec, meta any) error {
			m, ok := meta.(AttachMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameAttachment, meta)
			}
			pt, err := pointValue(s.Point)
			if err != nil {
				return err
			}
			if ifs, err := p.pnatInterfaces(ctx); err != nil {
				return err
			} else if len(ifs) == 0 {
				return ErrFlowHashUninitialised // nothing can be attached; detach would crash VPP
			}
			if _, err := p.svc.PnatBindingDetach(ctx, &pnatapi.PnatBindingDetach{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), Attachment: pt, BindingIndex: m.BindingIndex}); err != nil {
				if rv, ok := natcommon.Retval(err); ok && (rv == -1 || rv == -2) { // binding / flow already gone
					return nil
				}
				return fmt.Errorf("pnat_binding_detach: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[AttachmentSpec], error) {
			ifs, err := p.pnatInterfaces(ctx)
			if err != nil || len(ifs) == 0 {
				return nil, err // no pnat interface: nothing attached, flow hash may be uninitialised
			}
			table, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			bs, err := p.ownedBindings(ctx)
			if err != nil {
				return nil, err
			}
			var out []natcommon.Item[AttachmentSpec]
			for _, sw := range ifs {
				i, ok := table.ByIndex(sw)
				if !ok || !p.scope.OwnsInterface(i) {
					continue
				}
				for _, pt := range []string{PointInput, PointOutput} {
					v, _ := pointValue(pt)
					for _, b := range bs {
						m, err := encodeMatch(b.spec.Match)
						if err != nil {
							return nil, err
						}
						rep, err := p.svc.PnatFlowLookup(ctx, &pnatapi.PnatFlowLookup{SwIfIndex: interface_types.InterfaceIndex(sw), Attachment: v, Match: m})
						if err != nil {
							if rv, ok := natcommon.Retval(err); ok && rv == -1 { // not attached here
								continue
							}
							return nil, fmt.Errorf("pnat_flow_lookup: %w", err)
						}
						if rep.BindingIndex != b.index {
							continue
						}
						out = append(out, natcommon.Item[AttachmentSpec]{Spec: AttachmentSpec{Interface: i.Name, Point: pt, Binding: b.spec.Match.ID()},
							Meta: AttachMeta{SwIfIndex: sw, BindingIndex: b.index}})
					}
				}
			}
			sort.Slice(out, func(a, b int) bool { return attachID(out[a].Spec) < attachID(out[b].Spec) })
			return out, nil
		},
	})
}
