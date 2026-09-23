package pcap

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

func newFake() (*dfkittest.FakeVPP, *bool, *string) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"}, dfkittest.Iface{Index: 9, Name: "ens192"})
	running := false
	fn := DefaultFilterFunction
	f.On("pcap_trace_on", func(api.Message) ([]api.Message, error) {
		if running {
			return []api.Message{&interfaces.PcapTraceOnReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		running = true
		return []api.Message{&interfaces.PcapTraceOnReply{}}, nil
	})
	f.On("pcap_trace_off", func(api.Message) ([]api.Message, error) {
		if !running {
			return []api.Message{&interfaces.PcapTraceOffReply{Retval: int32(api.VALUE_EXIST)}}, nil
		}
		running = false
		return []api.Message{&interfaces.PcapTraceOffReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil // no packets captured
	})
	f.On("pcap_set_filter_function", func(msg api.Message) ([]api.Message, error) {
		name := msg.(*interfaces.PcapSetFilterFunction).FilterFunctionName
		if name != DefaultFilterFunction && name != "bpf_trace_filter" {
			return []api.Message{&interfaces.PcapSetFilterFunctionReply{Retval: -1}}, nil
		}
		fn = name
		return []api.Message{&interfaces.PcapSetFilterFunctionReply{}}, nil
	})
	return f, &running, &fn
}

func TestCapture(t *testing.T) {
	f, running, _ := newFake()
	ctx := context.Background()
	d := NewCapture(f, "w5")
	v := Capture{Rx: true, Drop: true, Interface: "loop501", MaxPackets: 100, MaxBytesPerPacket: 256, Filter: true, Error: "ip4-input/ttl_expired", File: "w5-cap.pcap"}.Proto()
	deps := d.Dependencies(v)
	if len(deps) != 2 || deps[0].Key != KeyFilterFunction || !deps[0].Optional || deps[1].Key != "interface/loop501" {
		t.Fatalf("deps %+v", deps)
	}
	if deps := d.Dependencies(Capture{Interface: AnyInterface}.Proto()); len(deps) != 1 {
		t.Fatalf("any: deps %+v", deps)
	}
	for range 2 {
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("pcap_trace_on")[0].(*interfaces.PcapTraceOn)
	if !req.CaptureRx || req.CaptureTx || !req.CaptureDrop || req.SwIfIndex != 7 || req.MaxPackets != 100 || req.MaxBytesPerPacket != 256 ||
		!req.Filter || req.Error != "ip4-input/ttl_expired" || req.Filename != "w5-cap.pcap" {
		t.Fatalf("request %+v", req)
	}
	other := NewCapture(f, "w5")
	if _, err := other.Create(ctx, v); !errors.Is(err, ErrCaptureBusy) {
		t.Fatalf("busy: %v", err)
	}
	if err := other.Delete(ctx, v, nil); err != nil || !*running {
		t.Fatalf("foreign delete stopped the capture: %v", err)
	}
	// a different capture from the same descriptor while running is busy too
	if _, err := d.Create(ctx, Capture{Tx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "w5-x.pcap"}.Proto()); !errors.Is(err, ErrCaptureBusy) {
		t.Fatalf("changed capture: %v", err)
	}
	if _, err := d.Update(ctx, v, v, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
	}
	if *running || len(f.CallsNamed("pcap_trace_off")) != 1 {
		t.Fatalf("running=%t offs=%d", *running, len(f.CallsNamed("pcap_trace_off")))
	}
	// "any" is sw_if_index 0 (VPP's wildcard, not local0)
	anyCap := Capture{Tx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "w5-any.pcap"}.Proto()
	if _, err := d.Create(ctx, anyCap); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("pcap_trace_on")
	if calls[len(calls)-1].(*interfaces.PcapTraceOn).SwIfIndex != 0 {
		t.Fatal("any must be sw_if_index 0")
	}
	if _, err := NewCapture(f, "w5").Create(ctx, Capture{Rx: true, Interface: "ens192", MaxPackets: 1, MaxBytesPerPacket: 64, File: "w5.pcap"}.Proto()); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatalf("unowned interface: %v", err)
	}
	for _, bad := range []Capture{
		{Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "a.pcap"},
		{Rx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "../etc/passwd"},
		{Rx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "a b.pcap"},
		{Rx: true, Interface: AnyInterface, MaxPackets: 0, MaxBytesPerPacket: 64, File: "a.pcap"},
		{Rx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 16, File: "a.pcap"},
		{Rx: true, Interface: "", MaxPackets: 1, MaxBytesPerPacket: 64, File: "a.pcap"},
		{Rx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: "a.pcap", Error: "x;y"},
	} {
		if err := bad.Validate(); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestFilterFunction(t *testing.T) {
	f, _, fn := newFake()
	d := NewFilterFunction(f)
	ctx := context.Background()
	v := FilterFunction{Name: "bpf_trace_filter"}.Proto()
	if deps := d.Dependencies(v); len(deps) != 1 || deps[0].Key != "trace.bpf-filter/global" {
		t.Fatalf("deps %+v", deps)
	}
	if _, err := d.Create(ctx, v); err != nil || *fn != "bpf_trace_filter" {
		t.Fatal(err)
	}
	if _, err := d.Update(ctx, v, FilterFunction{Name: "nope"}.Proto(), nil); !dfkit.IsVPPError(err, api.UNSPECIFIED) {
		t.Fatalf("unknown function: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v, nil); err != nil || *fn != DefaultFilterFunction {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, FilterFunction{Name: "a b"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, "w5")
	if names := r.Names(); len(names) != 2 || names[0] != NameFilterFunction {
		t.Fatal(names)
	}
}
