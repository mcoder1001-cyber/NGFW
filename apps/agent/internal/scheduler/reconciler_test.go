package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---- an in-memory "VPP" and a configurable descriptor --------------------------------------

// store is the backend shared by all mem descriptors; it records every mutation in order.
type store struct {
	mu      sync.Mutex
	objs    map[Key]*structpb.Struct
	log     []string
	failOn  map[string]error // "create:a/x" → error
	metaSeq int
}

func newStore() *store {
	return &store{objs: map[Key]*structpb.Struct{}, failOn: map[string]error{}}
}

func (s *store) fail(op string, k Key) error {
	if err, ok := s.failOn[op+":"+string(k)]; ok {
		return err
	}
	return nil
}

func (s *store) record(op string, k Key) {
	s.log = append(s.log, op+" "+string(k))
}

func (s *store) ops() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.log...)
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = nil
}

// obj builds a value: name (id), val (mutable), imm (immutable → ErrRecreate), deps.
func obj(name, val string, deps ...string) *structpb.Struct {
	d := make([]any, len(deps))
	for i, x := range deps {
		d[i] = x
	}
	s, err := structpb.NewStruct(map[string]any{"name": name, "val": val, "deps": d})
	if err != nil {
		panic(err)
	}
	return s
}

func str(o proto.Message, f string) string {
	return o.(*structpb.Struct).GetFields()[f].GetStringValue()
}

type mem struct {
	name  string
	st    *store
	alias string // if set, ProvidedKeys returns Join(alias, name)
}

type memMeta struct{ Handle int }

func (m *mem) Name() string              { return m.name }
func (m *mem) KeyOf(o proto.Message) Key { return Join(m.name, str(o, "name")) }
func (m *mem) ProvidedKeys(o proto.Message) []Key {
	if m.alias == "" {
		return nil
	}
	return []Key{Join(m.alias, str(o, "name"))}
}
func (m *mem) Dependencies(o proto.Message) []Dependency {
	var out []Dependency
	for _, v := range o.(*structpb.Struct).GetFields()["deps"].GetListValue().GetValues() {
		k := v.GetStringValue()
		opt := strings.HasPrefix(k, "?")
		out = append(out, Dependency{Key: Key(strings.TrimPrefix(k, "?")), Optional: opt})
	}
	return out
}
func (m *mem) Create(_ context.Context, o proto.Message) (any, error) {
	m.st.mu.Lock()
	defer m.st.mu.Unlock()
	k := m.KeyOf(o)
	if err := m.st.fail("create", k); err != nil {
		return nil, err
	}
	if _, ok := m.st.objs[k]; ok {
		return nil, fmt.Errorf("%s already exists", k)
	}
	m.st.objs[k] = proto.Clone(o).(*structpb.Struct)
	m.st.metaSeq++
	m.st.record("create", k)
	return memMeta{Handle: m.st.metaSeq}, nil
}
func (m *mem) Update(_ context.Context, oldO, newO proto.Message, meta any) (any, error) {
	m.st.mu.Lock()
	defer m.st.mu.Unlock()
	k := m.KeyOf(newO)
	if strings.HasPrefix(str(newO, "val"), "imm:") || strings.HasPrefix(str(oldO, "val"), "imm:") {
		return nil, ErrRecreate
	}
	if err := m.st.fail("update", k); err != nil {
		return nil, err
	}
	if _, ok := meta.(memMeta); !ok {
		return nil, fmt.Errorf("bad meta %T", meta)
	}
	m.st.objs[k] = proto.Clone(newO).(*structpb.Struct)
	m.st.record("update", k)
	return meta, nil
}
func (m *mem) Delete(_ context.Context, o proto.Message, meta any) error {
	m.st.mu.Lock()
	defer m.st.mu.Unlock()
	k := m.KeyOf(o)
	if err := m.st.fail("delete", k); err != nil {
		return err
	}
	if _, ok := meta.(memMeta); !ok {
		return fmt.Errorf("bad meta %T for %s", meta, k)
	}
	if _, ok := m.st.objs[k]; !ok {
		return fmt.Errorf("%s does not exist", k)
	}
	delete(m.st.objs, k)
	m.st.record("delete", k)
	return nil
}
func (m *mem) Retrieve(context.Context) ([]KV, error) {
	m.st.mu.Lock()
	defer m.st.mu.Unlock()
	if err := m.st.fail("retrieve", Key(m.name)); err != nil {
		return nil, err
	}
	var out []KV
	for k, v := range m.st.objs {
		if k.Descriptor() == m.name {
			out = append(out, KV{Key: k, Value: proto.Clone(v), Meta: memMeta{}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// fixture: descriptors "a" (base, alias "if"), "b" (depends on a), "c" (depends on b).
func fixture(t *testing.T) (*Scheduler, *store) {
	t.Helper()
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st, alias: "if"})
	reg.Register(&mem{name: "b", st: st})
	reg.Register(&mem{name: "c", st: st})
	s := New(reg, nil)
	s.VerifyRetries = 0
	return s, st
}

func kv(desc string, o *structpb.Struct) KV { return KV{Key: Join(desc, str(o, "name")), Value: o} }

func mustApplied(t *testing.T, r *TxnResult) {
	t.Helper()
	if r.Outcome != OutcomeApplied {
		t.Fatalf("outcome %s, err %v, results %+v", r.Outcome, r.Err, r.Results)
	}
}

// ---- tests ---------------------------------------------------------------------------------

func TestApplyOrdersByDependencies(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	// Given in reverse order on purpose; c → b → a, and b depends on a via the alias "if/x".
	desired := []KV{
		kv("c", obj("z", "1", "b/y")),
		kv("b", obj("y", "1", "if/x")),
		kv("a", obj("x", "1")),
	}
	r := s.Apply(ctx, desired, nil)
	mustApplied(t, r)
	want := []string{"create a/x", "create b/y", "create c/z"}
	if got := st.ops(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if r.Summary.Created != 3 || r.Summary.Unchanged != 0 {
		t.Fatalf("summary %+v", r.Summary)
	}

	// Removing everything deletes in reverse order (dependents first).
	st.reset()
	r = s.Apply(ctx, nil, nil)
	mustApplied(t, r)
	want = []string{"delete c/z", "delete b/y", "delete a/x"}
	if got := st.ops(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ops = %v, want %v", got, want)
	}
}

func TestApplyIdempotent(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	desired := []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}
	mustApplied(t, s.Apply(ctx, desired, nil))
	st.reset()
	r := s.Apply(ctx, desired, nil)
	mustApplied(t, r)
	if !r.Plan.Empty() || len(r.Results) != 0 || r.Summary.Unchanged != 2 || len(st.ops()) != 0 {
		t.Fatalf("second apply not empty: plan %+v results %+v ops %v", r.Plan.Ops, r.Results, st.ops())
	}
}

func TestUpdateInPlace(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("x", "1"))}, nil))
	st.reset()
	r := s.Apply(ctx, []KV{kv("a", obj("x", "2"))}, nil)
	mustApplied(t, r)
	if got := st.ops(); len(got) != 1 || got[0] != "update a/x" || r.Summary.Updated != 1 {
		t.Fatalf("ops %v summary %+v", got, r.Summary)
	}
}

