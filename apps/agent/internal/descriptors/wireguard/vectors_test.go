package wireguard_test

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	"ngfw/agent/internal/descriptors/vpn"
)

// Test vectors (00-CONTEXT: fixtures use the literal VRX_TEST_PSK_<id>): every key is the SHA-256
// of a "VRX_TEST_PSK_DF5_wg_…" label, so it is reproducible from this file and is not real
// material. The slot is part of the label because VPP requires peer public keys to be unique
// VPP-wide and several workers share the host VPP.
type vectors struct {
	itfPriv  []byte
	psk      []byte
	pskRef   string
	peerPub  []string
	resolver *vpn.MapResolver
}

func vector(label string) []byte {
	s := sha256.Sum256([]byte("VRX_TEST_PSK_DF5_wg_" + label))
	return s[:]
}

func slotVectors(t testing.TB, slot int) (vectors, string) {
	t.Helper()
	v := vectors{itfPriv: vector(fmt.Sprintf("itf_%d", slot)), psk: vector(fmt.Sprintf("psk_%d", slot)), resolver: vpn.NewMapResolver(keys)}
	for i := range 2 {
		priv, err := ecdh.X25519().NewPrivateKey(vector(fmt.Sprintf("peer_%d_%d", slot, i)))
		if err != nil {
			t.Fatal(err)
		}
		v.peerPub = append(v.peerPub, base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()))
	}
	itfRef, err := v.resolver.AddX25519(v.itfPriv)
	if err != nil {
		t.Fatal(err)
	}
	v.pskRef = v.resolver.Add(v.psk)
	return v, itfRef
}
