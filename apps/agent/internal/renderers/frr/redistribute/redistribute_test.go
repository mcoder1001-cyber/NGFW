package redistribute

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"testing"
)

func TestEdgesPreserveVRFAndUnknownCounts(t *testing.T) {
	r := &ngfwv1.RoutingConfig{Ospf: &ngfwv1.OspfConfig{Vrf: proto.String("blue"), Redistribute: &ngfwv1.Redistribute{Static: &ngfwv1.RedistributeOptions{RouteMap: proto.String("filtered"), Metric: proto.Uint32(10)}}}}
	out := Edges(r)
	if len(out) != 1 || out[0].Source != "static" || out[0].Target != "ospf" || out[0].Vrf != "blue" || out[0].RouteCount != nil || out[0].GetMetric() != 10 {
		t.Fatalf("%v", out)
	}
}
