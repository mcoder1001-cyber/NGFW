package coretest

// F-kea-dhcp-relay extension of the model: the DHCP relay (proxy) table of VPP's dhcp plugin — dhcp_proxy_config
// adds/removes a server of an rx VRF (one source address per rx VRF and family), dhcp_proxy_dump lists them the way
// VPP 26.06 does (only the address unions are filled; the family comes from is_ipv6), dhcp_proxy_set_vss stores the
// VSS of a relaying VRF. The agent's `services` domain Retrieve dumps it on every transaction.

import (
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/ip_types"
)

// DHCPProxy is one relay server of the model.
type DHCPProxy struct {
	IPv6      bool
	RxVRF     uint32
	ServerVRF uint32
	Server    ip_types.Address
	Src       ip_types.Address
}

type dhcpProxyModel struct {
	mu      sync.Mutex
	servers []DHCPProxy
	vss     map[bool]map[uint32]*dhcp.DHCPProxySetVss
}

var dhcpModels sync.Map // *VPP → *dhcpProxyModel

// DHCPProxies returns the relay servers of the model (tests: "deleted behind the agent's back" checks).
func (v *VPP) DHCPProxies() []DHCPProxy {
	m := v.dhcpModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DHCPProxy(nil), m.servers...)
}

// DeleteDHCPProxies drops every relay server of rx VRF rx (simulated loss behind the agent's back).
func (v *VPP) DeleteDHCPProxies(rx uint32) {
	m := v.dhcpModel()
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.servers[:0]
	for _, s := range m.servers {
		if s.RxVRF != rx {
			kept = append(kept, s)
		}
	}
	m.servers = kept
}

func (v *VPP) dhcpModel() *dhcpProxyModel {
	m, _ := dhcpModels.LoadOrStore(v, &dhcpProxyModel{vss: map[bool]map[uint32]*dhcp.DHCPProxySetVss{false: {}, true: {}}})
	return m.(*dhcpProxyModel)
}

func (v *VPP) installKeaDHCPRelay() {
	m := v.dhcpModel()
	v.On("dhcp_proxy_config", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp.DHCPProxyConfig)
		v6 := r.DHCPServer.Af == ip_types.ADDRESS_IP6
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, s := range m.servers {
			if s.IPv6 == v6 && s.RxVRF == r.RxVrfID && s.ServerVRF == r.ServerVrfID && s.Server == r.DHCPServer {
				if !r.IsAdd {
					m.servers = append(m.servers[:i], m.servers[i+1:]...)
				}
				return reply(&dhcp.DHCPProxyConfigReply{})
			}
		}
		if !r.IsAdd {
			return reply(&dhcp.DHCPProxyConfigReply{Retval: int32(api.NO_SUCH_ENTRY)})
		}
		m.servers = append(m.servers, DHCPProxy{IPv6: v6, RxVRF: r.RxVrfID, ServerVRF: r.ServerVrfID, Server: r.DHCPServer, Src: r.DHCPSrcAddress})
		return reply(&dhcp.DHCPProxyConfigReply{})
	})
	v.On("dhcp_proxy_set_vss", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*dhcp.DHCPProxySetVss)
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.IsAdd {
			m.vss[r.IsIPv6][r.TblID] = r
		} else {
			if _, ok := m.vss[r.IsIPv6][r.TblID]; !ok {
				return reply(&dhcp.DHCPProxySetVssReply{Retval: int32(api.NO_SUCH_ENTRY)})
			}
			delete(m.vss[r.IsIPv6], r.TblID)
		}
		return reply(&dhcp.DHCPProxySetVssReply{})
	})
	v.On("dhcp_proxy_dump", func(msg api.Message) ([]api.Message, error) {
		v6 := msg.(*dhcp.DHCPProxyDump).IsIP6
		m.mu.Lock()
		defer m.mu.Unlock()
		byVRF := map[uint32]*dhcp.DHCPProxyDetails{}
		for _, s := range m.servers {
			if s.IPv6 != v6 {
				continue
			}
			d, ok := byVRF[s.RxVRF]
			if !ok {
				d = &dhcp.DHCPProxyDetails{RxVrfID: s.RxVRF, IsIPv6: v6, VssType: dhcp.VSS_TYPE_API_INVALID, DHCPSrcAddress: ip_types.Address{Un: s.Src.Un}}
				if x := m.vss[v6][s.RxVRF]; x != nil {
					d.VssType, d.VssVPNAsciiID, d.VssOui, d.VssFibID = x.VssType, x.VPNAsciiID, x.Oui, x.VPNIndex
				}
				byVRF[s.RxVRF] = d
			}
			d.Servers = append(d.Servers, dhcp.DHCPServer{ServerVrfID: s.ServerVRF, DHCPServer: ip_types.Address{Un: s.Server.Un}})
			d.Count = uint8(len(d.Servers)) //nolint:gosec // test model, few servers
		}
		vrfs := make([]uint32, 0, len(byVRF))
		for id := range byVRF {
			vrfs = append(vrfs, id)
		}
		sort.Slice(vrfs, func(i, j int) bool { return vrfs[i] < vrfs[j] })
		out := make([]api.Message, 0, len(vrfs))
		for _, id := range vrfs {
			out = append(out, byVRF[id])
		}
		return out, nil
	})
}
