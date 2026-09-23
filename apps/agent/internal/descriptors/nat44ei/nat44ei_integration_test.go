package nat44ei_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat44EiOnHost: one integration check per nat44-ei object type. The slot is not the
// globals owner (D-071): the plugin is a test fixture (enabled if off — skipped while nat44-ed
// is on, the two are mutually exclusive — and disabled again only if this test enabled it and
// nat44-ei is empty); enable / timeouts / forwarding / ipfix are required, never set.
func TestNat44EiOnHost(t *testing.T) {
	c := nattest.Connect(t)
	nattest.SlotLock(t, "nat44")
	ctx := nattest.Ctx(t)
	p := nat44ei.New(c, vpptest.Prefix(t))

	ed, err := nat44_ed.NewServiceClient(c).Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if ed.Sessions != 0 {
		t.Skip("nat44-ed is enabled on this VPP; ED and EI are mutually exclusive")
	}
	svc := nat44_ei.NewServiceClient(c)
	nattest.EnsurePlugin(t, nattest.Plugin{
		Name: "nat44-ei",
		Enable: func(ctx context.Context) (bool, error) {
			_, err := svc.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: true})
			if natcommon.IsAlreadyEnabled(err) {
				return true, nil
			}
			return false, err
		},
		Empty: p.Empty,
		Disable: func(ctx context.Context) error {
			_, err := svc.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: false})
			return err
		},
	})
	rc, err := svc.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cur := natcommon.MustEncode(&nat44ei.EnableSpec{InsideVRF: rc.InsideVrf, OutsideVRF: rc.OutsideVrf,
		StaticMappingOnly: rc.Flags&nat44_ei.NAT44_EI_STATIC_MAPPING_ONLY != 0, ConnectionTracking: rc.Flags&nat44_ei.NAT44_EI_CONNECTION_TRACKING != 0, Out2InDPO: rc.Flags&nat44_ei.NAT44_EI_OUT2IN_DPO != 0})
	if _, err := p.Enable.Create(ctx, cur); err != nil {
		t.Fatalf("enable requirement: %v", err)
	}
	curTmo := natcommon.MustEncode(&nat44ei.TimeoutsSpec{UDP: rc.Timeouts.UDP, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP})
	if _, err := p.Timeouts.Create(ctx, curTmo); err != nil {
		t.Fatalf("timeouts requirement: %v", err)
	}
	if _, err := p.Timeouts.Create(ctx, natcommon.MustEncode(&nat44ei.TimeoutsSpec{UDP: rc.Timeouts.UDP + 1})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("timeouts requirement (other) must fail: %v", err)
	}
	if _, err := p.Forwarding.Create(ctx, natcommon.MustEncode(&nat44ei.ForwardingSpec{})); rc.ForwardingEnabled != (err == nil) {
		t.Fatalf("forwarding requirement (on=%v): %v", rc.ForwardingEnabled, err)
	}
	nattest.AssertWriteOnly(t, p.Enable)
	defer func() {
		rc2, err := svc.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
		if err != nil || rc2.Timeouts != rc.Timeouts || rc2.ForwardingEnabled != rc.ForwardingEnabled || rc2.IpfixLoggingEnabled != rc.IpfixLoggingEnabled {
			t.Errorf("a non-owner changed a nat44-ei global: before %+v after %+v %v", rc, rc2, err)
		} else {
			t.Log("globals required only: timeouts/forwarding/ipfix unchanged (D-071)")
		}
	}()

	inside, inIdx := nattest.Loopback(t, c, 3)
	outside, outIdx := nattest.Loopback(t, c, 4)
	nattest.AddAddress(t, c, inIdx, nattest.Addr4(t, 30, 1)+"/24")
	nattest.AddAddress(t, c, outIdx, nattest.Addr4(t, 40, 1)+"/24")
	vrf := nattest.Table(t, c, 2, false)

	featIn := natcommon.MustEncode(&nat44ei.InterfaceFeatureSpec{Interface: inside, Side: nat44ei.SideInside})
	featOut := natcommon.MustEncode(&nat44ei.InterfaceFeatureSpec{Interface: outside, Side: nat44ei.SideOutside})
	nattest.CreateAll(ctx, t, p.InterfaceFeature, featIn, featOut)
	nattest.AssertPlan(t, p.InterfaceFeature, featIn, featOut)

	ifAddr := natcommon.MustEncode(&nat44ei.InterfaceAddressSpec{Interface: outside})
	nattest.CreateAll(ctx, t, p.InterfaceAddress, ifAddr)
	nattest.AssertPlan(t, p.InterfaceAddress, ifAddr)

	pool := natcommon.MustEncode(&nat44ei.AddressPoolSpec{First: nattest.Addr4(t, 3, 1), Last: nattest.Addr4(t, 3, 2), VRF: vrf})
	nattest.CreateAll(ctx, t, p.AddressPool, pool)
	nattest.AssertPlan(t, p.AddressPool, pool)

	pf := natcommon.MustEncode(&nat44ei.StaticMappingSpec{Name: "eipf", Local: nat44ei.Endpoint{IP: nattest.Addr4(t, 30, 50), Port: 80}, External: nat44ei.Endpoint{IP: nattest.Addr4(t, 3, 1), Port: 8080}, Protocol: "tcp", VRF: vrf})
	ifMap := natcommon.MustEncode(&nat44ei.StaticMappingSpec{Name: "eiif", Local: nat44ei.Endpoint{IP: nattest.Addr4(t, 30, 51)}, External: nat44ei.Endpoint{Interface: outside}, AddrOnly: true})
	nattest.CreateAll(ctx, t, p.StaticMapping, pf, ifMap)
	nattest.AssertPlan(t, p.StaticMapping, pf, ifMap)

	ident := natcommon.MustEncode(&nat44ei.IdentityMappingSpec{Name: "eiid", IP: nattest.Addr4(t, 3, 2), Protocol: "udp", Port: 500})
	nattest.CreateAll(ctx, t, p.IdentityMapping, ident)
	nattest.AssertPlan(t, p.IdentityMapping, ident)

	// output feature on a third loopback (VPP refuses in2out+output on one interface)
	third, _ := nattest.Loopback(t, c, 5)
	outFeat := natcommon.MustEncode(&nat44ei.OutputFeatureSpec{Interface: third})
	nattest.CreateAll(ctx, t, p.OutputFeature, outFeat)
	nattest.AssertPlan(t, p.OutputFeature, outFeat)

	users, err := p.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nat44-ei users: %d (shape only)", len(users))

	nattest.Pause(t, "nat44ei") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.OutputFeature)
	nattest.DeleteAll(ctx, t, p.IdentityMapping)
	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.AddressPool)
	nattest.DeleteAll(ctx, t, p.InterfaceAddress)
	nattest.DeleteAll(ctx, t, p.InterfaceFeature)
}
