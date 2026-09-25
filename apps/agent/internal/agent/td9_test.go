package agent

// TD-9: bounded VPP calls and transaction semantics (REVIEW-2026-09-24 §1). New API is reached only
// through td9_helpers_test.go, so these tests also compile against the base, where the evidence run
// swaps the helpers for a shim that encodes the base's behaviour (docs/status/tasks/TD-9.md).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/memclnt"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
)

// flakyVPP is the fake VPP model with the failure modes of a VPP that hangs or dies mid-request.
type flakyVPP struct {
	*coretest.VPP
	mu        sync.Mutex
	hang      func(api.Message) bool // the request reaches VPP (the model applies it), the reply never comes
	slow      func(api.Message) bool // VPP answers only after delay
	delay     time.Duration
	failDumps bool // every dump fails (a resync that cannot read VPP)
	// fix round 1: a dump fails with dumpErr (once: only the next one); a dump waits dumpDelay (or its ctx);
	// onInvoke sees every request before it is served
	dumpErr     error
	dumpErrOnce bool
	dumpDelay   time.Duration
	onInvoke    func(api.Message)
	released    chan struct{}
}

func newFlaky() *flakyVPP { return &flakyVPP{VPP: coretest.New(), released: make(chan struct{})} }

// releaseAtEnd makes whatever a failed test leaves hanging return when the test ends. Call it after
// the service or agent was created: cleanups run last-in first-out, so this one runs before their
// Close/Stop, which takes the transaction lock.
func (f *flakyVPP) releaseAtEnd(t *testing.T) { t.Cleanup(func() { close(f.released) }) }

func (f *flakyVPP) set(fn func(f *flakyVPP)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *flakyVPP) hangOn(p func(api.Message) bool) { f.set(func(f *flakyVPP) { f.hang = p }) }

// Invoke: a hanging request is applied by VPP, whose answer never comes — only ctx (govpp waits on
// nothing else) or the test's end ends the wait.
func (f *flakyVPP) Invoke(ctx context.Context, req, reply api.Message) error {
	f.mu.Lock()
	hang, slow, delay, on := f.hang, f.slow, f.delay, f.onInvoke
	f.mu.Unlock()
	if on != nil {
		on(req)
	}
	if hang != nil && hang(req) {
		_ = f.VPP.Invoke(context.WithoutCancel(ctx), req, reply)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.released:
			return errors.New("fake vpp: released at the end of the test")
		}
	}
	if slow != nil && slow(req) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.VPP.Invoke(ctx, req, reply)
}

func (f *flakyVPP) NewStream(ctx context.Context, opts ...api.StreamOption) (api.Stream, error) {
	f.mu.Lock()
	fail, derr, delay := f.failDumps, f.dumpErr, f.dumpDelay
	if f.dumpErrOnce {
		f.dumpErr, f.dumpErrOnce = nil, false
	}
	f.mu.Unlock()
	if fail {
		return nil, errors.New("fake vpp: dump refused")
	}
	if derr != nil {
		return nil, derr
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.VPP.NewStream(ctx, opts...)
}

// flakyConn is a flakyVPP with connection-state notifications (Start's dialVPP).
type flakyConn struct {
	*flakyVPP
	states chan vpp.ConnState
}

func (f *flakyConn) States() <-chan vpp.ConnState { return f.states }
func (f *flakyConn) Close()                       {}

func tableAdd(id uint32, v6 bool) func(api.Message) bool {
	return func(m api.Message) bool {
		r, ok := m.(*ip.IPTableAddDel)
		return ok && r.IsAdd && r.Table.TableID == id && r.Table.IsIP6 == v6
	}
}

func tableDel(id uint32) func(api.Message) bool {
	return func(m api.Message) bool {
		r, ok := m.(*ip.IPTableAddDel)
		return ok && !r.IsAdd && r.Table.TableID == id
	}
}

// newSvcWith is newSvc over any client: the descriptors and the service both use c; model is the
// VPP model behind it (its boot identity).
func newSvcWith(t *testing.T, c vpp.Client, dir string) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: c, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: c, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc
}

