// Package dslite holds the descriptors of VPP's DS-Lite plugin (binapi/dslite, dslite_plugin.so):
// the AFTR endpoint, the B4 endpoint (both VPP-global singletons, D-071) and the IPv4 pool
// address ranges (claim-store ownership like nat64 pools). Object <-> message table:
// docs/agent/descriptors/dslite.md.
//
// VPP 26.06 has no "delete" for the AFTR/B4 addresses: dslite_set_aftr_addr / dslite_set_b4_addr
// overwrite them. The globals owner's Reset writes the unspecified addresses (:: / 0.0.0.0), which
// is also what the getters report on a fresh VPP ("absent"). A non-owner only requires them.
package dslite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"

	dsliteapi "ngfw/agent/binapi/dslite"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameAftr = "dslite.aftr"
	NameB4   = "dslite.b4"
	NamePool = "dslite.pool"
)

// Singleton is the id of the AFTR / B4 singletons.
const Singleton = "global"

// EndpointSpec is the AFTR or B4 endpoint: the IPv6 tunnel address and the optional IPv4
// address (AFTR: the IPv4 address ICMP errors are sourced from; B4: the CE's IPv4 address).
type EndpointSpec struct {
	IPv6 string `json:"ipv6"`
	IPv4 string `json:"ipv4"`
}

// Normalize canonicalises both addresses; the unspecified addresses become "".
func (s *EndpointSpec) Normalize() {
	s.IPv6, s.IPv4 = canonOpt(s.IPv6), canonOpt(s.IPv4)
}

// PoolSpec is one contiguous IPv4 pool range (dslite_add_del_pool_addr_range). VPP stores single
// addresses; Retrieve merges them back into ranges.
type PoolSpec struct {
	First string `json:"first"`
	Last  string `json:"last"`
}

// Normalize canonicalises the range (Last defaults to First, the pair is ordered).
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

// PoolID is the object id of a pool range.
func PoolID(s PoolSpec) string { return s.First + "-" + s.Last }

func canonOpt(s string) string {
	if s == "" {
		return ""
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return s
	}
	if a.IsUnspecified() {
		return ""
	}
	return a.Unmap().String()
}

// Plugin bundles the client, the owner scope and the descriptors of the dslite family.
type Plugin struct {
	client vpp.Client
	scope  natcommon.Scope
	cfg    natcommon.Config
	svc    dsliteapi.RPCService

	Aftr *natcommon.Descriptor[EndpointSpec]
	B4   *natcommon.Descriptor[EndpointSpec]
	Pool *natcommon.Descriptor[PoolSpec]
}

// New constructs the family for client and owner.
func New(client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := &Plugin{client: client, scope: natcommon.ScopeFor(owner), cfg: natcommon.BuildConfig(opts), svc: dsliteapi.NewServiceClient(client)}
	p.Aftr = p.newEndpoint(NameAftr, p.getAftr, p.setAftr)
	p.B4 = p.newEndpoint(NameB4, p.getB4, p.setB4)
	p.Pool = p.newPool()
	return p
}

// Descriptors returns the family in registration order.
func (p *Plugin) Descriptors() []scheduler.Descriptor {
	return []scheduler.Descriptor{p.Aftr, p.B4, p.Pool}
}

// Register constructs the family and registers every descriptor.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...natcommon.Option) *Plugin {
	p := New(client, owner, opts...)
	for _, d := range p.Descriptors() {
		r.Register(d)
	}
	return p
}

func endpointMsg(s EndpointSpec) (v4 [4]uint8, v6 [16]uint8, err error) {
	if s.IPv6 != "" {
		a, e := natcommon.IP6(s.IPv6)
		if e != nil {
			return v4, v6, e
		}
		v6 = a
	}
	if s.IPv4 != "" {
		a, e := natcommon.IP4(s.IPv4)
		if e != nil {
			return v4, v6, e
		}
		v4 = a
	}
	return v4, v6, nil
}

func (p *Plugin) getAftr(ctx context.Context) (EndpointSpec, error) {
	rep, err := p.svc.DsliteGetAftrAddr(ctx, &dsliteapi.DsliteGetAftrAddr{})
	if err != nil {
		return EndpointSpec{}, fmt.Errorf("dslite_get_aftr_addr: %w", err)
	}
	s := EndpointSpec{IPv6: natcommon.IP6String(rep.IP6Addr), IPv4: natcommon.IP4String(rep.IP4Addr)}
	s.Normalize()
	return s, nil
}

