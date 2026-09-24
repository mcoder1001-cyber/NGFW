package nat66_test

import (
	"context"
	"testing"

	"ngfw/agent/binapi/nat66"
	nat66d "ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat66OnHost: one integration check per nat66 object type. The slot is not the globals
// owner (D-071): nat66 is a test fixture (enabled if off; disabled again only if this test
// enabled it and nat66 is empty); the enable descriptor is exercised as a requirement.
func TestNat66OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := nat66d.New(c, vpptest.Prefix(t))
	svc := nat66.NewServiceClient(c)
	nattest.EnsurePlugin(t, nattest.Plugin{
		Name: "nat66",
		Enable: func(ctx context.Context) (bool, error) {
			_, err := svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: true})
			if natcommon.IsAlreadyEnabled(err) {
				return true, nil
			}
			return false, err
		},
		Empty: p.Empty,
		Disable: func(ctx context.Context) error {
			_, err := svc.Nat66PluginEnableDisable(ctx, &nat66.Nat66PluginEnableDisable{Enable: false})
			return err
		},
	})
	en := natcommon.MustEncode(&nat66d.EnableSpec{})
	if _, err := p.Enable.Create(ctx, en); err != nil {
		t.Fatal(err)
	}
	nattest.AssertWriteOnly(t, p.Enable)

	inside, _ := nattest.Loopback(t, c, 12)
	outside, _ := nattest.Loopback(t, c, 13)
	in := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: inside, Side: nat66d.SideInside})
	out := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: outside, Side: nat66d.SideOutside})
	nattest.CreateAll(ctx, t, p.Interface, in, out)
	nattest.AssertPlan(t, p.Interface, in, out)

	m := natcommon.MustEncode(&nat66d.StaticMappingSpec{Local: nattest.Addr6(t, 0x66), External: nattest.Addr6(t, 0x6600)})
	nattest.CreateAll(ctx, t, p.StaticMapping, m)
	nattest.AssertPlan(t, p.StaticMapping, m)

	nattest.Pause(t, "nat66") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.Interface)
}
