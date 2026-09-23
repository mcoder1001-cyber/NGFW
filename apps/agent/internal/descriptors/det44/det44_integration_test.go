package det44_test

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/det44"
	det44d "ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestDet44OnHost: one integration check per det44 object type; the enable singleton is
// probed and disabled only when this test enabled it.
func TestDet44OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := det44d.New(c, vpptest.Prefix(t))
	svc := det44.NewServiceClient(c)

	en := natcommon.MustEncode(&det44d.EnableSpec{})
	_, err := svc.Det44PluginEnableDisable(ctx, &det44.Det44PluginEnableDisable{Enable: true})
	alreadyOn := natcommon.IsAlreadyEnabled(err)
	if err != nil && !alreadyOn {
		t.Fatalf("det44 enable: %v", err)
	}
	if alreadyOn {
		t.Log("det44 already enabled by another owner: will not disable")
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Enable.Delete(cctx, en, nil); err != nil {
				t.Errorf("restore: disable det44: %v", err)
			}
		})
	}
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatal(err)
	}
	nattest.AssertPlan(t, p.Enable, en)

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

	nattest.DeleteAll(ctx, t, p.Map)
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Timeouts)
}
