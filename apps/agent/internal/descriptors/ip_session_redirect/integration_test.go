package sessionredirect

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/classify"
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

func TestRedirectOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	out, outIdx := df2test.Loopback(t, c, 10)
	df2test.AddAddress(t, c, outIdx, fmt.Sprintf("10.%d.10.1/24", slot))
	store := classify.NewMemStore()
	td := classify.NewTable(c, store)
	mask := make([]byte, 16)
	copy(mask[12:], []byte{255, 255, 255, 255})
	table := &classify.Table{Name: owner + "-isr", MatchNVectors: 1, Mask: mask, MissNextIndex: classify.NoIndex}
	tmeta, err := td.Create(ctx, table)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = td.Delete(df2test.Ctx(t), table, tmeta) })

	d := New(c, owner, store)
	match := make([]byte, 16)
	copy(match[12:], []byte{10, byte(slot), 10, 10}) //nolint:gosec // slot 1..12
	desired := &Redirect{Table: table.Name, Match: match, OpaqueIndex: classify.NoIndex, Paths: []*df2.FibPath{{NextHop: fmt.Sprintf("10.%d.10.254", slot), Interface: out}}}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Delete(df2test.Ctx(t), desired, meta) })
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, d.KeyOf(desired))
	if kv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", d.KeyOf(desired), actual)
	}
	t.Logf("redirect Retrieve = %+v", kv.Value)
	want, _ := Normalize(desired)
	if !proto.Equal(kv.Value, want) || kv.Meta != meta {
		t.Fatalf("Retrieve = %+v, want %+v", kv.Value, want)
	}
	updated := &Redirect{Table: table.Name, Match: match, OpaqueIndex: classify.NoIndex, Punt: true, Paths: desired.Paths}
	if _, err := d.Update(ctx, desired, updated, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update must ask for a recreate: %v", err)
	}
	df2test.Hold(t)
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); find(actual, d.KeyOf(desired)) != nil {
		t.Fatalf("still retrieved after Delete: %+v", actual)
	}
}
