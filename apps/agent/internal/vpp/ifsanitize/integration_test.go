package ifsanitize_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/vlib"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// host is the shared-VPP fixture of the V19 reproduction. It never sends a packet: the only
// interfaces are this slot's loopbacks, and the classify table they are bound to exists for
// the whole test, so no binding ever points at a freed table (which is the crash).
type host struct {
	t     *testing.T
	ctx   context.Context
	c     vpp.Client
	owner string
	table uint32
	spdID uint32
	// keep is set when a reused index went to someone else: the table and SPD are then left in
	// VPP (a leaked table is harmless, a deleted one a foreign interface is bound to is not).
	keep bool
}

func (h *host) must(what string, err error) {
	h.t.Helper()
	if err != nil {
		h.t.Fatalf("%s: %v", what, err)
	}
}

func (h *host) loopback(i int) (uint32, string) {
	h.t.Helper()
	inst := vpptest.LoopbackInstance(h.t, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(h.c)
	rep, err := svc.CreateLoopbackInstance(h.ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	h.must("create_loopback_instance "+name, err)
	tag, err := vpp.OwnerTag(h.owner, name)
	h.must("tag", err)
	_, err = svc.SwInterfaceTagAddDel(h.ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag})
	h.must("sw_interface_tag_add_del", err)
	return uint32(rep.SwIfIndex), name
}

// deleteLoopback sanitizes idx first, so no binding outlives the loopback whatever the test
// did (V19: a deleted interface's bindings stay on its index).
func (h *host) deleteLoopback(idx uint32) {
	h.t.Helper()
	if _, err := ifsanitize.Sanitize(h.ctx, h.c, idx, fmt.Sprintf("td3-cleanup-%d", idx)); err != nil {
		h.keep = true
		h.t.Errorf("sanitize before delete of %d: %v", idx, err)
	}
	_, err := interfaces.NewServiceClient(h.c).DeleteLoopback(h.ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)})
	h.must(fmt.Sprintf("delete_loopback %d", idx), err)
}

// bindAll binds the table (and the SPD) everywhere VPP keeps a per-index binding.
func (h *host) bindAll(idx uint32) {
	h.t.Helper()
	i := interface_types.InterfaceIndex(idx)
	cl := classifyapi.NewServiceClient(h.c)
	for _, v6 := range []bool{false, true} {
		_, err := cl.ClassifySetInterfaceIPTable(h.ctx, &classifyapi.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: i, TableIndex: h.table})
		h.must("classify_set_interface_ip_table", err)
	}
	_, err := cl.InputACLSetInterface(h.ctx, &classifyapi.InputACLSetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: ifsanitize.NoIndex, IsAdd: true})
	h.must("input_acl_set_interface", err)
	_, err = cl.OutputACLSetInterface(h.ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: ifsanitize.NoIndex, IsAdd: true})
	h.must("output_acl_set_interface", err)
	_, err = cl.PolicerClassifySetInterface(h.ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: ifsanitize.NoIndex, IsAdd: true})
	h.must("policer_classify_set_interface", err)
	_, err = cl.FlowClassifySetInterface(h.ctx, &classifyapi.FlowClassifySetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, IsAdd: true})
	h.must("flow_classify_set_interface", err)
	_, err = vxlanapi.NewServiceClient(h.c).SwInterfaceSetVxlanBypass(h.ctx, &vxlanapi.SwInterfaceSetVxlanBypass{SwIfIndex: i, Enable: true})
	h.must("sw_interface_set_vxlan_bypass", err)
	_, err = ipsecapi.NewServiceClient(h.c).IpsecInterfaceAddDelSpd(h.ctx, &ipsecapi.IpsecInterfaceAddDelSpd{IsAdd: true, SwIfIndex: i, SpdID: h.spdID})
	h.must("ipsec_interface_add_del_spd", err)
}

