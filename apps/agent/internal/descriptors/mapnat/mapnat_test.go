package mapnat_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip_types"
	maps "ngfw/agent/binapi/map"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeMap struct {
	*fake.Client
	next    uint32
	domains map[uint32]*maps.MapDomainDetails
	rules   map[uint32]map[uint16]ip_types.IP6Address
	params  maps.MapParamGetReply
	encap   map[uint32]bool
	trans   map[uint32]bool
}

func newFakeMap() *fakeMap {
	f := &fakeMap{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		domains: map[uint32]*maps.MapDomainDetails{}, rules: map[uint32]map[uint16]ip_types.IP6Address{},
		params: maps.MapParamGetReply{SecCheckEnable: true, TcCopy: true}, encap: map[uint32]bool{}, trans: map[uint32]bool{}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop930", Tag: "w9:loop930"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop330", Tag: "w3:loop330"})
	f.On("map_add_domain", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapAddDomain)
		idx := f.next
		f.next++
		f.domains[idx] = &maps.MapDomainDetails{DomainIndex: idx, IP4Prefix: r.IP4Prefix, IP6Prefix: r.IP6Prefix, IP6Src: r.IP6Src,
			EaBitsLen: r.EaBitsLen, PsidOffset: r.PsidOffset, PsidLength: r.PsidLength, Mtu: r.Mtu, Tag: r.Tag}
		return []api.Message{&maps.MapAddDomainReply{Index: idx}}, nil
	})
	f.On("map_del_domain", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapDelDomain)
		if _, ok := f.domains[r.Index]; !ok {
			return []api.Message{&maps.MapDelDomainReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		delete(f.domains, r.Index)
		delete(f.rules, r.Index)
		return []api.Message{&maps.MapDelDomainReply{}}, nil
	})
	f.On("map_domain_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for i := uint32(0); i < f.next; i++ {
			if d, ok := f.domains[i]; ok {
				out = append(out, d)
			}
		}
		return out, nil
	})
	f.On("map_add_del_rule", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapAddDelRule)
		d, ok := f.domains[r.Index]
		if !ok || d.EaBitsLen > 0 || int(r.Psid) >= 1<<d.PsidLength {
			return []api.Message{&maps.MapAddDelRuleReply{Retval: -1}}, nil
		}
		if f.rules[r.Index] == nil {
			f.rules[r.Index] = map[uint16]ip_types.IP6Address{}
		}
		if r.IsAdd {
			f.rules[r.Index][r.Psid] = r.IP6Dst
		} else {
			delete(f.rules[r.Index], r.Psid)
		}
		return []api.Message{&maps.MapAddDelRuleReply{}}, nil
	})
	f.On("map_rule_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapRuleDump)
		var out []api.Message
		for psid := uint16(0); psid < 1024; psid++ {
			if dst, ok := f.rules[r.DomainIndex][psid]; ok {
				out = append(out, &maps.MapRuleDetails{Psid: psid, IP6Dst: dst})
			}
		}
		return out, nil
	})
	f.On("map_param_get", func(api.Message) ([]api.Message, error) {
		p := f.params
		return []api.Message{&p}, nil
	})
	f.On("map_param_set_fragmentation", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapParamSetFragmentation)
		f.params.FragInner, f.params.FragIgnoreDf = b2u(r.Inner), b2u(r.IgnoreDf)
		return []api.Message{&maps.MapParamSetFragmentationReply{}}, nil
	})
	f.On("map_param_set_icmp", func(req api.Message) ([]api.Message, error) {
		f.params.ICMPIP4ErrRelaySrc = req.(*maps.MapParamSetICMP).IP4ErrRelaySrc
		return []api.Message{&maps.MapParamSetICMPReply{}}, nil
	})
	f.On("map_param_set_icmp6", func(req api.Message) ([]api.Message, error) {
		f.params.ICMP6EnableUnreachable = req.(*maps.MapParamSetICMP6).EnableUnreachable
		return []api.Message{&maps.MapParamSetICMP6Reply{}}, nil
	})
	f.On("map_param_set_security_check", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapParamSetSecurityCheck)
		f.params.SecCheckEnable, f.params.SecCheckFragments = r.Enable, r.Fragments
		return []api.Message{&maps.MapParamSetSecurityCheckReply{}}, nil
	})
	f.On("map_param_set_traffic_class", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapParamSetTrafficClass)
		f.params.TcCopy, f.params.TcClass = r.Copy, r.TcClass
		return []api.Message{&maps.MapParamSetTrafficClassReply{}}, nil
	})
	f.On("map_if_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapIfEnableDisable)
		m := f.encap
		if r.IsTranslation {
			m = f.trans
		}
		m[uint32(r.SwIfIndex)] = r.IsEnable
		return []api.Message{&maps.MapIfEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		idx := uint32(r.SwIfIndex)
		on := false
		switch {
		case r.ArcName != "ip4-unicast":
		case r.FeatureName == "ip4-map":
			on = f.encap[idx]
		case r.FeatureName == "ip4-map-t":
			on = f.trans[idx]
		}
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: on}}, nil
	})
	return f
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

