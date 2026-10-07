package hastatesync

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	nat "ngfw/agent/binapi/nat44_ei"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp/fake"
	"testing"
	"time"
)

func runtime(t *testing.T) (*Runtime, *fake.Client) {
	t.Helper()
	f := fake.New()
	f.Reply("nat44_ei_ha_get_listener", &nat.Nat44EiHaGetListenerReply{Port: 8750})
	f.Reply("nat44_ei_ha_get_failover", &nat.Nat44EiHaGetFailoverReply{Port: 8750})
	return &Runtime{Client: f, GlobalsOwner: true, LockDir: t.TempDir(), Timeout: 50 * time.Millisecond}, f
}
func TestNonOwnerNeverCallsNative(t *testing.T) {
	r, f := runtime(t)
	r.GlobalsOwner = false
	_, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC)
	if !errors.Is(err, dfkit.ErrNotGlobalsOwner) || len(f.Calls()) != 0 {
		t.Fatalf("err=%v calls=%v", err, f.Calls())
	}
}
func TestResyncCorrelatesCompletionAndReportsMisses(t *testing.T) {
	for _, missed := range []uint32{0, 3} {
		r, f := runtime(t)
		f.On("nat44_ei_ha_resync", func(m api.Message) ([]api.Message, error) {
			p := m.(*nat.Nat44EiHaResync).PID
			f.Emit(&nat.Nat44EiHaResyncCompletedEvent{PID: p + 1})
			f.Emit(&nat.Nat44EiHaResyncCompletedEvent{PID: p, MissedCount: missed})
			return []api.Message{&nat.Nat44EiHaResyncReply{}}, nil
		})
		out, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC)
		if out.Completed != 1 || out.MissedCount != missed || out.CompletedAt.IsZero() {
			t.Fatalf("observation=%+v", out)
		}
		if (missed > 0) != errors.Is(err, ErrMissed) {
			t.Fatalf("missed=%d error=%v", missed, err)
		}
	}
}
func TestResyncTimeoutDoesNotClaimCompletion(t *testing.T) {
	r, f := runtime(t)
	f.Reply("nat44_ei_ha_resync", &nat.Nat44EiHaResyncReply{})
	_, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC)
	if !errors.Is(err, context.DeadlineExceeded) || r.Observation().Completed != 0 {
		t.Fatalf("%v %+v", err, r.Observation())
	}
}
func TestFlushOnlyFlushesQueue(t *testing.T) {
	r, f := runtime(t)
	f.Reply("nat44_ei_ha_flush", &nat.Nat44EiHaFlushReply{})
	_, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_FLUSH)
	if err != nil || len(f.CallsNamed("nat44_ei_ha_flush")) != 1 || len(f.CallsNamed("nat44_ei_ha_resync")) != 0 {
		t.Fatal(err)
	}
}
func TestNativeFailureAndDisabledEndpoints(t *testing.T) {
	r, f := runtime(t)
	f.Reply("nat44_ei_ha_resync", &nat.Nat44EiHaResyncReply{Retval: -1})
	if _, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC); err == nil {
		t.Fatal("native error hidden")
	}
	f.Reply("nat44_ei_ha_get_listener", &nat.Nat44EiHaGetListenerReply{})
	if _, err := r.Run(context.Background(), ngfwv1.HaSyncOp_HA_SYNC_OP_FLUSH); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatalf("%v", err)
	}
}
