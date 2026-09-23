package nat44ed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// noInterface is VPP's "no sw_if_index".
const noInterface = ^interface_types.InterfaceIndex(0)

// Endpoint is an ip:port pair; Port 0 with AddrOnly. External endpoints may name an
// interface instead of an address (VPP resolves the interface's first address).
type Endpoint struct {
	IP        string `json:"ip"`
	Port      uint32 `json:"port"`
	Interface string `json:"interface"`
}

// StaticMappingSpec is one nat44-ed static mapping (nat44_add_del_static_mapping_v2). Name
// is the stable id and is written into VPP's tag as "<owner>:<name>"; Retrieve keeps only
// mappings whose tag parses that way. VPP 26.06 does not report match_pool /
// pool_ip_address in nat44_static_mapping_details, so pool pinning is not modelled (see
// docs/agent/descriptors/nat44-ed.md).
type StaticMappingSpec struct {
	Name         string   `json:"name"`
	Local        Endpoint `json:"local"`
	External     Endpoint `json:"external"`
	Protocol     string   `json:"protocol"`
	VRF          uint32   `json:"vrf"`
	AddrOnly     bool     `json:"addr_only"`
	TwiceNAT     bool     `json:"twice_nat"`
	SelfTwiceNAT bool     `json:"self_twice_nat"`
	Out2InOnly   bool     `json:"out2in_only"`
}

// Normalize canonicalises addresses and protocol and drops what VPP does not keep for
// address-only mappings (ports, protocol) or interface-bound externals (the address).
func (s *StaticMappingSpec) Normalize() {
	s.Local.IP, s.External.IP = natcommon.CanonAddr(s.Local.IP), natcommon.CanonAddr(s.External.IP)
	s.Protocol = natcommon.CanonProto(s.Protocol)
	if s.AddrOnly {
		s.Local.Port, s.External.Port, s.Protocol = 0, 0, "any"
	}
	if s.External.Interface != "" {
		s.External.IP = ""
	}
	s.Local.Interface = ""
}

// IdentityMappingSpec maps an address (or an interface's address) to itself
// (nat44_add_del_identity_mapping).
type IdentityMappingSpec struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	Protocol  string `json:"protocol"`
	Port      uint32 `json:"port"`
	VRF       uint32 `json:"vrf"`
	AddrOnly  bool   `json:"addr_only"`
}

// Normalize canonicalises the identity mapping.
func (s *IdentityMappingSpec) Normalize() {
	s.IP = natcommon.CanonAddr(s.IP)
	s.Protocol = natcommon.CanonProto(s.Protocol)
	if s.AddrOnly {
		s.Port, s.Protocol = 0, "any"
	}
	if s.Interface != "" {
		s.IP = ""
	}
}

// LBLocal is one backend of a load-balanced static mapping.
type LBLocal struct {
	IP          string `json:"ip"`
	Port        uint32 `json:"port"`
	Probability uint32 `json:"probability"`
	VRF         uint32 `json:"vrf"`
}

// LBStaticMappingSpec is a load-balanced static mapping (nat44_add_del_lb_static_mapping);
// backend changes are applied in place with nat44_lb_static_mapping_add_del_local.
type LBStaticMappingSpec struct {
	Name         string    `json:"name"`
	External     Endpoint  `json:"external"`
	Protocol     string    `json:"protocol"`
	Affinity     uint32    `json:"affinity"`
	TwiceNAT     bool      `json:"twice_nat"`
	SelfTwiceNAT bool      `json:"self_twice_nat"`
	Out2InOnly   bool      `json:"out2in_only"`
	Locals       []LBLocal `json:"locals"`
}

// Normalize canonicalises and sorts the backends.
func (s *LBStaticMappingSpec) Normalize() {
	s.External.IP, s.External.Interface = natcommon.CanonAddr(s.External.IP), ""
	s.Protocol = natcommon.CanonProto(s.Protocol)
	for i := range s.Locals {
		s.Locals[i].IP = natcommon.CanonAddr(s.Locals[i].IP)
	}
	if s.Locals == nil {
		s.Locals = []LBLocal{}
	}
	sort.Slice(s.Locals, func(i, j int) bool { return lbLess(s.Locals[i], s.Locals[j]) })
}

