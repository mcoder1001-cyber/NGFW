package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stageFixture registers the daemon-stage descriptor "k" FIRST — so the registration order alone
// would put its objects first — then the VPP-stage descriptors "a" (alias "if") and "b".
func stageFixture(t *testing.T) (*Scheduler, *store) {
	t.Helper()
	st := newStore()
	reg := NewRegistry()
	reg.Register(stagedMem{mem: &mem{name: "k", st: st}, stage: StageDaemon})
	reg.Register(&mem{name: "a", st: st, alias: "if"})
	reg.Register(&mem{name: "b", st: st})
	s := New(reg, nil)
	s.VerifyRetries = 0
	return s, st
}

func mustOps(t *testing.T, st *store, want ...string) {
	t.Helper()
	if got := strings.Join(st.ops(), ","); got != strings.Join(want, ",") {
		t.Fatalf("ops = %s\nwant %s", got, strings.Join(want, ","))
	}
}

// Independent objects: every VPP-stage create and update runs before the daemon-stage ones, and
// the deletes run the other way round — whatever the registration order says.
func TestStageDaemonAfterVPPWhenIndependent(t *testing.T) {
	s, st := stageFixture(t)
	ctx := context.Background()
	desired := []KV{kv("k", obj("x", "1")), kv("a", obj("y", "1")), kv("b", obj("z", "1", "a/y"))}
	mustApplied(t, s.Apply(ctx, desired, nil))
	mustOps(t, st, "create a/y", "create b/z", "create k/x")

	st.reset()
	mustApplied(t, s.Apply(ctx, []KV{kv("k", obj("x", "2")), kv("a", obj("y", "2")), kv("b", obj("z", "1", "a/y"))}, nil))
	mustOps(t, st, "update a/y", "update k/x")

	st.reset()
	mustApplied(t, s.Apply(ctx, nil, nil))
	mustOps(t, st, "delete k/x", "delete b/z", "delete a/y")
}

// A real dependency wins over the stage: a VPP-stage object that depends on a daemon-stage one is
// created after it (and deleted before it); an independent VPP-stage object still goes first.
func TestStageRealDependencyWins(t *testing.T) {
	s, st := stageFixture(t)
	ctx := context.Background()
	desired := []KV{kv("a", obj("y", "1", "k/x")), kv("k", obj("x", "1")), kv("b", obj("z", "1"))}
	mustApplied(t, s.Apply(ctx, desired, nil))
	mustOps(t, st, "create b/z", "create k/x", "create a/y")

	st.reset()
	mustApplied(t, s.Apply(ctx, nil, nil))
	mustOps(t, st, "delete a/y", "delete k/x", "delete b/z")
}

// The rollback stays the exact reverse of what ran, stages included.
func TestStageRollbackIsExactReverse(t *testing.T) {
	s, st := stageFixture(t)
	ctx := context.Background()
	st.failOn["create:k/x"] = errors.New("daemon refused")
	desired := []KV{kv("k", obj("x", "1")), kv("k", obj("w", "1")), kv("a", obj("y", "1")), kv("b", obj("z", "1"))}
	r := s.Apply(ctx, desired, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s, err %v", r.Outcome, r.Err)
	}
	applied := []string{"create a/y", "create b/z", "create k/w"}
	reverted := []string{"delete k/w", "delete b/z", "delete a/y"}
	mustOps(t, st, append(applied, reverted...)...)
	if len(st.objs) != 0 {
		t.Fatalf("left behind: %v", st.objs)
	}
}

// The plan (what DryRun reports) lists the operations in the same staged order.
func TestStagePlanOrder(t *testing.T) {
	s, _ := stageFixture(t)
	p, err := s.Plan(context.Background(), []KV{kv("k", obj("x", "1")), kv("a", obj("y", "1"))}, nil)
	if err != nil || len(p.Issues) > 0 {
		t.Fatalf("plan: %v %v", err, p.Issues)
	}
	var got []string
	for _, op := range p.Ops {
		got = append(got, op.Op+" "+string(op.Key))
	}
	if strings.Join(got, ",") != "create a/y,create k/x" {
		t.Fatalf("plan %v", got)
	}
	if StageOf(&mem{name: "a"}) != StageVPP || StageOf(stagedMem{mem: &mem{name: "k"}, stage: StageDaemon}) != StageDaemon {
		t.Fatal("StageOf")
	}
}
