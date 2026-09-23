package iface_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
)

const owner = "w2"

// world is the shared fixture: our loopback and tap, another owner's loopback, an untagged
// tap, a tagged interface of an unclaimed device class and local0.
type world struct {
	v                          *ifacetest.VPP
	loop, tap, other, untagged uint32
	unknown                    uint32
}

const (
	loopKey = "interface.loopback/loop201"
	tapKey  = "tapv2.tap/w2-tap0"
)

func newWorld() *world {
	w := &world{v: ifacetest.New()}
	w.loop = w.v.Add("loop201", "Loopback", "w2:loop201")
	w.other = w.v.Add("loop300", "Loopback", "w3:loop300")
	w.tap = w.v.Add("tap0", "tap", "w2:w2-tap0")
	w.untagged = w.v.Add("tap1", "tap", "")
	w.unknown = w.v.Add("gre0", "gre", "w2:gre0")
	return w
}

func TestTableKeysAndResolution(t *testing.T) {
	w := newWorld()
	tbl, err := iface.Dump(context.Background(), w.v, owner)
	if err != nil {
		t.Fatal(err)
	}
	for idx, want := range map[uint32]string{w.loop: loopKey, w.tap: tapKey} {
		if got, ok := tbl.KeyFor(idx); !ok || string(got) != want {
			t.Errorf("KeyFor(%d) = %q,%v want %q", idx, got, ok, want)
		}
	}
	for _, idx := range []uint32{0, w.other, w.untagged, w.unknown} {
		if k, ok := tbl.KeyFor(idx); ok {
			t.Errorf("KeyFor(%d) = %q, want not ours", idx, k)
		}
	}
	if idx, err := tbl.Index(loopKey); err != nil || idx != w.loop {
		t.Errorf("Index(%s) = %d, %v", loopKey, idx, err)
	}
	if _, err := tbl.Index("tapv2.tap/loop201"); !errors.Is(err, iface.ErrWrongKind) {
		t.Errorf("wrong kind: %v", err)
	}
	if _, err := tbl.Index("loop201"); !errors.Is(err, iface.ErrBadRef) {
		t.Errorf("bad ref: %v", err)
	}
	if _, err := tbl.Index("interface.loopback/loop300"); !errors.Is(err, iface.ErrNotFound) {
		t.Errorf("other owner's id must not resolve: %v", err)
	}
	// an unclaimed device class resolves by tag (kind unknown → not checked) …
	if idx, err := tbl.Index("gre.tunnel/gre0"); err != nil || idx != w.unknown {
		t.Errorf("unknown kind by tag: %d %v", idx, err)
	}
	// … and becomes a proper key once its descriptor registers the class
	iface.RegisterKind("gre", "gre.tunnel")
	if k, ok := tbl.KeyFor(w.unknown); !ok || k != "gre.tunnel/gre0" {
		t.Errorf("after RegisterKind: %q %v", k, ok)
	}
	defer func() {
		if recover() == nil {
			t.Error("conflicting RegisterKind must panic")
		}
	}()
	iface.RegisterKind("gre", "other.thing")
}

func TestRegisterNamesValidAndUnique(t *testing.T) {
	r := scheduler.NewRegistry()
	iface.Register(r, ifacetest.New(), owner)
	if r.Len() != 8 {
		t.Fatalf("registered %d descriptors, want 8: %v", r.Len(), r.Names())
	}
	for _, n := range r.Names() {
		if !scheduler.ValidName(n) {
			t.Errorf("invalid name %q", n)
		}
	}
}
