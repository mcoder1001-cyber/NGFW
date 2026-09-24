package vpn

import (
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
)

// Reference prefixes (see the package doc).
const (
	// RefHMAC is the keyed fingerprint of symmetric material (D-096): HMAC-SHA256 under the
	// agent-local key (Keyer). Not guessable offline without the key file.
	RefHMAC = "hmac:"
	// RefX25519 references a WireGuard private key by its (public) public key.
	RefX25519 = "x25519:"
)

// X25519KeyLen is the length of WireGuard private, public and preshared keys.
const X25519KeyLen = 32

// Errors of the secret contract. None of them ever carries the offending value.
var (
	ErrSecretNotFound = errors.New("vpn: secret not found")
	ErrSecretMismatch = errors.New("vpn: secret material does not match its reference")
	ErrBadRef         = errors.New("vpn: malformed secret reference")
	ErrNoResolver     = errors.New("vpn: no secret resolver configured")
	ErrNoKeyer        = errors.New("vpn: no fingerprint key configured (WithKeyer)")
)

var (
	hmacRefRe   = regexp.MustCompile(`^hmac:[0-9a-f]{64}$`)
	x25519RefRe = regexp.MustCompile(`^x25519:[A-Za-z0-9+/]{43}=$`)
	// d051RefRe is the configuration-level reference form (D-051); never material.
	d051RefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)
)

// CheckRef validates the grammar of a DF-5 secret reference (hmac:, x25519:) without echoing it:
// a pasted plaintext value fails with a bare ErrBadRef (review M1). An unkeyed "sha256:"
// fingerprint is refused as well (D-096 forbids it; fix round 2, N5).
func CheckRef(ref string) error {
	if hmacRefRe.MatchString(ref) || x25519RefRe.MatchString(ref) {
		return nil
	}
	return ErrBadRef
}

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
type MapResolver struct {
	s *mapStore
	k *Keyer
}

type mapStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

