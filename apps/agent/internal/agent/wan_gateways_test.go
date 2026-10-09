package agent

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	dhcpapi "ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/ip_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/subsystems"
)

// Keep the product runtime and WAN adapter contract connected at compile time.
var _ wanPPPoEForwarding = (*subsystems.PppoeRuntime)(nil)

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

type wanCarrierFake struct {
	mu                sync.Mutex
	ready             bool
	generation        string
	probeCalls        int
	mutateDuringProbe bool
}

func (f *wanCarrierFake) ForwardingGateway(string) (string, string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return "203.0.113.2/32", "169.254.254.2", f.generation, f.ready
}
func (f *wanCarrierFake) ProbeForwarding(context.Context, string, *ngfwv1.WanMonitor) multiwan.CheckResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probeCalls++
	if f.mutateDuringProbe {
		f.generation = "replacement"
	}
	return multiwan.CheckResult{Sent: 1, Received: 1}
}
func (f *wanCarrierFake) set(ready bool, generation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ready = ready
	f.generation = generation
}

func TestWANPPPCarrierGatewayProbeConvergenceAndWithdrawal(t *testing.T) {
	saved := doc(t, `{"interfaces":{"pppwan":{"pppoe":{}}},"routing":{"wanGroups":[{"name":"edge","members":[{"interface":"pppwan","nextHop":"pppoe"}],"monitors":[{"type":"http","target":"192.0.2.1","intervalMs":100,"timeoutMs":50,"lossPct":100,"downAfter":1,"upAfter":1}]}]}}`)
	carrier := &wanCarrierFake{}
	runtime := multiwan.NewRuntime(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	groups := saved.Routing.WanGroups
	identity := wanIdentity(saved)
	if err := runtime.ReplaceWithProbe(ctx, groups, identity, wanProbe(saved, carrier)); err != nil {
		t.Fatal(err)
	}
	if _, ready := wanPPPGateway(carrier, "pppwan"); ready {
		t.Fatal("unready carrier admitted")
	}
	for _, generation := range []string{"first", "replacement"} {
		carrier.set(true, generation)
		gateway, ok := wanPPPGateway(carrier, "pppwan")
		if !ok {
			t.Fatal("verified gateway unavailable")
		}
		runtime.SetGateways(groups, identity, map[string]multiwan.LearnedGateway{"pppwan": gateway})
		deadline := time.Now().Add(time.Second)
		for !runtime.Snapshot()[0].Members[0].Up && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if !runtime.Snapshot()[0].Members[0].Up {
			t.Fatal("verified carrier probe did not converge")
		}
		resolved := runtime.ResolveGateways(saved, identity, true)
		routes, issues := multiwan.Routes(resolved, runtime.HealthFor(groups, identity))
		if len(issues) > 0 || len(routes) != 1 {
			t.Fatal(routes, issues)
		}
		path := routes[0].Value.(*core.Route).Paths[0]
		if path.Interface != "pppwan" || path.Address != "169.254.254.2" {
			t.Fatal("raw WAN/ISP peer used", path)
		}
		carrier.set(false, generation)
		runtime.SetGateways(groups, identity, nil)
		resolved = runtime.ResolveGateways(saved, identity, true)
		routes, issues = multiwan.Routes(resolved, runtime.HealthFor(groups, identity))
		if len(routes) != 0 || len(issues) > 0 {
			t.Fatal("withdrawal failed", routes, issues)
		}
	}
	if len(saved.Interfaces["pppwan"].Ipv4) != 0 {
		t.Fatal("lease persisted")
	}
}

func TestWANPPPProbeRejectsLateGenerationAndUnsupportedTarget(t *testing.T) {
	saved := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"pppwan": {Pppoe: &ngfwv1.Pppoe{}}}}
	carrier := &wanCarrierFake{ready: true, generation: "first", mutateDuringProbe: true}
	probe := wanProbe(saved, carrier)
	result := probe(context.Background(), "pppwan", &ngfwv1.WanMonitor{Type: proto.String("dns"), Target: proto.String("192.0.2.1")})
	if !result.Unavailable || result.Received != 0 {
		t.Fatal("old generation admitted", result)
	}
	carrier.mutateDuringProbe = false
	carrier.probeCalls = 0
	result = probe(context.Background(), "pppwan", &ngfwv1.WanMonitor{Type: proto.String("http"), Target: proto.String("example.test")})
	if !result.Unavailable || carrier.probeCalls != 0 {
		t.Fatal("hostname escaped carrier policy")
	}
}
