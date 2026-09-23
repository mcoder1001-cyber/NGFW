package df7_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

type spec struct {
	Name  string   `json:"name,omitempty"`
	N     uint32   `json:"n,omitempty"`
	List  []string `json:"list,omitempty"`
	Inner *spec    `json:"inner,omitempty"`
}

func (spec) Validate() error { return nil }

func TestCodec(t *testing.T) {
	a := spec{Name: "x", N: 7, List: []string{"a"}, Inner: &spec{N: 1}}
	got, err := df7.Decode[spec](df7.Encode(a))
	if err != nil || got.Name != "x" || got.N != 7 || got.Inner.N != 1 {
		t.Fatal(got, err)
	}
	// zero values and nil/empty slices encode alike: one canonical form
	if !proto.Equal(df7.Encode(spec{List: nil}), df7.Encode(spec{List: []string{}})) {
		t.Fatal("nil and empty slices must encode alike")
	}
	if _, err := df7.Decode[spec](wrapperspb.String("x")); !errors.Is(err, df7.ErrSpec) {
		t.Fatal(err)
	}
	bad := df7.Encode(map[string]any{"name": "x", "unknown": 1})
	if _, err := df7.Decode[spec](bad); !errors.Is(err, df7.ErrSpec) {
		t.Fatal("unknown fields must be rejected", err)
	}
	if _, err := df7.DecodeValid[spec](df7.Encode(a)); err != nil {
		t.Fatal(err)
	}
}

func TestIDRangeAndOptions(t *testing.T) {
	var all *df7.IDRange
	if !all.Owns(123456) {
		t.Fatal("nil range owns everything")
	}
	o := df7.BuildOptions([]df7.Option{df7.WithIDRange(10, 20), nil})
	if o.IDs.Owns(9) || !o.IDs.Owns(20) || o.CheckID("x", 21) == nil || o.CheckID("x", 10) != nil {
		t.Fatal("range")
	}
	if o.IfaceDep("loop0").Key != "interface/loop0" || df7.VRFKey(5) != "vrf/5" || df7.ClassifyTableKey("t") != "classify.table/t" ||
		df7.InterfaceIPKey("loop0", "10.0.0.1/24") != "interface-ip/loop0/10.0.0.1/24" {
		t.Fatal("keys")
	}
	o = df7.BuildOptions([]df7.Option{df7.WithInterfaceKey(func(n string) scheduler.Key { return scheduler.Key("x/" + n) })})
	if o.IfaceDep("a").Key != "x/a" {
		t.Fatal("interface key override")
	}
}

func TestAddr(t *testing.T) {
	s, err := df7.SortedAddrs([]string{"10.0.0.2", "10.0.0.10", "2001:db8::1"})
	if err != nil || s[0] != "10.0.0.2" || s[1] != "10.0.0.10" {
		t.Fatal(s, err)
	}
	if _, err := df7.SortedAddrs([]string{"10.0.0.1", "10.0.0.1"}); !errors.Is(err, df7.ErrSpec) {
		t.Fatal("duplicate")
	}
	if _, err := df7.ParsePrefix("10.0.0.1/24"); !errors.Is(err, df7.ErrSpec) {
		t.Fatal("host bits")
	}
	p, _ := df7.ParseIfPrefix("10.0.0.1/24")
	if df7.FromPrefix(df7.ToPrefix(p)) != p || df7.FromAddressWithPrefix(df7.ToAddressWithPrefix(p)) != p {
		t.Fatal("prefix round trip")
	}
	a, _ := df7.ParseAddr("2001:db8::1")
	if df7.FromAddress(df7.ToAddress(a)) != a {
		t.Fatal("address round trip")
	}
}

