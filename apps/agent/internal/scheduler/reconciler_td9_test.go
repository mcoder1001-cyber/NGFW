package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// TD-9: descriptor panics, bounded rollback and operations whose outcome is unknown.

// hooked is a mem descriptor with a hook that runs before every call of the named kind ("create",
// "update", "delete", "retrieve", "reapply", "normalize") on the named key ("" = every key). A hook may
// panic, block on ctx, or return an error that replaces the call's result (after the call when after
// is set: the change is made, then the error — a reply that never came).
type hooked struct {
	*mem
	hook func(ctx context.Context, call string, k Key) (err error, after bool)
}

func (h *hooked) run(ctx context.Context, call string, k Key, fn func() error) error {
	if h.hook == nil {
		return fn()
	}
	err, after := h.hook(ctx, call, k)
	if err == nil {
		return fn()
	}
	if after {
		_ = fn()
	}
	return err
}

func (h *hooked) Create(ctx context.Context, o proto.Message) (meta any, err error) {
	err = h.run(ctx, "create", h.KeyOf(o), func() (e error) { meta, e = h.mem.Create(ctx, o); return e })
	return meta, err
}
func (h *hooked) Update(ctx context.Context, oldO, newO proto.Message, m any) (meta any, err error) {
	err = h.run(ctx, "update", h.KeyOf(newO), func() (e error) { meta, e = h.mem.Update(ctx, oldO, newO, m); return e })
	return meta, err
}
func (h *hooked) Delete(ctx context.Context, o proto.Message, m any) error {
	return h.run(ctx, "delete", h.KeyOf(o), func() error { return h.mem.Delete(ctx, o, m) })
}
func (h *hooked) Retrieve(ctx context.Context) (kvs []KV, err error) {
	err = h.run(ctx, "retrieve", "", func() (e error) { kvs, e = h.mem.Retrieve(ctx); return e })
	return kvs, err
}
func (h *hooked) Normalize(o proto.Message) proto.Message {
	_ = h.run(context.Background(), "normalize", h.KeyOf(o), func() error { return nil })
	return o
}
func (h *hooked) Reapply(ctx context.Context, o proto.Message, _ any) error {
	return h.run(ctx, "reapply", h.KeyOf(o), func() error { return nil })
}

// hookFixture: "a" plain, "b" hooked (depends on a).
func hookFixture(t *testing.T, hook func(ctx context.Context, call string, k Key) (error, bool)) (*Scheduler, *store) {
	t.Helper()
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(&hooked{mem: &mem{name: "b", st: st}, hook: hook})
	s := New(reg, slogDiscard())
	s.VerifyRetries = 0
	return s, st
}

func slogDiscard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// returnsWithin runs f and fails the test when it does not return within d.
func returnsWithin(t *testing.T, d time.Duration, f func() *TxnResult) (*TxnResult, time.Duration) {
	t.Helper()
	done := make(chan *TxnResult, 1)
	start := time.Now()
	go func() { done <- f() }()
	select {
	case r := <-done:
		return r, time.Since(start)
	case <-time.After(d):
		t.Fatalf("ApplyWith did not return within %s", d)
		return nil, 0
	}
}

func resultOf(r *TxnResult, k Key) OpResult {
	for _, x := range r.Results {
		if x.Key == k {
			return x
		}
	}
	return OpResult{}
}

