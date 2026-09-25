package subsystems

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
	"ngfw/agent/internal/descriptors/wireguard"
)

func TestWireguardSecrets(t *testing.T) {
	s := NewWireguardSecrets(vpntest.Keys)
	priv := sha256.Sum256([]byte("VRX_TEST_PSK_F-wireguard_unit_itf"))
	psk := sha256.Sum256([]byte("VRX_TEST_PSK_F-wireguard_unit_psk"))
	if err := s.Put("key/a", priv[:]); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("psk/b", psk[:]); err != nil {
		t.Fatal(err)
	}
	xr, err := s.Ref("key/a")
	if want, _ := vpn.X25519Ref(priv[:]); err != nil || xr != want {
		t.Fatalf("Ref(key/a) = %s, %v", xr, err)
	}
	hr, err := s.Ref("psk/b")
	if err != nil || hr != vpntest.Keys.Ref(psk[:]) {
		t.Fatalf("Ref(psk/b) = %s, %v", hr, err)
	}
	for ref, want := range map[string][]byte{xr: priv[:], hr: psk[:]} {
		m, err := vpn.Resolve(context.Background(), s, vpntest.Keys, ref) // verifies material ↔ reference
		if err != nil || string(m) != string(want) {
			t.Fatalf("Resolve(%s): %v", vpn.Redact(ref), err)
		}
	}
	if _, err := s.Ref("key/missing"); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.Put("password/x", psk[:]); err == nil {
		t.Fatal("only key/ and psk/ references")
	}
	if err := s.Put("psk/short", psk[:5]); err == nil {
		t.Fatal("32 bytes only")
	}
	// replacing a reference drops the old material
	other := sha256.Sum256([]byte("VRX_TEST_PSK_F-wireguard_unit_psk2"))
	if err := s.Put("psk/b", other[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(context.Background(), hr); err == nil {
		t.Fatal("old material still resolvable")
	}
	// never printed
	var b strings.Builder
	slog.New(slog.NewTextHandler(&b, nil)).Info("x", "s", s)
	out := fmt.Sprintf("%v %+v %#v %s", s, s, s, b.String())
	for _, m := range [][]byte{priv[:], psk[:], other[:]} {
		if strings.Contains(out, string(m)) || strings.Contains(out, fmt.Sprintf("%x", m)) || strings.Contains(out, decimal(m)) {
			t.Fatalf("material printed: %s", out)
		}
	}
	if !strings.Contains(out, "subsystems.WireguardSecrets(2 secrets)") {
		t.Fatalf("format %s", out)
	}
}

func TestWireguardEventAndRequirementDeclaration(t *testing.T) {
	ev := WireguardEvent(wireguard.PeerEvent{Interface: "wg7001", PublicKey: "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=", PeerIndex: 3, Dead: true})
	if ev.GetInterface() != "wg7001" || ev.GetAttributes()["dead"] != "true" || ev.GetAttributes()["peer_index"] != "3" || !strings.HasSuffix(ev.GetMessage(), ": dead") {
		t.Fatalf("event %v", ev)
	}
	// the non-owner async-mode requirement gets the TD-11b declaration through wgRegistry
	var d any = &wgRequirement{}
	if _, ok := d.(interface{ RecordsNoOwnership() }); !ok {
		t.Fatal("requirement undeclared")
	}
}

// decimal is how %v prints a byte slice ("[1 2 3]").
func decimal(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = strconv.Itoa(int(x))
	}
	return "[" + strings.Join(parts, " ") + "]"
}
