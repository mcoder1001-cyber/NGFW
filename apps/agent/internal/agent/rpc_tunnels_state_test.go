package agent

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/subsystems"
)

type tunnelStats struct {
	values []api.InterfaceCounters
	err    error
}

func (f tunnelStats) InterfaceStats() ([]api.InterfaceCounters, error) { return f.values, f.err }

func TestTunnelStateLiveReadbackAndCounterAvailability(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	s := newLispSvc(t, v, t.TempDir(), false)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "state", DesiredState: doc(t, tunnelsDoc)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.AddInterface("gre9999", "GRE tunnel device", "w9:gre9999")
	r, err := s.TunnelState(context.Background(), &ngfwv1.TunnelStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tunnels) != 3 {
		t.Fatalf("owned tunnels: %v", r.Tunnels)
	}
	g := r.Tunnels[0]
	if g.Name != "to-dc" || g.Src == nil || *g.Src != "10.7.1.1" || g.Dst == nil || *g.Dst != "10.7.1.2" || g.UnderlayTableId == nil || *g.UnderlayTableId != 0 || g.Ipv4TableId == nil || *g.Ipv4TableId != 7100 || g.Ipv6TableId == nil {
		t.Fatalf("live fields: %+v", g)
	}
	if g.Counters != nil {
		t.Fatal("unavailable counters fabricated")
	}
	stats := tunnelStats{values: []api.InterfaceCounters{{InterfaceIndex: g.SwIfIndex, InterfaceName: g.Interface, Rx: api.InterfaceCounterCombined{Packets: 19, Bytes: 2000}, Tx: api.InterfaceCounterCombined{Packets: 7, Bytes: 500}}}}
	r, err = s.tunnelStateWithStats(context.Background(), &ngfwv1.TunnelStateRequest{}, stats)
	if err != nil {
		t.Fatal(err)
	}
	if c := r.Tunnels[0].Counters; c == nil || c.RxPackets != 19 || c.TxBytes != 500 {
		t.Fatalf("counter readback %+v", c)
	}
	stats.values[0].InterfaceName = "reused-index"
	r, err = s.tunnelStateWithStats(context.Background(), &ngfwv1.TunnelStateRequest{}, stats)
	if err != nil || r.Tunnels[0].Counters != nil {
		t.Fatalf("reused index counters: %v %v", r, err)
	}
	r, err = s.tunnelStateWithStats(context.Background(), &ngfwv1.TunnelStateRequest{}, tunnelStats{err: errors.New("stats unavailable")})
	if err != nil || r.Tunnels[0].Counters != nil {
		t.Fatal("failed stats must remain unavailable")
	}
}

func TestTunnelStateQueuedCancellation(t *testing.T) {
	s := newLispSvc(t, coretest.New(), t.TempDir(), false)
	s.tunnelStateOnce.Do(func() { s.tunnelStateGate = make(chan struct{}, 1) })
	gate := s.tunnelStateGate
	gate <- struct{}{}
	defer func() { <-gate }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.TunnelState(ctx, &ngfwv1.TunnelStateRequest{})
	if status.Code(err) != codes.Canceled {
		t.Fatalf("queued canceled request: %v", err)
	}
}
