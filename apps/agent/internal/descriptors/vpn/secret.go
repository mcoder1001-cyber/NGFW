package vpn

import (
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Reference prefixes (see the package doc).
const (
	RefSHA256 = "sha256:"
	RefX25519 = "x25519:"
)

// X25519KeyLen is the length of WireGuard private, public and preshared keys.
const X25519KeyLen = 32

// Errors of the secret contract.
var (
	ErrSecretNotFound = errors.New("vpn: secret not found")
	ErrSecretMismatch = errors.New("vpn: secret material does not match its reference")
	ErrBadRef         = errors.New("vpn: malformed secret reference")
	ErrNoResolver     = errors.New("vpn: no secret resolver configured")
)

// Resolver returns the material behind a secret reference. The agent's secret store implements
// it; descriptors call Resolve (below), which also verifies the material against the reference.
// Implementations must never log ref → material, and must be opaque to fmt/slog: descriptors keep
// the Resolver in an unexported Config field, and fmt's %+v walks unexported fields by reflection
// without calling String, so any material reachable by value (a map of []byte, a struct field)
// would be printed. Keep it behind a pointer, as MapResolver does.
type Resolver interface {
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

// MapResolver is an in-memory Resolver for tests and for callers that ship material next to the
// desired state. Keys are references (Ref / X25519Ref). The material sits behind a pointer, so
// formatting a descriptor, its Config or the resolver prints an address or "vpn.MapResolver(n
// secrets)", never bytes. Safe for concurrent use.
type MapResolver struct{ s *mapStore }

type mapStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

// NewMapResolver returns a resolver holding the given symmetric materials under their sha256
// references.
func NewMapResolver(materials ...[]byte) *MapResolver {
	m := &MapResolver{s: &mapStore{m: map[string][]byte{}}}
	for _, mat := range materials {
		m.Add(mat)
	}
	return m
}

// Put stores material under ref as given, without checking that it matches (tests use it to
// provoke ErrSecretMismatch).
func (m *MapResolver) Put(ref string, material []byte) {
	m.s.mu.Lock()
	defer m.s.mu.Unlock()
	m.s.m[ref] = append([]byte(nil), material...)
}

// Add stores symmetric material under its sha256 reference and returns the reference.
func (m *MapResolver) Add(material []byte) string {
	ref := Ref(material)
	m.Put(ref, material)
	return ref
}

// AddX25519 stores a 32-byte X25519 private key under its x25519 reference and returns it.
func (m *MapResolver) AddX25519(private []byte) (string, error) {
	ref, err := X25519Ref(private)
	if err != nil {
		return "", err
	}
	m.Put(ref, private)
	return ref, nil
}

// Resolve implements Resolver.
func (m *MapResolver) Resolve(_ context.Context, ref string) ([]byte, error) {
	m.s.mu.RLock()
	mat, ok := m.s.m[ref]
	m.s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, Redact(ref))
	}
	return append([]byte(nil), mat...), nil
}

// String implements fmt.Stringer without revealing references or material.
func (m *MapResolver) String() string {
	m.s.mu.RLock()
	defer m.s.mu.RUnlock()
	return fmt.Sprintf("vpn.MapResolver(%d secrets)", len(m.s.m))
}

// GoString implements fmt.GoStringer (%#v) the same way.
func (m *MapResolver) GoString() string { return m.String() }

// LogValue implements slog.LogValuer.
func (m *MapResolver) LogValue() slog.Value { return slog.StringValue(m.String()) }

// Ref returns the sha256 reference of symmetric material.
func Ref(material []byte) string {
	sum := sha256.Sum256(material)
	return RefSHA256 + hex.EncodeToString(sum[:])
}

// X25519Ref returns the x25519 reference (base64 public key) of a 32-byte private key.
func X25519Ref(private []byte) (string, error) {
	priv, err := ecdh.X25519().NewPrivateKey(private)
	if err != nil {
		return "", fmt.Errorf("%w: x25519 private key: %v", ErrBadRef, err)
	}
	return X25519RefFromPublic(priv.PublicKey().Bytes()), nil
}

// X25519RefFromPublic returns the x25519 reference for a public key as VPP reports it.
func X25519RefFromPublic(public []byte) string {
	return RefX25519 + base64.StdEncoding.EncodeToString(public)
}

// PublicKeyOfRef returns the public key encoded in an x25519 reference.
func PublicKeyOfRef(ref string) ([]byte, error) {
	if !strings.HasPrefix(ref, RefX25519) {
		return nil, fmt.Errorf("%w: %s is not an x25519 reference", ErrBadRef, Redact(ref))
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ref, RefX25519))
	if err != nil || len(pub) != X25519KeyLen {
		return nil, fmt.Errorf("%w: %s: bad public key", ErrBadRef, Redact(ref))
	}
	return pub, nil
}

// Verify checks that material is what ref refers to.
func Verify(ref string, material []byte) error {
	var want string
	switch {
	case strings.HasPrefix(ref, RefSHA256):
		want = Ref(material)
	case strings.HasPrefix(ref, RefX25519):
		var err error
		if want, err = X25519Ref(material); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: %s", ErrBadRef, Redact(ref))
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(ref)) != 1 {
		return fmt.Errorf("%w: %s", ErrSecretMismatch, Redact(ref))
	}
	return nil
}

// Resolve fetches the material behind ref through r and verifies it. The caller owns the returned
// buffer and should Zero it when done. ref == "" resolves to nil (no secret).
func Resolve(ctx context.Context, r Resolver, ref string) ([]byte, error) {
	if ref == "" {
		return nil, nil
	}
	if r == nil {
		return nil, fmt.Errorf("%w (need %s)", ErrNoResolver, Redact(ref))
	}
	mat, err := r.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := Verify(ref, mat); err != nil {
		Zero(mat)
		return nil, err
	}
	return mat, nil
}

// Zero overwrites b (key material) with zeros.
func Zero(b []byte) { clear(b) }

// Redact shortens a reference for error messages: the prefix and the first 8 characters of the
// fingerprint. A reference is not secret, but keeping messages short avoids pasting whole hashes
// into logs.
func Redact(ref string) string {
	i := strings.IndexByte(ref, ':')
	if i < 0 || len(ref)-i-1 <= 8 {
		return ref
	}
	return ref[:i+1] + ref[i+1:i+9] + "…"
}
