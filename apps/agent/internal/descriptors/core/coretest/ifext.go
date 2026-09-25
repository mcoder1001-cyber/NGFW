package coretest

// P08 extension of the model: what the interfaces domain needs beyond P05's core — admin state,
// per-protocol MTU, rx mode, MAC, promiscuous mode, af_packet host-interfaces, sub-interfaces and
// the DHCPv4 client (dhcp.go, TD-24) — so the agent's projection, Retrieve and restart paths run in unit
// tests with DF-1's real descriptors. Values follow what the host VPP 26.06 reports (af_packet:
// link MTU 9000, sw MTU 9000/0/0/0, rx mode interrupt; sub-interface MTU 0/0/0/0).

import (
	"go.fd.io/govpp/api"

	afpapi "ngfw/agent/binapi/af_packet"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
)

// RetvalAlreadyExists is VPP's retval for an existing object (VNET_API_ERROR_IF_ALREADY_EXISTS / subif exists).
const RetvalAlreadyExists int32 = -80

func detailsOf(i *Iface) *interfaces.SwInterfaceDetails {
	d := &interfaces.SwInterfaceDetails{
		SwIfIndex: interface_types.InterfaceIndex(i.Index), InterfaceName: i.Name, InterfaceDevType: i.DevType, Tag: i.Tag,
		LinkMtu: i.LinkMtu, Mtu: append([]uint32(nil), i.Mtu[:]...), L2Address: i.L2, SupSwIfIndex: i.Index,
	}
	if i.AdminUp {
		d.Flags = interface_types.IF_STATUS_API_FLAG_ADMIN_UP | interface_types.IF_STATUS_API_FLAG_LINK_UP
	}
	if i.IsSub {
		d.Type = interface_types.IF_API_TYPE_SUB
		d.SupSwIfIndex = i.Sup
		d.SubID = i.SubID
		d.SubIfFlags = i.SubFlags
		d.SubOuterVlanID = i.Outer
		d.SubInnerVlanID = i.Inner
		switch {
		case i.SubFlags&interface_types.SUB_IF_API_FLAG_TWO_TAGS != 0:
			d.SubNumberOfTags = 2
		case i.SubFlags&interface_types.SUB_IF_API_FLAG_ONE_TAG != 0:
			d.SubNumberOfTags = 1
		}
	}
	return d
}

// SetMtuFilter makes every later sw_interface_set_mtu store f(sw_if_index, requested) instead of the
// requested MTU (nil: store it as requested) — fault injection for "VPP did not take the value".
func (v *VPP) SetMtuFilter(f func(swIfIndex uint32, mtu [4]uint32) [4]uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.mtuFilter = f
}

