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
	bad := vpn.NewMapResolver()
	bad.Put(ref, []byte("not the key"))
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
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "ipsec400", Tag: "w4:w4-tun0\x00\x00"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 6, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "wan0"},
	)
	f.Reply("sw_interface_tag_add_del", &interfaces.SwInterfaceTagAddDelReply{})
	ctx := context.Background()
	tbl, err := vpn.DumpInterfaces(ctx, f, "w4")
	if err != nil {
		t.Fatal(err)
	}
	// D-069: our interfaces by their logical (tag id) name, untagged ones by VPP's name
	if idx, err := tbl.Resolve("w4-tun0"); err != nil || idx != 5 {
		t.Fatalf("Resolve(own) = %d %v", idx, err)
	}
	if idx, err := tbl.Resolve("wan0"); err != nil || idx != 7 {
		t.Fatalf("Resolve(untagged) = %d %v", idx, err)
	}
	if _, err := tbl.Resolve("ipsec400"); !errors.Is(err, vpn.ErrNoInterface) {
		t.Fatalf("VPP name of our interface must not resolve: %v", err)
	}
	if _, err := tbl.Resolve("loop300"); !errors.Is(err, vpn.ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
	if _, err := tbl.Resolve("local0"); !errors.Is(err, vpn.ErrNoInterface) {
		t.Fatalf("local0: %v", err)
	}
	if _, err := tbl.ResolveOwn("wan0"); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("ResolveOwn(untagged): %v", err)
	}
	if idx, err := tbl.ResolveOwn("w4-tun0"); err != nil || idx != 5 {
		t.Fatalf("ResolveOwn = %d %v", idx, err)
	}
	if id, ok := tbl.Owned(5); !ok || id != "w4-tun0" {
		t.Fatalf("Owned = %q %v", id, ok)
	}
	if _, ok := tbl.Owned(6); ok {
		t.Fatal("another owner's interface reported as owned")
	}
	if tbl.Logical(6) != "" || tbl.Logical(7) != "wan0" || tbl.Logical(5) != "w4-tun0" || tbl.Logical(0) != "" || tbl.Logical(99) != "" {
		t.Fatal("Logical")
	}
	if !tbl.Untagged(7) || tbl.Untagged(5) || tbl.Untagged(0) {
		t.Fatal("Untagged")
	}
	// delete-by-index re-verification
	if ok, err := vpn.OwnedAt(ctx, f, "w4", 5, "w4-tun0"); !ok || err != nil {
		t.Fatalf("OwnedAt(own) = %v %v", ok, err)
	}
	if ok, err := vpn.OwnedAt(ctx, f, "w4", 99, "w4-tun0"); ok || err != nil {
		t.Fatalf("OwnedAt(gone) = %v %v", ok, err)
	}
	if _, err := vpn.OwnedAt(ctx, f, "w4", 6, "w4-tun0"); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("OwnedAt(reused index) = %v", err)
	}
	if err := vpn.TagInterface(ctx, f, 5, "w4", "x"); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("sw_interface_tag_add_del")
	if len(calls) != 1 || calls[0].(*interfaces.SwInterfaceTagAddDel).Tag != "w4:x" {
		t.Fatalf("tag call: %+v", calls)
	}
	f.Fail("sw_interface_dump", errors.New("boom"))
	if _, err := vpn.DumpInterfaces(ctx, f, "w4"); err == nil {
		t.Fatal("dump error must surface")
	}
	var _ api.Message = &interfaces.SwInterfaceDump{}
}
