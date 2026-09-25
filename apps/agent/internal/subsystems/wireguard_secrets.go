package subsystems

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"ngfw/agent/internal/descriptors/vpn"
)

// WireguardSecrets is the agent's WireGuard key material, by D-051 reference ("key/<name>": a 32-byte
// X25519 private key, "psk/<name>": a 32-byte preshared key). It serves both sides of the DF-5 secret
// contract: Ref maps a configuration reference to the DF-5 reference the builder puts into desired
// state ("x25519:<public key>", "hmac:<hex>" — never material), and Resolve (vpn.Resolver) returns the
// material behind a DF-5 reference to the descriptors' Create.
//
// Nothing fills it in the product agent yet: the API→agent secret channel is PENDING-secret-channel,
// so every WireGuard interface fails validation at privateKeyRef with agent.secret-unavailable. Tests
// fill it directly (Put) and test builds (-tags vrxtestsecrets) from a slot-local fixture file.
//
// The material sits behind a pointer and the type formats as "subsystems.WireguardSecrets(n
// secrets)", so %v/%+v/slog of a Config that holds it never print bytes.
type WireguardSecrets struct{ s *wgSecretStore }

type wgSecretStore struct {
	mu     sync.RWMutex
	keys   *vpn.Keyer
	byName map[string][]byte // D-051 ref → material
	byRef  map[string][]byte // DF-5 ref → material
	refOf  map[string]string // D-051 ref → DF-5 ref
}

// ErrSecretUnavailable is returned when the agent holds no material for a reference (PENDING-secret-channel).
var ErrSecretUnavailable = errors.New("no secret material in the agent (the API→agent secret channel is pending: PENDING-secret-channel)")

var d051Ref = regexp.MustCompile(`^(key|psk)/[A-Za-z0-9_.-]{1,64}$`)

// NewWireguardSecrets returns an empty store; k fingerprints preshared keys (D-096).
func NewWireguardSecrets(k *vpn.Keyer) *WireguardSecrets {
	return &WireguardSecrets{s: &wgSecretStore{keys: k, byName: map[string][]byte{}, byRef: map[string][]byte{}, refOf: map[string]string{}}}
}

// Put stores the material of a D-051 reference (32 bytes; a "key/" reference must be a valid X25519
// private key). The caller keeps ownership of material (it is copied).
func (w *WireguardSecrets) Put(ref string, material []byte) error {
	if !d051Ref.MatchString(ref) {
		return fmt.Errorf("wireguard secrets: %s is not a key/ or psk/ reference", vpn.Redact(ref))
	}
	if len(material) != vpn.X25519KeyLen {
		return fmt.Errorf("wireguard secrets: %s must be %d bytes", ref, vpn.X25519KeyLen)
	}
	var df5 string
	if strings.HasPrefix(ref, "key/") {
		r, err := vpn.X25519Ref(material)
		if err != nil {
			return fmt.Errorf("wireguard secrets: %s: %w", ref, err)
		}
		df5 = r
	} else {
		if w.s.keys == nil {
			return vpn.ErrNoKeyer
		}
		df5 = w.s.keys.Ref(material)
	}
	m := append([]byte(nil), material...)
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	if old, ok := w.s.refOf[ref]; ok {
		vpn.Zero(w.s.byRef[old])
		delete(w.s.byRef, old)
	}
	w.s.byName[ref], w.s.byRef[df5], w.s.refOf[ref] = m, m, df5
	return nil
}

// PutBase64 is Put for the WireGuard text form of a key (wg genkey / wg genpsk: std base64).
func (w *WireguardSecrets) PutBase64(ref, text string) error {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return fmt.Errorf("wireguard secrets: %s is not base64", ref)
	}
	defer vpn.Zero(b)
	return w.Put(ref, b)
}

// Ref maps a D-051 reference to its DF-5 reference (the builder's WireguardEnv.SecretRef).
func (w *WireguardSecrets) Ref(ref string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("%s: %w", vpn.Redact(ref), ErrSecretUnavailable)
	}
	w.s.mu.RLock()
	defer w.s.mu.RUnlock()
	r, ok := w.s.refOf[ref]
	if !ok {
		return "", fmt.Errorf("%s: %w", vpn.Redact(ref), ErrSecretUnavailable)
	}
	return r, nil
}

// Resolve implements vpn.Resolver: a copy of the material behind a DF-5 reference.
func (w *WireguardSecrets) Resolve(_ context.Context, ref string) ([]byte, error) {
	w.s.mu.RLock()
	defer w.s.mu.RUnlock()
	m, ok := w.s.byRef[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %s", vpn.ErrSecretNotFound, vpn.Redact(ref))
	}
	return append([]byte(nil), m...), nil
}

// Len is the number of references held.
func (w *WireguardSecrets) Len() int {
	w.s.mu.RLock()
	defer w.s.mu.RUnlock()
	return len(w.s.byName)
}

// String implements fmt.Stringer without references or material.
func (w *WireguardSecrets) String() string {
	return fmt.Sprintf("subsystems.WireguardSecrets(%d secrets)", w.Len())
}

// GoString implements fmt.GoStringer.
func (w *WireguardSecrets) GoString() string { return w.String() }

// LogValue implements slog.LogValuer.
func (w *WireguardSecrets) LogValue() slog.Value { return slog.StringValue(w.String()) }
