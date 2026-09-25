package core_test

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/dhcp"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// TD-24 (F-kea review Q5, rated H): VPP's DHCPv4 client installs its lease with the ordinary
// interface-address call, so ip_address_dump lists it next to our addresses. interface-ip's Retrieve
// used to report it on every tagged interface; the lease is never desired, so the next reconcile or
// resync deleted it (sw_interface_add_del_address is_add=0). Retrieve now leaves out exactly the
// installed lease that dhcp_client_dump reports for that sw_if_index, with one dump per Retrieve
// (D-132).

const (
	leaseOwner = "w7"
	leaseSub   = "host-w7c0.100" // tagged VLAN sub-interface with `dhcp: client` (the WAN case)
	leaseLAN   = "host-w7l0"     // tagged interface with a static IPv4
	lease      = "10.7.100.23/24"
	staticV4   = "10.7.1.1/24"
	staticV6   = "2001:db8:7:100::1/64" // static IPv6 on the DHCP sub-interface (allowed next to the v4 client)
)

// leaseRig is the core descriptors plus DF-1's observe-only interface alias (resolves the
// "interface/<name>" dependencies, D-065) on one fake VPP: a tagged af_packet parent with a tagged
// VLAN sub-interface, and a tagged LAN interface.
type leaseRig struct {
	vpp  *coretest.VPP
	s    *scheduler.Scheduler
	addr scheduler.Descriptor
}

func newLeaseAgent(v *coretest.VPP) *leaseRig {
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: v, Owner: leaseOwner, Owned: ownertable.NewMemory(), IfRef: core.AliasInterfaceRef})
	reg.Register(iface.NewAlias(v, leaseOwner))
	s := scheduler.New(reg, nil)
	s.VerifyRetries = 0
	addr, _ := reg.Get(core.InterfaceAddrName)
	return &leaseRig{vpp: v, s: s, addr: addr}
}

func newLeaseRig(t *testing.T) *leaseRig {
	t.Helper()
	ctx := context.Background()
	v := coretest.New()
	parent := v.AddInterface("host-w7c0", "af-packet", leaseOwner+":host-w7c0")
	svc := interfaces.NewServiceClient(v)
	sub, err := svc.CreateSubif(ctx, &interfaces.CreateSubif{SwIfIndex: interface_types.InterfaceIndex(parent), SubID: 100,
		SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, OuterVlanID: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: sub.SwIfIndex, Tag: leaseOwner + ":" + leaseSub}); err != nil {
		t.Fatal(err)
	}
	v.AddInterface(leaseLAN, "af-packet", leaseOwner+":"+leaseLAN)
	return newLeaseAgent(v)
}

func leaseDesired() []scheduler.KV {
	return []scheduler.KV{
		{Key: core.InterfaceAddrKey(leaseLAN, staticV4), Value: &core.InterfaceAddress{Interface: leaseLAN, Prefix: staticV4}},
		{Key: core.InterfaceAddrKey(leaseSub, staticV6), Value: &core.InterfaceAddress{Interface: leaseSub, Prefix: staticV6}},
	}
}

func addrKeys(t *testing.T, d scheduler.Descriptor) []string {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, kv := range kvs {
		out = append(out, string(kv.Key))
	}
	sort.Strings(out)
	return out
}

func wantAddrKeys() []string {
	return []string{"interface-ip/" + leaseSub + "/" + staticV6, "interface-ip/" + leaseLAN + "/" + staticV4} // sorted
}

func hasAddr(v *coretest.VPP, ifName, p string) bool {
	i, ok := v.InterfaceByName(ifName)
	return ok && i.Addrs[p]
}

// addrDeletes lists the sw_interface_add_del_address is_add=0 calls since the last Reset.
func addrDeletes(v *coretest.VPP) []string {
	var out []string
	for _, m := range v.CallsNamed("sw_interface_add_del_address") {
		if r := m.(*interfaces.SwInterfaceAddDelAddress); !r.IsAdd {
			out = append(out, r.Prefix.String())
		}
	}
	return out
}

// applyBound applies the static addresses and then lets the sub-interface's DHCP client bind.
func (r *leaseRig) applyBound(t *testing.T) {
	t.Helper()
	if res := r.s.Apply(context.Background(), leaseDesired(), nil); res.Outcome != scheduler.OutcomeApplied || res.Summary.Created != 2 {
		t.Fatalf("apply %s %+v %v", res.Outcome, res.Summary, res.Err)
	}
	if err := r.vpp.BindLease(leaseSub, lease); err != nil {
		t.Fatal(err)
	}
	if !hasAddr(r.vpp, leaseSub, lease) {
		t.Fatalf("model: lease not installed: %s", r.vpp.Snapshot())
	}
}

