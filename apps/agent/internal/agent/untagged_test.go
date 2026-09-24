package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	afpapi "ngfw/agent/binapi/af_packet"
	interfaces "ngfw/agent/binapi/interface"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// TD-11c on the fake VPP with the product wiring (subsystems.Register, persisted claim stores).
//
// Review 3.1b: an address and a VRF binding on an UNTAGGED NIC (a DPDK NIC named by F-startup-gen,
// here "lan") apply through the agent — core's interface-ip / interface-ip.table claim the NIC in the
// persisted interface claim store (D-071 claim path, D-080 boot identity + sw_if_index) — Retrieve ==
// desired, a restarted agent converges without touching VPP, removal releases the claims, and an
// address somebody else put on the NIC is never reported or removed.
//
// Review 3.1c: deletes run dependents-first through the observe-only interface/<name> alias — a
// sub-interface removed while its parent stays (F-vlan-qinq Q1) and an af_packet interface removed
// with its address, VRF binding, admin state and MTU. The af_packet netdevs (zq11c*) do not exist on
// the host, so the quiesce's netlink lookup finds nothing to bring down (no host netdev is touched).

const nicDoc = `{
  "vrfs": {"blue": {"id": 10001}},
  "interfaces": {
    "lan": {"enabled": true, "vrf": "blue", "ipv4": ["10.10.1.1/24"], "ipv6": ["2001:db8:a::1/64"]},
    "wan": {"ipv4": ["10.10.3.1/24"]}
  }
}`

const nicCanonical = `{
  "vrfs": {"blue": {"id": 10001}},
  "interfaces": {
    "lan": {"enabled": true, "promiscuous": false, "vrf": "blue", "ipv4": ["10.10.1.1/24"], "ipv6": ["2001:db8:a::1/64"]},
    "wan": {"enabled": false, "promiscuous": false, "vrf": "default", "ipv4": ["10.10.3.1/24"]}
  }
}`

func claimFile(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "claims-iface-"+testOwner+".json")) //nolint:gosec // the test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func sent(v *coretest.VPP, names ...string) []string {
	var out []string
	for _, c := range v.Calls() {
		for _, n := range names {
			if c.GetMessageName() == n {
				out = append(out, n)
			}
		}
	}
	return out
}

func TestPhysicalNICAddressAndVRF(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	wan := v.AddInterface("wan", "dpdk", "")
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n1", DesiredState: doc(t, nicDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if i, _ := v.InterfaceByName("lan"); i.Table4 != 10001 || i.Table6 != 10001 || !i.Addrs["10.10.1.1/24"] || !i.Addrs["2001:db8:a::1/64"] || !i.AdminUp {
		t.Fatalf("VPP lan %+v", i)
	}
	if got, want := retrieveIfs(t, s), doc(t, nicCanonical); !proto.Equal(got, want) {
		t.Fatalf("Retrieve != canonical desired:\n%s", protojson.Format(got))
	}
	for _, rec := range []string{`"lan|interface-ip.table"`, `"lan|interface-ip|10.10.1.1/24"`, `"lan|interface-ip|2001:db8:a::1/64"`} {
		if f := claimFile(t, dir); !strings.Contains(f, rec) {
			t.Fatalf("claim %s not persisted:\n%s", rec, f)
		}
	}

	// Somebody else's address on a NIC we hold an address on (linux-nl, a DHCP lease, vppctl): never
	// ours — not reported, not removed. (On lan it would block the VRF unbind: VPP refuses a table
	// change while the interface has an address of that family, ADDRESS_FOUND_FOR_INTERFACE.)
	v.Ifaces[wan].Addrs["192.0.2.1/24"] = true
	if r := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n2", DesiredState: doc(t, nicDoc)}); len(r.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", r.GetResults())
	}
	if got, want := retrieveIfs(t, s), doc(t, nicCanonical); !proto.Equal(got, want) {
		t.Fatalf("Retrieve reports a foreign address:\n%s", protojson.Format(got))
	}

	// Restart (new service over the same state dir): the claims are loaded, nothing is written.
	s.Close()
	s2 := newSvc(t, v, dir)
	v.Reset()
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED ||
		r.GetSummary().GetCreated()+r.GetSummary().GetDeleted()+r.GetSummary().GetUpdated() != 0 {
		t.Fatalf("restart resync %v", r)
	}
	if w := sent(v, "sw_interface_add_del_address", "sw_interface_set_table", "sw_interface_set_flags"); len(w) != 0 {
		t.Fatalf("restart resync wrote %v", w)
	}
	if got, want := retrieveIfs(t, s2), doc(t, nicCanonical); !proto.Equal(got, want) {
		t.Fatalf("after restart:\n%s", protojson.Format(got))
	}

	// Removal: our addresses and binding leave VPP, the claims are released, the NICs and the foreign
	// address stay.
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "n3", DesiredState: doc(t, `{"interfaces":{}}`), Subsystems: []string{"interfaces"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if i, ok := v.InterfaceByName("lan"); !ok || i.Table4 != 0 || i.Table6 != 0 || len(i.Addrs) != 0 {
		t.Fatalf("lan after removal %+v", i)
	}
	if i, ok := v.InterfaceByName("wan"); !ok || len(i.Addrs) != 1 || !i.Addrs["192.0.2.1/24"] {
		t.Fatalf("wan after removal %+v", i)
	}
	if f := claimFile(t, dir); strings.Contains(f, "interface-ip") {
		t.Fatalf("claims not released:\n%s", f)
	}
}