func setRetry(s *Service, lo, hi time.Duration) {
	_ = s.lock(context.Background())
	s.retryMin, s.retryMax = lo, hi
	s.unlock()
}

// within runs f and fails the test when it does not return within d.
func within[T any](t *testing.T, d time.Duration, what string, f func() T) (T, time.Duration) {
	t.Helper()
	done := make(chan T, 1)
	start := time.Now()
	go func() { done <- f() }()
	select {
	case r := <-done:
		return r, time.Since(start)
	case <-time.After(d):
		t.Fatalf("%s did not return within %s", what, d)
		var zero T
		return zero, 0
	}
}

// eventually polls cond every 20 ms until it holds or d passes.
func eventuallyWithin(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("not within %s: %s", d, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func scrape(m *metrics) string {
	rec := httptest.NewRecorder()
	m.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

// Review 1.1 — the acceptance: VPP applies a request and dies before it answers. Apply returns within
// the reply timeout + ε, DEGRADED (the outcome is unknown, never ROLLED_BACK), with a resync owed that
// converges VPP to the stored desired state; the answer is not stored under the txn_id (1.4), and the
// next Apply — the retry with the same txn_id included — works.
func TestApplyReturnsWhenVPPNeverReplies(t *testing.T) {
	const replyTimeout = 300 * time.Millisecond
	fv := newFlaky()
	s := newSvcWith(t, bounded(fv, replyTimeout), t.TempDir())
	fv.releaseAtEnd(t)
	setRetry(s, 50*time.Millisecond, 200*time.Millisecond)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	fv.hangOn(tableAdd(7002, true)) // VPP creates blue's IPv6 table, then never answers
	blue := &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`)}
	resp, took := within(t, replyTimeout+5*time.Second, "Apply", func() *vrxv1.ApplyResponse { return apply(t, s, blue) })
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	if took < replyTimeout || !strings.Contains(resp.GetMessage(), "outcome of that operation is unknown") {
		t.Fatalf("after %s: %q", took, resp.GetMessage())
	}
	if h := s.Health(); !h.GetDegraded() || h.GetLastTxnId() != "t1" {
		t.Fatalf("health %v", h)
	}
	// The owed resync (backoff 50 ms) converges VPP to the stored desired state: the table VPP created
	// without answering is gone, DEGRADED is cleared — although VPP still hangs on that request.
	eventuallyWithin(t, 5*time.Second, "the owed resync removed the unanswered table and cleared DEGRADED", func() bool {
		return !s.Health().GetDegraded() && !fv.HasTable(7002, true)
	})
	if !fv.HasTable(7001, false) || !fv.HasTable(7001, true) {
		t.Fatal("the resync lost red")
	}
	// VPP answers again: the retry with the same txn_id runs the transaction (nothing was stored).
	fv.hangOn(nil)
	mustStatus(t, apply(t, s, blue), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !fv.HasTable(7002, false) || !fv.HasTable(7002, true) || s.Health().GetLastTxnId() != "t2" {
		t.Fatalf("retry of t2 not applied: health %v", s.Health())
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
}

// Review 1.4: the caller's deadline bounds only the wait for the lock. A transaction that started runs
// to its end although the caller gave up; its outcome is stored, so the caller's retry gets it.
func TestCallerDeadlineDoesNotCutTheTransaction(t *testing.T) {
	fv := newFlaky()
	s := newSvcWith(t, fv, t.TempDir())
	fv.releaseAtEnd(t)
	fv.set(func(f *flakyVPP) { f.slow, f.delay = tableAdd(7001, false), 400*time.Millisecond })
	red := &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	resp, err := s.Apply(ctx, red)
	if err != nil {
		t.Fatal(err)
	}
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !fv.HasTable(7001, false) {
		t.Fatal("not applied")
	}
	n := len(fv.CallsNamed("ip_table_add_del"))
	mustStatus(t, apply(t, s, red), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED) // the retry: the stored response
	if len(fv.CallsNamed("ip_table_add_del")) != n {
		t.Fatal("the retry re-applied instead of returning the stored response")
	}
	// A caller that gives up while waiting for the lock gets DEADLINE_EXCEEDED and nothing is stored.
	_ = s.lock(context.Background())
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	blue := &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`)}
	_, err = s.Apply(ctx2, blue)
	s.unlock()
	if grpcCode(err) != codes.DeadlineExceeded {
		t.Fatalf("waiting for the lock: %v", err)
	}
	mustStatus(t, apply(t, s, blue), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !fv.HasTable(7002, false) {
		t.Fatal("t2 retry not applied")
	}
}

// Review 1.1: a resync and a confirm revert run on a deadline of their own. With a VPP that never
// answers (and no per-reply bound), each returns once that deadline passed, DEGRADED, and releases the
// transaction lock; the owed resync / revert converges once VPP answers again.
func TestResyncAndRevertHaveTheirOwnDeadline(t *testing.T) {
	const txn = 300 * time.Millisecond
	fv := newFlaky()
	s := newSvcWith(t, fv, t.TempDir())
	fv.releaseAtEnd(t)
	setTxnTimeout(s, txn)
	setRetry(s, 50*time.Millisecond, 200*time.Millisecond)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "base", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	// Resync: red's IPv4 table was lost; VPP re-creates it and never answers.
	fv.DeleteTable(7001, false)
	fv.hangOn(tableAdd(7001, false))
	resp, took := within(t, txn+5*time.Second, "Resync", func() *vrxv1.ApplyResponse { return s.Resync(context.Background()) })
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	if took < txn || !s.Health().GetDegraded() {
		t.Fatalf("resync after %s, health %v", took, s.Health())
	}
	fv.hangOn(nil)
	eventuallyWithin(t, 5*time.Second, "the owed resync converged", func() bool { return !s.Health().GetDegraded() && fv.HasTable(7001, false) })

	// Confirm revert: p1 adds blue; at the deadline the revert deletes it and VPP never answers.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`), ConfirmTimeoutSec: 1}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	fv.hangOn(tableDel(7002))
	eventuallyWithin(t, 1*time.Second+txn+5*time.Second, "the revert gave up and the agent is DEGRADED", func() bool {
		h := s.Health()
		return h.GetDegraded() && h.GetPendingConfirmTxnId() == "p1" && !h.GetReconcileInProgress()
	})
	// The lock is free between attempts: a late confirm is answered, not left waiting.
	cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.Apply(cctx, &vrxv1.ApplyRequest{ConfirmTxnId: "p1"}); grpcCode(err) != codes.FailedPrecondition {
		t.Fatalf("late confirm while the revert is owed: %v", err)
	}
	fv.hangOn(nil)
	eventuallyWithin(t, 5*time.Second, "the owed revert converged", func() bool {
		h := s.Health()
		return h.GetPendingConfirmTxnId() == "" && !h.GetDegraded() && !fv.HasTable(7002, false) && !fv.HasTable(7002, true)
	})
}

// Review 1.1b: a failed resync is retried with backoff (retryMin doubling to retryMax) while DEGRADED,
// without a VPP reconnect, until one succeeds.
func TestOwedResyncRetriedWithBackoff(t *testing.T) {
	fv := newFlaky()
	s := newSvcWith(t, fv, t.TempDir())
	fv.releaseAtEnd(t)
	setRetry(s, 50*time.Millisecond, 200*time.Millisecond)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_START}})
	defer s.events().unsubscribe(sub)
	fv.set(func(f *flakyVPP) { f.failDumps = true })
	fv.DeleteTable(7001, false) // lost while VPP cannot even be read
	if resp := s.Resync(context.Background()); resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || !s.Health().GetDegraded() {
		t.Fatalf("resync against an unreadable VPP: %v", resp)
	}
	evs := collect(t, sub, 4) // the failed resync + 3 retries
	for _, e := range evs {
		if !strings.HasPrefix(e.GetMessage(), "resync") {
			t.Fatalf("event %v", e)
		}
	}
	if d := resyncDelayOf(s); d != 200*time.Millisecond {
		t.Fatalf("backoff after 3 failed retries = %s, want retryMax 200ms", d)
	}
	fv.set(func(f *flakyVPP) { f.failDumps = false })
	eventuallyWithin(t, 5*time.Second, "a retry succeeded", func() bool { return !s.Health().GetDegraded() && fv.HasTable(7001, false) })
}

