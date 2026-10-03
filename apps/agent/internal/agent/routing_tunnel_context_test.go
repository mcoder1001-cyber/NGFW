package agent

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"testing"
)

func TestRoutingOnlyApplyUsesPersistedLogicalTunnelContext(t *testing.T) {
	t.Setenv("NGFW_VPP_TABLE_BASE", "7000")
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	baseline := doc(t, `{"interfaces":{"loop7001":{"ipv4":["198.18.7.1/24"]}},"tunnels":{"ipip":{"site":{"instance":7001,"src":"198.18.7.1","dst":"198.18.7.2","ipv4":["10.7.1.1/30"]}}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "logical-baseline", DesiredState: baseline}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	partial := doc(t, `{"routing":{"static":[{"prefix":"10.7.2.0/24","nextHops":[{"address":"10.7.1.2","interface":"site"}]}]}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "logical-route", DesiredState: partial, Subsystems: []string{"routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if partial.Tunnels != nil {
		t.Fatal("request mutated with unrequested tunnel context")
	}
	got, err := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if err != nil {
		t.Fatal(err)
	}
	routes := got.GetDesiredState().GetRouting().GetStatic()
	if len(routes) != 1 || routes[0].GetNextHops()[0].GetInterface() != "site" {
		t.Fatal("routing-only readback lost logical interface", routes)
	}
	if got.GetDesiredState().Tunnels != nil {
		t.Fatal("unrequested tunnel domain leaked into readback")
	}
	empty := doc(t, `{"routing":{},"tunnels":{}}`)
	if s.routingProjectionState(empty, []string{"routing"}) != empty {
		t.Fatal("explicit empty domain replaced")
	}
}
