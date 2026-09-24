package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// P08: the interfaces domain end to end on the fake VPP — af_packet creators, aliases, admin
// state, MTU (including a value equal to the creation default), description, VRF, addresses and a
// VLAN sub-interface; Retrieve == canonical desired; re-apply is empty; InterfaceState; rollback;
// loss + resync.

const ifDoc = `{
  "vrfs": {"blue": {"id": 1001}},
  "interfaces": {
    "host-w1l0": {"enabled": true, "description": "lan side", "mtu": 1400, "ipv4": ["10.1.1.1/24"]},
    "host-w1w0": {"enabled": true, "mtu": 9000, "ipv4": ["10.1.2.1/24"],
      "subinterfaces": {"100": {"vlanId": 100, "enabled": true, "vrf": "blue", "description": "tenant", "ipv4": ["10.1.100.1/24"]}}},
    "loop101": {"ipv6": ["2001:db8:1::1/64"]}
  }
}`

// canonicalIfDoc is what Retrieve must return for ifDoc (Zod defaults explicit, D-039).
const canonicalIfDoc = `{
  "vrfs": {"blue": {"id": 1001}},
  "interfaces": {
    "host-w1l0": {"enabled": true, "promiscuous": false, "description": "lan side", "mtu": 1400, "vrf": "default", "ipv4": ["10.1.1.1/24"]},
    "host-w1w0": {"enabled": true, "promiscuous": false, "mtu": 9000, "vrf": "default", "ipv4": ["10.1.2.1/24"],
      "subinterfaces": {"100": {"vlanId": 100, "dot1ad": false, "enabled": true, "vrf": "blue", "description": "tenant", "ipv4": ["10.1.100.1/24"]}}},
    "loop101": {"enabled": false, "promiscuous": false, "vrf": "default", "ipv6": ["2001:db8:1::1/64"]}
  }
}`

