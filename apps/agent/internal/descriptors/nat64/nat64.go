// Package nat64 holds the descriptors of VPP's NAT64 plugin (binapi/nat64, nat64_plugin.so):
// plugin enable, IPv6 prefixes per VRF, IPv4 pool ranges, inside/outside interfaces, static
// BIB entries and session timeouts, plus the Retrieve-only session-table helper. Object <->
// message table: docs/agent/descriptors/nat64.md.
//
// VPP 26.06 offers no "is nat64 enabled" getter: the enable singleton is write-only
// (ErrRetrieveUnsupported, D-063); Create/Delete treat VPP's "already enabled/disabled"
// (retval 1) as success.
package nat64

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat64"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable    = "nat64.enable"
	NamePrefix    = "nat64.prefix"
	NamePool      = "nat64.pool"
	NameInterface = "nat64.interface"
	NameStaticBIB = "nat64.static-bib"
	NameTimeouts  = "nat64.timeouts"
)

// Singleton is the id of the global singletons.
const Singleton = "global"

// Sides of a nat64 interface.
const (
	SideInside  = "inside"
	SideOutside = "outside"
)

// EnableKey is the key every other nat64 object depends on.
var EnableKey = scheduler.Join(NameEnable, Singleton)

// DefaultTimeouts are VPP's default nat64 timeouts (nat64.c).
var DefaultTimeouts = TimeoutsSpec{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}

// EnableSpec is the plugin singleton (nat64_plugin_enable_disable). Sizing parameters
// (bib/st buckets and memory) are startup-time hints without a getter and are not modelled;
// the object is on/off.
type EnableSpec struct{}

// PrefixSpec is one NAT64 IPv6 prefix for a VRF (nat64_add_del_prefix).
type PrefixSpec struct {
	Prefix string `json:"prefix"`
	VRF    uint32 `json:"vrf"`
}

// Normalize masks and canonicalises the prefix.
func (s *PrefixSpec) Normalize() { s.Prefix = natcommon.CanonPrefix(s.Prefix) }

// PoolSpec is one contiguous IPv4 pool range (nat64_add_del_pool_addr_range); VPP stores
// single addresses, Retrieve merges them back per VRF.
type PoolSpec struct {
	First string `json:"first"`
	Last  string `json:"last"`
	VRF   uint32 `json:"vrf"`
}

// Normalize canonicalises the range.
func (s *PoolSpec) Normalize() {
	s.First, s.Last = natcommon.CanonAddr(s.First), natcommon.CanonAddr(s.Last)
	if s.Last == "" {
		s.Last = s.First
	}
	a, errA := netip.ParseAddr(s.First)
	b, errB := netip.ParseAddr(s.Last)
	if errA == nil && errB == nil && b.Less(a) {
		s.First, s.Last = s.Last, s.First
	}
}

// InterfaceSpec puts an interface on the inside (IPv6) or outside (IPv4) of NAT64.
type InterfaceSpec struct {
	Interface string `json:"interface"`
	Side      string `json:"side"`
}

// StaticBIBSpec is one static binding (nat64_add_del_static_bib): inside IPv6 ip:port ->
// outside IPv4 ip:port for a protocol in a VRF.
type StaticBIBSpec struct {
	InsideIP    string `json:"inside_ip"`
	InsidePort  uint32 `json:"inside_port"`
	OutsideIP   string `json:"outside_ip"`
	OutsidePort uint32 `json:"outside_port"`
	Protocol    string `json:"protocol"`
	VRF         uint32 `json:"vrf"`
}

// Normalize canonicalises addresses and protocol.
func (s *StaticBIBSpec) Normalize() {
	s.InsideIP, s.OutsideIP = natcommon.CanonAddr(s.InsideIP), natcommon.CanonAddr(s.OutsideIP)
	s.Protocol = natcommon.CanonProto(s.Protocol)
}

// TimeoutsSpec is the session-timeout singleton (nat64_set_timeouts / nat64_get_timeouts);
// presence = non-default values.
type TimeoutsSpec struct {
	UDP            uint32 `json:"udp"`
	TCPEstablished uint32 `json:"tcp_established"`
	TCPTransitory  uint32 `json:"tcp_transitory"`
	ICMP           uint32 `json:"icmp"`
}

