package natcommon_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type spec struct {
	Addr string   `json:"addr"`
	VRF  uint32   `json:"vrf"`
	On   bool     `json:"on"`
	List []string `json:"list"`
}

func (s *spec) Normalize() {
	s.Addr = natcommon.CanonAddr(s.Addr)
	natcommon.SortStrings(s.List)
}

func TestEncodeDecodeCanonical(t *testing.T) {
	a := natcommon.MustEncode(&spec{Addr: "FD00:0009:0000::0001", VRF: 9001, On: true, List: []string{"b", "a"}})
	b := natcommon.MustEncode(&spec{Addr: "fd00:9::1", VRF: 9001, On: true, List: []string{"a", "b"}})
	if !proto.Equal(a, b) {
		t.Fatalf("canonical encodings differ:\n%v\n%v", a, b)
	}
	got, err := natcommon.Decode[spec](a)
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != "fd00:9::1" || got.VRF != 9001 || !got.On || len(got.List) != 2 || got.List[0] != "a" {
		t.Fatalf("decoded %+v", got)
	}
	// every field present even when zero → proto.Equal is a correct diff
	z := natcommon.MustEncode(&spec{})
	for _, k := range []string{"addr", "vrf", "on", "list"} {
		if _, ok := z.GetFields()[k]; !ok {
			t.Fatalf("zero spec lacks field %q: %v", k, z)
		}
	}
	if _, err := natcommon.Decode[spec](&structpb.Value{}); err == nil {
		t.Fatal("decode of a non-Struct must fail")
	}
	unknown, _ := structpb.NewStruct(map[string]any{"addr": "10.9.0.1", "bogus": 1})
	if _, err := natcommon.Decode[spec](unknown); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}

func TestScope(t *testing.T) {
	s := natcommon.ScopeFor("w9")
	if s.All || !s.OwnsAddrString("10.9.255.1") || s.OwnsAddrString("10.8.0.1") || !s.OwnsTable(9000) || !s.OwnsTable(9999) || s.OwnsTable(0) || s.OwnsTable(10000) {
		t.Fatalf("slot scope wrong: %+v", s)
	}
	if !s.OwnsAddrString("fd00:9::1") || s.OwnsAddrString("fd00:a::1") {
		t.Fatalf("v6 slot scope wrong: %+v", s)
	}
	if !s.OwnsInterface(natcommon.Iface{SwIfIndex: 5, Tag: "w9:loop900"}) || s.OwnsInterface(natcommon.Iface{SwIfIndex: 5, Tag: "w3:loop300"}) || s.OwnsInterface(natcommon.Iface{SwIfIndex: 0}) {
		t.Fatal("interface ownership by tag wrong")
	}
	tag, err := s.Tag("pool1")
	if err != nil || tag != "w9:pool1" {
		t.Fatalf("tag %q %v", tag, err)
	}
	if id, ok := s.ParseTag("w9:pool1"); !ok || id != "pool1" {
		t.Fatal("parse tag")
	}
	for _, owner := range []string{"vrx", "", "w0", "wx", "w256"} {
		if p := natcommon.ScopeFor(owner); !p.All || !p.OwnsAddrString("192.0.2.1") || !p.OwnsTable(0) || !p.OwnsInterface(natcommon.Iface{}) {
			t.Fatalf("owner %q must own everything: %+v", owner, p)
		}
	}
	if d, ok := natcommon.VRFDep(0); ok || d.Key != "" {
		t.Fatal("table 0 yields no dependency")
	}
	if d, ok := natcommon.VRFDep(9001); !ok || d.Key != "vrf/9001" || !d.Optional {
		t.Fatalf("vrf dep %+v", d)
	}
	if natcommon.InterfaceDep("loop900").Key != "interface/loop900" {
		t.Fatal("interface dep key")
	}
}

