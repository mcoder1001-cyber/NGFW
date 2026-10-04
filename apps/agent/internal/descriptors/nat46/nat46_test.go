package nat46_test

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	maps "ngfw/agent/binapi/map"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/nat46"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/fake"
)

func sample() nat46.Config {
	return nat46.Config{
		ClientPrefix: "64:ff9b::/96",
		Interfaces:   []string{"loop931", "loop930"},
		Mappings: []nat46.Mapping{
			{Name: "web", IPv4: "203.0.113.10", IPv6: "2001:db8:46::10", MTU: 1500},
			{Name: "mail", IPv4: "203.0.113.25", IPv6: "2001:db8:46::25"},
		},
	}
}

func TestProject(t *testing.T) {
	p, err := nat46.Project(sample())
	if err != nil {
		t.Fatal(err)
	}
	want := []mapnat.DomainSpec{
		{Name: "nat46-mail", IP4Prefix: "203.0.113.25/32", IP6Prefix: "2001:db8:46::25/128", IP6Src: "64:ff9b::/96"},
		{Name: "nat46-web", IP4Prefix: "203.0.113.10/32", IP6Prefix: "2001:db8:46::10/128", IP6Src: "64:ff9b::/96", MTU: 1500},
	}
	if !reflect.DeepEqual(p.Domains, want) {
		t.Fatalf("domains %+v", p.Domains)
	}
	if len(p.Interfaces) != 2 || p.Interfaces[0] != (mapnat.InterfaceSpec{Interface: "loop930", Translation: true}) {
		t.Fatalf("interfaces %+v", p.Interfaces)
	}
	// round trip
	c, err := nat46.Assemble(p.Domains, p.Interfaces)
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientPrefix != "64:ff9b::/96" || len(c.Mappings) != 2 || c.Mappings[1] != sample().Mappings[0] ||
		!reflect.DeepEqual(c.Interfaces, []string{"loop930", "loop931"}) {
		t.Fatalf("assemble %+v", c)
	}
	// empty config projects to nothing
	if p, err := nat46.Project(nat46.Config{}); err != nil || len(p.Domains)+len(p.Interfaces) != 0 {
		t.Fatalf("empty: %+v %v", p, err)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*nat46.Config)
		field string
	}{
		{"prefix length", func(c *nat46.Config) { c.ClientPrefix = "64:ff9b::/64" }, "clientPrefix"},
		{"prefix v4", func(c *nat46.Config) { c.ClientPrefix = "10.0.0.0/8" }, "clientPrefix"},
		{"prefix host bits", func(c *nat46.Config) { c.ClientPrefix = "64:ff9b::1/96" }, "clientPrefix"},
		{"no interfaces", func(c *nat46.Config) { c.Interfaces = nil }, "interfaces"},
		{"dup interface", func(c *nat46.Config) { c.Interfaces = []string{"a", "a"} }, "interfaces/1"},
		{"dup v4", func(c *nat46.Config) { c.Mappings[1].IPv4 = "203.0.113.10" }, "mappings/1/ipv4"},
		{"bad v4", func(c *nat46.Config) { c.Mappings[0].IPv4 = "2001:db8::1" }, "mappings/0/ipv4"},
		{"multicast v4", func(c *nat46.Config) { c.Mappings[0].IPv4 = "224.0.0.1" }, "mappings/0/ipv4"},
		{"dup v6", func(c *nat46.Config) { c.Mappings[1].IPv6 = "2001:db8:46::10" }, "mappings/1/ipv6"},
		{"v6 in client prefix", func(c *nat46.Config) { c.Mappings[0].IPv6 = "64:ff9b::cb00:710a" }, "mappings/0/ipv6"},
		{"dup name", func(c *nat46.Config) { c.Mappings[1].Name = "web" }, "mappings/1/name"},
		{"name #", func(c *nat46.Config) { c.Mappings[0].Name = "a#1" }, "mappings/0/name"},
		{"mtu", func(c *nat46.Config) { c.Mappings[0].MTU = 1000 }, "mappings/0/mtu"},
	}
	for _, tc := range cases {
		c := sample()
		tc.edit(&c)
		errs := nat46.Validate(c)
		if len(errs) == 0 || errs[0].Field != tc.field {
			t.Errorf("%s: %v", tc.name, errs)
			continue
		}
		if _, err := nat46.Project(c); !errors.Is(err, nat46.ErrInvalid) {
			t.Errorf("%s: Project err %v", tc.name, err)
		}
	}
	if errs := nat46.Validate(sample()); errs != nil {
		t.Fatalf("valid sample: %v", errs)
	}
}

