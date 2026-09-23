package vpn_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp/fake"
)

// test vector: 32 bytes of the documented placeholder, never real material
var testKey = []byte("VRX_TEST_PSK_DF5_0123456789abcde")

func TestSecretReferences(t *testing.T) {
	ref := vpn.Ref(testKey)
	if !strings.HasPrefix(ref, "sha256:") || len(ref) != len("sha256:")+64 {
		t.Fatalf("Ref = %q", ref)
	}
	if err := vpn.Verify(ref, testKey); err != nil {
		t.Fatal(err)
	}
	if err := vpn.Verify(ref, []byte("other")); !errors.Is(err, vpn.ErrSecretMismatch) {
		t.Fatalf("Verify wrong material: %v", err)
	}
	if err := vpn.Verify("plain:abc", testKey); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("Verify bad ref: %v", err)
	}

	xref, err := vpn.X25519Ref(testKey)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := vpn.PublicKeyOfRef(xref)
	if err != nil || len(pub) != 32 || vpn.X25519RefFromPublic(pub) != xref {
		t.Fatalf("x25519 ref round trip: %q %v", xref, err)
	}
	if err := vpn.Verify(xref, testKey); err != nil {
		t.Fatal(err)
	}
	if _, err := vpn.X25519Ref([]byte("short")); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("short private key: %v", err)
	}
	if _, err := vpn.PublicKeyOfRef(ref); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("sha256 ref as x25519: %v", err)
	}
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	r := vpn.NewMapResolver(testKey)
	ref := vpn.Ref(testKey)
	mat, err := vpn.Resolve(ctx, r, ref)
	if err != nil || string(mat) != string(testKey) {
		t.Fatalf("Resolve: %v", err)
	}
	vpn.Zero(mat)
	if string(mat) == string(testKey) {
		t.Fatal("Zero did not clear the buffer")
	}
	if got, _ := r.Resolve(ctx, ref); string(got) != string(testKey) {
		t.Fatal("Zero must not reach the resolver's copy")
	}
	if _, err := vpn.Resolve(ctx, r, "sha256:00"); !errors.Is(err, vpn.ErrSecretNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := vpn.Resolve(ctx, nil, ref); !errors.Is(err, vpn.ErrNoResolver) {
		t.Fatalf("nil resolver: %v", err)
	}
	if mat, err := vpn.Resolve(ctx, nil, ""); err != nil || mat != nil {
		t.Fatalf("empty ref must resolve to no secret: %v %v", mat, err)
	}
	// a resolver that lies about the material is caught
	bad := vpn.MapResolver{ref: []byte("not the key")}
	if _, err := vpn.Resolve(ctx, bad, ref); !errors.Is(err, vpn.ErrSecretMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	// error strings never contain material and keep references short
	if s := fmt.Sprint(err); strings.Contains(s, string(testKey)) || strings.Contains(s, ref[7:]) {
		t.Fatalf("error leaks: %s", s)
	}
	if r := vpn.Redact(ref); len(r) > 20 || !strings.HasPrefix(r, "sha256:") {
		t.Fatalf("Redact = %q", r)
	}
}

func TestAddresses(t *testing.T) {
	for in, want := range map[string]string{"10.4.0.1": "10.4.0.1", "::ffff:10.4.0.2": "10.4.0.2", "2001:DB8::1": "2001:db8::1"} {
		a, err := vpn.ParseAddress(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := vpn.AddressString(a); got != want {
			t.Fatalf("%s → %s, want %s", in, got, want)
		}
		if c, _ := vpn.CanonicalAddress(in); c != want {
			t.Fatalf("CanonicalAddress(%s) = %s", in, c)
		}
	}
	if _, err := vpn.ParseAddress("10.4.0"); err == nil {
		t.Fatal("bad address accepted")
	}
	z, _ := vpn.ParseAddress("0.0.0.0")
	if !vpn.IsUnspecified(z) {
		t.Fatal("0.0.0.0 must be unspecified")
	}
	p, err := vpn.ParsePrefix("10.4.1.0/24")
	if err != nil || vpn.PrefixString(p) != "10.4.1.0/24" || p.Len != 24 {
		t.Fatalf("prefix: %v %v", p, err)
	}
	if _, err := vpn.ParsePrefix("10.4.1.7/24"); err == nil {
		t.Fatal("host bits accepted")
	}
	if c, _ := vpn.CanonicalPrefix("2001:db8:0:0::/64"); c != "2001:db8::/64" {
		t.Fatalf("CanonicalPrefix = %s", c)
	}
}

func TestKeysAndRanges(t *testing.T) {
	if k := vpn.InterfaceKey("loop400"); k != "interface/loop400" || k.Descriptor() != "interface" {
		t.Fatalf("InterfaceKey = %s", k)
	}
	for name, want := range map[string]string{
		"loop400": "interface/loop400", "ipip4001": "interface/ipip4001", "ipsec4001": "ipsec.itf/ipsec4001",
		"wg4001": "wireguard.interface/wg4001", "ipsec": "interface/ipsec", "wgx": "interface/wgx",
	} {
		if k := vpn.InterfaceDependency(name); string(k) != want {
			t.Fatalf("InterfaceDependency(%s) = %s, want %s", name, k, want)
		}
	}
	if k := vpn.VRFKey(4001); k != "vrf/4001" {
		t.Fatalf("VRFKey = %s", k)
	}
	var all vpn.IDRange
	if !all.Contains(0) || !all.Contains(^uint32(0)) || all.Check("spd", 7) != nil {
		t.Fatal("zero range must own everything")
	}
	r := vpn.IDRange{Lo: 4000, Hi: 4999}
	if !r.Contains(4000) || !r.Contains(4999) || r.Contains(3999) || r.Contains(5000) {
		t.Fatal("range bounds")
	}
	if err := r.Check("sa", 1); err == nil || !strings.Contains(err.Error(), "sa id 1") {
		t.Fatalf("Check: %v", err)
	}
}

func TestInterfaces(t *testing.T) {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop400", Tag: "w4:loop400\x00\x00"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 6, InterfaceName: "loop300", Tag: "w3:loop300"},
	)
	f.Reply("sw_interface_tag_add_del", &interfaces.SwInterfaceTagAddDelReply{})
	ctx := context.Background()
	tbl, err := vpn.DumpInterfaces(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if idx, err := tbl.Index("loop400"); err != nil || idx != 5 {
		t.Fatalf("Index = %d %v", idx, err)
	}
	if _, err := tbl.Index("nope"); err == nil {
		t.Fatal("missing interface must error")
	}
	if id, ok := tbl.Owned(5, "w4"); !ok || id != "loop400" {
		t.Fatalf("Owned = %q %v", id, ok)
	}
	if _, ok := tbl.Owned(6, "w4"); ok {
		t.Fatal("another owner's interface reported as owned")
	}
	if _, ok := tbl.Owned(0, "w4"); ok {
		t.Fatal("local0 reported as owned")
	}
	if tbl.Name(6) != "loop300" || tbl.Name(99) != "" {
		t.Fatal("Name")
	}
	if err := vpn.TagInterface(ctx, f, 5, "w4", "x"); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("sw_interface_tag_add_del")
	if len(calls) != 1 || calls[0].(*interfaces.SwInterfaceTagAddDel).Tag != "w4:x" {
		t.Fatalf("tag call: %+v", calls)
	}
	f.Fail("sw_interface_dump", errors.New("boom"))
	if _, err := vpn.DumpInterfaces(ctx, f); err == nil {
		t.Fatal("dump error must surface")
	}
	var _ api.Message = &interfaces.SwInterfaceDump{}
}
