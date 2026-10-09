package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"testing"
)

func TestPppoeWANOwnershipReferencesPreserveConfiguredDefault(t *testing.T) {
	ifs := map[string]*ngfwv1.Interface{"pppwan": {Pppoe: &ngfwv1.Pppoe{DefaultRoute: proto.Bool(true)}, Lcp: &ngfwv1.InterfaceLcp{HostIfName: proto.String("ppp-parent")}}}
	groups := []*ngfwv1.WanGroup{{Name: proto.String("internet"), Members: []*ngfwv1.WanMember{{Interface: proto.String("pppwan"), Gateway: proto.String("192.0.2.1")}, {Interface: proto.String("other")}}, Monitors: []*ngfwv1.WanMonitor{{Target: proto.String("192.0.2.2")}}}}
	original := proto.Clone(groups[0])
	sink := &pppoeSink{}
	Pppoe(PppoeWANContext(sink, groups), ifs, false)
	if len(sink.kvs) != 1 {
		t.Fatal(sink)
	}
	value := sink.kvs[0].Value.(*ngfwv1.DesiredState)
	refs := value.GetRouting().GetWanGroups()
	if len(refs) != 1 || len(refs[0].Members) != 1 || refs[0].Members[0].GetInterface() != "pppwan" || refs[0].Members[0].Gateway != nil || len(refs[0].Monitors) != 0 {
		t.Fatal("unexpected ownership references", refs)
	}
	if !value.Interfaces["pppwan"].Pppoe.GetDefaultRoute() || !proto.Equal(groups[0], original) {
		t.Fatal("operator config changed")
	}
	restored := &ngfwv1.DesiredState{}
	AssemblePppoe(restored, sink.kvs)
	if !restored.Interfaces["pppwan"].Pppoe.GetDefaultRoute() || restored.Routing != nil {
		t.Fatal("private ownership leaked into retrieval")
	}
	removed := &pppoeSink{}
	Pppoe(PppoeWANContext(removed, nil), ifs, false)
	if removed.kvs[0].Value.(*ngfwv1.DesiredState).Routing != nil {
		t.Fatal("membership removal retained ownership")
	}
}