func TestRecreateCascadesToDependents(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{
		kv("a", obj("x", "imm:1")),
		kv("b", obj("y", "1", "if/x")),
		kv("c", obj("z", "1", "b/y")),
	}, nil))
	st.reset()
	r := s.Apply(ctx, []KV{
		kv("a", obj("x", "imm:2")),
		kv("b", obj("y", "1", "if/x")),
		kv("c", obj("z", "1", "b/y")),
	}, nil)
	mustApplied(t, r)
	want := "delete c/z,delete b/y,delete a/x,create a/x,create b/y,create c/z"
	if got := strings.Join(st.ops(), ","); got != want {
		t.Fatalf("ops = %s\nwant %s", got, want)
	}
	if r.Summary.Updated != 3 {
		t.Fatalf("summary %+v", r.Summary)
	}
	for _, res := range r.Results {
		if res.Op != OpRecreate || res.Code != CodeOK {
			t.Fatalf("result %+v", res)
		}
	}
}

func TestRollbackOnFailure(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	// Existing: a/old (will be deleted), a/x value 1 (will be updated).
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("old", "1")), kv("a", obj("x", "1"))}, nil))
	st.reset()
	st.failOn["create:c/z"] = errors.New("boom")
	r := s.Apply(ctx, []KV{
		kv("a", obj("x", "2")),
		kv("b", obj("y", "1", "a/x")),
		kv("c", obj("z", "1", "b/y")),
	}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	want := "delete a/old,update a/x,create b/y,delete b/y,update a/x,create a/old"
	if got := strings.Join(st.ops(), ","); got != want {
		t.Fatalf("ops = %s\nwant %s", got, want)
	}
	codes := map[Key]ResultCode{}
	for _, res := range r.Results {
		codes[res.Key] = res.Code
	}
	if codes["c/z"] != CodeFailed || codes["b/y"] != CodeReverted || codes["a/x"] != CodeReverted || codes["a/old"] != CodeReverted {
		t.Fatalf("codes %+v", codes)
	}
	if r.Summary.Reverted != 3 || r.Summary.Failed != 1 {
		t.Fatalf("summary %+v", r.Summary)
	}
	// State is back to the previous one.
	if len(st.objs) != 2 || str(st.objs["a/x"], "val") != "1" || st.objs["a/old"] == nil {
		t.Fatalf("state after rollback: %v", st.objs)
	}
}

