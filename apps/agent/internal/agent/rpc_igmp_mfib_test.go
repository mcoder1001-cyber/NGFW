package agent

import (
	"context"
	"fmt"
	"go.fd.io/govpp/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	bin "ngfw/agent/binapi/igmp"
	"ngfw/agent/binapi/ip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/scheduler"
	"testing"
	"time"
)

func TestMulticastStateOwnerConnectivityAndLiveDumps(t *testing.T) {
	ctx := context.Background()
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "in", Tag: "w1:in"}, dfkittest.Iface{Index: 2, Name: "out", Tag: "w1:out"}, dfkittest.Iface{Index: 3, Name: "foreign", Tag: "other:foreign"})
	var routes []ip.IPMroute
	f.On("ip_mtable_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ip.IPMtableDetails{Table: ip.IPTable{TableID: 1001, Name: "w1:test"}}}, nil
	})
	f.On("ip_mroute_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, r := range routes {
			out = append(out, &ip.IPMrouteDetails{Route: r})
		}
		return out, nil
	})
	f.On("ip_mroute_add_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*ip.IPMrouteAddDel)
		if r.IsAdd {
			routes = append(routes, r.Route)
		}
		return []api.Message{&ip.IPMrouteAddDelReply{}}, nil
	})
	f.On("igmp_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{&bin.IgmpDetails{SwIfIndex: 1, Gaddr: [4]byte{232, 1, 2, 3}, Saddr: [4]byte{10, 1, 0, 1}}, &bin.IgmpDetails{SwIfIndex: 3, Gaddr: [4]byte{232, 9, 9, 9}, Saddr: [4]byte{10, 9, 0, 1}}}, nil
	})
	d := mfib.New(f, "w1", dfkit.NewMemoryBootStore(), df7.WithIDRange(1000, 1999))
	r := mfib.Route{Table: 1001, Group: "239.1.2.3", Source: "10.1.0.1", Paths: []mfib.Path{{Interface: "in", Flags: "accept"}, {Interface: "out", Flags: "forward"}}}
	if _, e := d.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal(e)
	}
	reg := scheduler.NewRegistry()
	reg.Register(d)
	svc := &Service{owner: "w1", vpp: f, sched: scheduler.New(reg, nil), txn: make(chan struct{}, 1), now: time.Now, vrfIDs: map[string]uint32{"test": 1001}}
	if _, e := svc.MulticastState(ctx, &ngfwv1.MulticastStateRequest{Owner: "other"}); status.Code(e) != codes.InvalidArgument {
		t.Fatalf("owner: %v", e)
	}
	out, e := svc.MulticastState(ctx, &ngfwv1.MulticastStateRequest{Owner: "w1"})
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Groups) != 1 || len(out.Mroutes) != 1 || out.Mroutes[0].Vrf != "test" || out.Mroutes[0].Accept != "in" || out.Mroutes[0].Forward[0] != "out" {
		t.Fatalf("state %+v", out)
	}
	f.On("igmp_dump", func(api.Message) ([]api.Message, error) { return nil, fmt.Errorf("plugin failed") })
	if _, e := svc.MulticastState(ctx, nil); status.Code(e) != codes.Internal {
		t.Fatalf("failure: %v", e)
	}
	f.SetConnected(false)
	if _, e := svc.MulticastState(ctx, nil); status.Code(e) != codes.Unavailable {
		t.Fatalf("disconnected: %v", e)
	}
}