// NewMapResolver returns a resolver holding the given symmetric materials under their keyed
// references (k.Ref).
func NewMapResolver(k *Keyer, materials ...[]byte) *MapResolver {
	m := &MapResolver{s: &mapStore{m: map[string][]byte{}}, k: k}
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

// Add stores symmetric material under its keyed reference and returns the reference.
func (m *MapResolver) Add(material []byte) string {
	ref := m.k.Ref(material)
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

// Keyer computes keyed fingerprints (D-096): HMAC-SHA256 under an agent-local secret key. The key
// sits behind a pointer and the type formats as "vpn.Keyer(hmac-sha256)", so %+v of a descriptor
// Config never prints it.
type Keyer struct{ k *keyBytes }

type keyBytes struct{ b []byte }

// KeyLen is the length of the fingerprint key.
const KeyLen = 32

// NewKeyer returns a Keyer over a copy of key (≥ KeyLen bytes). Tests use a fixed test key.
func NewKeyer(key []byte) (*Keyer, error) {
	if len(key) < KeyLen {
		return nil, fmt.Errorf("vpn: fingerprint key must be at least %d bytes", KeyLen)
	}
	return &Keyer{k: &keyBytes{b: append([]byte(nil), key...)}}, nil
}

// LoadOrCreateKeyFile returns the Keyer of the agent-local key file at path (in the agent's
// state dir). On first use it is created crash-safe: KeyLen random bytes are written to a
// temporary 0600 file in the same directory, fsynced, hard-linked into place (first writer wins
// when two agents start together) and the directory fsynced — a crash never leaves a short key
// file. An existing file is opened without following symlinks and checked on the open descriptor:
// a regular file of exactly KeyLen bytes, mode 0600 or stricter, owned by this user. The directory
// must not be group/world-writable. The key is never logged.
func LoadOrCreateKeyFile(path string) (*Keyer, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("vpn: fingerprint key dir: %w", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("vpn: fingerprint key dir %s must exist and not be group/world-writable", dir)
	}
	if k, err := loadKeyFile(path); !errors.Is(err, os.ErrNotExist) {
		return k, err
	}
	if err := createKeyFile(dir, path); err != nil {
		return nil, err
	}
	return loadKeyFile(path)
}

func createKeyFile(dir, path string) error {
	tmp, err := os.CreateTemp(dir, ".fingerprint-key-*")
	if err != nil {
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	key := make([]byte, KeyLen)
	defer Zero(key)
	if _, err := rand.Read(key); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	if _, err := tmp.Write(key); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	d, err := os.Open(dir) //nolint:gosec // the agent's state dir
	if err != nil {
		return fmt.Errorf("vpn: fingerprint key dir: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("vpn: fingerprint key dir: %w", err)
	}
	return nil
}

func loadKeyFile(path string) (*Keyer, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // agent state file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() != KeyLen || !ok || int(st.Uid) != os.Geteuid() {
		return nil, fmt.Errorf("vpn: fingerprint key %s must be a regular %d-byte file, mode 0600, owned by this user", path, KeyLen)
	}
	key := make([]byte, KeyLen)
	defer Zero(key)
	if _, err := io.ReadFull(f, key); err != nil {
		return nil, fmt.Errorf("vpn: fingerprint key: %w", err)
	}
	return NewKeyer(key)
}

// Ref returns the keyed reference "hmac:<hex>" of symmetric material.
func (k *Keyer) Ref(material []byte) string {
	m := hmac.New(sha256.New, k.k.b)
	m.Write(material)
	return RefHMAC + hex.EncodeToString(m.Sum(nil))
}

// String implements fmt.Stringer without the key.
func (k *Keyer) String() string { return "vpn.Keyer(hmac-sha256)" }

// GoString implements fmt.GoStringer.
func (k *Keyer) GoString() string { return k.String() }

// LogValue implements slog.LogValuer.
func (k *Keyer) LogValue() slog.Value { return slog.StringValue(k.String()) }

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

// Verify checks that material is what ref refers to (keyed references need k).
func Verify(k *Keyer, ref string, material []byte) error {
	if err := CheckRef(ref); err != nil {
		return err
	}
	var want string
	switch {
	case strings.HasPrefix(ref, RefHMAC):
		if k == nil {
			return ErrNoKeyer
		}
		want = k.Ref(material)
	default: // x25519
		var err error
		if want, err = X25519Ref(material); err != nil {
			return err
		}
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(ref)) != 1 {
		return fmt.Errorf("%w: %s", ErrSecretMismatch, Redact(ref))
	}
	return nil
}

// Resolve fetches the material behind ref through r and verifies it (with k for keyed
// references). The caller owns the returned buffer and should Zero it when done. ref == ""
// resolves to nil (no secret). A malformed reference (e.g. pasted plaintext) fails with a bare
// ErrBadRef before anything else — it is never echoed.
func Resolve(ctx context.Context, r Resolver, k *Keyer, ref string) ([]byte, error) {
	if ref == "" {
		return nil, nil
	}
	if err := CheckRef(ref); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w (need %s)", ErrNoResolver, Redact(ref))
	}
	mat, err := r.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := Verify(k, ref, mat); err != nil {
		Zero(mat)
		return nil, err
	}
	return mat, nil
}

// Zero overwrites b (key material) with zeros.
func Zero(b []byte) { clear(b) }

// Redact is the only form in which a reference appears in an error or log line. A well-formed
// keyed or x25519 reference is shortened to its prefix and the first 8 characters; a D-051 name is
// shown as is; anything else — a pasted plaintext secret, an unkeyed sha256 — becomes "<redacted>".
func Redact(ref string) string {
	switch {
	case hmacRefRe.MatchString(ref), x25519RefRe.MatchString(ref):
		i := strings.IndexByte(ref, ':')
		return ref[:i+1] + ref[i+1:i+9] + "…"
	case d051RefRe.MatchString(ref):
		return ref
	default:
		return "<redacted>"
	}
}
