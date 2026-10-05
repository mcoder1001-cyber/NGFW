package ravpn

import (
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/tapv2"
	"testing"
)

func TestTransitReadbackRefusesReusedOrMovedEndpoint(t *testing.T) {
	plan := networkFixture()
	outer, inner, err := TransitTAPs(plan, 19001, 19002)
	if err != nil {
		t.Fatal(err)
	}
	if outer.HostIfName != "outer0" || inner.HostIfName != "inner0" || outer.HostMtu != 1500 || inner.HostMtu != 1400 || inner.HostIp4Prefix != plan.Inner.Namespace {
		t.Fatal("transit endpoint mismatch")
	}
	if VerifyTransitTAP(outer, proto.Clone(outer).(*tapv2.Tap)) != nil {
		t.Fatal("matching dump refused")
	}
	for _, change := range []func(*tapv2.Tap){
		func(tap *tapv2.Tap) { tap.HostNamespace = "/proc/1/ns/net" },
		func(tap *tapv2.Tap) { tap.Id++ },
		func(tap *tapv2.Tap) { tap.HostIfName = "foreign0" },
		func(tap *tapv2.Tap) { tap.HostIp4Prefix = "192.0.2.99/31" },
		func(tap *tapv2.Tap) { tap.HostBridge = "br0" },
		func(tap *tapv2.Tap) { tap.Gso = true },
	} {
		observed := proto.Clone(outer).(*tapv2.Tap)
		change(observed)
		if VerifyTransitTAP(outer, observed) == nil {
			t.Fatal("foreign or altered TAP adopted")
		}
	}
	if _, _, err := TransitTAPs(plan, 19001, 19001); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if _, _, err := TransitTAPs(plan, ^uint32(0), 19001); err == nil {
		t.Fatal("random ID accepted")
	}
}
