package subsystems

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// TD-11c fix round 1 (review F1, D-133): batching must keep TD-11b's claim-first order durable.
// Inside a transaction every keyed claim is appended to <store>.journal (one write(2), no fsync)
// before the descriptor writes VPP; the transaction end compacts the journal into the snapshot
// with one atomic, fsync'd write; opening the store replays the journal over the snapshot.

// keyedVPP is an in-memory "VPP" of untagged objects (NAT static mappings, say): they carry no
// owner, so the family reports them only through its keyed claims.
type keyedVPP struct {
	mu   sync.Mutex
	objs map[string]bool
}

// keyedFamily is a KeyedClaims family in TD-11b's order: Create claims, then writes VPP; Delete
// writes VPP, then releases; Retrieve reports claimed objects only; Create of an object VPP already
// has fails ("exists", as VPP answers for a duplicate static mapping).
type keyedFamily struct {
	vpp    *keyedVPP
	claims *KeyedClaims
}

const keyedFamilyName = "t.keyed"

func keyedKV(name string) scheduler.KV {
	return scheduler.KV{Key: scheduler.Join(keyedFamilyName, name), Value: wrapperspb.String(name)}
}

func (*keyedFamily) Name() string { return keyedFamilyName }
func (*keyedFamily) KeyOf(o proto.Message) scheduler.Key {
	return scheduler.Join(keyedFamilyName, o.(*wrapperspb.StringValue).GetValue())
}
func (*keyedFamily) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (f *keyedFamily) Create(_ context.Context, o proto.Message) (any, error) {
	name := o.(*wrapperspb.StringValue).GetValue()
	if err := f.claims.Claim(name); err != nil {
		return nil, err
	}
	f.vpp.mu.Lock()
	defer f.vpp.mu.Unlock()
	if f.vpp.objs[name] {
		_ = f.claims.Release(name)
		return nil, fmt.Errorf("%s exists", name)
	}
	f.vpp.objs[name] = true
	return nil, nil
}
func (*keyedFamily) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}
func (f *keyedFamily) Delete(_ context.Context, o proto.Message, _ any) error {
	name := o.(*wrapperspb.StringValue).GetValue()
	f.vpp.mu.Lock()
	delete(f.vpp.objs, name)
	f.vpp.mu.Unlock()
	return f.claims.Release(name)
}
func (f *keyedFamily) Retrieve(context.Context) ([]scheduler.KV, error) {
	f.vpp.mu.Lock()
	defer f.vpp.mu.Unlock()
	var out []scheduler.KV
	for name := range f.vpp.objs {
		if f.claims.Claimed(name) {
			out = append(out, keyedKV(name))
		}
	}
	return out, nil
}

// agentOn builds the product wiring over dir (one agent process) plus the keyed family on v.
func agentOn(t *testing.T, dir string, v *keyedVPP) (*Wiring, *scheduler.Scheduler, *KeyedClaims) {
	t.Helper()
	owned, err := ownertable.Open(dir, "w1")
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: coretest.New(), Owner: "w1", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	w.identity.Set(bootid.Identity{BootID: "b", PID: 1, StartTime: 1}) // the same VPP instance throughout
	k, err := w.KeyedClaims("nat")
	if err != nil {
		t.Fatal(err)
	}
	reg.Register(&keyedFamily{vpp: v, claims: k})
	s := scheduler.New(reg, nil)
	s.VerifyRetries = 0
	return w, s, k
}

func TestKeyedClaimsSurviveAgentDeathMidTransaction(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	v := &keyedVPP{objs: map[string]bool{}}
	scope := scheduler.Only(keyedFamilyName)
	desired := []scheduler.KV{keyedKV("10.1.0.1:80"), keyedKV("10.1.0.2:80")}

	// Agent 1: the transaction writes VPP and dies before the end of the transaction (no compaction).
	w1, s1, _ := agentOn(t, dir, v)
	_ = w1.ClaimsTxn() // opened by applyLocked; the process dies before the returned end runs
	if r := s1.Apply(ctx, desired, scope); r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply %s: %v", r.Outcome, r.Err)
	}
	if len(v.objs) != 2 {
		t.Fatalf("VPP %v", v.objs)
	}

	// Agent 2 over the same state dir: the claims are known (journal replayed), so the objects are
	// ours: re-committing the same document is a no-op, removing them converges.
	_, s2, k2 := agentOn(t, dir, v)
	if !k2.Claimed("10.1.0.1:80") || !k2.Claimed("10.1.0.2:80") {
		t.Fatalf("claims of the interrupted transaction lost: %d records", k2.Len())
	}
	kvs, err := s2.Retrieve(ctx, scope)
	if err != nil || len(kvs) != 2 {
		t.Fatalf("after restart Retrieve %v %v (objects invisible)", kvs, err)
	}
	if r := s2.Apply(ctx, desired, scope); r.Outcome != scheduler.OutcomeApplied || !r.Plan.Empty() {
		t.Fatalf("re-commit %s %v %+v", r.Outcome, r.Err, r.Plan.Ops)
	}
	if r := s2.Apply(ctx, nil, scope); r.Outcome != scheduler.OutcomeApplied || len(v.objs) != 0 {
		t.Fatalf("removal %s %v, VPP %v", r.Outcome, r.Err, v.objs)
	}
}

