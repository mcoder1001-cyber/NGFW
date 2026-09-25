package ifsanitize_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

const none = ifsanitize.NoIndex

func setup() (*fake.Client, *sanitizetest.Model) {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	m := sanitizetest.NewModel()
	m.Install(f)
	return f, m
}

func TestCleanInterfaceOnlyResets(t *testing.T) {
	f, m := setup()
	m.Tables[3] = true
	rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop201")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inherited() || len(rep.Skipped) > 0 {
		t.Fatalf("clean interface reported %+v", rep)
	}
	want := []string{"l2-mode l3", "ip-classify ip4", "ip-classify ip6", "l2-classify input", "l2-classify output", "adl adl-input", "vxlan-bypass ip4", "vxlan-bypass ip6"}
	if !slices.Equal(rep.Reset, want) {
		t.Fatalf("reset %v, want %v", rep.Reset, want)
	}
	// one placeholder — the probe table of the output ACL check — deleted again; nothing popped beyond it
	if rep.Placeholders != 1 || len(rep.Pops) != 1 || len(m.Tables) != 1 {
		t.Fatalf("placeholders %d (pops %v), tables left %v", rep.Placeholders, rep.Pops, m.Tables)
	}
	// exact readbacks, no probing: output ACL 3 × (unbind P, bind P, unbind P); policer/flow read by dump
	for name, n := range map[string]int{"output_acl_set_interface": 9, "policer_classify_set_interface": 0, "flow_classify_set_interface": 0,
		"input_acl_set_interface": 0, "ipsec_interface_add_del_spd": 0, "policer_classify_dump": 3, "flow_classify_dump": 2, "classify_add_del_table": 2} {
		if got := len(f.CallsNamed(name)); got != n {
			t.Errorf("%s sent %d times, want %d", name, got, n)
		}
	}
	for _, c := range append(f.CallsNamed("policer_classify_dump"), f.CallsNamed("flow_classify_dump")...) {
		var idx uint32
		switch r := c.(type) {
		case *classifyapi.PolicerClassifyDump:
			idx = uint32(r.SwIfIndex)
		case *classifyapi.FlowClassifyDump:
			idx = uint32(r.SwIfIndex)
		}
		if idx != 0 { // any other index reads out of bounds in VPP 26.06
			t.Fatalf("%s sent with sw_if_index %d", c.GetMessageName(), idx)
		}
	}
	t.Logf("clean create: %d API calls", len(f.Calls()))
}

// TestInheritedStateIsCleared plants everything VPP keeps on a deleted sw_if_index (V19, V21,
// DF-5 M3), deletes the interface as VPP does (features off, vectors kept) and checks that a
// Sanitize of the reused index leaves nothing behind.
func TestInheritedStateIsCleared(t *testing.T) {
	f, m := setup()
	m.Tables[3], m.Tables[5] = true, true
	m.SPDs[10] = 0
	s := m.If(7)
	s.IPTable = [2]uint32{3, 5}
	s.L2In = [3]uint32{none, none, 5}
	s.L2Out = [3]uint32{3, none, none}
	s.InACL = [3]uint32{3, none, none}
	s.OutACL = [3]uint32{none, 5, none}
	s.Policer = [3]uint32{none, none, 3}
	s.Flow = [2]uint32{5, none}
	s.Vxlan = [2]bool{true, true}
	s.SPD = 0
	s.ADL = true
	m.DeleteInterface(7)
	s.ADL = true // models an ADL feature that survived (reset blindly all the same)

	rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "tap2")
	if err != nil {
		t.Fatal(err)
	}
	wantCleared := []string{"input-acl ip4 table 3", "output-acl ip6 table 5", "policer-classify l2 table 3", "flow-classify ip4 table 5", "ipsec-spd spd-index 0"}
	if !slices.Equal(rep.Cleared, wantCleared) {
		t.Fatalf("cleared %v, want %v", rep.Cleared, wantCleared)
	}
	got := m.If(7)
	want := sanitizetest.Clear()
	if got.IPTable != want.IPTable || got.L2In != want.L2In || got.L2Out != want.L2Out || got.InACL != want.InACL ||
		got.OutACL != want.OutACL || got.Policer != want.Policer || got.Flow != want.Flow || got.Vxlan != want.Vxlan || got.ADL || got.SPD != none {
		t.Fatalf("state left after sanitize: %+v", got)
	}
	// an unrelated interface is never touched
	other := m.If(8)
	other.InACL = [3]uint32{5, none, none}
	if _, err := ifsanitize.Sanitize(context.Background(), f, 7, "tap2"); err != nil {
		t.Fatal(err)
	}
	if m.If(8).InACL[0] != 5 {
		t.Fatal("sanitize of 7 touched 8")
	}
}

