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
	// holes 0-2 and FreshRun fresh indices resurrected, all deleted again
	if rep.Placeholders != 3+ifsanitize.FreshRun || len(m.Tables) != 1 {
		t.Fatalf("placeholders %d, tables left %v", rep.Placeholders, m.Tables)
	}
	// one probe per table (live + placeholder) and slot: output acl ×3, policer ×3, flow ×2 — all NO_SUCH_TABLE
	n := 1 + rep.Placeholders
	for name, n := range map[string]int{"output_acl_set_interface": 3 * n, "policer_classify_set_interface": 3 * n, "flow_classify_set_interface": 2 * n, "input_acl_set_interface": 0, "ipsec_interface_add_del_spd": 0} {
		if got := len(f.CallsNamed(name)); got != n {
			t.Errorf("%s sent %d times, want %d", name, got, n)
		}
	}
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
	if rep.Placeholders < 3+ifsanitize.FreshRun {
		t.Fatalf("placeholders %d", rep.Placeholders)
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
// like fresh growth; FreshRun must see past that.
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
	f.Fail("flow_classify_set_interface", &adapter.UnknownMsgError{MsgName: "flow_classify_set_interface"})
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

// TestHoleTakenBySomeoneElse is TD-3 re-review M1: a hole popped by another client between the
// snapshot and the loop used to run resurrect to the cap (then 256, ~2850 API calls). Now the
// run re-reads the table list, drops the hole, stays within 3 holes + FreshRun placeholders, and
// the input ACL binding that names the stolen index is removed through the foreign table instead
// of being reported unclearable.
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
	if rep.Capped || rep.Rereads < 1 || rep.Placeholders > 2+ifsanitize.FreshRun {
		t.Fatalf("capped %v rereads %d placeholders %d", rep.Capped, rep.Rereads, rep.Placeholders)
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

// TestPlaceholderCap (D-105, TD-3 re-review M1 option b): the cap is holes + 2 × FreshRun, at most 64.
func TestPlaceholderCap(t *testing.T) {
	if ifsanitize.MaxPlaceholders != 64 || ifsanitize.FreshRun != 8 {
		t.Fatalf("MaxPlaceholders %d FreshRun %d", ifsanitize.MaxPlaceholders, ifsanitize.FreshRun)
	}
	for holes, want := range map[int]int{0: 16, 1: 17, 12: 28, 47: 63, 48: 64, 49: 64, 1000: 64} {
		if got := ifsanitize.PlaceholderCap(holes); got != want {
			t.Errorf("PlaceholderCap(%d) = %d, want %d", holes, got, want)
		}
	}
}

// TestTwelveFreedOutOfOrderSucceeds (D-105): a free list of 12 indices that come back out of order
// (not ascending: the fixed cap of 16 failed it — 12 placeholders + FreshRun is 20) now succeeds,
// the inherited binding to a freed table is removed through its placeholder, and no placeholder
// is left.
func TestTwelveFreedOutOfOrderSucceeds(t *testing.T) {
	freed := []uint32{9, 2, 14, 5, 11, 7, 1, 13, 4, 10, 6, 12} // free list: 12 pops first, then 6, 10, 4, …
	pool := func() (*fake.Client, *sanitizetest.Model) {
		f, m := setup()
		for id := uint32(0); id < 16; id++ {
			m.Tables[id] = true
		}
		m.If(3).OutACL = [3]uint32{9, none, none}
		for _, id := range freed {
			m.DeleteTable(id)
		}
		return f, m
	}
	// the old fixed cap (16) failed this pool closed
	old := ifsanitize.MaxPlaceholders
	ifsanitize.MaxPlaceholders = 16
	f, _ := pool()
	_, err := ifsanitize.Sanitize(context.Background(), f, 3, "loop209")
	ifsanitize.MaxPlaceholders = old
	if !errors.Is(err, ifsanitize.ErrCapped) {
		t.Fatalf("with the fixed cap of 16: err %v", err)
	}
	f, m := pool()
	before := ifsanitize.Snapshot()
	rep, err := ifsanitize.Sanitize(context.Background(), f, 3, "loop209")
	if err != nil || rep.Capped {
		t.Fatalf("err %v report %+v", err, rep)
	}
	if rep.Placeholders < len(freed)+ifsanitize.FreshRun || rep.Placeholders <= 16 || rep.Placeholders > rep.Cap {
		t.Fatalf("placeholders %d, cap %d (holes seen %d)", rep.Placeholders, rep.Cap, rep.Holes)
	}
	if d := m.Dirty(3); d != "" || len(rep.Freed) != 1 || len(m.Tables) != 16-len(freed) {
		t.Fatalf("dirty %q freed %v tables %v", d, rep.Freed, m.Tables)
	}
	if after := ifsanitize.Snapshot(); after.Capped["create"] != before.Capped["create"] {
		t.Fatalf("capped counter moved: %v → %v", before.Capped, after.Capped)
	}
	t.Logf("12 out-of-order freed indices: %d placeholders, cap %d (holes seen %d), freed %v", rep.Placeholders, rep.Cap, rep.Holes, rep.Freed)
}

// TestReplayedAscendingRunsAreCounted (TD-5 host run 14:32): a pool without live tables whose free
// list pops 7, 8…16, 0…6, 17…21 — a previous run's creation order, replayed because
// dropPlaceholders frees in reverse creation order — has only 7 gaps. Counting gaps alone gave a
// cap of 23 and the create failed closed on every run (the same pops replay each time). The
// ascending run 7…16 that the pop of 0 interrupts is proven freed and counts as seen, so the cap
// grows and the run finishes; a second run replays the first one's pops and succeeds again.
func TestReplayedAscendingRunsAreCounted(t *testing.T) {
	f, m := setup()
	var pops []uint32
	for i := uint32(7); i <= 16; i++ {
		pops = append(pops, i)
	}
	for i := uint32(0); i <= 6; i++ {
		pops = append(pops, i)
	}
	for i := uint32(17); i <= 21; i++ {
		pops = append(pops, i)
	}
	m.Len = 22
	for i := len(pops) - 1; i >= 0; i-- { // LIFO: the first pop is on top
		m.Free = append(m.Free, pops[i])
	}
	m.If(5).OutACL = [3]uint32{none, 12, none} // a binding to a freed table in the ascending run
	for run := 1; run <= 2; run++ {
		rep, err := ifsanitize.Sanitize(context.Background(), f, 5, "loop210")
		if err != nil || rep.Capped {
			t.Fatalf("run %d: err %v report %+v", run, err, rep)
		}
		// all 22 freed indices, then growth 22–24 completes the fresh run 17…24: 25, more than the
		// 7 gaps + 2 × FreshRun = 23 a gap-only count allowed
		if rep.Placeholders != 25 || rep.Placeholders <= 7+2*ifsanitize.FreshRun || rep.Cap < rep.Placeholders {
			t.Fatalf("run %d: placeholders %d cap %d holes seen %d", run, rep.Placeholders, rep.Cap, rep.Holes)
		}
		if d := m.Dirty(5); d != "" || len(m.Tables) != 0 {
			t.Fatalf("run %d: dirty %q tables %v", run, d, m.Tables)
		}
		t.Logf("run %d: %d placeholders, cap %d (freed indices seen %d), freed %v", run, rep.Placeholders, rep.Cap, rep.Holes, rep.Freed)
		m.If(5).OutACL = [3]uint32{none, 12, none}
	}
}

// TestAscendingRunAboveAHole (TD-5 review M1, the reviewer's probe): live tables {0,1,3,4}, an
// older hole 2, and above them r tables 5…5+r-1 freed in reverse creation order (a test's LIFO
// Cleanup), so the free list pops 5, 6, …, 5+r-1 and only then 2; an output-ACL binding names 2.
// The unbroken ascending run used to be counted only once a gap or a lower pop broke it: holes
// seen stayed 1, the cap 17, and every r ≥ 17 failed closed — on every run, as dropPlaceholders
// replays the same pops. A hole still free at the re-read proves the run came from the free list,
// so it needs exactly r + 1 + FreshRun placeholders, twice in a row; the cap of 64 still holds
// (r = 55 needs 64 and succeeds, r = 56 needs 65 and fails closed).
func TestAscendingRunAboveAHole(t *testing.T) {
	for _, r := range []uint32{16, 17, 20, 40, 55, 56} {
		f, m := setup()
		for _, id := range []uint32{0, 1, 3, 4} {
			m.Tables[id] = true
		}
		m.Len = 5 + r
		m.Free = []uint32{2}
		for i := 5 + r - 1; i >= 5; i-- { // LIFO: 5 is on top
			m.Free = append(m.Free, i)
		}
		need := int(r) + 1 + ifsanitize.FreshRun
		for run := 1; run <= 2; run++ {
			m.If(7).OutACL = [3]uint32{2, none, none}
			before := ifsanitize.Snapshot()
			rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop211")
			capped := ifsanitize.Snapshot().Capped["create"] != before.Capped["create"]
			t.Logf("r=%d run %d: needed %d, placeholders %d, cap %d, holes seen %d, rereads %d, capped %v, err %v", r, run, need, rep.Placeholders, rep.Cap, rep.Holes, rep.Rereads, rep.Capped, err)
			if need > ifsanitize.MaxPlaceholders {
				if !errors.Is(err, ifsanitize.ErrCapped) || !rep.Capped || !capped || rep.Placeholders != ifsanitize.MaxPlaceholders {
					t.Fatalf("r=%d run %d: want ErrCapped at %d placeholders", r, run, ifsanitize.MaxPlaceholders)
				}
			} else if err != nil || rep.Capped || capped || rep.Placeholders != need || m.Dirty(7) != "" {
				t.Fatalf("r=%d run %d: dirty %q", r, run, m.Dirty(7))
			}
			if len(m.Tables) != 4 {
				t.Fatalf("r=%d run %d: placeholders left: tables %v", r, run, m.Tables)
			}
		}
	}
}

// TestCappedFailsClosed: a free list that needs more than 64 placeholders (70 tables deleted in
// creation order, never ascending: 70 + FreshRun) is ErrCapped — ErrNoCleanIndex — at exactly 64,
// the capped counter moves, and every placeholder is deleted again.
func TestCappedFailsClosed(t *testing.T) {
	f, m := setup()
	for id := uint32(0); id < 70; id++ {
		m.Tables[id] = true
	}
	for id := uint32(0); id < 70; id++ {
		m.DeleteTable(id)
	}
	before := ifsanitize.Snapshot()
	rep, err := ifsanitize.Sanitize(context.Background(), f, 3, "loop208")
	if !errors.Is(err, ifsanitize.ErrCapped) || !errors.Is(err, ifsanitize.ErrNoCleanIndex) || !rep.Capped {
		t.Fatalf("err %v report %+v", err, rep)
	}
	if rep.Placeholders != ifsanitize.MaxPlaceholders || rep.Cap != 64 || len(m.Tables) != 0 {
		t.Fatalf("placeholders %d cap %d, tables left %v", rep.Placeholders, rep.Cap, m.Tables)
	}
	after := ifsanitize.Snapshot()
	if after.Capped["create"] != before.Capped["create"]+1 {
		t.Fatalf("capped counter %v → %v", before.Capped, after.Capped)
	}
	var b bytes.Buffer
	ifsanitize.WriteMetrics(&b)
	if !strings.Contains(b.String(), `vrx_agent_iface_sanitize_capped_total{phase="create"}`) {
		t.Fatalf("metrics lack the capped counter:\n%s", b.String())
	}
	t.Logf("capped: %v", err)
	// the delete phase never resurrects, so it is never capped
	if err := ifsanitize.BeforeDelete(context.Background(), f, 3, "loop208"); err != nil {
		t.Fatal(err)
	}
}
