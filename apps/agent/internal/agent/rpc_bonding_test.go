package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/core/coretest"
	iface "ngfw/agent/internal/descriptors/interface"
)

// F-bonding on the fake VPP with DF-1's real descriptors: an LACP bond of two untagged NICs (fixture taps, claimed
// through the persisted ClaimStore) that also carries an MTU, an address and a VLAN sub-interface — a bond behaves
// like any interface; Retrieve == canonical desired; BondState; an active-backup bond with weights (in-place
// update); a mode change re-creates the bond with its dependents; rollback deletes the address before the bond
// (KeyProvider) and leaves the NICs as plain interfaces; loss + resync; validation pointers.

const bondDoc = `{
  "interfaces": {
    "BondEthernet6000": {"enabled": true, "mtu": 1500, "ipv4": ["10.6.10.1/24"],
      "bond": {"mode": "lacp", "loadBalance": "l34",
        "members": {"tap6000": {}, "tap6001": {"passive": true, "longTimeout": true}}},
      "subinterfaces": {"100": {"vlanId": 100, "enabled": true, "ipv4": ["10.6.100.1/24"]}}},
    "tap6000": {"enabled": true},
    "tap6001": {"enabled": true}
  }
}`

const canonicalBondDoc = `{
  "interfaces": {
    "BondEthernet6000": {"enabled": true, "promiscuous": false, "mtu": 1500, "vrf": "default", "ipv4": ["10.6.10.1/24"],
      "bond": {"mode": "lacp", "loadBalance": "l34", "numaOnly": false,
        "members": {"tap6000": {"passive": false, "longTimeout": false}, "tap6001": {"passive": true, "longTimeout": true}}},
      "subinterfaces": {"100": {"vlanId": 100, "dot1ad": false, "enabled": true, "vrf": "default", "ipv4": ["10.6.100.1/24"]}}},
    "tap6000": {"enabled": true, "promiscuous": false, "vrf": "default"},
    "tap6001": {"enabled": true, "promiscuous": false, "vrf": "default"}
  }
}`

func bondFake(t *testing.T) (*coretest.VPP, *Service) {
	t.Helper()
	v := coretest.New()
	v.AddInterface("tap6000", "virtio", "") // fixture taps: untagged, like physical NICs
	v.AddInterface("tap6001", "virtio", "")
	v.AddInterface("tap6002", "virtio", "")
	return v, newSvc(t, v, t.TempDir())
}

func resultsByKey(resp *vrxv1.ApplyResponse) map[string]*vrxv1.ObjectResult {
	out := map[string]*vrxv1.ObjectResult{}
	for _, r := range resp.GetResults() {
		out[r.GetKey()] = r
	}
	return out
}

