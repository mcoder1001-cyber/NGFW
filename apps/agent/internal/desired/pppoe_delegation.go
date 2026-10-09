package desired

import (
	"fmt"
	"net/netip"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

const (
	PppoeDelegationAddress = "pppoe.pd.address"
	PppoeDelegationPrefix  = "pppoe.pd.prefix"
	PppoeDelegationRA      = "pppoe.pd.ra"
)

// PppoeDelegationLease is a verified current-carrier lease, never a raw hook up event.
type PppoeDelegationLease struct {
	Logical, Generation        string
	Delegated                  netip.Prefix
	ValidUntil, PreferredUntil time.Time
	Ready                      bool
}

// PppoeDelegationTargets validates the complete public plan again at the agent boundary.
func PppoeDelegationTargets(doc *ngfwv1.DesiredState, logical string) ([]pppoe.DelegationTarget, error) {
	iface := doc.GetInterfaces()[logical]
	cfg := iface.GetPppoe()
	if cfg == nil || len(cfg.GetDelegationTargets()) == 0 {
		return nil, nil
	}
	if cfg.GetIpv6() != "dhcpv6" {
		return nil, fmt.Errorf("delegated LANs require DHCPv6")
	}
	seen, ids := map[string]bool{}, map[uint32]bool{}
	var targets []pppoe.DelegationTarget
	for _, target := range cfg.GetDelegationTargets() {
		name := target.GetInterface()
		lan := doc.GetInterfaces()[name]
		if lan == nil || !lan.GetEnabled() || lan.GetPppoe() != nil || name == logical || name == cfg.GetParent() {
			return nil, fmt.Errorf("delegated LAN must be an enabled interface distinct from PPP and its parent")
		}
		if len(lan.GetIpv6()) != 0 || lan.GetIpv6Ra() != nil || lan.GetL2() != nil || lan.GetUnnumbered() != "" || lan.GetVrf() != iface.GetVrf() {
			return nil, fmt.Errorf("delegated LAN conflicts with static IPv6, RA or VRF")
		}
		if seen[name] || ids[target.GetSubnetId()] {
			return nil, fmt.Errorf("duplicate delegated LAN or subnet ID")
		}
		for other, itf := range doc.GetInterfaces() {
			if other == logical {
				continue
			}
			for _, candidate := range itf.GetPppoe().GetDelegationTargets() {
				if candidate.GetInterface() == name {
					return nil, fmt.Errorf("delegated LAN is assigned to another PPP client")
				}
			}
		}
		seen[name], ids[target.GetSubnetId()] = true, true
		targets = append(targets, pppoe.DelegationTarget{Interface: name, SubnetID: uint64(target.GetSubnetId())})
	}
	return targets, nil
}

// PppoeDelegation emits only live complete assignments. Omission withdraws old owned
// objects in the scheduler transaction, including disable/removal/rollback and expiry.
// Preferred expiry conservatively withdraws: the existing RA descriptor normalizes
// zero preferred lifetime to a static default, so it must never receive zero here.
func PppoeDelegation(doc *ngfwv1.DesiredState, leases []PppoeDelegationLease, now time.Time) []scheduler.KV {
	var out []scheduler.KV
	seen := map[string]bool{}
	for _, lease := range leases {
		cfg := doc.GetInterfaces()[lease.Logical].GetPppoe()
		if seen[lease.Logical] || !lease.Ready || lease.Generation == "" || cfg == nil ||
			(cfg.Enabled != nil && !cfg.GetEnabled()) || !doc.GetInterfaces()[lease.Logical].GetEnabled() ||
			lease.PreferredUntil.After(lease.ValidUntil) {
			continue
		}
		seen[lease.Logical] = true
		valid, preferred := int64(lease.ValidUntil.Sub(now)/time.Second), int64(lease.PreferredUntil.Sub(now)/time.Second)
		// Round down to a 30-second budget so a one-second observation loop does
		// not rewrite every RA every second. Never advertise beyond the actual lease.
		valid, preferred = valid/30*30, preferred/30*30
		if preferred <= 0 || valid <= 0 || valid > 4294967295 {
			continue
		}
		targets, err := PppoeDelegationTargets(doc, lease.Logical)
		if err != nil {
			continue
		}
		assignments, err := pppoe.AllocateDelegation(lease.Delegated, targets)
		if err != nil || delegationOverlaps(doc, lease, assignments, leases, now) {
			continue
		}
		for _, a := range assignments {
			out = append(out,
				scheduler.KV{Key: scheduler.Join(PppoeDelegationAddress, a.Interface), Value: &core.InterfaceAddress{Interface: a.Interface, Prefix: a.Router.String()}},
				scheduler.KV{Key: scheduler.Join(PppoeDelegationPrefix, a.Interface), Value: &ip6nd.RaPrefix{Interface: a.Interface, Prefix: a.Prefix.String(), ValidLifetime: uint32(valid), PreferredLifetime: uint32(preferred)}},
				scheduler.KV{Key: scheduler.Join(PppoeDelegationRA, a.Interface), Value: RaConfigOf(a.Interface, &ngfwv1.Ipv6Ra{Suppress: boolPointer(false)})},
			)
		}
	}
	return out
}
func boolPointer(value bool) *bool { return &value }

func delegationOverlaps(doc *ngfwv1.DesiredState, lease PppoeDelegationLease, assignments []pppoe.DelegationAssignment, leases []PppoeDelegationLease, now time.Time) bool {
	vrf := doc.GetInterfaces()[lease.Logical].GetVrf()
	overlaps := func(prefix netip.Prefix) bool {
		for _, a := range assignments {
			if a.Prefix.Overlaps(prefix) {
				return true
			}
		}
		return false
	}
	for _, iface := range doc.GetInterfaces() {
		if iface.GetVrf() == vrf {
			for _, cidr := range iface.GetIpv6() {
				if prefix, err := netip.ParsePrefix(cidr); err == nil && overlaps(prefix) {
					return true
				}
			}
			for cidr := range iface.GetIpv6Ra().GetPrefixes() {
				if prefix, err := netip.ParsePrefix(cidr); err == nil && overlaps(prefix) {
					return true
				}
			}
		}
		for _, sub := range iface.GetSubinterfaces() {
			if sub.GetVrf() != vrf {
				continue
			}
			for _, cidr := range sub.GetIpv6() {
				if prefix, err := netip.ParsePrefix(cidr); err == nil && overlaps(prefix) {
					return true
				}
			}
			for cidr := range sub.GetIpv6Ra().GetPrefixes() {
				if prefix, err := netip.ParsePrefix(cidr); err == nil && overlaps(prefix) {
					return true
				}
			}
		}
	}
	for _, other := range leases {
		if other.Logical == lease.Logical || !other.Ready || other.Generation == "" || !now.Before(other.PreferredUntil) ||
			doc.GetInterfaces()[other.Logical].GetVrf() != vrf {
			continue
		}
		targets, err := PppoeDelegationTargets(doc, other.Logical)
		if err != nil {
			continue
		}
		allocated, err := pppoe.AllocateDelegation(other.Delegated, targets)
		if err != nil {
			continue
		}
		for _, a := range allocated {
			if overlaps(a.Prefix) {
				return true
			}
		}
	}
	return false
}

// IsPppoeDelegationKey excludes operational PD values from static configuration
// assembly, even when an all-descriptor read supplies both runtime and static KVs.
func IsPppoeDelegationKey(key scheduler.Key) bool {
	switch key.Descriptor() {
	case PppoeDelegationAddress, PppoeDelegationPrefix, PppoeDelegationRA:
		return true
	default:
		return false
	}
}
