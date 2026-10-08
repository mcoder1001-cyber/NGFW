package agent

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	dhcpapi "ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/ip_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/multiwan"
)

func TestWANGatewayOwnedDHCPDumpRenewReleaseError(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	configured := doc(t, `{"interfaces":{"host-w1w0":{"enabled":true,"dhcpClient":{"hostname":"wan"}}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "dhcp", DesiredState: configured}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	client, ok := v.DHCPClientOf("host-w1w0")
	if !ok {
		t.Fatal("client missing")
	}
	saved := proto.Clone(configured).(*ngfwv1.DesiredState)
	saved.Routing = &ngfwv1.RoutingConfig{WanGroups: []*ngfwv1.WanGroup{{Name: proto.String("internet"), Members: []*ngfwv1.WanMember{{Interface: proto.String("host-w1w0"), NextHop: proto.String("dhcp")}}}}}
	a := &Agent{svc: s, log: s.log}
	lease := dhcpapi.DHCPLease{SwIfIndex: client.Config.SwIfIndex, State: dhcpapi.DHCP_CLIENT_STATE_API_BOUND, MaskWidth: 24,
		HostAddress:   ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4([4]byte{192, 0, 2, 2})},
		RouterAddress: ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4([4]byte{192, 0, 2, 1})}}
	var dumpErr error
	v.On("dhcp_client_dump", func(api.Message) ([]api.Message, error) {
		if dumpErr != nil {
			return nil, dumpErr
		}
		return []api.Message{&dhcpapi.DHCPClientDetails{Client: client.Config, Lease: lease}}, nil
	})
	check := func(want string) {
		t.Helper()
		got := a.readWANGateways(ctx, saved)
		if want == "" {
			if len(got) != 0 {
				t.Fatal(got)
			}
			return
		}
		if got["host-w1w0"].Gateway.String() != want {
			t.Fatal(got)
		}
	}
	check("192.0.2.1")
	lease.RouterAddress.Un = ip_types.AddressUnionIP4([4]byte{192, 0, 2, 9})
	check("192.0.2.9")
	lease.State = dhcpapi.DHCP_CLIENT_STATE_API_DISCOVER
	check("")
	lease.State = dhcpapi.DHCP_CLIENT_STATE_API_BOUND
	dumpErr = errors.New("dump disconnected")
	check("")
	dumpErr = nil
	check("192.0.2.9")
	saved.Interfaces["host-w1w0"].DhcpClient = nil
	check("")
}

func TestWANPBRRuntimeLeaseDoesNotPersistAddress(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	s.wan = multiwan.NewRuntime(nil)
	saved := doc(t, `{"interfaces":{"host-w1w0":{"dhcpClient":{"hostname":"wan"}}},"routing":{"wanGroups":[{"name":"internet","members":[{"interface":"host-w1w0","nextHop":"dhcp"}]}]}}`)
	s.wan.SetGateways(saved.Routing.WanGroups, wanIdentity(saved), map[string]multiwan.LearnedGateway{"host-w1w0": {Source: "dhcp", Address: netip.MustParsePrefix("192.0.2.2/24"), Gateway: netip.MustParseAddr("192.0.2.1")}})
	resolved := s.wan.ResolveGateways(saved, wanIdentity(saved), false)
	if len(resolved.Interfaces["host-w1w0"].Ipv4) != 0 || saved.Routing.WanGroups[0].Members[0].GetNextHop() != "dhcp" {
		t.Fatal("runtime leaked into desired config")
	}
}
