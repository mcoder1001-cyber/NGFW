package df2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ClaimStore is DF-4's claim store (acl.ClaimStore, merged on main): the record of objects
// this owner configured on untagged interfaces (physical ports carry no owner tag). DF-2
// claims by object key (e.g. "urpf.interface/GigabitEthernet0/0/0/ipv4/rx"), so every
// descriptor sees exactly its own objects on a shared untagged port. P05 passes one
// persisted store (FileClaimStore in the agent state dir) to every Register; the default
// is in memory.
type ClaimStore = acl.ClaimStore

// ErrForeignInterface is returned (wrapped) when a desired object names an interface tagged
// by another owner: DF-2 never configures another owner's interface.
var ErrForeignInterface = errors.New("interface belongs to another owner")

// Options configure the DF-2 descriptors that attach objects to interfaces.
type Options struct{ Claims ClaimStore }

// Option sets one Options field.
type Option func(*Options)

// WithClaims sets the claim store for objects on untagged interfaces.
func WithClaims(s ClaimStore) Option { return func(o *Options) { o.Claims = s } }

// BuildOptions applies opts over the defaults (an in-memory claim store).
func BuildOptions(opts ...Option) Options {
	var o Options
	for _, f := range opts {
		f(&o)
	}
	if o.Claims == nil {
		o.Claims = acl.NewMemoryClaimStore()
	}
	return o
}

// Resolve returns the sw_if_index of the interface an object is created on and whether
// the interface is untagged (then the object must be claimed after Create). An interface
// tagged by another owner is refused with ErrForeignInterface.
func (s *Interfaces) Resolve(name string) (idx uint32, untagged bool, err error) {
	d, ok := s.byName[name]
	if !ok {
		return 0, false, fmt.Errorf("%w: %q", ErrNoSuchInterface, name)
	}
	if d.SwIfIndex == 0 {
		return 0, false, fmt.Errorf("%w: %q (local0 is never configured)", ErrForeignInterface, name)
	}
	if d.Tag == "" {
		return uint32(d.SwIfIndex), true, nil
	}
	if _, ours := vpp.ParseOwnerTag(d.Tag, s.owner); !ours {
		return 0, false, fmt.Errorf("%w: %q is tagged %q", ErrForeignInterface, name, d.Tag)
	}
	return uint32(d.SwIfIndex), false, nil
}

// OwnsObject reports whether the object with key on interface idx is ours: the interface
// carries this owner's tag, or it is untagged and the key is claimed.
func (s *Interfaces) OwnsObject(idx uint32, key scheduler.Key, claims ClaimStore) bool {
	d, ok := s.byIndex[idx]
	if !ok {
		return false
	}
	if d.Tag == "" {
		return claims != nil && claims.Claimed(string(key))
	}
	_, ours := vpp.ParseOwnerTag(d.Tag, s.owner)
	return ours
}

// Candidates returns every interface index an object of this owner may live on (tagged by
// this owner, or untagged), ascending. Callers confirm with OwnsObject once the key is known.
func (s *Interfaces) Candidates() []uint32 {
	var out []uint32
	for idx, d := range s.byIndex {
		if idx == 0 {
			continue // local0
		}
		if d.Tag == "" {
			out = append(out, idx)
			continue
		}
		if _, ours := vpp.ParseOwnerTag(d.Tag, s.owner); ours {
			out = append(out, idx)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Claim records key when the object was created on an untagged interface.
func Claim(claims ClaimStore, untagged bool, key scheduler.Key) error {
	if !untagged || claims == nil {
		return nil
	}
	if err := claims.Claim(string(key)); err != nil {
		return fmt.Errorf("claim %s: %w", key, err)
	}
	return nil
}

// Release drops the claim on key (no-op when there is none).
func Release(claims ClaimStore, key scheduler.Key) error {
	if claims == nil {
		return nil
	}
	if err := claims.Release(string(key)); err != nil {
		return fmt.Errorf("release %s: %w", key, err)
	}
	return nil
}

// FileClaimStore is a ClaimStore persisted as a sorted JSON array (atomic rewrite on every
// change), so claims on physical ports survive an agent restart.
type FileClaimStore struct {
	mu   sync.Mutex
	path string
	m    map[string]struct{}
}

// OpenFileClaimStore loads path (a missing file is an empty store).
func OpenFileClaimStore(path string) (*FileClaimStore, error) {
	s := &FileClaimStore{path: path, m: map[string]struct{}{}}
	data, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("claim store %s: %w", path, err)
	}
	var keys []string
	if len(data) > 0 {
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, fmt.Errorf("claim store %s: %w", path, err)
		}
	}
	for _, k := range keys {
		s.m[k] = struct{}{}
	}
	return s, nil
}

// Claim implements ClaimStore.
func (s *FileClaimStore) Claim(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[key]; ok {
		return nil
	}
	s.m[key] = struct{}{}
	return s.save()
}

// Release implements ClaimStore.
func (s *FileClaimStore) Release(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[key]; !ok {
		return nil
	}
	delete(s.m, key)
	return s.save()
}

// Claimed implements ClaimStore.
func (s *FileClaimStore) Claimed(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	return ok
}

func (s *FileClaimStore) save() error {
	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.path, data)
}

// WriteFileAtomic writes data to path through a temp file + rename in the same directory.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".df2-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Named is any desired object attached to an interface by name.
type Named interface{ GetInterface() string }

// SkipDelete re-verifies, immediately before a delete by sw_if_index (D-071), that the index
// still names the object's interface and that the interface is still ours (own tag, or
// untagged and key claimed). skip=true means the interface is gone or the index now belongs
// to another interface / owner: the caller must not touch VPP (our object went with the
// interface) and only drops its claim.
func SkipDelete(ctx context.Context, c vpp.Client, owner string, swIfIndex uint32, obj Named, key scheduler.Key, claims ClaimStore) (bool, error) {
	ifs, err := DumpInterfaces(ctx, c, owner)
	if err != nil {
		return false, err
	}
	name, ok := ifs.Name(swIfIndex)
	if !ok || name != obj.GetInterface() {
		return true, nil
	}
	return !ifs.OwnsObject(swIfIndex, key, claims), nil
}