var owner = natcommon.WithGlobalsOwner(true)

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	mapnat.Register(reg, newFakeMap(), "w9")
	if reg.Len() != 4 {
		t.Fatalf("registered %d", reg.Len())
	}
}

func TestDomainAndRules(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9", owner)
	ctx := context.Background()

	// a foreign (w3) and an untagged domain must stay invisible
	f.domains[90] = &maps.MapDomainDetails{DomainIndex: 90, Tag: "w3:other"}
	f.domains[91] = &maps.MapDomainDetails{DomainIndex: 91}
	f.next = 92

	lw := natcommon.MustEncode(&mapnat.DomainSpec{Name: "lw", IP4Prefix: "10.9.46.7/24", IP6Prefix: "fd00:9:46::/48", IP6Src: "fd00:9::1/128", PSIDOffset: 6, PSIDLength: 4, MTU: 1460})
	if nattest.Apply(t, p.Domain, lw) != 1 || nattest.Apply(t, p.Domain, lw) != 0 {
		t.Fatal("domain create / idempotent")
	}
	req := f.CallsNamed("map_add_domain")[0].(*maps.MapAddDomain)
	if req.Tag != "w9:lw" || req.IP4Prefix.Len != 24 || req.IP4Prefix.Address != [4]uint8{10, 9, 46, 0} || req.PsidLength != 4 || req.Mtu != 1460 {
		t.Fatalf("map_add_domain %+v", req)
	}
	if keys := nattest.Keys(t, p.Domain); len(keys) != 1 || keys[0] != "map.domain/lw" {
		t.Fatalf("domain keys %v", keys)
	}
	kvs, _ := p.Domain.Retrieve(ctx)
	if m, ok := kvs[0].Meta.(mapnat.DomainMeta); !ok || m.Index != 92 {
		t.Fatalf("meta %+v", kvs[0].Meta)
	}

	r1 := natcommon.MustEncode(&mapnat.RuleSpec{Domain: "lw", PSID: 3, IP6Dst: "fd00:9:46::0003"})
	r2 := natcommon.MustEncode(&mapnat.RuleSpec{Domain: "lw", PSID: 7, IP6Dst: "fd00:9:46::7"})
	if deps := p.Rule.Dependencies(r1); len(deps) != 1 || deps[0].Key != "map.domain/lw" || deps[0].Optional {
		t.Fatalf("rule deps %+v", deps)
	}
	if nattest.Apply(t, p.Rule, r1, r2) != 2 || nattest.Apply(t, p.Rule, r1, r2) != 0 {
		t.Fatal("rules")
	}
	if keys := nattest.Keys(t, p.Rule); len(keys) != 2 || keys[0] != "map.rule/lw/3" || keys[1] != "map.rule/lw/7" {
		t.Fatalf("rule keys %v", keys)
	}
	// destination change = in-place re-add
	r2b := natcommon.MustEncode(&mapnat.RuleSpec{Domain: "lw", PSID: 7, IP6Dst: "fd00:9:46::77"})
	if nattest.Apply(t, p.Rule, r1, r2b) != 1 || natcommon.IP6String(f.rules[92][7]) != "fd00:9:46::77" {
		t.Fatal("rule update")
	}
	if nattest.Apply(t, p.Rule, r1) != 1 || len(f.rules[92]) != 1 {
		t.Fatal("rule leftover delete")
	}
	// a rule of an unknown domain fails cleanly
	if _, err := p.Rule.Create(ctx, natcommon.MustEncode(&mapnat.RuleSpec{Domain: "nope", PSID: 1, IP6Dst: "fd00:9::1"})); !errors.Is(err, mapnat.ErrNoDomainIndex) {
		t.Fatalf("unknown domain: %v", err)
	}
	// VPP error surfaces (psid outside 2^psid_length)
	if _, err := p.Rule.Create(ctx, natcommon.MustEncode(&mapnat.RuleSpec{Domain: "lw", PSID: 16, IP6Dst: "fd00:9::1"})); err == nil {
		t.Fatal("psid out of bounds must fail")
	}
	// any domain field change → recreate
	lw2 := natcommon.MustEncode(&mapnat.DomainSpec{Name: "lw", IP4Prefix: "10.9.46.0/24", IP6Prefix: "fd00:9:46::/48", IP6Src: "fd00:9::1/128", PSIDOffset: 6, PSIDLength: 4, MTU: 1500})
	if _, err := p.Domain.Update(ctx, lw, lw2, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("domain update: %v", err)
	}
	if nattest.Apply(t, p.Domain) != 1 || len(f.domains) != 2 {
		t.Fatal("domain delete, foreign kept")
	}
	if _, err := p.Domain.Create(ctx, natcommon.MustEncode(&mapnat.DomainSpec{Name: "bad", IP4Prefix: "10.9.0.0/24", IP6Prefix: "fd00:9::/48", IP6Src: "fd00:9::1/128", EABitsLen: 300})); err == nil {
		t.Fatal("ea_bits_len range")
	}
}