func lbLess(a, b LBLocal) bool {
	if a.VRF != b.VRF {
		return a.VRF < b.VRF
	}
	if a.IP != b.IP {
		return a.IP < b.IP
	}
	return a.Port < b.Port
}

func mappingFlags(addrOnly, twice, selfTwice, out2in bool) nat_types.NatConfigFlags {
	var f nat_types.NatConfigFlags
	if addrOnly {
		f |= nat_types.NAT_IS_ADDR_ONLY
	}
	if twice {
		f |= nat_types.NAT_IS_TWICE_NAT
	}
	if selfTwice {
		f |= nat_types.NAT_IS_SELF_TWICE_NAT
	}
	if out2in {
		f |= nat_types.NAT_IS_OUT2IN_ONLY
	}
	return f
}

func port(p uint32) (uint16, error) {
	if p > 65535 {
		return 0, fmt.Errorf("nat44-ed: port %d out of range", p)
	}
	return uint16(p), nil
}

func ip4OrZero(s string) (ip_types.IP4Address, error) {
	if s == "" {
		return ip_types.IP4Address{}, nil
	}
	return natcommon.IP4(s)
}

func (p *Plugin) staticMappingRequest(ctx context.Context, s StaticMappingSpec, add bool, extIdx interface_types.InterfaceIndex) (*nat44_ed.Nat44AddDelStaticMappingV2, error) {
	tag, err := p.scope.Tag(s.Name)
	if err != nil {
		return nil, err
	}
	local, err := natcommon.IP4(s.Local.IP)
	if err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}
	ext, err := ip4OrZero(s.External.IP)
	if err != nil {
		return nil, fmt.Errorf("external: %w", err)
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	lp, err := port(s.Local.Port)
	if err != nil {
		return nil, err
	}
	ep, err := port(s.External.Port)
	if err != nil {
		return nil, err
	}
	if s.External.Interface != "" && extIdx == noInterface {
		extIdx, err = natcommon.ResolveOwned(ctx, p.client, p.scope, s.External.Interface)
		if err != nil {
			return nil, err
		}
	} else if s.External.Interface == "" {
		extIdx = noInterface
	}
	return &nat44_ed.Nat44AddDelStaticMappingV2{
		IsAdd: add, Flags: mappingFlags(s.AddrOnly, s.TwiceNAT, s.SelfTwiceNAT, s.Out2InOnly),
		LocalIPAddress: local, ExternalIPAddress: ext, Protocol: proto, LocalPort: lp, ExternalPort: ep,
		ExternalSwIfIndex: extIdx, VrfID: s.VRF, Tag: tag,
	}, nil
}

// MappingMeta remembers the external interface a mapping was created with.
type MappingMeta struct{ ExternalSwIfIndex uint32 }