// IfMeta is the Meta of interface objects.
type IfMeta struct{ SwIfIndex uint32 }

// Plugin bundles the client, the owner scope and the descriptors of the nat64 family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    nat64.RPCService

	Enable    *natcommon.Descriptor[EnableSpec]
	Prefix    *natcommon.Descriptor[PrefixSpec]
	Pool      *natcommon.Descriptor[PoolSpec]
	Interface *natcommon.Descriptor[InterfaceSpec]
	StaticBIB *natcommon.Descriptor[StaticBIBSpec]
	Timeouts  *natcommon.Descriptor[TimeoutsSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: nat64.NewServiceClient(client)}
	p.Enable = p.newEnable()
	p.Timeouts = p.newTimeouts()
	p.Prefix = p.newPrefix()
	p.Pool = p.newPool()
	p.Interface = p.newInterface()
	p.StaticBIB = p.newStaticBIB()
	return p
}

// Descriptors returns the family in registration order (prefix and pool before static BIBs).
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Enable, p.Timeouts, p.Prefix, p.Pool, p.Interface, p.StaticBIB}
}

// Register constructs the family and registers every descriptor.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func enableDep() []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(EnableKey)} }

func port(v uint32) (uint16, error) {
	if v > 65535 {
		return 0, fmt.Errorf("nat64: port %d out of range", v)
	}
	return uint16(v), nil
}

func (p *Plugin) claims() natcommon.ClaimStore { return p.cfg.Claims }

// Empty reports whether nat64 holds no configuration object of ANY owner: interfaces,
// prefixes, pool addresses, static BIB entries (the D-071 precondition for a disable, which
// destroys all of them). It is also the only evidence of "enabled": VPP keeps no nat64
// object while the plugin is disabled.
func (p *Plugin) Empty(ctx context.Context) (bool, error) {
	counts := []func() (int, error){
		func() (int, error) {
			s, err := p.svc.Nat64InterfaceDump(ctx, &nat64.Nat64InterfaceDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat64PrefixDump(ctx, &nat64.Nat64PrefixDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat64PoolAddrDump(ctx, &nat64.Nat64PoolAddrDump{})
			if err != nil {
				return 0, err
			}
			return natcommon.Count(s.Recv)
		},
		func() (int, error) {
			s, err := p.svc.Nat64BibDump(ctx, &nat64.Nat64BibDump{Proto: 255})
			if err != nil {
				return 0, err
			}
			n := 0
			for {
				d, err := s.Recv()
				if errors.Is(err, io.EOF) {
					return n, nil
				}
				if err != nil {
					return 0, err
				}
				if d.Flags&nat_types.NAT_IS_STATIC != 0 {
					n++
				}
			}
		},
	}
	for _, c := range counts {
		n, err := c()
		if err != nil {
			return false, fmt.Errorf("nat64 emptiness check: %w", err)
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}

func (p *Plugin) ownsPrefix(pfx interface{ String() string }, vrf uint32) bool {
	np, err := netip.ParsePrefix(pfx.String())
	return err == nil && (p.scope.OwnsPrefix(np) || p.scope.OwnsTable(vrf))
}

// newEnable: VPP-global (D-071). VPP has no "is nat64 enabled" getter: the only evidence is
// an existing nat64 object (enabled for sure); otherwise the state is unobservable and a
// non-owner's requirement cannot be verified (Create succeeds without touching VPP,
// documented). Owner: enable (idempotent, write-only for Retrieve, D-063); Delete disables
// only when the plugin holds no object of any owner, else skips.
func (p *Plugin) newEnable() *natcommon.Descriptor[EnableSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[EnableSpec]{
		Name: NameEnable, ID: Singleton,
		Read: func(ctx context.Context) (natcommon.GlobalState[EnableSpec], error) {
			empty, err := p.Empty(ctx)
			if err != nil {
				return natcommon.GlobalState[EnableSpec]{}, err
			}
			return natcommon.GlobalState[EnableSpec]{Present: !empty, Observable: !empty}, nil
		},
		Match:     natcommon.AnyValue[EnableSpec],
		WriteOnly: true,
		Set: func(ctx context.Context, _ EnableSpec) error {
			if _, err := p.svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: true}); err != nil && !natcommon.IsAlreadyEnabled(err) {
				return fmt.Errorf("nat64_plugin_enable_disable: %w", err)
			}
			return nil
		},
		Reset: func(ctx context.Context, _ EnableSpec) error {
			empty, err := p.Empty(ctx)
			if err != nil || !empty {
				return err // not empty: skip, never disable under another owner's objects
			}
			if _, err := p.svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: false}); err != nil && !natcommon.IsAlreadyDisabled(err) {
				return fmt.Errorf("nat64_plugin_enable_disable: %w", err)
			}
			return nil
		},
	})
}