// Review 1.1b with the agent: the owed resync takes the agent's resync path (Env.Resync → watchVPP),
// so the wiring's after-resync hook runs too.
func TestOwedResyncTakesTheAgentsResyncPath(t *testing.T) {
	old := resyncMinInterval
	resyncMinInterval = 0
	t.Cleanup(func() { resyncMinInterval = old })
	fc := &flakyConn{flakyVPP: newFlaky(), states: make(chan vpp.ConnState, 4)}
	oldDial := dialVPP
	dialVPP = func(string, vpp.ConnOptions) vppConn { return fc }
	t.Cleanup(func() { dialVPP = oldDial })
	a, err := Start(context.Background(), testConfig(t), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Stop)
	fc.releaseAtEnd(t)
	sub := a.svc.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	defer a.svc.events().unsubscribe(sub)
	fc.states <- vpp.ConnState{Connected: true}
	collect(t, sub, 1)
	setRetry(a.svc, 50*time.Millisecond, 200*time.Millisecond)
	mustStatus(t, apply(t, a.svc, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	fc.set(func(f *flakyVPP) { f.failDumps = true })
	fc.DeleteTable(7001, true)
	a.wiring.RequestResync()
	eventuallyWithin(t, 5*time.Second, "the requested resync failed", func() bool { return a.svc.Health().GetDegraded() })
	fc.set(func(f *flakyVPP) { f.failDumps = false })
	eventuallyWithin(t, 5*time.Second, "the owed resync repaired VPP through the agent", func() bool {
		return !a.svc.Health().GetDegraded() && fc.HasTable(7001, true)
	})
}

// Review 1.3: confirm-and-apply with VPP down answers UNAVAILABLE before the confirm half: the pending
// transaction stays pending (nothing half-done).
func TestConfirmAndApplyWhileVPPDownConfirmsNothing(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`), ConfirmTimeoutSec: 60}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.SetConnected(false)
	_, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "t2", ConfirmTxnId: "p1", DesiredState: doc(t, `{"vrfs":{"blue":{"id":7002}}}`)})
	if grpcCode(err) != codes.Unavailable {
		t.Fatalf("confirm-and-apply with VPP down: %v", err)
	}
	if h := s.Health(); h.GetPendingConfirmTxnId() != "p1" || h.GetLastTxnId() == "p1" {
		t.Fatalf("the confirm half ran although the apply half could not: %v", h)
	}
	v.SetConnected(true)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", ConfirmTxnId: "p1", DesiredState: doc(t, `{"vrfs":{"blue":{"id":7002}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); h.GetPendingConfirmTxnId() != "" || h.GetLastTxnId() != "t2" {
		t.Fatalf("after the retry: %v", h)
	}
}

// Review 1.2: the projection's warnings reach APPLIED and ROLLED_BACK answers (ok = true), not only
// DryRun.
func TestApplyAnswersCarryWarnings(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	warned := func(resp *vrxv1.ApplyResponse) bool {
		rep := resp.GetValidation()
		return rep.GetOk() && len(rep.GetErrors()) == 1 && rep.GetErrors()[0].GetRule() == "agent.unimplemented-domain" &&
			rep.GetErrors()[0].GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING && rep.GetErrors()[0].GetPointer() == "/system"
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}) // sampleDoc has "system"
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !warned(resp) {
		t.Fatalf("APPLIED without the warning: %v", resp.GetValidation())
	}
	bad := doc(t, sampleDoc)
	bad.Interfaces["loop703"] = &vrxv1.Interface{Vrf: proto.String("red"), Ipv4: []string{"10.7.1.2/24"}}
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: bad})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	if !warned(resp) {
		t.Fatalf("ROLLED_BACK without the warning: %v", resp.GetValidation())
	}
	// No warning, no report: an answer without warnings is unchanged.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)})
	if resp.GetValidation() != nil {
		t.Fatalf("validation without warnings: %v", resp.GetValidation())
	}
}