func TestSkippedAfterFailure(t *testing.T) {
	s, st := fixture(t)
	st.failOn["create:a/x"] = errors.New("boom")
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s", r.Outcome)
	}
	if len(r.Results) != 2 || r.Results[0].Code != CodeFailed || r.Results[1].Code != CodeSkipped {
		t.Fatalf("results %+v", r.Results)
	}
}

func TestDegradedWhenRollbackFails(t *testing.T) {
	s, st := fixture(t)
	st.failOn["create:b/y"] = errors.New("boom")
	st.failOn["delete:a/x"] = errors.New("stuck")
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, nil)
	if r.Outcome != OutcomeDegraded {
		t.Fatalf("outcome %s", r.Outcome)
	}
	if r.Results[0].Key != "a/x" || r.Results[0].Code != CodeRevertFailed {
		t.Fatalf("results %+v", r.Results)
	}
}

func TestDependencyMissingFailsBeforeApply(t *testing.T) {
	s, st := fixture(t)
	r := s.Apply(context.Background(), []KV{
		kv("a", obj("x", "1")),
		kv("b", obj("y", "1", "a/nope", "?a/optional-missing")),
	}, nil)
	if r.Outcome != OutcomeFailed || len(st.ops()) != 0 {
		t.Fatalf("outcome %s ops %v", r.Outcome, st.ops())
	}
	if len(r.Plan.Issues) != 1 || r.Plan.Issues[0].Key != "b/y" || r.Plan.Issues[0].Code != CodeDependencyMissing {
		t.Fatalf("issues %+v", r.Plan.Issues)
	}
}

func TestDependencySatisfiedByActualState(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("x", "1"))}, nil))
	// Only descriptor b is in scope; a/x exists and stays (out of scope).
	r := s.Apply(ctx, []KV{kv("b", obj("y", "1", "a/x"))}, Only("b"))
	mustApplied(t, r)
	if r.Summary.Created != 1 || r.Summary.Deleted != 0 {
		t.Fatalf("summary %+v", r.Summary)
	}
}

func TestScopeNeverTouchesOtherDescriptors(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("c", obj("z", "1"))}, nil))
	st.reset()
	// Authoritative-empty for "c" only: c/z is deleted, a/x untouched.
	r := s.Apply(ctx, nil, Only("c"))
	mustApplied(t, r)
	if got := st.ops(); len(got) != 1 || got[0] != "delete c/z" {
		t.Fatalf("ops %v", got)
	}
	// Desired object outside the scope is a validation error.
	r = s.Apply(ctx, []KV{kv("a", obj("q", "1"))}, Only("c"))
	if r.Outcome != OutcomeFailed {
		t.Fatalf("outcome %s", r.Outcome)
	}
}

func TestCannotDeleteWhatOutOfScopeObjectsNeed(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, nil))
	st.reset()
	r := s.Apply(ctx, nil, Only("a"))
	if r.Outcome != OutcomeFailed || len(st.ops()) != 0 {
		t.Fatalf("outcome %s ops %v", r.Outcome, st.ops())
	}
}

func TestCycleIsInvalid(t *testing.T) {
	s, _ := fixture(t)
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1", "b/y")), kv("b", obj("y", "1", "a/x"))}, nil)
	if r.Outcome != OutcomeFailed || len(r.Plan.Issues) != 2 {
		t.Fatalf("outcome %s issues %+v", r.Outcome, r.Plan.Issues)
	}
}

func TestInvalidDesired(t *testing.T) {
	s, _ := fixture(t)
	r := s.Apply(context.Background(), []KV{
		{Key: "nope/x", Value: obj("x", "1")},
		{Key: "a/wrong", Value: obj("x", "1")},
		kv("a", obj("d", "1")), kv("a", obj("d", "2")),
	}, nil)
	if r.Outcome != OutcomeFailed || len(r.Plan.Issues) != 3 {
		t.Fatalf("outcome %s issues %+v", r.Outcome, r.Plan.Issues)
	}
}

