package desired

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/mfib"
	"strings"
	"testing"
)

func multicastDoc() *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Multicast: &ngfwv1.MulticastConfig{Igmp: &ngfwv1.IgmpConfig{Interfaces: map[string]*ngfwv1.IgmpInterface{"in": {Mode: strPtr("host"), Joins: []*ngfwv1.IgmpJoin{{Group: strPtr("232.1.2.3"), Sources: []string{"10.1.0.2", "10.1.0.1"}}}}, "out": {Mode: strPtr("router")}}, SsmRanges: []string{"232.0.0.0/8"}, Proxies: map[string]*ngfwv1.IgmpProxy{"test": {Upstream: strPtr("in"), Downstream: []string{"out"}}}}, Mroutes: []*ngfwv1.Mroute{{Vrf: strPtr("test"), Group: strPtr("239.1.2.3"), Paths: []*ngfwv1.MroutePath{{Interface: strPtr("out"), Flags: strPtr("forward")}}}}}}}
}
func TestIgmpMfibProjectionAndGlobals(t *testing.T) {
	ds := multicastDoc()
	resolve := func(s string) (uint32, bool) { return 1001, s == "test" }
	sink := newRecSink()
	IgmpMfib(sink, ds, map[string]bool{"routing": true}, resolve, false)
	for _, v := range sink.kvs {
		if v.Key.Descriptor() == igmp.NameGroupPrefix {
			t.Fatal("slot projected globals")
		}
	}
	if len(sink.kvs) != 6 {
		t.Fatalf("projection objects %d %v", len(sink.kvs), sink.issues)
	}
	out := &ngfwv1.DesiredState{}
	AssembleIgmpMfib(out, sink.kvs, func(uint32) string { return "test" })
	if len(out.GetRouting().GetMulticast().GetMroutes()) != 1 {
		t.Fatal("mroute assembly")
	}
	if out.GetRouting().GetMulticast().GetIgmp().GetInterfaces()["in"].GetJoins()[0].GetSources()[0] != "10.1.0.1" {
		t.Fatal("canonical sources")
	}
	sink = newRecSink()
	ds.Routing.Multicast.Igmp.SsmRanges = []string{}
	IgmpMfib(sink, ds, map[string]bool{"routing": true}, resolve, true)
	for _, v := range sink.kvs {
		if v.Key.Descriptor() == igmp.NameGroupPrefix {
			t.Fatal("explicit empty ranges overridden")
		}
	}
	_ = mfib.Name
}
func TestIgmpRejectsEmptyAndRouterJoin(t *testing.T) {
	for _, mode := range []string{"host", "router"} {
		ds := multicastDoc()
		ds.Routing.Multicast.Igmp.Interfaces["in"].Mode = strPtr(mode)
		ds.Routing.Multicast.Igmp.Interfaces["in"].Joins[0].Sources = nil
		sink := newRecSink()
		IgmpMfib(sink, ds, map[string]bool{"routing": true}, func(string) (uint32, bool) { return 1001, true }, false)
		found := false
		for _, v := range sink.issues {
			if strings.HasPrefix(v, "E /routing/multicast/igmp/interfaces/in/joins/0") {
				found = true
			}
		}
		if !found {
			t.Fatal("join not rejected", sink.issues)
		}
	}
}
