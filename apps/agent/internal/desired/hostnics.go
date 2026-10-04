package desired

// F-default-vpp-nics (D-164): which seeded physical NICs are not bound to the data plane yet.

import (
	"sort"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// SplitUnboundPhysical returns ifs without the seeded physical NICs (KindExisting + `physical`) that the data plane
// does not have: every NIC released to the host (`owner: "host"`, never handed to DPDK: released), and every
// engine-owned NIC that is still a Linux kernel NIC — the netdev lookup finds a device of that name with no rtnetlink
// kind (a hardware NIC): unbound. Names are sorted. Such a NIC has no VPP counterpart until it is handed to DPDK, so its objects must not be
// projected (they would fail to apply); the caller reports agent.nic-not-bound instead. A linux-cp tap ("tun"),
// veth, bridge… of the same name is VPP-side plumbing, not the unbound NIC (review R2R4 #3). A lookup error or a
// nil lookup keeps the row (the alias Create then reports a missing interface as usual).
func SplitUnboundPhysical(ifs map[string]*ngfwv1.Interface, lookup NetdevKind) (kept map[string]*ngfwv1.Interface, unbound, released []string) {
	kept = make(map[string]*ngfwv1.Interface, len(ifs))
	for name, itf := range ifs {
		if k, _ := KindOf(name); k == KindExisting && itf.GetPhysical() != nil {
			if itf.GetPhysical().GetOwner() == "host" {
				released = append(released, name)
				continue
			}
			if lookup == nil {
				kept[name] = itf
				continue
			}
			if kind, exists, err := lookup(name); err == nil && exists && kind == "" {
				unbound = append(unbound, name)
				continue
			}
		}
		kept[name] = itf
	}
	sort.Strings(unbound)
	sort.Strings(released)
	return kept, unbound, released
}
