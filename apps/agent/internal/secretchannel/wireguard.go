package secretchannel

import (
	"context"
	"encoding/base64"
	"strings"

	"ngfw/agent/internal/descriptors/vpn"
)

// wireguardMaterial decodes the API's canonical wg key text. No plaintext is retained.
func wireguardMaterial(value []byte) ([]byte, error) {
	if len(value) != 44 {
		return nil, ErrUnavailable
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(string(value))
	if err != nil || len(raw) != vpn.X25519KeyLen {
		vpn.Zero(raw)
		return nil, ErrUnavailable
	}
	return raw, nil
}

// WireguardRef selects only the active transaction's key, with DF-5 identity.
func (s *Store) WireguardRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, "key/") && !strings.HasPrefix(ref, "psk/") {
		return "", ErrUnavailable
	}
	text, err := s.Text(ref)
	if err != nil {
		return "", err
	}
	defer vpn.Zero(text)
	raw, err := wireguardMaterial(text)
	if err != nil {
		return "", err
	}
	defer vpn.Zero(raw)
	if strings.HasPrefix(ref, "key/") {
		return vpn.X25519Ref(raw)
	}
	return s.state.keyer.Ref(raw), nil
}

// ResolveWireguard preserves old sealed generations for rollback/restart, like Resolve.
func (s *Store) ResolveWireguard(_ context.Context, ref string) ([]byte, error) {
	if vpn.CheckRef(ref) != nil {
		return nil, ErrUnavailable
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	for _, snapshot := range s.state.snapshots {
		for name, value := range snapshot {
			if !strings.HasPrefix(name, "key/") && !strings.HasPrefix(name, "psk/") {
				continue
			}
			raw, err := wireguardMaterial(value)
			if err != nil {
				continue
			}
			var candidate string
			if strings.HasPrefix(name, "key/") {
				candidate, err = vpn.X25519Ref(raw)
			} else {
				candidate = s.state.keyer.Ref(raw)
			}
			if err == nil && candidate == ref {
				return raw, nil
			}
			vpn.Zero(raw)
		}
	}
	return nil, ErrUnavailable
}
