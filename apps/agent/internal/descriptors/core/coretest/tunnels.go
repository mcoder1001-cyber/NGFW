package coretest

// F-tunnels extension of the model (A6: own file, installed through the extensions seam): the gre,
// ipip and vxlan plugins as far as DF-6's tunnel descriptors use them — add/del creating a modelled
// Iface named gre<n> / ipip<n> / vxlan_tunnel<n> with VPP 26.06's device class, the dumps, and
// VPP's refusal of a duplicate instance. A VXLAN L2 tunnel gets a MAC, an L3 one none (the v2 dump
// does not report is_l3; the descriptor reads it from the interface). Agent tests of other domains
// retrieve the (empty) tunnel families unchanged.

import (
	"fmt"
	"sync"

	"go.fd.io/govpp/api"

	greapi "ngfw/agent/binapi/gre"
	"ngfw/agent/binapi/interface_types"
	ipipapi "ngfw/agent/binapi/ipip"
	vxlanapi "ngfw/agent/binapi/vxlan"
)

type tunnelModel struct {
	gre   map[uint32]greapi.GreTunnelV2
	ipip  map[uint32]ipipapi.IpipTunnel
	vxlan map[uint32]vxlanapi.VxlanTunnelV2Details
}

var tunnelModels sync.Map

func init() { RegisterExtension("tunnels", installTunnels) }

// TunnelCount is the number of gre + ipip + vxlan tunnels in the model (all owners).
func (v *VPP) TunnelCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, _ := tunnelModels.Load(v)
	t, _ := m.(*tunnelModel)
	if t == nil {
		return 0
	}
	t.pruneLocked(v)
	return len(t.gre) + len(t.ipip) + len(t.vxlan)
}

// pruneLocked forgets tunnels whose interface was removed behind the agent's back (DeleteInterface).
func (m *tunnelModel) pruneLocked(v *VPP) {
	for idx := range m.gre {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.gre, idx)
		}
	}
	for idx := range m.ipip {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.ipip, idx)
		}
	}
	for idx := range m.vxlan {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(m.vxlan, idx)
		}
	}
}

func (v *VPP) newTunnelIfaceLocked(name, devType string, l2 bool) (uint32, bool) {
	for _, i := range v.Ifaces {
		if i.Name == name {
			return 0, false
		}
	}
	idx := v.next
	v.next++
	i := &Iface{Index: idx, Name: name, DevType: devType, Addrs: map[string]bool{}, LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_POLLING}
	if l2 {
		i.L2 = [6]uint8{0x02, 0xfe, 0, 0, byte(idx >> 8), byte(idx)} //nolint:gosec // a MAC from the low index bytes
	}
	v.Ifaces[idx] = i
	return idx, true
}

func (v *VPP) dropTunnelIfaceLocked(idx uint32) {
	if i, ok := v.Ifaces[idx]; ok {
		v.dropInterfaceLocked(i)
	}
}

