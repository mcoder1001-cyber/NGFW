// Package secretchannel seals API-delivered snapshots separately from DesiredState.
package secretchannel

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"ngfw/agent/internal/descriptors/vpn"
)

const maxValue = 64 << 10
const maxSnapshot = 4 << 20

var reference = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// ErrUnavailable indicates that a referenced secret is missing.
var ErrUnavailable = errors.New("secret channel: referenced secret is unavailable")

// Store is opaque to fmt/slog. Snapshots are immutable; the txn state owns active IDs.
type Store struct{ state *state }
type state struct {
	mu        sync.RWMutex
	path      string
	owner     string
	keyer     *vpn.Keyer
	aead      cipher.AEAD
	snapshots map[string]map[string][]byte
	durable   map[string]bool
	active    string
}

func (s *Store) String() string { return "secretchannel.Store(<redacted>)" }

// GoString redacts the store during Go formatting.
func (s *Store) GoString() string { return s.String() }

// LogValue redacts the store during structured logging.
func (s *Store) LogValue() slog.Value { return slog.StringValue(s.String()) }

// Open loads and authenticates the owner-bound cache before the first resync.
func Open(dir, owner string) (*Store, error) {
	if owner == "" || !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(owner) {
		return nil, errors.New("secret channel: invalid owner")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "secret-cache-"+owner+".key")
	key, err := readPrivate(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		// Never manufacture a new key if a sealed cache already exists.
		if _, e := os.Stat(filepath.Join(dir, "secret-cache-"+owner+".sealed")); e == nil {
			return nil, errors.New("secret channel: cache key missing")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		var f *os.File
		//nolint:gosec // Fixed cache filename under the configured private state directory; owner is validated and reads reject symlinks and public permissions.
		f, err = os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, err = f.Write(key)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return nil, errors.New("secret channel: cannot open private cache key")
	}
	defer vpn.Zero(key)
	if len(key) != 32 {
		return nil, errors.New("secret channel: invalid cache key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	keyer, err := vpn.LoadOrCreateKeyFile(filepath.Join(dir, "vpn-"+owner+".key"))
	if err != nil {
		return nil, err
	}
	st := &state{path: filepath.Join(dir, "secret-cache-"+owner+".sealed"), owner: owner, keyer: keyer, aead: aead, snapshots: map[string]map[string][]byte{}, durable: map[string]bool{}}
	raw, err := readPrivate(st.path)
	if err == nil {
		if len(raw) < aead.NonceSize() {
			return nil, errors.New("secret channel: invalid sealed cache")
		}
		plain, e := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(owner))
		if e != nil {
			return nil, errors.New("secret channel: cannot authenticate sealed cache")
		}
		defer vpn.Zero(plain)
		if json.Unmarshal(plain, &st.snapshots) != nil {
			return nil, errors.New("secret channel: invalid sealed snapshot")
		}
		for id, m := range st.snapshots {
			st.durable[id] = true
			payload, e := validate(m)
			if e != nil {
				return nil, e
			}
			match := keyer.Ref(payload) == id
			vpn.Zero(payload)
			if !match {
				return nil, errors.New("secret channel: snapshot identity mismatch")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &Store{state: st}, nil
}
func readPrivate(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		return nil, errors.New("secret channel: cache must be a private regular file")
	}
	//nolint:gosec // Fixed cache filename under the configured private state directory; owner is validated and reads reject symlinks and public permissions.
	return os.ReadFile(path)
}
func validate(values map[string][]byte) ([]byte, error) {
	if values == nil {
		values = map[string][]byte{}
	}
	if len(values) > 1024 {
		return nil, errors.New("secret channel: too many references")
	}
	for ref, v := range values {
		if !reference.MatchString(ref) || len(v) == 0 || len(v) > maxValue {
			return nil, errors.New("secret channel: invalid reference or value size")
		}
	}
	p, e := json.Marshal(values)
	if len(p) > maxSnapshot {
		vpn.Zero(p)
		return nil, errors.New("secret channel: bundle too large")
	}
	return p, e
}

// Stage seals a complete immutable snapshot before any dataplane change.
func (s *Store) Stage(values map[string][]byte) (string, error) { return s.stage(values, true) }

// Transient validates a DryRun snapshot without writing the cache.
func (s *Store) Transient(values map[string][]byte) (string, error) { return s.stage(values, false) }
func (s *Store) stage(values map[string][]byte, persist bool) (string, error) {
	payload, err := validate(values)
	if err != nil {
		return "", err
	}
	defer vpn.Zero(payload)
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	id := st.keyer.Ref(payload)
	if _, exists := st.snapshots[id]; !exists && len(st.snapshots) >= 32 {
		return "", errors.New("secret channel: too many retained snapshots")
	}
	if _, ok := st.snapshots[id]; !ok {
		m := map[string][]byte{}
		for r, v := range values {
			m[r] = append([]byte(nil), v...)
		}
		st.snapshots[id] = m
	}
	if persist {
		st.durable[id] = true
		if err := st.save(); err != nil {
			return "", err
		}
	}
	return id, nil
}
func (st *state) save() error {
	durable := map[string]map[string][]byte{}
	for id, m := range st.snapshots {
		if st.durable[id] {
			durable[id] = m
		}
	}
	plain, err := json.Marshal(durable)
	if err != nil {
		return err
	}
	defer vpn.Zero(plain)
	nonce := make([]byte, st.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	sealed := st.aead.Seal(nonce, nonce, plain, []byte(st.owner))
	f, err := os.CreateTemp(filepath.Dir(st.path), ".secret-cache-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	e := f.Close()
	if err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, st.path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(st.path))
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	return errors.Join(syncErr, closeErr)
}

// Active returns the current immutable snapshot identifier.
func (s *Store) Active() string {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	return s.state.active
}

// Activate selects a previously staged snapshot.
func (s *Store) Activate(id string) error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if id != "" {
		if _, ok := s.state.snapshots[id]; !ok {
			return ErrUnavailable
		}
	}
	s.state.active = id
	return nil
}

// Ref converts a config reference to the existing keyed descriptor fingerprint.
func (s *Store) Ref(_ context.Context, ref string) (string, error) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	v, ok := s.state.snapshots[s.state.active][ref]
	if !ok {
		return "", ErrUnavailable
	}
	return s.state.keyer.Ref(v), nil
}

// Resolve keeps historical fingerprints available for scheduler rollback and confirm revert.
func (s *Store) Resolve(_ context.Context, ref string) ([]byte, error) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	for _, m := range s.state.snapshots {
		for _, v := range m {
			if s.state.keyer.Ref(v) == ref {
				return append([]byte(nil), v...), nil
			}
		}
	}
	return nil, ErrUnavailable
}

// Text is only for protocol-specific adapters; it never formats material in errors.
func (s *Store) Text(ref string) ([]byte, error) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	v, ok := s.state.snapshots[s.state.active][ref]
	if !ok {
		return nil, ErrUnavailable
	}
	return append([]byte(nil), v...), nil
}

// ID is the keyed identity used for transaction retry checks without persisting.
func (s *Store) ID(values map[string][]byte) (string, error) {
	p, e := validate(values)
	if e != nil {
		return "", e
	}
	defer vpn.Zero(p)
	return s.state.keyer.Ref(p), nil
}

// DiscardTransient removes a dry-run-only snapshot after restoring the active ID.
func (s *Store) DiscardTransient(id string) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if id != s.state.active && !s.state.durable[id] {
		for _, v := range s.state.snapshots[id] {
			vpn.Zero(v)
		}
		delete(s.state.snapshots, id)
	}
}

// Retain prunes obsolete sealed material after durable transaction state selects its bindings.
// Keep both current and confirmed snapshots while a timed commit is pending.
func (s *Store) Retain(ids ...string) error {
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	keep := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			keep[id] = true
		}
	}
	changed := false
	for id, m := range st.snapshots {
		if !keep[id] && id != st.active {
			for _, v := range m {
				vpn.Zero(v)
			}
			delete(st.snapshots, id)
			delete(st.durable, id)
			changed = true
		}
	}
	if changed {
		return st.save()
	}
	return nil
}
