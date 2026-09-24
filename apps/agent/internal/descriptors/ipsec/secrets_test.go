package ipsec_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
)

// TestPastedPlaintextKey: a plaintext key where a reference belongs is refused before VPP is
// called and never echoed (review M1).
func TestPastedPlaintextKey(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewSa(newCfg(v))
	s := transportSA()
	s.CryptoKey = "Summer2026!Summer2026!"
	_, err := d.Create(ctx, s)
	if err == nil || strings.Contains(err.Error(), "Summer") {
		t.Fatalf("pasted plaintext: %v", err)
	}
	if len(v.CallsNamed("ipsec_sad_entry_add_v2")) != 0 {
		t.Fatal("VPP must not be called")
	}
}

// TestLegacyReferenceMigration (D-096): a desired SA that still carries a pre-D-096 unkeyed
// "sha256:" reference is applied (the material verifies), Retrieve reports the keyed reference,
// so the object differs from the old desired value exactly once — the scheduler re-applies it
// (ErrRecreate) and it converges as soon as the desired state carries the keyed reference.
func TestLegacyReferenceMigration(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	sum := sha256.Sum256(cryptoKey)
	legacy := "sha256:" + hex.EncodeToString(sum[:])
	secrets.Put(legacy, cryptoKey)
	d := ipsecd.NewSa(cfg)
	old := transportSA()
	old.CryptoKey = legacy
	if _, err := d.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("Retrieve %v %v", kvs, err)
	}
	got := kvs[0].Value.(*vpnpb.IpsecSa)
	if proto.Equal(got, old) || got.GetCryptoKey() != keys.Ref(cryptoKey) {
		t.Fatalf("retrieved %s, want the keyed reference (mismatch → one re-apply)", got.GetCryptoKey())
	}
	if !proto.Equal(got, transportSA()) {
		t.Fatal("the keyed desired value must equal what is retrieved")
	}
}
