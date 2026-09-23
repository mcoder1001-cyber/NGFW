package nat44ed_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// edFixture is the nat44-ed plugin as a test fixture (D-071): enabled by the test if off,
// disabled again only if this test enabled it and nat44-ed is completely empty.
func edFixture(c *nattest.Conn, p *nat44ed.Plugin) nattest.Plugin {
	svc := nat44_ed.NewServiceClient(c)
	return nattest.Plugin{
		Name: "nat44-ed",
		Enable: func(ctx context.Context) (bool, error) {
			_, err := svc.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: true, Sessions: 1024})
			if natcommon.IsAlreadyEnabled(err) {
				return true, nil
			}
			return false, err
		},
		Empty: p.Empty,
		Disable: func(ctx context.Context) error {
			_, err := svc.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: false})
			return err
		},
	}
}

// TestNat44EdOnHost is the one integration check per object type against the host VPP
// (VRX_INTEGRATION=1, shared lab lock, slot prefix). The slot is NOT the globals owner
// (D-071): the plugin is a fixture (enabled if off, previous state restored), and the global
// descriptors (enable, timeouts, forwarding) are exercised as requirements only — they never
// set, reset or disable anything. Every object carries the slot: loopbacks loop<N>xx, pool
// 10.<N>.0.0/16, tables N000–N999, tags w<N>:*.
func TestNat44EdOnHost(t *testing.T) {
	c := nattest.Connect(t)
	nattest.SlotLock(t, "nat44") // ED and EI are mutually exclusive; serialise this slot's packages
	ctx := nattest.Ctx(t)
	owner := vpptest.Prefix(t)
	p := nat44ed.New(c, owner) // default config: not the globals owner
	svc := nat44_ed.NewServiceClient(c)

	ei, err := nat44eiRunning(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if ei {
		t.Skip("nat44-ei is enabled on this VPP by another owner; ED and EI are mutually exclusive")
	}
	wasOn := nattest.EnsurePlugin(t, edFixture(c, p))
	rc, err := svc.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// --- globals as requirements (D-071): current values satisfy, others fail, nothing changes
	cur := natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: rc.Sessions, InsideVRF: rc.InsideVrf, OutsideVRF: rc.OutsideVrf, Out2InDPO: rc.Flags&nat44_ed.NAT44_IS_OUT2IN_DPO != 0})
	if _, err := p.Enable.Create(ctx, cur); err != nil {
		t.Fatalf("enable requirement (current config): %v", err)
	}
	if _, err := p.Enable.Create(ctx, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: rc.Sessions + 1})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("enable requirement (other config) must fail: %v", err)
	}
	nattest.AssertWriteOnly(t, p.Enable)
	curTmo := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: rc.Timeouts.UDP, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP})
	if _, err := p.Timeouts.Create(ctx, curTmo); err != nil {
		t.Fatalf("timeouts requirement (current): %v", err)
	}
	other := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: rc.Timeouts.UDP + 1, TCPEstablished: rc.Timeouts.TCPEstablished, TCPTransitory: rc.Timeouts.TCPTransitory, ICMP: rc.Timeouts.ICMP})
	if _, err := p.Timeouts.Create(ctx, other); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("timeouts requirement (other) must fail: %v", err)
	}
	if err := p.Timeouts.Delete(ctx, curTmo, nil); err != nil {
		t.Fatal(err)
	}
	fwd := natcommon.MustEncode(&nat44ed.ForwardingSpec{})
	if _, err := p.Forwarding.Create(ctx, fwd); rc.ForwardingEnabled != (err == nil) {
		t.Fatalf("forwarding requirement (on=%v): %v", rc.ForwardingEnabled, err)
	}
	if rc2, err := svc.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{}); err != nil || rc2.Timeouts != rc.Timeouts || rc2.ForwardingEnabled != rc.ForwardingEnabled || rc2.Sessions != rc.Sessions {
		t.Fatalf("a non-owner changed a global: before %+v after %+v %v", rc, rc2, err)
	}
	t.Log("globals required only: timeouts/forwarding/enable unchanged (D-071)")

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

	// VPP refuses a NAT interface to also be an output-feature interface (VALUE_EXIST): own loopback
	outputIf, _ := nattest.Loopback(t, c, 6)
	outFeat := natcommon.MustEncode(&nat44ed.OutputFeatureSpec{Interface: outputIf})
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
	// VPP 26.06 rejects twice-NAT on address-only mappings (UNSUPPORTED): twice-NAT with ports,
	// address-only 1:1 separately, and an interface-bound external.
	twiceMap := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "twice", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 51), Port: 443}, External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 2), Port: 8443}, Protocol: "tcp", TwiceNAT: true})
	one2one := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "one2one", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 53)}, External: nat44ed.Endpoint{IP: nattest.Addr4(t, 1, 3)}, AddrOnly: true})
	ifMap := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "ifmap", Local: nat44ed.Endpoint{IP: nattest.Addr4(t, 10, 52), Port: 22}, External: nat44ed.Endpoint{Interface: outside, Port: 2222}, Protocol: "tcp"})
	nattest.CreateAll(ctx, t, p.StaticMapping, pf, twiceMap, one2one, ifMap)
	nattest.AssertPlan(t, p.StaticMapping, pf, twiceMap, one2one, ifMap)

	ident := natcommon.MustEncode(&nat44ed.IdentityMappingSpec{Name: "id", IP: nattest.Addr4(t, 1, 4), Protocol: "udp", Port: 500})
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
	nattest.Pause(t, "nat44ed") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.VRFTable)
	nattest.DeleteAll(ctx, t, p.LBStaticMapping)
	nattest.DeleteAll(ctx, t, p.IdentityMapping)
	nattest.DeleteAll(ctx, t, p.StaticMapping)
	nattest.DeleteAll(ctx, t, p.AddressPool)
	nattest.DeleteAll(ctx, t, p.InterfaceAddress)
	nattest.DeleteAll(ctx, t, p.OutputFeature)
	nattest.DeleteAll(ctx, t, p.InterfaceFeature)

	// --- review finding 1 regression (H1): w9 can never disable NAT under another owner's object
	// a non-owner's Delete of the enable singleton is a no-op
	if err := p.Enable.Delete(ctx, cur, nil); err != nil || !edEnabled(ctx, t, svc) {
		t.Fatalf("non-owner Delete disabled nat44-ed: %v", err)
	}
	if wasOn {
		t.Log("H1: nat44-ed was enabled by another owner; non-owner Delete left it enabled")
		return
	}
	// only when THIS test enabled the plugin: a foreign owner's output-feature interface (the
	// object kind the original check missed) must keep even the globals owner's Delete from
	// disabling nat44-ed
	foreignIf, foreignIdx := nattest.LoopbackOwnedBy(t, c, 7, owner+"b")
	if _, err := svc.Nat44EdAddDelOutputInterface(ctx, &nat44_ed.Nat44EdAddDelOutputInterface{IsAdd: true, SwIfIndex: interface_types.InterfaceIndex(foreignIdx)}); err != nil {
		t.Fatal(err)
	}
	removeForeign := func() {
		_, _ = svc.Nat44EdAddDelOutputInterface(context.Background(), &nat44_ed.Nat44EdAddDelOutputInterface{IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(foreignIdx)})
	}
	t.Cleanup(removeForeign)
	globalsOwner := nat44ed.New(c, owner, natcommon.WithGlobalsOwner(true))
	if err := globalsOwner.Enable.Delete(ctx, cur, nil); err != nil || !edEnabled(ctx, t, svc) {
		t.Fatalf("H1: disable went through with %s's output interface %s present: %v", owner+"b", foreignIf, err)
	}
	t.Logf("H1: %s's output-feature interface %s present → globals-owner Delete skipped, nat44-ed still enabled", owner+"b", foreignIf)
	removeForeign()
}

func edEnabled(ctx context.Context, t *testing.T, svc nat44_ed.RPCService) bool {
	t.Helper()
	rc, err := svc.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return rc.Sessions != 0
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
