package ipfix

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the host VPP: additional exporters only with collectors in
// 10.<slot>.0.0/16; the default exporter and the classify stream are VPP-globals — read first,
// skipped when another slot holds them, reset to "unset" in Cleanup.
func TestIPFIXOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "ipfix_export", &ipfix_export.SetIpfixExporter{}, &ipfix_export.IpfixExporterCreateDelete{},
		&ipfix_export.IpfixAllExporterGet{}, &ipfix_export.SetIpfixClassifyStream{}, &ipfix_export.IpfixClassifyTableAddDel{})
	c := h.Client()
	ctx := context.Background()
	slot := vpptest.Slot(t)
	pool := netip.MustParsePrefix(vpptest.NATPool(t))
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
		dfkittest.HoldForEvidence(t, "vppctl show ipfix exporter all")
		if err := d.Delete(ctx, v2, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertAbsent(t, d, d.KeyOf(v2))
	})

	t.Run("default exporter + classify", func(t *testing.T) {
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

		// The classify stream has no working read-back (ErrClassifyDumpBroken): write-only. Nobody
		// else on this host uses IPFIX classify reports; it is reset to "unset" in Cleanup.
		cs := NewClassifyStream(c, own)
		if _, err := cs.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("classify stream retrieve: %v", err)
		}
		sv := ClassifyStream{DomainID: uint32(slot), SrcPort: uint16(4739 + slot)}.Proto() //nolint:gosec // slot ≤ 12
		t.Cleanup(func() { _ = cs.Delete(context.Background(), sv, nil) })
		for range 2 {
			if _, err := cs.Create(ctx, sv); err != nil {
				t.Fatal(err)
			}
		}

		// a raw classify table stands in for DF-2's descriptor
		cl := classify.NewServiceClient(c)
		mask := make([]byte, 16)
		mask[12] = 0xff
		tbl, err := cl.ClassifyAddDelTable(ctx, &classify.ClassifyAddDelTable{
			IsAdd: true, TableIndex: ^uint32(0), Nbuckets: 2, MemorySize: 2 << 20, MatchNVectors: 1,
			NextTableIndex: ^uint32(0), MissNextIndex: ^uint32(0), Mask: mask, MaskLen: 16,
		})
		if err != nil {
			t.Fatalf("classify_add_del_table: %v", err)
		}
		t.Cleanup(func() {
			_, _ = cl.ClassifyAddDelTable(context.Background(), &classify.ClassifyAddDelTable{TableIndex: tbl.NewTableIndex, DelChain: true})
		})
		ct := NewClassifyTable(c, WithClassifyTableScope(func(i uint32) bool { return i == tbl.NewTableIndex }))
		tv := ClassifyTable{Table: tbl.NewTableIndex, IPVersion: "ip4", Protocol: 17}.Proto()
		t.Cleanup(func() { _ = ct.Delete(context.Background(), tv, nil) })
		if _, err := ct.Create(ctx, tv); err != nil {
			t.Fatal(err)
		}
		if _, err := ct.Create(ctx, tv); err != nil { // re-apply: VALUE_EXIST is success
			t.Fatal(err)
		}
		if _, err := ct.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("classify table retrieve: %v", err)
		}
		dfkittest.HoldForEvidence(t, "ipfix default exporter + classify stream/table configured")
		for range 2 { // second delete: NO_SUCH_ENTRY counts as deleted
			if err := ct.Delete(ctx, tv, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := cs.Delete(ctx, sv, nil); err != nil {
			t.Fatal(err)
		}
		if err := d.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertAbsent(t, d, KeyDefaultExporter)
	})
}
