package agent

import (
	"context"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"testing"
)

func TestWANRoutingOnlyJoinLeaveReprojectsPPPDefaultOwnership(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	s.st.desired = doc(t, `{"interfaces":{"host-w1wan":{"enabled":true,"lcp":{"hostIfName":"w1wan"},"pppoe":{"username":"test","passwordRef":"password/test","defaultRoute":true}}}}`)
	before := proto.Clone(s.st.desired)
	for _, member := range []bool{true, false, true} {
		request := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{}}
		if member {
			request.Routing.WanGroups = []*ngfwv1.WanGroup{{Name: proto.String("internet"), Members: []*ngfwv1.WanMember{{Interface: proto.String("host-w1wan"), NextHop: proto.String("pppoe")}}}}
		}
		projected := s.projectWithBasePolicy(context.Background(), request, []string{"routing"})
		if !contains(projected.scopeDomains, "interfaces") {
			t.Fatal("routing-only commit skipped PPP ownership update")
		}
		found := false
		for _, kv := range projected.kvs {
			if kv.Key != desired.PppoeClientKey {
				continue
			}
			found = true
			value := kv.Value.(*ngfwv1.DesiredState)
			if (len(value.GetRouting().GetWanGroups()) > 0) != member {
				t.Fatal("membership stale", value)
			}
			if !value.Interfaces["host-w1wan"].Pppoe.GetDefaultRoute() {
				t.Fatal("stored default setting overwritten")
			}
		}
		if !found {
			t.Fatal("PPP singleton absent", projected.issues)
		}
	}
	if !proto.Equal(before, s.st.desired) {
		t.Fatal("projection changed stored config")
	}
}