func (h *host) inputACL(idx uint32) uint32 {
	h.t.Helper()
	rep, err := classifyapi.NewServiceClient(h.c).ClassifyTableByInterface(h.ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
	h.must("classify_table_by_interface", err)
	return rep.IP4TableID
}

func (h *host) spdBound(idx uint32) bool {
	h.t.Helper()
	stream, err := ipsecapi.NewServiceClient(h.c).IpsecSpdInterfaceDump(h.ctx, &ipsecapi.IpsecSpdInterfaceDump{})
	h.must("ipsec_spd_interface_dump", err)
	found := false
	for {
		d, err := stream.Recv()
		if err == io.EOF {
			return found
		}
		h.must("ipsec_spd_interface_dump", err)
		if uint32(d.SwIfIndex) == idx {
			found = true
		}
	}
}

// stillBound probes the write-only bindings without changing a clean interface: an unbind of
// the table answers NO_SUCH_TABLE unless it is bound (and removes it when it was).
func (h *host) stillBound(idx uint32) []string {
	h.t.Helper()
	i := interface_types.InterfaceIndex(idx)
	cl := classifyapi.NewServiceClient(h.c)
	var bound []string
	check := func(what string, err error) {
		switch {
		case err == nil:
			bound = append(bound, what)
		case !errors.As(err, new(api.VPPApiError)) || !errors.Is(err, ifsanitize.ErrNoSuchTable):
			h.t.Fatalf("%s probe: %v", what, err)
		}
	}
	_, err := cl.OutputACLSetInterface(h.ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: ifsanitize.NoIndex})
	check("output-acl", err)
	_, err = cl.PolicerClassifySetInterface(h.ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: ifsanitize.NoIndex})
	check("policer-classify", err)
	_, err = cl.FlowClassifySetInterface(h.ctx, &classifyapi.FlowClassifySetInterface{SwIfIndex: i, IP4TableIndex: h.table, IP6TableIndex: ifsanitize.NoIndex})
	check("flow-classify", err)
	return bound
}

// address adds/removes an IPv4 address; with an ip classify binding VPP gives its /32 a
// classify DPO (the 04:50:27 crash path, once the table is gone and a packet arrives).
func (h *host) address(idx uint32, prefix string, add bool) {
	h.t.Helper()
	p := netip.MustParsePrefix(prefix)
	_, err := interfaces.NewServiceClient(h.c).SwInterfaceAddDelAddress(h.ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(idx), IsAdd: add,
		Prefix: ip_types.AddressWithPrefix{Address: ip_types.Address{Af: ip_types.ADDRESS_IP4, Un: ip_types.AddressUnionIP4(p.Addr().As4())}, Len: uint8(p.Bits())}}) //nolint:gosec // ≤ 32
	h.must("sw_interface_add_del_address "+prefix, err)
}

// classifyDPO reports whether the FIB entry of addr/32 carries a classify DPO. Test-only
// evidence through cli_inband: VPP has no binary-API readback of the ip classify binding.
func (h *host) classifyDPO(addr string) (bool, string) {
	h.t.Helper()
	rep, err := vlib.NewServiceClient(h.c).CliInband(h.ctx, &vlib.CliInband{Cmd: "show ip fib " + addr + "/32"})
	h.must("cli_inband show ip fib", err)
	return strings.Contains(rep.Reply, "classify:"), rep.Reply
}

