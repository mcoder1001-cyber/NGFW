package det44_test

import (
	"context"
	"testing"

	"ngfw/agent/binapi/det44"
	det44d "ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestDet44OnHost: one integration check per det44 object type. The plugin is enabled and
// deliberately never disabled (VPP 26.06 crash, see det44.go Enable.Delete).
func TestDet44OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := det44d.New(c, vpptest.Prefix(t))
	svc := det44.NewServiceClient(c)

	// det44 is enabled and never disabled here: det44_plugin_enable_disable(disable)
	// crashes VPP 26.06 once any det44 interface was removed (det44.c, see det44.go).
	// The plugin stays enabled and idle until the next VPP restart; that is harmless for
	// other slots (det44 does nothing without interfaces and maps).
	en := natcommon.MustEncode(&det44d.EnableSpec{})
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatal(err)
	}
	nattest.AssertPlan(t, p.Enable, en)
	t.Cleanup(func() { _ = p.Enable.Delete(context.Background(), en, nil) }) // agent-side release only

	// timeouts are a global singleton: only touch them when they are at VPP's defaults, and
	// Delete (run by CreateAll's cleanup) restores exactly those defaults.
	cur, err := svc.Det44GetTimeouts(ctx, &det44.Det44GetTimeouts{})
	if err != nil {
		t.Fatal(err)
	}
	if (det44d.TimeoutsSpec{UDP: cur.UDP, TCPEstablished: cur.TCPEstablished, TCPTransitory: cur.TCPTransitory, ICMP: cur.ICMP}) != det44d.DefaultTimeouts {
		t.Skipf("det44 timeouts already changed by another owner (%+v): not touching the global", cur)
	}
	tmo := natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: 299, TCPEstablished: 7439, TCPTransitory: 239, ICMP: 59})
	nattest.CreateAll(ctx, t, p.Timeouts, tmo)
	nattest.AssertPlan(t, p.Timeouts, tmo)

	inside, _ := nattest.Loopback(t, c, 20)
	outside, _ := nattest.Loopback(t, c, 21)
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: inside, Side: det44d.SideInside})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: outside, Side: det44d.SideOutside})
	nattest.CreateAll(ctx, t, p.Interface, in, out)
	nattest.AssertPlan(t, p.Interface, in, out)

	// deterministic map: 10.<N>.44.0/24 inside → 10.<N>.45.0/30 outside (slot block)
	m := natcommon.MustEncode(&det44d.MapSpec{Inside: nattest.Addr4(t, 44, 0) + "/24", Outside: nattest.Addr4(t, 45, 0) + "/30"})
	nattest.CreateAll(ctx, t, p.Map, m)
	nattest.AssertPlan(t, p.Map, m)

	if sess, err := p.Sessions(ctx, nattest.Addr4(t, 44, 1), 0, 10); err != nil {
		t.Fatalf("session dump: %v", err)
	} else {
		t.Logf("det44 sessions of %s: %d (shape only)", nattest.Addr4(t, 44, 1), len(sess))
	}

	nattest.Pause(t, "det44") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.Map)
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Timeouts)
}
