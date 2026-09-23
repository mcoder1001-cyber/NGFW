package classify

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
)

func ip4Mask() []byte {
	m := make([]byte, 16)
	copy(m[12:], []byte{255, 255, 255, 255})
	return m
}

// H1: a store written against an earlier VPP instance claims nothing, and is reset.
func TestStoreDropsRecordsOfAnotherVPPInstance(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st, err := OpenFileStore(filepath.Join(t.TempDir(), "classify.json"))
	if err != nil {
		t.Fatal(err)
	}
	td := NewTable(v, st)
	desired := NormalizeTable(&Table{Name: "w3-a", Mask: ip4Mask(), MissNextIndex: NoIndex})
	if _, err := td.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if pid, known := st.Instance(); !known || pid != 0 {
		t.Fatalf("instance = %d %v", pid, known)
	}
	// VPP restarts (new vpe_pid); another owner's table gets index 0 with the same geometry.
	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4242})
	actual, err := td.Retrieve(ctx)
	if err != nil || len(actual) != 0 {
		t.Fatalf("Retrieve after VPP restart = %+v, %v (must claim nothing)", actual, err)
	}
	if recs := st.All(); len(recs) != 0 {
		t.Fatalf("records of the old instance kept: %+v", recs)
	}
	if pid, _ := st.Instance(); pid != 4242 {
		t.Fatalf("instance not updated: %d", pid)
	}
	reopened, _ := OpenFileStore(st.path)
	if pid, known := reopened.Instance(); !known || pid != 4242 || len(reopened.All()) != 0 {
		t.Fatalf("persisted instance = %d %v %+v", pid, known, reopened.All())
	}
}

// H1: within one instance, a record whose index now holds a table of another geometry (an
// index reused by someone else) is stale.
func TestStoreRejectsReusedIndexWithOtherGeometry(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	if err := st.Reset(0); err != nil {
		t.Fatal(err)
	}
	// "someone else's" table at index 0: skip 1, match 1, other mask.
	rep, _ := classifyapi.NewServiceClient(v).ClassifyAddDelTable(ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: NoIndex, Nbuckets: 2, MemorySize: DefaultMemorySize,
		SkipNVectors: 1, MatchNVectors: 1, MaskLen: 16, Mask: make([]byte, 16), NextTableIndex: NoIndex, MissNextIndex: NoIndex})
	if err := st.Put(TableRecord{Name: "w3-stale", Index: rep.NewTableIndex, MatchNVectors: 1, Mask: ip4Mask()}); err != nil {
		t.Fatal(err)
	}
	if live, err := LiveTables(ctx, v, st); err != nil || len(live) != 0 {
		t.Fatalf("LiveTables = %+v, %v", live, err)
	}
	// M6: LiveTables is read-only — the stale record is still there until a prune.
	if _, ok := st.Get("w3-stale"); !ok {
		t.Fatal("LiveTables mutated the store")
	}
	if actual, err := NewTable(v, st).Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, ok := st.Get("w3-stale"); ok {
		t.Fatal("Retrieve did not prune the stale record")
	}
	if _, ok := v.tables[rep.NewTableIndex]; !ok {
		t.Fatal("the foreign table must never be touched")
	}
	// A legacy record without a mask cannot be verified either.
	if err := st.Put(TableRecord{Name: "w3-legacy", Index: rep.NewTableIndex, SkipNVectors: 1, MatchNVectors: 1}); err != nil {
		t.Fatal(err)
	}
	if live, _ := LiveTables(ctx, v, st); len(live) != 0 {
		t.Fatalf("mask-less record trusted: %+v", live)
	}
}

