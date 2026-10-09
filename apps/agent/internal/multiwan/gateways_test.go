package multiwan

import (
	"net/netip"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
)

func TestLearnedGatewayConvergenceWithdrawalAndGeneration(t *testing.T) {
	doc := routeDoc()
	doc.Routing.WanGroups[0].Members[0].NextHop = proto.String("dhcp")
	doc.Routing.WanGroups[0].Members[0].Gateway = nil
	doc.Interfaces["wan1"].DhcpClient = &ngfwv1.DhcpClient{}
	original := proto.Clone(doc)
	rt := NewRuntime(nil)
	lease := LearnedGateway{Source: "dhcp", Address: netip.MustParsePrefix("203.0.113.10/24"), Gateway: netip.MustParseAddr("203.0.113.1")}
	check := func(identity string, want string) {
		t.Helper()
		resolved := rt.ResolveGateways(doc, identity, true)
		routes, issues := Routes(resolved, health(true, false))
		if len(issues) != 0 {
			t.Fatal(issues)
		}
		if want == "" {
			if len(routes) != 0 {
				t.Fatal("stale route", routes)
			}
			return
		}
		if len(routes) != 1 || routes[0].Value.(*core.Route).Paths[0].Address != want {
			t.Fatal(routes)
		}
		if resolved.Interfaces["wan1"].Ipv4[0] != lease.Address.String() {
			t.Fatal("missing NAT address")
		}
	}
	check("v1", "")
	if !rt.SetGateways(doc.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": lease}) {
		t.Fatal("missing initial change")
	}
	check("v1", "203.0.113.1")
	check("v2", "")
	if rt.SetGateways(doc.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": lease}) {
		t.Fatal("unchanged refresh triggered resync")
	}
	lease.Gateway = netip.MustParseAddr("203.0.113.2")
	if !rt.SetGateways(doc.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": lease}) {
		t.Fatal("renewal not observed")
	}
	check("v1", "203.0.113.2")
	if got := rt.ResolveGateways(doc, "v1", false); len(got.Interfaces["wan1"].Ipv4) != 0 {
		t.Fatal("PBR polluted desired address")
	}
	rt.gatewayUntil = time.Now().Add(-time.Second)
	check("v1", "")
	rt.SetGateways(doc.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": lease})
	rt.SetGateways(doc.Routing.WanGroups, "v1", nil)
	check("v1", "")
	if !proto.Equal(doc, original) {
		t.Fatal("persisted document mutated")
	}
}

func TestLearnedGatewayRejectsInvalidOrWrongSource(t *testing.T) {
	doc := routeDoc()
	doc.Routing.WanGroups[0].Members[0].NextHop = proto.String("dhcp")
	doc.Routing.WanGroups[0].Members[0].Gateway = nil
	rt := NewRuntime(nil)
	for _, lease := range []LearnedGateway{
		{Source: "dhcp", Address: netip.MustParsePrefix("192.0.2.2/24"), Gateway: netip.IPv4Unspecified()},
		{Source: "dhcp", Address: netip.MustParsePrefix("192.0.2.2/24"), Gateway: netip.MustParseAddr("ff02::1")},
		{Source: "dhcp", Gateway: netip.MustParseAddr("192.0.2.1")},
		{Source: "pppoe", Address: netip.MustParsePrefix("192.0.2.2/32"), Gateway: netip.MustParseAddr("192.0.2.1")},
	} {
		rt.SetGateways(doc.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": lease})
		if rt.ResolveGateways(doc, "v1", true).Routing.WanGroups[0].Members[0].GetNextHop() != "dhcp" {
			t.Fatal("accepted invalid source")
		}
	}
}

func TestDynamicUnboundReservesDefaultOwnership(t *testing.T) {
	doc := routeDoc()
	for _, m := range doc.Routing.WanGroups[0].Members {
		m.NextHop = proto.String("dhcp")
		m.Gateway = nil
	}
	other := proto.Clone(doc.Routing.WanGroups[0]).(*ngfwv1.WanGroup)
	other.Name = proto.String("conflict")
	doc.Routing.WanGroups = append(doc.Routing.WanGroups, other)
	if _, issues := Routes(doc, nil); len(issues) == 0 {
		t.Fatal("unbound defaults failed to reserve ownership")
	}
}

func TestRetiredLeaseNATCleanupAndPBR(t *testing.T) {
	doc := routeDoc()
	doc.Nat = &ngfwv1.NatConfig{Enabled: proto.Bool(true)}
	doc.Interfaces["wan1"].Ipv4 = []string{"192.0.2.2/24"}
	doc.Interfaces["wan2"].Ipv4 = []string{"198.51.100.2/24"}
	after := proto.Clone(doc).(*ngfwv1.DesiredState)
	after.Interfaces["wan1"].Ipv4 = []string{"192.0.2.3/24"}
	dead := RetiredAddresses(doc, after)
	if len(dead) != 1 || !dead["192.0.2.2"] {
		t.Fatal(dead)
	}
	after.Routing.Pbr = &ngfwv1.PbrConfig{Policies: map[string]*ngfwv1.PbrPolicy{"p": {Paths: []*ngfwv1.PbrPath{{WanGroup: proto.String("internet")}}}}}
	after.Routing.WanGroups[0].Members[0].NextHop = proto.String("dhcp")
	rt := NewRuntime(nil)
	rt.SetGateways(after.Routing.WanGroups, "v1", map[string]LearnedGateway{"wan1": {Source: "dhcp", Address: netip.MustParsePrefix("192.0.2.3/24"), Gateway: netip.MustParseAddr("192.0.2.9")}})
	resolved := rt.ResolveGateways(after, "v1", true)
	if issues := ExpandPBR(resolved, health(true, false)); len(issues) > 0 {
		t.Fatal(issues)
	}
	if resolved.Routing.Pbr.Policies["p"].Paths[0].GetAddress() != "192.0.2.9" {
		t.Fatal("PBR did not follow lease")
	}
	if len(NATObjects(resolved)) != 4 {
		t.Fatal("missing learned NAT objects")
	}
}
