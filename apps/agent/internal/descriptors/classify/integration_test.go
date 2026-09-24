package classify

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func find(kvs []scheduler.KV, k scheduler.Key) *scheduler.KV {
	for i := range kvs {
		if kvs[i].Key == k {
			return &kvs[i]
		}
	}
	return nil
}

// ip4SrcMask is a one-vector L3 mask matching the IPv4 source address (bytes 12..15 of the
// IP header); the matching key carries the address at the same offset.
func ip4SrcMask() []byte {
	m := make([]byte, 16)
	copy(m[12:], []byte{255, 255, 255, 255})
	return m
}

func ip4SrcMatch(a, b, c, d byte) []byte {
	m := make([]byte, 16)
	copy(m[12:], []byte{a, b, c, d})
	return m
}

func TestClassifyOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	loop, idx := df2test.Loopback(t, c, 9)
	df2test.AddAddress(t, c, idx, fmt.Sprintf("10.%d.9.1/24", slot))
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "classify.json"))
	if err != nil {
		t.Fatal(err)
	}

	td := NewTable(c, store)
	table := &Table{Name: owner + "-t1", SkipNVectors: 0, MatchNVectors: 1, Mask: ip4SrcMask(), MissNextIndex: NoIndex, Nbuckets: 8}
	tmeta, err := td.Create(ctx, table)
	if err != nil {
		t.Fatal(err)
	}
	// D-095 c: the tables' cleanup is registered before every binding's, so on a failure the
	// bindings (registered later, run first) are gone before either table is deleted — and
	// TableDescriptor.Delete refuses a table that is still bound anyway
	var tableB *Table
	var tbmeta any
	t.Cleanup(func() {
		if tableB != nil {
			_ = td.Delete(df2test.Ctx(t), tableB, tbmeta)
		}
		_ = td.Delete(df2test.Ctx(t), table, tmeta)
	})
	actual, err := td.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, td.KeyOf(table))
	if kv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", td.KeyOf(table), actual)
	}
	t.Logf("table Retrieve = %+v (meta %+v)", kv.Value, kv.Meta)
	if want := NormalizeTable(table); !proto.Equal(kv.Value, want) || kv.Meta != tmeta {
		t.Fatalf("table Retrieve = %+v, want %+v", kv.Value, want)
	}

	sd := NewSession(c, store)
	session := &Session{Table: table.Name, Match: ip4SrcMatch(10, byte(slot), 9, 10), HitNextIndex: NoIndex, OpaqueIndex: NoIndex} //nolint:gosec // slot 1..12
	smeta, err := sd.Create(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sd.Delete(df2test.Ctx(t), session, smeta) })
	sactual, err := sd.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	skv := find(sactual, sd.KeyOf(session))
	if skv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", sd.KeyOf(session), sactual)
	}
	t.Logf("session Retrieve = %+v", skv.Value)
	if want := NormalizeSession(session); !proto.Equal(skv.Value, want) || skv.Meta != smeta {
		t.Fatalf("session Retrieve = %+v, want %+v", skv.Value, want)
	}

	// input-acl: the binding VPP reads back.
	ind := NewInputACL(c, owner, store)
	in := &InputAcl{Interface: loop, Ip4Table: table.Name}
	imeta, err := ind.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ind.Delete(df2test.Ctx(t), in, imeta) })
	iactual, err := ind.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ikv := find(iactual, ind.KeyOf(in))
	if ikv == nil {
		t.Fatalf("input-acl Retrieve does not show %s: %+v", ind.KeyOf(in), iactual)
	}
	t.Logf("input-acl Retrieve = %+v", ikv.Value)
	if !proto.Equal(ikv.Value, in) || ikv.Meta != imeta {
		t.Fatalf("input-acl Retrieve = %+v, want %+v", ikv.Value, in)
	}

	// Write-only bindings: apply and clear must succeed; Retrieve is typed-unsupported.
	ipd := NewInterfaceIPTable(c, owner, store)
	ipt := &InterfaceIpTable{Interface: loop, Table: table.Name}
	ipmeta, err := ipd.Create(ctx, ipt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ipd.Delete(df2test.Ctx(t), ipt, ipmeta) })
	if _, err := ipd.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("ip-table Retrieve: %v", err)
	}
	outd := NewOutputACL(c, owner, store)
	out := &OutputAcl{Interface: loop, Ip4Table: table.Name}
	ometa, err := outd.Create(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = outd.Delete(df2test.Ctx(t), out, ometa) })
	oactual, err := outd.Retrieve(ctx)
	if err != nil || find(oactual, outd.KeyOf(out)) == nil || !proto.Equal(find(oactual, outd.KeyOf(out)).Value, out) {
		t.Fatalf("output-acl Retrieve = %+v, %v", oactual, err)
	}
	t.Logf("output-acl Retrieve = %+v", find(oactual, outd.KeyOf(out)).Value)
	// A→B: Delete(A) + Create(B) as the scheduler does on ErrRecreate; the successful unbind
	// of B below proves VPP holds B (an unbind naming the wrong table fails with NO_SUCH_TABLE).
	tb := &Table{Name: owner + "-t2", MatchNVectors: 1, Mask: ip4SrcMask(), MissNextIndex: NoIndex, Nbuckets: 8}
	tbmeta, err = td.Create(ctx, tb)
	if err != nil {
		t.Fatal(err)
	}
	tableB = tb
	if err := outd.Delete(ctx, out, ometa); err != nil {
		t.Fatal(err)
	}
	outB := &OutputAcl{Interface: loop, Ip4Table: tableB.Name}
	if ometa, err = outd.Create(ctx, outB); err != nil {
		t.Fatal(err)
	}
	out = outB
	oactual, _ = outd.Retrieve(ctx)
	if kv := find(oactual, outd.KeyOf(outB)); kv == nil || !proto.Equal(kv.Value, outB) {
		t.Fatalf("output-acl after A→B = %+v", oactual)
	}
	t.Logf("output-acl after A→B = %+v", find(oactual, outd.KeyOf(outB)).Value)
	l2d := NewInterfaceL2Tables(c, owner, store)
	l2 := &InterfaceL2Tables{Interface: loop, Input: true, Ip4Table: table.Name}
	l2meta, err := l2d.Create(ctx, l2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l2d.Delete(df2test.Ctx(t), l2, l2meta) })
	t.Logf("bindings applied on %s: input-acl, output-acl, ip-table, l2-tables (table index %d)", loop, tmeta.(TableMeta).Index)

	df2test.Hold(t)
	// D-095 b: the table is refused while bindings exist — re-read from VPP (input ACL) and from
	// the records of the bindings VPP cannot report (ip-table, l2-tables; output ACL is on tableB)
	if err := td.Delete(ctx, table, tmeta); !errors.Is(err, ErrTableInUse) {
		t.Fatalf("Delete of a bound table: %v, want ErrTableInUse", err)
	} else if ids, _ := tableIDs(ctx, c); !ids[tmeta.(TableMeta).Index] {
		t.Fatal("bound table freed")
	} else {
		t.Logf("Delete of bound table refused: %v", err)
	}
	for _, del := range []func() error{
		func() error { return l2d.Delete(ctx, l2, l2meta) },
		func() error { return outd.Delete(ctx, out, ometa) },
		func() error { return ipd.Delete(ctx, ipt, ipmeta) },
		func() error { return ind.Delete(ctx, in, imeta) },
		func() error { return sd.Delete(ctx, session, smeta) },
		func() error { return td.Delete(ctx, table, tmeta) },
		func() error { return td.Delete(ctx, tableB, tbmeta) },
	} {
		if err := del(); err != nil {
			t.Fatal(err)
		}
	}
	if iactual, _ = ind.Retrieve(ctx); find(iactual, ind.KeyOf(in)) != nil {
		t.Fatalf("input-acl still retrieved: %+v", iactual)
	}
	if oactual, _ := outd.Retrieve(ctx); find(oactual, outd.KeyOf(out)) != nil {
		t.Fatalf("output-acl still retrieved: %+v", oactual)
	}
	if actual, _ = td.Retrieve(ctx); len(actual) != 0 || len(store.All()) != 0 {
		t.Fatalf("tables still retrieved: %+v store=%+v", actual, store.All())
	}
}

