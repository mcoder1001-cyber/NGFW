package core_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// TD-11c (review 3.1b, README M5): interface-ip and interface-ip.table accept an UNTAGGED interface
// (a DPDK NIC named by F-startup-gen, any interface no descriptor created) through the owner's
// interface claim store (D-071 claim path; the product store binds each claim to the boot identity
// and the sw_if_index, D-080). Without a store an untagged interface is refused as before; another
// owner's interface is refused always.

// claims is an in-memory core.ClaimStore that records calls (the product passes the persisted
// subsystems.IfaceClaims; its identity/index binding is tested in package subsystems).
type claims struct {
	m        map[[2]string]bool
	failNext error
}

func newClaims() *claims { return &claims{m: map[[2]string]bool{}} }

func (c *claims) Claim(ifName, holder string) error {
	if err := c.failNext; err != nil {
		c.failNext = nil
		return err
	}
	c.m[[2]string{ifName, holder}] = true
	return nil
}

func (c *claims) Release(ifName, holder string) error {
	delete(c.m, [2]string{ifName, holder})
	return nil
}

func (c *claims) Claimed(ifName, holder string) bool { return c.m[[2]string{ifName, holder}] }

func descs(v *coretest.VPP, owner string, cs core.ClaimStore) (tbl, addr scheduler.Descriptor) {
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: v, Owner: owner, Owned: ownertable.NewMemory(), Claims: cs})
	tbl, _ = reg.Get(core.InterfaceTableName)
	addr, _ = reg.Get(core.InterfaceAddrName)
	return tbl, addr
}

func retrieveKeys(t *testing.T, d scheduler.Descriptor) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		out[kv.Key] = kv.Value
	}
	return out
}

func TestUntaggedInterfaceNeedsClaimStore(t *testing.T) {
	v := coretest.New()
	v.AddInterface("lan", "dpdk", "")
	_, addr := descs(v, "w10", nil)
	_, err := addr.Create(context.Background(), &core.InterfaceAddress{Interface: "lan", Prefix: "10.10.1.1/24"})
	if !errors.Is(err, core.ErrNotOwned) {
		t.Fatalf("untagged NIC without a claim store: %v, want ErrNotOwned", err)
	}
}