// TestBindingToDeletedTable is TD-3 review H1/H2: bindings (input ACL, output ACL, policer — the
// write-only kinds too) to tables that were deleted are removed through resurrected placeholder
// tables, the placeholders are deleted again, and the L2 feature bits are cleared by the L3 reset.
func TestBindingToDeletedTable(t *testing.T) {
	f, m := setup()
	for _, id := range []uint32{0, 1, 2, 3, 4, 5, 6} {
		m.Tables[id] = true
	}
	s := m.If(4)
	s.InACL = [3]uint32{none, none, 2}   // L2 input ACL (l2 feature bit, a live crash vector once bridged)
	s.OutACL = [3]uint32{5, none, none}  // write-only kind: invisible before (H2)
	s.Policer = [3]uint32{none, none, 6} // L2 policer
	s.Features["l2:l2-input-acl"], s.Features["l2:l2-policer"] = true, true
	m.DeleteInterface(4)
	// deleted in creation order: 5 is popped last, 2 first
	m.DeleteTable(2)
	m.DeleteTable(6)
	m.DeleteTable(5)
	live := len(m.Tables)

	rep, err := ifsanitize.Sanitize(context.Background(), f, 4, "loop202")
	if err != nil {
		t.Fatalf("sanitize: %v (report %+v)", err, rep)
	}
	if d := m.Dirty(4); d != "" {
		t.Fatalf("still inherited: %s (report %+v)", d, rep)
	}
	if len(rep.Freed) != 3 || len(rep.Unclearable) != 0 {
		t.Fatalf("freed %v unclearable %v", rep.Freed, rep.Unclearable)
	}
	for _, want := range []string{"input-acl l2 table 2", "output-acl ip4 table 5", "policer-classify l2 table 6"} {
		if !slices.ContainsFunc(rep.Freed, func(e string) bool { return strings.HasPrefix(e, want) }) {
			t.Errorf("freed %v lacks %q", rep.Freed, want)
		}
	}
	if len(m.Tables) != live {
		t.Fatalf("placeholders left: %d tables, want %d", len(m.Tables), live)
	}
	// free list [2 6 5]: the probe table is 5 (the output ACL's table), then 6 and 2 on demand
	if !slices.Equal(rep.Pops, []uint32{5, 6, 2}) || rep.Wanted != 2 {
		t.Fatalf("pops %v wanted %d", rep.Pops, rep.Wanted)
	}
	if n, free := m.Pool(); n != 7 || !slices.Equal(free, []uint32{2, 6, 5}) {
		t.Fatalf("classify pool after the run: vector %d free %v, want 7 [2 6 5]", n, free)
	}
	if m.If(4).L3Resets != 1 {
		t.Fatal("no L3-mode reset")
	}
	// feature_is_enabled is never asked: VPP 26.06 answers true for any error (out-of-range
	// sw_if_index on the arc), so it cannot tell an inherited feature from a new index
	if n := len(f.CallsNamed("feature_is_enabled")); n != 0 {
		t.Fatalf("feature_is_enabled sent %d times", n)
	}
}

// TestFreedInReverseOrder: tables freed in reverse creation order come back ascending, which looks
// like fresh growth (the old FreshRun blind spot, TD-3 re-review L1): an output ACL slot proven
// bound is resurrected until the unbind through a placeholder succeeds, however long the run.
func TestFreedInReverseOrder(t *testing.T) {
	f, m := setup()
	for id := uint32(0); id < 7; id++ {
		m.Tables[id] = true
	}
	m.If(9).OutACL = [3]uint32{6, none, none}
	for id := uint32(6); id >= 2; id-- { // free list [6 5 4 3 2]: pops 2,3,4,5,6
		m.DeleteTable(id)
	}
	rep, err := ifsanitize.Sanitize(context.Background(), f, 9, "loop205")
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(9); d != "" || len(rep.Freed) != 1 {
		t.Fatalf("dirty %q, report %+v", d, rep)
	}
}

