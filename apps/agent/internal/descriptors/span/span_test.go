package span

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/span"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

type mk struct {
	from, to uint32
	l2       bool
}

func fakeSpan() (*df7test.Fake, map[mk]span.SpanState) {
	f := df7test.NewFake()
	st := map[mk]span.SpanState{}
	f.On("sw_interface_span_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*span.SwInterfaceSpanEnableDisable)
		k := mk{uint32(r.SwIfIndexFrom), uint32(r.SwIfIndexTo), r.IsL2}
		if r.State == span.SPAN_STATE_API_DISABLED {
			delete(st, k)
		} else {
			st[k] = r.State
		}
		return []api.Message{&span.SwInterfaceSpanEnableDisableReply{}}, nil
	})
	f.On("sw_interface_span_dump", func(m api.Message) ([]api.Message, error) {
		l2 := m.(*span.SwInterfaceSpanDump).IsL2
		var out []api.Message
		for k, s := range st {
			if k.l2 == l2 {
				out = append(out, &span.SwInterfaceSpanDetails{SwIfIndexFrom: interface_types.InterfaceIndex(k.from), SwIfIndexTo: interface_types.InterfaceIndex(k.to), State: s, IsL2: l2})
			}
		}
		return out, nil
	})
	return f, st
}

func TestMirror(t *testing.T) {
	f, st := fakeSpan()
	ctx := t.Context()
	d := New(f, df7test.Owner)
	v := df7test.Desired(d, df7.Encode(Mirror{Source: "loop0", Destination: "eth0", State: StateBoth}))
	if v.Key != "span.mirror/loop0/eth0/device" {
		t.Fatal(v.Key)
	}
	deps := d.Dependencies(v.Value)
	if len(deps) != 2 || deps[0].Key != "interface/loop0" || deps[1].Key != "interface/eth0" {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(ctx, v.Value) // an untagged destination (e.g. a physical port) is fine
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*span.SwInterfaceSpanEnableDisable](t, f, "sw_interface_span_enable_disable"); r.SwIfIndexFrom != 1 || r.SwIfIndexTo != 4 || r.State != span.SPAN_STATE_API_RX_TX || r.IsL2 {
		t.Fatalf("%+v", r)
	}
	l2 := df7test.Desired(d, df7.Encode(Mirror{Source: "loop1", Destination: "loop0", State: StateRx, L2: true}))
	if _, err := d.Create(ctx, l2.Value); err != nil {
		t.Fatal(err)
	}
	st[mk{3, 1, false}] = span.SPAN_STATE_API_TX // another owner's mirror
	df7test.AssertEmptyPlan(t, d, v, l2)

	n := df7.Encode(Mirror{Source: "loop0", Destination: "eth0", State: StateTx})
	if _, err := d.Update(ctx, v.Value, n, meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, n), l2)
	if _, err := d.Update(ctx, n, df7.Encode(Mirror{Source: "loop0", Destination: "eth0", State: StateTx, L2: true}), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, n, meta); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*span.SwInterfaceSpanEnableDisable](t, f, "sw_interface_span_enable_disable"); r.State != span.SPAN_STATE_API_DISABLED {
		t.Fatalf("%+v", r)
	}
	df7test.AssertEmptyPlan(t, d, l2)
	for i, bad := range []Mirror{{Source: "a", Destination: "a", State: StateRx}, {Source: "a", Destination: "b", State: "none"}, {Destination: "b", State: StateRx}} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := d.Create(ctx, df7.Encode(Mirror{Source: "loop9", Destination: "loop0", State: StateRx})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("foreign source: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(Mirror{Source: "loop0", Destination: "loop9", State: StateRx})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("foreign destination: %v", err)
	}
	if err := d.Delete(ctx, df7.Encode(Mirror{Source: "gone0", Destination: "eth0", State: StateRx}), nil); err != nil {
		t.Fatalf("delete on a vanished interface: %v", err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 1 {
		t.Fatal(r.Names())
	}
}
