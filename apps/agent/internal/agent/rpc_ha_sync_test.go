package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/binapi/ip_types"
	nat "ngfw/agent/binapi/nat44_ei"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
	"testing"
)

func TestHaSyncStateTruthAndSlotDenial(t *testing.T) {
	f := fake.New()
	f.Reply("nat44_ei_ha_get_listener", &nat.Nat44EiHaGetListenerReply{IPAddress: ip_types.IP4Address{192, 0, 2, 1}, Port: 8750, PathMtu: 1500})
	f.Reply("nat44_ei_ha_get_failover", &nat.Nat44EiHaGetFailoverReply{IPAddress: ip_types.IP4Address{192, 0, 2, 2}, Port: 8750, SessionRefreshInterval: 10})
	s, err := NewService(ServiceConfig{Owner: "w18", VPP: f, Scheduler: scheduler.New(scheduler.NewRegistry(), nil), StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"nat":{"mode":"ei"},"ha":{"cluster":{"enabled":true,"stateSync":{"nat":true,"ipsec":true,"natListener":{"address":"192.0.2.1","port":8750,"pathMtu":1500},"natFailover":{"address":"192.0.2.2","port":8750,"sessionRefreshSec":10}}}}}`), ds); err != nil {
		t.Fatal(err)
	}
	s.st.desired = ds
	out, err := s.haSyncState(context.Background(), &ngfwv1.HaSyncStateRequest{Owner: "w18"})
	if err != nil || !out.Kinds[0].Active || out.Kinds[3].Supported || out.Kinds[3].Active || out.PacketCountersAvailable || out.LastResync != nil || out.LastMissedCount != nil || out.ActionsAllowed {
		t.Fatalf("%+v %v", out, err)
	}
	ds.Ha.Cluster.Vrf = proto.String("blue")
	out, err = s.haSyncState(context.Background(), &ngfwv1.HaSyncStateRequest{})
	if err != nil || out.Kinds[0].Active {
		t.Fatalf("nondefault VRF active %+v %v", out, err)
	}
	calls := len(f.Calls())
	err = s.haSyncAction(context.Background(), &ngfwv1.HaSyncAction{Op: ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC}, func(*ngfwv1.ActionOutput) error { return nil })
	if status.Code(err) != codes.PermissionDenied || len(f.Calls()) != calls {
		t.Fatalf("slot action %v", err)
	}
	if _, err = s.haSyncState(context.Background(), &ngfwv1.HaSyncStateRequest{Owner: "other"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner %v", err)
	}
	f.SetConnected(false)
	out, err = s.haSyncState(context.Background(), &ngfwv1.HaSyncStateRequest{})
	if err != nil || out.Kinds[0].Active || out.ObservationError == "" {
		t.Fatalf("disconnected %+v %v", out, err)
	}
}