func TestUntaggedInterfaceClaimPath(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	lan := v.AddInterface("lan", "dpdk", "")
	v.AddInterface("wan", "dpdk", "")
	v.AddInterface("x0", "dpdk", "w3:x0") // another owner's
	// an address someone else put on wan (Linux via linux-nl, a DHCP lease, vppctl): never ours
	cs := newClaims()
	tbl, addr := descs(v, "w10", cs)
	if _, err := descs2vrf(t, v); err != nil {
		t.Fatal(err)
	}
	foreignTbl, foreignAddr := descs(v, "w9", newClaims())
	if _, err := foreignAddr.Create(ctx, &core.InterfaceAddress{Interface: "wan", Prefix: "192.0.2.1/24"}); err != nil {
		t.Fatal(err)
	}
	_ = foreignTbl

	// Create: claim first, then VPP; Meta is the NIC's sw_if_index.
	m, err := tbl.Create(ctx, &core.InterfaceTable{Interface: "lan", TableId: 10001})
	if err != nil || m != (core.IfMeta{SwIfIndex: lan}) {
		t.Fatalf("table binding on untagged lan: %v %v", m, err)
	}
	for _, p := range []string{"10.10.1.1/24", "2001:db8:a::1/64"} {
		if _, err := addr.Create(ctx, &core.InterfaceAddress{Interface: "lan", Prefix: p}); err != nil {
			t.Fatalf("address %s on untagged lan: %v", p, err)
		}
	}
	if i, _ := v.InterfaceByName("lan"); i.Table4 != 10001 || i.Table6 != 10001 || !i.Addrs["10.10.1.1/24"] || !i.Addrs["2001:db8:a::1/64"] {
		t.Fatalf("VPP lan %+v", i)
	}
	if !cs.Claimed("lan", core.InterfaceTableName) || !cs.Claimed("lan", core.AddrHolder("10.10.1.1/24")) || !cs.Claimed("lan", core.AddrHolder("2001:db8:a::1/64")) {
		t.Fatalf("claims %v", cs.m)
	}

	// Retrieve reports exactly the claimed objects: not wan's foreign address, not another owner's.
	gotT := retrieveKeys(t, tbl)
	if len(gotT) != 1 || !proto.Equal(gotT["interface-ip.table/lan"], &core.InterfaceTable{Interface: "lan", TableId: 10001}) {
		t.Fatalf("table Retrieve %v", gotT)
	}
	gotA := retrieveKeys(t, addr)
	if len(gotA) != 2 || gotA["interface-ip/lan/10.10.1.1/24"] == nil || gotA["interface-ip/lan/2001:db8:a::1/64"] == nil {
		t.Fatalf("address Retrieve %v", gotA)
	}

	// Another owner's interface is never taken, claim store or not.
	if _, err := addr.Create(ctx, &core.InterfaceAddress{Interface: "x0", Prefix: "10.10.9.1/24"}); !errors.Is(err, core.ErrNotOwned) {
		t.Fatalf("foreign-tagged x0: %v", err)
	}
	if cs.Claimed("x0", core.AddrHolder("10.10.9.1/24")) {
		t.Fatal("claim recorded for a refused interface")
	}
	// Delete of an address we never claimed (wan's foreign one): VPP untouched.
	if err := addr.Delete(ctx, &core.InterfaceAddress{Interface: "wan", Prefix: "192.0.2.1/24"}, nil); err != nil {
		t.Fatal(err)
	}
	if i, _ := v.InterfaceByName("wan"); !i.Addrs["192.0.2.1/24"] {
		t.Fatal("unclaimed foreign address deleted")
	}

	// A claim-store failure refuses the Create before VPP is touched.
	cs.failNext = errors.New("disk full")
	if _, err := addr.Create(ctx, &core.InterfaceAddress{Interface: "lan", Prefix: "10.10.2.1/24"}); err == nil {
		t.Fatal("Create without a recorded claim succeeded")
	}
	if i, _ := v.InterfaceByName("lan"); i.Addrs["10.10.2.1/24"] {
		t.Fatal("VPP written although the claim failed")
	}
	// A VPP failure after the claim releases it again (no stale claim on nothing).
	if _, err := tbl.Create(ctx, &core.InterfaceTable{Interface: "wan", TableId: 10999}); err == nil {
		t.Fatal("binding to a missing table succeeded")
	}
	if cs.Claimed("wan", core.InterfaceTableName) {
		t.Fatal("claim kept after the VPP call failed")
	}
	if i, _ := v.InterfaceByName("wan"); i.Table4 != 0 || i.Table6 != 0 {
		t.Fatalf("wan binding after failed Create %+v", i)
	}

	// Delete: VPP first, then the claim is released; the NIC stays.
	for _, p := range []string{"10.10.1.1/24", "2001:db8:a::1/64"} {
		if err := addr.Delete(ctx, &core.InterfaceAddress{Interface: "lan", Prefix: p}, core.IfMeta{SwIfIndex: lan}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tbl.Delete(ctx, &core.InterfaceTable{Interface: "lan", TableId: 10001}, core.IfMeta{SwIfIndex: lan}); err != nil {
		t.Fatal(err)
	}
	if i, ok := v.InterfaceByName("lan"); !ok || i.Table4 != 0 || i.Table6 != 0 || len(i.Addrs) != 0 {
		t.Fatalf("lan after delete %+v", i)
	}
	if len(cs.m) != 0 {
		t.Fatalf("claims left %v", cs.m)
	}
	if len(retrieveKeys(t, tbl))+len(retrieveKeys(t, addr)) != 0 {
		t.Fatal("Retrieve still reports deleted objects")
	}
}

// descs2vrf creates table 10001 (both families) for the binding tests.
func descs2vrf(t *testing.T, v *coretest.VPP) (any, error) {
	t.Helper()
	reg := scheduler.NewRegistry()
	core.Register(reg, core.Env{Client: v, Owner: "w10", Owned: ownertable.NewMemory()})
	d, _ := reg.Get(core.VRFName)
	return d.Create(context.Background(), &core.Table{Id: 10001, Vrf: "blue"})
}

// A binding VPP accepts for IPv4 and refuses for IPv6 (the NIC carries somebody else's IPv6 address:
// ADDRESS_FOUND_FOR_INTERFACE) leaves nothing behind: IPv4 gets its previous table back and the
// claim is released.
func TestUntaggedPartialBindRestored(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	v.AddInterface("dmz", "dpdk", "")
	if _, err := descs2vrf(t, v); err != nil {
		t.Fatal(err)
	}
	_, other := descs(v, "w9", newClaims())
	if _, err := other.Create(ctx, &core.InterfaceAddress{Interface: "dmz", Prefix: "2001:db8:f::1/64"}); err != nil {
		t.Fatal(err)
	}
	cs := newClaims()
	tbl, _ := descs(v, "w10", cs)
	if _, err := tbl.Create(ctx, &core.InterfaceTable{Interface: "dmz", TableId: 10001}); err == nil {
		t.Fatal("binding over a foreign IPv6 address succeeded")
	}
	if i, _ := v.InterfaceByName("dmz"); i.Table4 != 0 || i.Table6 != 0 {
		t.Fatalf("dmz after the refused binding %+v", i)
	}
	if cs.Claimed("dmz", core.InterfaceTableName) {
		t.Fatal("claim kept although nothing is bound")
	}
}
