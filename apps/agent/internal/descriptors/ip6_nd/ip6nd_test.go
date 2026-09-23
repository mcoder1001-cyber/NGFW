package ip6nd

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_dad"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

// radv mirrors VPP's per-interface RA state and applies sw_interface_ip6nd_ra_config with the
// toggle semantics of ip6_ra.c (a zero field leaves the setting alone; is_no restores the
// default of every non-zero field).
type radv struct {
	det      ip6_nd.SwInterfaceIP6ndRaDetails
	prefixes []ip6_nd.IP6ndRaPrefix
}

func defaultRadv(idx uint32) *radv {
	return &radv{det: ip6_nd.SwInterfaceIP6ndRaDetails{
		SwIfIndex: interface_types.InterfaceIndex(idx), SendRadv: true, AdvLinkLayerAddress: true,
		AdvRouterLifetime: DefaultRouterLifetime, MaxRadvInterval: DefaultMaxInterval, MinRadvInterval: DefaultMinInterval,
		InitialAdvertsCount: DefaultInitialCount, InitialAdvertsInterval: DefaultInitialInterval, CurHopLimit: 64,
	}}
}

func (r *radv) apply(m *ip6_nd.SwInterfaceIP6ndRaConfig) int32 {
	no := m.IsNo
	set := func(flag uint8, cur *bool, on bool) {
		if flag != 0 {
			*cur = on != no
		}
	}
	d := &r.det
	set(m.Suppress, &d.SendRadv, false)
	set(m.Managed, &d.AdvManagedFlag, true)
	set(m.Other, &d.AdvOtherFlag, true)
	set(m.LlOption, &d.AdvLinkLayerAddress, false)
	set(m.SendUnicast, &d.SendUnicast, true)
	set(m.Cease, &d.CeaseRadv, true)
	maxI, minI := m.MaxInterval, m.MinInterval
	if maxI != 0 && minI == 0 {
		minI = maxI * 3 / 4
	}
	if maxI != 0 {
		if no {
			maxI = DefaultMaxInterval
		}
		d.MaxRadvInterval = float64(maxI)
	}
	if minI != 0 {
		if no {
			minI = DefaultMinInterval
		}
		d.MinRadvInterval = float64(minI)
	}
	if m.DefaultRouter != 0 {
		lt := m.Lifetime
		if no {
			lt = DefaultRouterLifetime
		}
		if lt != 0 && float64(lt) < d.MaxRadvInterval {
			return -1
		}
		d.AdvRouterLifetime = uint16(lt) //nolint:gosec // test values
	}
	if m.InitialCount != 0 {
		d.InitialAdvertsCount = m.InitialCount
		if no {
			d.InitialAdvertsCount = DefaultInitialCount
		}
	}
	if m.InitialInterval != 0 {
		d.InitialAdvertsInterval = float64(m.InitialInterval)
		if no {
			d.InitialAdvertsInterval = DefaultInitialInterval
		}
	}
	return 0
}