// TestUnclearableIsAnError: what cannot be resurrected fails the create (H1: no "warn and go on"),
// and only warns before a delete.
func TestUnclearableIsAnError(t *testing.T) {
	f, m := setup()
	m.Tables[3] = true
	m.If(4).InACL = [3]uint32{9, none, none}
	m.DeleteTable(9)
	defer ifsanitize.DisableResurrect()()
	before := ifsanitize.Snapshot()
	rep, err := ifsanitize.Sanitize(context.Background(), f, 4, "loop206")
	if !errors.Is(err, ifsanitize.ErrUnclearable) || len(rep.Unclearable) != 1 {
		t.Fatalf("err %v, unclearable %v", err, rep.Unclearable)
	}
	if n := len(f.CallsNamed("input_acl_set_interface")); n != 0 {
		t.Fatalf("an unbind naming a freed table was sent (%d)", n)
	}
	if err := ifsanitize.BeforeDelete(context.Background(), f, 4, "loop206"); err != nil {
		t.Fatalf("before delete: %v", err)
	}
	after := ifsanitize.Snapshot()
	if after.Unclearable["create/input-acl"] != before.Unclearable["create/input-acl"]+1 || after.Unclearable["delete/input-acl"] != before.Unclearable["delete/input-acl"]+1 {
		t.Fatalf("counters %+v → %+v", before, after)
	}
}

func TestPluginNotLoadedIsSkipped(t *testing.T) {
	f, m := setup()
	m.Tables[1] = true
	f.Fail("sw_interface_set_vxlan_bypass", &adapter.UnknownMsgError{MsgName: "sw_interface_set_vxlan_bypass"})
	f.Fail("flow_classify_dump", &adapter.UnknownMsgError{MsgName: "flow_classify_dump"})
	f.Fail("ipsec_spd_interface_dump", &adapter.UnknownMsgError{MsgName: "ipsec_spd_interface_dump"})
	rep, err := ifsanitize.Sanitize(context.Background(), f, 2, "loop203")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Skipped, []string{"flow-classify", "vxlan-bypass", "ipsec-spd"}) {
		t.Fatalf("skipped %v", rep.Skipped)
	}
}

func TestErrorsFailAndAreCounted(t *testing.T) {
	f, _ := setup()
	before := ifsanitize.Snapshot()
	f.Fail("classify_set_interface_ip_table", errors.New("socket closed"))
	if _, err := ifsanitize.Sanitize(context.Background(), f, 2, "loop204"); err == nil || !strings.Contains(err.Error(), "loop204") {
		t.Fatalf("err = %v", err)
	}
	after := ifsanitize.Snapshot()
	if after.Runs["create"] != before.Runs["create"]+1 || after.Errors["create"] != before.Errors["create"]+1 {
		t.Fatalf("counters %+v → %+v", before, after)
	}
}

func TestMetrics(t *testing.T) {
	f, m := setup()
	m.Tables[3] = true
	m.If(6).InACL = [3]uint32{3, none, none}
	m.If(6).OutACL = [3]uint32{none, none, 3}
	before := ifsanitize.Snapshot()
	if _, err := ifsanitize.Sanitize(context.Background(), f, 6, "memif0"); err != nil {
		t.Fatal(err)
	}
	after := ifsanitize.Snapshot()
	if after.Inherited["create"] != before.Inherited["create"]+1 || after.Cleared["create/input-acl"] != before.Cleared["create/input-acl"]+1 || after.Cleared["create/output-acl"] != before.Cleared["create/output-acl"]+1 {
		t.Fatalf("counters %+v → %+v", before, after)
	}
	var b bytes.Buffer
	ifsanitize.WriteMetrics(&b)
	for _, want := range []string{`vrx_agent_iface_sanitize_total{phase="create"}`, `vrx_agent_iface_sanitize_cleared_total{phase="create",state="input-acl"}`, `vrx_agent_iface_sanitize_inherited_total{phase="create"}`, "vrx_agent_iface_quarantined "} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("metrics lack %q:\n%s", want, b.String())
		}
	}
}