func retrieveIfs(t *testing.T, s *Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

func TestInterfacesDomainOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "i1", DesiredState: doc(t, ifDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	byKey := map[string]*vrxv1.ObjectResult{}
	for _, r := range resp.GetResults() {
		byKey[r.GetKey()] = r
	}
	for k, ptr := range map[string]string{
		"af-packet.host-interface/host-w1l0":       "/interfaces/host-w1l0",
		"interface/host-w1l0":                      "/interfaces/host-w1l0",
		"interface.admin-state/host-w1l0":          "/interfaces/host-w1l0/enabled",
		"interface.mtu/host-w1l0":                  "/interfaces/host-w1l0/mtu",
		"interface.mtu/host-w1w0":                  "/interfaces/host-w1w0/mtu", // 9000 = af_packet default: tolerated no-op
		"interface.subinterface/host-w1w0.100":     "/interfaces/host-w1w0/subinterfaces/100",
		"interface-ip.table/host-w1w0.100":         "/interfaces/host-w1w0/subinterfaces/100/vrf",
		"interface-ip/host-w1w0.100/10.1.100.1/24": "/interfaces/host-w1w0/subinterfaces/100/ipv4/0",
		"interface.loopback/loop101":               "/interfaces/loop101",
	} {
		if r := byKey[k]; r == nil || r.GetPointer() != ptr || r.GetSubsystem() != "interfaces" {
			t.Fatalf("result %s = %v, want pointer %s", k, r, ptr)
		}
	}
	if !v.AdminUp("host-w1l0") || v.MTU("host-w1l0") != 1400 || v.MTU("host-w1w0") != 9000 || !v.AdminUp("host-w1w0.100") || v.AdminUp("loop101") {
		t.Fatalf("VPP state: %s", v.Snapshot())
	}
	if got, want := retrieveIfs(t, s), doc(t, canonicalIfDoc); !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n%s", protojson.Format(got))
	}
	// Idempotent: an empty plan, only dumps.
	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "i2", DesiredState: doc(t, ifDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}
	// The retrieved document re-applies as a no-op too (running-vs-actual has no drift).
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "i3", DesiredState: doc(t, canonicalIfDoc)})
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-applying the retrieved document changed %v", resp.GetResults())
	}

	// InterfaceState: the live table.
	st, err := s.InterfaceState(context.Background(), &vrxv1.InterfaceStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]*vrxv1.InterfaceState{}
	for _, i := range st.GetInterfaces() {
		by[i.GetName()] = i
	}
	l0, sub := by["host-w1l0"], by["host-w1w0.100"]
	if l0 == nil || l0.GetType() != "af-packet" || !l0.GetAdminUp() || l0.GetMtu() != 1400 || l0.GetLinkMtu() != 9000 || l0.GetVrf() != "default" ||
		len(l0.GetIpv4()) != 1 || l0.GetIpv4()[0] != "10.1.1.1/24" || !l0.GetManaged() || l0.GetDescription() != "lan side" || l0.GetSwIfIndex() == 0 || l0.GetRxMode() != "interrupt" {
		t.Fatalf("host-w1l0 state %v", l0)
	}
	if sub == nil || sub.GetType() != "sub-interface" || sub.GetParent() != "host-w1w0" || sub.GetVlanId() != 100 || sub.GetVrf() != "blue" || sub.GetTableId() != 1001 || sub.GetDescription() != "tenant" {
		t.Fatalf("sub-interface state %v", sub)
	}
	if lo := by["loop101"]; lo == nil || lo.GetType() != "loopback" || lo.GetAdminUp() {
		t.Fatalf("loop101 state %v", lo)
	}
	if st, _ := s.InterfaceState(context.Background(), &vrxv1.InterfaceStateRequest{Names: []string{"loop101"}}); len(st.GetInterfaces()) != 1 {
		t.Fatalf("names filter %v", st)
	}

	// Rollback to a baseline without the MTU and the address: both leave VPP.
	base := doc(t, ifDoc)
	base.Interfaces["host-w1l0"].Mtu = nil
	base.Interfaces["host-w1l0"].Ipv4 = nil
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "i4", DesiredState: base}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if v.MTU("host-w1l0") != 9000 {
		t.Fatalf("MTU not restored to the default: %d", v.MTU("host-w1l0"))
	}
	got := retrieveIfs(t, s).GetInterfaces()["host-w1l0"]
	if got.Mtu != nil || len(got.GetIpv4()) != 0 {
		t.Fatalf("after rollback Retrieve still has %v", got)
	}

	// Simulated loss of both host-interfaces (and the sub-interface) → resync re-creates everything.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "i5", DesiredState: doc(t, ifDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.DeleteHostInterface("w1l0") || !v.DeleteHostInterface("w1w0") {
		t.Fatal("loss simulation")
	}
	if r := s.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync %v", r)
	}
	if got, want := retrieveIfs(t, s), doc(t, canonicalIfDoc); !proto.Equal(got, want) {
		t.Fatalf("after loss + resync:\n%s", protojson.Format(got))
	}

	// A restarted agent (new service over the same state dir) converges without touching VPP.
	s.Close()
	s2 := newSvc(t, v, dir)
	v.Reset()
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated()+r.GetSummary().GetDeleted()+r.GetSummary().GetUpdated() != 1 {
		// the only operation is the tolerated MTU 9000 no-op (per-process memory, re-learned once)
		t.Fatalf("restart resync %v", r)
	}
	for _, c := range v.Calls() {
		switch c.GetMessageName() {
		case "sw_interface_set_mtu", "sw_interface_set_flags", "af_packet_create_v3", "create_subif", "sw_interface_add_del_address":
			t.Fatalf("restart resync sent %s", c.GetMessageName())
		}
	}
}

// Physical / pre-existing interfaces: no creator, attributes claimed, observe-only alias; an
// interface the document names that does not exist fails validation-free with the alias' error.
func TestPhysicalInterfaceOnFake(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	s := newSvc(t, v, t.TempDir())
	d := `{"interfaces":{"lan":{"enabled":true,"mtu":1500}}}`
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: doc(t, d)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.AdminUp("lan") || v.MTU("lan") != 1500 {
		t.Fatalf("lan %s", v.Snapshot())
	}
	got := retrieveIfs(t, s).GetInterfaces()["lan"]
	if got == nil || !got.GetEnabled() || got.GetMtu() != 1500 {
		t.Fatalf("Retrieve lan %v", got)
	}
	st, _ := s.InterfaceState(context.Background(), &vrxv1.InterfaceStateRequest{Names: []string{"lan"}})
	if len(st.GetInterfaces()) != 1 || !st.GetInterfaces()[0].GetManaged() || st.GetInterfaces()[0].GetType() != "dpdk" {
		t.Fatalf("lan state %v", st)
	}
	// Removing it from the document: attributes are released (admin down, MTU default), the NIC stays.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p2", DesiredState: doc(t, `{"interfaces":{}}`), Subsystems: []string{"interfaces"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("lan"); !ok || v.AdminUp("lan") {
		t.Fatalf("lan after removal %s", v.Snapshot())
	}
	// A NIC that does not exist: the alias fails, nothing is applied.
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "p3", DesiredState: doc(t, `{"interfaces":{"wan":{"enabled":true}}}`)})
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("missing NIC applied: %v", resp)
	}
}