type fakeVPP struct {
	*fake.Client
	radvs   map[uint32]*radv
	proxies []ip6_nd.IP6ndProxyDetails
	proxyOn map[uint32]bool
	dad     ip6_dad.IP6DadDetails
	dadOff  bool // plugin not loaded
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), radvs: map[uint32]*radv{}, proxyOn: map[uint32]bool{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 6, InterfaceName: "loop301", Tag: "w3:loop301"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.On("sw_interface_ip6nd_ra_config", func(req api.Message) ([]api.Message, error) {
		m := req.(*ip6_nd.SwInterfaceIP6ndRaConfig)
		r, ok := v.radvs[uint32(m.SwIfIndex)]
		if !ok {
			return []api.Message{&ip6_nd.SwInterfaceIP6ndRaConfigReply{Retval: -25}}, nil // IP6_NOT_ENABLED
		}
		return []api.Message{&ip6_nd.SwInterfaceIP6ndRaConfigReply{Retval: r.apply(m)}}, nil
	})
	v.On("sw_interface_ip6nd_ra_prefix", func(req api.Message) ([]api.Message, error) {
		m := req.(*ip6_nd.SwInterfaceIP6ndRaPrefix)
		r, ok := v.radvs[uint32(m.SwIfIndex)]
		if !ok {
			return []api.Message{&ip6_nd.SwInterfaceIP6ndRaPrefixReply{Retval: -25}}, nil
		}
		for i, p := range r.prefixes {
			if p.Prefix == m.Prefix {
				if m.IsNo {
					r.prefixes = append(r.prefixes[:i], r.prefixes[i+1:]...)
				} else {
					r.prefixes[i] = ip6_nd.IP6ndRaPrefix{Prefix: m.Prefix, OnlinkFlag: !m.OffLink, AutonomousFlag: !m.NoAutoconfig, ValLifetime: m.ValLifetime, PrefLifetime: m.PrefLifetime, NoAdvertise: m.NoAdvertise}
				}
				return []api.Message{&ip6_nd.SwInterfaceIP6ndRaPrefixReply{}}, nil
			}
		}
		if m.IsNo {
			return []api.Message{&ip6_nd.SwInterfaceIP6ndRaPrefixReply{Retval: -6}}, nil
		}
		r.prefixes = append(r.prefixes, ip6_nd.IP6ndRaPrefix{Prefix: m.Prefix, OnlinkFlag: !m.OffLink, AutonomousFlag: !m.NoAutoconfig, ValLifetime: m.ValLifetime, PrefLifetime: m.PrefLifetime, NoAdvertise: m.NoAdvertise})
		return []api.Message{&ip6_nd.SwInterfaceIP6ndRaPrefixReply{}}, nil
	})
	v.On("sw_interface_ip6nd_ra_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if r, ok := v.radvs[idx]; ok {
				det := r.det
				det.Prefixes = append([]ip6_nd.IP6ndRaPrefix(nil), r.prefixes...)
				det.NPrefixes = uint32(len(det.Prefixes)) //nolint:gosec // test
				out = append(out, &det)
			}
		}
		return out, nil
	})
	v.On("ip6nd_proxy_enable_disable", func(req api.Message) ([]api.Message, error) {
		m := req.(*ip6_nd.IP6ndProxyEnableDisable)
		v.proxyOn[uint32(m.SwIfIndex)] = m.IsEnable
		return []api.Message{&ip6_nd.IP6ndProxyEnableDisableReply{}}, nil
	})
	v.On("ip6nd_proxy_add_del", func(req api.Message) ([]api.Message, error) {
		m := req.(*ip6_nd.IP6ndProxyAddDel)
		for i, p := range v.proxies {
			if p.SwIfIndex == m.SwIfIndex && p.IP == m.IP {
				if !m.IsAdd {
					v.proxies = append(v.proxies[:i], v.proxies[i+1:]...)
				}
				return []api.Message{&ip6_nd.IP6ndProxyAddDelReply{}}, nil
			}
		}
		if m.IsAdd {
			v.proxies = append(v.proxies, ip6_nd.IP6ndProxyDetails{SwIfIndex: m.SwIfIndex, IP: m.IP})
		}
		return []api.Message{&ip6_nd.IP6ndProxyAddDelReply{}}, nil
	})
	v.On("ip6nd_proxy_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.proxies))
		for i := range v.proxies {
			p := v.proxies[i]
			out = append(out, &p)
		}
		return out, nil
	})
	notLoaded := func(name string) error { return &adapter.UnknownMsgError{MsgName: name, MsgCrc: "deadbeef"} }
	v.On("ip6_dad_enable_disable", func(req api.Message) ([]api.Message, error) {
		if v.dadOff {
			return nil, notLoaded("ip6_dad_enable_disable")
		}
		m := req.(*ip6_dad.IP6DadEnableDisable)
		v.dad = ip6_dad.IP6DadDetails{Enabled: m.Enable, DadTransmits: m.DadTransmits, DadRetransmitDelay: m.DadRetransmitDelay}
		return []api.Message{&ip6_dad.IP6DadEnableDisableReply{}}, nil
	})
	v.On("ip6_dad_dump", func(api.Message) ([]api.Message, error) {
		if v.dadOff {
			return nil, notLoaded("ip6_dad_dump")
		}
		d := v.dad
		return []api.Message{&d}, nil
	})
	return v
}

func find(kvs []scheduler.KV, k scheduler.Key) *scheduler.KV {
	for i := range kvs {
		if kvs[i].Key == k {
			return &kvs[i]
		}
	}
	return nil
}

func TestRaConfigLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.radvs[5], v.radvs[6], v.radvs[7] = defaultRadv(5), defaultRadv(6), defaultRadv(7)
	v.radvs[7].det.AdvManagedFlag = true // another worker's non-default RA config
	d := NewRaConfig(v, "w3")
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	desired := &RaConfig{Interface: "loop300", Managed: true, Other: true, SuppressLinkLayerOption: true, RouterLifetime: 1800, MaxInterval: 300}
	if k := d.KeyOf(desired); k != "ip6-nd.ra-config/loop300" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	norm := NormalizeRaConfig(desired)
	if norm.MinInterval != 225 || norm.InitialCount != 3 || norm.InitialInterval != 16 || norm.MaxInterval != 300 {
		t.Fatalf("Normalize = %+v", norm)
	}

	// Default interfaces are not retrieved; the foreign one is filtered by owner.
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("Retrieve before Create = %+v, %v", actual, err)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (RaMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	calls := v.CallsNamed("sw_interface_ip6nd_ra_config")
	if len(calls) != 2 || !calls[0].(*ip6_nd.SwInterfaceIP6ndRaConfig).IsNo || calls[1].(*ip6_nd.SwInterfaceIP6ndRaConfig).IsNo {
		t.Fatalf("expected reset + set, got %+v", calls)
	}
	set := calls[1].(*ip6_nd.SwInterfaceIP6ndRaConfig)
	if set.Managed != 1 || set.Other != 1 || set.LlOption != 1 || set.Suppress != 0 || set.DefaultRouter != 1 || set.Lifetime != 1800 || set.MaxInterval != 300 || set.MinInterval != 225 {
		t.Fatalf("set request = %+v", set)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if !proto.Equal(actual[0].Value, norm) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, want %+v", actual[0].Value, norm)
	}

	// Update: fewer flags must clear the ones no longer desired (that is what the reset does).
	updated := &RaConfig{Interface: "loop300", Suppress: true, RouterLifetime: 0}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	actual, _ = d.Retrieve(ctx)
	if got := actual[0].Value.(*RaConfig); !proto.Equal(got, NormalizeRaConfig(updated)) {
		t.Fatalf("after Update = %+v", got)
	}
	if _, err := d.Update(ctx, desired, &RaConfig{Interface: "loop301"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("interface change: %v", err)
	}
	// Explicit defaults are indistinguishable from "unconfigured" (documented).
	if _, err := d.Create(ctx, &RaConfig{Interface: "loop301", RouterLifetime: DefaultRouterLifetime}); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 1 {
		t.Fatalf("default config must not be retrieved: %+v", actual)
	}
	// Delete → defaults → nothing retrieved; the foreign interface is untouched.
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || !v.radvs[7].det.AdvManagedFlag {
		t.Fatalf("after Delete = %+v", actual)
	}

	// Errors: lifetime below max interval, IPv6 not enabled, unknown interface, bad meta.
	if _, err := d.Create(ctx, &RaConfig{Interface: "loop300", RouterLifetime: 100, MaxInterval: 300}); err == nil {
		t.Fatal("lifetime < max_interval accepted")
	}
	delete(v.radvs, 6)
	if _, err := d.Create(ctx, &RaConfig{Interface: "loop301", Managed: true}); err == nil {
		t.Fatal("VPP retval must surface")
	}
	if _, err := d.Create(ctx, &RaConfig{Interface: "nope"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestRaPrefixLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.radvs[5], v.radvs[7] = defaultRadv(5), defaultRadv(7)
	foreign, _ := ip_types.ParsePrefix("2001:db8:2::/64")
	v.radvs[7].prefixes = []ip6_nd.IP6ndRaPrefix{{Prefix: foreign, OnlinkFlag: true, AutonomousFlag: true, ValLifetime: 1, PrefLifetime: 1}}
	d := NewRaPrefix(v, "w3")
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	desired := &RaPrefix{Interface: "loop300", Prefix: "2001:DB8:3:0:1::/64", OffLink: true}
	if k := d.KeyOf(desired); k != "ip6-nd.ra-prefix/loop300/2001:db8:3::/64" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 2 || deps[0].Key != "interface/loop300" || deps[1].Key != "interface-ip/loop300/2001:db8:3::/64" || !deps[1].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (RaMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("sw_interface_ip6nd_ra_prefix")[0].(*ip6_nd.SwInterfaceIP6ndRaPrefix)
	if req.IsNo || req.UseDefault || !req.OffLink || req.NoAutoconfig || req.ValLifetime != DefaultValidLifetime || req.PrefLifetime != DefaultPreferredLifetime || req.Prefix.Len != 64 {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, NormalizeRaPrefix(desired)) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	updated := &RaPrefix{Interface: "loop300", Prefix: "2001:db8:3::/64", NoAutoconfig: true, NoAdvertise: true, ValidLifetime: 7200, PreferredLifetime: 3600}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); !proto.Equal(actual[0].Value, updated) {
		t.Fatalf("after Update = %+v", actual[0].Value)
	}
	if _, err := d.Update(ctx, desired, &RaPrefix{Interface: "loop300", Prefix: "2001:db8:4::/64"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("prefix change: %v", err)
	}
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.radvs[7].prefixes) != 1 {
		t.Fatalf("after Delete = %+v", actual)
	}
	if _, err := d.Create(ctx, &RaPrefix{Interface: "loop300", Prefix: "10.3.0.0/24"}); err == nil {
		t.Fatal("IPv4 prefix accepted")
	}
	if _, err := d.Create(ctx, &RaPrefix{Interface: "loop300", Prefix: "2001:db8:3::/64", ValidLifetime: 10, PreferredLifetime: 20}); err == nil {
		t.Fatal("preferred > valid accepted")
	}
	if err := d.Delete(ctx, updated, meta); err == nil {
		t.Fatal("deleting a missing prefix must surface the retval")
	}
}

func TestProxyNdLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	other, _ := ip_types.ParseIP6Address("2001:db8:2::9")
	v.proxies = []ip6_nd.IP6ndProxyDetails{{SwIfIndex: 7, IP: other}}
	d := NewProxyNd(v, "w3")
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	a := &ProxyNd{Interface: "loop300", Address: "2001:DB8:3::0010"}
	b := &ProxyNd{Interface: "loop300", Address: "2001:db8:3::11"}
	if k := d.KeyOf(a); k != "ip6-nd.proxy/loop300/2001:db8:3::10" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(a); len(deps) != 1 || deps[0].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	metaA, err := d.Create(ctx, a)
	if err != nil || metaA != (ProxyNdMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, metaA)
	}
	if _, err := d.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if !v.proxyOn[5] || len(v.CallsNamed("ip6nd_proxy_add_del")) != 2 {
		t.Fatalf("proxy not enabled/added: on=%v", v.proxyOn)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 2 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if kv := find(actual, d.KeyOf(a)); kv == nil || !proto.Equal(kv.Value, &ProxyNd{Interface: "loop300", Address: "2001:db8:3::10"}) || kv.Meta != metaA {
		t.Fatalf("Retrieve[a] = %+v", kv)
	}
	if _, err := d.Update(ctx, a, b, metaA); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	// Deleting one address keeps the interface feature on; deleting the last one turns it off.
	if err := d.Delete(ctx, a, metaA); err != nil {
		t.Fatal(err)
	}
	if !v.proxyOn[5] {
		t.Fatal("feature disabled while an address remains")
	}
	if err := d.Delete(ctx, b, ProxyNdMeta{SwIfIndex: 5}); err != nil {
		t.Fatal(err)
	}
	if v.proxyOn[5] {
		t.Fatal("feature still enabled after the last address")
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.proxies) != 1 {
		t.Fatalf("after Delete = %+v proxies=%+v", actual, v.proxies)
	}
	if _, err := d.Create(ctx, &ProxyNd{Interface: "loop300", Address: "10.3.0.1"}); err == nil {
		t.Fatal("IPv4 accepted")
	}
	if err := d.Delete(ctx, a, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestDadLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	d := NewDad(v)
	if !scheduler.ValidName(d.Name()) || d.Dependencies(&Dad{}) != nil {
		t.Fatal(d.Name())
	}
	if k := d.KeyOf(&Dad{}); k != "ip6-nd.dad/global" {
		t.Fatalf("KeyOf = %s", k)
	}
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("disabled DAD must be absent: %+v %v", actual, err)
	}
	desired := &Dad{Transmits: 3}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("ip6_dad_enable_disable")[0].(*ip6_dad.IP6DadEnableDisable)
	if !req.Enable || req.DadTransmits != 3 || req.DadRetransmitDelay != 1 {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, NormalizeDad(desired)) {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := d.Update(ctx, desired, &Dad{Transmits: 2, RetransmitDelay: 0.5}, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); actual[0].Value.(*Dad).GetRetransmitDelay() != 0.5 {
		t.Fatalf("after Update = %+v", actual)
	}
	if err := d.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete = %+v", actual)
	}
	if _, err := d.Create(ctx, &Dad{Transmits: 300}); err == nil {
		t.Fatal("transmits > 255 accepted")
	}
	// Plugin not loaded → typed error the integration test skips on.
	v.dadOff = true
	if _, err := d.Retrieve(ctx); !errors.Is(err, df2.ErrPluginNotLoaded) {
		t.Fatalf("not loaded: %v", err)
	}
	if _, err := d.Create(ctx, desired); !errors.Is(err, df2.ErrPluginNotLoaded) {
		t.Fatalf("not loaded: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3")
	want := []string{RaConfigName, RaPrefixName, ProxyNdName, DadName}
	got := reg.Names()
	if len(got) != len(want) {
		t.Fatalf("Names = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names = %v", got)
		}
	}
}