func TestAssembleIgnoresForeignShapes(t *testing.T) {
	ds := []mapnat.DomainSpec{
		{Name: "lw", IP4Prefix: "10.0.0.0/24", IP6Prefix: "fd00::/48", IP6Src: "fd00::1/128", PSIDLength: 4},
		{Name: "nat46-x", IP4Prefix: "10.0.0.0/24", IP6Prefix: "fd00::/48", IP6Src: "64:ff9b::/96"},
		{Name: "nat46-ok", IP4Prefix: "198.51.100.1/32", IP6Prefix: "2001:db8::1/128", IP6Src: "64:ff9b::/96"},
	}
	c, err := nat46.Assemble(ds, []mapnat.InterfaceSpec{{Interface: "e", Translation: false}})
	if err != nil || len(c.Mappings) != 1 || c.Mappings[0].Name != "ok" || len(c.Interfaces) != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	ds = append(ds, mapnat.DomainSpec{Name: "nat46-y", IP4Prefix: "198.51.100.2/32", IP6Prefix: "2001:db8::2/128", IP6Src: "fd00:64::/96"})
	if _, err := nat46.Assemble(ds, nil); err == nil {
		t.Fatal("two client prefixes accepted")
	}
}

func TestClientAddress(t *testing.T) {
	a, err := nat46.ClientAddress("64:ff9b::/96", "192.0.2.33")
	if err != nil || a != "64:ff9b::c000:221" {
		t.Fatalf("%q %v", a, err)
	}
	if _, err := nat46.ClientAddress("64:ff9b::/64", "192.0.2.33"); !errors.Is(err, nat46.ErrInvalid) {
		t.Fatal(err)
	}
}

// fakeMap models the MAP messages the projection reaches through mapnat.
type fakeMap struct {
	*fake.Client
	next    uint32
	domains map[uint32]*maps.MapDomainDetails
	trans   map[uint32]bool
}

func newFakeMap() *fakeMap {
	f := &fakeMap{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), domains: map[uint32]*maps.MapDomainDetails{}, trans: map[uint32]bool{}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop930", Tag: "w9:loop930"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop931", Tag: "w9:loop931"})
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
	f.On("map_if_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*maps.MapIfEnableDisable)
		if r.IsTranslation {
			f.trans[uint32(r.SwIfIndex)] = r.IsEnable
		}
		return []api.Message{&maps.MapIfEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		on := r.ArcName == "ip4-unicast" && r.FeatureName == "ip4-map-t" && f.trans[uint32(r.SwIfIndex)]
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: on}}, nil
	})
	return f
}

// TestApplyThroughMapnat: the projection applied by the mapnat descriptors on the fake VPP —
// map_add_domain carries the 1:1 shape, Retrieve + Assemble gives the config back, a second
// apply plans nothing, and removing the config deletes every domain and interface feature.
func TestApplyThroughMapnat(t *testing.T) {
	f := newFakeMap()
	p := mapnat.New(f, "w9", natcommon.WithGlobalsOwner(true))
	pr, err := nat46.Project(sample())
	if err != nil {
		t.Fatal(err)
	}
	var doms, ifs []proto.Message
	for i := range pr.Domains {
		doms = append(doms, natcommon.MustEncode(&pr.Domains[i]))
	}
	for i := range pr.Interfaces {
		ifs = append(ifs, natcommon.MustEncode(&pr.Interfaces[i]))
	}
	if n := nattest.Apply(t, p.Domain, doms...); n != 2 {
		t.Fatalf("domain ops %d", n)
	}
	if n := nattest.Apply(t, p.Interface, ifs...); n != 2 {
		t.Fatalf("interface ops %d", n)
	}
	req := f.CallsNamed("map_add_domain")[0].(*maps.MapAddDomain)
	if req.Tag != "w9:nat46-mail" || req.IP4Prefix.Len != 32 || req.IP6Prefix.Len != 128 || req.IP6Src.Len != 96 || req.EaBitsLen != 0 || req.PsidLength != 0 {
		t.Fatalf("map_add_domain %+v", req)
	}
	if nattest.Apply(t, p.Domain, doms...) != 0 || nattest.Apply(t, p.Interface, ifs...) != 0 {
		t.Fatal("second apply not idempotent")
	}

	ctx := context.Background()
	gotD, err := p.Domain.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gotI, err := p.Interface.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ds []mapnat.DomainSpec
	for _, kv := range gotD {
		s, err := natcommon.Decode[mapnat.DomainSpec](kv.Value)
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, s)
	}
	var is []mapnat.InterfaceSpec
	for _, kv := range gotI {
		s, err := natcommon.Decode[mapnat.InterfaceSpec](kv.Value)
		if err != nil {
			t.Fatal(err)
		}
		is = append(is, s)
	}
	c, err := nat46.Assemble(ds, is)
	if err != nil {
		t.Fatal(err)
	}
	want := sample()
	want.Interfaces = []string{"loop930", "loop931"}
	want.Mappings = []nat46.Mapping{want.Mappings[1], want.Mappings[0]}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("assembled %+v", c)
	}

	// rollback: nothing desired → everything removed
	nattest.Apply(t, p.Interface)
	nattest.Apply(t, p.Domain)
	if len(f.domains) != 0 || f.trans[1] || f.trans[2] {
		t.Fatalf("left behind: domains %v trans %v", f.domains, f.trans)
	}
}

