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
	want := []string{"ip-classify ip4", "ip-classify ip6", "l2-classify input", "l2-classify output", "adl adl-input", "vxlan-bypass ip4", "vxlan-bypass ip6"}
	if !slices.Equal(rep.Reset, want) {
		t.Fatalf("reset %v, want %v", rep.Reset, want)
	}
	// one probe per live table and slot: output acl ×3, policer ×3, flow ×2 — all NO_SUCH_TABLE
	for name, n := range map[string]int{"output_acl_set_interface": 3, "policer_classify_set_interface": 3, "flow_classify_set_interface": 2, "input_acl_set_interface": 0, "ipsec_interface_add_del_spd": 0} {
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

func TestBindingToDeletedTable(t *testing.T) {
	f, m := setup()
	m.Tables[3] = true
	s := m.If(4)
	s.InACL = [3]uint32{9, none, none} // table 9 is gone: VPP refuses the unbind
	rep, err := ifsanitize.Sanitize(context.Background(), f, 4, "loop202")
	if err != nil {
		t.Fatalf("a dormant stale binding must not fail the create: %v", err)
	}
	if len(rep.Unclearable) != 1 || !strings.Contains(rep.Unclearable[0], "input-acl ip4 table 9") {
		t.Fatalf("unclearable %v", rep.Unclearable)
	}
	if n := len(f.CallsNamed("input_acl_set_interface")); n != 0 {
		t.Fatalf("an unbind naming a freed table was sent (%d)", n)
	}
	// feature_is_enabled is never asked: VPP 26.06 answers true for any error (out-of-range
	// sw_if_index on the arc), so it cannot tell an inherited feature from a new index
	if n := len(f.CallsNamed("feature_is_enabled")); n != 0 {
		t.Fatalf("feature_is_enabled sent %d times", n)
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
	if after.Runs != before.Runs+1 || after.Errors != before.Errors+1 {
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
	if after.Inherited != before.Inherited+1 || after.Cleared["input-acl"] != before.Cleared["input-acl"]+1 || after.Cleared["output-acl"] != before.Cleared["output-acl"]+1 {
		t.Fatalf("counters %+v → %+v", before, after)
	}
	var b bytes.Buffer
	ifsanitize.WriteMetrics(&b)
	for _, want := range []string{"vrx_agent_iface_sanitize_total ", `vrx_agent_iface_sanitize_cleared_total{state="input-acl"}`, "vrx_agent_iface_sanitize_inherited_total "} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("metrics lack %q:\n%s", want, b.String())
		}
	}
}
