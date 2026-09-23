package nat64_test

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/nat64"
	nat64d "ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat64OnHost: one integration check per nat64 object type. VPP has no "enabled" getter:
// the test probes with an enable and remembers whether it was already on (retval 1), so it
// disables only what it enabled.
func TestNat64OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := nat64d.New(c, vpptest.Prefix(t))
	svc := nat64.NewServiceClient(c)

	en := natcommon.MustEncode(&nat64d.EnableSpec{})
	_, err := svc.Nat64PluginEnableDisable(ctx, &nat64.Nat64PluginEnableDisable{Enable: true})
	alreadyOn := natcommon.IsAlreadyEnabled(err)
	if err != nil && !alreadyOn {
		t.Fatalf("nat64 enable: %v", err)
	}
	if alreadyOn {
		t.Log("nat64 already enabled by another owner: will not disable")
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Enable.Delete(cctx, en, nil); err != nil {
				t.Errorf("restore: disable nat64: %v", err)
			}
		})
	}
	if _, err := p.Enable.Create(ctx, en); err != nil { // idempotent on an enabled plugin
		t.Fatal(err)
	}
	nattest.AssertPlan(t, p.Enable, en)

	tmo := natcommon.MustEncode(&nat64d.TimeoutsSpec{UDP: 299, TCPEstablished: 7439, TCPTransitory: 239, ICMP: 59})
	nattest.CreateAll(ctx, t, p.Timeouts, tmo)
	nattest.AssertPlan(t, p.Timeouts, tmo)

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

	nattest.DeleteAll(ctx, t, p.StaticBIB)
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Pool)
	nattest.DeleteAll(ctx, t, p.Prefix)
	nattest.DeleteAll(ctx, t, p.Timeouts)
}
