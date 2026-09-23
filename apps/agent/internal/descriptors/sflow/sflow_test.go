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

func TestInterfaceLearnAndProbe(t *testing.T) {
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
	if _, err := d.Create(ctx, v); err != nil { // VALUE_EXIST = success
		t.Fatal(err)
	}
	toggles := m.toggles
	got := dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	if got.Meta != meta || m.toggles != toggles {
		t.Fatalf("learned retrieve must not probe: meta %v toggles %d→%d", got.Meta, toggles, m.toggles)
	}
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))

	// agent restart + another owner's interface enabled: the fresh descriptor probes loop501 and
	// loop502 (ours), never loop601; loop502 is turned on and straight off again
	m.enabled[9] = true
	fresh := NewInterface(f, "w5")
	kvs := dfkittest.MustRetrieve(t, fresh)
	if len(kvs) != 1 || kvs[0].Key != "sflow.interface/loop501" || m.enabled[8] || !m.enabled[9] {
		t.Fatalf("probe result %v, state %v", kvs, m.enabled)
	}
	for _, c := range f.CallsNamed("sflow_enable_disable") {
		if uint32(c.(*sflow.SflowEnableDisable).HwIfIndex) == 9 {
			t.Fatal("probed another owner's interface")
		}
	}
	// two unknown hw indexes (3 and 5), one found → ambiguous, not learned: meta has no hw
	if kvs[0].Meta.(InterfaceMeta).HwIfIndex != 0 {
		t.Fatalf("ambiguous mapping learned: %v", kvs[0].Meta)
	}
	for range 2 {
		if err := fresh.Delete(ctx, v, kvs[0].Meta); err != nil {
			t.Fatal(err)
		}
	}
	if m.enabled[7] {
		t.Fatal("not disabled")
	}
	delete(m.enabled, 9)
	if kvs := dfkittest.MustRetrieve(t, fresh); len(kvs) != 0 {
		t.Fatalf("after delete %v", kvs)
	}
	// unambiguous probe learns the mapping
	m.enabled[8] = true
	kvs = dfkittest.MustRetrieve(t, NewInterface(f, "w5"))
	if len(kvs) != 1 || kvs[0].Meta != (InterfaceMeta{SwIfIndex: 8, HwIfIndex: 4}) {
		t.Fatalf("unambiguous probe %v", kvs)
	}
	if _, err := d.Create(ctx, Interface{Interface: "loop601"}.Proto()); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatal(err)
	}
	// an untagged NIC is probed only while claimed (D-071): never another owner's or unclaimed one
	delete(m.enabled, 8)
	uv := Interface{Interface: "ens192"}.Proto()
	if _, err := d.Create(ctx, uv); err != nil {
		t.Fatal(err)
	}
	if kvs := dfkittest.MustRetrieve(t, NewInterface(f, "w5")); len(kvs) != 1 || kvs[0].Key != "sflow.interface/ens192" {
		t.Fatalf("claimed untagged NIC after restart: %v", kvs)
	}
	for _, c := range f.CallsNamed("sflow_enable_disable") {
		if r := c.(*sflow.SflowEnableDisable); uint32(r.HwIfIndex) == 10 && r.EnableDisable && dfkit.Claims("w7").Claimed("ens192", NameInterface) {
			t.Fatal("unexpected claim")
		}
	}
	if err := d.Delete(ctx, uv, nil); err != nil || m.enabled[10] || dfkit.Claims("w5").Claimed("ens192", NameInterface) {
		t.Fatalf("untagged delete: %v", err)
	}
	r := scheduler.NewRegistry()
	RegisterGlobals(r, f)
	Register(r, f, "w5")
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
}