func (p *Plugin) newStaticMapping() *natcommon.Descriptor[StaticMappingSpec] {
	return natcommon.New(natcommon.Ops[StaticMappingSpec]{
		Claims: p.claims(),
		Name: NameStaticMapping,
		ID:   func(s StaticMappingSpec) string { return s.Name },
		Deps: func(s StaticMappingSpec) []scheduler.Dependency {
			deps := natcommon.WithVRF(enableDep(), s.VRF)
			if s.External.Interface != "" {
				deps = append(deps, natcommon.InterfaceDep(s.External.Interface))
			}
			return deps
		},
		Create: func(ctx context.Context, s StaticMappingSpec) (any, error) {
			req, err := p.staticMappingRequest(ctx, s, true, noInterface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44AddDelStaticMappingV2(ctx, req); err != nil {
				return nil, fmt.Errorf("nat44_add_del_static_mapping_v2: %w", err)
			}
			return MappingMeta{ExternalSwIfIndex: uint32(req.ExternalSwIfIndex)}, nil
		},
		Delete: func(ctx context.Context, s StaticMappingSpec, meta any) error {
			m, ok := meta.(MappingMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameStaticMapping, meta)
			}
			req, err := p.staticMappingRequest(ctx, s, false, interface_types.InterfaceIndex(m.ExternalSwIfIndex))
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat44AddDelStaticMappingV2(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_add_del_static_mapping_v2: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[StaticMappingSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44StaticMappingDump(ctx, &nat44_ed.Nat44StaticMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_static_mapping_dump: %w", err)
			}
			var out []natcommon.Item[StaticMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return dedupeByName(out, func(it natcommon.Item[StaticMappingSpec]) (string, bool) {
						return it.Spec.Name, it.Meta.(MappingMeta).ExternalSwIfIndex != uint32(noInterface)
					}), nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_static_mapping_dump: %w", err)
				}
				name, ok := p.scope.ParseTag(d.Tag)
				if !ok {
					continue
				}
				s := StaticMappingSpec{
					Name:     name,
					Local:    Endpoint{IP: natcommon.IP4String(d.LocalIPAddress), Port: uint32(d.LocalPort)},
					External: Endpoint{IP: natcommon.IP4String(d.ExternalIPAddress), Port: uint32(d.ExternalPort)},
					Protocol: natcommon.ProtoName(d.Protocol), VRF: d.VrfID,
					AddrOnly: d.Flags&nat_types.NAT_IS_ADDR_ONLY != 0, TwiceNAT: d.Flags&nat_types.NAT_IS_TWICE_NAT != 0,
					SelfTwiceNAT: d.Flags&nat_types.NAT_IS_SELF_TWICE_NAT != 0, Out2InOnly: d.Flags&nat_types.NAT_IS_OUT2IN_ONLY != 0,
				}
				if d.ExternalSwIfIndex != noInterface {
					s.External.Interface = ifaces.Name(uint32(d.ExternalSwIfIndex))
				}
				s.Normalize()
				out = append(out, natcommon.Item[StaticMappingSpec]{Spec: s, Meta: MappingMeta{ExternalSwIfIndex: uint32(d.ExternalSwIfIndex)}})
			}
		},
	})
}

func (p *Plugin) identityRequest(ctx context.Context, s IdentityMappingSpec, add bool, idx interface_types.InterfaceIndex) (*nat44_ed.Nat44AddDelIdentityMapping, error) {
	tag, err := p.scope.Tag(s.Name)
	if err != nil {
		return nil, err
	}
	addr, err := ip4OrZero(s.IP)
	if err != nil {
		return nil, err
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	pt, err := port(s.Port)
	if err != nil {
		return nil, err
	}
	if s.Interface != "" && idx == noInterface {
		idx, err = natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
		if err != nil {
			return nil, err
		}
	} else if s.Interface == "" {
		idx = noInterface
	}
	var flags nat_types.NatConfigFlags
	if s.AddrOnly {
		flags |= nat_types.NAT_IS_ADDR_ONLY
	}
	return &nat44_ed.Nat44AddDelIdentityMapping{IsAdd: add, Flags: flags, IPAddress: addr, Protocol: proto, Port: pt, SwIfIndex: idx, VrfID: s.VRF, Tag: tag}, nil
}

func (p *Plugin) newIdentityMapping() *natcommon.Descriptor[IdentityMappingSpec] {
	return natcommon.New(natcommon.Ops[IdentityMappingSpec]{
		Claims: p.claims(),
		Name: NameIdentityMapping,
		ID:   func(s IdentityMappingSpec) string { return s.Name },
		Deps: func(s IdentityMappingSpec) []scheduler.Dependency {
			deps := natcommon.WithVRF(enableDep(), s.VRF)
			if s.Interface != "" {
				deps = append(deps, natcommon.InterfaceDep(s.Interface))
			}
			return deps
		},
		Create: func(ctx context.Context, s IdentityMappingSpec) (any, error) {
			req, err := p.identityRequest(ctx, s, true, noInterface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44AddDelIdentityMapping(ctx, req); err != nil {
				return nil, fmt.Errorf("nat44_add_del_identity_mapping: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(req.SwIfIndex)}, nil
		},
		Delete: func(ctx context.Context, s IdentityMappingSpec, meta any) error {
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameIdentityMapping, meta)
			}
			req, err := p.identityRequest(ctx, s, false, interface_types.InterfaceIndex(m.SwIfIndex))
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat44AddDelIdentityMapping(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_add_del_identity_mapping: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[IdentityMappingSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat44IdentityMappingDump(ctx, &nat44_ed.Nat44IdentityMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_identity_mapping_dump: %w", err)
			}
			var out []natcommon.Item[IdentityMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return dedupeByName(out, func(it natcommon.Item[IdentityMappingSpec]) (string, bool) {
						return it.Spec.Name, it.Meta.(IfMeta).SwIfIndex != uint32(noInterface)
					}), nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_identity_mapping_dump: %w", err)
				}
				name, ok := p.scope.ParseTag(d.Tag)
				if !ok {
					continue
				}
				s := IdentityMappingSpec{Name: name, IP: natcommon.IP4String(d.IPAddress), Protocol: natcommon.ProtoName(d.Protocol), Port: uint32(d.Port), VRF: d.VrfID, AddrOnly: d.Flags&nat_types.NAT_IS_ADDR_ONLY != 0}
				if d.SwIfIndex != noInterface {
					s.Interface = ifaces.Name(uint32(d.SwIfIndex))
				}
				s.Normalize()
				out = append(out, natcommon.Item[IdentityMappingSpec]{Spec: s, Meta: IfMeta{SwIfIndex: uint32(d.SwIfIndex)}})
			}
		},
	})
}

func lbLocals(locals []LBLocal) ([]nat44_ed.Nat44LbAddrPort, error) {
	out := make([]nat44_ed.Nat44LbAddrPort, 0, len(locals))
	for _, l := range locals {
		a, err := natcommon.IP4(l.IP)
		if err != nil {
			return nil, err
		}
		pt, err := port(l.Port)
		if err != nil {
			return nil, err
		}
		if l.Probability > 255 {
			return nil, fmt.Errorf("nat44-ed: probability %d out of range", l.Probability)
		}
		out = append(out, nat44_ed.Nat44LbAddrPort{Addr: a, Port: pt, Probability: uint8(l.Probability), VrfID: l.VRF})
	}
	return out, nil
}

func (p *Plugin) lbRequest(s LBStaticMappingSpec, add bool) (*nat44_ed.Nat44AddDelLbStaticMapping, error) {
	tag, err := p.scope.Tag(s.Name)
	if err != nil {
		return nil, err
	}
	ext, err := natcommon.IP4(s.External.IP)
	if err != nil {
		return nil, fmt.Errorf("external: %w", err)
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	ep, err := port(s.External.Port)
	if err != nil {
		return nil, err
	}
	locals, err := lbLocals(s.Locals)
	if err != nil {
		return nil, err
	}
	return &nat44_ed.Nat44AddDelLbStaticMapping{
		IsAdd: add, Flags: mappingFlags(false, s.TwiceNAT, s.SelfTwiceNAT, s.Out2InOnly),
		ExternalAddr: ext, ExternalPort: ep, Protocol: proto, Affinity: s.Affinity, Tag: tag,
		LocalNum: uint32(len(locals)), Locals: locals, //nolint:gosec // bounded by the API
	}, nil
}

func (p *Plugin) newLBStaticMapping() *natcommon.Descriptor[LBStaticMappingSpec] {
	return natcommon.New(natcommon.Ops[LBStaticMappingSpec]{
		Claims: p.claims(),
		Name: NameLBStaticMapping,
		ID:   func(s LBStaticMappingSpec) string { return s.Name },
		Deps: func(s LBStaticMappingSpec) []scheduler.Dependency {
			deps := enableDep()
			for _, l := range s.Locals {
				deps = natcommon.WithVRF(deps, l.VRF)
			}
			return deps
		},
		Create: func(ctx context.Context, s LBStaticMappingSpec) (any, error) {
			req, err := p.lbRequest(s, true)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat44AddDelLbStaticMapping(ctx, req); err != nil {
				return nil, fmt.Errorf("nat44_add_del_lb_static_mapping: %w", err)
			}
			return nil, nil
		},
		// Only the backend set changes in place; anything else recreates.
		Update: func(ctx context.Context, o, n LBStaticMappingSpec, meta any) (any, error) {
			same := o.Name == n.Name && o.External == n.External && o.Protocol == n.Protocol && o.Affinity == n.Affinity &&
				o.TwiceNAT == n.TwiceNAT && o.SelfTwiceNAT == n.SelfTwiceNAT && o.Out2InOnly == n.Out2InOnly
			if !same {
				return nil, scheduler.ErrRecreate
			}
			return meta, p.updateLBLocals(ctx, o, n)
		},
		Delete: func(ctx context.Context, s LBStaticMappingSpec, _ any) error {
			req, err := p.lbRequest(s, false)
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat44AddDelLbStaticMapping(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat44_add_del_lb_static_mapping: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[LBStaticMappingSpec], error) {
			stream, err := p.svc.Nat44LbStaticMappingDump(ctx, &nat44_ed.Nat44LbStaticMappingDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_lb_static_mapping_dump: %w", err)
			}
			var out []natcommon.Item[LBStaticMappingSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_lb_static_mapping_dump: %w", err)
				}
				name, ok := p.scope.ParseTag(d.Tag)
				if !ok {
					continue
				}
				s := LBStaticMappingSpec{
					Name: name, External: Endpoint{IP: natcommon.IP4String(d.ExternalAddr), Port: uint32(d.ExternalPort)},
					Protocol: natcommon.ProtoName(d.Protocol), Affinity: d.Affinity,
					TwiceNAT: d.Flags&nat_types.NAT_IS_TWICE_NAT != 0, SelfTwiceNAT: d.Flags&nat_types.NAT_IS_SELF_TWICE_NAT != 0, Out2InOnly: d.Flags&nat_types.NAT_IS_OUT2IN_ONLY != 0,
					Locals: make([]LBLocal, 0, len(d.Locals)),
				}
				for _, l := range d.Locals {
					s.Locals = append(s.Locals, LBLocal{IP: natcommon.IP4String(l.Addr), Port: uint32(l.Port), Probability: uint32(l.Probability), VRF: l.VrfID})
				}
				s.Normalize()
				out = append(out, natcommon.Item[LBStaticMappingSpec]{Spec: s})
			}
		},
	})
}

// updateLBLocals applies a backend-set change in place: new locals are added, removed ones
// deleted (nat44_lb_static_mapping_add_del_local).
func (p *Plugin) updateLBLocals(ctx context.Context, o, n LBStaticMappingSpec) error {
	ext, err := natcommon.IP4(n.External.IP)
	if err != nil {
		return err
	}
	proto, err := natcommon.ProtoNumber(n.Protocol)
	if err != nil {
		return err
	}
	ep, err := port(n.External.Port)
	if err != nil {
		return err
	}
	have := map[LBLocal]bool{}
	for _, l := range o.Locals {
		have[l] = true
	}
	want := map[LBLocal]bool{}
	for _, l := range n.Locals {
		want[l] = true
	}
	apply := func(l LBLocal, add bool) error {
		ls, err := lbLocals([]LBLocal{l})
		if err != nil {
			return err
		}
		if _, err := p.svc.Nat44LbStaticMappingAddDelLocal(ctx, &nat44_ed.Nat44LbStaticMappingAddDelLocal{IsAdd: add, ExternalAddr: ext, ExternalPort: ep, Protocol: proto, Local: ls[0]}); err != nil {
			return fmt.Errorf("nat44_lb_static_mapping_add_del_local: %w", err)
		}
		return nil
	}
	for _, l := range n.Locals {
		if !have[l] {
			if err := apply(l, true); err != nil {
				return err
			}
		}
	}
	for _, l := range o.Locals {
		if !want[l] {
			if err := apply(l, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// dedupeByName collapses the several details VPP sends for one tagged mapping into one item
// (review finding 2): for an interface-bound mapping nat44_static_mapping_dump /
// nat44_identity_mapping_dump send the resolved entry (external = the interface's current
// address, sw_if_index ~0) *and* the to-resolve record (external = the interface), both with
// the same tag; an identity mapping additionally sends one detail per local/VRF. The
// interface-bound record wins (it is what was desired and carries the sw_if_index Meta);
// otherwise the first detail is kept.
func dedupeByName[T any](items []natcommon.Item[T], key func(natcommon.Item[T]) (name string, ifBound bool)) []natcommon.Item[T] {
	pos := map[string]int{}
	out := items[:0]
	for _, it := range items {
		name, ifBound := key(it)
		if i, seen := pos[name]; seen {
			if _, curIf := key(out[i]); ifBound && !curIf {
				out[i] = it
			}
			continue
		}
		pos[name] = len(out)
		out = append(out, it)
	}
	return out
}