func (p *Plugin) setTimeouts(ctx context.Context, s TimeoutsSpec) error {
	if _, err := p.svc.Nat64SetTimeouts(ctx, &nat64.Nat64SetTimeouts{UDP: s.UDP, TCPEstablished: s.TCPEstablished, TCPTransitory: s.TCPTransitory, ICMP: s.ICMP}); err != nil {
		return fmt.Errorf("nat64_set_timeouts: %w", err)
	}
	return nil
}

// newTimeouts: VPP-global (D-071).
func (p *Plugin) newTimeouts() *natcommon.Descriptor[TimeoutsSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[TimeoutsSpec]{
		Name: NameTimeouts, ID: Singleton,
		Deps: func(TimeoutsSpec) []scheduler.Dependency { return enableDep() },
		Read: func(ctx context.Context) (natcommon.GlobalState[TimeoutsSpec], error) {
			rep, err := p.svc.Nat64GetTimeouts(ctx, &nat64.Nat64GetTimeouts{})
			if err != nil {
				return natcommon.GlobalState[TimeoutsSpec]{}, fmt.Errorf("nat64_get_timeouts: %w", err)
			}
			t := TimeoutsSpec{UDP: rep.UDP, TCPEstablished: rep.TCPEstablished, TCPTransitory: rep.TCPTransitory, ICMP: rep.ICMP}
			return natcommon.GlobalState[TimeoutsSpec]{Value: t, Present: true, Observable: true}, nil
		},
		Absent: func(t TimeoutsSpec) bool { return t == DefaultTimeouts },
		Set:    p.setTimeouts,
		Reset:  func(ctx context.Context, _ TimeoutsSpec) error { return p.setTimeouts(ctx, DefaultTimeouts) },
	})
}

func (p *Plugin) addDelPrefix(ctx context.Context, s PrefixSpec, add bool) error {
	pfx, err := natcommon.Prefix6(s.Prefix)
	if err != nil {
		return err
	}
	if _, err := p.svc.Nat64AddDelPrefix(ctx, &nat64.Nat64AddDelPrefix{Prefix: pfx, VrfID: s.VRF, IsAdd: add}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("nat64_add_del_prefix: %w", err)
	}
	return nil
}

func (p *Plugin) newPrefix() *natcommon.Descriptor[PrefixSpec] {
	return natcommon.New(natcommon.Ops[PrefixSpec]{
		Claims: p.claims(),
		Name:   NamePrefix,
		ID:     func(s PrefixSpec) string { return fmt.Sprintf("%s/%d", s.Prefix, s.VRF) },
		Deps:   func(s PrefixSpec) []scheduler.Dependency { return natcommon.WithVRF(enableDep(), s.VRF) },
		Create: func(ctx context.Context, s PrefixSpec) (any, error) { return nil, p.addDelPrefix(ctx, s, true) },
		Delete: func(ctx context.Context, s PrefixSpec, _ any) error { return p.addDelPrefix(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[PrefixSpec], error) {
			stream, err := p.svc.Nat64PrefixDump(ctx, &nat64.Nat64PrefixDump{})
			if err != nil {
				return nil, fmt.Errorf("nat64_prefix_dump: %w", err)
			}
			var out []natcommon.Item[PrefixSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat64_prefix_dump: %w", err)
				}
				out = append(out, natcommon.Item[PrefixSpec]{Spec: PrefixSpec{Prefix: natcommon.Prefix6String(d.Prefix), VRF: d.VrfID},
					NeedsClaim: p.scope.NeedsClaim(p.ownsPrefix(d.Prefix, d.VrfID))})
			}
		},
	})
}

