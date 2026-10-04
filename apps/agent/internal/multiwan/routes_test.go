package multiwan

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"testing"
)

func routeDoc() *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"wan1": {}, "wan2": {}}, Routing: &ngfwv1.RoutingConfig{WanGroups: []*ngfwv1.WanGroup{{Name: proto.String("internet"), Mode: proto.String("failover"), Members: []*ngfwv1.WanMember{{Interface: proto.String("wan1"), NextHop: proto.String("gateway"), Gateway: proto.String("192.0.2.1"), Priority: proto.Uint32(1), Weight: proto.Uint32(3)}, {Interface: proto.String("wan2"), NextHop: proto.String("gateway"), Gateway: proto.String("198.51.100.1"), Priority: proto.Uint32(2), Weight: proto.Uint32(1)}}}}}}
}
func health(a, b bool) []*ngfwv1.WanGroupState {
	return []*ngfwv1.WanGroupState{{Name: "internet", Members: []*ngfwv1.WanMemberState{{Interface: "wan1", Up: a}, {Interface: "wan2", Up: b}}}}
}
func TestRoutesHealthLifecycle(t *testing.T) {
	doc := routeDoc()
	for _, tc := range []struct {
		name string
		a, b bool
		want string
	}{{"primary", true, true, "wan1"}, {"failover", false, true, "wan2"}, {"restore", true, true, "wan1"}, {"all-down", false, false, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			kvs, issues := Routes(doc, health(tc.a, tc.b))
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			if tc.want == "" {
				if len(kvs) != 0 {
					t.Fatal(kvs)
				}
				return
			}
			if len(kvs) != 1 {
				t.Fatal(kvs)
			}
			r := kvs[0].Value.(*core.Route)
			if len(r.Paths) != 1 || r.Paths[0].Interface != tc.want || kvs[0].Key.Descriptor() != RouteName {
				t.Fatal(r, kvs[0].Key)
			}
		})
	}
	doc.Routing.WanGroups[0].Mode = proto.String("balance")
	kvs, issues := Routes(doc, health(true, true))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	r := kvs[0].Value.(*core.Route)
	if len(r.Paths) != 2 || r.Paths[0].Weight != 3 || r.Paths[1].Weight != 1 {
		t.Fatal(r)
	}
	kvs, _ = Routes(doc, health(false, true))
	if len(kvs[0].Value.(*core.Route).Paths) != 1 {
		t.Fatal(kvs)
	}
	doc.Routing.WanGroups = nil
	kvs, issues = Routes(doc, health(true, true))
	if len(kvs) != 0 || len(issues) != 0 {
		t.Fatal("rollback retained route")
	}
}
func TestRoutesRejectOwnershipConflictEvenAllDown(t *testing.T) {
	doc := routeDoc()
	doc.Routing.Static = []*ngfwv1.StaticRoute{{Prefix: proto.String("0.0.0.0/0")}}
	kvs, issues := Routes(doc, nil)
	if len(kvs) != 0 || len(issues) != 1 || issues[0].Pointer != "/routing/static/0" {
		t.Fatal(kvs, issues)
	}
	doc.Routing.Static = nil
	doc.Routing.WanGroups = append(doc.Routing.WanGroups, proto.Clone(doc.Routing.WanGroups[0]).(*ngfwv1.WanGroup))
	kvs, issues = Routes(doc, nil)
	if len(kvs) != 0 || len(issues) == 0 {
		t.Fatal(kvs, issues)
	}
}
func TestRoutesRejectVRFAndUnsupportedGateway(t *testing.T) {
	doc := routeDoc()
	doc.Interfaces["wan2"].Vrf = proto.String("other")
	doc.Vrfs = map[string]*ngfwv1.Vrf{"other": {Id: proto.Uint32(7)}}
	_, issues := Routes(doc, nil)
	if len(issues) == 0 || issues[0].Pointer != "/routing/wanGroups/0/members/1/interface" {
		t.Fatal(issues)
	}
	doc = routeDoc()
	doc.Routing.WanGroups[0].Members[1].NextHop = proto.String("dhcp")
	kvs, issues := Routes(doc, health(false, true))
	if len(issues) != 0 || len(kvs) != 0 {
		t.Fatal("unsupported runtime gateway was routed", kvs, issues)
	}
}