// Review 1.1e: a descriptor panic is recovered into errDescriptorPanic; the journal is undone (the
// agent does not crash with VPP half-applied), and since the panicking call's own effect is unknown
// the transaction is DEGRADED.
func TestDescriptorPanicRollsBack(t *testing.T) {
	// before: the state first applied; desired: the transaction whose b/y call panics; journaled: the
	// key applied before the panic, which the rollback must revert.
	cases := map[string]struct {
		before, desired []KV
		journaled       Key
	}{
		"create": {nil, []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, "a/x"},
		"update": {[]KV{kv("b", obj("y", "1"))}, []KV{kv("a", obj("x", "1")), kv("b", obj("y", "2", "a/x"))}, "a/x"},
		"delete": {[]KV{kv("b", obj("y", "1")), kv("a", obj("w", "1", "b/y"))}, nil, "a/w"},
	}
	for call, tc := range cases {
		t.Run(call, func(t *testing.T) {
			panicking := false
			s, st := hookFixture(t, func(_ context.Context, c string, k Key) (error, bool) {
				if panicking && c == call && k == "b/y" {
					panic("descriptor bug: " + call)
				}
				return nil, false
			})
			ctx := context.Background()
			mustApplied(t, s.Apply(ctx, tc.before, nil))
			snapshot := fmt.Sprint(st.objs)
			panicking = true
			var r *TxnResult
			func() {
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("ApplyWith panicked: %v", p)
					}
				}()
				r = s.Apply(ctx, tc.desired, nil)
			}()
			if !errors.Is(r.Err, errDescriptorPanic) || r.Outcome != OutcomeDegraded || !uncertainOf(r) {
				t.Fatalf("outcome %s uncertain %v err %v", r.Outcome, uncertainOf(r), r.Err)
			}
			if strings.Contains(r.Err.Error(), "descriptor bug") {
				t.Fatalf("the panic value reached the error (log only): %v", r.Err)
			}
			if b := resultOf(r, "b/y"); b.Code != CodeFailed || !errors.Is(b.Err, errDescriptorPanic) {
				t.Fatalf("b/y result %+v", b)
			}
			if j := resultOf(r, tc.journaled); j.Code != CodeReverted {
				t.Fatalf("%s not rolled back: %+v (ops %v)", tc.journaled, j, st.ops())
			}
			if got := fmt.Sprint(st.objs); got != snapshot {
				t.Fatalf("store after the rollback %s, want %s", got, snapshot)
			}
			panicking = false
			mustApplied(t, s.Apply(ctx, tc.desired, nil)) // the scheduler's lock was released
		})
	}
}

// A panic in Retrieve, Normalize (plan phase) fails the transaction with nothing touched; in Plan and
// Retrieve it is an error; in Reapply it is a reapply error; in an undo it is a failed revert.
func TestDescriptorPanicOutsideOperations(t *testing.T) {
	var where string
	s, st := hookFixture(t, func(_ context.Context, c string, _ Key) (error, bool) {
		if c == where {
			panic("descriptor bug in " + c)
		}
		return nil, false
	})
	ctx := context.Background()
	for _, w := range []string{"retrieve", "normalize"} {
		where = w
		r := s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1"))}, nil)
		if r.Outcome != OutcomeFailed || !errors.Is(r.Err, errDescriptorPanic) || len(st.ops()) != 0 {
			t.Fatalf("%s: outcome %s err %v ops %v", w, r.Outcome, r.Err, st.ops())
		}
		if _, err := s.Plan(ctx, []KV{kv("b", obj("y", "1"))}, nil); !errors.Is(err, errDescriptorPanic) {
			t.Fatalf("%s: Plan err %v", w, err)
		}
	}
	where = "retrieve"
	if _, err := s.Retrieve(ctx, nil); !errors.Is(err, errDescriptorPanic) {
		t.Fatalf("Retrieve err %v", err)
	}
	where = "reapply"
	mustApplied(t, s.Apply(ctx, []KV{kv("b", obj("y", "1"))}, nil))
	r := s.ApplyWith(ctx, []KV{kv("b", obj("y", "1"))}, nil, ApplyOptions{Resync: true})
	if r.Outcome != OutcomeApplied || r.ReapplyErrors != 1 {
		t.Fatalf("reapply panic: outcome %s reapply errors %d", r.Outcome, r.ReapplyErrors)
	}
	// undo: b/y is created, a/z fails, the undo (delete b/y) panics → revert failed.
	where = ""
	s2, st2 := hookFixture(t, func(_ context.Context, c string, _ Key) (error, bool) {
		if c == "delete" {
			panic("undo bug")
		}
		return nil, false
	})
	st2.failOn["create:a/z"] = errors.New("vpp says no")
	r = s2.Apply(ctx, []KV{kv("b", obj("y", "1")), kv("a", obj("z", "1", "b/y"))}, nil)
	if r.Outcome != OutcomeDegraded || resultOf(r, "b/y").Code != CodeRevertFailed || !errors.Is(resultOf(r, "b/y").Err, errDescriptorPanic) {
		t.Fatalf("undo panic: outcome %s results %+v", r.Outcome, r.Results)
	}
}