func (p *Plugin) addDelPool(ctx context.Context, s PoolSpec, add bool) error {
	first, err := natcommon.IP4(s.First)
	if err != nil {
		return err
	}
	last, err := natcommon.IP4(s.Last)
	if err != nil {
		return err
	}
	if _, err := p.svc.Nat64AddDelPoolAddrRange(ctx, &nat64.Nat64AddDelPoolAddrRange{StartAddr: first, EndAddr: last, VrfID: s.VRF, IsAdd: add}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("nat64_add_del_pool_addr_range: %w", err)
	}
	return nil
}

type poolAddr struct {
	addr netip.Addr
	vrf  uint32
}

func mergeRanges(addrs []poolAddr) []PoolSpec {
	sort.Slice(addrs, func(i, j int) bool {
		if addrs[i].vrf != addrs[j].vrf {
			return addrs[i].vrf < addrs[j].vrf
		}
		return addrs[i].addr.Less(addrs[j].addr)
	})
	var out []PoolSpec
	for i := 0; i < len(addrs); {
		j := i
		for j+1 < len(addrs) && addrs[j+1].vrf == addrs[i].vrf && addrs[j+1].addr == addrs[j].addr.Next() {
			j++
		}
		out = append(out, PoolSpec{First: addrs[i].addr.String(), Last: addrs[j].addr.String(), VRF: addrs[i].vrf})
		i = j + 1
	}
	return out
}

func (p *Plugin) newPool() *natcommon.Descriptor[PoolSpec] {
	return natcommon.New(natcommon.Ops[PoolSpec]{
		Claims: p.claims(),
		Name:   NamePool,
		ID:     func(s PoolSpec) string { return fmt.Sprintf("%s-%s/%d", s.First, s.Last, s.VRF) },
		Deps: func(s PoolSpec) []scheduler.Dependency {
			deps := enableDep()
			if s.VRF != ^uint32(0) {
				deps = natcommon.WithVRF(deps, s.VRF)
			}
			return deps
		},
		Create: func(ctx context.Context, s PoolSpec) (any, error) { return nil, p.addDelPool(ctx, s, true) },
		Delete: func(ctx context.Context, s PoolSpec, _ any) error { return p.addDelPool(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[PoolSpec], error) {
			stream, err := p.svc.Nat64PoolAddrDump(ctx, &nat64.Nat64PoolAddrDump{})
			if err != nil {
				return nil, fmt.Errorf("nat64_pool_addr_dump: %w", err)
			}
			var addrs []poolAddr
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("nat64_pool_addr_dump: %w", err)
				}
				if a := netip.AddrFrom4(d.Address); p.scope.All || p.scope.OwnsAddr(a) {
					addrs = append(addrs, poolAddr{addr: a, vrf: d.VrfID})
				}
			}
			var out []natcommon.Item[PoolSpec]
			for _, s := range mergeRanges(addrs) {
				in := p.scope.OwnsAddrString(s.First) && p.scope.OwnsAddrString(s.Last)
				out = append(out, natcommon.Item[PoolSpec]{Spec: s, NeedsClaim: p.scope.NeedsClaim(in)})
			}
			return out, nil
		},
	})
}

func sideFlag(side string) (nat_types.NatConfigFlags, error) {
	switch side {
	case SideInside:
		return nat_types.NAT_IS_INSIDE, nil
	case SideOutside:
		return nat_types.NAT_IS_OUTSIDE, nil
	}
	return 0, fmt.Errorf("nat64: side must be %q or %q, got %q", SideInside, SideOutside, side)
}