func TestAddrHelpers(t *testing.T) {
	a4, err := natcommon.IP4("10.9.0.1")
	if err != nil || natcommon.IP4String(a4) != "10.9.0.1" {
		t.Fatal(err)
	}
	if _, err := natcommon.IP4("fd00::1"); err == nil {
		t.Fatal("v6 must not parse as v4")
	}
	a6, err := natcommon.IP6("fd00:9::1")
	if err != nil || natcommon.IP6String(a6) != "fd00:9::1" {
		t.Fatal(err)
	}
	p4, err := natcommon.Prefix4("10.9.1.7/24")
	if err != nil || natcommon.Prefix4String(p4) != "10.9.1.0/24" {
		t.Fatalf("%v %v", p4, err)
	}
	p6, err := natcommon.Prefix6("64:ff9b::1/96")
	if err != nil || natcommon.Prefix6String(p6) != "64:ff9b::/96" {
		t.Fatalf("%v %v", p6, err)
	}
	pu, err := natcommon.Prefix("10.9.0.0/16")
	if err != nil || natcommon.PrefixString(pu) != "10.9.0.0/16" {
		t.Fatalf("%v %v", pu, err)
	}
	u, err := natcommon.Addr("fd00:9::2")
	if err != nil || natcommon.AddrString(u) != "fd00:9::2" {
		t.Fatal(err)
	}
	if natcommon.CanonProto("6") != "tcp" || natcommon.CanonProto("udp") != "udp" || natcommon.CanonProto("") != "any" || natcommon.CanonProto("junk") != "junk" {
		t.Fatal("proto canon")
	}
	if n, _ := natcommon.ProtoNumber("icmp"); n != 1 {
		t.Fatal("proto number")
	}
}

func TestErrorClassification(t *testing.T) {
	if !natcommon.IsAlreadyEnabled(api.RetvalToVPPApiError(int32(api.FEATURE_ALREADY_ENABLED))) || !natcommon.IsAlreadyEnabled(api.VPPApiError(1)) || natcommon.IsAlreadyEnabled(api.VPPApiError(-1)) {
		t.Fatal("already enabled")
	}
	if !natcommon.IsAlreadyDisabled(api.VPPApiError(-169)) || !natcommon.IsNoSuchEntry(api.VPPApiError(-6)) || !natcommon.IsValueExist(api.VPPApiError(-81)) {
		t.Fatal("classification")
	}
	if !natcommon.IsUnknownMessage(&api.CompatibilityError{IncompatibleMessages: []string{"x"}}) || !natcommon.IsUnknownMessage(errors.New("unknown message: npt66_binding_add_del")) || natcommon.IsUnknownMessage(nil) {
		t.Fatal("unknown message")
	}
	if !errors.Is(fmt.Errorf("x: retrieve: %w", natcommon.ErrRetrieveUnsupported), natcommon.ErrRetrieveUnsupported) {
		t.Fatal("ErrRetrieveUnsupported must survive wrapping")
	}
	if !natcommon.PluginLoaded(struct{}{}) {
		t.Fatal("a client without CompatChecker counts as loaded")
	}
}

func TestInterfaceTable(t *testing.T) {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.On("sw_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceDump)
		all := []api.Message{
			&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
			&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop900", Tag: "w9:loop900\x00\x00"},
			&interfaces.SwInterfaceDetails{SwIfIndex: 4, InterfaceName: "loop9000", Tag: "w3:x"},
		}
		if !r.NameFilterValid {
			return all, nil
		}
		var out []api.Message
		for _, m := range all {
			if d := m.(*interfaces.SwInterfaceDetails); len(d.InterfaceName) >= len(r.NameFilter) && d.InterfaceName[:len(r.NameFilter)] == r.NameFilter {
				out = append(out, d)
			}
		}
		return out, nil
	})
	ctx := context.Background()
	tbl, err := natcommon.DumpInterfaces(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if i, ok := tbl.ByName("loop900"); !ok || i.SwIfIndex != 3 || i.Tag != "w9:loop900" {
		t.Fatalf("by name %+v %v", i, ok)
	}
	if tbl.Name(4) != "loop9000" || tbl.Name(77) != "sw_if_index:77" {
		t.Fatal("name lookup")
	}
	idx, err := natcommon.ResolveInterface(ctx, f, "loop900") // substring filter also matches loop9000
	if err != nil || idx != 3 {
		t.Fatalf("resolve: %d %v", idx, err)
	}
	if _, err := natcommon.ResolveInterface(ctx, f, "loop901"); !errors.Is(err, natcommon.ErrNoSuchInterface) {
		t.Fatalf("missing interface: %v", err)
	}
}

