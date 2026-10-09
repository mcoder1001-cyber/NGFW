package pppoe

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
)

// DelegationTarget is an explicit LAN assignment within an ISP delegated prefix.
// SubnetID selects the bits between the delegation length and /64. It is stable
// across target ordering and lease renewal; callers must never infer LAN targets
// from interface enumeration. This is an internal source contract, not a config
// field until schema/proto and desired-state projection are integrated.
type DelegationTarget struct {
	Interface string
	SubnetID  uint64
}

// DelegationAssignment is a LAN /64 plus the router's first address in that /64.
// Applying it requires the normal owned interface-address and RA descriptors;
// allocation alone does not configure a LAN or advertise a prefix.
type DelegationAssignment struct {
	Interface string
	Prefix    netip.Prefix
	Router    netip.Prefix
}

// AllocateDelegation validates an entire PD assignment before returning any
// changes. Repeated subnet IDs are rejected even on different LANs: advertising
// the same /64 on separate routed links would create ambiguous return routing.
// A changed delegated prefix produces a new plan with the same relative subnet
// selection. The controller must withdraw the old plan on lease loss/replacement.
func AllocateDelegation(delegated netip.Prefix, targets []DelegationTarget) ([]DelegationAssignment, error) {
	if !delegated.IsValid() || !globalV6(delegated.Addr()) || delegated.Bits() < 16 || delegated.Bits() > 64 {
		return nil, fmt.Errorf("%w: delegation must be a global/ULA IPv6 /16 through /64", ErrInput)
	}
	if len(targets) > 1024 {
		return nil, fmt.Errorf("%w: more than 1024 delegated LAN targets", ErrInput)
	}
	delegated = delegated.Masked()
	capacity := uint64(1) << uint(64-delegated.Bits())
	base := delegated.Addr().As16()
	high := binary.BigEndian.Uint64(base[:8])
	names := make(map[string]bool, len(targets))
	ids := make(map[uint64]bool, len(targets))
	out := make([]DelegationAssignment, 0, len(targets))
	for _, target := range targets {
		if target.Interface == "" || len(target.Interface) > 63 {
			return nil, fmt.Errorf("%w: delegated LAN interface is missing or too long", ErrInput)
		}
		if names[target.Interface] || ids[target.SubnetID] {
			return nil, fmt.Errorf("%w: duplicate delegated LAN interface or subnet ID", ErrInput)
		}
		if target.SubnetID >= capacity {
			return nil, fmt.Errorf("%w: delegated LAN subnet ID does not fit the lease", ErrInput)
		}
		names[target.Interface], ids[target.SubnetID] = true, true
		address := base
		binary.BigEndian.PutUint64(address[:8], high|target.SubnetID)
		prefix := netip.PrefixFrom(netip.AddrFrom16(address), 64)
		address[15] = 1
		out = append(out, DelegationAssignment{
			Interface: target.Interface,
			Prefix:    prefix,
			Router:    netip.PrefixFrom(netip.AddrFrom16(address), 64),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out, nil
}
