package tapv2_test

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestTapSanitizesReusedIndex (D-095 a): inherited state on the new tap's sw_if_index is
// cleared before Create returns; a tap that cannot be made safe is deleted again.
func TestTapSanitizesReusedIndex(t *testing.T) {
	f := newFake()
	m := sanitizetest.NewModel()
	m.Install(f.Client)
	m.Poison(1, 2, 3, 4, 5)
	d := tapv2.New(f, owner)
	meta, err := d.Create(ctx, &tapv2.Tap{Name: "w2-tap0", Id: 200, HostIfName: "w2-tap0", RxRingSize: 256, TxRingSize: 256})
	if err != nil {
		t.Fatal(err)
	}
	if dirty := m.Dirty(meta.(iface.Meta).SwIfIndex); dirty != "" {
		t.Fatalf("new tap still has inherited %s", dirty)
	}
	f.Client.Fail("classify_set_interface_ip_table", errRefused) // VPP refuses the reset: the interface must not be reported created
	_, err = d.Create(ctx, &tapv2.Tap{Name: "w2-tap1", Id: 201, HostIfName: "w2-tap1", RxRingSize: 256, TxRingSize: 256})
	if !errors.Is(err, errRefused) {
		t.Fatalf("err = %v", err)
	}
	if len(f.CallsNamed("tap_delete_v2")) != 1 {
		t.Fatal("the unsafe tap was not deleted")
	}
	for _, tp := range f.taps {
		if tp.ID == 201 {
			t.Fatal("tap 201 left behind")
		}
	}
}

var errRefused = errors.New("vpp refused")