func TestParams(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9", owner)
	if len(nattest.Keys(t, p.Params)) != 0 {
		t.Fatal("defaults = no object")
	}
	want := natcommon.MustEncode(&mapnat.ParamsSpec{FragInner: true, ICMPRelaySrc: "10.9.46.1", SecurityCheck: true, TCClass: 32})
	if nattest.Apply(t, p.Params, want) != 1 || nattest.Apply(t, p.Params, want) != 0 {
		t.Fatal("params")
	}
	if f.params.FragInner != 1 || f.params.TcCopy || f.params.TcClass != 32 || natcommon.IP4String(f.params.ICMPIP4ErrRelaySrc) != "10.9.46.1" {
		t.Fatalf("params on VPP %+v", f.params)
	}
	upd := natcommon.MustEncode(&mapnat.ParamsSpec{SecurityCheck: true, TCCopy: true, ICMP6Unreachable: true})
	if nattest.Apply(t, p.Params, upd) != 1 || !f.params.ICMP6EnableUnreachable || f.params.FragInner != 0 {
		t.Fatal("params update in place")
	}
	if nattest.Apply(t, p.Params) != 1 || len(nattest.Keys(t, p.Params)) != 0 || !f.params.SecCheckEnable || !f.params.TcCopy {
		t.Fatal("params delete restores defaults")
	}
}

