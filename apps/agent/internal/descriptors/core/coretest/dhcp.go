package coretest

// TD-24: VPP's DHCPv4 client (src/plugins/dhcp/client.c, dhcp_api.c) as far as the agent can see it.
//   - dhcp_client_config is_add adds a client in DISCOVER on an interface: one per interface, so a
//     second add, or a delete where there is none, is INVALID_VALUE (dhcp_client_add_del).
//   - BindLease is the BOUND transition (dhcp_client_acquire_address). VPP installs the lease with the
//     ordinary interface-address call, so ip_address_dump reports it like any other address, and
//     dhcp_client_dump reports it as lease.host_address/mask_width (client->installed).
//   - Deleting the client releases the lease address (dhcp_client_reset → release_address). A deleted
//     interface takes its client with it.

import (
	"fmt"
	"net/netip"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
)

// DHCPClient is the modelled DHCPv4 client of one interface (Iface.DHCP).
type DHCPClient struct {
	Config dhcp.DHCPClient // as configured (dhcp_client_config.client)
	Lease  netip.Prefix    // the installed lease; invalid until BindLease
}

func (v *VPP) installDHCPClient() {
	v.On("dhcp_client_config", func(m api.Message) ([]api.Message, error) {
		req := m.(*dhcp.DHCPClientConfig)
		v.mu.Lock()
		defer v.mu.Unlock()
		i, ok := v.Ifaces[uint32(req.Client.SwIfIndex)]
		if !ok {
			return reply(&dhcp.DHCPClientConfigReply{Retval: RetvalInvalidSwIfIndex})
		}
		if req.IsAdd == (i.DHCP != nil) {
			return reply(&dhcp.DHCPClientConfigReply{Retval: int32(api.INVALID_VALUE)})
		}
		if !req.IsAdd {
			if i.DHCP.Lease.IsValid() {
				delete(i.Addrs, i.DHCP.Lease.String())
			}
			i.DHCP = nil
			return reply(&dhcp.DHCPClientConfigReply{})
		}
		c := req.Client
		c.ID = append(append([]byte(nil), c.ID...), make([]byte, 64-min(len(c.ID), 64))...) // VPP returns u8[64], NUL-padded
		i.DHCP = &DHCPClient{Config: c}
		return reply(&dhcp.DHCPClientConfigReply{})
	})
	v.On("dhcp_client_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, idx := range v.indexesLocked() {
			i := v.Ifaces[idx]
			if i.DHCP == nil {
				continue
			}
			c := i.DHCP.Config
			c.SwIfIndex = interface_types.InterfaceIndex(idx)
			l := dhcp.DHCPLease{SwIfIndex: c.SwIfIndex, State: dhcp.DHCP_CLIENT_STATE_API_DISCOVER, Hostname: c.Hostname}
			if p := i.DHCP.Lease; p.IsValid() {
				l.State = dhcp.DHCP_CLIENT_STATE_API_BOUND
				l.HostAddress = ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(p.Addr().As4())}
				l.MaskWidth = uint8(p.Bits()) //nolint:gosec // G115: an IPv4 prefix length (0–32)
			}
			out = append(out, &dhcp.DHCPClientDetails{Client: c, Lease: l})
		}
		return out, nil
	})
}

// BindLease models the DHCP server's ACK for ifName's client (the BOUND transition): VPP installs
// lease ("a.b.c.d/len") as an interface address and reports it in dhcp_client_dump. A client that was
// bound before gives up its old lease first. With no client on ifName, one is created (as if someone
// else had configured it).
func (v *VPP) BindLease(ifName, lease string) error {
	p, err := netip.ParsePrefix(lease)
	if err != nil || !p.Addr().Is4() {
		return fmt.Errorf("coretest: BindLease %q: not an IPv4 address/len", lease)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name != ifName {
			continue
		}
		if i.DHCP == nil {
			i.DHCP = &DHCPClient{Config: dhcp.DHCPClient{SwIfIndex: interface_types.InterfaceIndex(i.Index), Hostname: "vpp"}}
		}
		if old := i.DHCP.Lease; old.IsValid() {
			delete(i.Addrs, old.String())
		}
		i.DHCP.Lease = p
		i.Addrs[p.String()] = true
		return nil
	}
	return fmt.Errorf("coretest: BindLease: no interface %q", ifName)
}

// DHCPClientOf returns a copy of ifName's modelled DHCPv4 client (false: none).
func (v *VPP) DHCPClientOf(ifName string) (DHCPClient, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == ifName && i.DHCP != nil {
			return *i.DHCP, true
		}
	}
	return DHCPClient{}, false
}