// Review 1.5b: the confirm window starts when the transaction was applied (applied_at + timeout,
// proto.md §4), so a slow apply does not eat into it.
func TestConfirmWindowStartsAtAppliedAt(t *testing.T) {
	fv := newFlaky()
	s := newSvcWith(t, fv, t.TempDir())
	fv.releaseAtEnd(t)
	fv.set(func(f *flakyVPP) { f.slow, f.delay = tableAdd(7001, false), 300*time.Millisecond })
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`), ConfirmTimeoutSec: 2})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := resp.GetConfirmDeadline().AsTime().Sub(resp.GetAppliedAt().AsTime()); got != 2*time.Second {
		t.Fatalf("confirm_deadline − applied_at = %s, want 2s", got)
	}
	if h := s.Health(); !h.GetConfirmDeadline().AsTime().Equal(resp.GetConfirmDeadline().AsTime()) {
		t.Fatalf("health deadline %v", h.GetConfirmDeadline())
	}
}

// Tech-debt "P05 owed revert dropped": only an APPLIED transaction supersedes an owed confirm revert; a
// failed one leaves it owed (still pending, still retried), so the unconfirmed config does not stay.
func TestFailedApplyKeepsTheOwedRevert(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	other := revertObstacle(t, v, s)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "bad", DesiredState: doc(t, `{"routing":{"static":[{"prefix":"10.7.67.0/24","blackhole":false}]}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	if h := s.Health(); h.GetPendingConfirmTxnId() != "p1" || !h.GetDegraded() || !s.st.meta.Reverting {
		t.Fatalf("a failed apply dropped the owed revert: %v reverting=%v", h, s.st.meta.Reverting)
	}
	setRetry(s, 50*time.Millisecond, 200*time.Millisecond)
	if r := other.Apply(context.Background(), nil, nil); r.Outcome != scheduler.OutcomeApplied { // obstacle removed
		t.Fatal(r.Err)
	}
	_ = s.lock(context.Background())
	s.scheduleRetryLocked("p1") // re-arm on the short backoff (it was armed on the test's 1 h)
	s.unlock()
	eventuallyWithin(t, 5*time.Second, "the owed revert converged", func() bool {
		h := s.Health()
		return h.GetPendingConfirmTxnId() == "" && !h.GetDegraded()
	})
	if !v.HasRoute(0, "10.7.99.0/24") {
		t.Fatal("baseline route not restored")
	}
}

// Review 1.1d: a panic in a gRPC handler answers INTERNAL and is counted; the agent stays up.
func TestGRPCHandlerPanicAnswersInternal(t *testing.T) {
	m := newMetrics()
	g := newRecoveringServer(nil, m)
	vrxv1.RegisterDataplaneServer(g, &server{svc: nil, log: nil}) // every handler dereferences svc
	sock := filepath.Join(t.TempDir(), "agent.sock")
	l, err := listenUnix(sock, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = g.Serve(l) }()
	defer g.Stop()
	cc, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cc.Close() }()
	c := vrxv1.NewDataplaneClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Health(ctx, &vrxv1.HealthRequest{}); grpcCode(err) != codes.Internal {
		t.Fatalf("unary panic: %v", err)
	}
	st, err := c.StreamEvents(ctx, &vrxv1.StreamEventsRequest{})
	if err == nil {
		_, err = st.Recv()
	}
	if grpcCode(err) != codes.Internal {
		t.Fatalf("stream panic: %v", err)
	}
	if out := scrape(m); !strings.Contains(out, `vrx_agent_panics_total{where="grpc"} 2`) {
		t.Fatalf("panics not counted:\n%s", out)
	}
}

