package objects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// kvsOf is what the desired-state builder produces for an objects document: one single-entry
// value per object.
func kvsOf(t *testing.T, doc *vrxv1.ObjectsConfig) []scheduler.KV {
	t.Helper()
	var out []scheduler.KV
	for _, k := range Kinds {
		for _, e := range listKind(doc, k) {
			v, err := Value(k, e.name, e.value)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, scheduler.KV{Key: Key(k, e.name), Value: v})
		}
	}
	return out
}

// assembleDoc merges retrieved values back into one document (what the agent's assembler does).
func assembleDoc(t *testing.T, kvs []scheduler.KV) *vrxv1.ObjectsConfig {
	t.Helper()
	out := &vrxv1.ObjectsConfig{}
	for _, kv := range kvs {
		proto.Merge(out, kv.Value)
	}
	return out
}

// failing is a descriptor whose Create always fails: it makes a transaction roll back.
type failing struct{}

func (failing) Name() string                                      { return "test.fail" }
func (failing) KeyOf(proto.Message) scheduler.Key                 { return "test.fail/x" }
func (failing) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (failing) Create(context.Context, proto.Message) (any, error) {
	return nil, errors.New("injected failure")
}
func (failing) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, nil
}
func (failing) Delete(context.Context, proto.Message, any) error { return nil }
func (failing) Retrieve(context.Context) ([]scheduler.KV, error) { return nil, nil }

const familyDoc = `{
  "tags": {"prod": {"color": "#1e88e5"}},
  "addresses": {
    "web1": {"type": "host", "address": "192.0.2.10", "tags": ["prod"]},
    "web2": {"type": "host", "address": "192.0.2.11"},
    "cdn":  {"type": "fqdn", "fqdn": "cdn.w3.test", "description": "CDN"}
  },
  "addressGroups": {"web-servers": {"members": ["web1", "web2"], "tags": ["prod"]}},
  "services": {"https": {"protocol": "tcp", "destinationPorts": ["443"]}},
  "serviceGroups": {"web": {"members": ["https"]}},
  "schedules": {"office-hours": {"type": "recurring", "days": ["mon", "fri"], "start": "08:00", "end": "18:00"}},
  "zones": {"lan": {"interfaces": ["host-w3l0"]}}
}`

// The family through the scheduler: Apply creates every object in the store, Retrieve equals the
// desired document, a second Apply is empty, a failing transaction rolls the store back, and a
// reopened store (agent restart) retrieves the same document.
func TestFamilyApplyRetrieveRollbackRestart(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	dns := startDNS(t)
	rt := openRT(t, dir, dns, clock, &logBuf{})
	reg := scheduler.NewRegistry()
	Register(reg, rt)
	reg.Register(failing{})
	sched := scheduler.New(reg, nil)
	scope := scheduler.Only(DescriptorNames()...)
	ctx := context.Background()

	want := objectsDoc(t, familyDoc)
	res := sched.ApplyWith(ctx, kvsOf(t, want), scope, scheduler.ApplyOptions{})
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Created != 9 {
		t.Fatalf("apply: %v %+v %v", res.Outcome, res.Summary, res.Err)
	}
	got, err := sched.Retrieve(ctx, scope)
	if err != nil || !proto.Equal(assembleDoc(t, got), want) {
		t.Fatalf("Retrieve != desired (%v):\n%s", err, protojson.Format(assembleDoc(t, got)))
	}
	if !proto.Equal(rt.Snapshot(), want) {
		t.Fatalf("store != desired")
	}
	if res := sched.ApplyWith(ctx, kvsOf(t, want), scope, scheduler.ApplyOptions{}); res.Outcome != scheduler.OutcomeApplied || res.Summary.Created+res.Summary.Updated+res.Summary.Deleted != 0 {
		t.Fatalf("second apply not empty: %+v", res.Summary)
	}
	if len(rt.FQDNStates()) != 1 || rt.FQDNStates()[0].FQDN != "cdn.w3.test" {
		t.Fatalf("resolver does not track the fqdn object: %+v", rt.FQDNStates())
	}

	// a transaction that also changes web2, deletes the zone and fails elsewhere: nothing changes
	next := proto.Clone(want).(*vrxv1.ObjectsConfig)
	next.Addresses["web2"].Address = ptr("192.0.2.99")
	delete(next.Zones, "lan")
	kvs := append(kvsOf(t, next), scheduler.KV{Key: "test.fail/x", Value: &vrxv1.Tag{}})
	res = sched.ApplyWith(ctx, kvs, scheduler.Only(append(DescriptorNames(), "test.fail")...), scheduler.ApplyOptions{})
	if res.Outcome != scheduler.OutcomeRolledBack {
		t.Fatalf("want a rollback: %v %v", res.Outcome, res.Err)
	}
	if !proto.Equal(rt.Snapshot(), want) {
		t.Fatalf("rollback did not restore the store:\n%s", protojson.Format(rt.Snapshot()))
	}

	// restart: a new runtime on the same dir retrieves the applied document from disk
	rt.Close()
	rt2 := openRT(t, dir, dns, clock, &logBuf{})
	reg2 := scheduler.NewRegistry()
	Register(reg2, rt2)
	got, err = scheduler.New(reg2, nil).Retrieve(ctx, scope)
	if err != nil || !proto.Equal(assembleDoc(t, got), want) {
		t.Fatalf("Retrieve after restart != desired (%v)", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "objects-w3.json")) //nolint:gosec // the test's own temp dir
	if fi, _ := os.Stat(filepath.Join(dir, "objects-w3.json")); fi.Mode().Perm() != 0o600 || !strings.Contains(string(raw), "web-servers") {
		t.Fatalf("store file: %v\n%s", fi.Mode(), raw)
	}

	// an empty desired set deletes every object (D-041: an explicitly empty domain)
	res = scheduler.New(reg2, nil).ApplyWith(ctx, nil, scope, scheduler.ApplyOptions{})
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Deleted != 9 || len(kvsOf(t, rt2.Snapshot())) != 0 {
		t.Fatalf("delete all: %+v", res.Summary)
	}
}

