package vpn_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// testKeys is the fixed fingerprint key of the unit tests (a test vector, not an agent key).
var testKeys, _ = vpn.NewKeyer([]byte("VRX_TEST_PSK_DF5_fingerprint_key"))

func TestSecretReferences(t *testing.T) {
	ref := testKeys.Ref(testKey)
	if !strings.HasPrefix(ref, "hmac:") || len(ref) != len("hmac:")+64 {
		t.Fatalf("Ref = %q", ref)
	}
	// D-096: keyed, never the plain sha256 of the secret; another key gives another reference
	plain := sha256.Sum256(testKey)
	if strings.Contains(ref, hex.EncodeToString(plain[:])) {
		t.Fatal("reference is the unkeyed sha256 of the material")
	}
	other, _ := vpn.NewKeyer(bytes.Repeat([]byte{9}, 32))
	if other.Ref(testKey) == ref {
		t.Fatal("two keys must produce different references")
	}
	if _, err := vpn.NewKeyer([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
	if err := vpn.Verify(testKeys, ref, testKey); err != nil {
		t.Fatal(err)
	}
	if err := vpn.Verify(testKeys, ref, []byte("other")); !errors.Is(err, vpn.ErrSecretMismatch) {
		t.Fatalf("Verify wrong material: %v", err)
	}
	if err := vpn.Verify(nil, ref, testKey); !errors.Is(err, vpn.ErrNoKeyer) {
		t.Fatalf("Verify without keyer: %v", err)
	}
	if err := vpn.Verify(testKeys, "plain:abc", testKey); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("Verify bad ref: %v", err)
	}
	// legacy (pre-D-096) references still verify, so an old desired state applies once
	if err := vpn.Verify(testKeys, "sha256:"+hex.EncodeToString(plain[:]), testKey); err != nil {
		t.Fatalf("legacy ref: %v", err)
	}

	xref, err := vpn.X25519Ref(testKey)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := vpn.PublicKeyOfRef(xref)
	if err != nil || len(pub) != 32 {
		t.Fatalf("PublicKeyOfRef: %v", err)
	}
	if err := vpn.Verify(nil, xref, testKey); err != nil {
		t.Fatal(err)
	}
	if _, err := vpn.X25519Ref([]byte("short")); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("short x25519 key: %v", err)
	}
	if _, err := vpn.PublicKeyOfRef(ref); !errors.Is(err, vpn.ErrBadRef) {
		t.Fatalf("PublicKeyOfRef(hmac): %v", err)
	}
	if s := fmt.Sprintf("%v %+v %#v", testKeys, *testKeys, testKeys); strings.Contains(s, "VRX_TEST_PSK") || strings.Contains(s, "fingerprint_key") {
		t.Fatalf("keyer formatting leaks the key: %s", s)
	}
}

func TestKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "fingerprint.key")
	k1, err := vpn.LoadOrCreateKeyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() != 32 {
		t.Fatalf("key file %v %v", info, err)
	}
	k2, err := vpn.LoadOrCreateKeyFile(path) // agent restart: same key, same references
	if err != nil || k2.Ref(testKey) != k1.Ref(testKey) {
		t.Fatalf("reload: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := vpn.LoadOrCreateKeyFile(path); err == nil {
		t.Fatal("a group/world-readable key file must be refused")
	}
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	r := vpn.NewMapResolver(testKeys, testKey)
	ref := testKeys.Ref(testKey)
	mat, err := vpn.Resolve(ctx, r, testKeys, ref)
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
	if _, err := vpn.Resolve(ctx, r, testKeys, "hmac:"+strings.Repeat("0", 64)); !errors.Is(err, vpn.ErrSecretNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := vpn.Resolve(ctx, nil, testKeys, ref); !errors.Is(err, vpn.ErrNoResolver) {
		t.Fatalf("nil resolver: %v", err)
	}
	if mat, err := vpn.Resolve(ctx, nil, testKeys, ""); err != nil || mat != nil {
		t.Fatalf("empty ref must resolve to no secret: %v %v", mat, err)
	}
	// a resolver that lies about the material is caught
	bad := vpn.NewMapResolver(testKeys)
	bad.Put(ref, []byte("not the key"))
	if _, err := vpn.Resolve(ctx, bad, testKeys, ref); !errors.Is(err, vpn.ErrSecretMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	// error strings never contain material and keep references short
	if s := fmt.Sprint(err); strings.Contains(s, string(testKey)) || strings.Contains(s, ref[14:]) {
		t.Fatalf("error leaks: %s", s)
	}
	if r := vpn.Redact(ref); len(r) > 20 || !strings.HasPrefix(r, "hmac:") {
		t.Fatalf("Redact = %q", r)
	}
}

// TestPastedPlaintextNeverEchoed: a plaintext secret put where a reference belongs (review M1)
// never appears in an error or in Redact's output.
func TestPastedPlaintextNeverEchoed(t *testing.T) {
	ctx := context.Background()
	r := vpn.NewMapResolver(testKeys, testKey)
	for _, pasted := range []string{"Summer2026!", "hmac:Summer2026!", "sha256:Summer2026!xyz", "x25519:Summer2026!",
		"sha256:" + strings.Repeat("a", 64)} {
		if got := vpn.Redact(pasted); strings.Contains(got, "Summer") || strings.Contains(got, "aaaaaaaa") {
			t.Fatalf("Redact(%q) = %q", pasted, got)
		}
		_, err := vpn.Resolve(ctx, r, testKeys, pasted)
		if err == nil {
			t.Fatalf("%q resolved", pasted)
		}
		if strings.Contains(err.Error(), "Summer") || strings.Contains(err.Error(), "aaaaaaaa") {
			t.Fatalf("error echoes the value: %v", err)
		}
		if _, err := vpn.PublicKeyOfRef(pasted); err == nil || strings.Contains(err.Error(), "Summer") {
			t.Fatalf("PublicKeyOfRef: %v", err)
		}
	}
	if vpn.Redact("psk/site-a") != "psk/site-a" {
		t.Fatal("a D-051 name is not secret")
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
