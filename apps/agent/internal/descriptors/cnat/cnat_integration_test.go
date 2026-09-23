package cnat_test

import (
	"testing"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/internal/descriptors/cnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestCnatOnHost: one integration check per cnat object type on the host VPP. Every address
// is in the slot block (VIP 10.<N>.47.1, backends 10.<N>.48.x, SNAT 10.<N>.49.1, excluded
// 10.<N>.50.0/24); features only on this slot's loopbacks. The default SNAT entry is a global
// singleton: the snat-* part runs only when no other owner holds it, and it is deleted again
// before the test ends (DeleteAll + CreateAll's cleanup).
func TestCnatOnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := cnat.New(c, vpptest.Prefix(t))

	// translations (no SNAT entry needed; the dependency is optional)
	tr := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 80, Proto: "tcp",
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 1), DstPort: 8080}, {Dst: nattest.Addr4(t, 48, 2), DstPort: 8080}}})
	tr2 := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 53, Proto: "udp", LBType: cnat.LBMaglev, AllocPort: true,
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 3), DstPort: 5353}}})
	nattest.CreateAll(ctx, t, p.Translation, tr, tr2)
	nattest.AssertPlan(t, p.Translation, tr, tr2)
	// backend change converges in place
	tr2b := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 53, Proto: "udp", LBType: cnat.LBMaglev, AllocPort: true,
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 3), DstPort: 5353}, {Dst: nattest.Addr4(t, 48, 4), DstPort: 5353, NoNAT: true}}})
	if n := nattest.Apply(t, p.Translation, tr, tr2b); n != 1 {
		t.Fatalf("translation update planned %d ops", n)
	}
	nattest.AssertPlan(t, p.Translation, tr, tr2b)

	// per-interface cnat feature
	l0, _ := nattest.Loopback(t, c, 40)
	l1, _ := nattest.Loopback(t, c, 41)
	feat := natcommon.MustEncode(&cnat.InterfaceFeatureSpec{Interface: l0})
	nattest.CreateAll(ctx, t, p.InterfaceFeature, feat)
	nattest.AssertPlan(t, p.InterfaceFeature, feat)

	if sess, err := p.Sessions(ctx, 0, 100); err != nil {
		t.Fatalf("session dump: %v", err)
	} else {
		t.Logf("cnat sessions: %d (shape only)", len(sess))
	}

	// default SNAT entry and its dependents — only when nobody else holds it
	cur, err := cnatapi.NewServiceClient(c).CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{})
	if err == nil {
		t.Logf("cnat default SNAT entry held by another owner (%+v): skipping the snat-* checks", cur)
	} else {
		snat := natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: nattest.Addr4(t, 49, 1), IP6: nattest.Addr6(t, 0x4901)})
		nattest.CreateAll(ctx, t, p.SnatAddresses, snat)
		nattest.AssertPlan(t, p.SnatAddresses, snat)

		pol := natcommon.MustEncode(&cnat.SnatPolicySpec{Policy: cnat.PolicyIfPfx})
		nattest.CreateAll(ctx, t, p.SnatPolicy, pol)
		nattest.AssertPlan(t, p.SnatPolicy, pol)

		sif := natcommon.MustEncode(&cnat.SnatInterfaceSpec{Interface: l1, Table: cnat.TableIncludeV4})
		nattest.CreateAll(ctx, t, p.SnatInterface, sif)
		nattest.AssertPlan(t, p.SnatInterface, sif)

		ex := natcommon.MustEncode(&cnat.SnatExcludePrefixSpec{Prefix: nattest.Addr4(t, 50, 0) + "/24"})
		nattest.CreateAll(ctx, t, p.SnatExcludePfx, ex)
		nattest.AssertPlan(t, p.SnatExcludePfx, ex)

		nattest.DeleteAll(ctx, t, p.SnatExcludePfx)
		nattest.DeleteAll(ctx, t, p.SnatInterface)
		nattest.DeleteAll(ctx, t, p.SnatPolicy)
		nattest.DeleteAll(ctx, t, p.SnatAddresses)
		if _, err := cnatapi.NewServiceClient(c).CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{}); err == nil {
			t.Fatal("default SNAT entry still present after delete")
		}
		t.Log("cnat default SNAT entry removed again (global restored)")
	}

	nattest.DeleteAll(ctx, t, p.InterfaceFeature)
	nattest.DeleteAll(ctx, t, p.Translation)
}
