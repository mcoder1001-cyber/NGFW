package multiwan

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"testing"
)

func TestStaticWANPoolUsesInterfaceFIBAndPreservesDynamicTracking(t *testing.T) {
	doc := routeDoc()
	doc.Nat = &ngfwv1.NatConfig{Enabled: proto.Bool(true), Mode: proto.String("ed")}
	doc.Interfaces["wan1"].Ipv4 = []string{"192.0.2.2/24"}
	doc.Interfaces["wan1"].Vrf = proto.String("blue")
	doc.Vrfs = map[string]*ngfwv1.Vrf{"blue": {Id: proto.Uint32(901)}}
	got := NATObjects(doc)
	if len(got) != 4 {
		t.Fatal(got)
	}
	if got[1].Key.Descriptor() != NATStaticAddressName {
		t.Fatal(got[1].Key)
	}
	spec, err := natcommon.Decode[nat44ed.WANPoolSpec](got[1].Value)
	if err != nil || spec.Interface != "wan1" || spec.Prefix != "192.0.2.2/24" || spec.VRF != 901 {
		t.Fatalf("%+v %v", spec, err)
	}
	if got[3].Key.Descriptor() != NATAddressName {
		t.Fatal("dynamic address tracking changed", got[3].Key)
	}
	doc.Nat.Pools = []*ngfwv1.NatPool{{Range: proto.String("192.0.2.1-192.0.2.10")}}
	got = NATObjects(doc)
	if len(got) != 2 || got[0].Key.ID() != "wan2" {
		t.Fatal("explicit pool gained second writer", got)
	}
}
