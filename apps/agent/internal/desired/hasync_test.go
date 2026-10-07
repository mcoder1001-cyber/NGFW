package desired

import (
	"google.golang.org/protobuf/encoding/protojson"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/scheduler"
	"testing"
)

func haConfig(t *testing.T) *ngfwv1.DesiredState {
	t.Helper()
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"interfaces":{"loop18":{"ipv4":["192.0.2.1/30"]}},"nat":{"mode":"ei"},"ha":{"cluster":{"enabled":true,"interface":"loop18","stateSync":{"nat":true,"acl":true,"ipsec":true,"natListener":{"address":"192.0.2.1","port":8750,"pathMtu":1500},"natFailover":{"address":"192.0.2.2","port":8750,"sessionRefreshSec":10}}}}}`), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}
func TestHaSyncProjectionAndObservableAssembly(t *testing.T) {
	ds := haConfig(t)
	s := &sink{}
	HaSync(s, ds, map[string]bool{"ha": true})
	if len(s.errs) != 0 || len(s.kvs) != 2 {
		t.Fatalf("%v %v", s.errs, s.kvs)
	}
	out := &ngfwv1.DesiredState{}
	AssembleHaSync(out, s.kvs, map[string]bool{"ha": true})
	if out.GetHa().GetCluster().GetStateSync().GetNatListener().GetAddress() != "192.0.2.1" || out.GetHa().GetCluster().Enabled != nil || out.GetHa().GetCluster().SecretRef != nil {
		t.Fatalf("fabricated membership: %s", protojson.Format(out))
	}
	empty := &ngfwv1.DesiredState{}
	AssembleHaSync(empty, []scheduler.KV{}, map[string]bool{"ha": true})
	if empty.Ha != nil {
		t.Fatal("fabricated empty cluster")
	}
}
func TestHaSyncUnsupportedAndValidation(t *testing.T) {
	for _, mode := range []string{"ed", "ei"} {
		ds := haConfig(t)
		ds.Nat.Mode = &mode
		s := &sink{}
		HaSync(s, ds, map[string]bool{"ha": true})
		if len(s.errs) != 0 {
			t.Fatal(s.errs)
		}
		if mode == "ed" && len(s.kvs) != 0 {
			t.Fatal("ED globals applied")
		}
	}
	ds := haConfig(t)
	a := "192.0.2.9"
	ds.Ha.Cluster.StateSync.NatListener.Address = &a
	s := &sink{}
	HaSync(s, ds, map[string]bool{"ha": true})
	if len(s.errs) == 0 || len(s.kvs) != 0 {
		t.Fatal("nonlocal listener accepted")
	}
	ds = haConfig(t)
	b := false
	ds.Ha.Cluster.Enabled = &b
	s = &sink{}
	HaSync(s, ds, map[string]bool{"ha": true})
	if len(s.errs) != 3 {
		t.Fatalf("disabled flags: %v", s.errs)
	}
}
