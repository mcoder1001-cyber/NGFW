package pppoe

import (
	"context"
	"google.golang.org/protobuf/proto"
	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/vpp/fake"
	"testing"
)

func vlanParents() map[string]*ngfwv1.Interface {
	return map[string]*ngfwv1.Interface{"eth0": {Enabled: proto.Bool(true), Subinterfaces: map[string]*ngfwv1.Subinterface{
		"100": {Enabled: proto.Bool(true), VlanId: proto.Uint32(100)},
		"200": {Enabled: proto.Bool(true), VlanId: proto.Uint32(200), InnerVlanId: proto.Uint32(300)},
	}}}
}
func TestCarrierVLANConfigAndManifest(t *testing.T) {
	ifs := vlanParents()
	for name, op := range map[string]l2.VtrOp{"eth0.100": l2.VtrOp_VTR_OP_POP_1, "eth0.200": l2.VtrOp_VTR_OP_POP_2} {
		p, err := ResolveCarrierParent(ifs, name)
		if err != nil || p.Rewrite.Op != op || !p.Rewrite.Xconnect || p.Rewrite.Tag1 != 0 || p.Rewrite.PushDot1Q {
			t.Fatalf("%s: %+v %v", name, p, err)
		}
	}
	doc := &ngfwv1.DesiredState{}
	if err := CopyCarrierParentReference(doc, ifs, "eth0.100"); err != nil {
		t.Fatal(err)
	}
	if len(doc.Interfaces["eth0"].Subinterfaces) != 1 {
		t.Fatal("unrelated sibling copied")
	}
	doc.Interfaces["eth0"].Subinterfaces["100"].VlanId = proto.Uint32(101)
	if ifs["eth0"].Subinterfaces["100"].GetVlanId() != 100 {
		t.Fatal("mutated source")
	}
	if err := CopyCarrierParentReference(doc, ifs, "eth0.200"); err != nil {
		t.Fatal(err)
	}
	if len(doc.Interfaces["eth0"].Subinterfaces) != 2 {
		t.Fatal("lost first selected child")
	}
	k, ok, err := CarrierVLANDependency(doc, "eth0.200")
	if err != nil || !ok || k.ID() != "eth0.200" {
		t.Fatalf("%s %v %v", k, ok, err)
	}
	for _, name := range []string{"eth0", "eth0.999", "eth0.0100"} {
		if _, err := ResolveCarrierParent(ifs, name); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	ifs["eth0"].Subinterfaces["100"].Ipv4 = []string{"192.0.2.1/24"}
	if _, err := ResolveCarrierParent(ifs, "eth0.100"); err == nil {
		t.Fatal("accepted addressed child")
	}
}
func TestCarrierVLANLiveExactOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ifapi.SwInterfaceDetails)
		fail   bool
	}{
		{"exact", func(*ifapi.SwInterfaceDetails) {}, false},
		{"qinq-dot1ad", func(r *ifapi.SwInterfaceDetails) {
			r.SubNumberOfTags = 2
			r.SubInnerVlanID = 300
			r.SubIfFlags = interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_EXACT_MATCH | interface_types.SUB_IF_API_FLAG_DOT1AD
		}, false},
		{"qinq-missing-inner", func(r *ifapi.SwInterfaceDetails) {
			r.SubNumberOfTags = 2
			r.SubIfFlags = interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_EXACT_MATCH
		}, true},
		{"foreign", func(r *ifapi.SwInterfaceDetails) { r.Tag = "other:eth0.100" }, true},
		{"wildcard", func(r *ifapi.SwInterfaceDetails) { r.SubIfFlags |= interface_types.SUB_IF_API_FLAG_OUTER_VLAN_ID_ANY }, true},
		{"wrong-parent-name", func(r *ifapi.SwInterfaceDetails) { r.Tag = "ngfw:other.100" }, true},
		{"wrong-inner", func(r *ifapi.SwInterfaceDetails) { r.SubInnerVlanID = 4 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
			root := &ifapi.SwInterfaceDetails{SwIfIndex: 1, SupSwIfIndex: 1, InterfaceName: "Ethernet0", Tag: "ngfw:eth0"}
			child := &ifapi.SwInterfaceDetails{SwIfIndex: 2, SupSwIfIndex: 1, SubID: 100, SubNumberOfTags: 1, SubOuterVlanID: 100, SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, Tag: "ngfw:eth0.100"}
			tc.mutate(child)
			f.Reply("sw_interface_dump", root, child)
			table, err := iface.Dump(context.Background(), f, "ngfw")
			if err != nil {
				t.Fatal(err)
			}
			got, err := carrierLiveVLAN(table, 2)
			if (err != nil) != tc.fail {
				t.Fatalf("%+v %v", got, err)
			}
			if err == nil && child.SubNumberOfTags == 2 && got.Op != l2.VtrOp_VTR_OP_POP_2 {
				t.Fatal("QinQ did not require POP_2")
			}
		})
	}
}

func TestCarrierVLANReadinessAndAdmission(t *testing.T) {
	spec, err := ren.NewCarrierSpec("ngfw", "pppwan", "eth0.100", 1492)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		op       uint32
		rawOp    uint32
		ready    bool
		admitted bool
	}{
		{"not-yet-configured", 0, 0, false, true}, {"correct-pop", 3, 0, true, false},
		{"wrong-pop", 4, 0, false, false}, {"double-tagging-tap", 3, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := func() *fake.Client {
				f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
				f.Reply("sw_interface_dump",
					&ifapi.SwInterfaceDetails{SwIfIndex: 1, SupSwIfIndex: 1, InterfaceName: "Ethernet0", Tag: "ngfw:eth0"},
					&ifapi.SwInterfaceDetails{SwIfIndex: 2, SupSwIfIndex: 1, SubID: 100, SubNumberOfTags: 1, SubOuterVlanID: 100, SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, Tag: "ngfw:eth0.100", VtrOp: tc.op},
					&ifapi.SwInterfaceDetails{SwIfIndex: 3, SupSwIfIndex: 3, Tag: "ngfw:" + spec.RawLogical(), VtrOp: tc.rawOp})
				return f
			}
			if e := VerifyCarrierVLANReadiness(context.Background(), client(), "ngfw", spec); (e == nil) != tc.ready {
				t.Fatalf("ready=%v error=%v", tc.ready, e)
			}
			if e := AdmitCarrierVLAN(context.Background(), client(), "ngfw", spec); (e == nil) != tc.admitted {
				t.Fatalf("admitted=%v error=%v", tc.admitted, e)
			}
		})
	}
}

func TestCarrierVLANRejectsDifferentValidConfiguredTag(t *testing.T) {
	spec, _ := ren.NewCarrierSpec("ngfw", "pppwan", "eth0.100", 1492)
	expected := &ren.CarrierVLAN{Root: "eth0", SubID: 100, Outer: 100}
	for _, outer := range []uint16{100, 200} {
		f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
		f.Reply("sw_interface_dump", &ifapi.SwInterfaceDetails{SwIfIndex: 1, SupSwIfIndex: 1, InterfaceName: "Ethernet0", Tag: "ngfw:eth0"}, &ifapi.SwInterfaceDetails{SwIfIndex: 2, SupSwIfIndex: 1, SubID: 100, SubNumberOfTags: 1, SubOuterVlanID: outer, SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, Tag: "ngfw:eth0.100"})
		err := VerifyCarrierVLANConfiguration(t.Context(), f, "ngfw", spec, expected)
		if (err == nil) != (outer == 100) {
			t.Fatalf("outer=%d err=%v", outer, err)
		}
	}
}
