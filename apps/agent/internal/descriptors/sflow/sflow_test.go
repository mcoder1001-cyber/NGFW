package sflow

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/sflow"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

type model struct {
	g       Global
	hwOf    map[uint32]uint32 // sw → hw (deliberately different numbers)
	enabled map[uint32]bool   // by sw
	toggles int
}

func newFake() (*dfkittest.FakeVPP, *model) {
	f := dfkittest.NewFake(
		dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"},
		dfkittest.Iface{Index: 8, Name: "loop502", Tag: "w5:loop502"},
		dfkittest.Iface{Index: 9, Name: "loop601", Tag: "w6:loop601"},
		dfkittest.Iface{Index: 10, Name: "ens192"},
	)
	m := &model{g: DefaultGlobal(), hwOf: map[uint32]uint32{7: 3, 8: 4, 9: 5, 10: 6}, enabled: map[uint32]bool{}}
	f.On("sflow_enable_disable", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*sflow.SflowEnableDisable)
		sw := uint32(r.HwIfIndex)
		if m.enabled[sw] == r.EnableDisable {
			return []api.Message{&sflow.SflowEnableDisableReply{Retval: int32(api.VALUE_EXIST)}}, nil
		}
		m.enabled[sw] = r.EnableDisable
		m.toggles++
		return []api.Message{&sflow.SflowEnableDisableReply{}}, nil
	})
	f.On("sflow_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for sw, on := range m.enabled {
			if on {
				out = append(out, &sflow.SflowInterfaceDetails{HwIfIndex: interface_types.InterfaceIndex(m.hwOf[sw])})
			}
		}
		return out, nil
	})
	f.On("sflow_sampling_rate_set", func(msg api.Message) ([]api.Message, error) {
		m.g.SamplingRate = msg.(*sflow.SflowSamplingRateSet).SamplingN
		return []api.Message{&sflow.SflowSamplingRateSetReply{}}, nil
	})
	f.On("sflow_polling_interval_set", func(msg api.Message) ([]api.Message, error) {
		m.g.PollingInterval = msg.(*sflow.SflowPollingIntervalSet).PollingS
		return []api.Message{&sflow.SflowPollingIntervalSetReply{}}, nil
	})
	f.On("sflow_header_bytes_set", func(msg api.Message) ([]api.Message, error) {
		m.g.HeaderBytes = msg.(*sflow.SflowHeaderBytesSet).HeaderB
		return []api.Message{&sflow.SflowHeaderBytesSetReply{}}, nil
	})
	f.On("sflow_direction_set", func(msg api.Message) ([]api.Message, error) {
		m.g.Direction = dirFromAPI[msg.(*sflow.SflowDirectionSet).SamplingD]
		return []api.Message{&sflow.SflowDirectionSetReply{}}, nil
	})
	f.On("sflow_drop_monitoring_set", func(msg api.Message) ([]api.Message, error) {
		m.g.DropMonitoring = msg.(*sflow.SflowDropMonitoringSet).DropM == 1
		return []api.Message{&sflow.SflowDropMonitoringSetReply{}}, nil
	})
	f.On("sflow_sampling_rate_get", func(api.Message) ([]api.Message, error) {
		return []api.Message{&sflow.SflowSamplingRateGetReply{SamplingN: m.g.SamplingRate}}, nil
	})
	f.On("sflow_polling_interval_get", func(api.Message) ([]api.Message, error) {
		return []api.Message{&sflow.SflowPollingIntervalGetReply{PollingS: m.g.PollingInterval}}, nil
	})
	f.On("sflow_header_bytes_get", func(api.Message) ([]api.Message, error) {
		return []api.Message{&sflow.SflowHeaderBytesGetReply{HeaderB: m.g.HeaderBytes}}, nil
	})
	f.On("sflow_direction_get", func(api.Message) ([]api.Message, error) {
		return []api.Message{&sflow.SflowDirectionGetReply{SamplingD: dirToAPI[m.g.Direction]}}, nil
	})
	f.On("sflow_drop_monitoring_get", func(api.Message) ([]api.Message, error) {
		var v uint32
		if m.g.DropMonitoring {
			v = 1
		}
		return []api.Message{&sflow.SflowDropMonitoringGetReply{DropM: v}}, nil
	})
	return f, m
}