// TestV19InheritanceClearedOnHost reproduces V19 on the host VPP without traffic: bind a
// classify table (and an SPD) everywhere on loopback A, delete A without unbinding, create B on
// the reused sw_if_index and show B inherited the bindings; Sanitize(B) clears them. Then the
// product path: the loopback descriptor's Create on a reused index returns only after the
// bindings are gone. The table is deleted last, when nothing refers to it.
func TestV19InheritanceClearedOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	h := &host{t: t, ctx: context.Background(), c: ifacetest.Connect(t), owner: vpptest.Prefix(t)}
	h.spdID = vpptest.TableBase(t) + 91

	cl := classifyapi.NewServiceClient(h.c)
	mask := make([]byte, 16)
	rep, err := cl.ClassifyAddDelTable(h.ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: ifsanitize.NoIndex, Nbuckets: 2, MemorySize: 2 << 20,
		MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, MissNextIndex: ifsanitize.NoIndex, MaskLen: 16, Mask: mask})
	h.must("classify_add_del_table", err)
	h.table = rep.NewTableIndex
	t.Logf("classify table %d created", h.table)
	_, err = ipsecapi.NewServiceClient(h.c).IpsecSpdAddDel(h.ctx, &ipsecapi.IpsecSpdAddDel{IsAdd: true, SpdID: h.spdID})
	h.must("ipsec_spd_add_del", err)
	t.Cleanup(func() {
		ctx := context.Background()
		if h.keep {
			t.Errorf("LEFT IN VPP ON PURPOSE: classify table %d and SPD %d (a reused index went to another interface; deleting them could leave it a dangling binding)", h.table, h.spdID)
			return
		}
		// deleting the SPD clears every binding to it; the table goes only when no binding is left
		if _, err := ipsecapi.NewServiceClient(h.c).IpsecSpdAddDel(ctx, &ipsecapi.IpsecSpdAddDel{IsAdd: false, SpdID: h.spdID}); err != nil {
			t.Errorf("cleanup ipsec_spd_add_del %d: %v", h.spdID, err)
		}
		if _, err := cl.ClassifyAddDelTable(ctx, &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: h.table, Nbuckets: 2, MemorySize: 2 << 20,
			MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, MissNextIndex: ifsanitize.NoIndex, MaskLen: 16, Mask: mask}); err != nil {
			t.Errorf("cleanup classify table %d: %v", h.table, err)
		} else {
			t.Logf("classify table %d deleted after every binding to it was gone", h.table)
		}
	})

	// --- phase 1: reproduce the inheritance with raw binapi, then Sanitize
	a, aName := h.loopback(91)
	h.bindAll(a)
	t.Logf("%s sw_if_index %d: ip4/ip6 classify, input/output ACL, policer, flow classify → table %d; vxlan bypass; SPD %d", aName, a, h.table, h.spdID)
	h.keep = true                                                                                                                             // until the freed index is back in our hands
	_, err = interfaces.NewServiceClient(h.c).DeleteLoopback(h.ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(a)}) // raw: leave the bindings behind
	h.must("delete_loopback "+aName, err)
	b, bName := h.loopback(92)
	defer h.deleteLoopback(b)
	if b != a {
		t.Fatalf("%s got sw_if_index %d, not the freed %d (another creator took it)", bName, b, a)
	}
	h.keep = false
	if got := h.inputACL(b); got != h.table {
		t.Fatalf("VPP no longer shows V19? %s (reused %d) input ACL = %d, want inherited %d", bName, b, int32(got), h.table) //nolint:gosec // ~0 → -1
	}
	if !h.spdBound(b) {
		t.Fatalf("VPP no longer shows the SPD inheritance on %d", b)
	}
	// admin up so VPP installs the address's /32 (no packet is ever sent: nothing routes here)
	_, err = interfaces.NewServiceClient(h.c).SwInterfaceSetFlags(h.ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: interface_types.InterfaceIndex(b), Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP})
	h.must("admin up", err)
	h.address(b, "10.2.91.1/32", true)
	defer h.address(b, "10.2.91.1/32", false)
	on, fib := h.classifyDPO("10.2.91.1")
	if !on {
		t.Fatalf("expected the inherited ip classify binding to put a classify DPO on 10.2.91.1/32:\n%s", fib)
	}
	t.Logf("INHERITED on %s (sw_if_index %d, reused): input ACL ip4 = table %d, SPD bound, 10.2.91.1/32 has a classify DPO:\n%s", bName, b, h.table, strings.TrimSpace(fib))

	r, err := ifsanitize.Sanitize(h.ctx, h.c, b, bName)
	h.must("Sanitize", err)
	t.Logf("Sanitize(%s): cleared=%v reset=%v unclearable=%v skipped=%v", bName, r.Cleared, r.Reset, r.Unclearable, r.Skipped)
	for _, want := range []string{
		fmt.Sprintf("input-acl ip4 table %d", h.table), fmt.Sprintf("output-acl ip4 table %d", h.table),
		fmt.Sprintf("policer-classify ip4 table %d", h.table), fmt.Sprintf("flow-classify ip4 table %d", h.table),
	} {
		if !slices.Contains(r.Cleared, want) {
			t.Errorf("Sanitize did not clear %q (cleared %v)", want, r.Cleared)
		}
	}
	if len(r.Unclearable) > 0 {
		t.Errorf("unclearable %v", r.Unclearable)
	}
	if got := h.inputACL(b); got != ifsanitize.NoIndex {
		t.Errorf("input ACL still %d after Sanitize", got)
	}
	if h.spdBound(b) {
		t.Error("SPD still bound after Sanitize")
	}
	if bound := h.stillBound(b); len(bound) > 0 {
		t.Errorf("still bound after Sanitize: %v", bound)
	}
	if on, fib := h.classifyDPO("10.2.91.1"); on {
		t.Errorf("classify DPO still on 10.2.91.1/32 after Sanitize:\n%s", fib)
	} else {
		t.Logf("after Sanitize: no classify DPO on 10.2.91.1/32, input ACL none, SPD none, output/policer/flow probes NO_SUCH_TABLE")
	}

	// --- phase 2: the product creator (core loopback descriptor) on a reused index
	c, cName := h.loopback(93)
	h.bindAll(c)
	h.keep = true
	_, err = interfaces.NewServiceClient(h.c).DeleteLoopback(h.ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(c)}) // raw
	h.must("delete_loopback "+cName, err)
	before := ifsanitize.Snapshot()
	d := &core.LoopbackDescriptor{Env: core.Env{Client: h.c, Owner: h.owner}}
	inst := vpptest.LoopbackInstance(t, 94)
	dName := fmt.Sprintf("loop%d", inst)
	meta, err := d.Create(h.ctx, &core.Loopback{Name: dName, Instance: inst})
	h.must("LoopbackDescriptor.Create", err)
	obj := &core.Loopback{Name: dName, Instance: inst}
	defer func() { h.must("LoopbackDescriptor.Delete", d.Delete(context.Background(), obj, meta)) }()
	idx := meta.(core.IfMeta).SwIfIndex
	if idx != c {
		t.Fatalf("%s got sw_if_index %d, not the freed %d of %s", dName, idx, c, cName)
	}
	h.keep = false
	// right after Create returned — before anything else touches the interface
	if got := h.inputACL(idx); got != ifsanitize.NoIndex {
		t.Fatalf("Create returned with inherited input ACL %d on %s", got, dName)
	}
	if h.spdBound(idx) {
		t.Fatal("Create returned with an inherited SPD binding")
	}
	if bound := h.stillBound(idx); len(bound) > 0 {
		t.Fatalf("Create returned with inherited %v", bound)
	}
	after := ifsanitize.Snapshot()
	if after.Inherited["create"] != before.Inherited["create"]+1 || after.Cleared["create/input-acl"] != before.Cleared["create/input-acl"]+1 || after.Cleared["create/ipsec-spd"] != before.Cleared["create/ipsec-spd"]+1 {
		t.Errorf("metrics %+v → %+v", before, after)
	}
	t.Logf("%s (reused sw_if_index %d of %s) created clean by the loopback descriptor; metric inherited %d→%d, cleared %v", dName, idx, cName, before.Inherited["create"], after.Inherited["create"], after.Cleared)
}