func TestInterfaceAddrRetrieveOmitsDHCPLease(t *testing.T) {
	r := newLeaseRig(t)
	r.applyBound(t)
	r.vpp.Reset()
	if got := addrKeys(t, r.addr); !slices.Equal(got, wantAddrKeys()) {
		t.Fatalf("Retrieve = %v, want %v (the DHCP lease %s is VPP's, not ours)", got, wantAddrKeys(), lease)
	}
	// D-132: one dhcp_client_dump per Retrieve, not one per interface or address.
	if n := len(r.vpp.CallsNamed("dhcp_client_dump")); n != 1 {
		t.Fatalf("dhcp_client_dump calls per Retrieve = %d, want 1", n)
	}
}

func TestPlanHasNoDeleteForDHCPLease(t *testing.T) {
	r := newLeaseRig(t)
	r.applyBound(t)
	p, err := r.s.Plan(context.Background(), leaseDesired(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range p.Ops {
		if op.Op == scheduler.OpDelete {
			t.Errorf("plan deletes %s", op.Key)
		}
	}
	if !p.Empty() {
		t.Fatalf("plan with a bound lease is not empty: %+v", p.Ops)
	}
}

// A reconcile (any commit) and resyncs keep the lease, in the same agent and after an agent restart.
func TestReconcileAndResyncKeepDHCPLease(t *testing.T) {
	ctx := context.Background()
	r := newLeaseRig(t)
	r.applyBound(t)
	r.vpp.Reset()
	if res := r.s.Apply(ctx, leaseDesired(), nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("reconcile: %s %v", res.Outcome, res.Err)
	}
	for i := range 2 {
		if res := r.s.ApplyWith(ctx, leaseDesired(), nil, scheduler.ApplyOptions{Resync: true}); res.Outcome != scheduler.OutcomeApplied {
			t.Fatalf("resync %d: %s %v", i, res.Outcome, res.Err)
		}
	}
	// agent restart: a new process (fresh registry and scheduler) resyncs twice against the same VPP
	restarted := newLeaseAgent(r.vpp)
	for i := range 2 {
		if res := restarted.s.ApplyWith(ctx, leaseDesired(), nil, scheduler.ApplyOptions{Resync: true}); res.Outcome != scheduler.OutcomeApplied {
			t.Fatalf("resync %d after restart: %s %v", i, res.Outcome, res.Err)
		}
	}
	if d := addrDeletes(r.vpp); len(d) != 0 {
		t.Fatalf("address deletes during reconcile/resync: %v", d)
	}
	if !hasAddr(r.vpp, leaseSub, lease) {
		t.Fatalf("lease %s gone: %s", lease, r.vpp.Snapshot())
	}
	if c, ok := r.vpp.DHCPClientOf(leaseSub); !ok || c.Lease.String() != lease {
		t.Fatalf("DHCP client %+v %v", c, ok)
	}
}

// The static addresses around the lease are still reconciled: an undesired address of ours on the
// DHCP sub-interface is deleted (the filter is the exact lease, not "every address of a DHCP
// interface"), a lost static is re-created, and a static that leaves the desired state is removed.
func TestStaticAddressesNextToDHCPLeaseStillReconciled(t *testing.T) {
	ctx := context.Background()
	r := newLeaseRig(t)
	r.applyBound(t)
	svc := interfaces.NewServiceClient(r.vpp)
	idx := func(name string) interface_types.InterfaceIndex {
		i, _ := r.vpp.InterfaceByName(name)
		return interface_types.InterfaceIndex(i.Index)
	}
	stray, _ := ip_types.ParseAddressWithPrefix("10.7.100.99/24") // ours (tagged), same subnet as the lease, never desired
	if _, err := svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: idx(leaseSub), IsAdd: true, Prefix: stray}); err != nil {
		t.Fatal(err)
	}
	lost, _ := ip_types.ParseAddressWithPrefix(staticV4)
	if _, err := svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: idx(leaseLAN), IsAdd: false, Prefix: lost}); err != nil {
		t.Fatal(err)
	}
	r.vpp.Reset()
	res := r.s.ApplyWith(ctx, leaseDesired(), nil, scheduler.ApplyOptions{Resync: true})
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Created != 1 || res.Summary.Deleted != 1 {
		t.Fatalf("resync %s %+v %v", res.Outcome, res.Summary, res.Err)
	}
	if d := addrDeletes(r.vpp); !slices.Equal(d, []string{"10.7.100.99/24"}) {
		t.Fatalf("deleted %v, want only the stray 10.7.100.99/24", d)
	}
	if !hasAddr(r.vpp, leaseLAN, staticV4) || hasAddr(r.vpp, leaseSub, "10.7.100.99/24") || !hasAddr(r.vpp, leaseSub, lease) {
		t.Fatalf("after resync: %s", r.vpp.Snapshot())
	}
	// the IPv6 static on the DHCP sub-interface leaves the desired state: removed, the lease stays
	r.vpp.Reset()
	res = r.s.Apply(ctx, leaseDesired()[:1], nil)
	if res.Outcome != scheduler.OutcomeApplied || res.Summary.Deleted != 1 {
		t.Fatalf("apply %s %+v %v", res.Outcome, res.Summary, res.Err)
	}
	if d := addrDeletes(r.vpp); !slices.Equal(d, []string{staticV6}) || hasAddr(r.vpp, leaseSub, staticV6) || !hasAddr(r.vpp, leaseSub, lease) {
		t.Fatalf("deleted %v: %s", d, r.vpp.Snapshot())
	}
}