func TestBondingOnFake(t *testing.T) {
	v, s := bondFake(t)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "b1", DesiredState: doc(t, bondDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	by := resultsByKey(resp)
	for k, ptr := range map[string]string{
		"bond.bond/BondEthernet6000":                      "/interfaces/BondEthernet6000/bond",
		"bond.member/BondEthernet6000/tap6000":            "/interfaces/BondEthernet6000/bond/members/tap6000",
		"bond.member/BondEthernet6000/tap6001":            "/interfaces/BondEthernet6000/bond/members/tap6001",
		"interface.admin-state/BondEthernet6000":          "/interfaces/BondEthernet6000/enabled",
		"interface.mtu/BondEthernet6000":                  "/interfaces/BondEthernet6000/mtu",
		"interface-ip/BondEthernet6000/10.6.10.1/24":      "/interfaces/BondEthernet6000/ipv4/0",
		"interface.subinterface/BondEthernet6000.100":     "/interfaces/BondEthernet6000/subinterfaces/100",
		"interface-ip/BondEthernet6000.100/10.6.100.1/24": "/interfaces/BondEthernet6000/subinterfaces/100/ipv4/0",
		"interface.admin-state/tap6000":                   "/interfaces/tap6000/enabled",
	} {
		if r := by[k]; r == nil || r.GetPointer() != ptr || r.GetSubsystem() != "interfaces" {
			t.Fatalf("result %s = %v, want pointer %s", k, r, ptr)
		}
	}
	b, ok := v.Bond("BondEthernet6000")
	if !ok || b.ID != 6000 || b.Mode != bondapi.BOND_API_MODE_LACP || b.Lb != bondapi.BOND_API_LB_ALGO_L34 {
		t.Fatalf("VPP bond %+v %v", b, ok)
	}
	ms := v.BondMembers("BondEthernet6000")
	if len(ms) != 2 || ms["tap6000"].Passive || !ms["tap6001"].Passive || !ms["tap6001"].LongTimeout {
		t.Fatalf("VPP members %+v", ms)
	}
	if row, _ := v.InterfaceByName("BondEthernet6000"); row.Tag != testOwner+":BondEthernet6000" || !row.AdminUp || row.Mtu[0] != 1500 {
		t.Fatalf("bond interface %+v", row)
	}
	// untagged members are ours through the persisted claims (D-075), never tagged
	for _, m := range []string{"tap6000", "tap6001"} {
		if row, _ := v.InterfaceByName(m); row.Tag != "" {
			t.Fatalf("%s tagged %q", m, row.Tag)
		}
		if !iface.Claims(testOwner).Claimed(m, bond.MemberName) {
			t.Fatalf("no bond.member claim on %s", m)
		}
	}
	if got, want := retrieveIfs(t, s), doc(t, canonicalBondDoc); !proto.Equal(got.GetInterfaces()["BondEthernet6000"], want.GetInterfaces()["BondEthernet6000"]) || !proto.Equal(got, &vrxv1.DesiredState{Interfaces: want.GetInterfaces()}) {
		t.Fatalf("Retrieve != canonical desired:\n%s", protojson.Format(got))
	}
	v.Reset()
	if resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "b2", DesiredState: doc(t, canonicalBondDoc)}); len(resp.GetResults()) != 0 {
		t.Fatalf("re-applying the retrieved document changed %v", resp.GetResults())
	}

	// BondState: the live view.
	st, err := s.BondState(context.Background(), &vrxv1.BondStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetBonds()) != 1 || st.GetOwner() != testOwner {
		t.Fatalf("BondState %v", st)
	}
	bs := st.GetBonds()[0]
	if bs.GetName() != "BondEthernet6000" || bs.GetVppName() != "BondEthernet6000" || bs.GetId() != 6000 || bs.GetMode() != "lacp" || bs.GetLoadBalance() != "l34" ||
		!bs.GetAdminUp() || bs.GetMemberCount() != 2 || bs.GetActiveMemberCount() != 0 || len(bs.GetMembers()) != 2 {
		t.Fatalf("bond status %v", bs)
	}
	m0, m1 := bs.GetMembers()[0], bs.GetMembers()[1]
	if m0.GetInterface() != "tap6000" || !m0.GetAdminUp() || m0.GetLacp().GetRxState() != "defaulted" || m0.GetLacp().GetMuxState() != "detached" ||
		m0.GetLacp().GetPtxState() != "fast-periodic" || strings.Join(m0.GetLacp().GetActor().GetStateFlags(), ",") != "activity,timeout,aggregation,defaulted" ||
		m0.GetLacp().GetPartner().GetSystem() != "00:00:00:00:00:00" || m0.GetLacp().GetActor().GetKey() != 6000 {
		t.Fatalf("member 0 %v", m0)
	}
	if m1.GetInterface() != "tap6001" || !m1.GetPassive() || !m1.GetLongTimeout() || strings.Contains(strings.Join(m1.GetLacp().GetActor().GetStateFlags(), ","), "timeout") {
		t.Fatalf("member 1 %v", m1)
	}
	if st, _ := s.BondState(context.Background(), &vrxv1.BondStateRequest{Names: []string{"BondEthernet1"}}); len(st.GetBonds()) != 0 {
		t.Fatalf("names filter %v", st)
	}
	if _, err := s.BondState(context.Background(), &vrxv1.BondStateRequest{Owner: "w9"}); err == nil {
		t.Fatal("foreign owner accepted")
	}

	// Rollback to a baseline without the bond: address, MTU, admin state, sub-interface and memberships go before the
	// bond (bond.bond provides interface/BondEthernet6000), the NICs stay as plain L3 interfaces, the claims are released.
	// The sub-interface's own attributes are dropped in a first commit: their dependency on the observed-only alias
	// interface/BondEthernet6000.100 gives the planner no edge to interface.subinterface (TD-11c 3.1c; the same holds
	// for a sub-interface of any parent), so they cannot yet be deleted in the same transaction as the sub-interface.
	noSubAttrs := doc(t, bondDoc)
	noSubAttrs.Interfaces["BondEthernet6000"].Subinterfaces["100"] = &vrxv1.Subinterface{VlanId: proto.Uint32(100)}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "b3a", DesiredState: noSubAttrs}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	v.Reset()
	base := doc(t, `{"interfaces": {"tap6000": {"enabled": true}, "tap6001": {"enabled": true}}}`)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "b3", DesiredState: base}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var order []string
	for _, c := range v.Calls() {
		switch m := c.(type) {
		case *ifapi.SwInterfaceAddDelAddress, *ifapi.DeleteSubif, *bondapi.BondDetachMember, *bondapi.BondDelete:
			order = append(order, m.GetMessageName())
		}
	}
	if got := strings.Join(order, " "); !strings.HasSuffix(got, "bond_delete") || strings.Index(got, "sw_interface_add_del_address") > strings.Index(got, "bond_delete") ||
		strings.Index(got, "delete_subif") > strings.Index(got, "bond_delete") || strings.Count(got, "bond_detach_member") != 2 {
		t.Fatalf("rollback order: %s", got)
	}
	if v.BondCount() != 0 {
		t.Fatalf("bonds left: %d", v.BondCount())
	}
	got := retrieveIfs(t, s)
	if _, ok := got.GetInterfaces()["BondEthernet6000"]; ok || len(got.GetInterfaces()) != 2 || got.GetInterfaces()["tap6000"].GetBond() != nil {
		t.Fatalf("after rollback Retrieve = %s", protojson.Format(got))
	}
	for _, m := range []string{"tap6000", "tap6001"} {
		if iface.Claims(testOwner).Claimed(m, bond.MemberName) {
			t.Fatalf("claim on %s not released", m)
		}
		if row, ok := v.InterfaceByName(m); !ok || !row.AdminUp {
			t.Fatalf("%s after rollback %+v", m, row)
		}
	}

	// Loss behind the agent's back (members first, then the bond — D-095c) → resync re-creates all of it.
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "b4", DesiredState: doc(t, bondDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	b, _ = v.Bond("BondEthernet6000")
	ctx := context.Background()
	for _, m := range []string{"tap6000", "tap6001"} {
		row, _ := v.InterfaceByName(m)
		if _, err := bondapi.NewServiceClient(v).BondDetachMember(ctx, &bondapi.BondDetachMember{SwIfIndex: ifIndex(row.Index)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := bondapi.NewServiceClient(v).BondDelete(ctx, &bondapi.BondDelete{SwIfIndex: ifIndex(b.Index)}); err != nil {
		t.Fatal(err)
	}
	if r := s.Resync(ctx); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync %v", r)
	}
	if got, want := retrieveIfs(t, s), doc(t, canonicalBondDoc); !proto.Equal(got, &vrxv1.DesiredState{Interfaces: want.GetInterfaces()}) {
		t.Fatalf("after resync Retrieve != canonical desired:\n%s", protojson.Format(got))
	}
}

func TestBondingActiveBackupWeightsAndModeChange(t *testing.T) {
	v, s := bondFake(t)
	ab := `{"interfaces": {
	  "BondEthernet6001": {"enabled": true, "ipv4": ["10.6.11.1/24"],
	    "bond": {"mode": "active-backup", "id": 6001, "members": {"tap6000": {"weight": 200}, "tap6001": {"weight": 100}, "tap6002": {}}}},
	  "tap6000": {"enabled": true}, "tap6001": {"enabled": true}, "tap6002": {"enabled": true}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "w1", DesiredState: doc(t, ab)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	b, _ := v.Bond("BondEthernet6001")
	ms := v.BondMembers("BondEthernet6001")
	if b.Mode != bondapi.BOND_API_MODE_ACTIVE_BACKUP || b.Lb != bondapi.BOND_API_LB_ALGO_AB || ms["tap6000"].Weight != 200 || ms["tap6001"].Weight != 100 || ms["tap6002"].Weight != 0 {
		t.Fatalf("VPP %+v %+v", b, ms)
	}
	got := retrieveIfs(t, s).GetInterfaces()["BondEthernet6001"].GetBond()
	want := &vrxv1.Bond{Mode: proto.String("active-backup"), NumaOnly: proto.Bool(false), Id: proto.Uint32(6001), Members: map[string]*vrxv1.BondMember{
		"tap6000": {Passive: proto.Bool(false), LongTimeout: proto.Bool(false), Weight: proto.Uint32(200)},
		"tap6001": {Passive: proto.Bool(false), LongTimeout: proto.Bool(false), Weight: proto.Uint32(100)},
		"tap6002": {Passive: proto.Bool(false), LongTimeout: proto.Bool(false)},
	}}
	if !proto.Equal(got, want) { // no loadBalance on an active-backup bond; id because the document sets it
		t.Fatalf("Retrieve bond = %v", got)
	}
	st, _ := s.BondState(context.Background(), &vrxv1.BondStateRequest{})
	if bs := st.GetBonds()[0]; bs.GetLoadBalance() != "active-backup" || bs.GetActiveMemberCount() != 3 || bs.GetMembers()[0].GetWeight() != 200 || bs.GetMembers()[0].GetLacp() != nil {
		t.Fatalf("BondState %v", bs)
	}

	// a weight change is an in-place update of one object
	v.Reset()
	upd := strings.Replace(ab, `"weight": 100`, `"weight": 250`, 1)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "w2", DesiredState: doc(t, upd)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetKey() != "bond.member-weight/BondEthernet6001/tap6001" || resp.GetResults()[0].GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_UPDATE ||
		v.BondMembers("BondEthernet6001")["tap6001"].Weight != 250 || len(v.CallsNamed("bond_delete")) != 0 {
		t.Fatalf("weight update %v, VPP %+v", resp.GetResults(), v.BondMembers("BondEthernet6001"))
	}

	// a mode change re-creates the bond; its members, address and admin state come back with it
	xor := `{"interfaces": {
	  "BondEthernet6001": {"enabled": true, "ipv4": ["10.6.11.1/24"],
	    "bond": {"mode": "xor", "loadBalance": "l23", "id": 6001, "members": {"tap6000": {}, "tap6001": {}}}},
	  "tap6000": {"enabled": true}, "tap6001": {"enabled": true}, "tap6002": {"enabled": true}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "w3", DesiredState: doc(t, xor)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	b, _ = v.Bond("BondEthernet6001")
	row, _ := v.InterfaceByName("BondEthernet6001")
	if b.Mode != bondapi.BOND_API_MODE_XOR || b.Lb != bondapi.BOND_API_LB_ALGO_L23 || len(v.BondMembers("BondEthernet6001")) != 2 || !row.AdminUp || !row.Addrs["10.6.11.1/24"] {
		t.Fatalf("after mode change: %+v %+v %+v", b, v.BondMembers("BondEthernet6001"), row)
	}
	if got := retrieveIfs(t, s).GetInterfaces()["BondEthernet6001"].GetBond(); got.GetMode() != "xor" || got.GetLoadBalance() != "l23" || len(got.GetMembers()) != 2 {
		t.Fatalf("Retrieve after mode change %v", got)
	}
}

func TestBondingValidation(t *testing.T) {
	_, s := bondFake(t)
	for _, tc := range []struct{ doc, pointer, rule string }{
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "lacp", "members": {"tap6000": {}}}},
		   "BondEthernet6001": {"bond": {"mode": "xor", "members": {"tap6000": {}}}}, "tap6000": {"enabled": true}}}`,
			"/interfaces/BondEthernet6001/bond/members/tap6000", "interfaces.bonding-member-unique"},
		{`{"interfaces": {"lag0": {"bond": {"mode": "lacp"}}}}`, "/interfaces/lag0/bond", "interfaces.bonding-name"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "lacp", "id": 6001}}}}`, "/interfaces/BondEthernet6000/bond/id", "interfaces.bonding-name"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "lag"}}}}`, "/interfaces/BondEthernet6000/bond/mode", "interfaces.bonding-mode"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "round-robin", "loadBalance": "l34"}}}}`, "/interfaces/BondEthernet6000/bond/loadBalance", "interfaces.bonding-load-balance"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "xor", "members": {"tap6009": {}}}}}}`, "/interfaces/BondEthernet6000/bond/members/tap6009", "interfaces.bonding-member-exists"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "xor", "members": {"loop6000": {}}}}, "loop6000": {}}}`, "/interfaces/BondEthernet6000/bond/members/loop6000", "interfaces.bonding-member-kind"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "xor", "members": {"tap6000": {"passive": true}}}}, "tap6000": {}}}`, "/interfaces/BondEthernet6000/bond/members/tap6000", "interfaces.bonding-lacp-options"},
		{`{"interfaces": {"BondEthernet6000": {"bond": {"mode": "lacp", "members": {"tap6000": {"weight": 5}}}}, "tap6000": {}}}`, "/interfaces/BondEthernet6000/bond/members/tap6000/weight", "interfaces.bonding-weight"},
	} {
		rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "v", DesiredState: doc(t, tc.doc)})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, is := range rep.GetErrors() {
			if is.GetPointer() == tc.pointer && is.GetRule() == tc.rule && is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
				found = true
			}
		}
		if !found || rep.GetOk() {
			t.Fatalf("%s: want %s at %s, got %v", tc.doc, tc.rule, tc.pointer, rep.GetErrors())
		}
	}
	// a BondEthernet<id> without a bond leaf is a pre-existing bond: an alias without a creator
	p := project(doc(t, `{"interfaces": {"BondEthernet7": {"enabled": true}}}`), []string{"interfaces"}, nil, nil)
	for _, kv := range p.kvs {
		if a, ok := kv.Value.(*iface.InterfaceAlias); ok && (a.GetName() != "BondEthernet7" || a.GetCreator() != "") {
			t.Fatalf("alias %v", a)
		}
		if kv.Key.Descriptor() == bond.BondName {
			t.Fatalf("a bond without a bond leaf is created: %v", kv)
		}
	}
}

func ifIndex(i uint32) interface_types.InterfaceIndex { return interface_types.InterfaceIndex(i) }
