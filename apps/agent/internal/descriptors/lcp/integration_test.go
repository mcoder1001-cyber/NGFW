package lcp

import (
	"context"
	"errors"
	"os"
	"testing"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// Integration test against the host VPP (linux_cp loaded since D-060; skip-unless-plugin-loaded
// otherwise). The pair is made on this slot's tagged loopback with a Linux tap named
// "<prefix>-lcp0" in the host's root namespace; never an ens* NIC or local0. The default netns is
// a VPP-global: read first, skipped when set by someone else, unset again in Cleanup.
func TestLCPOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, Plugin, &lcp.LcpItfPairAddDelV3{}, &lcp.LcpItfPairGet{}, &lcp.LcpDefaultNsSet{}, &lcp.LcpDefaultNsGet{})
	c := h.Client()
	ctx := context.Background()
	nd := NewDefaultNetns(c, WithGlobals(dfkit.GlobalsOwner(true))) // test acts as globals owner, restores "unset"
	pd := NewItfPair(c, h.Owner)

	t.Run("itf-pair", func(t *testing.T) {
		if ns, err := nd.Current(ctx); err != nil {
			t.Fatal(err)
		} else if ns != "" {
			t.Skipf("default netns is %q (set by someone else): a pair with netns \"\" would land there", ns)
		}
		ifName, _ := h.Loopback(t, 86)
		v := ItfPair{Interface: ifName, HostIfName: h.Owner + "-lcp0", HostIfType: "tap"}.Proto()
		t.Cleanup(func() { _ = pd.Delete(context.Background(), v, nil) })
		meta, err := pd.Create(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("pair %s ↔ %s-lcp0: %+v", ifName, h.Owner, meta)
		got := dfkittest.AssertRetrieved(t, pd, dfkittest.KV(pd, v))
		if got.Meta != meta {
			t.Fatalf("retrieve meta %+v, create meta %+v", got.Meta, meta)
		}
		dfkittest.AssertEmptyPlan(t, pd, dfkittest.KV(pd, v))
		if again, err := pd.Create(ctx, v); err != nil || again != meta { // re-apply: VALUE_EXIST + identical
			t.Fatalf("re-apply: %v %v", again, err)
		}
		other := ItfPair{Interface: ifName, HostIfName: h.Owner + "-lcp1", HostIfType: "tap"}.Proto()
		if _, err := pd.Create(ctx, other); err == nil {
			t.Fatal("a second, different pair on the same interface must fail")
		}
		dfkittest.HoldForEvidence(t, "CLI: show lcp; ip link show "+h.Owner+"-lcp0")
		for range 2 {
			if err := pd.Delete(ctx, v, meta); err != nil {
				t.Fatal(err)
			}
		}
		dfkittest.AssertAbsent(t, pd, pd.KeyOf(v))
		if tbl, err := dfkit.DumpInterfaces(ctx, c, h.Owner); err != nil {
			t.Fatal(err)
		} else if _, ok := tbl.ByIndex[meta.(PairMeta).HostSwIfIndex]; ok {
			t.Fatalf("host tap sw_if_index %d still exists after pair delete", meta.(PairMeta).HostSwIfIndex)
		}
	})

	t.Run("foreign pair on an untagged interface (H1)", func(t *testing.T) {
		// an untagged loopback of this slot stands in for a physical NIC; a pair made on it
		// through the raw API stands in for a pair of the operator / another owner
		ifName, idx := h.UntaggedLoopback(t, 97)
		svc := lcp.NewServiceClient(c)
		if _, err := svc.LcpItfPairAddDelV3(ctx, &lcp.LcpItfPairAddDelV3{IsAdd: true, SwIfIndex: interface_types.InterfaceIndex(idx),
			HostIfName: h.Owner + "-for0", HostIfType: lcp.LCP_API_ITF_HOST_TAP}); err != nil {
			t.Fatalf("raw foreign pair: %v", err)
		}
		t.Cleanup(func() {
			_, _ = svc.LcpItfPairAddDelV3(context.Background(), &lcp.LcpItfPairAddDelV3{SwIfIndex: interface_types.InterfaceIndex(idx)})
		})
		for _, v := range []ItfPair{
			{Interface: ifName, HostIfName: h.Owner + "-lcp9", HostIfType: "tap"},
			{Interface: ifName, HostIfName: h.Owner + "-for0", HostIfType: "tap"},
		} {
			if _, err := pd.Create(ctx, v.Proto()); !errors.Is(err, dfkit.ErrNotOurs) {
				t.Fatalf("%+v: foreign pair adopted: %v", v, err)
			}
			t.Logf("refused as expected: %+v", v)
		}
		v := ItfPair{Interface: ifName, HostIfName: h.Owner + "-for0", HostIfType: "tap"}.Proto()
		dfkittest.AssertAbsent(t, pd, pd.KeyOf(v))
		if err := pd.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
		pairs, err := Pairs(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range pairs {
			found = found || uint32(p.PhySwIfIndex) == idx
		}
		if !found {
			t.Fatal("the foreign pair was deleted")
		}
		t.Logf("foreign pair on %s kept: not adopted, not reported, not deleted", ifName)
	})

	t.Run("default-netns", func(t *testing.T) {
		// a nonexistent default netns would break other slots' pairs with netns "" meanwhile
		dfkittest.SkipUnlessGlobals(t, "lcp_default_ns_set")
		cur, err := nd.Current(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if cur != "" {
			t.Skipf("default netns is %q (set by someone else): not touching a global", cur)
		}
		v := DefaultNetns{Netns: "ns-" + h.Owner + "-lcp"}.Proto()
		t.Cleanup(func() { _ = nd.Delete(context.Background(), v, nil) })
		if _, err := nd.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertRetrieved(t, nd, dfkittest.KV(nd, v))
		dfkittest.AssertEmptyPlan(t, nd, dfkittest.KV(nd, v))
		if err := nd.Delete(ctx, v, nil); err != nil {
			t.Fatal(err)
		}
		dfkittest.AssertAbsent(t, nd, KeyDefaultNetns)
	})

	t.Run("replace helpers", func(t *testing.T) {
		if os.Getenv("VRX_DF8_LCP_REPLACE") != "1" {
			t.Skip("replace_begin/end can delete pairs other slots create meanwhile; VRX_DF8_LCP_REPLACE=1 in a manager window (unit-tested with the fake)")
		}
		// begin marks every existing pair stale and end deletes the stale ones — of every owner.
		// Run the round trip only while no pair exists at all (pairs made after begin are fresh).
		pairs, err := Pairs(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(pairs) != 0 {
			t.Skipf("%d pair(s) exist: a replace transaction would delete other owners' pairs", len(pairs))
		}
		if err := ReplaceBegin(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := ReplaceEnd(ctx, c); err != nil {
			t.Fatal(err)
		}
	})
}