// Review F5: an MTU change TO the interface's creation default is a journaled recreate (DF-1 restores
// the default in a Delete, the tolerant wrapper's Create verifies it). When VPP does not end up at the
// default, the transaction rolls back AND VPP gets the old MTU back — nothing escapes the journal.
func TestMtuToDefaultIsJournaled(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	withMtu := func(mtu uint32) *vrxv1.DesiredState {
		d := doc(t, `{"interfaces":{"host-w1l0":{"enabled":true,"ipv4":["10.1.1.1/24"]}}}`)
		d.Interfaces["host-w1l0"].Mtu = proto.Uint32(mtu)
		return d
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "m1", DesiredState: withMtu(1400)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if v.MTU("host-w1l0") != 1400 {
		t.Fatalf("MTU %d", v.MTU("host-w1l0"))
	}

	// 1400 → 9000 (af_packet default): a recreate, VPP at the default, Retrieve reports 9000, re-apply is empty
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "m2", DesiredState: withMtu(9000)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var ops []string
	for _, r := range resp.GetResults() {
		ops = append(ops, r.GetKey()+":"+r.GetOp().String())
	}
	if len(ops) != 1 || ops[0] != "interface.mtu/host-w1l0:APPLY_OPERATION_RECREATE" {
		t.Fatalf("results %v", ops)
	}
	if v.MTU("host-w1l0") != 9000 {
		t.Fatalf("MTU after the change to the default: %d", v.MTU("host-w1l0"))
	}
	if got := retrieveIfs(t, s).GetInterfaces()["host-w1l0"].GetMtu(); got != 9000 {
		t.Fatalf("Retrieve mtu %d", got)
	}
	if r := apply(t, s, &vrxv1.ApplyRequest{TxnId: "m3", DesiredState: withMtu(9000)}); len(r.GetResults()) != 0 {
		t.Fatalf("re-apply %v", r.GetResults())
	}

	// back to 1400, then 1400 → 9000 while VPP does not take the default (it keeps 8999): the create
	// fails, the transaction rolls back and the journaled delete is undone — VPP has 1400 again
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "m4", DesiredState: withMtu(1400)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.SetMtuFilter(func(_ uint32, m [4]uint32) [4]uint32 {
		if m[0] == 9000 {
			m[0] = 8999
		}
		return m
	})
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "m5", DesiredState: withMtu(9000)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	v.SetMtuFilter(nil)
	if v.MTU("host-w1l0") != 1400 {
		t.Fatalf("after the rolled-back change to the default VPP has MTU %d, want the old 1400", v.MTU("host-w1l0"))
	}
	if got := retrieveIfs(t, s).GetInterfaces()["host-w1l0"].GetMtu(); got != 1400 {
		t.Fatalf("Retrieve mtu after rollback %d", got)
	}
}

// Re-review R1 (fix round 2): a value remembered as an untagged NIC's default (nothing written, nothing
// claimed) that changes to a non-default value is a recreate — the wrapper's Delete forgets it and
// DF-1's Create writes AND claims — so verification, Retrieve, re-apply and rollback work. A plain
// DF-1 Update would write without a claim: verify fails, the revert cannot undo the write (DEGRADED).
func TestRememberedDefaultThenChange(t *testing.T) {
	v := coretest.New()
	idx := v.AddInterface("lan", "dpdk", "")
	v.Ifaces[idx].LinkMtu = 1500
	v.Ifaces[idx].Mtu = [4]uint32{1500}
	v.Ifaces[idx].RxMode = interface_types.RX_MODE_API_POLLING
	s := newSvc(t, v, t.TempDir())
	set := func(txn, attrs string, want vrxv1.ApplyStatus) []string {
		t.Helper()
		resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: txn, DesiredState: doc(t, `{"interfaces":{"lan":{"enabled":true,`+attrs+`}}}`)})
		mustStatus(t, resp, want)
		var ops []string
		for _, r := range resp.GetResults() {
			ops = append(ops, r.GetKey()+":"+r.GetOp().String())
		}
		return ops
	}
	const applied, rolledBack = vrxv1.ApplyStatus_APPLY_STATUS_APPLIED, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK
	converged := func(txn, attrs string, mtu uint32) {
		t.Helper()
		if v.MTU("lan") != mtu {
			t.Fatalf("%s: VPP MTU %d, want %d", txn, v.MTU("lan"), mtu)
		}
		if got := retrieveIfs(t, s).GetInterfaces()["lan"].GetMtu(); got != mtu {
			t.Fatalf("%s: Retrieve mtu %d, want %d", txn, got, mtu)
		}
		if ops := set(txn+"-again", attrs, applied); len(ops) != 0 {
			t.Fatalf("%s: re-apply %v", txn, ops)
		}
	}

	set("r1", `"mtu":1500`, applied) // the link MTU: remembered, nothing written
	converged("r1", `"mtu":1500`, 1500)
	if ops := set("r2", `"mtu":1400`, applied); len(ops) != 1 || ops[0] != "interface.mtu/lan:APPLY_OPERATION_RECREATE" {
		t.Fatalf("remembered 1500 → 1400: results %v, want one RECREATE", ops)
	}
	converged("r2", `"mtu":1400`, 1400)
	set("r3", `"mtu":1500`, applied) // back to the default (F5: a recreate), remembered again
	converged("r3", `"mtu":1500`, 1500)
	set("r4", `"mtu":9000`, applied)
	converged("r4", `"mtu":9000`, 9000)

	// rollback from a remembered default: VPP does not take 1400 (keeps 1399) → ROLLED_BACK, VPP at 1500 again
	set("r5", `"mtu":1500`, applied)
	v.SetMtuFilter(func(_ uint32, m [4]uint32) [4]uint32 {
		if m[0] == 1400 {
			m[0] = 1399
		}
		return m
	})
	set("r6", `"mtu":1400`, rolledBack)
	v.SetMtuFilter(nil)
	converged("r6", `"mtu":1500`, 1500)

	// rx-mode: polling (the DPDK default, remembered) → interrupt
	set("x1", `"mtu":1500,"rxMode":"polling"`, applied)
	set("x2", `"mtu":1500,"rxMode":"interrupt"`, applied)
	if got, _ := v.InterfaceByName("lan"); got.RxMode != interface_types.RX_MODE_API_INTERRUPT {
		t.Fatalf("VPP rx-mode %v, want interrupt", got.RxMode)
	}
	if got := retrieveIfs(t, s).GetInterfaces()["lan"].GetRxMode(); got != "interrupt" {
		t.Fatalf("Retrieve rxMode %q, want interrupt", got)
	}
	if ops := set("x3", `"mtu":1500,"rxMode":"interrupt"`, applied); len(ops) != 0 {
		t.Fatalf("rx-mode re-apply %v", ops)
	}
}