// A renewal to another address: VPP swaps the installed lease; the new one is left out as well. An
// unbound client (DISCOVER, host address 0.0.0.0) has installed nothing, so nothing is left out.
func TestDHCPLeaseChangesAndUnboundClient(t *testing.T) {
	r := newLeaseRig(t)
	r.applyBound(t)
	if err := r.vpp.BindLease(leaseSub, "10.7.100.42/24"); err != nil {
		t.Fatal(err)
	}
	if got := addrKeys(t, r.addr); !slices.Equal(got, wantAddrKeys()) {
		t.Fatalf("after renewal Retrieve = %v", got)
	}
	// unbound client on the LAN interface: its static IPv4 is still ours
	cfg := &dhcp.DHCPClientConfig{IsAdd: true, Client: dhcp.DHCPClient{SwIfIndex: func() interface_types.InterfaceIndex {
		i, _ := r.vpp.InterfaceByName(leaseLAN)
		return interface_types.InterfaceIndex(i.Index)
	}(), Hostname: "w7"}}
	if _, err := dhcp.NewServiceClient(r.vpp).DHCPClientConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if got := addrKeys(t, r.addr); !slices.Equal(got, wantAddrKeys()) {
		t.Fatalf("with an unbound client Retrieve = %v", got)
	}
}

// Without the lease list an address cannot be classified: a failing dhcp_client_dump fails the
// Retrieve (and the transaction), so the lease is never deleted by mistake. A VPP without the dhcp
// plugin (unknown message) has no client and so no lease. No address of ours: no dump at all.
func TestDHCPLeaseDumpFailureAndMissingPlugin(t *testing.T) {
	ctx := context.Background()
	r := newLeaseRig(t)
	r.applyBound(t)
	r.vpp.On("dhcp_client_dump", func(api.Message) ([]api.Message, error) { return nil, errors.New("api socket reset") })
	if _, err := r.addr.Retrieve(ctx); err == nil {
		t.Fatal("Retrieve succeeded without the lease list")
	}
	r.vpp.Reset()
	if res := r.s.Apply(ctx, leaseDesired(), nil); res.Outcome == scheduler.OutcomeApplied {
		t.Fatalf("apply without the lease list: %s", res.Outcome)
	}
	if d := addrDeletes(r.vpp); len(d) != 0 || !hasAddr(r.vpp, leaseSub, lease) {
		t.Fatalf("deleted %v: %s", d, r.vpp.Snapshot())
	}

	r.vpp.On("dhcp_client_dump", func(api.Message) ([]api.Message, error) {
		return nil, &adapter.UnknownMsgError{MsgName: "dhcp_client_dump", MsgCrc: "51077d14"}
	})
	if got := addrKeys(t, r.addr); len(got) != 3 {
		t.Fatalf("no dhcp plugin: Retrieve = %v, want all three addresses", got)
	}

	empty := newLeaseAgent(coretest.New())
	empty.vpp.Reset()
	if got := addrKeys(t, empty.addr); len(got) != 0 || len(empty.vpp.CallsNamed("dhcp_client_dump")) != 0 {
		t.Fatalf("no address of ours: %v, %d dumps", got, len(empty.vpp.CallsNamed("dhcp_client_dump")))
	}
}
