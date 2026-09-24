package subsystems

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/scheduler"
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

// TD-8: the id range fails closed — unset is ErrNoIDRange (no id), never "every id"; every id needs
// VRX_VPP_ID_RANGE=all explicitly, and contradictory or malformed settings are errors.
func TestSlotIDRange(t *testing.T) {
	t.Setenv(EnvTableBase, "")
	t.Setenv(EnvIDRange, "")
	if r, err := SlotIDRange(); r != nil || !errors.Is(err, ErrNoIDRange) {
		t.Fatalf("unset: %v %v (want ErrNoIDRange: fail closed)", r, err)
	}
	if s, err := ResolveIDScope(); s != (IDScope{}) || !errors.Is(err, ErrNoIDRange) || s.String() != "none (fail closed)" {
		t.Fatalf("unset scope: %+v %v", s, err)
	}
	t.Setenv(EnvIDRange, "all")
	if r, err := SlotIDRange(); r != nil || err != nil {
		t.Fatalf("%s=all: %v %v (want nil range = every id)", EnvIDRange, r, err)
	}
	if s, _ := ResolveIDScope(); !s.All || s.Range != nil || s.String() != "all" {
		t.Fatalf("%s=all scope: %+v", EnvIDRange, s)
	}
	for _, bad := range []string{"ALL", "yes", "1000-1999", "*"} {
		t.Setenv(EnvIDRange, bad)
		if s, err := ResolveIDScope(); err == nil || s != (IDScope{}) {
			t.Errorf("%s=%q: want an error and no id, got %+v", EnvIDRange, bad, s)
		}
	}
	t.Setenv(EnvIDRange, "all")
	t.Setenv(EnvTableBase, "3000")
	if s, err := ResolveIDScope(); err == nil || s != (IDScope{}) {
		t.Fatalf("both set: want an error and no id, got %+v %v", s, err)
	}
	t.Setenv(EnvIDRange, "")
	r, err := SlotIDRange()
	if err != nil || r == nil || *r != (IDRange{Lo: 3000, Hi: 3999}) {
		t.Fatalf("slot 3: %v %v", r, err)
	}
	if s, _ := ResolveIDScope(); s.String() != "3000-3999" || s.All {
		t.Fatalf("slot 3 scope: %v", s)
	}
	for _, bad := range []string{"x", "-1", "0", "4294967295", "4294967296"} {
		t.Setenv(EnvTableBase, bad)
		if r, err := SlotIDRange(); err == nil || errors.Is(err, ErrNoIDRange) {
			t.Errorf("%s=%q: want a malformed-value error, got %v %v", EnvTableBase, bad, r, err)
		}
	}
	t.Setenv(EnvTableBase, "4294966296") // the last base whose range still fits in 32 bits
	if r, err := SlotIDRange(); err != nil || r.Hi != 4294967295 {
		t.Fatalf("top range: %v %v", r, err)
	}
}

// Families read the range through the wiring (Env.IDs), which fails closed on the zero value.
func TestWiringIDRangeFailsClosed(t *testing.T) {
	t.Setenv(EnvTableBase, "7000") // the environment is never read by Wiring.IDRange
	w := &Wiring{}
	if r, err := w.IDRange(); r != nil || !errors.Is(err, ErrNoIDRange) {
		t.Fatalf("zero Env.IDs: %v %v (want ErrNoIDRange)", r, err)
	}
	w = &Wiring{env: Env{IDs: IDScope{All: true}}}
	if r, err := w.IDRange(); r != nil || err != nil {
		t.Fatalf("all: %v %v (want nil = every id)", r, err)
	}
	w = &Wiring{env: Env{IDs: IDScope{Range: &IDRange{Lo: 5000, Hi: 5999}, All: true}}}
	r, err := w.IDRange()
	if err != nil || r == nil || *r != (IDRange{Lo: 5000, Hi: 5999}) {
		t.Fatalf("range wins over all: %v %v", r, err)
	}
	r.Lo = 1 // a copy: a family cannot widen the agent's range
	if again, _ := w.IDRange(); again.Lo != 5000 {
		t.Fatalf("IDRange returned the agent's own range: %v", again)
	}
}

// S1: AddDynamicSource keeps sources out of the configuration domains and apart from each other.
func TestAddDynamicSourceValidation(t *testing.T) {
	desired := func(*vrxv1.DesiredState) []scheduler.KV { return nil }
	w := &Wiring{}
	if got := w.DynamicSources(); len(got) != 0 {
		t.Fatalf("default: %v", got)
	}
	ok := DynamicSource{Name: "mpls-ldp", Descriptors: []string{"mpls-route.ldp"}, Desired: desired}
	if err := w.AddDynamicSource(ok); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]DynamicSource{
		"duplicate name":        {Name: "mpls-ldp", Descriptors: []string{"mfib.route.pim"}, Desired: desired},
		"empty name":            {Descriptors: []string{"x.y"}, Desired: desired},
		"upper-case name":       {Name: "Mpls", Descriptors: []string{"x.y"}, Desired: desired},
		"no descriptors":        {Name: "a", Desired: desired},
		"no Desired":            {Name: "b", Descriptors: []string{"x.y"}},
		"domain descriptor":     {Name: "c", Descriptors: []string{core.RouteName}, Desired: desired},
		"other source's":        {Name: "d", Descriptors: []string{"mpls-route.ldp"}, Desired: desired},
		"listed twice":          {Name: "e", Descriptors: []string{"x.y", "x.y"}, Desired: desired},
		"not a descriptor name": {Name: "f", Descriptors: []string{"Bad/Name"}, Desired: desired},
	} {
		if err := w.AddDynamicSource(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	descs := []string{"mfib.route.pim", "mfib.itf.pim"}
	if err := w.AddDynamicSource(DynamicSource{Name: "igmp-mfib", Descriptors: descs, Desired: desired}); err != nil {
		t.Fatal(err)
	}
	descs[0] = "changed" // the wiring keeps its own copy
	got := w.DynamicSources()
	if len(got) != 2 || got[0].Name != "mpls-ldp" || got[1].Name != "igmp-mfib" || got[1].Descriptors[0] != "mfib.route.pim" {
		t.Fatalf("sources %+v", got)
	}
}

// Metrics collectors: inert by default, unique names, sorted.
func TestAddMetricsCollector(t *testing.T) {
	w := &Wiring{}
	if got := w.MetricsCollectors(); len(got) != 0 {
		t.Fatalf("default: %v", got)
	}
	collect := func(_ context.Context, out io.Writer) error { _, err := io.WriteString(out, "x 1\n"); return err }
	for _, n := range []string{"unbound", "dashboard"} {
		if err := w.AddMetricsCollector(MetricsCollector{Name: n, Collect: collect}); err != nil {
			t.Fatal(err)
		}
	}
	for name, bad := range map[string]MetricsCollector{
		"duplicate":  {Name: "unbound", Collect: collect},
		"no name":    {Collect: collect},
		"no Collect": {Name: "z"},
		"bad name":   {Name: "a b", Collect: collect},
	} {
		if err := w.AddMetricsCollector(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	got := w.MetricsCollectors()
	if len(got) != 2 || got[0].Name != "dashboard" || got[1].Name != "unbound" {
		t.Fatalf("collectors %+v", got)
	}
	var b strings.Builder
	if err := got[0].Collect(context.Background(), &b); err != nil || b.String() != "x 1\n" {
		t.Fatalf("collect: %q %v", b.String(), err)
	}
}