func TestFibPaths(t *testing.T) {
	f := df7test.NewFake()
	ifs, err := df7.DumpInterfaces(t.Context(), f, df7test.Owner, df7.Options{})
	if err != nil {
		t.Fatal(err)
	}
	in := []df7.Path{
		{Interface: "loop0", NextHop: "10.0.0.2", TableID: 9, Labels: []df7.Label{{Label: 100, TTL: 64}}},
		{NextHop: "2001:db8::2", TableID: 5, Weight: 3},
		{Type: df7.PathDrop},
	}
	norm, err := df7.NormalizePaths(in)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := df7.EncodePaths(norm, ifs)
	if err != nil {
		t.Fatal(err)
	}
	dec := df7.DecodePaths(enc, ifs)
	if !proto.Equal(df7.Encode(map[string]any{"p": toAny(norm)}), df7.Encode(map[string]any{"p": toAny(dec)})) {
		t.Fatalf("round trip\n%+v\n%+v", norm, dec)
	}
	for _, p := range norm {
		if p.Weight == 0 || (p.NextHop == "2001:db8::2" && (p.Proto != df7.ProtoIP6 || p.TableID != 5)) || (p.Interface == "loop0" && p.TableID != 0) {
			t.Fatalf("not canonical: %+v", norm)
		}
	}
	for i, bad := range [][]df7.Path{
		{{Type: "blackhole"}},
		{{NextHop: "10.0.0.1", Proto: df7.ProtoIP6}},
		{{Labels: []df7.Label{{Label: 1 << 21}}}},
		{{Labels: []df7.Label{{Label: 16, Exp: 8}}}},
	} {
		if _, err := df7.NormalizePaths(bad); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := df7.EncodePaths([]df7.Path{{Interface: "loop9", Proto: df7.ProtoIP4, Weight: 1}}, ifs); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal("paths never reference another owner's interface", err)
	}
	unknown := df7.DecodePaths([]fib_types.FibPath{{SwIfIndex: 77, Type: 99, Proto: 9}}, ifs)
	if unknown[0].Interface != "#77" || unknown[0].Type != "#99" || unknown[0].Proto != "#9" {
		t.Fatalf("%+v", unknown)
	}
	if got := df7.PathInterfaces(append(norm, norm[0])); len(got) != 1 || got[0] != "loop0" {
		t.Fatal(got)
	}
}

func toAny(ps []df7.Path) []any {
	out := make([]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, df7.Encode(p).AsMap())
	}
	return out
}

func TestInterfacesAndClaims(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	ifs, err := df7.DumpInterfaces(ctx, f, df7test.Owner, df7.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ifs.Resolve("loop9"); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal(err)
	}
	if _, err := ifs.Resolve("local0"); !errors.Is(err, df7.ErrNoSuchInterface) {
		t.Fatal(err)
	}
	holder := func(n string) string { return "x/" + n }
	if _, ok := ifs.Owned(4, holder); ok {
		t.Fatal("untagged without a claim is not ours")
	}
	if _, err := ifs.Attach("eth0", "x/eth0"); err != nil {
		t.Fatal(err)
	}
	if n, ok := ifs.Owned(4, holder); !ok || n != "eth0" {
		t.Fatal("claimed untagged interface")
	}
	if _, found, err := ifs.Reresolve("eth0", "y/eth0"); !errors.Is(err, df7.ErrForeignInterface) || found {
		t.Fatal("a claim is per object key", err)
	}
	if _, found, err := ifs.Reresolve("gone", "x/gone"); err != nil || found {
		t.Fatal(found, err)
	}
	if err := df7.Release(df7test.Owner, "eth0", "x/eth0"); err != nil || iface.Claims(df7test.Owner).Claimed("eth0", "x/eth0") {
		t.Fatal("release")
	}
	if ifs.Name(77) != "#77" || ifs.Name(1) != "loop0" {
		t.Fatal("names")
	}
	if n, ok := ifs.OwnedTagged(1); !ok || n != "loop0" {
		t.Fatal("tagged")
	}
}

func TestApplyOnce(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	b := df7.NewBase("x.y", f, "w-apply", nil)
	n := 0
	apply := func() error { n++; return nil }
	for i := 0; i < 3; i++ {
		if _, err := b.ApplyOnce(ctx, "k", apply); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("applied %d times in one VPP lifetime", n)
	}
	if on, _ := b.AppliedNow(ctx, "k"); !on {
		t.Fatal("applied now")
	}
	f.Reboot()
	if on, _ := b.AppliedNow(ctx, "k"); on {
		t.Fatal("a VPP restart forgets the application")
	}
	if skipped, _ := b.ApplyOnce(ctx, "k", apply); skipped || n != 2 {
		t.Fatal("re-applied once after a restart")
	}
	if err := b.ForgetApplied("k"); err != nil {
		t.Fatal(err)
	}
	df7.SetAppliedStore("w-apply", nil)
	if _, ok := df7.AppliedFor("w-apply").Applied("k"); ok {
		t.Fatal("fresh store")
	}
	failing := func() error { return errors.New("vpp said no") }
	if _, err := b.ApplyOnce(ctx, "k2", failing); err == nil {
		t.Fatal("error must surface")
	}
	if _, ok := df7.AppliedFor("w-apply").Applied("k2"); ok {
		t.Fatal("a failed apply is not recorded")
	}
}

func TestErrors(t *testing.T) {
	if !errors.Is(df7.Unsupported("a.b", "why"), df7.ErrRetrieveUnsupported) || df7.ErrRetrieveUnsupported.Error() != "vpp has no dump for this object type" {
		t.Fatal("ErrRetrieveUnsupported text must match the scheduler's (P05) sentinel")
	}
	if !errors.Is(df7.BadMeta("a", 1), df7.ErrBadMeta) || df7.PluginError("x", nil) != nil {
		t.Fatal("errors")
	}
}