func TestExpandPBRUsesSelectedGroupAndAllDownLookup(t *testing.T) {
	doc := routeDoc()
	group := "internet"
	doc.Routing.Pbr = &ngfwv1.PbrConfig{Policies: map[string]*ngfwv1.PbrPolicy{"pin": {Paths: []*ngfwv1.PbrPath{{WanGroup: &group}}}}}
	if issues := ExpandPBR(doc, health(false, true)); len(issues) > 0 {
		t.Fatal(issues)
	}
	got := doc.Routing.Pbr.Policies["pin"].Paths
	if len(got) != 1 || got[0].GetInterface() != "wan2" || got[0].GetAddress() != "198.51.100.1" {
		t.Fatal(got)
	}
	doc.Routing.Pbr.Policies["pin"].Paths = []*ngfwv1.PbrPath{{WanGroup: &group}}
	if issues := ExpandPBR(doc, health(false, false)); len(issues) > 0 {
		t.Fatal(issues)
	}
	got = doc.Routing.Pbr.Policies["pin"].Paths
	if len(got) != 1 || got[0].GetInterface() != "" || got[0].GetVrf() != "default" {
		t.Fatal(got)
	}
}
func TestNATObjectsRequireExplicitEDAndAvoidConfigWriters(t *testing.T) {
	doc := routeDoc()
	if got := NATObjects(doc); len(got) != 0 {
		t.Fatal(got)
	}
	doc.Nat = &ngfwv1.NatConfig{Enabled: proto.Bool(true), Mode: proto.String("ed")}
	if got := NATObjects(doc); len(got) != 4 {
		t.Fatal(got)
	}
	doc.Nat.OutputFeature = []string{"wan1"}
	if got := NATObjects(doc); len(got) != 2 || got[0].Key.ID() != "wan2" {
		t.Fatal(got)
	}
}
func TestDeadAddressesOnlyHealthyDownStickyConfiguredMembers(t *testing.T) {
	doc := routeDoc()
	doc.Interfaces["wan1"].Ipv4 = []string{"192.0.2.2/24"}
	if got := DeadAddresses(doc, health(true, true), health(false, true)); len(got) != 0 {
		t.Fatal("cleanup without enabled NAT", got)
	}
	doc.Nat = &ngfwv1.NatConfig{Enabled: proto.Bool(true), Mode: proto.String("ed")}
	got := DeadAddresses(doc, health(true, true), health(false, true))
	if !got["192.0.2.2"] || len(got) != 1 {
		t.Fatal(got)
	}
	if got := DeadAddresses(doc, health(false, true), health(false, true)); len(got) != 0 {
		t.Fatal(got)
	}
	doc.Routing.WanGroups[0].StickySessions = proto.Bool(false)
	if got := DeadAddresses(doc, health(true, true), health(false, true)); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestRestorePBRReferencesDoesNotHideForwardingDrift(t *testing.T) {
	original := routeDoc()
	group := "internet"
	original.Routing.Pbr = &ngfwv1.PbrConfig{Policies: map[string]*ngfwv1.PbrPolicy{"pin": {Paths: []*ngfwv1.PbrPath{{WanGroup: &group}}}}}
	live := proto.Clone(original).(*ngfwv1.DesiredState)
	if issues := ExpandPBR(live, health(false, true)); len(issues) != 0 {
		t.Fatal(issues)
	}
	RestorePBRReferences(live, original, health(false, true))
	if live.Routing.Pbr.Policies["pin"].Paths[0].GetWanGroup() != "internet" {
		t.Fatal("configuration reference lost")
	}
	ExpandPBR(live, health(false, true))
	live.Routing.Pbr.Policies["pin"].Paths[0].Address = proto.String("203.0.113.9")
	RestorePBRReferences(live, original, health(false, true))
	if live.Routing.Pbr.Policies["pin"].Paths[0].GetWanGroup() != "" {
		t.Fatal("VPP forwarding drift hidden")
	}
}

func TestRoutesOptionalPriorityUsesSchemaDefault(t *testing.T) {
	doc := routeDoc()
	doc.Routing.WanGroups[0].Members[0].Priority = nil
	kvs, issues := Routes(doc, health(true, true))
	if len(issues) != 0 || len(kvs) != 1 {
		t.Fatal(kvs, issues)
	}
	if kvs[0].Value.(*core.Route).Paths[0].Interface != "wan2" {
		t.Fatal("omitted priority outranked explicit priority2")
	}
}

func TestExpandPBRSharedMemberDoesNotCrossAddressFamily(t *testing.T) {
	doc := routeDoc()
	v6 := proto.Clone(doc.Routing.WanGroups[0]).(*ngfwv1.WanGroup)
	v6.Name = proto.String("internet6")
	v6.Members[0].Gateway = proto.String("2001:db8:1::1")
	v6.Members[1].Gateway = proto.String("2001:db8:2::1")
	doc.Routing.WanGroups = append(doc.Routing.WanGroups, v6)
	doc.Routing.Pbr = &ngfwv1.PbrConfig{Policies: map[string]*ngfwv1.PbrPolicy{
		"v4": {Paths: []*ngfwv1.PbrPath{{WanGroup: proto.String("internet")}}},
		"v6": {Paths: []*ngfwv1.PbrPath{{WanGroup: proto.String("internet6")}}},
	}}
	states := append(health(true, true), &ngfwv1.WanGroupState{Name: "internet6", Members: []*ngfwv1.WanMemberState{{Interface: "wan1", Up: false}, {Interface: "wan2", Up: true}}})
	if issues := ExpandPBR(doc, states); len(issues) > 0 {
		t.Fatal(issues)
	}
	if p := doc.Routing.Pbr.Policies["v4"].Paths[0]; p.GetAddress() != "192.0.2.1" || p.GetInterface() != "wan1" {
		t.Fatal("IPv4 group used overlapping IPv6 group", p)
	}
	if p := doc.Routing.Pbr.Policies["v6"].Paths[0]; p.GetAddress() != "2001:db8:2::1" || p.GetInterface() != "wan2" {
		t.Fatal("IPv6 group used overlapping IPv4 group", p)
	}
}

func TestRoutesWeightWidthBounds(t *testing.T) {
	for _, weight := range []uint32{0, 1, 255, 256, 4294967295} {
		doc := routeDoc()
		doc.Routing.WanGroups[0].Mode = proto.String("balance")
		doc.Routing.WanGroups[0].Members[0].Weight = proto.Uint32(weight)
		kvs, issues := Routes(doc, health(true, true))
		if weight > 255 {
			if len(kvs) != 0 || len(issues) == 0 {
				t.Fatalf("weight%d emitted partial route: %v %v", weight, kvs, issues)
			}
			continue
		}
		if len(issues) != 0 || len(kvs) != 1 {
			t.Fatalf("weight%d refused: %v %v", weight, kvs, issues)
		}
		want := weight
		if want == 0 {
			want = 1
		}
		got := kvs[0].Value.(*core.Route).Paths[0].Weight
		if got != want {
			t.Fatalf("weight%d got%d want%d", weight, got, want)
		}
	}
}