// TestHoleTakenBySomeoneElse is TD-3 re-review M1: a freed index a binding names, popped by another
// client between the snapshot and the resurrection, never comes back from the pool. The run pops
// the free list until it runs dry; the one pop above every index seen (the only fresh index it
// takes: the race's cost) re-reads the table list, and the input ACL binding that names the stolen
// index is removed through the foreign table instead of being reported unclearable.
func TestHoleTakenBySomeoneElse(t *testing.T) {
	f, m := setup()
	for id := uint32(0); id < 6; id++ {
		m.Tables[id] = true
	}
	m.If(4).InACL = [3]uint32{3, none, none}
	m.If(4).OutACL = [3]uint32{none, 2, none}
	for _, id := range []uint32{1, 2, 3} { // free list [1 2 3]: 3 pops first
		m.DeleteTable(id)
	}
	hs := m.Handlers()
	var stolen []uint32
	f.On("classify_table_ids", func(req api.Message) ([]api.Message, error) {
		rep, err := hs["classify_table_ids"](req)
		if len(stolen) == 0 { // after the first snapshot: another slot creates a table
			r, aerr := hs["classify_add_del_table"](&classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: none, Nbuckets: 2, MemorySize: 64 << 10,
				MatchNVectors: 1, NextTableIndex: none, MissNextIndex: none, MaskLen: 16, Mask: []byte("foreign-table-00")})
			if aerr != nil {
				t.Fatal(aerr)
			}
			stolen = append(stolen, r[0].(*classifyapi.ClassifyAddDelTableReply).NewTableIndex)
		}
		return rep, err
	})
	before := ifsanitize.Snapshot()
	rep, err := ifsanitize.Sanitize(context.Background(), f, 4, "loop207")
	if err != nil {
		t.Fatalf("sanitize: %v (report %+v)", err, rep)
	}
	if len(stolen) != 1 || stolen[0] != 3 {
		t.Fatalf("stolen %v", stolen)
	}
	if d := m.Dirty(4); d != "" {
		t.Fatalf("still inherited: %s (report %+v)", d, rep)
	}
	// probe table 2 (the output ACL's freed table), 1, then the fresh 6 that triggers the re-read
	if rep.Capped || rep.Rereads != 1 || !slices.Equal(rep.Pops, []uint32{2, 1, 6}) {
		t.Fatalf("capped %v rereads %d pops %v", rep.Capped, rep.Rereads, rep.Pops)
	}
	if !slices.ContainsFunc(rep.Freed, func(e string) bool {
		return strings.HasPrefix(e, "input-acl ip4 table 3 (deleted table; its index was taken by another client")
	}) ||
		!slices.ContainsFunc(rep.Freed, func(e string) bool { return strings.HasPrefix(e, "output-acl ip6 table 2") }) || len(rep.Unclearable) != 0 {
		t.Fatalf("freed %v unclearable %v", rep.Freed, rep.Unclearable)
	}
	if !m.Tables[3] || len(m.Tables) != 4 { // the foreign table stays, every placeholder is gone
		t.Fatalf("tables %v", m.Tables)
	}
	calls := len(f.Calls()) // before the fix: 256 placeholders, ~2850 calls
	if calls > 200 {
		t.Fatalf("%d API calls for one create", calls)
	}
	if after := ifsanitize.Snapshot(); after.Capped["create"] != before.Capped["create"] {
		t.Fatalf("capped counter moved: %v → %v", before.Capped, after.Capped)
	}
	t.Logf("placeholders %d, rereads %d, API calls %d, freed %v", rep.Placeholders, rep.Rereads, calls, rep.Freed)
}

// TestMaxPlaceholders: one run makes at most 64 placeholder tables (the probe table included).
func TestMaxPlaceholders(t *testing.T) {
	if ifsanitize.MaxPlaceholders != 64 {
		t.Fatalf("MaxPlaceholders %d", ifsanitize.MaxPlaceholders)
	}
}