func TestGlobal(t *testing.T) {
	f, m := newFake()
	d := NewGlobal(f, WithGlobals(dfkit.GlobalsOwner(true)))
	ctx := context.Background()
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("defaults reported: %v", kvs)
	}
	v := Global{SamplingRate: 500, PollingInterval: 10, HeaderBytes: 256, Direction: "tx", DropMonitoring: true}.Proto()
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	v2 := Global{SamplingRate: 0, PollingInterval: 10, HeaderBytes: 64, Direction: "both"}.Proto()
	if _, err := d.Update(ctx, v, v2, nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v2))
	// D-071: a non-owner only requires the globals
	other := NewGlobal(f)
	if _, err := other.Create(ctx, v2); err != nil {
		t.Fatalf("requirement met: %v", err)
	}
	if _, err := other.Create(ctx, v); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("requirement not met: %v", err)
	}
	if err := other.Delete(ctx, v2, nil); err != nil || m.g.Direction != "both" {
		t.Fatalf("non-owner must never reset: %v %+v", err, m.g)
	}
	if err := d.Delete(ctx, v2, nil); err != nil {
		t.Fatal(err)
	}
	if m.g != DefaultGlobal() {
		t.Fatalf("delete left %+v", m.g)
	}
	for _, bad := range []Global{{HeaderBytes: 100, Direction: "rx"}, {HeaderBytes: 32, Direction: "rx"}, {HeaderBytes: 128, Direction: "in"}} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func enables(f *dfkittest.FakeVPP) int { return len(f.CallsNamed("sflow_enable_disable")) }

func TestInterfaceLearnReadOnlyRetrieve(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	d := NewInterface(f, "w5")
	v := Interface{Interface: "loop501"}.Proto()
	if deps := d.Dependencies(v); len(deps) != 2 || deps[0].Key != "interface/loop501" || deps[1].Key != KeyGlobal || !deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, v)
	if err != nil || meta != (InterfaceMeta{SwIfIndex: 7, HwIfIndex: 3}) {
		t.Fatalf("create %v %v", meta, err)
	}
	req := f.CallsNamed("sflow_enable_disable")[0].(*sflow.SflowEnableDisable)
	if uint32(req.HwIfIndex) != 7 || !req.EnableDisable {
		t.Fatalf("request must carry the sw_if_index: %+v", req)
	}
	if _, err := d.Create(ctx, v); err != nil { // VALUE_EXIST on our tagged interface = success
		t.Fatal(err)
	}
	// M2: Retrieve never writes, also with another owner's unknown enabled index present
	m.enabled[9] = true
	n := enables(f)
	got := dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	if got.Meta != meta || enables(f) != n {
		t.Fatalf("retrieve wrote the data plane or lost the mapping: %v, %d→%d", got.Meta, n, enables(f))
	}

	// agent restart: a fresh descriptor knows nothing and reports nothing (read-only); the
	// scheduler's Create finds VALUE_EXIST on our interface and learns by toggling only it
	fresh := NewInterface(f, "w5")
	if kvs := dfkittest.MustRetrieve(t, fresh); len(kvs) != 0 || enables(f) != n {
		t.Fatalf("fresh retrieve %v, enables %d→%d", kvs, n, enables(f))
	}
	meta2, err := fresh.Create(ctx, v)
	if err != nil || meta2 != (InterfaceMeta{SwIfIndex: 7, HwIfIndex: 3}) || !m.enabled[7] {
		t.Fatalf("learn by toggle: %v %v", meta2, err)
	}
	for _, c := range f.CallsNamed("sflow_enable_disable") {
		if uint32(c.(*sflow.SflowEnableDisable).HwIfIndex) == 9 {
			t.Fatal("touched another owner's interface")
		}
	}
	dfkittest.AssertRetrieved(t, fresh, dfkittest.KV(fresh, v))

	// M1: VPP restart — the map is dropped; a foreign interface now owning hw index 3 is not ours
	f.RestartVPP()
	m.enabled = map[uint32]bool{9: true}
	m.hwOf[9] = 3
	if kvs := dfkittest.MustRetrieve(t, fresh); len(kvs) != 0 {
		t.Fatalf("stale hw→sw map survived the VPP restart: %v", kvs)
	}
	m.hwOf[9] = 5
	delete(m.enabled, 9)

	// learning only from an unambiguous dump difference: a concurrent enable (hw 6 of ens192)
	// during our add makes Create fall back to the toggle, which still finds hw 4 for loop502
	v2 := Interface{Interface: "loop502"}.Proto()
	f.On("sflow_enable_disable", concurrentEnabler(m, 8, 10))
	meta3, err := fresh.Create(ctx, v2)
	if err != nil || meta3 != (InterfaceMeta{SwIfIndex: 8, HwIfIndex: 4}) {
		t.Fatalf("concurrent enable: %v %v", meta3, err)
	}
	for range 2 {
		if err := fresh.Delete(ctx, v2, nil); err != nil {
			t.Fatal(err)
		}
	}
	if m.enabled[8] {
		t.Fatal("not disabled")
	}
	if _, err := d.Create(ctx, Interface{Interface: "loop601"}.Proto()); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	RegisterGlobals(r, f)
	Register(r, f, "w5")
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
}