// Review 1.1d: a panic in the agent's own transaction code answers INTERNAL, releases the lock, reloads
// the state from disk and leaves the agent DEGRADED with a resync owed; the next Apply works.
func TestPanicInATransactionIsContained(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	_ = s.lock(context.Background())
	before := s.beforeTxn
	s.beforeTxn = func() { panic("agent bug") }
	s.unlock()
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Apply panicked: %v", r)
			}
		}()
		_, err = s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, `{"vrfs":{"blue":{"id":7002}}}`)})
	}()
	if grpcCode(err) != codes.Internal || !s.Health().GetDegraded() {
		t.Fatalf("err %v health %v", err, s.Health())
	}
	_ = s.lock(context.Background())
	s.beforeTxn = before
	s.unlock()
	resp, _ := within(t, 5*time.Second, "the next Apply", func() *vrxv1.ApplyResponse {
		return apply(t, s, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, `{"vrfs":{"red":{"id":7001},"blue":{"id":7002}}}`)})
	})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if s.Health().GetDegraded() {
		t.Fatal("still degraded after a successful apply over every managed domain")
	}
}

// Review 1.1c: the link-event watcher restarts (with backoff) after an error while VPP stays connected.
func TestLinkEventsWatcherRestarts(t *testing.T) {
	defer setLinkRetry(20*time.Millisecond, 100*time.Millisecond)()
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	v.Fail("want_interface_events", errors.New("vpp busy"))
	fc := &fakeConn{VPP: v, states: make(chan vpp.ConnState, 4)}
	a := &Agent{log: s.log, conn: fc, svc: s, metrics: s.metrics}
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE, vrxv1.EventKind_EVENT_KIND_LINK_UP}})
	defer s.events().unsubscribe(sub)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.watchVPP(ctx); close(done) }()
	defer func() { cancel(); <-done; a.wg.Wait() }()
	fc.states <- vpp.ConnState{Connected: true}
	collect(t, sub, 1)                 // the connect resync
	time.Sleep(100 * time.Millisecond) // the first watches fail
	v.Reply("want_interface_events", &interfaces.WantInterfaceEventsReply{})
	eventuallyWithin(t, 3*time.Second, "the link watcher came back", func() bool {
		return v.Emit(&interfaces.SwInterfaceEvent{SwIfIndex: 0, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP | interface_types.IF_STATUS_API_FLAG_LINK_UP}) > 0
	})
	if evs := collect(t, sub, 1); evs[0].GetKind() != vrxv1.EventKind_EVENT_KIND_LINK_UP || evs[0].GetInterface() != "local0" {
		t.Fatalf("event %v", evs)
	}
}