// l2Features returns the l2-input and l2-output feature lists of idx as `show interface <idx>
// feat` prints them (VPP has no binary-API readback of the l2 bitmaps of an L3-mode interface:
// l2_flags_get answers INVALID_SW_IF_INDEX unless the interface is bridged/xconnected).
func (h *host) l2Features(idx uint32) (in, out string) {
	h.t.Helper()
	rep, err := vlib.NewServiceClient(h.c).CliInband(h.ctx, &vlib.CliInband{Cmd: fmt.Sprintf("show interface %d feat", idx)})
	h.must("cli_inband show interface feat", err)
	text := rep.Reply
	i, o := strings.Index(text, "l2-input:"), strings.Index(text, "l2-output:")
	if i < 0 || o < i {
		h.t.Fatalf("unexpected show interface feat output:\n%s", text)
	}
	return text[i:o], text[o:]
}

func (h *host) newTable() uint32 {
	h.t.Helper()
	rep, err := classifyapi.NewServiceClient(h.c).ClassifyAddDelTable(h.ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: ifsanitize.NoIndex, Nbuckets: 2, MemorySize: 2 << 20,
		MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, MissNextIndex: ifsanitize.NoIndex, MaskLen: 16, Mask: make([]byte, 16)})
	h.must("classify_add_del_table", err)
	return rep.NewTableIndex
}