// TestEmbeddedReturnPath (F-nat46-return-path): a server <P>::<ipv4> projects to ip6-pfx P/64 —
// the only shape VPP 26.06's ip6-map-t finds for replies (its LPM never matches > /64 and it takes
// the IPv4 source from the last 32 bits) — and assembles back to the configured server.
func TestEmbeddedReturnPath(t *testing.T) {
	yes := [][2]string{{"10.17.46.10", "fd00:11:46::a11:2e0a"}, {"203.0.113.10", "2001:db8:46::cb00:710a"}}
	no := [][2]string{{"10.17.46.10", "fd00:11:2::46"}, {"10.17.46.10", "fd00:11:46:0:1::a11:2e0a"}, {"10.17.46.10", "fd00:11:46::a11:2e0b"}}
	for _, c := range yes {
		if !nat46.Embedded(netip.MustParseAddr(c[0]), netip.MustParseAddr(c[1])) {
			t.Errorf("Embedded(%s, %s) = false", c[0], c[1])
		}
	}
	for _, c := range no {
		if nat46.Embedded(netip.MustParseAddr(c[0]), netip.MustParseAddr(c[1])) {
			t.Errorf("Embedded(%s, %s) = true", c[0], c[1])
		}
	}
	cfg := nat46.Config{ClientPrefix: "fd00:11:4646::/96", Interfaces: []string{"a", "b"}, Mappings: []nat46.Mapping{
		{Name: "web", IPv4: "10.17.46.10", IPv6: "fd00:11:46::a11:2e0a"},
		{Name: "legacy", IPv4: "10.17.46.11", IPv6: "fd00:11:2::46"},
	}}
	p, err := nat46.Project(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []mapnat.DomainSpec{
		{Name: "nat46-legacy", IP4Prefix: "10.17.46.11/32", IP6Prefix: "fd00:11:2::46/128", IP6Src: "fd00:11:4646::/96"},
		{Name: "nat46-web", IP4Prefix: "10.17.46.10/32", IP6Prefix: "fd00:11:46::/64", IP6Src: "fd00:11:4646::/96"},
	}
	if !reflect.DeepEqual(p.Domains, want) {
		t.Fatalf("domains %+v", p.Domains)
	}
	for _, d := range p.Domains {
		if !nat46.IsNAT46Domain(d) {
			t.Fatalf("IsNAT46Domain(%+v) = false", d)
		}
	}
	back, err := nat46.Assemble(p.Domains, p.Interfaces)
	if err != nil {
		t.Fatal(err)
	}
	if back.Mappings[1] != cfg.Mappings[0] || back.Mappings[0] != cfg.Mappings[1] {
		t.Fatalf("assemble %+v", back.Mappings)
	}
	// two embedded servers in one /64 share one VPP LPM entry → refused with a pointer
	cfg.Mappings[1] = nat46.Mapping{Name: "mail", IPv4: "10.17.46.25", IPv6: "fd00:11:46::a11:2e19"}
	errs := nat46.Validate(cfg)
	if len(errs) != 1 || errs[0].Field != "mappings/1/ipv6" {
		t.Fatalf("shared /64: %v", errs)
	}
}

func TestEmbeddedForeignFamilyRefused(t *testing.T) {
	for _, d := range []mapnat.DomainSpec{
		{Name: "nat46-foreign", IP4Prefix: "2001:db8::/32", IP6Prefix: "2001:db8:46::/64", IP6Src: "64:ff9b::/96"},
		{Name: "nat46-foreign", IP4Prefix: "192.0.2.1/32", IP6Prefix: "::ffff:192.0.2.1/128", IP6Src: "64:ff9b::/96"},
	} {
		if nat46.IsNAT46Domain(d) {
			t.Fatalf("foreign family accepted: %+v", d)
		}
		got, err := nat46.Assemble([]mapnat.DomainSpec{d}, nil)
		if err != nil || len(got.Mappings) != 0 {
			t.Fatalf("foreign family assembled: %+v error %v", got, err)
		}
	}
}
