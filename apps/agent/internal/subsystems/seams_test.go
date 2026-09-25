package subsystems

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/vpn"
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
	if r, err := SlotIDRange(); r == nil || !r.Empty() || !errors.Is(err, ErrNoIDRange) {
		t.Fatalf("unset: %v %v (want the empty range and ErrNoIDRange: fail closed)", r, err)
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
		if r, err := SlotIDRange(); err == nil || errors.Is(err, ErrNoIDRange) || r == nil || !r.Empty() {
			t.Errorf("%s=%q: want a malformed-value error and the empty range, got %v %v", EnvTableBase, bad, r, err)
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
	none, err := w.IDRange()
	if none == nil || !errors.Is(err, ErrNoIDRange) {
		t.Fatalf("zero Env.IDs: %v %v (want a non-nil empty range and ErrNoIDRange)", none, err)
	}
	// R4: a family that ignores the error owns nothing, whichever range type it converts to — nil
	// (df2/df7) and the zero vpn.IDRange mean "every id".
	for _, id := range []uint32{0, 1, 7000, 13000, ^uint32(0)} {
		if (*df2.IDRange)(none).Owns(id) || (*df7.IDRange)(none).Owns(id) || vpn.IDRange(*none).Contains(id) {
			t.Errorf("id %d is owned by the fail-closed range %v", id, none)
		}
	}
	// ... and so does one that converts with the helpers.
	for _, id := range []uint32{0, 1, 7000, 13000, ^uint32(0)} {
		if none.DF2().Owns(id) || none.DF7().Owns(id) || none.VPN().Contains(id) {
			t.Errorf("id %d is owned by the converted fail-closed range", id)
		}
	}
	// The zero range 0..0 has no vpn.IDRange of its own (the zero value means every id): it fails closed.
	if zero := (&IDRange{}); zero.VPN().Contains(0) || zero.VPN().Contains(1) || !zero.DF2().Owns(0) || zero.DF7().Owns(1) {
		t.Errorf("0..0 converts wrongly: %v %v %v", zero.VPN(), zero.DF2(), zero.DF7())
	}
	w = &Wiring{env: Env{IDs: IDScope{All: true}}}
	all, err := w.IDRange()
	if all != nil || err != nil {
		t.Fatalf("all: %v %v (want nil = every id)", all, err)
	}
	if !all.DF2().Owns(13000) || !all.DF7().Owns(1) || !all.VPN().Contains(^uint32(0)) {
		t.Fatal("nil (every id) does not convert to every id")
	}
	slot := &IDRange{Lo: 3000, Hi: 3999}
	if !slot.DF2().Owns(3000) || slot.DF7().Owns(4000) || !slot.VPN().Contains(3999) || slot.VPN().Contains(2999) {
		t.Fatal("a slot range converts wrongly")
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

// TD-8b (TD-8 verify V2): the Register-line pattern of an id-allocating df7 family —
// df7.WithIDs(ids.DF7()) from w.IDRange(), never nil or a missing option — owns nothing with NoIDs(),
// exactly the slot's range with one, and every id only with VRX_VPP_ID_RANGE=all.
func TestDF7FamilyTakesItsRangeFromTheWiring(t *testing.T) {
	opts := func(env Env) df7.Options {
		ids, _ := (&Wiring{env: env}).IDRange() // a family that fails on the error never gets here
		return df7.BuildOptions([]df7.Option{df7.WithIDs(ids.DF7())})
	}
	none := opts(Env{})
	for _, id := range []uint32{0, 1, 5000, 13000, ^uint32(0)} {
		if none.IDs.Owns(id) || none.CheckID("egress map", id) == nil {
			t.Errorf("NoIDs(): id %d is owned", id)
		}
	}
	if o := df7.BuildOptions([]df7.Option{df7.WithIDs(NoIDs().DF7())}); o.IDs.Owns(1) || o.IDs.Owns(0) {
		t.Error("df7.WithIDs(NoIDs().DF7()) owns an id")
	}
	slot := opts(Env{IDs: IDScope{Range: &IDRange{Lo: 5000, Hi: 5999}}})
	if !slot.IDs.Owns(5000) || !slot.IDs.Owns(5999) || slot.IDs.Owns(4999) || slot.IDs.Owns(6000) {
		t.Errorf("slot 5: %+v", slot.IDs)
	}
	if all := opts(Env{IDs: IDScope{All: true}}); all.IDs != nil || !all.IDs.Owns(^uint32(0)) {
		t.Errorf("all: %+v, want every id", all.IDs)
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