func (h *host) delTable(t uint32) {
	h.t.Helper()
	_, err := classifyapi.NewServiceClient(h.c).ClassifyAddDelTable(h.ctx, &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: t, Nbuckets: 2, MemorySize: 2 << 20,
		MatchNVectors: 1, NextTableIndex: ifsanitize.NoIndex, MissNextIndex: ifsanitize.NoIndex, MaskLen: 16, Mask: make([]byte, 16)})
	h.must(fmt.Sprintf("delete classify table %d", t), err)
}

func (h *host) tableIDs() []uint32 {
	h.t.Helper()
	rep, err := classifyapi.NewServiceClient(h.c).ClassifyTableIds(h.ctx, &classifyapi.ClassifyTableIds{})
	h.must("classify_table_ids", err)
	ids := append([]uint32(nil), rep.Ids...)
	slices.Sort(ids)
	return ids
}

// plantFreed makes the TD-3 review H1/H2 state on a fresh loopback: in L3 mode (no bridge, no
// address, admin down: nothing can reach it) bind a new table as L2 input ACL, L2 output ACL, L2
// policer classify and ip4 output ACL, delete the loopback raw (VPP keeps the bindings and the l2
// feature bits on the index) and then delete the table. Returns the freed sw_if_index and table.
func (h *host) plantFreed(i int) (uint32, uint32) {
	h.t.Helper()
	table := h.newTable()
	a, aName := h.loopback(i)
	x := interface_types.InterfaceIndex(a)
	cl := classifyapi.NewServiceClient(h.c)
	planted := false
	h.t.Cleanup(func() { // a failure while planting: unbind (the table is alive), delete loopback and table
		if planted {
			return
		}
		if err := ifsanitize.BeforeDelete(context.Background(), h.c, a, aName); err != nil {
			h.t.Errorf("cleanup: %v — LEFT %s and table %d in VPP", err, aName, table)
			return
		}
		_, _ = interfaces.NewServiceClient(h.c).DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: x})
		h.delTable(table)
	})
	_, err := cl.InputACLSetInterface(h.ctx, &classifyapi.InputACLSetInterface{SwIfIndex: x, IP4TableIndex: ifsanitize.NoIndex, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: table, IsAdd: true})
	h.must("input_acl_set_interface l2", err)
	_, err = cl.OutputACLSetInterface(h.ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: x, IP4TableIndex: table, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: table, IsAdd: true})
	h.must("output_acl_set_interface ip4+l2", err)
	_, err = cl.PolicerClassifySetInterface(h.ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: x, IP4TableIndex: ifsanitize.NoIndex, IP6TableIndex: ifsanitize.NoIndex, L2TableIndex: table, IsAdd: true})
	h.must("policer_classify_set_interface l2", err)
	in, out := h.l2Features(a)
	if !strings.Contains(in, "(l2-input-acl)") || !strings.Contains(in, "(l2-policer-classify)") || !strings.Contains(out, "(l2-output-acl)") {
		h.t.Fatalf("VPP did not set the l2 feature bits:\n%s%s", in, out)
	}
	h.t.Logf("%s sw_if_index %d (L3 mode): L2 input ACL, L2 output ACL, L2 policer classify, ip4 output ACL → table %d; show interface feat:\n%s%s", aName, a, table, in, out)
	_, err = interfaces.NewServiceClient(h.c).DeleteLoopback(h.ctx, &interfaces.DeleteLoopback{SwIfIndex: x}) // raw: leave everything behind
	h.must("delete_loopback "+aName, err)
	planted = true
	h.delTable(table)
	return a, table
}

