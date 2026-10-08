package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/l2"
	"testing"
)

func TestCarrierVLANProjectionDependencyOrder(t *testing.T) {
	ifs := map[string]*ngfwv1.Interface{"eth0": {Enabled: proto.Bool(true), Subinterfaces: map[string]*ngfwv1.Subinterface{"100": {Enabled: proto.Bool(true), VlanId: proto.Uint32(100)}}}}
	parent, err := PppoeCarrierParent(ifs, "eth0.100")
	if err != nil {
		t.Fatal(err)
	}
	sink := &pppoeSink{}
	PppoeCarrierVLAN(sink, parent, "/interfaces/pppwan/pppoe")
	if len(sink.kvs) != 1 {
		t.Fatal(sink.kvs)
	}
	rewrite := sink.kvs[0].Value.(*l2.VlanTagRewrite)
	if rewrite.Op != l2.VtrOp_VTR_OP_POP_1 || rewrite.Interface != "interface/eth0.100" {
		t.Fatal(rewrite)
	}
	deps := l2.NewVlanTagRewrite(nil, "ngfw").Dependencies(rewrite)
	if len(deps) != 2 || deps[0].Key != "interface/eth0.100" || deps[1].Key != l2.XconnectKey("interface/eth0.100") {
		t.Fatal(deps)
	}
}
