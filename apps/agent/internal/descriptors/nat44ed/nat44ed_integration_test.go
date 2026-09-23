package nat44ed_test

import (
	"context"
	"testing"
	"time"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat44EdOnHost is the one integration check per object type against the host VPP
// (VRX_INTEGRATION=1, shared lab lock, slot prefix). The plugin enable is a global
// singleton: it is read first, enabled only when nobody has, never disabled if foreign
// objects exist, and restored in Cleanup. Every object carries the slot: loopbacks
// loop<N>xx, pool 10.<N>.0.0/16, tables N000–N999, tags w<N>:*.
func TestNat44EdOnHost(t *testing.T) {
	c := nattest.Connect(t)
	nattest.SlotLock(t, "nat44") // ED and EI are mutually exclusive; serialise this slot's packages
	ctx := nattest.Ctx(t)
	owner := vpptest.Prefix(t)
	p := nat44ed.New(c, owner)
	svc := nat44_ed.NewServiceClient(c)

	// --- global singleton: read first --------------------------------------------------------
	rc, err := svc.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}
	foreignEnabled := rc.Sessions != 0
	ei, err := nat44eiRunning(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if ei {
		t.Skip("nat44-ei is enabled on this VPP by another owner; ED and EI are mutually exclusive")
	}
	enable := natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024})
	if foreignEnabled {
		enable = natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: rc.Sessions, InsideVRF: rc.InsideVrf, OutsideVRF: rc.OutsideVrf, Out2InDPO: rc.Flags&nat44_ed.NAT44_IS_OUT2IN_DPO != 0})
		t.Logf("nat44-ed already enabled by another owner (sessions=%d): treating as converged, will not disable", rc.Sessions)
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Enable.Delete(cctx, enable, nil); err != nil {
				t.Errorf("restore: disable nat44-ed: %v", err)
			}
			if rc2, err := svc.Nat44ShowRunningConfig(cctx, &nat44_ed.Nat44ShowRunningConfig{}); err != nil || rc2.Sessions != 0 {
				t.Errorf("restore: nat44-ed still enabled: %+v %v", rc2, err)
			}
		})
	}
	if _, err := p.Enable.Create(ctx, enable); err != nil {
		t.Fatalf("enable: %v", err)
	}
	nattest.AssertPlan(t, p.Enable, enable)

	// timeouts: remember and restore
	oldTimeouts := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: rc.Timeouts.UDP, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP})
	tmo := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: 299, TCPEstablished: 7439, TCPTransitory: 239, ICMP: 59})
	if _, err := p.Timeouts.Create(ctx, tmo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if foreignEnabled {
			_, _ = p.Timeouts.Update(context.Background(), tmo, oldTimeouts, nil)
		}
	})
	nattest.AssertPlan(t, p.Timeouts, tmo)

	fwd := natcommon.MustEncode(&nat44ed.ForwardingSpec{Enabled: true})
	if !rc.ForwardingEnabled {
		if _, err := p.Forwarding.Create(ctx, fwd); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = p.Forwarding.Delete(context.Background(), fwd, nil) })
		nattest.AssertPlan(t, p.Forwarding, fwd)
	}

	// --- prefixed interfaces and table ------------------------------------------------------
	inside, inIdx := nattest.Loopback(t, c, 1)
	outside, outIdx := nattest.Loopback(t, c, 2)
	nattest.AddAddress(t, c, inIdx, nattest.Addr4(t, 10, 1)+"/24")
	nattest.AddAddress(t, c, outIdx, nattest.Addr4(t, 20, 1)+"/24")
	vrf := nattest.Table(t, c, 1, false)

	// --- interface features, output feature, interface address -------------------------------
	featIn := natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: inside, Side: nat44ed.SideInside})
	featOut := natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: outside, Side: nat44ed.SideOutside})
	nattest.CreateAll(ctx, t, p.InterfaceFeature, featIn, featOut)
	nattest.AssertPlan(t, p.InterfaceFeature, featIn, featOut)

	outFeat := natcommon.MustEncode(&nat44ed.OutputFeatureSpec{Interface: outside})
	nattest.CreateAll(ctx, t, p.OutputFeature, outFeat)
	nattest.AssertPlan(t, p.OutputFeature, outFeat)

	ifAddr := natcommon.MustEncode(&nat44ed.InterfaceAddressSpec{Interface: outside})
	nattest.CreateAll(ctx, t, p.InterfaceAddress, ifAddr)
	nattest.AssertPlan(t, p.InterfaceAddress, ifAddr)

	// --- pools (regular + twice-nat, in the slot block) ---------------------------------------
	pool := natcommon.MustEncode(&nat44ed.AddressPoolSpec{First: nattest.Addr4(t, 1, 1), Last: nattest.Addr4(t, 1, 4)})
	twice := natcommon.MustEncode(&nat44ed.AddressPoolSpec{First: nattest.Addr4(t, 2, 1), Last: nattest.Addr4(t, 2, 1), TwiceNAT: true})
	nattest.CreateAll(ctx, t, p.AddressPool, pool, twice)
	nattest.AssertPlan(t, p.AddressPool, pool, twice)

	// --- mappings ---------------------------------------------------------------------------
	pf := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "pf", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 50), Port: 80}, External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 1), Port: 8080}, Protocol: "tcp"})
	twiceMap := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "twice", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 51)}, External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 2)}, AddrOnly: true, TwiceNAT: true})
	ifMap := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "ifmap", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 52), Port: 22}, External: nat44ed.Endpoint{Interface: outside, Port: 2222}, Protocol: "tcp"})
	nattest.CreateAll(ctx, t, p.StaticMapping, pf, twiceMap, ifMap)
	nattest.AssertPlan(t, p.StaticMapping, pf, twiceMap, ifMap)

	ident := natcommon.MustEncode(&nat44ed.IdentityMappingSpec{Name: "id", IP: nattest.Addr4(t, 1, 3), Protocol: "udp", Port: 500})
	nattest.CreateAll(ctx, t, p.IdentityMapping, ident)
	nattest.AssertPlan(t, p.IdentityMapping, ident)

	lb := natcommon.MustEncode(&nat44ed.LBStaticMappingSpec{Name: "lb", External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 4), Port: 80}, Protocol: "tcp",
		Locals: []nat44ed.LBLocal{{IP: nattest.Addr4(t, 10, 60), Port: 8080, Probability: 50}, {IP: nattest.Addr4(t, 10, 61), Port: 8080, Probability: 50}}})
	nattest.CreateAll(ctx, t, p.LBStaticMapping, lb)
	nattest.AssertPlan(t, p.LBStaticMapping, lb)
	lb2 := natcommon.MustEncode(&nat44ed.LBStaticMappingSpec{Name: "lb", External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 4), Port: 80}, Protocol: "tcp",
		Locals: []nat44ed.LBLocal{{IP: nattest.Addr4(t, 10, 60), Port: 8080, Probability: 50}, {IP: nattest.Addr4(t, 10, 62), Port: 8080, Probability: 50}}})
	if _, err := p.LBStaticMapping.Update(ctx, lb, lb2, nil); err != nil {
		t.Fatalf("lb locals update: %v", err)
	}
	nattest.AssertPlan(t, p.LBStaticMapping, lb2)

	// --- vrf table --------------------------------------------------------------------------
	tbl := natcommon.MustEncode(&nat44ed.VRFTableSpec{Table: vrf, Routes: []uint32{0}})
	nattest.CreateAll(ctx, t, p.VRFTable, tbl)
	nattest.AssertPlan(t, p.VRFTable, tbl)

	// --- sessions: shape only (no traffic on the host) ----------------------------------------
	users, err := p.Users(ctx)
	if err != nil {
		t.Fatalf("users: %v", err)
	}
	for _, u := range users {
		if _, err := p.UserSessions(ctx, u, 0, 10); err != nil {
			t.Fatalf("sessions of %s: %v", u.IP, err)
		}
	}
	t.Logf("nat44-ed session users on host: %d (asserted for shape only)", len(users))

	// --- delete everything (reverse order), Retrieve shows nothing of ours --------------------
	nattest.DeleteAll(ctx, t, p.VRFTable)
	nattest.DeleteAll(ctx, t, p.LBStaticMapping)
	nattest.DeleteAll(ctx, t, p.IdentityMapping)
	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.AddressPool)
	nattest.DeleteAll(ctx, t, p.InterfaceAddress)
	nattest.DeleteAll(ctx, t, p.OutputFeature)
	nattest.DeleteAll(ctx, t, p.InterfaceFeature)
}

// nat44eiRunning reports whether the nat44-ei plugin is enabled (sessions != 0 in its
// running config, exactly like ED).
func nat44eiRunning(ctx context.Context, c *nattest.Conn) (bool, error) {
	rep, err := nat44_ei.NewServiceClient(c).Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		return false, err
	}
	return rep.Sessions != 0, nil
}
