package coretest

// P12 addition to the model (wave-A-hotspots A6: a new file, the existing ones stay read-only): the linux_cp plugin's
// pairs (lcp_itf_pair_add_del_v3, lcp_itf_pair_get, lcp_default_ns_get), installed on every model through the
// `extensions` seam so agent tests of every domain retrieve lcp.itf-pair unchanged. A pair add creates VPP's end of the
// host tap as an interface of the model ("tap<N>", untagged — DF-8 L3), a delete removes it; the stored namespace is
// the effective one (the default namespace when the request names none), as VPP reports it.

import (
	"fmt"
	"sort"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lcp"
)

// LcpPair is one modelled pair (guarded by VPP.mu).
type LcpPair struct {
	Phy, Host  uint32
	VifIndex   uint32
	HostIfName string
	HostIfType lcp.LcpItfHostType
	Netns      string
}

// lcpModel is the plugin state.
type lcpModel struct {
	pairs     map[uint32]*LcpPair // by phy sw_if_index
	defaultNS string
	nextTap   uint32
}

var lcpState = map[*VPP]*lcpModel{}

func init() { extensions = append(extensions, (*VPP).installLcp) }

// LcpPairs returns the modelled pairs sorted by phy sw_if_index.
func (v *VPP) LcpPairs() []LcpPair {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := lcpState[v]
	var out []LcpPair
	for _, p := range m.pairs {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Phy < out[j].Phy })
	return out
}

func (v *VPP) installLcp() {
	v.mu.Lock()
	m := &lcpModel{pairs: map[uint32]*LcpPair{}}
	lcpState[v] = m
	v.mu.Unlock()
	v.On("lcp_default_ns_get", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		return reply(&lcp.LcpDefaultNsGetReply{Netns: m.defaultNS})
	})
	v.On("lcp_itf_pair_add_del_v3", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*lcp.LcpItfPairAddDelV3)
		v.mu.Lock()
		defer v.mu.Unlock()
		phy := uint32(r.SwIfIndex)
		p, exists := m.pairs[phy]
		switch {
		case r.IsAdd && exists:
			return reply(&lcp.LcpItfPairAddDelV3Reply{Retval: int32(api.VALUE_EXIST)})
		case !r.IsAdd && !exists:
			return reply(&lcp.LcpItfPairAddDelV3Reply{Retval: int32(api.INVALID_SW_IF_INDEX)})
		case !r.IsAdd:
			if host, ok := v.Ifaces[p.Host]; ok {
				v.dropInterfaceLocked(host)
			}
			delete(m.pairs, phy)
			return reply(&lcp.LcpItfPairAddDelV3Reply{})
		}
		if _, ok := v.Ifaces[phy]; !ok {
			return reply(&lcp.LcpItfPairAddDelV3Reply{Retval: int32(api.INVALID_SW_IF_INDEX)})
		}
		ns := r.Netns
		if ns == "" {
			ns = m.defaultNS
		}
		idx := v.next
		v.next++
		name := fmt.Sprintf("tap%d", 4096+m.nextTap)
		m.nextTap++
		v.Ifaces[idx] = &Iface{Index: idx, Name: name, DevType: "virtio", Addrs: map[string]bool{}, AdminUp: true,
			LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_POLLING}
		np := &LcpPair{Phy: phy, Host: idx, VifIndex: 100 + idx, HostIfName: r.HostIfName, HostIfType: r.HostIfType, Netns: ns}
		m.pairs[phy] = np
		return reply(&lcp.LcpItfPairAddDelV3Reply{HostSwIfIndex: interface_types.InterfaceIndex(idx), VifIndex: np.VifIndex})
	})
	v.On("lcp_itf_pair_get", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		phys := make([]uint32, 0, len(m.pairs))
		for k := range m.pairs {
			phys = append(phys, k)
		}
		sort.Slice(phys, func(i, j int) bool { return phys[i] < phys[j] })
		var out []api.Message
		for _, k := range phys {
			p := m.pairs[k]
			out = append(out, &lcp.LcpItfPairDetails{PhySwIfIndex: interface_types.InterfaceIndex(p.Phy),
				HostSwIfIndex: interface_types.InterfaceIndex(p.Host), VifIndex: p.VifIndex, HostIfName: p.HostIfName,
				HostIfType: p.HostIfType, Netns: p.Netns})
		}
		return append(out, &lcp.LcpItfPairGetReply{Cursor: ^uint32(0)}), nil
	})
}
