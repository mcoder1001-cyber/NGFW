package mapnat_test

import (
	"errors"
	"testing"

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

	// global parameters (D-071): the slot is not the globals owner — the current values are
	// required (satisfied), other values fail, and nothing is ever set
	before, err := svc.MapParamGet(ctx, &maps.MapParamGet{})
	if err != nil {
		t.Fatal(err)
	}
	// only the fields VPP fills: map_param_get leaves ip4_lifetime_ms / ip4_pool_size /
	// ip4_buffers / ip4_ht_ratio uninitialised (vl_msg_api_alloc, map_api.c) — re-review N1
	cur := modelled(before)
	if _, err := p.Params.Create(ctx, natcommon.MustEncode(&cur)); err != nil {
		t.Fatalf("params requirement (current): %v", err)
	}
	other := cur
	other.TCClass++
	if _, err := p.Params.Create(ctx, natcommon.MustEncode(&other)); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("params requirement (other) must fail: %v", err)
	}
	if err := p.Params.Delete(ctx, natcommon.MustEncode(&cur), nil); err != nil {
		t.Fatal(err)
	}
	if after, err := svc.MapParamGet(ctx, &maps.MapParamGet{}); err != nil || modelled(after) != cur {
		t.Fatalf("a non-owner changed MAP params: %+v → %+v %v", cur, after, err)
	}
	t.Log("map params required only, unchanged (D-071)")

	nattest.Pause(t, "map") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Rule)
	nattest.DeleteAll(ctx, t, p.Domain)
}

// modelled is the part of map_param_get_reply that VPP actually fills (and ParamsSpec models).
func modelled(r *maps.MapParamGetReply) mapnat.ParamsSpec {
	s := mapnat.ParamsSpec{FragInner: r.FragInner != 0, FragIgnoreDF: r.FragIgnoreDf != 0, ICMPRelaySrc: natcommon.IP4String(r.ICMPIP4ErrRelaySrc),
		ICMP6Unreachable: r.ICMP6EnableUnreachable, SecurityCheck: r.SecCheckEnable, SecurityCheckFrags: r.SecCheckFragments, TCCopy: r.TcCopy, TCClass: uint32(r.TcClass)}
	s.Normalize()
	return s
}
