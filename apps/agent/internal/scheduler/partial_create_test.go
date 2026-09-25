package scheduler

// TD-11b (review 3.3 / 3.3b generic half): a Create that fails AFTER it changed VPP returns the
// Meta of what it made together with the error; the reconciler journals it, so the rollback
// deletes the partial object instead of leaving it in VPP, unjournaled and (for untagged objects)
// invisible to Retrieve.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

// partial is a mem descriptor whose Create can fail after the object was written (the claim-after-
// write or event-subscription-after-add shape: wireguard/peer.go, DF-1 before TD-11b).
type partial struct {
	mem
	failAfter map[Key]error // key → error returned together with the Meta of the created object
	plain     error         // Create fails before writing, but returns a (zero) Meta with it
}

func (p *partial) Create(ctx context.Context, o proto.Message) (any, error) {
	if p.plain != nil {
		return memMeta{}, p.plain
	}
	meta, err := p.mem.Create(ctx, o)
	if err != nil {
		return nil, err
	}
	if ferr, ok := p.failAfter[p.KeyOf(o)]; ok {
		delete(p.failAfter, p.KeyOf(o))  // one shot: the rollback's re-create of the old value succeeds
		return meta, PartialCreate(ferr) // VPP was written: the Meta says what to delete
	}
	return meta, nil
}

func partialFixture(t *testing.T) (*Scheduler, *store, *partial) {
	t.Helper()
	st := newStore()
	p := &partial{mem: mem{name: "p", st: st}, failAfter: map[Key]error{}}
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(p)
	s := New(reg, nil)
	s.VerifyRetries = 0
	return s, st, p
}

