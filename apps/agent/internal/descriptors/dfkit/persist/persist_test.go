package persist_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
)

type memStore struct{}

type fileStore struct{ on bool }

func (f *fileStore) Persistent() bool { return f.on }

func TestIsAndRequire(t *testing.T) {
	var nilFile *fileStore
	for _, c := range []struct {
		s    any
		want bool
	}{{nil, false}, {memStore{}, false}, {&fileStore{on: false}, false}, {nilFile, false}, {&fileStore{on: true}, true}} {
		if got := persist.Is(c.s); got != c.want {
			t.Errorf("Is(%#v) = %v", c.s, got)
		}
	}
	if err := persist.Require("x: claims", &fileStore{on: true}, &fileStore{on: true}); err != nil {
		t.Fatal(err)
	}
	err := persist.Require("x: claims", &fileStore{on: true}, memStore{})
	if !errors.Is(err, persist.ErrVolatile) || !strings.Contains(err.Error(), "x: claims") || !strings.Contains(err.Error(), "memStore") {
		t.Fatalf("Require = %v", err)
	}
}

// desc is a minimal descriptor recording in store.
type desc struct {
	name  string
	store any
}

func (d *desc) Name() string                                      { return d.name }
func (d *desc) KeyOf(proto.Message) scheduler.Key                 { return scheduler.Join(d.name, "x") }
func (d *desc) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *desc) Create(context.Context, proto.Message) (any, error) {
	return nil, nil
}
func (d *desc) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, nil
}
func (d *desc) Delete(context.Context, proto.Message, any) error { return nil }
func (d *desc) Retrieve(context.Context) ([]scheduler.KV, error) { return nil, nil }
func (d *desc) CheckPersistent() error                           { return persist.Require(d.name, d.store) }

// wrapper decorates a descriptor by embedding the interface (subsystems' defaultTolerant/vethOnly shape).
type wrapper struct{ scheduler.Descriptor }

// unwrapper exposes the inner descriptor through Unwrap.
type unwrapper struct {
	scheduler.Descriptor
	inner scheduler.Descriptor
}

func (u unwrapper) Unwrap() scheduler.Descriptor { return u.inner }

func TestCheckSeesThroughWrappers(t *testing.T) {
	vol := &desc{name: "t.volatile", store: memStore{}}
	ok := &desc{name: "t.persisted", store: &fileStore{on: true}}
	for name, d := range map[string]scheduler.Descriptor{
		"plain":    vol,
		"embedded": &wrapper{vol},
		"twice":    &wrapper{&wrapper{vol}},
		"unwrap":   unwrapper{Descriptor: ok, inner: vol},
	} {
		if err := persist.Check(d); !errors.Is(err, persist.ErrVolatile) || !strings.Contains(err.Error(), "t.volatile") {
			t.Errorf("%s: Check = %v", name, err)
		}
	}
	if err := persist.Check(&wrapper{ok}); err != nil {
		t.Fatal(err)
	}
	if err := persist.Check(&wrapper{}); err != nil { // nil inner descriptor: nothing to check
		t.Fatal(err)
	}
	if err := persist.Check(struct{}{}); err != nil {
		t.Fatal(err)
	}
}

// none declares that it records no ownership.
type none struct{ scheduler.Descriptor }

func (none) RecordsNoOwnership() {}

// both declares both (a contradiction).
type both struct{ desc }

func (both) RecordsNoOwnership() {}

// bare declares nothing (scheduler.Descriptor only).
type bare struct{ scheduler.Descriptor }

// TestDeclared (fix round 1, review M1): every descriptor must declare how it records ownership —
// through wrappers; undeclared and contradictory declarations are refused.
func TestDeclared(t *testing.T) {
	checker := &desc{name: "t.checker", store: &fileStore{on: true}}
	noOwn := &none{}
	for name, c := range map[string]struct {
		d    any
		want error
	}{
		"checker":            {checker, nil},
		"no ownership":       {noOwn, nil},
		"wrapped checker":    {&wrapper{checker}, nil},
		"wrapped none":       {&wrapper{&wrapper{noOwn}}, nil},
		"undeclared":         {&bare{}, persist.ErrUndeclared},
		"wrapped undeclared": {&wrapper{&bare{}}, persist.ErrUndeclared},
		"both":               {&both{desc{name: "t.both"}}, persist.ErrConflictingDeclaration},
		"nil":                {nil, persist.ErrUndeclared},
	} {
		if err := persist.Declared(c.d); !errors.Is(err, c.want) || (c.want == nil) != (err == nil) {
			t.Errorf("%s: Declared = %v, want %v", name, err, c.want)
		}
	}
}
