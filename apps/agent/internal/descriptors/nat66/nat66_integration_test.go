package nat66_test

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/nat66"
	nat66d "ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat66OnHost: one integration check per nat66 object type; enable is probed (retval on
// an already enabled plugin) and disabled only when this test enabled it.
func TestNat66OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := nat66d.New(c, vpptest.Prefix(t))
	svc := nat66.NewServiceClient(c)

	en := natcommon.MustEncode(&nat66d.EnableSpec{})
	_, err := svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: true})
	alreadyOn := natcommon.IsAlreadyEnabled(err)
	if err != nil && !alreadyOn {
		t.Fatalf("nat66 enable: %v", err)
	}
	if alreadyOn {
		t.Log("nat66 already enabled by another owner: will not disable")
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Enable.Delete(cctx, en, nil); err != nil {
				t.Errorf("restore: disable nat66: %v", err)
			}
		})
	}
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatal(err)
	}
	nattest.AssertPlan(t, p.Enable, en)

	inside, _ := nattest.Loopback(t, c, 12)
	outside, _ := nattest.Loopback(t, c, 13)
	in := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: inside, Side: nat66d.SideInside})
	out := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: outside, Side: nat66d.SideOutside})
	nattest.CreateAll(ctx, t, p.Interface, in, out)
	nattest.AssertPlan(t, p.Interface, in, out)

	m := natcommon.MustEncode(&nat66d.StaticMappingSpec{Local: nattest.Addr6(t, 0x66), External: nattest.Addr6(t, 0x6600)})
	nattest.CreateAll(ctx, t, p.StaticMapping, m)
	nattest.AssertPlan(t, p.StaticMapping, m)

	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.Interface)
}
