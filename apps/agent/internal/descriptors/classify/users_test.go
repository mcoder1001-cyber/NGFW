package classify

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
)

// TestTableDeleteRefusesWhileBound (D-095 b): a table is never freed while a binding still
// refers to it — re-read from VPP (input ACL, chained table) or, for bindings VPP cannot
// report, from the store's records (output ACL, interface-ip-table, interface-l2-tables).
func TestTableDeleteRefusesWhileBound(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	store, err := OpenFileStore(filepath.Join(t.TempDir(), "classify.json"))
	if err != nil {
		t.Fatal(err)
	}
	td := NewTable(v, store)
	tbl := &Table{Name: "w2-t", MatchNVectors: 1, Mask: make([]byte, 16), MissNextIndex: NoIndex}
	meta, err := td.Create(ctx, tbl)
	if err != nil {
		t.Fatal(err)
	}
	mustRefuse := func(what string) {
		t.Helper()
		err := td.Delete(ctx, tbl, meta)
		if !errors.Is(err, ErrTableInUse) || !strings.Contains(err.Error(), what) {
			t.Fatalf("Delete while %s bound: %v", what, err)
		}
		if _, ok := v.tables[meta.(TableMeta).Index]; !ok {
			t.Fatalf("table freed while %s bound", what)
		}
	}

	// input ACL: read back from VPP
	in := NewInputACL(v, "w2", store)
	inObj := &InputAcl{Interface: "loop200", Ip4Table: "w2-t"}
	imeta, err := in.Create(ctx, inObj)
	if err != nil {
		t.Fatal(err)
	}
	mustRefuse("input-acl ip4 on loop200")
	if err := in.Delete(ctx, inObj, imeta); err != nil {
		t.Fatal(err)
	}

	// write-only interface-ip-table: from the (persisted) record
	ipd := NewInterfaceIPTable(v, "w2", store)
	ipObj := &InterfaceIpTable{Interface: "loop200", Table: "w2-t"}
	ipmeta, err := ipd.Create(ctx, ipObj)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFileStore(store.path)
	if err != nil || len(reopened.Bindings()) != 1 || reopened.Bindings()[0].Key != string(ipd.KeyOf(ipObj)) {
		t.Fatalf("binding record not persisted: %+v (%v)", reopened.Bindings(), err)
	}
	mustRefuse("classify.interface-ip-table/loop200/ipv4 (record)")
	if err := ipd.Delete(ctx, ipObj, ipmeta); err != nil {
		t.Fatal(err)
	}

	// l2 tables and output ACL records
	l2d := NewInterfaceL2Tables(v, "w2", store)
	l2 := &InterfaceL2Tables{Interface: "loop200", Input: true, OtherTable: "w2-t"}
	l2meta, err := l2d.Create(ctx, l2)
	if err != nil {
		t.Fatal(err)
	}
	mustRefuse("interface-l2-tables")
	if err := l2d.Delete(ctx, l2, l2meta); err != nil {
		t.Fatal(err)
	}
	outd := NewOutputACL(v, "w2", store)
	out := &OutputAcl{Interface: "loop200", Ip4Table: "w2-t"}
	ometa, err := outd.Create(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	mustRefuse("output-acl on loop200")
	if err := outd.Delete(ctx, out, ometa); err != nil {
		t.Fatal(err)
	}

	// a chained table
	next := &Table{Name: "w2-head", MatchNVectors: 1, Mask: make([]byte, 16), MissNextIndex: NoIndex, NextTable: "w2-t"}
	nmeta, err := td.Create(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	mustRefuse("chains to it")
	if err := td.Delete(ctx, next, nmeta); err != nil {
		t.Fatal(err)
	}

	// a write-only record of an interface that is gone does not block (its index is sanitized on reuse)
	if err := store.PutBinding(BindingRecord{Key: "classify.interface-ip-table/gone/ipv4", Interface: "gone", SwIfIndex: 99, Tables: []uint32{meta.(TableMeta).Index}}); err != nil {
		t.Fatal(err)
	}
	if err := td.Delete(ctx, tbl, meta); err != nil {
		t.Fatalf("Delete after every binding is gone: %v", err)
	}
	if _, ok := v.tables[meta.(TableMeta).Index]; ok {
		t.Fatal("table not deleted")
	}
}

// TestBindingDependencies (D-095 b): every descriptor that binds a classify table depends on
// the table and on the interface, so the scheduler deletes the binding before either.
func TestBindingDependencies(t *testing.T) {
	store := NewMemStore()
	for _, c := range []struct {
		d   scheduler.Descriptor
		obj proto.Message
	}{
		{NewInterfaceIPTable(nil, "w2", store), &InterfaceIpTable{Interface: "lan", Table: "t"}},
		{NewInterfaceL2Tables(nil, "w2", store), &InterfaceL2Tables{Interface: "lan", Input: true, Ip4Table: "t"}},
		{NewInputACL(nil, "w2", store), &InputAcl{Interface: "lan", Ip6Table: "t"}},
		{NewOutputACL(nil, "w2", store), &OutputAcl{Interface: "lan", Ip4Table: "t"}},
		{NewSession(nil, store), &Session{Table: "t"}},
	} {
		deps := map[scheduler.Key]bool{}
		for _, dep := range c.d.Dependencies(c.obj) {
			deps[dep.Key] = !dep.Optional
		}
		if !deps[TableKey("t")] {
			t.Errorf("%s: no mandatory dependency on %s (%v)", c.d.Name(), TableKey("t"), deps)
		}
		if _, isSession := c.obj.(*Session); !isSession && !deps["interface/lan"] {
			t.Errorf("%s: no mandatory dependency on interface/lan (%v)", c.d.Name(), deps)
		}
	}
}
