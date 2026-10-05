package ravpn

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core"
	"strings"
	"testing"
)

func controllerSpec(t *testing.T) EngineSpec {
	t.Helper()
	raw := []byte(`{"vpn":{"remoteAccess":{"road":{"localAddr":"192.0.2.1","transport":{"outer":{"vpp":"198.18.0.0/31","namespace":"198.18.0.1/31"},"inner":{"vpp":"198.18.1.0/31","namespace":"198.18.1.1/31"}},"pools":[{"prefix":"10.10.0.0/24"}],"outerPolicy":{"ingress":["outer-in"],"egress":["outer-out"]},"accessPolicy":{"ingress":["inner-in"],"egress":["inner-out"]}}}}}`)
	var ds ngfwv1.DesiredState
	if protojson.Unmarshal(raw, &ds) != nil {
		t.Fatal("fixture")
	}
	return EngineSpec{Owner: "w19", Profile: "road", Instance: InstanceID("w19", "road"), Configuration: ds.GetVpn().GetRemoteAccess()["road"], Proposal: &ngfwv1.IpsecProposal{}, OuterID: 10, InnerID: 11, OuterTable: 7, InnerTable: 8, Fingerprints: map[string]string{"cert/server": "hmac:" + strings.Repeat("a", 64)}}
}
func TestEngineContractRejectsUnsupportedIDAndUnkeyedFingerprint(t *testing.T) {
	s := controllerSpec(t)
	if s.Validate() != nil {
		t.Fatal("valid")
	}
	v, e := s.Proto()
	if e != nil {
		t.Fatal(e)
	}
	d, e := DecodeEngine(v)
	if e != nil || !proto.Equal(s.Configuration, d.Configuration) {
		t.Fatal("roundtrip")
	}
	for _, change := range []func(*EngineSpec){func(s *EngineSpec) { s.OuterID = 19000 }, func(s *EngineSpec) { s.InnerID = s.OuterID }, func(s *EngineSpec) { s.Fingerprints["cert/server"] = strings.Repeat("a", 64) }, func(s *EngineSpec) { s.Configuration.OuterPolicy.Egress = nil }} {
		copy := controllerSpec(t)
		change(&copy)
		if copy.Validate() == nil {
			t.Fatal("unsafe contract accepted")
		}
	}
}
func TestTransportCarriesExplicitVRFsRoutesAndBothACLDirections(t *testing.T) {
	s := controllerSpec(t)
	kvs, e := TransportObjects(s)
	if e != nil {
		t.Fatal(e)
	}
	routes, bindings := 0, 0
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *core.Route:
			routes++
			if len(v.Paths) != 1 || v.Paths[0].Interface == "" || v.Paths[0].Address == "" || v.Paths[0].NextHopTable != nil {
				t.Fatal("recursive or wrong-table route")
			}
			if v.Prefix == "192.0.2.1/32" && v.TableId != 7 {
				t.Fatal("outer vrf")
			}
			if v.Prefix == "10.10.0.0/24" && v.TableId != 8 {
				t.Fatal("inner vrf")
			}
		}
		if kv.Key.Descriptor() == "remote-access."+acl.NameInterfaceBinding {
			bindings++
			b, e := acl.InterfaceBindingFromProto(kv.Value)
			if e != nil || len(b.Input) != 1 || len(b.Output) != 1 {
				t.Fatal("one-way policy")
			}
		}
	}
	if routes != 2 || bindings != 2 {
		t.Fatalf("routes%d bindings%d", routes, bindings)
	}
}