// verifying descriptor: Create "succeeds" but the object never shows up in Retrieve.
type lying struct{ mem }

func (l *lying) Create(context.Context, proto.Message) (any, error) { return memMeta{}, nil }
func (l *lying) Delete(context.Context, proto.Message, any) error   { return nil }

func TestVerifyMismatchRollsBack(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(&lying{mem{name: "l", st: st}})
	s := New(reg, nil)
	s.VerifyRetries = 1
	s.VerifyDelay = 0
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1")), kv("l", obj("q", "1"))}, nil)
	if r.Outcome != OutcomeRolledBack || !strings.Contains(r.Err.Error(), "l/q missing") {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	if len(st.objs) != 0 {
		t.Fatalf("a/x not rolled back: %v", st.objs)
	}
}

func TestRetrieveErrorFailsPlan(t *testing.T) {
	s, st := fixture(t)
	st.failOn["retrieve:a"] = errors.New("vpp gone")
	r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1"))}, nil)
	if r.Outcome != OutcomeFailed || r.Err == nil {
		t.Fatalf("outcome %s err %v", r.Outcome, r.Err)
	}
	// Out-of-scope descriptor failing Retrieve does not block a transaction on another one.
	r = s.Apply(context.Background(), []KV{kv("c", obj("z", "1"))}, Only("c"))
	mustApplied(t, r)
}

func TestRetrieveSorted(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("b", obj("y", "1")), kv("a", obj("x", "1"))}, nil))
	kvs, err := s.Retrieve(ctx, nil)
	if err != nil || len(kvs) != 2 || kvs[0].Key != "a/x" || kvs[1].Key != "b/y" {
		t.Fatalf("retrieve %v %+v", err, kvs)
	}
	kvs, _ = s.Retrieve(ctx, Only("b"))
	if len(kvs) != 1 {
		t.Fatalf("scoped retrieve %+v", kvs)
	}
}

func TestPlanDoesNotMutate(t *testing.T) {
	s, st := fixture(t)
	p, err := s.Plan(context.Background(), []KV{kv("a", obj("x", "1"))}, nil)
	if err != nil || len(p.Ops) != 1 || p.Ops[0].Op != OpCreate || len(st.ops()) != 0 {
		t.Fatalf("plan %+v err %v ops %v", p, err, st.ops())
	}
	if sm := p.Summary(); sm.Created != 1 {
		t.Fatalf("summary %+v", sm)
	}
}

// wo is a write-only descriptor (D-063): Retrieve has no dump; Create is idempotent.
type wo struct{ mem }

func (w *wo) Retrieve(context.Context) ([]KV, error) {
	return nil, fmt.Errorf("%s: %w", w.name, ErrRetrieveUnsupported)
}
func (w *wo) Create(_ context.Context, o proto.Message) (any, error) {
	w.st.mu.Lock()
	defer w.st.mu.Unlock()
	k := w.KeyOf(o)
	if err := w.st.fail("create", k); err != nil {
		return nil, err
	}
	w.st.objs[k] = proto.Clone(o).(*structpb.Struct) // idempotent overwrite
	w.st.record("create", k)
	return memMeta{Handle: 1}, nil
}

// woForeign wraps the sentinel of another package (df2.ErrRetrieveUnsupported has the same text).
type woForeign struct{ mem }

func (w *woForeign) Retrieve(context.Context) ([]KV, error) {
	return nil, fmt.Errorf("x: %w", errors.New("vpp has no dump for this object type"))
}

