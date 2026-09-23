package dfkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// BootRecord says that this owner applied an object to the VPP process with identity VPPID
// (main-thread PID, iface.VPPIdentity) with the given canonical value (JSON).
type BootRecord struct {
	Key   string `json:"key"`
	VPPID uint32 `json:"vpp_id"`
	Value string `json:"value"`
}

// BootStore keeps BootRecords (D-076): write-only objects whose VPP add is not idempotent (pcap
// capture, http_static) record what they applied, keyed by the VPP boot identity, and skip the
// re-add on a resync while VPP is the same process; after a VPP restart the identity differs and
// they add once more. P05/P08 install a persisted store (NewFileBootStore in the state dir) with
// SetBootStore; the default is in memory.
type BootStore interface {
	Get(key string) (BootRecord, bool)
	Put(r BootRecord) error
	Delete(key string) error
}

type memBoot struct {
	mu sync.Mutex
	m  map[string]BootRecord
}

// NewMemoryBootStore returns an in-memory BootStore.
func NewMemoryBootStore() BootStore { return &memBoot{m: map[string]BootRecord{}} }

func (s *memBoot) Get(key string) (BootRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[key]
	return r, ok
}

func (s *memBoot) Put(r BootRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[r.Key] = r
	return nil
}

func (s *memBoot) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// FileBootStore is a BootStore persisted as a JSON array (atomic rewrite on every change).
type FileBootStore struct {
	mu   sync.Mutex
	path string
	mem  map[string]BootRecord
}

// NewFileBootStore opens (or starts) the store at path.
func NewFileBootStore(path string) (*FileBootStore, error) {
	s := &FileBootStore{path: path, mem: map[string]BootRecord{}}
	raw, err := os.ReadFile(path) //nolint:gosec // path is the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("boot store %s: %w", path, err)
	}
	var recs []BootRecord
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil, fmt.Errorf("boot store %s: %w", path, err)
	}
	for _, r := range recs {
		s.mem[r.Key] = r
	}
	return s, nil
}

// Get implements BootStore.
func (s *FileBootStore) Get(key string) (BootRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.mem[key]
	return r, ok
}

// Put implements BootStore.
func (s *FileBootStore) Put(r BootRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem[r.Key] = r
	return s.flush()
}

// Delete implements BootStore.
func (s *FileBootStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mem, key)
	return s.flush()
}

func (s *FileBootStore) flush() error {
	recs := make([]BootRecord, 0, len(s.mem))
	for _, r := range s.mem {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Key < recs[j].Key })
	raw, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("boot store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("boot store: %w", err)
	}
	return nil
}

var (
	bootMu sync.Mutex
	boots  = map[string]BootStore{}
)

// SetBootStore installs the BootStore of owner; nil restores a fresh in-memory store.
func SetBootStore(owner string, s BootStore) {
	bootMu.Lock()
	defer bootMu.Unlock()
	if s == nil {
		s = NewMemoryBootStore()
	}
	boots[owner] = s
}

// Boot returns the BootStore of owner (in memory unless SetBootStore installed another).
func Boot(owner string) BootStore {
	bootMu.Lock()
	defer bootMu.Unlock()
	s, ok := boots[owner]
	if !ok {
		s = NewMemoryBootStore()
		boots[owner] = s
	}
	return s
}

// AppliedThisBoot reports whether owner recorded exactly value for key on the running VPP
// process, and returns the current VPP identity for a following Record.
func AppliedThisBoot(ctx context.Context, c vpp.Client, owner string, key scheduler.Key, value string) (bool, uint32, error) {
	id, err := iface.VPPIdentity(ctx, c)
	if err != nil {
		return false, 0, err
	}
	r, ok := Boot(owner).Get(string(key))
	return ok && r.VPPID == id && r.Value == value, id, nil
}

// StartedThisBoot reports whether owner has any record for key on the running VPP process.
func StartedThisBoot(ctx context.Context, c vpp.Client, owner string, key scheduler.Key) (bool, error) {
	id, err := iface.VPPIdentity(ctx, c)
	if err != nil {
		return false, err
	}
	r, ok := Boot(owner).Get(string(key))
	return ok && r.VPPID == id, nil
}

// Record stores that owner applied value for key to the VPP process id.
func Record(owner string, key scheduler.Key, id uint32, value string) error {
	return Boot(owner).Put(BootRecord{Key: string(key), VPPID: id, Value: value})
}

// Forget removes owner's record for key.
func Forget(owner string, key scheduler.Key) error { return Boot(owner).Delete(string(key)) }

// Dedupe drops KVs with a key seen before (keeps the first) — a Retrieve must never report one
// key twice (two interfaces with one logical name, a repeated dump entry).
func Dedupe(kvs []scheduler.KV) []scheduler.KV {
	seen := make(map[scheduler.Key]bool, len(kvs))
	out := kvs[:0]
	for _, kv := range kvs {
		if seen[kv.Key] {
			continue
		}
		seen[kv.Key] = true
		out = append(out, kv)
	}
	return out
}
