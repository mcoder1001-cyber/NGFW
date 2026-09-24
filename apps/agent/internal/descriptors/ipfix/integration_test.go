package ipfix

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the host VPP: additional exporters only with collectors in
// 10.<slot>.0.0/16; the default exporter and the classify stream are VPP-globals — read first,
// skipped when another slot holds them, reset to "unset" in Cleanup.
func TestIPFIXOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, "ipfix_export", &ipfix_export.SetIpfixExporter{}, &ipfix_export.IpfixExporterCreateDelete{},
		&ipfix_export.IpfixAllExporterGet{}, &ipfix_export.SetIpfixClassifyStream{}, &ipfix_export.IpfixClassifyTableAddDel{})
	c := h.Client()
	ctx := context.Background()
	slot := vpptest.Slot(t)
	pool := netip.MustParsePrefix(fmt.Sprintf("10.%d.90.0/23", slot)) // restarttest uses 10.<N>.96.0/24
	scope := WithCollectorScope(pool.Contains)
	ip := func(x, y int) string { return fmt.Sprintf("10.%d.%d.%d", slot, x, y) }

	t.Run("exporter", func(t *testing.T) {
		d := NewExporter(c, scope)
		v := Exporter{Collector: ip(90, 1), CollectorPort: 4739, Src: ip(90, 2), VRF: 0, PathMTU: 1400, TemplateInterval: 20}.Proto()
		t.Cleanup(func() { _ = d.Delete(context.Background(), v, nil) })
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
		dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
		v2 := Exporter{Collector: ip(90, 1), CollectorPort: 4740, Src: ip(90, 2), VRF: 0, PathMTU: 1200, TemplateInterval: 30, UDPChecksum: true}.Proto()
		if _, err := d.Update(ctx, v, v2, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v2))
		dfkittest.HoldForEvidence(t, "CLI: show ipfix exporter all")
		if err := d.Delete(ctx, v2, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertAbsent(t, d, d.KeyOf(v2))
	})

	t.Run("default exporter", func(t *testing.T) {
		own := WithGlobals(dfkit.GlobalsOwner(true)) // the test acts as globals owner and restores the unset state
		d := NewDefaultExporter(c, own)
		if cur, ok, err := d.Current(ctx); err != nil {
			t.Fatal(err)
		} else if ok {
			t.Skipf("default exporter is held by someone else (%+v): not touching a global", cur)
		}
		v := Exporter{Collector: ip(91, 1), CollectorPort: 4739, Src: ip(91, 2), VRF: NoVRF, PathMTU: 1400, TemplateInterval: 20}.Proto()
		t.Cleanup(func() { _ = d.Delete(context.Background(), v, nil) })
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
		dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))

		if err := d.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertAbsent(t, d, KeyDefaultExporter)
	})
}

// The classify stream is a getter-less VPP-global (its dump is broken, V16) and VPP refuses
// classify tables before it is set: this host check is opt-in (VRX_DF8_GLOBALS=1, manager window,
// review M3). The table is DF-2's, created by name through classify.TableDescriptor (H2).
func TestIPFIXClassifyOnHost(t *testing.T) {
	dfkittest.SkipUnlessGlobals(t, "set_ipfix_classify_stream")
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	c := h.Client()
	ctx := context.Background()
	slot := vpptest.Slot(t)
	cs := NewClassifyStream(c, WithGlobals(dfkit.GlobalsOwner(true)))
	sv := ClassifyStream{DomainID: uint32(slot), SrcPort: uint16(4739 + slot)}.Proto() //nolint:gosec // slot ≤ 12
	t.Cleanup(func() { _ = cs.Delete(context.Background(), sv, nil) })
	for range 2 {
		if _, err := cs.Create(ctx, sv); err != nil {
			t.Fatal(err)
		}
	}
	store := classify.NewMemStore()
	tdesc := classify.NewTable(c, store)
	mask := make([]byte, 16)
	mask[12] = 0xff
	tbl := &classify.Table{Name: h.Owner + "-ipfix", Mask: mask}
	if _, err := tdesc.Create(ctx, tbl); err != nil {
		t.Fatalf("DF-2 classify.table: %v", err)
	}
	t.Cleanup(func() { _ = tdesc.Delete(context.Background(), tbl, nil) })
	ct := NewClassifyTable(c, store)
	tv := ClassifyTable{Table: tbl.Name, IPVersion: "ip4", Protocol: 17}.Proto()
	t.Cleanup(func() { _ = ct.Delete(context.Background(), tv, nil) })
	for range 2 { // re-apply: VALUE_EXIST is success
		if _, err := ct.Create(ctx, tv); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := ct.Delete(ctx, tv, nil); err != nil {
			t.Fatal(err)
		}
	}
}