// TestTableDeleteStaleIndexOnHost is the host regression for re-review N2 / probe P3 (D-071):
// our table disappears out of band, another owner's table takes an index, and a Delete with a
// stale TableMeta pointing at that index must not remove it.
func TestTableDeleteStaleIndexOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	store := NewMemStore()
	td := NewTable(c, store)
	ours := &Table{Name: owner + "-stale", MatchNVectors: 1, Mask: ip4SrcMask(), MissNextIndex: NoIndex, Nbuckets: 8}
	meta, err := td.Create(ctx, ours)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteTable(ctx, c, meta.(TableMeta).Index); err != nil { // out of band
		t.Fatal(err)
	}
	// "Another owner's" table (other geometry), created straight through binapi.
	rep, err := classifyapi.NewServiceClient(c).ClassifyAddDelTable(ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: NoIndex, Nbuckets: 2,
		MemorySize: DefaultMemorySize, SkipNVectors: 1, MatchNVectors: 1, MaskLen: VectorSize, Mask: make([]byte, VectorSize), NextTableIndex: NoIndex, MissNextIndex: NoIndex})
	if err != nil {
		t.Fatal(err)
	}
	foreign := rep.NewTableIndex
	t.Cleanup(func() { _ = deleteTable(df2test.Ctx(t), c, foreign) })
	t.Logf("our table %d deleted out of band; foreign table now at index %d", meta.(TableMeta).Index, foreign)
	if err := td.Delete(ctx, ours, TableMeta{Index: foreign}); err != nil { // stale meta → foreign index (probe P3)
		t.Fatal(err)
	}
	if err := td.Delete(ctx, ours, meta); err != nil { // the original, now stale, meta
		t.Fatal(err)
	}
	ids, err := tableIDs(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !ids[foreign] {
		t.Fatalf("Delete by a stale index removed the foreign table %d", foreign)
	}
	if _, ok := store.Get(ours.Name); ok {
		t.Fatal("record of the vanished table kept")
	}
	t.Logf("foreign table %d survived both stale deletes; record dropped", foreign)
}
