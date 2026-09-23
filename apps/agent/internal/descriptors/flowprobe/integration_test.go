package flowprobe

import (
	"context"
	"testing"

	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// Integration test against the host VPP. flowprobe.params is a VPP-global: read first, skipped
// when someone else has set record flags, reset to "unset" in Cleanup. Flowprobe runs only on
// this slot's tagged loopback.
func TestFlowprobeOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, "flowprobe", &flowprobe.FlowprobeSetParams{}, &flowprobe.FlowprobeGetParams{},
		&flowprobe.FlowprobeInterfaceAddDel{}, &flowprobe.FlowprobeInterfaceDump{})
	c := h.Client()
	ctx := context.Background()
	pd := NewParams(c, WithGlobals(dfkit.GlobalsOwner(true))) // test acts as globals owner, restores "unset"
	if kvs := dfkittest.MustRetrieve(t, pd); len(kvs) != 0 {
		t.Skipf("flowprobe params are set by someone else (%v): not touching a global", kvs[0].Value)
	}
	ifName, _ := h.Loopback(t, 83)
	id := NewInterface(c, h.Owner)
	pv := Params{RecordL3: true, RecordL4: true, ActiveTimer: 10, PassiveTimer: 60}.Proto()
	iv := Interface{Interface: ifName, Which: "ip4", Direction: "both"}.Proto()
	t.Cleanup(func() {
		_ = id.Delete(context.Background(), iv, nil)
		_ = pd.Delete(context.Background(), pv, nil)
	})
	if _, err := pd.Create(ctx, pv); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, pd, dfkittest.KV(pd, pv))
	meta, err := id.Create(ctx, iv)
	if err != nil {
		t.Fatal(err)
	}
	got := dfkittest.AssertRetrieved(t, id, dfkittest.KV(id, iv))
	if got.Meta != meta {
		t.Fatalf("meta %v vs %v", got.Meta, meta)
	}
	dfkittest.AssertEmptyPlan(t, pd, dfkittest.KV(pd, pv))
	dfkittest.AssertEmptyPlan(t, id, dfkittest.KV(id, iv))
	if _, err := id.Create(ctx, iv); err != nil { // re-apply: ENTRY_ALREADY_EXISTS + identical → ok
		t.Fatalf("re-apply: %v", err)
	}
	// VPP refuses param changes while an interface is enabled: the descriptor recreates instead
	if _, err := pd.Create(ctx, Params{RecordL2: true, ActiveTimer: 5, PassiveTimer: 50}.Proto()); err == nil {
		t.Fatal("set_params with an enabled interface must fail")
	} else {
		t.Logf("expected: %v", err)
	}
	dfkittest.HoldForEvidence(t, "CLI: show flowprobe params / show flowprobe feature")
	if err := id.Delete(ctx, iv, meta); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, id, id.KeyOf(iv))
	if err := pd.Delete(ctx, pv, nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, pd, KeyParams)
}