// P08's boot-identity ControlPing on connect has a deadline of its own: a VPP that does not answer it
// no longer keeps watchVPP from the resync.
func TestConnectHookHasItsOwnDeadline(t *testing.T) {
	defer setConnectHookTimeout(200 * time.Millisecond)()
	fc := &flakyConn{flakyVPP: newFlaky(), states: make(chan vpp.ConnState, 4)}
	var first atomic.Bool // only the connect hook's ControlPing (the boot identity) goes unanswered
	fc.hangOn(func(m api.Message) bool {
		_, ok := m.(*memclnt.ControlPing)
		return ok && first.CompareAndSwap(false, true)
	})
	oldDial := dialVPP
	dialVPP = func(string, vpp.ConnOptions) vppConn { return fc }
	t.Cleanup(func() { dialVPP = oldDial })
	a, err := Start(context.Background(), testConfig(t), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Stop)
	fc.releaseAtEnd(t)
	sub := a.svc.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	defer a.svc.events().unsubscribe(sub)
	fc.states <- vpp.ConnState{Connected: true}
	collect(t, sub, 1) // collect fails after 5 s
}

// Review 1.1b: the drift check is a Plan — it reports (gauge + ERROR event) and changes nothing.
func TestDriftCheckIsPlanOnly(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	sub := s.events().subscribe(&vrxv1.StreamEventsRequest{Kinds: []vrxv1.EventKind{vrxv1.EventKind_EVENT_KIND_ERROR, vrxv1.EventKind_EVENT_KIND_RECONCILE_START}})
	defer s.events().unsubscribe(sub)
	checkDrift(s)
	if out := scrape(s.metrics); !strings.Contains(out, "vrx_agent_drift_objects 0\n") {
		t.Fatalf("no drift expected:\n%s", out)
	}
	v.DeleteInterface("loop702") // lost behind the agent's back
	before := v.Snapshot()
	checkDrift(s)
	evs := collect(t, sub, 1)
	n, _ := strconv.Atoi(evs[0].GetAttributes()["objects"])
	if evs[0].GetKind() != vrxv1.EventKind_EVENT_KIND_ERROR || evs[0].GetAttributes()["reason"] != "drift" || n == 0 {
		t.Fatalf("drift event %v", evs[0])
	}
	if out := scrape(s.metrics); !strings.Contains(out, "vrx_agent_drift_objects "+strconv.Itoa(n)+"\n") {
		t.Fatalf("gauge:\n%s", out)
	}
	if v.Snapshot() != before {
		t.Fatal("the drift check changed VPP")
	}
	checkDrift(s) // unchanged drift: no second event
	noEvent(t, sub, 100*time.Millisecond)
	s.Resync(context.Background())
	collect(t, sub, 1) // RECONCILE_START of the resync
	checkDrift(s)
	if out := scrape(s.metrics); !strings.Contains(out, "vrx_agent_drift_objects 0\n") {
		t.Fatalf("drift after the resync:\n%s", out)
	}
}