// concurrentEnabler behaves like VPP's sflow_enable_disable and, the first time sw is enabled,
// also enables other (someone else's concurrent change).
func concurrentEnabler(m *model, sw, other uint32) func(api.Message) ([]api.Message, error) {
	done := false
	return func(msg api.Message) ([]api.Message, error) {
		r := msg.(*sflow.SflowEnableDisable)
		idx := uint32(r.HwIfIndex)
		if m.enabled[idx] == r.EnableDisable {
			return []api.Message{&sflow.SflowEnableDisableReply{Retval: int32(api.VALUE_EXIST)}}, nil
		}
		m.enabled[idx] = r.EnableDisable
		if idx == sw && r.EnableDisable && !done {
			done = true
			m.enabled[other] = true
		}
		return []api.Message{&sflow.SflowEnableDisableReply{}}, nil
	}
}

// H1: an sFlow enable found on an untagged NIC without our claim is foreign: Create fails, nothing
// is claimed, Retrieve does not report it and Delete leaves it alone.
func TestInterfaceForeignOnUntagged(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	d := NewInterface(f, "w5h1")
	uv := Interface{Interface: "ens192"}.Proto()
	m.enabled[10] = true // someone else's
	if _, err := d.Create(ctx, uv); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("foreign enable adopted: %v", err)
	}
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("foreign reported: %v", kvs)
	}
	if err := d.Delete(ctx, uv, nil); err != nil || !m.enabled[10] {
		t.Fatalf("foreign enable deleted: %v", err)
	}
	// our own enable on an untagged NIC: claimed after the add, adopted after an agent restart
	delete(m.enabled, 10)
	if _, err := d.Create(ctx, uv); err != nil {
		t.Fatal(err)
	}
	fresh := NewInterface(f, "w5h1")
	if _, err := fresh.Create(ctx, uv); err != nil { // VALUE_EXIST + our claim → adopt, learn
		t.Fatalf("own claimed enable: %v", err)
	}
	dfkittest.AssertRetrieved(t, fresh, dfkittest.KV(fresh, uv))
	if err := fresh.Delete(ctx, uv, nil); err != nil || m.enabled[10] {
		t.Fatalf("own delete: %v", err)
	}
}
