package mapnat_test

import (
	"context"
	"testing"
	"time"

	maps "ngfw/agent/binapi/map"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestMapOnHost: one integration check per MAP object type on the host VPP. Domains carry the
// owner tag "w<N>:<name>" and slot addresses (10.<N>.46.0/24, fd00:<N>:46::/48); the
// parameter singleton is touched only when it is at VPP's defaults and restored in Cleanup;
// the MAP feature is enabled only on this slot's loopbacks.
func TestMapOnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	owner := vpptest.Prefix(t)
	p := mapnat.New(c, owner)
	svc := maps.NewServiceClient(c)

	// domain (LW4o6-style: ea_bits_len 0, 4 PSID bits → per-PSID rules)
	dom := natcommon.MustEncode(&mapnat.DomainSpec{Name: owner + "-lw", IP4Prefix: nattest.Addr4(t, 46, 0) + "/24",
		IP6Prefix: nattest.Prefix6(t, 0x46, 48), IP6Src: nattest.Addr6(t, 0x4601) + "/128", PSIDOffset: 6, PSIDLength: 4, MTU: 1460})
	nattest.CreateAll(ctx, t, p.Domain, dom)
	nattest.AssertPlan(t, p.Domain, dom)

	r1 := natcommon.MustEncode(&mapnat.RuleSpec{Domain: owner + "-lw", PSID: 3, IP6Dst: nattest.Addr6(t, 0x4603)})
	r2 := natcommon.MustEncode(&mapnat.RuleSpec{Domain: owner + "-lw", PSID: 9, IP6Dst: nattest.Addr6(t, 0x4609)})
	nattest.CreateAll(ctx, t, p.Rule, r1, r2)
	nattest.AssertPlan(t, p.Rule, r1, r2)
	// in-place destination change converges
	r2b := natcommon.MustEncode(&mapnat.RuleSpec{Domain: owner + "-lw", PSID: 9, IP6Dst: nattest.Addr6(t, 0x4699)})
	if n := nattest.Apply(t, p.Rule, r1, r2b); n != 1 {
		t.Fatalf("rule update planned %d ops", n)
	}
	nattest.AssertPlan(t, p.Rule, r1, r2b)

	// interfaces: MAP-E on one loopback, MAP-T on another
	e, _ := nattest.Loopback(t, c, 30)
	tr, _ := nattest.Loopback(t, c, 31)
	ie := natcommon.MustEncode(&mapnat.InterfaceSpec{Interface: e})
	it := natcommon.MustEncode(&mapnat.InterfaceSpec{Interface: tr, Translation: true})
	nattest.CreateAll(ctx, t, p.Interface, ie, it)
	nattest.AssertPlan(t, p.Interface, ie, it)

	nattest.Pause(t, "map") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	// global parameters: only from defaults, restored in Cleanup
	if kvs, err := p.Params.Retrieve(ctx); err != nil {
		t.Fatal(err)
	} else if len(kvs) != 0 {
		t.Logf("map params already changed by another owner (%v): not touching the global", kvs[0].Value)
	} else {
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.Params.Delete(cctx, natcommon.MustEncode(&mapnat.DefaultParams), nil); err != nil {
				t.Errorf("restore map params: %v", err)
			}
		})
		want := natcommon.MustEncode(&mapnat.ParamsSpec{FragInner: true, SecurityCheck: true, TCCopy: false, TCClass: 8})
		if n := nattest.Apply(t, p.Params, want); n != 1 {
			t.Fatalf("params planned %d", n)
		}
		nattest.AssertPlan(t, p.Params, want)
		nattest.DeleteAll(ctx, t, p.Params)
		rep, err := svc.MapParamGet(ctx, &maps.MapParamGet{})
		if err != nil || rep.FragInner != 0 || !rep.SecCheckEnable || !rep.TcCopy || rep.TcClass != 0 {
			t.Fatalf("params not restored: %+v %v", rep, err)
		}
		t.Log("map params restored to VPP defaults")
	}

	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Rule)
	nattest.DeleteAll(ctx, t, p.Domain)
}
