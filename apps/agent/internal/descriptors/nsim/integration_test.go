package nsim_test

import (
	"os"
	"testing"

	nsimapi "ngfw/agent/binapi/nsim"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/nsim"
)

// Host test, OPT-IN: VRX_INTEGRATION=1 and VRX_NSIM_HOST=1, globals lock exclusive (D-082). nsim_configure2 is
// VPP-global and has no getter, so the previous model cannot be restored (shared-host-rules §7): run it only in a
// manager VPP window. The default-gate evidence for nsim is the fake client (nsim_test.go, agent-level tests).
// It configures a small model (1 ms, 10 Mbit/s: a 2-slot wheel), cross-connects two of this slot's loopbacks and
// enables the output feature on a third, re-applies everything (no stacking), and removes the cross-connect and the
// output feature again (VPP cannot unconfigure the model itself). No packets are sent.
func TestNsimOnHost(t *testing.T) {
	if os.Getenv("VRX_NSIM_HOST") != "1" {
		t.Skip("nsim_configure2 is a getter-less VPP-global: set VRX_NSIM_HOST=1 in a manager VPP window (D-082)")
	}
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "nsim", &nsimapi.NsimConfigure2{}, &nsimapi.NsimConfigure2Reply{},
		&nsimapi.NsimCrossConnectEnableDisable{}, &nsimapi.NsimOutputFeatureEnableDisable{})
	h.LockGlobals(t)
	c := h.Client()
	a, _ := h.Loopback(t, 70)
	b, _ := h.Loopback(t, 71)
	o, _ := h.Loopback(t, 72)
	store := dfkit.NewMemoryBootStore()
	cfg, xc, out := nsim.NewConfig(c, store), nsim.NewCrossConnect(c, h.Owner, store), nsim.NewOutput(c, h.Owner, store)
	model := nsim.Config{DelayUsec: 1000, BandwidthBps: 1e7, PacketSize: 1500}.Proto()
	pair := nsim.CrossConnect{A: a, B: b}.Proto()
	port := nsim.Output{Interface: o}.Proto()
	var xm, om any
	for i := 0; i < 2; i++ {
		if _, err := cfg.Create(ctx, model); err != nil {
			t.Fatalf("nsim.config #%d: %v", i+1, err)
		}
		var err error
		if xm, err = xc.Create(ctx, pair); err != nil {
			t.Fatalf("nsim.cross-connect #%d: %v", i+1, err)
		}
		if om, err = out.Create(ctx, port); err != nil {
			t.Fatalf("nsim.output #%d: %v", i+1, err)
		}
	}
	dfkittest.HoldForEvidence(t, "nsim cross-connect "+a+"/"+b+", output "+o)
	if err := out.Delete(ctx, port, om); err != nil {
		t.Fatal(err)
	}
	if err := xc.Delete(ctx, pair, xm); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Delete(ctx, model, nil); err != nil {
		t.Fatal(err)
	}
	t.Log("nsim applied once per value and removed (the model stays configured: VPP cannot unconfigure it)")
}