func (p *Plugin) newInterface() *natcommon.Descriptor[InterfaceSpec] {
	return natcommon.New(natcommon.Ops[InterfaceSpec]{
		Claims: p.claims(),
		Name:   NameInterface,
		ID:     func(s InterfaceSpec) string { return s.Interface + "/" + s.Side },
		Deps: func(s InterfaceSpec) []scheduler.Dependency {
			return append(enableDep(), natcommon.InterfaceDep(s.Interface))
		},
		Create: func(ctx context.Context, s InterfaceSpec) (any, error) {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return nil, err
			}
			idx, err := natcommon.ResolveOwned(ctx, p.client, p.scope, s.Interface)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat64AddDelInterface(ctx, &nat64.Nat64AddDelInterface{IsAdd: true, Flags: flag, SwIfIndex: idx}); err != nil {
				return nil, fmt.Errorf("nat64_add_del_interface: %w", err)
			}
			return IfMeta{SwIfIndex: uint32(idx)}, nil
		},
		Delete: func(ctx context.Context, s InterfaceSpec, meta any) error {
			flag, err := sideFlag(s.Side)
			if err != nil {
				return err
			}
			m, ok := meta.(IfMeta)
			if !ok {
				return fmt.Errorf("%s: unexpected meta %T", NameInterface, meta)
			}
			if _, err := p.svc.Nat64AddDelInterface(ctx, &nat64.Nat64AddDelInterface{IsAdd: false, Flags: flag, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat64_add_del_interface: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[InterfaceSpec], error) {
			ifaces, err := natcommon.DumpInterfaces(ctx, p.client)
			if err != nil {
				return nil, err
			}
			stream, err := p.svc.Nat64InterfaceDump(ctx, &nat64.Nat64InterfaceDump{})
			if err != nil {
				return nil, fmt.Errorf("nat64_interface_dump: %w", err)
			}
			var out []natcommon.Item[InterfaceSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat64_interface_dump: %w", err)
				}
				i, _ := ifaces.ByIndex(uint32(d.SwIfIndex))
				ok, nc := p.scope.InterfaceOwnership(i)
				if !ok {
					continue
				}
				meta := IfMeta{SwIfIndex: uint32(d.SwIfIndex)}
				if d.Flags&nat_types.NAT_IS_INSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: p.scope.LogicalName(i), Side: SideInside}, Meta: meta, NeedsClaim: nc})
				}
				if d.Flags&nat_types.NAT_IS_OUTSIDE != 0 {
					out = append(out, natcommon.Item[InterfaceSpec]{Spec: InterfaceSpec{Interface: p.scope.LogicalName(i), Side: SideOutside}, Meta: meta, NeedsClaim: nc})
				}
			}
		},
	})
}

func (p *Plugin) bibRequest(s StaticBIBSpec, add bool) (*nat64.Nat64AddDelStaticBib, error) {
	in, err := natcommon.IP6(s.InsideIP)
	if err != nil {
		return nil, fmt.Errorf("inside: %w", err)
	}
	out, err := natcommon.IP4(s.OutsideIP)
	if err != nil {
		return nil, fmt.Errorf("outside: %w", err)
	}
	ip, err := port(s.InsidePort)
	if err != nil {
		return nil, err
	}
	op, err := port(s.OutsidePort)
	if err != nil {
		return nil, err
	}
	proto, err := natcommon.ProtoNumber(s.Protocol)
	if err != nil {
		return nil, err
	}
	return &nat64.Nat64AddDelStaticBib{IAddr: in, OAddr: out, IPort: ip, OPort: op, VrfID: s.VRF, Proto: proto, IsAdd: add}, nil
}

