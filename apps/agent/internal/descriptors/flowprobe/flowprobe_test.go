package flowprobe

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

type model struct {
	params flowprobe.FlowprobeGetParamsReply
	ifs    map[uint32]flowprobe.FlowprobeInterfaceDetails
}

func newFake() (*dfkittest.FakeVPP, *model) {
	f := dfkittest.NewFake(
		dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"},
		dfkittest.Iface{Index: 8, Name: "loop601", Tag: "w6:loop601"},
		dfkittest.Iface{Index: 9, Name: "ens192"},
	)
	m := &model{params: flowprobe.FlowprobeGetParamsReply{ActiveTimer: 15, PassiveTimer: 120}, ifs: map[uint32]flowprobe.FlowprobeInterfaceDetails{}}
	f.On("flowprobe_set_params", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*flowprobe.FlowprobeSetParams)
		if len(m.ifs) > 0 {
			return []api.Message{&flowprobe.FlowprobeSetParamsReply{Retval: int32(api.UNSUPPORTED)}}, nil
		}
		m.params = flowprobe.FlowprobeGetParamsReply{RecordFlags: r.RecordFlags, ActiveTimer: r.ActiveTimer, PassiveTimer: r.PassiveTimer}
		return []api.Message{&flowprobe.FlowprobeSetParamsReply{}}, nil
	})
	f.On("flowprobe_get_params", func(api.Message) ([]api.Message, error) {
		p := m.params
		return []api.Message{&p}, nil
	})
	f.On("flowprobe_interface_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*flowprobe.FlowprobeInterfaceAddDel)
		idx := uint32(r.SwIfIndex)
		cur, exists := m.ifs[idx]
		rv := int32(0)
		switch {
		case m.params.RecordFlags == 0:
			rv = int32(api.CANNOT_ENABLE_DISABLE_FEATURE)
		case r.IsAdd && exists:
			rv = int32(api.ENTRY_ALREADY_EXISTS)
		case !r.IsAdd && (!exists || cur.Which != r.Which):
			rv = int32(api.NO_SUCH_ENTRY)
		case r.IsAdd:
			m.ifs[idx] = flowprobe.FlowprobeInterfaceDetails{SwIfIndex: r.SwIfIndex, Which: r.Which, Direction: r.Direction}
		default:
			delete(m.ifs, idx)
		}
		return []api.Message{&flowprobe.FlowprobeInterfaceAddDelReply{Retval: rv}}, nil
	})
	f.On("flowprobe_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, d := range m.ifs {
			d := d
			out = append(out, &d)
		}
		return out, nil
	})
	return f, m
}

func TestParams(t *testing.T) {
	f, m := newFake()
	d := NewParams(f, WithGlobals(dfkit.GlobalsOwner(true)))
	ctx := context.Background()
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("unset params reported %v", kvs)
	}
	v := Params{RecordL2: true, RecordL4: true, ActiveTimer: 10, PassiveTimer: 60}.Proto()
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if m.params.RecordFlags != flowprobe.FLOWPROBE_RECORD_FLAG_L2|flowprobe.FLOWPROBE_RECORD_FLAG_L4 {
		t.Fatalf("flags %v", m.params.RecordFlags)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	if _, err := d.Update(ctx, v, v, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	// D-071: a non-owner only requires the params
	other := NewParams(f)
	sets := len(f.CallsNamed("flowprobe_set_params"))
	if _, err := other.Create(ctx, v); err != nil {
		t.Fatalf("requirement met: %v", err)
	}
	if _, err := other.Create(ctx, Params{RecordL2: true, ActiveTimer: 1, PassiveTimer: 2}.Proto()); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("requirement not met: %v", err)
	}
	if err := other.Delete(ctx, v, nil); err != nil || len(f.CallsNamed("flowprobe_set_params")) != sets {
		t.Fatalf("non-owner must never set: %v", err)
	}
	if _, err := other.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, d, KeyParams)
	for _, bad := range []Params{{}, {RecordL3: true, ActiveTimer: 100, PassiveTimer: 10}, {RecordL3: true, ActiveTimer: ^uint32(0)}} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestInterface(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	pd := NewParams(f, WithGlobals(dfkit.GlobalsOwner(true)))
	d := NewInterface(f, "w5")
	v := Interface{Interface: "loop501", Which: "ip6", Direction: "tx"}.Proto()
	deps := d.Dependencies(v)
	if len(deps) != 3 || deps[0].Key != "interface/loop501" || deps[1].Key != KeyParams || deps[2].Key != "ipfix.default-exporter/global" || !deps[2].Optional {
		t.Fatalf("deps %+v", deps)
	}
	// without params VPP refuses
	if _, err := d.Create(ctx, v); !dfkit.IsVPPError(err, api.CANNOT_ENABLE_DISABLE_FEATURE) {
		t.Fatalf("create before params: %v", err)
	}
	pv := Params{RecordL3: true, ActiveTimer: 15, PassiveTimer: 120}.Proto()
	if _, err := pd.Create(ctx, pv); err != nil {
		t.Fatal(err)
	}
	meta, err := d.Create(ctx, v)
	if err != nil || meta != (InterfaceMeta{SwIfIndex: 7}) {
		t.Fatalf("create %v %v", meta, err)
	}
	m.ifs[8] = flowprobe.FlowprobeInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(8)} // w6's
	got := dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	if got.Meta != meta || len(dfkittest.MustRetrieve(t, d)) != 1 {
		t.Fatalf("retrieve %v", dfkittest.MustRetrieve(t, d))
	}
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	if _, err := d.Create(ctx, v); err != nil { // idempotent re-apply
		t.Fatal(err)
	}
	other := Interface{Interface: "loop501", Which: "ip4", Direction: "rx"}.Proto()
	if _, err := d.Create(ctx, other); !dfkit.IsVPPError(err, api.ENTRY_ALREADY_EXISTS) {
		t.Fatalf("conflicting variant: %v", err)
	}
	if _, err := d.Update(ctx, v, other, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	// params cannot change while an interface is enabled
	if _, err := pd.Create(ctx, Params{RecordL2: true, ActiveTimer: 1, PassiveTimer: 2}.Proto()); !dfkit.IsVPPError(err, api.UNSUPPORTED) {
		t.Fatalf("params with enabled iface: %v", err)
	}
	for range 2 {
		if err := d.Delete(ctx, v, meta); err != nil {
			t.Fatal(err)
		}
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(v))
	if _, err := d.Create(ctx, Interface{Interface: "loop601", Which: "ip4", Direction: "rx"}.Proto()); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatalf("foreign interface: %v", err)
	}
	for _, bad := range []Interface{{Interface: "loop501", Which: "mpls", Direction: "rx"}, {Interface: "loop501", Which: "ip4", Direction: "in"}, {Which: "ip4", Direction: "rx"}} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	// untagged interface: usable with a claim, reported only while claimed (D-071)
	uv := Interface{Interface: "ens192", Which: "l2", Direction: "rx"}.Proto()
	if _, err := d.Create(ctx, uv); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, uv))
	if kvs := dfkittest.MustRetrieve(t, NewInterface(f, "w7")); len(kvs) != 0 {
		t.Fatalf("unclaimed untagged interface reported: %v", kvs)
	}
	if err := d.Delete(ctx, uv, nil); err != nil || dfkit.Claims("w5").Claimed("ens192", NameInterface) {
		t.Fatalf("delete/release: %v", err)
	}
	r := scheduler.NewRegistry()
	RegisterGlobals(r, f)
	Register(r, f, "w5")
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
}