// rescue is the safety net of the freed-table test: if the planted index is still dirty when the
// test ends, take it back with a raw loopback and sanitize it (placeholders) before deleting it.
func (h *host) rescue(idx uint32, clean *bool) {
	if *clean {
		return
	}
	svc := interfaces.NewServiceClient(h.c)
	rep, err := svc.CreateLoopback(context.Background(), &interfaces.CreateLoopback{})
	if err != nil {
		h.t.Errorf("RESCUE: create_loopback: %v — sw_if_index %d may still carry bindings to a deleted table", err, idx)
		return
	}
	got := uint32(rep.SwIfIndex)
	if got == idx {
		if _, err := ifsanitize.Sanitize(context.Background(), h.c, got, "td3-rescue"); err != nil {
			h.t.Errorf("RESCUE: sanitize %d: %v", got, err)
		}
	} else {
		h.t.Errorf("RESCUE: sw_if_index %d was taken by another creator (we got %d): it may carry bindings to a deleted table — tell the manager", idx, got)
	}
	_, _ = svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex})
}

// TestV19FreedTableOnHost is TD-3 review H1/H2 on the host VPP, without traffic: an index that
// carries L2 ACL / L2 policer / output ACL bindings to a DELETED classify table (the state that
// crashes l2-input-acl once the index is bridged, and makes a later ACL bind a silent no-op) is
// (1) cleaned by the loopback descriptor's Create — L3 mode clears the l2 bits, the table index is
// resurrected by placeholders, the bindings unbound, the placeholders deleted — and (2) when
// resurrection is impossible (MaxPlaceholders 0), quarantined: an admin-down "quarantine:<owner>"
// holder takes the index, the pre-flight reports it as WARN, the interface is created on a fresh
// index, and Release frees the holder once the index can be cleaned.
func TestV19FreedTableOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	h := &host{t: t, ctx: context.Background(), c: ifacetest.Connect(t), owner: vpptest.Prefix(t)}
	d := &core.LoopbackDescriptor{Env: core.Env{Client: h.c, Owner: h.owner}}
	baseline := h.tableIDs()

	// --- 1: resurrect
	a, table := h.plantFreed(85)
	clean := false
	defer h.rescue(a, &clean)
	for _, cmd := range []string{"show inacl type l2", "show outacl type l2", "show classify policer type l2"} {
		rep, err := vlib.NewServiceClient(h.c).CliInband(h.ctx, &vlib.CliInband{Cmd: cmd})
		h.must(cmd, err)
		t.Logf("after the raw delete of the loopback and of table %d — %s:\n%s", table, cmd, strings.TrimRight(rep.Reply, "\n"))
	}
	before := ifsanitize.Snapshot()
	inst := vpptest.LoopbackInstance(t, 86)
	obj := &core.Loopback{Name: fmt.Sprintf("loop%d", inst), Instance: inst}
	meta, err := d.Create(h.ctx, obj)
	h.must("LoopbackDescriptor.Create", err)
	defer func() { h.must("LoopbackDescriptor.Delete", d.Delete(context.Background(), obj, meta)) }()
	idx := meta.(core.IfMeta).SwIfIndex
	if idx != a {
		t.Fatalf("%s got sw_if_index %d, not the planted %d (another creator took it)", obj.Name, idx, a)
	}
	clean = true
	cur, err := classifyapi.NewServiceClient(h.c).ClassifyTableByInterface(h.ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
	h.must("classify_table_by_interface", err)
	if cur.IP4TableID != ifsanitize.NoIndex || cur.IP6TableID != ifsanitize.NoIndex || cur.L2TableID != ifsanitize.NoIndex {
		clean = false
		t.Fatalf("input ACL left on %d: %+v", idx, cur)
	}
	in, out := h.l2Features(idx)
	if strings.Contains(in, "(l2-input-acl)") || strings.Contains(in, "(l2-policer-classify)") || strings.Contains(out, "(l2-output-acl)") {
		clean = false
		t.Fatalf("l2 feature bits left on %d:\n%s%s", idx, in, out)
	}
	for _, cmd := range []string{"show inacl type l2", "show outacl type ip4", "show outacl type l2", "show classify policer type l2"} {
		rep, err := vlib.NewServiceClient(h.c).CliInband(h.ctx, &vlib.CliInband{Cmd: cmd})
		h.must(cmd, err)
		for _, line := range strings.Split(rep.Reply, "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == fmt.Sprint(idx) {
				clean = false
				t.Fatalf("%s still lists sw_if_index %d: %q", cmd, idx, line)
			}
		}
	}
	if got := h.tableIDs(); !slices.Equal(got, baseline) {
		t.Errorf("classify tables %v after Create, want the baseline %v (placeholders left?)", got, baseline)
	}
	after := ifsanitize.Snapshot()
	t.Logf("%s created on the planted sw_if_index %d: input ACL none, l2 features %q %q, no row in show inacl/outacl/classify policer; classify tables back to %v; freed %v → %v",
		obj.Name, idx, in, out, baseline, before.Freed, after.Freed)
	if after.Freed["create/input-acl"] != before.Freed["create/input-acl"]+1 || after.Freed["create/output-acl"] < before.Freed["create/output-acl"]+2 ||
		after.Freed["create/policer-classify"] != before.Freed["create/policer-classify"]+1 {
		t.Errorf("freed counters %v → %v", before.Freed, after.Freed)
	}

	// --- 2: quarantine (resurrection disabled), pre-flight WARN, release
	c, table2 := h.plantFreed(87)
	cleanC := false
	defer h.rescue(c, &cleanC)
	defer func(n int) { ifsanitize.MaxPlaceholders = n }(ifsanitize.MaxPlaceholders)
	ifsanitize.MaxPlaceholders = 0
	inst2 := vpptest.LoopbackInstance(t, 88)
	obj2 := &core.Loopback{Name: fmt.Sprintf("loop%d", inst2), Instance: inst2}
	meta2, err := d.Create(h.ctx, obj2)
	h.must("LoopbackDescriptor.Create (quarantine path)", err)
	defer func() { h.must("LoopbackDescriptor.Delete", d.Delete(context.Background(), obj2, meta2)) }()
	idx2 := meta2.(core.IfMeta).SwIfIndex
	ifsanitize.MaxPlaceholders = 256
	if idx2 == c {
		t.Fatalf("%s was reported created on the dirty index %d", obj2.Name, c)
	}
	holder := h.ifDetails(c)
	if holder == nil || holder.Tag != ifsanitize.QuarantineTagPrefix+h.owner || holder.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0 {
		t.Fatalf("no admin-down quarantine holder on %d: %+v", c, holder)
	}
	t.Logf("quarantine: %s created on fresh sw_if_index %d; dirty %d held by %s (tag %q, admin-down); gauge vrx_agent_iface_quarantined=%d",
		obj2.Name, idx2, c, holder.InterfaceName, holder.Tag, ifsanitize.Snapshot().Quarantined)
	findings, err := ifsanitize.Preflight(h.ctx, h.c)
	h.must("Preflight", err)
	for _, f := range findings {
		if f.SwIfIndex == c {
			if f.Fatal {
				t.Errorf("pre-flight FAILs the quarantine holder: %s", f)
			}
			t.Logf("pre-flight: %s", f)
		}
	}
	n, err := ifsanitize.Release(h.ctx, h.c, h.owner)
	h.must("Release", err)
	if n != 1 || h.ifDetails(c) != nil {
		t.Fatalf("released %d, holder still there: %v", n, h.ifDetails(c) != nil)
	}
	cleanC = true
	t.Logf("Release: holder of %d sanitized through placeholders (table %d resurrected) and deleted; classify tables %v", c, table2, h.tableIDs())
	if got := h.tableIDs(); !slices.Equal(got, baseline) {
		t.Errorf("classify tables %v after Release, want %v", got, baseline)
	}
}

func (h *host) ifDetails(idx uint32) *interfaces.SwInterfaceDetails {
	h.t.Helper()
	stream, err := interfaces.NewServiceClient(h.c).SwInterfaceDump(h.ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
	h.must("sw_interface_dump", err)
	var out *interfaces.SwInterfaceDetails
	for {
		d, err := stream.Recv()
		if err != nil {
			return out
		}
		if uint32(d.SwIfIndex) == idx {
			out = d
		}
	}
}