func (p *Plugin) setAftr(ctx context.Context, s EndpointSpec) error {
	v4, v6, err := endpointMsg(s)
	if err != nil {
		return err
	}
	if _, err := p.svc.DsliteSetAftrAddr(ctx, &dsliteapi.DsliteSetAftrAddr{IP4Addr: v4, IP6Addr: v6}); err != nil {
		return fmt.Errorf("dslite_set_aftr_addr: %w", err)
	}
	return nil
}

func (p *Plugin) getB4(ctx context.Context) (EndpointSpec, error) {
	rep, err := p.svc.DsliteGetB4Addr(ctx, &dsliteapi.DsliteGetB4Addr{})
	if err != nil {
		return EndpointSpec{}, fmt.Errorf("dslite_get_b4_addr: %w", err)
	}
	s := EndpointSpec{IPv6: natcommon.IP6String(rep.IP6Addr), IPv4: natcommon.IP4String(rep.IP4Addr)}
	s.Normalize()
	return s, nil
}

func (p *Plugin) setB4(ctx context.Context, s EndpointSpec) error {
	v4, v6, err := endpointMsg(s)
	if err != nil {
		return err
	}
	if _, err := p.svc.DsliteSetB4Addr(ctx, &dsliteapi.DsliteSetB4Addr{IP4Addr: v4, IP6Addr: v6}); err != nil {
		return fmt.Errorf("dslite_set_b4_addr: %w", err)
	}
	return nil
}

// newEndpoint: AFTR and B4 are VPP globals (D-071). The owner sets / resets (to the unspecified
// addresses) and retrieves them; a non-owner requires the exact value.
func (p *Plugin) newEndpoint(name string, get func(context.Context) (EndpointSpec, error), set func(context.Context, EndpointSpec) error) *natcommon.Descriptor[EndpointSpec] {
	return natcommon.Global(p.cfg, natcommon.GlobalOps[EndpointSpec]{
		Name: name, ID: Singleton,
		Read: func(ctx context.Context) (natcommon.GlobalState[EndpointSpec], error) {
			v, err := get(ctx)
			if err != nil {
				return natcommon.GlobalState[EndpointSpec]{}, err
			}
			return natcommon.GlobalState[EndpointSpec]{Value: v, Present: v != (EndpointSpec{}), Observable: true}, nil
		},
		Set:   set,
		Reset: func(ctx context.Context, _ EndpointSpec) error { return set(ctx, EndpointSpec{}) },
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
	if _, err := p.svc.DsliteAddDelPoolAddrRange(ctx, &dsliteapi.DsliteAddDelPoolAddrRange{StartAddr: first, EndAddr: last, IsAdd: add}); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("dslite_add_del_pool_addr_range: %w", err)
	}
	return nil
}

// MergeRanges folds single addresses into contiguous ranges (sorted).
func MergeRanges(addrs []netip.Addr) []PoolSpec {
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	var out []PoolSpec
	for i := 0; i < len(addrs); {
		j := i
		for j+1 < len(addrs) && addrs[j+1] == addrs[j].Next() {
			j++
		}
		out = append(out, PoolSpec{First: addrs[i].String(), Last: addrs[j].String()})
		i = j + 1
	}
	return out
}

func (p *Plugin) newPool() *natcommon.Descriptor[PoolSpec] {
	return natcommon.New(natcommon.Ops[PoolSpec]{
		Claims: p.cfg.Claims,
		Name:   NamePool,
		ID:     PoolID,
		Create: func(ctx context.Context, s PoolSpec) (any, error) { return nil, p.addDelPool(ctx, s, true) },
		Delete: func(ctx context.Context, s PoolSpec, _ any) error { return p.addDelPool(ctx, s, false) },
		Retrieve: func(ctx context.Context) ([]natcommon.Item[PoolSpec], error) {
			stream, err := p.svc.DsliteAddressDump(ctx, &dsliteapi.DsliteAddressDump{})
			if err != nil {
				return nil, fmt.Errorf("dslite_address_dump: %w", err)
			}
			var addrs []netip.Addr
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("dslite_address_dump: %w", err)
				}
				if a := netip.AddrFrom4(d.IPAddress); p.scope.All || p.scope.OwnsAddr(a) {
					addrs = append(addrs, a)
				}
			}
			var out []natcommon.Item[PoolSpec]
			for _, s := range MergeRanges(addrs) {
				in := p.scope.OwnsAddrString(s.First) && p.scope.OwnsAddrString(s.Last)
				out = append(out, natcommon.Item[PoolSpec]{Spec: s, NeedsClaim: p.scope.NeedsClaim(in)})
			}
			return out, nil
		},
	})
}
