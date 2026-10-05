package agent

import (
	"context"
	"go.fd.io/govpp/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	binbfd "ngfw/agent/binapi/bfd"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"strings"
	"testing"
)

func TestBfdApplyStateRestartAndRollback(t *testing.T)         { testBfdLifecycle(t, false) }
func TestBfdMultihopApplyStateRestartAndRollback(t *testing.T) { testBfdLifecycle(t, true) }
func testBfdLifecycle(t *testing.T, multihop bool) {
	v := coretest.New()
	dir := t.TempDir()
	newService := newSvc
	if multihop {
		newService = newOwnerSvc
	}
	s := newService(t, v, dir)
	raw := `{"interfaces":{"loop1401":{"ipv4":["10.14.1.1/24"]}},"routing":{"bfd":{"sessions":[{"interface":"loop1401","localAddress":"10.14.1.1","peerAddress":"10.14.1.2","desiredMinTxUs":300000,"requiredMinRxUs":300000,"detectMultiplier":3,"enabled":true}]}}}`
	if multihop {
		raw = strings.Replace(raw, `"enabled":true`, `"enabled":true,"multihop":true`, 1)
	}
	ds := doc(t, raw)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "bfd-create", DesiredState: ds, Subsystems: []string{"interfaces", "routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	live := func(s *Service) {
		t.Helper()
		r, e := (&server{svc: s}).BfdState(context.Background(), &ngfwv1.BfdStateRequest{Owner: testOwner})
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Sessions) != 1 || r.Sessions[0].PeerAddress != "10.14.1.2" || r.Sessions[0].State != "down" || r.Sessions[0].DesiredMinTxUs != 300000 || r.Sessions[0].Multihop != multihop {
			t.Fatalf("%v", r)
		}
	}
	live(s)
	if _, e := (&server{svc: s}).BfdState(context.Background(), &ngfwv1.BfdStateRequest{Owner: "foreign"}); status.Code(e) != codes.InvalidArgument {
		t.Fatalf("owner %v", e)
	}
	s.Close()
	s = newService(t, v, dir)
	live(s)
	got, e := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"routing"}})
	if e != nil || len(got.GetDesiredState().GetRouting().GetBfd().GetSessions()) != 1 {
		t.Fatalf("retrieve %v %v", got, e)
	}
	empty := doc(t, `{"routing":{}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "bfd-remove", DesiredState: empty, Subsystems: []string{"routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	r, e := (&server{svc: s}).BfdState(context.Background(), &ngfwv1.BfdStateRequest{Owner: testOwner})
	if e != nil || len(r.Sessions) != 0 {
		t.Fatalf("rollback %v %v", r, e)
	}
}

func TestFRRBfdObservedTimers(t *testing.T) {
	// Upstream show bfd peers json shape: operational timers are top-level ms.
	raw := `[{"peer":"192.0.2.2","local":"192.0.2.1","interface":"eth0","status":"up","multihop":false,"receive-interval":300,"transmit-interval":500,"detect-multiplier":5,"remote-receive-interval":100,"remote-transmit-interval":100}]`
	peers, err := decodeFRRBfdPeers(raw)
	if err != nil || len(peers) != 1 {
		t.Fatalf("decode: %v %v", peers, err)
	}
	p := peers[0]
	if p.RequiredMinRxUs != 300000 || p.DesiredMinTxUs != 500000 || p.DetectMultiplier != 5 || p.State != "up" {
		t.Fatalf("observed timers: %v", p)
	}
	if _, err := decodeFRRBfdPeers(`{`); err == nil {
		t.Fatal("invalid response accepted")
	}
}
func TestRedistributionCountsUseTargetFamily(t *testing.T) {
	edges := []*ngfwv1.RedistributionEdge{{Source: "static", Target: "ospf", Vrf: "default"}, {Source: "static", Target: "ospf6", Vrf: "default"}, {Source: "static", Target: "ripng", Vrf: "default"}, {Source: "bgp", Target: "ospf6", Vrf: "default"}}
	applyRedistributionCounts(edges, map[string]uint32{"ipv4/default/static": 7, "ipv6/default/static": 11})
	for i, want := range []uint64{7, 11, 11} {
		if edges[i].RouteCount == nil || *edges[i].RouteCount != want {
			t.Fatalf("edge %d: %v", i, edges[i])
		}
	}
	if edges[3].RouteCount != nil {
		t.Fatal("unknown count must remain absent")
	}
}

func TestBfdMultihopFailedApplyRollbackReleasesTuple(t *testing.T) {
	v := coretest.New()
	s := newOwnerSvc(t, v, t.TempDir())
	initial := doc(t, `{"interfaces":{"loop1401":{"ipv4":["10.14.1.1/24"]}},"routing":{}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "mh-initial", DesiredState: initial, Subsystems: []string{"interfaces", "routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.On("bfd_udp_session_set_flags", func(api.Message) ([]api.Message, error) {
		return []api.Message{&binbfd.BfdUDPSessionSetFlagsReply{Retval: -1}}, nil
	})
	failed := doc(t, `{"routing":{"bfd":{"sessions":[{"interface":"loop1401","localAddress":"10.14.1.1","peerAddress":"10.14.1.2","multihop":true,"enabled":false}]}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "mh-fail", DesiredState: failed, Subsystems: []string{"routing"}}), ngfwv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	live, err := (&server{svc: s}).BfdState(t.Context(), &ngfwv1.BfdStateRequest{Owner: testOwner})
	if err != nil || len(live.Sessions) != 0 {
		t.Fatalf("rollback left session: %v %v", live, err)
	}
}

func TestFRRBfdObservedTimerOverflowRejected(t *testing.T) {
	for _, raw := range []string{
		`[{"receive-interval":4294968,"transmit-interval":1}]`,
		`[{"receive-interval":1,"transmit-interval":4294968}]`,
	} {
		if peers, err := decodeFRRBfdPeers(raw); err == nil || peers != nil {
			t.Fatal("overflowing observed timer accepted")
		}
	}
	if peers, err := decodeFRRBfdPeers(`[{"receive-interval":4294967,"transmit-interval":4294967}]`); err != nil || len(peers) != 1 || peers[0].RequiredMinRxUs != 4294967000 {
		t.Fatalf("largest representable interval: %v %v", peers, err)
	}
}
