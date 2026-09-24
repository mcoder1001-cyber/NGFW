package ifsanitize_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/adapter"

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
	defer func(n int) { ifsanitize.MaxPlaceholders = n }(ifsanitize.MaxPlaceholders)
	ifsanitize.MaxPlaceholders = 0
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
