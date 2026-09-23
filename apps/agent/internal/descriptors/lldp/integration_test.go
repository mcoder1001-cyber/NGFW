package lldp

import (
	"errors"
	"os"
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
)

// Host test. lldp.global is a VPP-wide singleton with no getter: by default the test applies
// VPP's own defaults (tx hold 4, interval 30, system name unchanged), which changes nothing;
// VRX_DF7_LLDP_GLOBAL=1 also sets the system name "<prefix>-vrx-a" (it cannot be unset
// afterwards — only for an operator who owns the host's LLDP).
//
// lldp.interface needs a loopback whose sw_if_index equals its hw_if_index (VPP uses one as the
// other); df7test.AlignedLoopback finds one in this slot's range (loop<slot>50…) or the
// subtest skips with the reason.
func TestLLDPOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	gd := NewGlobal(h.C, h.Owner)

	t.Run("global (write-only)", func(t *testing.T) {
		g := Global{TxHold: DefaultTxHold, TxInterval: DefaultTxInterval}
		if os.Getenv("VRX_DF7_LLDP_GLOBAL") == "1" {
			g.SystemName = h.Owner + "-vrx-a"
		}
		h.Apply(gd, df7test.Desired(gd, df7.Encode(g)))
		h.Must("delete (restore defaults)", gd.Delete(h.Ctx, df7.Encode(g), nil))
		if _, err := gd.Retrieve(h.Ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
			t.Fatalf("Retrieve: %v", err)
		}
	})

	t.Run("interface (write-only, verified via lldp_dump)", func(t *testing.T) {
		name, idx, ok := h.AlignedLoopback(50, 12)
		if !ok {
			t.Skip("skip: no loopback with sw_if_index == hw_if_index in this slot's range (VPP 26.06 sw_interface_set_lldp uses the sw index as a hw index)")
		}
		d := NewInterface(h.C, h.Owner)
		v := df7.Encode(Interface{Interface: name, PortDesc: h.Owner + " uplink", MgmtIP4: h.Addr(50, 1), MgmtOID: "1.3.6.1.4.1.8072"})
		kv := h.Apply(d, df7test.Desired(d, v))
		h.Apply(d, df7test.Desired(d, v)) // idempotent re-apply
		t.Cleanup(func() { h.DeleteAll(d, kv) })
		n, err := Neighbours(h.Ctx, h.C)
		h.Must("lldp_dump", err)
		if _, ok := n[idx]; !ok {
			t.Fatalf("lldp_dump does not list %s (%d): %v", name, idx, n)
		}
		v2 := df7.Encode(Interface{Interface: name, PortDesc: h.Owner + " uplink 2"})
		m, err := d.Update(h.Ctx, v, v2, kv[0].Meta)
		h.Must("update", err)
		kv[0].Value, kv[0].Meta = v2, m
		h.Hold("lldp on " + name)
		h.DeleteAll(d, kv)
		kv = nil
		n, err = Neighbours(h.Ctx, h.C)
		h.Must("lldp_dump", err)
		if _, ok := n[idx]; ok {
			t.Fatalf("lldp still enabled on %s after delete", name)
		}
		t.Logf("lldp_dump after delete: %s not listed", name)
	})
}
