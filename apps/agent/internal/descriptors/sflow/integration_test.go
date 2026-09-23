package sflow

import (
	"context"
	"testing"

	"ngfw/agent/binapi/sflow"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// Integration test against the host VPP. sflow.global is a VPP-global: read first, skipped when
// not at VPP's defaults (someone else holds it), restored to the defaults in Cleanup. sFlow runs
// only on this slot's tagged loopbacks.
func TestSflowOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "sflow", &sflow.SflowEnableDisable{}, &sflow.SflowInterfaceDump{}, &sflow.SflowSamplingRateSet{},
		&sflow.SflowSamplingRateGet{}, &sflow.SflowDirectionGet{}, &sflow.SflowDropMonitoringGet{})
	c := h.Client()
	ctx := context.Background()
	gd := NewGlobal(c, WithGlobals(dfkit.GlobalsOwner(true))) // test acts as globals owner, restores the defaults it found
	if kvs := dfkittest.MustRetrieve(t, gd); len(kvs) != 0 {
		t.Skipf("sflow globals are not at VPP defaults (%v): held by someone else", kvs[0].Value)
	}
	if1, _ := h.Loopback(t, 84)
	if2, _ := h.Loopback(t, 85)
	id := NewInterface(c, h.Owner)
	gv := Global{SamplingRate: 1000, PollingInterval: 30, HeaderBytes: 192, Direction: "both", DropMonitoring: true}.Proto()
	v1 := Interface{Interface: if1}.Proto()
	v2 := Interface{Interface: if2}.Proto()
	t.Cleanup(func() {
		_ = id.Delete(context.Background(), v1, nil)
		_ = id.Delete(context.Background(), v2, nil)
		_ = gd.Delete(context.Background(), gv, nil)
	})
	if _, err := gd.Create(ctx, gv); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, gd, dfkittest.KV(gd, gv))
	dfkittest.AssertEmptyPlan(t, gd, dfkittest.KV(gd, gv))
	m1, err := id.Create(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s meta %+v", if1, m1)
	if _, err := id.Create(ctx, v1); err != nil { // re-apply: VALUE_EXIST = success
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, id, dfkittest.KV(id, v1))
	dfkittest.AssertAbsent(t, id, id.KeyOf(v2))
	dfkittest.AssertEmptyPlan(t, id, dfkittest.KV(id, v1))

	// agent restart: a fresh descriptor has learned nothing and finds if1 by probing; if2 (off)
	// is probed and left off
	fresh := NewInterface(c, h.Owner)
	got := dfkittest.AssertRetrieved(t, fresh, dfkittest.KV(fresh, v1))
	t.Logf("fresh descriptor (probe) meta %+v", got.Meta)
	dfkittest.AssertAbsent(t, fresh, fresh.KeyOf(v2))
	dfkittest.HoldForEvidence(t, "vppctl show sflow")
	if err := fresh.Delete(ctx, v1, got.Meta); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, id, id.KeyOf(v1))
	if err := gd.Delete(ctx, gv, nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertAbsent(t, gd, KeyGlobal)
}