// TestOnDemandPopsOnlyDownToTheNamedIndex (TD-25): with twelve freed indices out of order, a run
// pops exactly the free-list entries above the index a binding names — never the rest, never a
// fresh index — and puts the free list back in the same order. A clean interface on the same
// pool pops only the probe table.
func TestOnDemandPopsOnlyDownToTheNamedIndex(t *testing.T) {
	freed := []uint32{9, 2, 14, 5, 11, 7, 1, 13, 4, 10, 6, 12} // free list: 12 pops first, then 6, 10, 4, …
	for _, tc := range []struct {
		name string
		plant func(m *sanitizetest.Model)
		pops []uint32
	}{
		{"clean", func(*sanitizetest.Model) {}, []uint32{12}},
		{"input ACL names 7", func(m *sanitizetest.Model) { m.If(3).InACL = [3]uint32{7, none, none} }, []uint32{12, 6, 10, 4, 13, 1, 7}},
		{"policer names 13", func(m *sanitizetest.Model) { m.If(3).Policer = [3]uint32{none, 13, none} }, []uint32{12, 6, 10, 4, 13}},
		{"output ACL (no readback) bound to 9", func(m *sanitizetest.Model) { m.If(3).OutACL = [3]uint32{9, none, none} }, []uint32{12, 6, 10, 4, 13, 1, 7, 11, 5, 14, 2, 9}},
		{"flow names the probe table's index", func(m *sanitizetest.Model) { m.If(3).Flow = [2]uint32{12, none} }, []uint32{12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := setup()
			for id := uint32(0); id < 16; id++ {
				m.Tables[id] = true
			}
			for _, id := range freed {
				m.DeleteTable(id)
			}
			tc.plant(m)
			n0, free0 := m.Pool()
			rep, err := ifsanitize.Sanitize(context.Background(), f, 3, "loop209")
			if err != nil || rep.Capped {
				t.Fatalf("err %v report %+v", err, rep)
			}
			if !slices.Equal(rep.Pops, tc.pops) {
				t.Fatalf("pops %v, want %v", rep.Pops, tc.pops)
			}
			if d := m.Dirty(3); d != "" || len(m.Tables) != 16-len(freed) {
				t.Fatalf("dirty %q tables %v", d, m.Tables)
			}
			if n, free := m.Pool(); n != n0 || !slices.Equal(free, free0) {
				t.Fatalf("classify pool changed: vector %d→%d, free %v → %v", n0, n, free0, free)
			}
			t.Logf("%s: pops %v, freed %v", tc.name, rep.Pops, rep.Freed)
		})
	}
}

// TestLongAscendingRunIsNoBlindSpot is TD-3 re-review L1: 20 tables above every live one, freed
// in reverse creation order (a test's LIFO Cleanup), pop back ascending like fresh growth. The
// output ACL binding to the last of them used to be missed silently (FreshRun gave up after 8
// ascending pops); a slot proven bound is now followed down the free list until it is found — on
// every run, with the pool unchanged.
func TestLongAscendingRunIsNoBlindSpot(t *testing.T) {
	f, m := setup()
	for id := uint32(0); id < 25; id++ {
		m.Tables[id] = true
	}
	for id := uint32(24); id >= 5; id-- {
		m.DeleteTable(id)
	}
	n0, free0 := m.Pool()
	for run := 1; run <= 2; run++ {
		m.If(5).OutACL = [3]uint32{none, 24, none}
		rep, err := ifsanitize.Sanitize(context.Background(), f, 5, "loop210")
		if err != nil || rep.Capped || rep.Placeholders != 20 || m.Dirty(5) != "" || len(rep.Freed) != 1 {
			t.Fatalf("run %d: err %v dirty %q report %+v", run, err, m.Dirty(5), rep)
		}
		if n, free := m.Pool(); n != n0 || !slices.Equal(free, free0) {
			t.Fatalf("run %d: classify pool changed: vector %d→%d, free %v → %v", run, n0, n, free0, free)
		}
		t.Logf("run %d: %d placeholders, freed %v", run, rep.Placeholders, rep.Freed)
	}
}

