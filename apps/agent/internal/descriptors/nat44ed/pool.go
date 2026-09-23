package nat44ed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// AnyVRF is VPP's "match any VRF" table id for pool addresses.
const AnyVRF = ^uint32(0)

// AddressPoolSpec is one contiguous range of pool addresses (nat44_add_del_address_range),
// First..Last inclusive, at most 1024 addresses (VPP limit). VPP stores addresses one by one;
// Retrieve merges consecutive addresses of the same VRF / twice-NAT class back into ranges,
// so two adjacent desired pools must be one object (schema rule for F-nat44-ed-sessions).
type AddressPoolSpec struct {
	First    string `json:"first"`
	Last     string `json:"last"`
	VRF      uint32 `json:"vrf"`
	TwiceNAT bool   `json:"twice_nat"`
}

// Normalize canonicalises the addresses and orders First <= Last.
func (s *AddressPoolSpec) Normalize() {
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

// PoolID is the key id of a pool: "<first>-<last>/<vrf>[/twice-nat]".
func PoolID(s AddressPoolSpec) string {
	id := fmt.Sprintf("%s-%s/%d", s.First, s.Last, s.VRF)
	if s.TwiceNAT {
		id += "/twice-nat"
	}
	return id
}

func (p *Plugin) addDelAddressRange(ctx context.Context, s AddressPoolSpec, add bool) error {
	first, err := natcommon.IP4(s.First)
	if err != nil {
		return err
	}
	last, err := natcommon.IP4(s.Last)
	if err != nil {
		return err
	}
	req := &nat44_ed.Nat44AddDelAddressRange{FirstIPAddress: first, LastIPAddress: last, VrfID: s.VRF, IsAdd: add}
	if s.TwiceNAT {
		req.Flags = nat_types.NAT_IS_TWICE_NAT
	}
	if _, err := p.svc.Nat44AddDelAddressRange(ctx, req); err != nil {
		if !add && natcommon.IsNoSuchEntry(err) {
			return nil
		}
		return fmt.Errorf("nat44_add_del_address_range: %w", err)
	}
	return nil
}

func (p *Plugin) newAddressPool() *natcommon.Descriptor[AddressPoolSpec] {
	return natcommon.New(natcommon.Ops[AddressPoolSpec]{
		Name: NameAddressPool,
		ID:   PoolID,
		Deps: func(s AddressPoolSpec) []scheduler.Dependency {
			deps := enableDep()
			if s.VRF != AnyVRF {
				deps = natcommon.WithVRF(deps, s.VRF)
			}
			return deps
		},
		Create: func(ctx context.Context, s AddressPoolSpec) (any, error) {
			return nil, p.addDelAddressRange(ctx, s, true)
		},
		Delete: func(ctx context.Context, s AddressPoolSpec, _ any) error {
			return p.addDelAddressRange(ctx, s, false)
		},
		Retrieve: func(ctx context.Context) ([]natcommon.Item[AddressPoolSpec], error) {
			stream, err := p.svc.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
			if err != nil {
				return nil, fmt.Errorf("nat44_address_dump: %w", err)
			}
			var addrs []poolAddr
			for {
				d, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("nat44_address_dump: %w", err)
				}
				a := netip.AddrFrom4(d.IPAddress)
				if !p.scope.OwnsAddr(a) {
					continue
				}
				addrs = append(addrs, poolAddr{addr: a, vrf: d.VrfID, twice: d.Flags&nat_types.NAT_IS_TWICE_NAT != 0})
			}
			var out []natcommon.Item[AddressPoolSpec]
			for _, s := range mergeRanges(addrs) {
				out = append(out, natcommon.Item[AddressPoolSpec]{Spec: s})
			}
			return out, nil
		},
	})
}

type poolAddr struct {
	addr  netip.Addr
	vrf   uint32
	twice bool
}

// mergeRanges turns individual pool addresses into maximal contiguous ranges per VRF and
// twice-NAT class, sorted by first address.
func mergeRanges(addrs []poolAddr) []AddressPoolSpec {
	sort.Slice(addrs, func(i, j int) bool {
		if addrs[i].twice != addrs[j].twice {
			return !addrs[i].twice
		}
		if addrs[i].vrf != addrs[j].vrf {
			return addrs[i].vrf < addrs[j].vrf
		}
		return addrs[i].addr.Less(addrs[j].addr)
	})
	var out []AddressPoolSpec
	for i := 0; i < len(addrs); {
		j := i
		for j+1 < len(addrs) && addrs[j+1].twice == addrs[i].twice && addrs[j+1].vrf == addrs[i].vrf && addrs[j+1].addr == addrs[j].addr.Next() {
			j++
		}
		out = append(out, AddressPoolSpec{First: addrs[i].addr.String(), Last: addrs[j].addr.String(), VRF: addrs[i].vrf, TwiceNAT: addrs[i].twice})
		i = j + 1
	}
	return out
}
