package trace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/bpf_trace_filter"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

func TestBPFFilter(t *testing.T) {
	f := dfkittest.NewFake()
	prog := ""
	f.On("bpf_trace_filter_set_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*bpf_trace_filter.BpfTraceFilterSetV2)
		switch {
		case !r.IsAdd:
			prog = ""
		case strings.Contains(r.Filter, "port port"): // libpcap compile error
			return []api.Message{&bpf_trace_filter.BpfTraceFilterSetV2Reply{Retval: -1}}, nil
		default:
			prog = r.Filter
		}
		return []api.Message{&bpf_trace_filter.BpfTraceFilterSetV2Reply{}}, nil
	})
	ctx := context.Background()
	d := NewBPFFilter(f, dfkit.GlobalsOwner(true))
	if _, err := NewBPFFilter(f, dfkit.GlobalsOwner(false)).Create(context.Background(), BPFFilter{Expression: "udp"}.Proto()); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner: %v", err)
	}
	if err := NewBPFFilter(f, dfkit.GlobalsOwner(false)).Delete(context.Background(), BPFFilter{Expression: "udp"}.Proto(), nil); err != nil || len(f.Calls()) != 0 {
		t.Fatalf("non-owner must never send: %v %d", err, len(f.Calls()))
	}
	v := BPFFilter{Expression: "udp port 4739 and net 10.5.0.0/16", Optimize: true}.Proto()
	if d.KeyOf(v) != KeyBPFFilter || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	for range 2 {
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("bpf_trace_filter_set_v2")[0].(*bpf_trace_filter.BpfTraceFilterSetV2)
	if !req.IsAdd || !req.Optimize || req.Filter != "udp port 4739 and net 10.5.0.0/16" {
		t.Fatalf("request %+v", req)
	}
	if _, err := d.Update(ctx, v, BPFFilter{Expression: "tcp and (port 179 or port 22)"}.Proto(), nil); err != nil || prog != "tcp and (port 179 or port 22)" {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, BPFFilter{Expression: "udp port port"}.Proto()); !dfkit.IsVPPError(err, api.UNSPECIFIED) {
		t.Fatalf("compile error: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.Delete(ctx, v, nil); err != nil || prog != "" {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"", "host 1.2.3.4; rm -rf /", "`id`", "$(id)", "port '22'", "a\\b", strings.Repeat("x", MaxExpressionLen+1)} {
		if _, err := d.Create(ctx, BPFFilter{Expression: bad}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	f.Fail("bpf_trace_filter_set_v2", &adapter.UnknownMsgError{MsgName: "bpf_trace_filter_set_v2"})
	if _, err := d.Create(ctx, v); !errors.Is(err, dfkit.ErrPluginNotLoaded) {
		t.Fatalf("plugin not loaded: %v", err)
	}
	r := scheduler.NewRegistry()
	RegisterGlobals(r, f)
	if r.Len() != 1 {
		t.Fatal(r.Names())
	}
}