func (p *Plugin) newStaticBIB() *natcommon.Descriptor[StaticBIBSpec] {
	return natcommon.New(natcommon.Ops[StaticBIBSpec]{
		Claims: p.claims(),
		Name:   NameStaticBIB,
		ID: func(s StaticBIBSpec) string {
			return fmt.Sprintf("%s/%s/%d/%d", s.Protocol, s.InsideIP, s.InsidePort, s.VRF)
		},
		Deps: func(s StaticBIBSpec) []scheduler.Dependency { return natcommon.WithVRF(enableDep(), s.VRF) },
		Create: func(ctx context.Context, s StaticBIBSpec) (any, error) {
			req, err := p.bibRequest(s, true)
			if err != nil {
				return nil, err
			}
			if _, err := p.svc.Nat64AddDelStaticBib(ctx, req); err != nil {
				return nil, fmt.Errorf("nat64_add_del_static_bib: %w", err)
			}
			return nil, nil
		},
		Delete: func(ctx context.Context, s StaticBIBSpec, _ any) error {
			req, err := p.bibRequest(s, false)
			if err != nil {
				return err
			}
			if _, err := p.svc.Nat64AddDelStaticBib(ctx, req); err != nil && !natcommon.IsNoSuchEntry(err) {
				return fmt.Errorf("nat64_add_del_static_bib: %w", err)
			}
			return nil
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[StaticBIBSpec], error) {
			stream, err := p.svc.Nat64BibDump(ctx, &nat64.Nat64BibDump{Proto: 255}) // 255 = every protocol
			if err != nil {
				return nil, fmt.Errorf("nat64_bib_dump: %w", err)
			}
			var out []natcommon.Item[StaticBIBSpec]
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return nil, fmt.Errorf("nat64_bib_dump: %w", err)
				}
				if d.Flags&nat_types.NAT_IS_STATIC == 0 {
					continue // dynamic BIB entries are session state
				}
				in := p.scope.OwnsAddr(netip.AddrFrom4(d.OAddr)) || p.scope.OwnsTable(d.VrfID)
				s := StaticBIBSpec{InsideIP: natcommon.IP6String(d.IAddr), InsidePort: uint32(d.IPort), OutsideIP: natcommon.IP4String(d.OAddr), OutsidePort: uint32(d.OPort), Protocol: natcommon.ProtoName(d.Proto), VRF: d.VrfID}
				out = append(out, natcommon.Item[StaticBIBSpec]{Spec: s, NeedsClaim: p.scope.NeedsClaim(in)})
			}
		},
	})
}

// SessionEntry is one NAT64 session-table row (nat64_st_details), Retrieve-only state.
type SessionEntry struct {
	InsideLocal   string `json:"inside_local"`
	OutsideLocal  string `json:"outside_local"`
	InsidePort    uint32 `json:"inside_port"`
	OutsidePort   uint32 `json:"outside_port"`
	InsideRemote  string `json:"inside_remote"`
	OutsideRemote string `json:"outside_remote"`
	RemotePort    uint32 `json:"remote_port"`
	Protocol      string `json:"protocol"`
	VRF           uint32 `json:"vrf"`
}

// Sessions returns one page (offset, limit; 0 = all) of the session table for a protocol
// ("any" = every protocol).
func (p *Plugin) Sessions(ctx context.Context, protocol string, offset, limit int) ([]SessionEntry, error) {
	proto := uint8(255)
	if protocol != "" && protocol != "any" {
		n, err := natcommon.ProtoNumber(protocol)
		if err != nil {
			return nil, err
		}
		proto = n
	}
	stream, err := p.svc.Nat64StDump(ctx, &nat64.Nat64StDump{Proto: proto})
	if err != nil {
		return nil, fmt.Errorf("nat64_st_dump: %w", err)
	}
	var out []SessionEntry
	for i := 0; ; i++ {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("nat64_st_dump: %w", err)
		}
		if i < offset || (limit > 0 && len(out) >= limit) {
			continue
		}
		out = append(out, SessionEntry{
			InsideLocal: natcommon.IP6String(d.IlAddr), OutsideLocal: natcommon.IP4String(d.OlAddr), InsidePort: uint32(d.IlPort), OutsidePort: uint32(d.OlPort),
			InsideRemote: natcommon.IP6String(d.IrAddr), OutsideRemote: natcommon.IP4String(d.OrAddr), RemotePort: uint32(d.RPort),
			Protocol: natcommon.ProtoName(d.Proto), VRF: d.VrfID,
		})
	}
}
