package ipsec_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
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

// TestLegacyReferenceRefused (D-096, fix round 2 N5): an unkeyed "sha256:" reference is refused
// before VPP is called — no plain-SHA-256 input path remains.
func TestLegacyReferenceRefused(t *testing.T) {
	v := newFakeVPP()
	sum := sha256.Sum256(cryptoKey)
	old := transportSA()
	old.CryptoKey = "sha256:" + hex.EncodeToString(sum[:])
	_, err := ipsecd.NewSa(newCfg(v)).Create(ctx, old)
	if !errors.Is(err, vpn.ErrBadRef) || strings.Contains(err.Error(), hex.EncodeToString(sum[:])[:16]) {
		t.Fatalf("legacy ref: %v", err)
	}
	if len(v.CallsNamed("ipsec_sad_entry_add_v2")) != 0 {
		t.Fatal("VPP must not be called")
	}
}