// F-vlan-qinq Q1 through the scheduler (no KeyProvider on the sub-interface needed): removing an
// enabled sub-interface with an address while its parent stays deletes the attributes first.
func TestSubinterfaceRemovedWhileParentStays(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	withSub := `{"interfaces":{"host-zq11c0":{"enabled":true,
	  "subinterfaces":{"100":{"vlanId":100,"enabled":true,"ipv4":["10.10.100.1/24"]}}}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "s1", DesiredState: doc(t, withSub)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if !v.AdminUp("host-zq11c0.100") {
		t.Fatalf("sub-interface %s", v.Snapshot())
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "s2", DesiredState: doc(t, `{"interfaces":{"host-zq11c0":{"enabled":true}}}`)})
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		var rs []string
		for _, r := range resp.GetResults() {
			rs = append(rs, r.GetKey()+" "+r.GetOp().String()+" "+r.GetCode().String()+" "+r.GetMessage())
		}
		t.Fatalf("removing the sub-interface: %s %s\n  %s", resp.GetStatus(), resp.GetMessage(), strings.Join(rs, "\n  "))
	}
	if _, ok := v.InterfaceByName("host-zq11c0.100"); ok {
		t.Fatalf("sub-interface still in VPP %s", v.Snapshot())
	}
	if !v.AdminUp("host-zq11c0") {
		t.Fatal("parent lost its admin state")
	}
}

// An af_packet interface (registered after core) removed with its address, VRF binding, admin state
// and MTU: every attribute is deleted in VPP before af_packet_delete. Before TD-11c af_packet_delete
// ran first and the address/binding deletes found no owned interface (a silent no-op).
func TestInterfaceRemovedDeletesAttributesFirst(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	d := `{"vrfs":{"blue":{"id":10001}},"interfaces":{"host-zq11c1":{"enabled":true,"mtu":1400,"vrf":"blue","ipv4":["10.10.2.1/24"]}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", DesiredState: doc(t, d)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.Reset()
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a2", DesiredState: doc(t, `{"interfaces":{}}`), Subsystems: []string{"interfaces"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var seq []string
	for _, c := range v.Calls() {
		switch m := c.(type) {
		case *interfaces.SwInterfaceAddDelAddress:
			if !m.IsAdd {
				seq = append(seq, "address-del")
			}
		case *interfaces.SwInterfaceSetTable:
			seq = append(seq, "table-unbind")
		case *interfaces.SwInterfaceSetFlags:
			seq = append(seq, "admin-down")
		case *interfaces.SwInterfaceSetMtu:
			seq = append(seq, "mtu-default")
		case *afpapi.AfPacketDelete:
			seq = append(seq, "af_packet_delete")
		}
	}
	last := strings.Join(seq, ",")
	if !strings.HasSuffix(last, "af_packet_delete") {
		t.Fatalf("af_packet_delete is not the last write: %s", last)
	}
	for _, w := range []string{"address-del", "table-unbind", "admin-down", "mtu-default"} {
		if !strings.Contains(last, w) {
			t.Fatalf("%s never sent (deleted after the interface was gone?): %s", w, last)
		}
	}
	if _, ok := v.InterfaceByName("host-zq11c1"); ok {
		t.Fatalf("interface still in VPP %s", v.Snapshot())
	}
}
