package desired

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/vrrp"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// TestVrrpKeepalivedStageValue: a keepalived instance on an interface with a linux-cp pair goes into the
// keepalived.config value together with that pair (the stage's mapper input); the assembler reads it back.
func TestVrrpKeepalivedStageValue(t *testing.T) {
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{
	  "system": {"hostname": "fw-a"},
	  "interfaces": {"loop7301": {"lcp": {"hostIfName": "lan0"}}, "loop7302": {}},
	  "ha": {"vrrp": {
	    "k": {"interface": "loop7301", "vrId": 20, "engine": "keepalived", "priority": 150, "addresses": ["10.7.4.1"]},
	    "v": {"interface": "loop7302", "vrId": 21, "addresses": ["10.7.5.1"]}
	  }}}`), ds); err != nil {
		t.Fatal(err)
	}
	s := &sink{}
	Vrrp(s, ds, map[string]bool{"ha": true}, VrrpOptions{VPPEngine: true, Keepalived: true})
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	val, ok := s.value(KeepalivedKey).(*vrxv1.DesiredState)
	if !ok {
		t.Fatalf("no %s in %v", KeepalivedKey, s.kvs)
	}
	if len(val.GetHa().GetVrrp()) != 1 || val.GetHa().GetVrrp()["k"] == nil || val.GetInterfaces()["loop7301"].GetLcp().GetHostIfName() != "lan0" || val.GetSystem().GetHostname() != "fw-a" {
		t.Fatalf("stage value %s", protojson.Format(val))
	}
	out := &vrxv1.DesiredState{}
	AssembleVrrp(out, []scheduler.KV{{Key: KeepalivedKey, Value: val}}, map[string]bool{"ha": true})
	if !proto.Equal(out.GetHa().GetVrrp()["k"], ds.GetHa().GetVrrp()["k"]) {
		t.Fatalf("assembled %s", protojson.Format(out))
	}
	// not an authoritative domain: nothing
	s2 := &sink{}
	Vrrp(s2, ds, map[string]bool{"interfaces": true}, VrrpOptions{VPPEngine: true, Keepalived: true})
	if len(s2.kvs) != 0 {
		t.Fatalf("ha not in the transaction, got %v", s2.kvs)
	}
}

func TestVrrpAddressOrderPreservedOnlyForEqualLiveSet(t *testing.T) {
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"ha":{"vrrp":{"site":{"interface":"loop7301","vrId":20,"addresses":["10.7.4.254","10.7.4.1"]}}}}`), ds); err != nil {
		t.Fatal(err)
	}
	s := &sink{}
	Vrrp(s, ds, map[string]bool{"ha": true}, VrrpOptions{VPPEngine: true})
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	out := &vrxv1.DesiredState{}
	AssembleVrrp(out, s.kvs, map[string]bool{"ha": true})
	if !reflect.DeepEqual(out.GetHa().GetVrrp()["site"].GetAddresses(), ds.GetHa().GetVrrp()["site"].GetAddresses()) {
		t.Fatalf("address presentation order lost: %v", out)
	}
	for i, kv := range s.kvs {
		if kv.Key.Descriptor() == vrrp.NameVR {
			spec, err := df7.Decode[vrrp.VRSpec](kv.Value)
			if err != nil {
				t.Fatal(err)
			}
			spec.Addresses = []string{"10.7.4.99"}
			s.kvs[i].Value = df7.Encode(spec)
		}
	}
	out = &vrxv1.DesiredState{}
	AssembleVrrp(out, s.kvs, map[string]bool{"ha": true})
	if !reflect.DeepEqual(out.GetHa().GetVrrp()["site"].GetAddresses(), []string{"10.7.4.99"}) {
		t.Fatalf("saved metadata hid actual VIP drift: %v", out)
	}
}