func TestInterface(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9", owner)
	f.encap[3] = true // w3's interface: invisible
	e := natcommon.MustEncode(&mapnat.InterfaceSpec{Interface: "loop930"})
	tr := natcommon.MustEncode(&mapnat.InterfaceSpec{Interface: "loop930", Translation: true})
	if deps := p.Interface.Dependencies(e); len(deps) != 1 || deps[0].Key != "interface/loop930" {
		t.Fatalf("deps %+v", deps)
	}
	if nattest.Apply(t, p.Interface, e, tr) != 2 || nattest.Apply(t, p.Interface, e, tr) != 0 {
		t.Fatal("interface map-e + map-t")
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 2 || keys[0] != "map.interface/loop930/map-e" || keys[1] != "map.interface/loop930/map-t" {
		t.Fatalf("keys %v", keys)
	}
	if nattest.Apply(t, p.Interface, tr) != 1 || f.encap[1] || !f.trans[1] {
		t.Fatal("map-e removed, map-t kept")
	}
	if nattest.Apply(t, p.Interface) != 1 || f.trans[1] || !f.encap[3] {
		t.Fatal("delete leftovers, foreign kept")
	}
	for _, c := range f.CallsNamed("feature_is_enabled") {
		if c.(*feature.FeatureIsEnabled).SwIfIndex == 0 || c.(*feature.FeatureIsEnabled).SwIfIndex == 3 {
			t.Fatal("never probe interfaces we do not own")
		}
	}
	if _, err := p.Interface.Create(context.Background(), natcommon.MustEncode(&mapnat.InterfaceSpec{Interface: "loop999"})); !errors.Is(err, natcommon.ErrNoSuchInterface) {
		t.Fatalf("missing interface: %v", err)
	}
}

// TestDeleteReverifies is review finding 3 for MAP: a delete by index re-verifies the tag at
// that index right before map_del_domain / map_add_del_rule and never deletes another
// owner's domain that reused the index; duplicate tags surface as "<name>#<index>" extras.
func TestDeleteReverifies(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9", owner)
	ctx := context.Background()
	dom := natcommon.MustEncode(&mapnat.DomainSpec{Name: "d", IP4Prefix: "10.9.46.0/24", IP6Prefix: "fd00:9:46::/48", IP6Src: "fd00:9::1/128", PSIDLength: 4})
	meta, err := p.Domain.Create(ctx, dom)
	if err != nil {
		t.Fatal(err)
	}
	// index 0 is now another owner's domain (VPP restart + re-creation by w3)
	f.domains[0] = &maps.MapDomainDetails{DomainIndex: 0, Tag: "w3:theirs"}
	if err := p.Domain.Delete(ctx, dom, meta); err != nil || f.domains[0] == nil || len(f.CallsNamed("map_del_domain")) != 0 {
		t.Fatalf("stale delete must not touch the reused index: %v", err)
	}
	if err := p.Rule.Delete(ctx, natcommon.MustEncode(&mapnat.RuleSpec{Domain: "d", PSID: 1, IP6Dst: "fd00:9::5"}), meta); err != nil || len(f.CallsNamed("map_add_del_rule")) != 0 {
		t.Fatalf("stale rule delete must not touch the reused index: %v", err)
	}
	// duplicate tags (retried create): lowest index canonical, the extra is reported and deleted
	f.domains = map[uint32]*maps.MapDomainDetails{}
	f.next = 0
	if _, err := p.Domain.Create(ctx, dom); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Domain.Create(ctx, dom); err != nil {
		t.Fatal(err)
	}
	if keys := nattest.Keys(t, p.Domain); len(keys) != 2 || keys[0] != "map.domain/d" || keys[1] != "map.domain/d#1" {
		t.Fatalf("keys %v", keys)
	}
	if nattest.Apply(t, p.Domain, dom) != 1 || len(f.domains) != 1 || f.domains[0] == nil {
		t.Fatal("extra deleted, canonical kept")
	}
	if _, err := p.Domain.Create(ctx, natcommon.MustEncode(&mapnat.DomainSpec{Name: "x#1", IP4Prefix: "10.9.47.0/24", IP6Prefix: "fd00:9:47::/48", IP6Src: "fd00:9::1/128"})); err == nil {
		t.Fatal("'#' in a domain name must be rejected")
	}
}

// TestParamsNonOwner: MAP params are a VPP global (D-071); a non-owner requires, never sets.
func TestParamsNonOwner(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9")
	ctx := context.Background()
	if _, err := p.Params.Create(ctx, natcommon.MustEncode(&mapnat.DefaultParams)); err != nil {
		t.Fatalf("defaults required and present: %v", err)
	}
	if _, err := p.Params.Create(ctx, natcommon.MustEncode(&mapnat.ParamsSpec{FragInner: true})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	if err := p.Params.Delete(ctx, natcommon.MustEncode(&mapnat.DefaultParams), nil); err != nil {
		t.Fatal(err)
	}
	nattest.AssertWriteOnly(t, p.Params)
	for _, c := range f.Calls() {
		if n := c.GetMessageName(); len(n) > 14 && n[:14] == "map_param_set_" {
			t.Fatalf("non-owner sent %s", n)
		}
	}
}