// TestAscendingRunAboveAHole (TD-5 review M1, the reviewer's probe): live tables {0,1,3,4}, an
// older hole 2, and above them r tables 5…5+r-1 freed in reverse creation order, so the free list
// pops 5, 6, …, 5+r-1 and only then 2; an output-ACL binding names 2. The run needs exactly r + 1
// placeholders (the probe table 5, then 6…5+r-1 and 2), twice in a row with the pool unchanged;
// the cap of 64 still holds: r = 63 needs 64 and succeeds, r = 64 needs 65 and fails closed
// (ErrCapped and ErrUnclearable) with the pool unchanged as well.
func TestAscendingRunAboveAHole(t *testing.T) {
	for _, r := range []uint32{16, 40, 63, 64} {
		f, m := setup()
		for _, id := range []uint32{0, 1, 3, 4} {
			m.Tables[id] = true
		}
		m.Len = 5 + r
		m.Free = []uint32{2}
		for i := 5 + r - 1; i >= 5; i-- { // LIFO: 5 is on top
			m.Free = append(m.Free, i)
		}
		n0, free0 := m.Pool()
		need := int(r) + 1
		for run := 1; run <= 2; run++ {
			m.If(7).OutACL = [3]uint32{2, none, none}
			before := ifsanitize.Snapshot()
			rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop211")
			capped := ifsanitize.Snapshot().Capped["create"] != before.Capped["create"]
			t.Logf("r=%d run %d: needed %d, placeholders %d, capped %v, err %v", r, run, need, rep.Placeholders, rep.Capped, err)
			if need > ifsanitize.MaxPlaceholders {
				if !errors.Is(err, ifsanitize.ErrCapped) || !errors.Is(err, ifsanitize.ErrUnclearable) || !rep.Capped || !capped || rep.Placeholders != ifsanitize.MaxPlaceholders {
					t.Fatalf("r=%d run %d: want ErrCapped at %d placeholders", r, run, ifsanitize.MaxPlaceholders)
				}
			} else if err != nil || rep.Capped || capped || rep.Placeholders != need || m.Dirty(7) != "" {
				t.Fatalf("r=%d run %d: dirty %q", r, run, m.Dirty(7))
			}
			if n, free := m.Pool(); len(m.Tables) != 4 || n != n0 || !slices.Equal(free, free0) {
				t.Fatalf("r=%d run %d: tables %v, classify pool vector %d→%d free %v → %v", r, run, m.Tables, n0, n, free0, free)
			}
		}
	}
}

// TestCappedFailsClosed: 70 tables deleted in creation order put table 0 at the bottom of the free
// list; a binding that names it needs 70 pops, more than MaxPlaceholders: the run is Capped, the
// binding Unclearable, the create fails closed (ErrCapped, which is ErrNoCleanIndex, and
// ErrUnclearable), the capped counter moves, and every placeholder is deleted again with the pool
// unchanged. A clean interface on the same pool — which failed closed before TD-25 — needs only
// the probe table; the delete phase of the dirty one only logs what it could not remove.
func TestCappedFailsClosed(t *testing.T) {
	f, m := setup()
	for id := uint32(0); id < 70; id++ {
		m.Tables[id] = true
	}
	for id := uint32(0); id < 70; id++ {
		m.DeleteTable(id)
	}
	n0, free0 := m.Pool()
	rep, err := ifsanitize.Sanitize(context.Background(), f, 2, "loop212")
	if err != nil || rep.Placeholders != 1 {
		t.Fatalf("clean interface on a 70-deep free list: err %v, placeholders %d", err, rep.Placeholders)
	}
	m.If(3).InACL = [3]uint32{0, none, none}
	before := ifsanitize.Snapshot()
	rep, err = ifsanitize.Sanitize(context.Background(), f, 3, "loop208")
	if !errors.Is(err, ifsanitize.ErrCapped) || !errors.Is(err, ifsanitize.ErrNoCleanIndex) || !errors.Is(err, ifsanitize.ErrUnclearable) || !rep.Capped {
		t.Fatalf("err %v report %+v", err, rep)
	}
	if rep.Placeholders != ifsanitize.MaxPlaceholders || rep.Cap != 64 || len(m.Tables) != 0 || !slices.Equal(rep.Unclearable, []string{"input-acl ip4 table 0"}) {
		t.Fatalf("placeholders %d cap %d unclearable %v, tables left %v", rep.Placeholders, rep.Cap, rep.Unclearable, m.Tables)
	}
	if n, free := m.Pool(); n != n0 || !slices.Equal(free, free0) {
		t.Fatalf("classify pool changed: vector %d→%d", n0, n)
	}
	after := ifsanitize.Snapshot()
	if after.Capped["create"] != before.Capped["create"]+1 {
		t.Fatalf("capped counter %v → %v", before.Capped, after.Capped)
	}
	var b bytes.Buffer
	ifsanitize.WriteMetrics(&b)
	if !strings.Contains(b.String(), `vrx_agent_iface_sanitize_capped_total{phase="create"}`) || !strings.Contains(b.String(), `vrx_agent_iface_sanitize_placeholders_total{phase="create"}`) {
		t.Fatalf("metrics lack the capped / placeholders counters:\n%s", b.String())
	}
	t.Logf("capped: %v", err)
	if err := ifsanitize.BeforeDelete(context.Background(), f, 3, "loop208"); err != nil {
		t.Fatal(err)
	}
}
