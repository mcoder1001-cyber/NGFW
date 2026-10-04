package ifsanitize_test

import (
	"context"
	"fmt"
	"testing"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestResetIPClassifyOnHost (INC-vpp-classify-crash M1, D-185) on the host VPP, without traffic:
//
//  1. positive control: a loopback of this slot bound to a LIVE classify table gets a classify DPO
//     on its address's /32 (the crash path — harmless while the table exists and no packet
//     arrives; nothing routes to 10.<slot>.95.1);
//  2. repair: ResetIPClassify with the address still present removes that /32 (reset BEFORE the
//     address delete — after it the /32 would stay);
//  3. prevention: a fresh loopback created by the fixture (reset right after create) gets no
//     classify DPO when its address is added.
//
// Cleanup resets before it removes addresses and deletes the table last; if a reset fails, the table
// is leaked (t.Errorf), never freed under a classify /32 that may still point at it.
func TestResetIPClassifyOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	h := &host{t: t, ctx: context.Background(), c: ifacetest.Connect(t), owner: vpptest.Prefix(t)}
	slot := vpptest.Slot(t)
	cl := classifyapi.NewServiceClient(h.c)
	mask := make([]byte, 16)
	table := func(add bool, idx uint32) (*classifyapi.ClassifyAddDelTableReply, error) {
		return cl.ClassifyAddDelTable(context.Background(), &classifyapi.ClassifyAddDelTable{IsAdd: add, TableIndex: idx, Nbuckets: 2, MemorySize: 2 << 20,
			MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, MissNextIndex: ifsanitize.NoIndex, MaskLen: 16, Mask: mask})
	}
	rep, err := table(true, ifsanitize.NoIndex)
	h.must("classify_add_del_table", err)
	tbl := rep.NewTableIndex
	t.Cleanup(func() { // runs last: every binding is gone by then
		if h.keep { // review F5: a /32 may still point at the table — a leaked table is harmless, a freed one is the crash
			t.Errorf("LEFT IN VPP ON PURPOSE: classify table %d (a reset or sanitize failed, a classify /32 may still reference it)", tbl)
			return
		}
		if _, err := table(false, tbl); err != nil {
			t.Errorf("cleanup classify table %d: %v", tbl, err)
		}
	})

	up := func(idx uint32) {
		_, err := interfaces.NewServiceClient(h.c).SwInterfaceSetFlags(h.ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP})
		h.must("admin up", err)
	}
	withAddr := func(idx uint32, prefix string) {
		h.address(idx, prefix, true)
		t.Cleanup(func() {
			if err := ifsanitize.ResetIPClassify(context.Background(), h.c, idx); err != nil { // D-185 repair order: reset before address delete
				h.keep = true // leak the table rather than free it under a live classify /32 (review F5)
				t.Errorf("reset before address delete of %s on %d: %v", prefix, idx, err)
			}
			h.address(idx, prefix, false)
		})
	}

	// 1: positive control
	a, aName := h.loopback(95)
	t.Cleanup(func() { h.deleteLoopback(a) })
	up(a)
	_, err = cl.ClassifySetInterfaceIPTable(h.ctx, &classifyapi.ClassifySetInterfaceIPTable{SwIfIndex: interface_types.InterfaceIndex(a), TableIndex: tbl})
	h.must("classify_set_interface_ip_table (bind live table)", err)
	addrA := fmt.Sprintf("10.%d.95.1", slot)
	withAddr(a, addrA+"/32")
	on, fib := h.classifyDPO(addrA)
	if !on {
		t.Fatalf("positive control: no classify DPO on %s/32 with ip4 classify → live table %d:\n%s", addrA, tbl, fib)
	}
	t.Logf("1 positive control: %s (sw_if_index %d) bound to live table %d, %s/32 has a classify DPO:\n%s", aName, a, tbl, addrA, fib)

	// 2: repair with the address present
	h.must("ResetIPClassify", ifsanitize.ResetIPClassify(h.ctx, h.c, a))
	if on, fib := h.classifyDPO(addrA); on {
		t.Fatalf("classify DPO still on %s/32 after ResetIPClassify:\n%s", addrA, fib)
	}
	t.Logf("2 repair: ResetIPClassify(%d) with the address present removed the classify /32", a)

	// 3: prevention on a fixture-created interface
	b, bName := h.loopback(96) // resets right after create
	t.Cleanup(func() { h.deleteLoopback(b) })
	up(b)
	addrB := fmt.Sprintf("10.%d.96.1", slot)
	withAddr(b, addrB+"/32")
	if on, fib := h.classifyDPO(addrB); on {
		t.Fatalf("fresh %s (sw_if_index %d) got a classify DPO on %s/32 although it was reset at create:\n%s", bName, b, addrB, fib)
	}
	t.Logf("3 prevention: %s (sw_if_index %d) reset at create, %s/32 has no classify DPO", bName, b, addrB)
}
