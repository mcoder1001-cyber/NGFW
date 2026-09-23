package pcap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// Integration test against the host VPP. One pcap capture per VPP: when someone else's capture
// is running, Create reports ErrCaptureBusy and the test skips. The capture runs on this slot's
// tagged loopback only; the file is /tmp/<prefix>-df8.pcap (VPP forces /tmp) and is removed.
// The filter function is a getter-less global, restored to VPP's default in Cleanup.
func TestPcapOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "vnet interface (pcap)", &interfaces.PcapTraceOn{}, &interfaces.PcapTraceOff{}, &interfaces.PcapSetFilterFunction{})
	c := h.Client()
	ctx := context.Background()
	ifName, _ := h.Loopback(t, 87)
	file := h.Owner + "-df8.pcap"
	t.Cleanup(func() { _ = os.Remove(filepath.Join(FileDir, file)) })

	ff := NewFilterFunction(c)
	fv := FilterFunction{Name: "bpf_trace_filter"}.Proto()
	t.Cleanup(func() { _ = ff.Delete(context.Background(), fv, nil) })
	for range 2 {
		if _, err := ff.Create(ctx, fv); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ff.Create(ctx, FilterFunction{Name: "no_such_filter"}.Proto()); err == nil {
		t.Fatal("unknown filter function accepted")
	}
	if _, err := ff.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}

	d := NewCapture(c, h.Owner)
	v := Capture{Rx: true, Tx: true, Interface: ifName, MaxPackets: 10, MaxBytesPerPacket: 128, File: file}.Proto()
	t.Cleanup(func() { _ = d.Delete(context.Background(), v, nil) })
	if _, err := d.Create(ctx, v); errors.Is(err, ErrCaptureBusy) {
		t.Skipf("another pcap capture is running on this VPP: %v", err)
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, v); err != nil { // re-apply of this process's capture
		t.Fatalf("re-apply: %v", err)
	}
	other := NewCapture(c, h.Owner) // a second agent (or slot) must be refused, not take over
	if _, err := other.Create(ctx, v); !errors.Is(err, ErrCaptureBusy) {
		t.Fatalf("second capture: %v", err)
	}
	if err := other.Delete(ctx, v, nil); err != nil { // never stops a capture it did not start
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatalf("capture must still be ours after the other descriptor's Delete: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	dfkittest.HoldForEvidence(t, "vppctl pcap trace status")
	for range 2 {
		if err := d.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
	}
	// the capture is off: a new one can start (and is stopped again)
	if _, err := other.Create(ctx, v); err != nil {
		t.Fatalf("after delete: %v", err)
	}
	if err := other.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
}
