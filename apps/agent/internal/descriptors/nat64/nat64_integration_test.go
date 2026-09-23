package nat64_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/binapi/nat64"
	nat64d "ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat64OnHost: one integration check per nat64 object type. The slot is not the globals
// owner (D-071): nat64 is a test fixture (enabled if off; disabled again only if this test
// enabled it and nat64 is empty), timeouts are required at their current value, never set.
func TestNat64OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := nat64d.New(c, vpptest.Prefix(t))
	svc := nat64.NewServiceClient(c)
	nattest.EnsurePlugin(t, nattest.Plugin{
		Name: "nat64",
		Enable: func(ctx context.Context) (bool, error) {
			_, err := svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: true})
			if natcommon.IsAlreadyEnabled(err) {
				return true, nil
			}
			return false, err
		},
		Empty: p.Empty,
		Disable: func(ctx context.Context) error {
			_, err := svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: false})
			return err
		},
	})
	en := natcommon.MustEncode(&nat64d.EnableSpec{})
	if _, err := p.Enable.Create(ctx, en); err != nil { // requirement: unobservable or satisfied
		t.Fatal(err)
	}
	nattest.AssertWriteOnly(t, p.Enable)
	before, err := svc.Nat64GetTimeouts(ctx, &nat64.Nat64GetTimeouts{})
	if err != nil {
		t.Fatal(err)
	}
	cur := natcommon.MustEncode(&nat64d.TimeoutsSpec{UDP: before.UDP, TCPEstablished: before.TCPEstablished, TCPTransitory: before.TCPTransitory, ICMP: before.ICMP})
	if _, err := p.Timeouts.Create(ctx, cur); err != nil {
		t.Fatalf("timeouts requirement: %v", err)
	}
	if _, err := p.Timeouts.Create(ctx, natcommon.MustEncode(&nat64d.TimeoutsSpec{UDP: before.UDP + 1})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("timeouts requirement (other) must fail: %v", err)
	}
	if after, err := svc.Nat64GetTimeouts(ctx, &nat64.Nat64GetTimeouts{}); err != nil || *after != *before {
		t.Fatalf("a non-owner changed nat64 timeouts: %+v → %+v %v", before, after, err)
	}

	inside, _ := nattest.Loopback(t, c, 10)
	outside, _ := nattest.Loopback(t, c, 11)
	vrf := nattest.Table(t, c, 10, false)
	nattest.Table(t, c, 10, true) // nat64 prefix per vrf needs the ip6 table too

	pfx := natcommon.MustEncode(&nat64d.PrefixSpec{Prefix: nattest.Prefix6(t, 64, 96), VRF: vrf})
	nattest.CreateAll(ctx, t, p.Prefix, pfx)
	nattest.AssertPlan(t, p.Prefix, pfx)

	pool := natcommon.MustEncode(&nat64d.PoolSpec{First: nattest.Addr4(t, 64, 1), Last: nattest.Addr4(t, 64, 2)})
	nattest.CreateAll(ctx, t, p.Pool, pool)
	nattest.AssertPlan(t, p.Pool, pool)

	in := natcommon.MustEncode(&nat64d.InterfaceSpec{Interface: inside, Side: nat64d.SideInside})
	out := natcommon.MustEncode(&nat64d.InterfaceSpec{Interface: outside, Side: nat64d.SideOutside})
	nattest.CreateAll(ctx, t, p.Interface, in, out)
	nattest.AssertPlan(t, p.Interface, in, out)

	bib := natcommon.MustEncode(&nat64d.StaticBIBSpec{InsideIP: nattest.Addr6(t, 0x64), InsidePort: 80, OutsideIP: nattest.Addr4(t, 64, 1), OutsidePort: 8080, Protocol: "tcp"})
	nattest.CreateAll(ctx, t, p.StaticBIB, bib)
	nattest.AssertPlan(t, p.StaticBIB, bib)

	if sess, err := p.Sessions(ctx, "any", 0, 10); err != nil {
		t.Fatalf("st dump: %v", err)
	} else {
		t.Logf("nat64 sessions: %d (shape only)", len(sess))
	}

	nattest.Pause(t, "nat64") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.StaticBIB)
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Pool)
	nattest.DeleteAll(ctx, t, p.Prefix)
}
