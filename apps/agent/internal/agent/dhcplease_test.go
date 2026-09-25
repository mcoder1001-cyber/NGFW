package agent

import (
	"context"
	"slices"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// TD-24 (F-kea review Q5, rated H) through the product wiring: a VLAN sub-interface of an af_packet
// interface runs `dhcpClient` (DF-8's dhcp.client), VPP binds a lease on it, and every later
// reconcile — a commit, the agent's resync, a restarted agent's resync — must leave the lease alone.
// The running document never shows the lease (it is state: /state/interfaces/{name}/dhcp-client),
// so there is no drift either; the live interface table still lists it.

const leaseDoc = `{
  "interfaces": {
    "host-w1l0": {"enabled": true, "ipv4": ["10.1.1.1/24"]},
    "host-w1w0": {"enabled": true,
      "subinterfaces": {"100": {"vlanId": 100, "enabled": true, "dhcpClient": {"hostname": "w1-wan"}, "ipv6": ["2001:db8:1:100::1/64"]}}}
  }
}`

const canonicalLeaseDoc = `{
  "vrfs": {},
  "interfaces": {
    "host-w1l0": {"enabled": true, "promiscuous": false, "vrf": "default", "ipv4": ["10.1.1.1/24"]},
    "host-w1w0": {"enabled": true, "promiscuous": false, "vrf": "default",
      "subinterfaces": {"100": {"vlanId": 100, "dot1ad": false, "enabled": true, "vrf": "default",
        "dhcpClient": {"hostname": "w1-wan", "setBroadcastFlag": false}, "ipv6": ["2001:db8:1:100::1/64"]}}}
  }
}`

const (
	leaseIf   = "host-w1w0.100"
	leaseAddr = "10.1.100.50/24"
)

func leaseAddrDeletes(v *coretest.VPP) []string {
	var out []string
	for _, m := range v.CallsNamed("sw_interface_add_del_address") {
		if r := m.(*interfaces.SwInterfaceAddDelAddress); !r.IsAdd {
			out = append(out, r.Prefix.String())
		}
	}
	return out
}

func TestDHCPLeaseSurvivesReconcileResyncRestart(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "l1", DesiredState: doc(t, leaseDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if c, ok := v.DHCPClientOf(leaseIf); !ok || c.Lease.IsValid() {
		t.Fatalf("dhcp client on %s: %+v %v", leaseIf, c, ok)
	}
	// the server ACKs: VPP installs the lease on the sub-interface
	if err := v.BindLease(leaseIf, leaseAddr); err != nil {
		t.Fatal(err)
	}
	v.Reset()

	check := func(step string) {
		t.Helper()
		if d := leaseAddrDeletes(v); len(d) != 0 {
			t.Fatalf("%s: address deletes %v", step, d)
		}
		if i, ok := v.InterfaceByName(leaseIf); !ok || !i.Addrs[leaseAddr] || !i.Addrs["2001:db8:1:100::1/64"] {
			t.Fatalf("%s: VPP %s", step, v.Snapshot())
		}
		if got, want := retrieveIfs(t, s), doc(t, canonicalLeaseDoc); !proto.Equal(got, want) {
			t.Fatalf("%s: Retrieve != canonical desired (the lease is not config):\n%s", step, protojson.Format(got))
		}
	}
	// a commit of the same document, then two resyncs
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "l2", DesiredState: doc(t, leaseDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply with a bound lease changed %v", resp.GetResults())
	}
	check("commit")
	for i := range 2 {
		if r := s.Resync(ctx); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetDeleted() != 0 {
			t.Fatalf("resync %d: %v", i, r)
		}
	}
	check("resync")

	// a restarted agent (new service over the same state dir) resyncs twice
	s.Close()
	s = newSvc(t, v, dir)
	for i := range 2 {
		if r := s.Resync(ctx); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetDeleted() != 0 {
			t.Fatalf("restart resync %d: %v", i, r)
		}
	}
	check("restart")

	// the live table still shows the lease (state, not config)
	st, err := s.InterfaceState(ctx, &vrxv1.InterfaceStateRequest{Names: []string{leaseIf}})
	if err != nil || len(st.GetInterfaces()) != 1 || !slices.Contains(st.GetInterfaces()[0].GetIpv4(), leaseAddr) {
		t.Fatalf("InterfaceState %v %v", st, err)
	}

	// switching the sub-interface from DHCP to a static IPv4 equal to the old lease: the client is
	// deleted first (VPP releases the lease), then the static address is added
	static := doc(t, leaseDoc)
	sub := static.Interfaces["host-w1w0"].Subinterfaces["100"]
	sub.DhcpClient, sub.Ipv4 = nil, []string{leaseAddr}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "l3", DesiredState: static}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.DHCPClientOf(leaseIf); ok {
		t.Fatalf("dhcp client still on %s", leaseIf)
	}
	if got := retrieveIfs(t, s).GetInterfaces()["host-w1w0"].GetSubinterfaces()["100"].GetIpv4(); !slices.Equal(got, []string{leaseAddr}) {
		t.Fatalf("static ipv4 after the switch: %v", got)
	}
}