func installTunnels(v *VPP) {
	m := &tunnelModel{gre: map[uint32]greapi.GreTunnelV2{}, ipip: map[uint32]ipipapi.IpipTunnel{}, vxlan: map[uint32]vxlanapi.VxlanTunnelV2Details{}}
	tunnelModels.Store(v, m)
	v.On("gre_tunnel_add_del_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*greapi.GreTunnelAddDelV2)
		v.mu.Lock()
		defer v.mu.Unlock()
		if r.IsAdd {
			idx, ok := v.newTunnelIfaceLocked(fmt.Sprintf("gre%d", r.Tunnel.Instance), "GRE tunnel device", r.Tunnel.Type != greapi.GRE_API_TUNNEL_TYPE_L3)
			if !ok {
				return reply(&greapi.GreTunnelAddDelV2Reply{Retval: RetvalInstanceInUse})
			}
			t := r.Tunnel
			t.SwIfIndex = interface_types.InterfaceIndex(idx)
			m.gre[idx] = t
			return reply(&greapi.GreTunnelAddDelV2Reply{SwIfIndex: t.SwIfIndex})
		}
		for idx, t := range m.gre {
			if t.Src == r.Tunnel.Src && t.Dst == r.Tunnel.Dst && t.OuterTableID == r.Tunnel.OuterTableID && t.Instance == r.Tunnel.Instance {
				delete(m.gre, idx)
				v.dropTunnelIfaceLocked(idx)
				return reply(&greapi.GreTunnelAddDelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return reply(&greapi.GreTunnelAddDelV2Reply{Retval: RetvalNoSuchEntry})
	})
	v.On("gre_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m.pruneLocked(v)
		var out []api.Message
		for _, idx := range sortedU32(m.gre) {
			out = append(out, &greapi.GreTunnelV2Details{Tunnel: m.gre[idx]})
		}
		return out, nil
	})
	v.On("ipip_add_tunnel", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ipipapi.IpipAddTunnel)
		v.mu.Lock()
		defer v.mu.Unlock()
		idx, ok := v.newTunnelIfaceLocked(fmt.Sprintf("ipip%d", r.Tunnel.Instance), "IPIP tunnel device", false)
		if !ok {
			return reply(&ipipapi.IpipAddTunnelReply{Retval: RetvalInstanceInUse})
		}
		t := r.Tunnel
		t.SwIfIndex = interface_types.InterfaceIndex(idx)
		m.ipip[idx] = t
		return reply(&ipipapi.IpipAddTunnelReply{SwIfIndex: t.SwIfIndex})
	})
	v.On("ipip_del_tunnel", func(msg api.Message) ([]api.Message, error) {
		idx := uint32(msg.(*ipipapi.IpipDelTunnel).SwIfIndex)
		v.mu.Lock()
		defer v.mu.Unlock()
		if _, ok := m.ipip[idx]; !ok {
			return reply(&ipipapi.IpipDelTunnelReply{Retval: RetvalNoSuchEntry})
		}
		delete(m.ipip, idx)
		v.dropTunnelIfaceLocked(idx)
		return reply(&ipipapi.IpipDelTunnelReply{})
	})
	v.On("ipip_tunnel_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m.pruneLocked(v)
		var out []api.Message
		for _, idx := range sortedU32(m.ipip) {
			out = append(out, &ipipapi.IpipTunnelDetails{Tunnel: m.ipip[idx]})
		}
		return out, nil
	})
	v.On("vxlan_add_del_tunnel_v3", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*vxlanapi.VxlanAddDelTunnelV3)
		v.mu.Lock()
		defer v.mu.Unlock()
		if r.IsAdd {
			idx, ok := v.newTunnelIfaceLocked(fmt.Sprintf("vxlan_tunnel%d", r.Instance), "VXLAN", !r.IsL3)
			if !ok {
				return reply(&vxlanapi.VxlanAddDelTunnelV3Reply{Retval: RetvalInstanceInUse})
			}
			port := func(p uint16) uint16 {
				if p == 0 {
					return 4789
				}
				return p
			}
			m.vxlan[idx] = vxlanapi.VxlanTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), Instance: r.Instance, SrcAddress: r.SrcAddress,
				DstAddress: r.DstAddress, SrcPort: port(r.SrcPort), DstPort: port(r.DstPort), McastSwIfIndex: r.McastSwIfIndex, EncapVrfID: r.EncapVrfID,
				DecapNextIndex: r.DecapNextIndex, Vni: r.Vni}
			return reply(&vxlanapi.VxlanAddDelTunnelV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)})
		}
		for idx, t := range m.vxlan {
			if t.SrcAddress == r.SrcAddress && t.DstAddress == r.DstAddress && t.Vni == r.Vni && t.EncapVrfID == r.EncapVrfID {
				delete(m.vxlan, idx)
				v.dropTunnelIfaceLocked(idx)
				return reply(&vxlanapi.VxlanAddDelTunnelV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return reply(&vxlanapi.VxlanAddDelTunnelV3Reply{Retval: RetvalNoSuchEntry})
	})
	v.On("vxlan_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		m.pruneLocked(v)
		var out []api.Message
		for _, idx := range sortedU32(m.vxlan) {
			d := m.vxlan[idx]
			out = append(out, &d)
		}
		return out, nil
	})
}