func TestWriteOnlyDescriptor(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(&wo{mem{name: "w", st: st}})
	s := New(reg, nil)
	ctx := context.Background()
	desired := []KV{kv("a", obj("x", "1")), kv("w", obj("q", "1", "a/x"))}
	r := s.Apply(ctx, desired, nil)
	mustApplied(t, r) // verification skipped for w
	if got := strings.Join(st.ops(), ","); got != "create a/x,create w/q" {
		t.Fatalf("ops %s", got)
	}
	if d, n := s.WriteOnly(); len(d) != 1 || d[0] != "w" || n != 1 {
		t.Fatalf("write-only %v %d", d, n)
	}
	// Same desired again: nothing (the cached value equals desired).
	st.reset()
	r = s.Apply(ctx, desired, nil)
	mustApplied(t, r)
	if !r.Plan.Empty() || len(st.ops()) != 0 {
		t.Fatalf("second apply %+v %v", r.Plan.Ops, st.ops())
	}
	// Resync re-applies it (VPP may have lost it).
	r = s.ApplyWith(ctx, desired, nil, ApplyOptions{Resync: true})
	mustApplied(t, r)
	if got := strings.Join(st.ops(), ","); got != "create w/q" {
		t.Fatalf("resync ops %s", got)
	}
	// Changed value → update with the cached old value.
	st.reset()
	desired[1] = kv("w", obj("q", "2", "a/x"))
	mustApplied(t, s.Apply(ctx, desired, nil))
	if got := strings.Join(st.ops(), ","); got != "update w/q" {
		t.Fatalf("update ops %s", got)
	}
	// A fresh scheduler (agent restart) never deletes what it cannot see.
	s2 := New(reg, nil)
	st.reset()
	r = s2.Apply(ctx, []KV{kv("a", obj("x", "1"))}, nil)
	mustApplied(t, r)
	if len(st.ops()) != 0 {
		t.Fatalf("restart deleted unseen write-only object: %v", st.ops())
	}
	// Removed from desired after this process applied it → deleted.
	r = s.Apply(ctx, []KV{kv("a", obj("x", "1"))}, nil)
	mustApplied(t, r)
	if got := strings.Join(st.ops(), ","); got != "delete w/q" {
		t.Fatalf("delete ops %s", got)
	}
	if _, n := s.WriteOnly(); n != 0 {
		t.Fatalf("cache not emptied: %d", n)
	}
	// Rollback restores the cache too.
	st.failOn["create:a/y"] = errors.New("boom")
	r = s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("w", obj("q", "1")), kv("a", obj("y", "1", "w/q"))}, nil)
	if r.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome %s", r.Outcome)
	}
	if _, n := s.WriteOnly(); n != 0 {
		t.Fatalf("cache after rollback: %d", n)
	}
	// The foreign sentinel (same message) is recognised as well.
	reg2 := NewRegistry()
	reg2.Register(&woForeign{mem{name: "f", st: newStore()}})
	mustApplied(t, New(reg2, nil).Apply(ctx, []KV{kv("f", obj("z", "1"))}, nil))
}

// norm canonicalises val to lower case.
type norm struct{ mem }

func (n *norm) Normalize(o proto.Message) proto.Message {
	c := proto.Clone(o).(*structpb.Struct)
	c.Fields["val"] = structpb.NewStringValue(strings.ToLower(str(o, "val")))
	return c
}

func TestNormalizerAppliedBeforeDiff(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	reg.Register(&norm{mem{name: "n", st: st}})
	s := New(reg, nil)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("n", obj("x", "ABC"))}, nil))
	if str(st.objs["n/x"], "val") != "abc" {
		t.Fatalf("stored %v", st.objs["n/x"])
	}
	r := s.Apply(ctx, []KV{kv("n", obj("x", "AbC"))}, nil)
	mustApplied(t, r)
	if !r.Plan.Empty() {
		t.Fatalf("normalised value planned an update: %+v", r.Plan.Ops)
	}
}

func TestCreateRecreatesLiveOptionalDependents(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	// b/y exists and optionally depends on a/x, which does not exist yet.
	mustApplied(t, s.Apply(ctx, []KV{kv("b", obj("y", "1", "?a/x"))}, nil))
	st.reset()
	r := s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "?a/x"))}, nil)
	mustApplied(t, r)
	if got := strings.Join(st.ops(), ","); got != "delete b/y,create a/x,create b/y" {
		t.Fatalf("ops %s", got)
	}
	// Deleting a/x again re-creates b/y around it.
	st.reset()
	mustApplied(t, s.Apply(ctx, []KV{kv("b", obj("y", "1", "?a/x"))}, nil))
	if got := strings.Join(st.ops(), ","); got != "delete b/y,delete a/x,create b/y" {
		t.Fatalf("ops %s", got)
	}
}

// observe is an observe-only descriptor (D-065): Retrieve reports foreign objects too.
type observe struct{ mem }

func (*observe) DeleteOnAbsence() bool { return false }

