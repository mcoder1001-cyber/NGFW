package agent

// TD-9 fix round 1 (review M2, M3, L2, L3, L5, L6).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

var dumpTimeout = fmt.Errorf("ip_table_dump: %w", vpp.ErrTimeout)

// M2: an outcome in which a VPP reply timeout took part is never stored under the txn_id — in the plan
// (FAILED), in verify (ROLLED_BACK), or in verify and the rollback (DEGRADED). A retry with the same
// txn_id runs again once VPP answers.
func TestTimeoutOutcomesAreNeverStored(t *testing.T) {
	blue := doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`)
	cases := []struct {
		name  string
		want  vrxv1.ApplyStatus
		stage func(fv *flakyVPP)
	}{
		{"plan", vrxv1.ApplyStatus_APPLY_STATUS_FAILED, func(fv *flakyVPP) {
			fv.set(func(f *flakyVPP) { f.dumpErr = dumpTimeout })
		}},
		{"verify", vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK, func(fv *flakyVPP) {
			fv.set(func(f *flakyVPP) { // the dump after blue's last table add (verify) times out, once
				f.onInvoke = func(m api.Message) {
					if tableAdd(7002, true)(m) {
						fv.set(func(f *flakyVPP) { f.dumpErr, f.dumpErrOnce, f.onInvoke = dumpTimeout, true, nil })
					}
				}
			})
		}},
		{"verify+rollback", vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED, func(fv *flakyVPP) {
			fv.set(func(f *flakyVPP) { // every dump from verify on times out
				f.onInvoke = func(m api.Message) {
					if tableAdd(7002, true)(m) {
						fv.set(func(f *flakyVPP) { f.dumpErr, f.onInvoke = dumpTimeout, nil })
					}
				}
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fv := newFlaky()
			s := newSvcWith(t, fv, t.TempDir())
			fv.releaseAtEnd(t)
			mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
			tc.stage(fv)
			resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: blue})
			mustStatus(t, resp, tc.want)
			if !strings.Contains(resp.GetMessage()+fmt.Sprint(resp.GetResults()), "no reply in time") {
				t.Fatalf("not a timeout: %s %v", resp.GetMessage(), resp.GetResults())
			}
			fv.set(func(f *flakyVPP) { f.dumpErr, f.dumpErrOnce, f.onInvoke = nil, false, nil })
			mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: blue}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
			if !fv.HasTable(7002, false) || !fv.HasTable(7002, true) {
				t.Fatal("the retry did not apply blue")
			}
		})
	}
}

// M3: an Apply narrower than the managed domains that supersedes an owed confirm revert leaves DEGRADED
// (it does not cover every managed domain) — and the owed resync still retries until VPP converges.
func TestNarrowerSupersedeOfAnOwedRevertStillResyncs(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}},"routing":{"static":[{"prefix":"10.7.99.0/24","blackhole":true}]}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}},"routing":{}}`), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	// Another owner takes the prefix: the revert cannot restore the route and stays owed.
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: v, Owner: "w7x", Owned: ownertable.NewMemory()})
	other := scheduler.New(reg, nil)
	if r := other.Apply(context.Background(), []scheduler.KV{{Key: "ip.route/0/10.7.99.0/24", Value: &core.Route{Prefix: "10.7.99.0/24"}}}, nil); r.Outcome != scheduler.OutcomeApplied {
		t.Fatal(r.Err)
	}
	eventually(t, 5*time.Second, "the revert failed and is owed", func() bool {
		h := s.Health()
		return h.GetDegraded() && h.GetPendingConfirmTxnId() == "p1"
	})
	setRetry(s, 50*time.Millisecond, 200*time.Millisecond)
	// A vrfs-only Apply supersedes the owed revert: APPLIED, but it does not cover routing.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); !h.GetDegraded() || h.GetPendingConfirmTxnId() != "" {
		t.Fatalf("after the narrower supersede: %v", h)
	}
	if r := other.Apply(context.Background(), nil, nil); r.Outcome != scheduler.OutcomeApplied { // obstacle gone
		t.Fatal(r.Err)
	}
	eventually(t, 5*time.Second, "the owed resync restored the baseline route and cleared DEGRADED", func() bool {
		return !s.Health().GetDegraded() && v.HasRoute(0, "10.7.99.0/24")
	})
}

// L2: a panic in a wiring hook or the link watcher is counted as where="hook", not as a transaction panic.
func TestHookPanicsHaveTheirOwnLabel(t *testing.T) {
	m := newMetrics()
	a := &Agent{log: newSvc(t, coretest.New(), t.TempDir()).log, metrics: m}
	a.safely("wiring connect hook", func() { panic("hook bug") })
	out := scrape(m)
	if !strings.Contains(out, `vrx_agent_panics_total{where="hook"} 1`) || !strings.Contains(out, `vrx_agent_panics_total{where="transaction"} 0`) {
		t.Fatalf("hook panic counted as:\n%s", out)
	}
}

// L3: FlushClaims gets a context of its own — no caller cancel, a short deadline of its own, not whatever
// is left of the transaction's — and its failure answers DEGRADED, which is not stored.
func TestFlushClaimsContextAndFailure(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	setTxnTimeout(s, time.Hour)
	var deadline time.Time
	fail := errors.New("claim store: disk full")
	_ = s.lock(context.Background())
	s.flushClaims = func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		return fail
	}
	s.unlock()
	req := &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}
	resp := apply(t, s, req)
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	if !strings.Contains(resp.GetMessage(), "claims could not be flushed") {
		t.Fatalf("message %q", resp.GetMessage())
	}
	if deadline.IsZero() || time.Until(deadline) > time.Minute {
		t.Fatalf("flush deadline %v: want a short bound of its own, not the transaction's hour", deadline)
	}
	_ = s.lock(context.Background())
	s.flushClaims = func(context.Context) error { return nil }
	s.unlock()
	mustStatus(t, apply(t, s, req), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED) // not the stored DEGRADED
}

// L5: the drift check's Plan has a short bound of its own, so a slow VPP cannot keep an Apply waiting
// behind a drift walk for the transaction's minutes.
func TestDriftPlanIsBoundedTightly(t *testing.T) {
	defer setDriftPlanTimeout(200 * time.Millisecond)()
	fv := newFlaky()
	s := newSvcWith(t, fv, t.TempDir())
	fv.releaseAtEnd(t)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	fv.set(func(f *flakyVPP) { f.dumpDelay = time.Second })
	_, took := within(t, 30*time.Second, "CheckDrift", func() bool { checkDrift(s); return true })
	if took > 2*time.Second {
		t.Fatalf("the drift Plan held the transaction lock for %s", took)
	}
}

// L6: a reply timeout below govpp's health-check window refuses to start: a late reply on a reused
// channel id could otherwise reach a later call before the health check reconnects.
func TestReplyTimeoutBelowTheHealthCheckWindowRefused(t *testing.T) {
	for _, in := range []string{"5", "1500ms", "14s"} {
		t.Setenv("VRX_AGENT_VPP_REPLY_TIMEOUT", in)
		if err := ConfigFromEnv().Validate(); err == nil || !strings.Contains(err.Error(), "VRX_AGENT_VPP_REPLY_TIMEOUT") {
			t.Errorf("%q accepted: %v", in, err)
		}
	}
	for _, in := range []string{"15", "30s", "2m"} {
		t.Setenv("VRX_AGENT_VPP_REPLY_TIMEOUT", in)
		if err := ConfigFromEnv().Validate(); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
}
