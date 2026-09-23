package trace

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ngfw/agent/binapi/bpf_trace_filter"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the host VPP. The BPF program is a getter-less VPP-global; nobody
// else on this host sets it (checked with `show bpf trace filter` before the first run); the
// test removes it in Cleanup.
func TestBPFFilterOnHost(t *testing.T) {
	dfkittest.SkipUnlessGlobals(t, "bpf_trace_filter_set_v2")
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, Plugin, &bpf_trace_filter.BpfTraceFilterSetV2{})
	c := h.Client()
	ctx := context.Background()
	d := NewBPFFilter(c, dfkit.GlobalsOwner(true)) // test acts as globals owner; nobody else sets it
	v := BPFFilter{Expression: fmt.Sprintf("udp port 4739 and net 10.%d.0.0/16", vpptest.Slot(t)), Optimize: true}.Proto()
	t.Cleanup(func() { _ = d.Delete(context.Background(), v, nil) })
	for range 2 {
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	v2 := BPFFilter{Expression: "tcp and (port 179 or port 22)"}.Proto()
	if _, err := d.Update(ctx, v, v2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	dfkittest.HoldForEvidence(t, "CLI: show bpf trace filter")
	// libpcap rejects it inside VPP: the error surfaces, nothing crashes. VPP frees the old
	// program before compiling, so a failed Create leaves no filter (documented in trace.md).
	if _, err := d.Create(ctx, BPFFilter{Expression: "udp port port"}.Proto()); err == nil {
		t.Fatal("uncompilable expression accepted")
	} else {
		t.Logf("expected compile error: %v", err)
	}
	for range 2 {
		if err := d.Delete(ctx, v2, nil); err != nil {
			t.Fatal(err)
		}
	}
}
