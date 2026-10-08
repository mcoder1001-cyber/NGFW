package desired

import (
	"net/netip"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

// KernelPppoeEnabled selects an enabled logical PPP interface. Disabled legacy
// configuration remains storable; enabling it requires an explicit distinct parent.
func KernelPppoeEnabled(itf *ngfwv1.Interface) bool {
	p := itf.GetPppoe()
	return p != nil && (p.Enabled == nil || p.GetEnabled())
}

// PppoeCarriers projects the complete transport dependency chain separately from
// the daemon stage. Logical names resolve to the IP transit TAP for every existing
// routing/NAT/firewall descriptor; the raw parent is never shadowed or retagged.
func PppoeCarriers(s Sink, ifs map[string]*ngfwv1.Interface, owner string) {
	var specs []pppoe.CarrierSpec
	var reserved []netip.Prefix
	valid := true
	for _, name := range sortedKeys(ifs) {
		itf := ifs[name]
		for _, raw := range append(append([]string(nil), itf.GetIpv4()...), itf.GetIpv6()...) {
			if p, err := netip.ParsePrefix(raw); err == nil {
				reserved = append(reserved, p)
			}
		}
		if !KernelPppoeEnabled(itf) {
			continue
		}
		if _, err := PppoeDelegationTargets(&ngfwv1.DesiredState{Interfaces: ifs}, name); err != nil {
			s.Errorf(Ptr("interfaces", name, "pppoe", "delegation"), "pppoe.delegation-invalid", "%v", err)
			valid = false
			continue
		}
		p := itf.GetPppoe()
		pt := Ptr("interfaces", name, "pppoe")
		parent := ifs[p.GetParent()]
		if p.GetParent() == "" || p.GetParent() == name || parent == nil {
			s.Errorf(pt+"/parent", "pppoe.carrier-parent", "kernel PPP requires a distinct logical interface and an explicit existing raw parent; migrate the PPP configuration and its policy references to a logical PPP interface")
			valid = false
			continue
		}
		if parent.Unnumbered != nil || len(parent.GetSubinterfaces()) != 0 || parent.GetBond() != nil || parent.GetLcp() != nil || parent.GetL2() != nil || parent.GetPppoe() != nil || len(parent.GetIpv4()) != 0 || len(parent.GetIpv6()) != 0 || parent.GetDhcpClient() != nil {
			s.Errorf(pt+"/parent", "pppoe.carrier-exclusive", "raw PPP parent must have no LCP, L2, PPP, DHCP or static IP configuration")
			valid = false
			continue
		}
		inBond := false
		for _, other := range ifs {
			if _, ok := other.GetBond().GetMembers()[p.GetParent()]; ok {
				inBond = true
				break
			}
		}
		if inBond {
			s.Errorf(pt+"/parent", "pppoe.carrier-exclusive", "raw PPP parent belongs to a bond")
			valid = false
			continue
		}
		if itf.GetPhysical() != nil || itf.GetBond() != nil || itf.GetLcp() != nil || itf.GetL2() != nil || len(itf.GetSubinterfaces()) != 0 || itf.Unnumbered != nil {
			s.Errorf(pt, "pppoe.carrier-logical", "the PPP logical interface is an owned IP transit, not a physical, bonded, LCP, L2, unnumbered or VLAN parent")
			valid = false
			continue
		}
		if len(itf.GetIpv4()) != 0 || len(itf.GetIpv6()) != 0 || itf.GetDhcpClient() != nil {
			s.Errorf(pt, "pppoe.carrier-address", "logical PPP addresses are negotiated and cannot have static or DHCP addressing")
			valid = false
			continue
		}
		mtu := p.GetMtu()
		if mtu == 0 {
			mtu = 1492
		}
		if mtu < 1280 && p.GetIpv6() != "" && p.GetIpv6() != "off" {
			s.Errorf(pt+"/mtu", "pppoe.ipv6-mtu", "IPv6 PPP requires MTU at least1280")
			valid = false
			continue
		}
		spec, err := pppoe.NewCarrierSpec(owner, name, p.GetParent(), mtu)
		if err != nil {
			s.Errorf(pt, "pppoe.carrier-spec", "%v", err)
			valid = false
			continue
		}
		if itf.Mtu != nil && itf.GetMtu() != spec.MTU {
			s.Errorf(pt+"/mtu", "pppoe.carrier-mtu", "logical interface MTU must equal PPP MTU")
			valid = false
			continue
		}
		specs = append(specs, spec)
	}
	if !valid {
		return
	}
	if err := pppoe.CheckCarrierPrefixes(specs, reserved); err != nil {
		s.Errorf("/interfaces", "pppoe.carrier-overlap", "%v", err)
		return
	}
	ids := map[uint32]bool{}
	for _, spec := range specs {
		raw, transit := spec.TapIDs()
		if ids[raw] || ids[transit] {
			s.Errorf("/interfaces", "pppoe.carrier-id", "internal PPP TAP identifiers collide")
			return
		}
		ids[raw], ids[transit] = true, true
	}
	for _, spec := range specs {
		pt := Ptr("interfaces", spec.Logical, "pppoe")
		s.Add(pppoedesc.CarrierNamespaceKey(spec.Token()), dfkit.Encode(spec), pt)
		rawID, transitID := spec.TapIDs()
		rawKey := scheduler.Join(pppoedesc.CarrierTapName, spec.RawLogical())
		s.Add(rawKey, &tapv2.Tap{Name: spec.RawLogical(), Id: rawID, HostIfName: spec.RawHost(), HostNamespace: spec.Token(), HostMtu: spec.MTU + 8, RxRingSize: 256, TxRingSize: 256}, pt)
		transit := &tapv2.Tap{Name: spec.Logical, Id: transitID, HostIfName: spec.TransitHost(), HostNamespace: spec.Token(), HostMtu: spec.MTU, HostIp4Prefix: spec.Host4, HostIp6Prefix: spec.Host6, RxRingSize: 256, TxRingSize: 256}
		if spec.MTU < 1280 {
			transit.HostIp6Prefix = ""
		}
		s.Add(scheduler.Join(pppoedesc.CarrierTapName, spec.Logical), transit, pt)
		s.Add(iface.AliasKey(spec.RawLogical()), &iface.InterfaceAlias{Name: spec.RawLogical(), Creator: string(scheduler.Join(tapv2.TapName, spec.RawLogical()))}, pt)
		s.Add(scheduler.Join(iface.AdminStateName, spec.RawLogical()), &iface.AdminState{Interface: string(iface.AliasKey(spec.RawLogical()))}, pt)
		for _, pair := range [][2]string{{spec.Parent, spec.RawLogical()}, {spec.RawLogical(), spec.Parent}} {
			rx, tx := string(iface.AliasKey(pair[0])), string(iface.AliasKey(pair[1]))
			s.Add(l2.XconnectKey(rx), &l2.Xconnect{Rx: rx, Tx: tx}, pt)
		}
		addresses := []string{spec.VPP4()}
		if spec.MTU >= 1280 {
			addresses = append(addresses, spec.VPP6())
		}
		for _, address := range addresses {
			s.Add(core.InterfaceAddrKey(spec.Logical, address), &core.InterfaceAddress{Interface: spec.Logical, Prefix: address}, pt)
		}
		// Advertised/negotiated MTU must match the transit. An absent general MTU
		// inherits PPP MTU; a conflicting explicit value is a validation error.
		itf := ifs[spec.Logical]
		if itf.Mtu == nil {
			s.Add(scheduler.Join(iface.MtuName, spec.Logical), &iface.Mtu{Interface: string(iface.AliasKey(spec.Logical)), Mtu: spec.MTU}, pt+"/mtu")
		}
	}
}