// Review 1.5c/1.5e: an unknown log level and an unauthenticated /metrics on a non-loopback address
// refuse to start.
func TestConfigRefusesUnknownLogLevelAndRemoteMetrics(t *testing.T) {
	base := Config{Owner: "w7", Socket: "/run/vrx-test/w7/agent.sock", StateDir: t.TempDir(), LogLevel: "info", MetricsAddr: "127.0.0.1:9171"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, lvl := range []string{"verbose", "information", "trace"} {
		c := base
		c.LogLevel = lvl
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "VRX_LOG_LEVEL") {
			t.Errorf("log level %q: %v", lvl, err)
		}
	}
	for _, lvl := range []string{"", "debug", "INFO", "warn", "error"} {
		c := base
		c.LogLevel = lvl
		if err := c.Validate(); err != nil {
			t.Errorf("log level %q: %v", lvl, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:9171", ":9171", "172.30.126.195:9171", "[::]:9171"} {
		c := base
		c.MetricsAddr = addr
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "VRX_METRICS_ALLOW_REMOTE") {
			t.Errorf("metrics %q: %v", addr, err)
		}
	}
	for _, addr := range []string{"127.0.0.1:9171", "127.0.0.2:9171", "[::1]:9171", "localhost:9171", "off", ""} {
		c := base
		c.MetricsAddr = addr
		if err := c.Validate(); err != nil {
			t.Errorf("metrics %q: %v", addr, err)
		}
	}
}