// Review F4 / D-105: af_packet attaches only to a Linux veth. A netdev that exists and is not a veth
// is a validation error with the interface's pointer (nothing reaches VPP); a netdev that is missing
// passes validation (a vanished rig veth must not block unrelated commits) and the af_packet Create
// guard refuses it.
func TestAfPacketOnlyOnVeth(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, `{"interfaces":{
	  "host-ens192":{"enabled":true,"ipv4":["10.1.9.1/24"]},
	  "host-br-lab":{"enabled":true},
	  "host-w1l0":{"enabled":true}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	rules := map[string]string{}
	for _, e := range resp.GetValidation().GetErrors() {
		rules[e.GetPointer()] = e.GetRule() + ": " + e.GetMessage()
	}
	for _, p := range []string{"/interfaces/host-ens192", "/interfaces/host-br-lab"} {
		if !strings.HasPrefix(rules[p], "interfaces.af-packet-veth: ") {
			t.Errorf("%s: %q (all %v)", p, rules[p], rules)
		}
	}
	if _, bad := rules["/interfaces/host-w1l0"]; bad || !strings.Contains(rules["/interfaces/host-ens192"], `"ens192" is a physical`) {
		t.Errorf("issues %v", rules)
	}
	if n := len(v.CallsNamed("af_packet_create_v3")); n != 0 {
		t.Fatalf("validation failure sent %d af_packet_create_v3", n)
	}

	// missing netdev: validation passes, the Create guard refuses before VPP is asked
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "v2", DesiredState: doc(t, `{"interfaces":{"host-gone0":{"enabled":true}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	if !strings.Contains(resp.GetMessage(), `no Linux netdev "gone0"`) || len(v.CallsNamed("af_packet_create_v3")) != 0 {
		t.Fatalf("missing netdev: %v (af_packet_create_v3 calls %d)", resp.GetMessage(), len(v.CallsNamed("af_packet_create_v3")))
	}
}
