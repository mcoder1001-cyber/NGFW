package qos

// F-qos-flat gap tests: TD-11b ownership declarations, claim-first per-interface Creates and the qos.meta record.

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	binqos "ngfw/agent/binapi/qos"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

type regRecorder struct{ ds []scheduler.Descriptor }

func (r *regRecorder) Register(d scheduler.Descriptor) { r.ds = append(r.ds, d) }

type persistedClaims struct {
	mu   sync.Mutex
	m    map[[2]string]bool
	fail error
}

func (c *persistedClaims) Persistent() bool { return true }
func (c *persistedClaims) Claim(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.m[[2]string{n, h}] = true
	return nil
}
func (c *persistedClaims) Release(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{n, h})
	return nil
}
func (c *persistedClaims) Claimed(n, h string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[[2]string{n, h}]
}

// TestOwnershipDeclared (TD-11b): every qos descriptor declares how it records ownership; the per-interface ones
// accept only a claim store that survives an agent restart, the record only a persisted file.
func TestOwnershipDeclared(t *testing.T) {
	const owner = "w0qq"
	r := &regRecorder{}
	Register(r, df7test.NewFake(), owner)
	r.Register(NewMeta(nil))
	if len(r.ds) != 5 {
		t.Fatalf("registered %d", len(r.ds))
	}
	iface.SetClaimStore(owner, nil)
	for _, d := range r.ds {
		if err := persist.Declared(d); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
		}
		if _, checks := d.(persist.Checker); checks && persist.Check(d) == nil {
			t.Errorf("%s accepted an in-memory store", d.Name())
		}
	}
	iface.SetClaimStore(owner, &persistedClaims{m: map[[2]string]bool{}})
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	r.ds[4] = NewMeta(&FileMetaStore{Path: filepath.Join(t.TempDir(), "qos.json")})
	for _, d := range r.ds {
		if err := persist.Check(d); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
		}
	}
}

// TestClaimFirst (TD-11b): record, store and mark on an untagged interface claim before the VPP write; a claim that
// cannot be recorded fails with nothing written.
func TestClaimFirst(t *testing.T) {
	claims := &persistedClaims{m: map[[2]string]bool{}, fail: errors.New("disk full")}
	iface.SetClaimStore(df7test.Owner, claims)
	t.Cleanup(func() { iface.SetClaimStore(df7test.Owner, nil) })
	f, _, maps := fakeQoS()
	ctx := t.Context()
	cases := []struct {
		d   scheduler.Descriptor
		obj any
		msg string
	}{
		{NewRecord(f, df7test.Owner), Record{Interface: "eth0", Source: SourceIP}, "qos_record_enable_disable"},
		{NewStore(f, df7test.Owner), Store{Interface: "eth0", Source: SourceIP, Value: 3}, "qos_store_enable_disable"},
		{NewMark(f, df7test.Owner), Mark{Interface: "eth0", Source: SourceIP, Map: 7}, "qos_mark_enable_disable"},
	}
	maps[7] = binqos.QosEgressMap{ID: 7}
	for _, c := range cases {
		if _, err := c.d.Create(ctx, df7.Encode(c.obj)); err == nil || len(f.CallsNamed(c.msg)) != 0 {
			t.Errorf("%s: refused claim must fail before VPP: %v, %d calls", c.d.Name(), err, len(f.CallsNamed(c.msg)))
		}
	}
	claims.fail = nil
	for _, c := range cases {
		if _, err := c.d.Create(ctx, df7.Encode(c.obj)); err != nil {
			t.Fatalf("%s: %v", c.d.Name(), err)
		}
		if !df7test.Claimed(ctx, f, df7test.Owner, "eth0", string(c.d.KeyOf(df7.Encode(c.obj)))) {
			t.Errorf("%s: not claimed", c.d.Name())
		}
	}
}

// TestMetaRecord: the record round-trips through the file store canonically, an empty store retrieves nothing, a
// corrupt file counts as absent, and Validate refuses duplicate map ids.
func TestMetaRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qos-w0.json")
	d := NewMeta(&FileMetaStore{Path: path})
	ctx := t.Context()
	if kvs, err := d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("empty store: %v %v", kvs, err)
	}
	m := DocMeta{
		Policers:   map[string]string{"gold": "customer"},
		Shapers:    map[string]ShaperMeta{"up": {Burst: true}},
		Maps:       map[string]MapMeta{"remark": {ID: 1001, ExplicitID: true, Rows: map[string][]int{"ip": {46, 8}}}, "auto": {ID: 1000}},
		Interfaces: map[string]string{"loop1": "uplink"},
	}
	want := df7.KV(KeyMeta(), m, nil)
	if d.KeyOf(want.Value) != "qos.meta/services.qos" {
		t.Fatalf("key %s", d.KeyOf(want.Value))
	}
	if _, err := d.Create(ctx, want.Value); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, want)
	got, ok := DocMetaOf([]scheduler.KV{want})
	if !ok || got.Maps["remark"].ID != 1001 {
		t.Fatalf("meta %+v", got)
	}
	if name, mm, ok := got.MapByID(1000); !ok || name != "auto" || mm.ExplicitID {
		t.Fatalf("by id: %s %+v", name, mm)
	}
	if err := d.Delete(ctx, want.Value, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatal("record survived its delete")
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if kvs, err := d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("corrupt file: %v %v", kvs, err)
	}
	dup := DocMeta{Maps: map[string]MapMeta{"a": {ID: 1}, "b": {ID: 1}}}
	if _, err := d.Create(ctx, df7.Encode(dup)); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("duplicate ids: %v", err)
	}
}
