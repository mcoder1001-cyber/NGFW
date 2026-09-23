package nat44ei_test

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat44EiOnHost: one integration check per nat44-ei object type. The plugin is a global
// singleton and mutually exclusive with nat44-ed: skip when ED is enabled (by anyone), enable
// EI only when nobody has, restore in Cleanup.
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
	rc, err := svc.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}
	enable := natcommon.MustEncode(&nat44ei.EnableSpec{})
	if rc.Sessions != 0 {
		enable = natcommon.MustEncode(&nat44ei.EnableSpec{InsideVRF: rc.InsideVrf, OutsideVRF: rc.OutsideVrf,
			StaticMappingOnly: rc.Flags&nat44_ei.NAT44_EI_STATIC_MAPPING_ONLY != 0, ConnectionTracking: rc.Flags&nat44_ei.NAT44_EI_CONNECTION_TRACKING != 0, Out2InDPO: rc.Flags&nat44_ei.NAT44_EI_OUT2IN_DPO != 0})
		t.Logf("nat44-ei already enabled by another owner: converged, will not disable")
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Enable.Delete(cctx, enable, nil); err != nil {
				t.Errorf("restore: disable nat44-ei: %v", err)
			}
			if rc2, err := svc.Nat44EiShowRunningConfig(cctx, &nat44_ei.Nat44EiShowRunningConfig{}); err != nil || rc2.Sessions != 0 {
				t.Errorf("restore: nat44-ei still enabled: %+v %v", rc2, err)
			}
		})
	}
	if _, err := p.Enable.Create(ctx, enable); err != nil {
		t.Fatalf("enable: %v", err)
	}
	nattest.AssertPlan(t, p.Enable, enable)

	tmo := natcommon.MustEncode(&nat44ei.TimeoutsSpec{UDP: 299, TCPEstablished: 7439, TCPTransitory: 239, ICMP: 59})
	nattest.CreateAll(ctx, t, p.Timeouts, tmo)
	nattest.AssertPlan(t, p.Timeouts, tmo)
	fwd := natcommon.MustEncode(&nat44ei.ForwardingSpec{})
	nattest.CreateAll(ctx, t, p.Forwarding, fwd)
	nattest.AssertPlan(t, p.Forwarding, fwd)

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

	nattest.DeleteAll(ctx, t, p.OutputFeature)
	nattest.DeleteAll(ctx, t, p.IdentityMapping)
	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.AddressPool)
	nattest.DeleteAll(ctx, t, p.InterfaceAddress)
	nattest.DeleteAll(ctx, t, p.InterfaceFeature)
	nattest.DeleteAll(ctx, t, p.Forwarding)
}
