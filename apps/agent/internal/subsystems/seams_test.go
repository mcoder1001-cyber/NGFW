package subsystems

import (
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// The A5 seams are inert by default: without Env.Publish / Env.Resync nothing happens (and nothing
// panics); with them, Wiring forwards exactly what it was given.
func TestEventAndResyncHooksDefaultInert(t *testing.T) {
	w := &Wiring{}
	w.Publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_ERROR, Message: "dropped"})
	w.Publish(nil)
	w.RequestResync()

	var got []*vrxv1.Event
	resyncs := 0
	w = &Wiring{env: Env{Publish: func(ev *vrxv1.Event) { got = append(got, ev) }, Resync: func() { resyncs++ }}}
	ev := &vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_ERROR, Message: "x"}
	w.Publish(ev)
	w.Publish(nil) // a nil event never reaches the sink
	w.RequestResync()
	if len(got) != 1 || got[0] != ev || resyncs != 1 {
		t.Fatalf("hooks: events %v, resyncs %d", got, resyncs)
	}
}

func TestSlotIDRange(t *testing.T) {
	t.Setenv(EnvTableBase, "")
	if r, err := SlotIDRange(); r != nil || err != nil {
		t.Fatalf("unset: %v %v (want nil range = every id)", r, err)
	}
	t.Setenv(EnvTableBase, "3000")
	r, err := SlotIDRange()
	if err != nil || r == nil || *r != (IDRange{Lo: 3000, Hi: 3999}) {
		t.Fatalf("slot 3: %v %v", r, err)
	}
	for _, bad := range []string{"x", "-1", "0", "4294967295", "4294967296"} {
		t.Setenv(EnvTableBase, bad)
		if r, err := SlotIDRange(); err == nil {
			t.Errorf("%s=%q: want an error, got %v", EnvTableBase, bad, r)
		}
	}
	t.Setenv(EnvTableBase, "4294966296") // the last base whose range still fits in 32 bits
	if r, err := SlotIDRange(); err != nil || r.Hi != 4294967295 {
		t.Fatalf("top range: %v %v", r, err)
	}
}