// Review 1.1: the rollback has a deadline of its own. A delete that never returns (VPP stopped
// replying) no longer parks the scheduler: ApplyWith returns once RollbackTimeout ran out, DEGRADED.
func TestRollbackIsBounded(t *testing.T) {
	// The caller's ctx is cancelled right after b/y was created: a/z then fails before it starts, and
	// the rollback must still run (WithoutCancel) and still end (its own deadline) although deleting b/y
	// never returns.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, st := hookFixture(t, func(hctx context.Context, c string, k Key) (error, bool) {
		switch {
		case c == "create" && k == "b/y":
			cancel()
		case c == "delete" && k == "b/y":
			<-hctx.Done() // never replies: only the ctx ends the wait
			return hctx.Err(), false
		}
		return nil, false
	})
	const rb = 300 * time.Millisecond
	withRollbackTimeout(s, rb)
	r, took := returnsWithin(t, 10*time.Second, func() *TxnResult {
		return s.Apply(ctx, []KV{kv("b", obj("y", "1")), kv("a", obj("z", "1", "b/y"))}, nil)
	})
	if r.Outcome != OutcomeDegraded || resultOf(r, "b/y").Code != CodeRevertFailed || !errors.Is(r.Err, context.Canceled) {
		t.Fatalf("outcome %s err %v results %+v", r.Outcome, r.Err, r.Results)
	}
	if took < rb || took > rb+3*time.Second {
		t.Fatalf("returned after %s, RollbackTimeout %s", took, rb)
	}
	if _, ok := st.objs["b/y"]; !ok {
		t.Fatal("b/y should still exist (its delete never completed)")
	}
	// The write lock was released: the next transaction runs.
	r, _ = returnsWithin(t, 10*time.Second, func() *TxnResult {
		return s.Apply(context.Background(), []KV{kv("a", obj("z", "1"))}, Only("a"))
	})
	mustApplied(t, r)
}

// Review 1.1/1.4: an operation cut off by a deadline (or a VPP reply timeout, which wraps
// context.DeadlineExceeded, also when a descriptor formats it with %v) may have taken effect: the
// journal is undone, but the outcome is DEGRADED with Uncertain set, never ROLLED_BACK.
func TestTimedOutOperationIsUncertain(t *testing.T) {
	cases := map[string]error{
		"deadline":     fmt.Errorf("create_loopback: %w", context.DeadlineExceeded),
		"reply %v":     fmt.Errorf("sw_interface_add_del_address: %v", fmt.Errorf("vpp: no reply in time: %w", context.DeadlineExceeded)),
		"cancelled":    fmt.Errorf("ip_route_add_del: %w", context.Canceled),
		"plain (sure)": errors.New("VPPApiError: invalid value (-1)"),
	}
	for name, cause := range cases {
		t.Run(name, func(t *testing.T) {
			s, st := hookFixture(t, func(_ context.Context, c string, k Key) (error, bool) {
				if c == "create" && k == "b/y" {
					return cause, true // applied, then the reply was lost
				}
				return nil, false
			})
			r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, nil)
			if resultOf(r, "a/x").Code != CodeReverted {
				t.Fatalf("journal not undone: %+v", r.Results)
			}
			sure := name == "plain (sure)"
			if sure {
				if r.Outcome != OutcomeRolledBack || uncertainOf(r) {
					t.Fatalf("a certain failure: outcome %s uncertain %v", r.Outcome, uncertainOf(r))
				}
				return
			}
			if r.Outcome != OutcomeDegraded || !uncertainOf(r) {
				t.Fatalf("outcome %s uncertain %v, want DEGRADED (b/y may exist: %v)", r.Outcome, uncertainOf(r), st.objs["b/y"] != nil)
			}
		})
	}
	// A transaction whose deadline passed BETWEEN operations sent nothing it cannot account for.
	s, _ := hookFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	s.reg.Register(&hooked{mem: &mem{name: "c", st: newStore()}, hook: func(_ context.Context, c string, _ Key) (error, bool) {
		if c == "create" {
			cancel() // the next operation sees a done ctx before it starts
		}
		return nil, false
	}})
	r := s.Apply(ctx, []KV{kv("c", obj("w", "1")), kv("a", obj("x", "1", "c/w"))}, nil)
	if r.Outcome != OutcomeRolledBack || uncertainOf(r) {
		t.Fatalf("deadline between operations: outcome %s uncertain %v err %v", r.Outcome, uncertainOf(r), r.Err)
	}
}