func TestPartialCreateIsRolledBack(t *testing.T) {
	s, st, p := partialFixture(t)
	p.failAfter["p/x"] = errors.New("claim store: flush failed")
	r := s.Apply(context.Background(), []KV{kv("a", obj("base", "1")), kv("p", obj("x", "1", "a/base"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v results %+v", r.Outcome, r.Err, r.Results)
	}
	// the partial object and the object before it are both undone, in reverse order
	want := "create a/base,create p/x,delete p/x,delete a/base"
	if got := strings.Join(st.ops(), ","); got != want {
		t.Fatalf("ops = %s\nwant %s", got, want)
	}
	if len(st.objs) != 0 {
		t.Fatalf("left in VPP after rollback: %v", st.objs)
	}
	codes := map[Key]ResultCode{}
	for _, res := range r.Results {
		codes[res.Key] = res.Code
	}
	if codes["p/x"] != CodeFailed || codes["a/base"] != CodeReverted {
		t.Fatalf("codes %+v", codes)
	}
	if !strings.Contains(r.Err.Error(), "flush failed") {
		t.Fatalf("err %v", r.Err)
	}
}

// A partial Create whose rollback Delete fails leaves the agent DEGRADED with the failure on the
// operation's own result (never a silent ROLLED_BACK while the object is still in VPP).
func TestPartialCreateRollbackFailureDegrades(t *testing.T) {
	s, st, p := partialFixture(t)
	p.failAfter["p/x"] = errors.New("event subscription failed")
	st.failOn["delete:p/x"] = errors.New("stuck")
	r := s.Apply(context.Background(), []KV{kv("p", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeDegraded {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	if len(r.Results) != 1 || r.Results[0].Key != "p/x" || r.Results[0].Code != CodeRevertFailed {
		t.Fatalf("results %+v", r.Results)
	}
}

// A Create that fails with a Meta but WITHOUT PartialCreate changed nothing: many descriptors return
// a zero Meta value with a failed add (core interface-ip after "address in use"), and deleting it in
// the rollback would fail (DEGRADED) or remove an object that existed before.
func TestFailedCreateWithMetaButNotPartialIsNotJournaled(t *testing.T) {
	s, st, p := partialFixture(t)
	p.plain = errors.New("VPPApiError: Address in use (-105)")
	r := s.Apply(context.Background(), []KV{kv("p", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	if got := st.ops(); len(got) != 0 {
		t.Fatalf("ops %v (the rollback must not delete what the failed Create did not make)", got)
	}
	if !errors.Is(PartialCreate(p.plain), ErrPartialCreate) || PartialCreate(p.plain).Error() != p.plain.Error() || PartialCreate(nil) != nil {
		t.Fatal("PartialCreate wrapping")
	}
}

// A Create that fails before changing anything returns nil Meta: nothing is journaled, nothing is
// deleted (the existing behaviour, unchanged).
func TestFailedCreateWithoutMetaIsNotJournaled(t *testing.T) {
	s, st := fixture(t)
	st.failOn["create:a/x"] = errors.New("boom")
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s", r.Outcome)
	}
	if got := st.ops(); len(got) != 0 {
		t.Fatalf("ops %v", got)
	}
}

// A partial Create inside a recreate (ErrRecreate → Delete old + Create new) is undone first, then
// the old object is restored.
func TestPartialCreateInRecreateRestoresOld(t *testing.T) {
	s, st, p := partialFixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("p", obj("x", "imm:1"))}, nil))
	st.reset()
	p.failAfter["p/x"] = errors.New("claim store: no identity")
	r := s.Apply(ctx, []KV{kv("p", obj("x", "imm:2"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	want := "delete p/x,create p/x,delete p/x,create p/x"
	if got := strings.Join(st.ops(), ","); got != want {
		t.Fatalf("ops = %s\nwant %s", got, want)
	}
	if v := str(st.objs["p/x"], "val"); v != "imm:1" {
		t.Fatalf("after rollback p/x = %q, want the old imm:1", v)
	}
}

// keyed is a partial descriptor whose objects are addressed by key alone (df6 keyed, natcommon ops
// with nil Meta): Create returns nil Meta, Delete ignores it.
type keyed struct{ partial }

func (k *keyed) Create(ctx context.Context, o proto.Message) (any, error) {
	if _, err := k.partial.Create(ctx, o); err != nil {
		return nil, err // nil Meta, whatever the error — PartialCreate included
	}
	return nil, nil
}

func (k *keyed) Delete(ctx context.Context, o proto.Message, _ any) error {
	return k.mem.Delete(ctx, o, memMeta{})
}

// Review M2: a PartialCreate with a nil Meta is journaled on the marker alone and deleted by the
// rollback (before fix round 1 it was dropped: object left in VPP, unjournaled).
func TestPartialCreateWithNilMetaIsRolledBack(t *testing.T) {
	st := newStore()
	k := &keyed{partial{mem: mem{name: "k", st: st}, failAfter: map[Key]error{"k/x": errors.New("claim record failed")}}}
	reg := NewRegistry()
	reg.Register(k)
	s := New(reg, nil)
	s.VerifyRetries = 0
	r := s.Apply(context.Background(), []KV{kv("k", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v results %+v", r.Outcome, r.Err, r.Results)
	}
	if got := strings.Join(st.ops(), ","); got != "create k/x,delete k/x" {
		t.Fatalf("ops = %s, want the partial object deleted by the rollback", got)
	}
	if len(st.objs) != 0 {
		t.Fatalf("left in VPP: %v", st.objs)
	}
}

// A PartialCreate with a nil Meta whose Delete needs the Meta: the rollback fails loudly (DEGRADED),
// never a silent ROLLED_BACK with the object still in VPP.
func TestPartialCreateNilMetaDeleteNeedsMetaDegrades(t *testing.T) {
	st := newStore()
	p := &nilMetaPartial{partial{mem: mem{name: "p", st: st}, failAfter: map[Key]error{"p/x": errors.New("subscription failed")}}}
	reg := NewRegistry()
	reg.Register(p)
	s := New(reg, nil)
	s.VerifyRetries = 0
	r := s.Apply(context.Background(), []KV{kv("p", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeDegraded || r.Results[0].Code != CodeRevertFailed {
		t.Fatalf("outcome %s results %+v", r.Outcome, r.Results)
	}
}

// nilMetaPartial returns nil Meta with its PartialCreate error; its Delete needs a memMeta.
type nilMetaPartial struct{ partial }

func (p *nilMetaPartial) Create(ctx context.Context, o proto.Message) (any, error) {
	meta, err := p.partial.Create(ctx, o)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func TestIsPartialCreate(t *testing.T) {
	base := errors.New("boom")
	if !IsPartialCreate(PartialCreate(base)) || !IsPartialCreate(fmt.Errorf("ctx: %w", PartialCreate(base))) {
		t.Fatal("marked errors")
	}
	if IsPartialCreate(base) || IsPartialCreate(nil) || !errors.Is(PartialCreate(base), base) {
		t.Fatal("unmarked errors / unwrap")
	}
}