// A minimal descriptor through the generic adapter: verifies routing, key building,
// ErrRecreate on nil Update / id change, Retrieve encoding.
func TestGenericDescriptor(t *testing.T) {
	store := map[string]spec{}
	d := natcommon.New(natcommon.Ops[spec]{
		Name: "test.thing",
		ID:   func(s spec) string { return s.Addr },
		Deps: func(s spec) []scheduler.Dependency { return natcommon.WithVRF(nil, s.VRF) },
		Create: func(_ context.Context, s spec) (any, error) {
			store[s.Addr] = s
			return "meta:" + s.Addr, nil
		},
		Update: func(_ context.Context, _, n spec, meta any) (any, error) {
			store[n.Addr] = n
			return meta, nil
		},
		Delete: func(_ context.Context, s spec, _ any) error {
			delete(store, s.Addr)
			return nil
		},
		Retrieve: func(context.Context) ([]natcommon.Item[spec], error) {
			var out []natcommon.Item[spec]
			for _, s := range store {
				out = append(out, natcommon.Item[spec]{Spec: s, Meta: "meta:" + s.Addr})
			}
			return out, nil
		},
	})
	var _ scheduler.Descriptor = d
	ctx := context.Background()
	obj := natcommon.MustEncode(&spec{Addr: "10.9.0.1", VRF: 9001, List: []string{}})
	if d.KeyOf(obj) != "test.thing/10.9.0.1" {
		t.Fatalf("key %s", d.KeyOf(obj))
	}
	if deps := d.Dependencies(obj); len(deps) != 1 || deps[0].Key != "vrf/9001" {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, obj)
	if err != nil || meta != "meta:10.9.0.1" {
		t.Fatal(err)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Key != "test.thing/10.9.0.1" || !proto.Equal(kvs[0].Value, obj) || kvs[0].Meta != meta {
		t.Fatalf("retrieve %+v %v", kvs, err)
	}
	if _, err := d.Update(ctx, obj, natcommon.MustEncode(&spec{Addr: "10.9.0.2"}), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("id change must recreate: %v", err)
	}
	if _, err := d.Update(ctx, obj, natcommon.MustEncode(&spec{Addr: "10.9.0.1", On: true, List: []string{}}), meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, obj, meta); err != nil || len(store) != 0 {
		t.Fatal(err)
	}
	bad := &structpb.Value{}
	if _, err := d.Create(ctx, bad); err == nil {
		t.Fatal("undecodable object must fail")
	}
	if k := d.KeyOf(bad); k.Descriptor() != "test.thing" || k.ID() == "" {
		t.Fatalf("invalid key %q", k)
	}
	noUpdate := natcommon.New(natcommon.Ops[spec]{Name: "test.other", ID: func(s spec) string { return s.Addr },
		Create:   func(context.Context, spec) (any, error) { return nil, nil },
		Delete:   func(context.Context, spec, any) error { return nil },
		Retrieve: func(context.Context) ([]natcommon.Item[spec], error) { return nil, nil }})
	if _, err := noUpdate.Update(ctx, obj, obj, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal("nil Update must recreate")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("invalid name must panic")
			}
		}()
		natcommon.New(natcommon.Ops[spec]{Name: "Bad/Name"})
	}()
}