// M6: a prune holds the store lock, so it waits for a Create in progress (which holds it
// from its own prune to the Put) instead of judging the new record on an old snapshot.
func TestPruneWaitsForCreate(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	st.Lock()
	done := make(chan error, 1)
	go func() { done <- Prune(ctx, v, st) }()
	select {
	case err := <-done:
		t.Fatalf("Prune ran while the transaction lock was held: %v", err)
	default:
	}
	st.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type failingStore struct{ *MemStore }

func (failingStore) Put(TableRecord) error { return errors.New("disk full") }

// L8: a table whose record cannot be stored is removed again.
func TestTableCreateRollsBackOnStoreError(t *testing.T) {
	v := newFakeVPP()
	_, err := NewTable(v, failingStore{NewMemStore()}).Create(context.Background(), &Table{Name: "w3-x", Mask: ip4Mask(), MissNextIndex: NoIndex})
	if err == nil {
		t.Fatal("Create succeeded without a record")
	}
	if len(v.tables) != 0 {
		t.Fatalf("orphaned table left in VPP: %+v", v.tables)
	}
}

func twoTables(t *testing.T, v *fakeVPP, st Store) {
	t.Helper()
	for _, n := range []string{"w3-A", "w3-B"} {
		if _, err := NewTable(v, st).Create(context.Background(), &Table{Name: n, Mask: ip4Mask(), MissNextIndex: NoIndex}); err != nil {
			t.Fatal(err)
		}
	}
}

// H2: output ACL A→B really switches (VPP keeps A on a plain add).
func TestOutputACLSwitchesTables(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	twoTables(t, v, st) // A = 0, B = 1
	d := NewOutputACL(v, "w3", st)
	a := &OutputAcl{Interface: "loop300", Ip4Table: "w3-A"}
	b := &OutputAcl{Interface: "loop300", Ip4Table: "w3-B"}
	meta, err := d.Create(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	// The planner sees A, desired B → Update → ErrRecreate → Delete(A) + Create(B).
	if _, err := d.Update(ctx, a, b, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, a, meta); err != nil {
		t.Fatal(err)
	}
	if meta, err = d.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got := v.output[5][0]; got != 1 {
		t.Fatalf("after A→B VPP binds table %d, want 1 (B)", got)
	}
	// And a Create while a recorded binding exists (e.g. after a lost Delete) unbinds first.
	if _, err := d.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got := v.output[5][0]; got != 0 {
		t.Fatalf("Create over a recorded binding kept table %d, want 0 (A)", got)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, a) {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	_ = meta
}

// H2: an output ACL bound outside this agent's records is visible (so it diffs) and a
// Create over it is refused instead of silently keeping the other table.
func TestOutputACLUnknownBinding(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	twoTables(t, v, st)
	v.output[5] = &[2]uint32{7, NoIndex}
	d := NewOutputACL(v, "w3", st)
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || actual[0].Value.(*OutputAcl).GetIp4Table() != "#unknown" {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := d.Create(ctx, &OutputAcl{Interface: "loop300", Ip4Table: "w3-A"}); err == nil {
		t.Fatal("Create over an unknown binding succeeded")
	}
}

// H3: bindings on an untagged (physical) interface are ours only when claimed; a foreign
// interface is refused.
func TestBindingsOnUntaggedInterfaces(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	twoTables(t, v, st)
	claims := acl.NewMemoryClaimStore()
	d := NewInputACL(v, "w3", st, df2.WithClaims(claims))
	in := &InputAcl{Interface: "GigabitEthernet0/0/0", Ip4Table: "w3-A"}
	meta, err := d.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.Claimed(string(d.KeyOf(in))) {
		t.Fatal("binding on an untagged interface not claimed")
	}
	// A restarted agent with the same (persisted) claims sees it ...
	actual, err := NewInputACL(v, "w3", st, df2.WithClaims(claims)).Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, in) {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	// ... an agent without the claim does not.
	if actual, _ = NewInputACL(v, "w3", st).Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("unclaimed binding reported: %+v", actual)
	}
	if err := d.Delete(ctx, in, meta); err != nil {
		t.Fatal(err)
	}
	if claims.Claimed(string(d.KeyOf(in))) {
		t.Fatal("claim not released")
	}
	if _, err := d.Create(ctx, &InputAcl{Interface: "loop200", Ip4Table: "w3-A"}); !errors.Is(err, df2.ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
	if _, err := d.Create(ctx, &InputAcl{Interface: "local0", Ip4Table: "w3-A"}); !errors.Is(err, df2.ErrForeignInterface) {
		t.Fatalf("local0: %v", err)
	}
}

// L8: a session may not take the match of an ip_session_redirect in the same table.
func TestSessionRefusesRedirectMatch(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	twoTables(t, v, st)
	m := append(make([]byte, 12), 10, 3, 0, 1)
	v.redirects = map[uint32][][]byte{0: {m}}
	if _, err := NewSession(v, st).Create(ctx, &Session{Table: "w3-A", Match: m, HitNextIndex: NoIndex, OpaqueIndex: NoIndex}); err == nil {
		t.Fatal("session over a redirect match accepted")
	}
}

// Fix round 2 / N2 (D-071): Delete by a stale index re-verifies identity first. Our table
// was removed out of band and its index now holds someone else's table: Delete must drop the
// record and leave the foreign table alone.
func TestTableDeleteReverifiesIdentity(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	td := NewTable(v, st)
	ours := &Table{Name: "w3-a", Mask: ip4Mask(), MissNextIndex: NoIndex}
	meta, err := td.Create(ctx, ours)
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(TableMeta).Index
	delete(v.tables, idx) // out of band
	v.next = idx          // the next table reuses the index
	foreign, _ := classifyapi.NewServiceClient(v).ClassifyAddDelTable(ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: NoIndex, Nbuckets: 2, MemorySize: DefaultMemorySize,
		SkipNVectors: 1, MatchNVectors: 1, MaskLen: 16, Mask: make([]byte, 16), NextTableIndex: NoIndex, MissNextIndex: NoIndex})
	if foreign.NewTableIndex != idx {
		t.Fatalf("fake did not reuse the index: %d", foreign.NewTableIndex)
	}
	if err := td.Delete(ctx, ours, meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.tables[idx]; !ok {
		t.Fatal("Delete by a stale index removed another owner's table")
	}
	if _, ok := st.Get("w3-a"); ok {
		t.Fatal("record of the vanished table kept")
	}
	// A meta index that is not the live record's index is not deleted either.
	m2, err := td.Create(ctx, &Table{Name: "w3-b", Mask: ip4Mask(), MissNextIndex: NoIndex})
	if err != nil {
		t.Fatal(err)
	}
	if err := td.Delete(ctx, &Table{Name: "w3-b"}, TableMeta{Index: idx}); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.tables[idx]; !ok {
		t.Fatal("Delete with a mismatching meta index removed the foreign table")
	}
	if _, ok := v.tables[m2.(TableMeta).Index]; !ok {
		t.Fatal("w3-b removed although the meta did not match it")
	}
}

// Fix round 2 / N4: cleanup succeeds when the bound table is already gone, and input-ACL
// Create refuses an interface that already has tables bound (VPP's add would be a no-op).
func TestACLCleanupWhenTableGone(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	st := NewMemStore()
	twoTables(t, v, st) // A = 0, B = 1
	out := NewOutputACL(v, "w3", st)
	o := &OutputAcl{Interface: "loop300", Ip4Table: "w3-A"}
	ometa, err := out.Create(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	delete(v.tables, 0) // table A removed out of band; VPP could no longer unbind it
	if err := out.Delete(ctx, o, ometa); err != nil {
		t.Fatalf("output-acl Delete with its table gone: %v", err)
	}
	if _, ok := st.GetOutput("loop300"); ok {
		t.Fatal("output record kept")
	}

	in := NewInputACL(v, "w3", st)
	i := &InputAcl{Interface: "loop300", Ip4Table: "w3-B"}
	imeta, err := in.Create(ctx, i)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := in.Create(ctx, &InputAcl{Interface: "loop300", Ip6Table: "w3-B"}); err == nil {
		t.Fatal("input-acl Create over a bound interface accepted")
	}
	// Delete of a retrieved value naming an unknown table ("#7") unbinds what VPP reports.
	if err := in.Delete(ctx, &InputAcl{Interface: "loop300", Ip4Table: "#7"}, imeta); err != nil {
		t.Fatalf("input-acl Delete by dumped indices: %v", err)
	}
	if _, bound := v.inputACL[5]; bound {
		t.Fatal("input ACL still bound")
	}
}