func TestObserveOnlyNeverDeletesOnAbsence(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(&observe{mem{name: "interface", st: st}})
	s := New(reg, nil)
	ctx := context.Background()
	// Foreign objects visible through the observe-only descriptor.
	st.objs["interface/eth0"] = obj("eth0", "1")
	st.objs["interface/w3-tap"] = obj("w3-tap", "1")
	r := s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("interface", obj("x", "1", "a/x"))}, nil)
	mustApplied(t, r)
	if got := strings.Join(st.ops(), ","); got != "create a/x,create interface/x" {
		t.Fatalf("ops %s", got)
	}
	// Removing the desired alias: a/x is deleted, the undesired aliases (ours or foreign) are not.
	st.reset()
	r = s.Apply(ctx, nil, nil)
	mustApplied(t, r)
	if got := strings.Join(st.ops(), ","); got != "delete a/x" {
		t.Fatalf("ops %s", got)
	}
	if st.objs["interface/eth0"] == nil || st.objs["interface/w3-tap"] == nil {
		t.Fatal("foreign objects deleted")
	}
}

// re records Reapply calls.
type re struct {
	mem
	calls []Key
}

func (r *re) Reapply(_ context.Context, o proto.Message, _ any) error {
	r.calls = append(r.calls, r.KeyOf(o))
	return nil
}

func TestReapplyOnResyncOnly(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	d := &re{mem: mem{name: "t", st: st}}
	reg.Register(d)
	s := New(reg, nil)
	ctx := context.Background()
	desired := []KV{kv("t", obj("x", "1"))}
	mustApplied(t, s.Apply(ctx, desired, nil)) // created, not reapplied
	mustApplied(t, s.Apply(ctx, desired, nil)) // unchanged, no resync → no reapply
	if len(d.calls) != 0 {
		t.Fatalf("reapply outside resync: %v", d.calls)
	}
	r := s.ApplyWith(ctx, desired, nil, ApplyOptions{Resync: true})
	mustApplied(t, r)
	if len(d.calls) != 1 || r.Reapplied != 1 || !r.Plan.Empty() {
		t.Fatalf("resync reapply: calls %v result %+v", d.calls, r)
	}
}

// M2: concurrent Plans (DryRun) and Retrieves with a write-only descriptor must not race
// (run with -race).
func TestConcurrentPlansWithWriteOnly(t *testing.T) {
	st := newStore()
	reg := NewRegistry()
	reg.Register(&mem{name: "a", st: st})
	reg.Register(&wo{mem{name: "w", st: st}})
	s := New(reg, nil)
	ctx := context.Background()
	mustApplied(t, s.Apply(ctx, []KV{kv("a", obj("x", "1")), kv("w", obj("q", "1"))}, nil))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := s.Plan(ctx, []KV{kv("a", obj("x", "1")), kv("w", obj("q", "2"))}, nil); err != nil {
					t.Error(err)
				}
				_, _ = s.Retrieve(ctx, nil)
				_, _ = s.WriteOnly()
			}
		}()
	}
	wg.Wait()
}

// changing descriptor: Create stores a different value than desired, so verification fails.
type changing struct{ mem }

func (c *changing) Create(ctx context.Context, o proto.Message) (any, error) {
	v := proto.Clone(o).(*structpb.Struct)
	v.Fields["val"] = structpb.NewStringValue("other")
	return c.mem.Create(ctx, v)
}

// TestErrorsAndLogsNeverPrintValues plants a secret in a desired value and makes verification fail:
// neither the transaction error nor any log line may contain it (review DF-5 H1).
func TestErrorsAndLogsNeverPrintValues(t *testing.T) {
	const secret = "VRX_TEST_PSK_scheduler_plant"
	st := newStore()
	reg := NewRegistry()
	reg.Register(&changing{mem{name: "s", st: st}})
	var logs strings.Builder
	s := New(reg, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	s.VerifyRetries = 0
	r := s.Apply(context.Background(), []KV{kv("s", obj("x", secret))}, nil)
	if r.Outcome == OutcomeApplied || r.Err == nil {
		t.Fatalf("expected a verification failure, got %s", r.Outcome)
	}
	all := r.Err.Error() + logs.String()
	for _, res := range r.Results {
		if res.Err != nil {
			all += res.Err.Error()
		}
	}
	if strings.Contains(all, secret) || strings.Contains(all, "other") {
		t.Fatalf("a value leaked into errors/logs:\n%s", all)
	}
	if !strings.Contains(r.Err.Error(), "s/x differs: google.protobuf.Struct fields [fields] (values redacted)") {
		t.Fatalf("error lacks the redacted summary: %v", r.Err)
	}
}