func (v *VPP) installIfExt() {
	v.On("sw_interface_set_flags", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceSetFlags)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceSetFlagsReply{Retval: RetvalInvalidSwIfIndex})
		}
		i.AdminUp = req.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0
		return reply(&interfaces.SwInterfaceSetFlagsReply{})
	})
	v.On("sw_interface_set_mtu", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceSetMtu)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceSetMtuReply{Retval: RetvalInvalidSwIfIndex})
		}
		copy(i.Mtu[:], req.Mtu)
		if v.mtuFilter != nil {
			i.Mtu = v.mtuFilter(i.Index, i.Mtu)
		}
		return reply(&interfaces.SwInterfaceSetMtuReply{})
	})
	v.On("sw_interface_set_rx_mode", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceSetRxMode)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceSetRxModeReply{Retval: RetvalInvalidSwIfIndex})
		}
		i.RxMode = req.Mode
		return reply(&interfaces.SwInterfaceSetRxModeReply{})
	})
	v.On("sw_interface_rx_placement_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceRxPlacementDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			i := v.Ifaces[idx]
			if idx == 0 || i.IsSub || (uint32(req.SwIfIndex) != ^uint32(0) && uint32(req.SwIfIndex) != idx) {
				continue
			}
			out = append(out, &interfaces.SwInterfaceRxPlacementDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Mode: i.RxMode})
		}
		return out, nil
	})
	v.On("sw_interface_set_mac_address", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.SwInterfaceSetMacAddress)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.SwInterfaceSetMacAddressReply{Retval: RetvalInvalidSwIfIndex})
		}
		i.L2 = req.MacAddress
		return reply(&interfaces.SwInterfaceSetMacAddressReply{})
	})
	v.On("sw_interface_set_promisc", func(api.Message) ([]api.Message, error) {
		return reply(&interfaces.SwInterfaceSetPromiscReply{})
	})
	v.On("create_subif", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.CreateSubif)
		v.mu.Lock()
		defer v.mu.Unlock()
		p, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&interfaces.CreateSubifReply{Retval: RetvalInvalidSwIfIndex})
		}
		name := p.Name + "." + itoa(req.SubID)
		for _, i := range v.Ifaces {
			if i.Name == name {
				return reply(&interfaces.CreateSubifReply{Retval: RetvalAlreadyExists})
			}
		}
		idx := v.next
		v.next++
		v.Ifaces[idx] = &Iface{
			Index: idx, Name: name, DevType: p.DevType, Addrs: map[string]bool{}, LinkMtu: p.LinkMtu, L2: p.L2,
			IsSub: true, Sup: p.Index, SubID: req.SubID, SubFlags: req.SubIfFlags, Outer: req.OuterVlanID, Inner: req.InnerVlanID,
		}
		return reply(&interfaces.CreateSubifReply{SwIfIndex: interface_types.InterfaceIndex(idx)})
	})
	v.On("delete_subif", func(m api.Message) ([]api.Message, error) {
		req := m.(*interfaces.DeleteSubif)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.SwIfIndex)]
		if !ok || !i.IsSub {
			return reply(&interfaces.DeleteSubifReply{Retval: RetvalInvalidSwIfIndex})
		}
		v.dropInterfaceLocked(i)
		return reply(&interfaces.DeleteSubifReply{})
	})
	v.On("af_packet_create_v3", func(m api.Message) ([]api.Message, error) {
		req := m.(*afpapi.AfPacketCreateV3)
		v.mu.Lock()
		defer v.mu.Unlock()
		name := "host-" + req.HostIfName
		for _, i := range v.Ifaces {
			if i.Name == name {
				return reply(&afpapi.AfPacketCreateV3Reply{Retval: RetvalAlreadyExists})
			}
		}
		idx := v.next
		v.next++
		v.Ifaces[idx] = &Iface{
			Index: idx, Name: name, DevType: "af-packet", HostIf: req.HostIfName, Addrs: map[string]bool{},
			LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_INTERRUPT, L2: [6]uint8{0x02, 0xfe, 0, 0, 0, uint8(idx)}, //nolint:gosec // G115: a fake MAC byte; test indexes are small
		}
		return reply(&afpapi.AfPacketCreateV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)})
	})
	v.On("af_packet_delete", func(m api.Message) ([]api.Message, error) {
		req := m.(*afpapi.AfPacketDelete)
		v.mu.Lock()
		defer v.mu.Unlock()
		for _, i := range v.Ifaces {
			if i.DevType == "af-packet" && !i.IsSub && i.HostIf == req.HostIfName {
				v.dropInterfaceLocked(i)
				return reply(&afpapi.AfPacketDeleteReply{})
			}
		}
		return reply(&afpapi.AfPacketDeleteReply{Retval: RetvalInvalidSwIfIndex})
	})
	v.On("af_packet_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			if i := v.Ifaces[idx]; i.DevType == "af-packet" && !i.IsSub {
				out = append(out, &afpapi.AfPacketDetails{SwIfIndex: interface_types.InterfaceIndex(idx), HostIfName: i.HostIf})
			}
		}
		return out, nil
	})
	v.installDHCPClient() // TD-24: the DHCPv4 client and its lease (dhcp.go)
}

func itoa(n uint32) string {
	var b [10]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	return string(b[i:])
}

// AdminUp reports the modelled admin state of an interface (tests).
func (v *VPP) AdminUp(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == name {
			return i.AdminUp
		}
	}
	return false
}

// MTU returns the modelled L3 MTU of an interface (tests).
func (v *VPP) MTU(name string) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == name {
			return i.Mtu[0]
		}
	}
	return 0
}

// DeleteHostInterface removes the af_packet interface on netdev and its sub-interfaces behind the
// agent's back (simulated loss).
func (v *VPP) DeleteHostInterface(netdev string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.DevType == "af-packet" && !i.IsSub && i.HostIf == netdev {
			for _, s := range v.Ifaces {
				if s.IsSub && s.Sup == i.Index {
					v.dropInterfaceLocked(s)
				}
			}
			v.dropInterfaceLocked(i)
			return true
		}
	}
	return false
}
