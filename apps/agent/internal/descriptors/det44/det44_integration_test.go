package det44_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"ngfw/agent/binapi/det44"
	det44d "ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestDet44OnHost: one integration check per det44 object type (opt-in, D-064). The slot is not
// the globals owner (D-071); the plugin is a never-disabled fixture (D-068).
func TestDet44OnHost(t *testing.T) {
	// D-064: this test's first host run crashed the shared VPP (det44_plugin_enable_disable
	// with enable=0, VPP 26.06 bug — fixed here by never disabling det44). It stays opt-in.
	if os.Getenv("VRX_DF3_DET44") != "1" {
		t.Skip("det44 host test is opt-in (D-064): set VRX_DF3_DET44=1")
	}
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := det44d.New(c, vpptest.Prefix(t))
	svc := det44.NewServiceClient(c)

	// det44 is a test fixture (D-071), enabled if off and NEVER disabled (D-068: the
	// disable crashes VPP 26.06). It stays enabled and idle until the next VPP restart.
	nattest.EnsurePlugin(t, nattest.Plugin{
		Name: "det44",
		Enable: func(ctx context.Context) (bool, error) {
			_, err := svc.Det44PluginEnableDisable(ctx, &det44.Det44PluginEnableDisable{Enable: true})
			if natcommon.IsAlreadyEnabled(err) {
				return true, nil
			}
			return false, err
		},
	})
	en := natcommon.MustEncode(&det44d.EnableSpec{})
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatal(err)
	}
	nattest.AssertWriteOnly(t, p.Enable)

	// timeouts: global, required at their current value only (never set by a slot)
	cur, err := svc.Det44GetTimeouts(ctx, &det44.Det44GetTimeouts{})
	if err != nil {
		t.Fatal(err)
	}
	curTmo := natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: cur.UDP, TCPEstablished: cur.TCPEstablished, TCPTransitory: cur.TCPTransitory, ICMP: cur.ICMP})
	if _, err := p.Timeouts.Create(ctx, curTmo); err != nil {
		t.Fatalf("timeouts requirement: %v", err)
	}
	if _, err := p.Timeouts.Create(ctx, natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: cur.UDP + 1})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("timeouts requirement (other) must fail: %v", err)
	}

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
}