// The journal is written before Claim returns, compacted by the end of the transaction, replayed
// on open with a torn last line ignored; a failed journal write fails the Claim (so the descriptor
// never writes VPP without a durable claim).
func TestKeyedClaimsJournal(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{}
	id.Set(bootid.Identity{BootID: "b", PID: 1, StartTime: 1})
	k, err := OpenKeyedClaims(dir, "nat", "w1", id)
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(dir, "claims-nat-w1.json")
	journal := filepath.Join(dir, "claims-nat-w1.journal")
	size := func() int64 {
		fi, err := os.Stat(journal)
		if err != nil {
			return 0
		}
		return fi.Size()
	}
	if err := k.Claim("old"); err != nil { // outside a transaction: snapshot at once, no journal
		t.Fatal(err)
	}
	if size() != 0 {
		t.Fatal("journal written outside a transaction")
	}
	k.Begin()
	for _, key := range []string{"a", "b", "c"} {
		before := size()
		if err := k.Claim(key); err != nil {
			t.Fatal(err)
		}
		if size() <= before {
			t.Fatalf("claim %s not in the journal when Claim returned", key)
		}
	}
	if err := k.Release("b"); err != nil {
		t.Fatal(err)
	}
	if err := k.Release("old"); err != nil {
		t.Fatal(err)
	}
	// death before the end: a reopen replays snapshot + journal
	k2, err := OpenKeyedClaims(dir, "nat", "w1", id)
	if err != nil {
		t.Fatal(err)
	}
	if !k2.Claimed("a") || k2.Claimed("b") || !k2.Claimed("c") || k2.Claimed("old") || k2.Len() != 2 {
		t.Fatalf("replay: a=%v b=%v c=%v old=%v len=%d", k2.Claimed("a"), k2.Claimed("b"), k2.Claimed("c"), k2.Claimed("old"), k2.Len())
	}
	// the end of the transaction compacts: one snapshot write, journal empty
	w0 := k.writes
	if err := k.Flush(); err != nil {
		t.Fatal(err)
	}
	if k.writes != w0+1 || size() != 0 {
		t.Fatalf("compaction: writes %d→%d, journal %d bytes", w0, k.writes, size())
	}
	if raw, _ := os.ReadFile(snap); len(raw) == 0 { //nolint:gosec // the test's temp dir
		t.Fatal("snapshot empty")
	}

	// a torn last line (the process died inside write(2)) is ignored; the whole lines before it count
	k.Begin()
	if err := k.Claim("d"); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(journal, os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // the test's temp dir
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"set":{"key":"e","bo`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	k3, err := OpenKeyedClaims(dir, "nat", "w1", id)
	if err != nil {
		t.Fatalf("a torn last journal line must not fail the open: %v", err)
	}
	if !k3.Claimed("d") || k3.Claimed("e") {
		t.Fatalf("torn line: d=%v e=%v", k3.Claimed("d"), k3.Claimed("e"))
	}
	if size() != 0 {
		t.Fatalf("open did not compact the replayed journal (%d bytes)", size())
	}
	// a corrupt line that is not the last fails closed, like a corrupt snapshot
	if err := os.WriteFile(journal, []byte("{broken\n{\"del\":\"a\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenKeyedClaims(dir, "nat", "w1", id); err == nil {
		t.Fatal("a corrupt journal line in the middle must fail closed")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}

	// a journal that cannot be written fails the Claim, and nothing is recorded
	k4, err := OpenKeyedClaims(dir, "nat", "w1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0o700); err != nil { // the journal path is unusable
		t.Fatal(err)
	}
	k4.Begin()
	if err := k4.Claim("f"); err == nil || k4.Claimed("f") {
		t.Fatalf("claim without a journal line: err %v, claimed %v", err, k4.Claimed("f"))
	}
	_ = os.Remove(journal)
	_ = k4.Flush()
}
