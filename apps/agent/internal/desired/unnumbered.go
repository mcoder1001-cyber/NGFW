package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// unnumberedInterfaces validates the complete graph before projecting borrowing edges.
// VPP borrows both families; numbered donors in the same VRF prevent cycles and cross-table leaks.
func unnumberedInterfaces(s Sink, ifs map[string]*ngfwv1.Interface) {
	type entry struct {
		donor, vrf, pt string
		addresses      int
		dhcp           bool
	}
	entries := map[string]entry{}
	for name, itf := range ifs {
		entries[name] = entry{itf.GetUnnumbered(), itf.GetVrf(), Ptr("interfaces", name), len(itf.GetIpv4()) + len(itf.GetIpv6()), itf.DhcpClient != nil}
		for id, sub := range itf.GetSubinterfaces() {
			entries[SubName(name, id)] = entry{sub.GetUnnumbered(), sub.GetVrf(), Ptr("interfaces", name, "subinterfaces", id), len(sub.GetIpv4()) + len(sub.GetIpv6()), sub.DhcpClient != nil}
		}
	}
	for _, name := range sortedKeys(entries) {
		e := entries[name]
		if e.donor == "" {
			continue
		}
		d, ok := entries[e.donor]
		switch {
		case !ok:
			s.Errorf(e.pt+"/unnumbered", "interfaces.unnumbered-donor", "donor %q does not exist", e.donor)
		case e.donor == name || d.donor != "":
			s.Errorf(e.pt+"/unnumbered", "interfaces.unnumbered-cycle", "donor must be a different numbered interface")
		case unnumberedVRF(e.vrf) != unnumberedVRF(d.vrf):
			s.Errorf(e.pt+"/unnumbered", "interfaces.unnumbered-vrf", "donor and borrower must share the same VRF")
		case e.addresses != 0 || e.dhcp:
			s.Errorf(e.pt+"/unnumbered", "interfaces.unnumbered-exclusive", "borrower cannot have addresses or DHCP")
		default:
			s.Add(scheduler.Join(iface.UnnumberedName, name), &iface.Unnumbered{Interface: string(iface.AliasKey(name)), Donor: string(iface.AliasKey(e.donor))}, e.pt+"/unnumbered")
		}
	}
}
func (n *node) setUnnumbered(donor string) {
	if n.sub != nil {
		n.sub.Unnumbered = proto.String(donor)
	} else {
		n.itf.Unnumbered = proto.String(donor)
	}
}

func unnumberedVRF(name string) string {
	if name == "" {
		return "default"
	}
	return name
}
