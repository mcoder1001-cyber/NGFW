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
// The filter function is a getter-less VPP-global: its host check is opt-in (VRX_DF8_GLOBALS=1,
// manager window, review M3).
func TestPcapOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "vnet interface (pcap)", &interfaces.PcapTraceOn{}, &interfaces.PcapTraceOff{}, &interfaces.PcapSetFilterFunction{})
	c := h.Client()
	ctx := context.Background()
	ifName, _ := h.Loopback(t, 87)
	file := h.Owner + "-df8.pcap"
	t.Cleanup(func() { _ = os.Remove(filepath.Join(FileDir, file)) })
	bootPath := filepath.Join(t.TempDir(), "boot.json")
	boot, err := dfkit.NewFileBootStore(bootPath)
	if err != nil {
		t.Fatal(err)
	}

	d := NewCapture(c, h.Owner, boot)
	v := Capture{Rx: true, Tx: true, Interface: ifName, MaxPackets: 10, MaxBytesPerPacket: 128, File: file}.Proto()
	t.Cleanup(func() { _ = d.Delete(context.Background(), v, nil) })
	if _, err := d.Create(ctx, v); errors.Is(err, ErrCaptureBusy) {
		t.Skipf("another pcap capture is running on this VPP: %v", err)
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, v); err != nil { // resync: skipped via the boot record (D-076)
		t.Fatalf("re-apply: %v", err)
	}
	// agent restart with the persisted store: the new process recognises its own capture (M4)
	boot2, err := dfkit.NewFileBootStore(bootPath)
	if err != nil {
		t.Fatal(err)
	}
	d2 := NewCapture(c, h.Owner, boot2)
	if _, err := d2.Create(ctx, v); err != nil {
		t.Fatalf("re-apply after agent restart: %v", err)
	}
	t.Log("agent restart with the persisted boot store: own capture recognised, not re-added")
	// another owner (slot) must be refused, not take over; its Delete never stops our capture
	other := NewCapture(c, h.Owner+"x", dfkit.NewMemoryBootStore())
	vOther := Capture{Rx: true, Interface: AnyInterface, MaxPackets: 1, MaxBytesPerPacket: 64, File: h.Owner + "x-df8.pcap"}.Proto()
	if _, err := other.Create(ctx, vOther); !errors.Is(err, ErrCaptureBusy) {
		t.Fatalf("second capture: %v", err)
	}
	if err := other.Delete(ctx, vOther, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	dfkittest.HoldForEvidence(t, "CLI: pcap trace status")
	for range 2 { // the restarted agent stops its own capture
		if err := d2.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d2.Create(ctx, v); err != nil { // the capture is off: it starts again
		t.Fatalf("after delete: %v", err)
	}
	if err := d2.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPcapFilterFunctionOnHost(t *testing.T) {
	dfkittest.SkipUnlessGlobals(t, "pcap_set_filter_function")
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	c := h.Client()
	ctx := context.Background()
	ff := NewFilterFunction(c, WithGlobals(dfkit.GlobalsOwner(true)))
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
}