func TestFamilyValueShape(t *testing.T) {
	d := &descriptor{kind: KindAddresses}
	two := objectsDoc(t, `{"addresses": {"a": {"type": "host", "address": "192.0.2.1"}, "b": {"type": "host", "address": "192.0.2.2"}}}`)
	wrongKind := objectsDoc(t, `{"tags": {"a": {}}}`)
	for name, v := range map[string]proto.Message{"two entries": two, "wrong kind": wrongKind, "not ObjectsConfig": &vrxv1.Tag{}} {
		if _, err := d.Create(context.Background(), v); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
		if k := d.KeyOf(v); k != "objects.address/" {
			t.Errorf("%s: key %q", name, k)
		}
	}
	g, _ := Value(KindAddressGroups, "g", &vrxv1.AddressGroup{Members: []string{"a", "h"}, Tags: []string{"prod"}})
	var deps []string
	for _, dep := range (&descriptor{kind: KindAddressGroups}).Dependencies(g) {
		if !dep.Optional {
			t.Fatalf("mandatory dependency %s", dep.Key)
		}
		deps = append(deps, string(dep.Key))
	}
	if strings.Join(deps, " ") != "objects.address/a objects.address-group/a objects.address/h objects.address-group/h objects.tag/prod" {
		t.Fatalf("deps %v", deps)
	}
	if _, err := Value(KindTags, "x", &vrxv1.AddressObject{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Value with the wrong message: %v", err)
	}
}

func TestStoreCorruptFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "objects-w3.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt store: %v", err)
	}
	// the FQDN state is a cache: unreadable → ignored, never fatal
	if err := os.WriteFile(filepath.Join(dir, "objects-fqdn-w3.json"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	lb := &logBuf{}
	if _, err := Open(Config{StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(lb)}); err != nil || !strings.Contains(lb.String(), "fqdn state unreadable") {
		t.Fatalf("corrupt fqdn state: %v %s", err, lb.String())
	}
}

// Open registers by (state dir, owner); reopening closes the previous runtime; Close unregisters.
func TestRuntimeRegistry(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(Config{StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(&logBuf{})})
	if err != nil {
		t.Fatal(err)
	}
	a.Start()
	if RuntimeFor(dir+"/", "w3") != a || RuntimeFor(dir, "w4") != nil {
		t.Fatal("lookup")
	}
	b, err := Open(Config{StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{"127.0.0.1:9"}), Log: slogTo(&logBuf{})})
	if err != nil {
		t.Fatal(err)
	}
	if RuntimeFor(dir, "w3") != b || a.cancel != nil {
		t.Fatal("reopen did not replace and stop the previous runtime")
	}
	a.Close() // no effect on b's registration
	if RuntimeFor(dir, "w3") != b {
		t.Fatal("closing the old runtime unregistered the new one")
	}
	b.Close()
	if RuntimeFor(dir, "w3") != nil {
		t.Fatal("Close did not unregister")
	}
}
