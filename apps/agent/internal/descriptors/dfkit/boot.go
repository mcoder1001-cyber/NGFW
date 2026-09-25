package dfkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// BootRecord says that this owner applied an object to the VPP instance with the D-080 boot
// identity Identity (bootid.Identity.String of BootIdentity), with the given canonical value
// (JSON). A record in another format (pre-D-080 PID only) never matches: re-added once.
type BootRecord struct {
	Key      string `json:"key"`
	Identity string `json:"identity"`
	Value    string `json:"value"`
}

// BootStore keeps BootRecords (D-076): write-only objects whose VPP add is not idempotent (the
// pcap capture) record what they applied, keyed by the VPP boot identity, and skip the re-add on
// a resync while VPP is the same process; after a VPP restart the identity differs and they add
// once more. The store is an explicit constructor argument (review M4): the agent passes a
// persisted NewFileBootStore in its state dir so an agent restart does not orphan its own
// capture; NewMemoryBootStore is for tests.
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

// Put implements BootStore. The file is written (and fsynced) before memory changes, so a
// failed write leaves both unchanged.
func (s *FileBootStore) Put(r BootRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.copyMem()
	next[r.Key] = r
	if err := s.flush(next); err != nil {
		return err
	}
	s.mem = next
	return nil
}

// Delete implements BootStore (write first, as Put).
func (s *FileBootStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.mem[key]; !ok {
		return nil
	}
	next := s.copyMem()
	delete(next, key)
	if err := s.flush(next); err != nil {
		return err
	}
	s.mem = next
	return nil
}

func (s *FileBootStore) copyMem() map[string]BootRecord {
	out := make(map[string]BootRecord, len(s.mem)+1)
	for k, v := range s.mem {
		out[k] = v
	}
	return out
}

func (s *FileBootStore) flush(m map[string]BootRecord) error {
	recs := make([]BootRecord, 0, len(m))
	for _, r := range m {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Key < recs[j].Key })
	raw, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	// temp file + fsync + rename + directory fsync (TD-16b; TD-9 review 1.5a: the shared helper)
	if err := ownertable.WriteAtomic(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("boot store: %w", err)
	}
	return nil
}

// AppliedThisBoot reports whether store holds exactly value for key on the running VPP instance,
// and returns the current boot identity (encoded, for a following BootRecord).
func AppliedThisBoot(ctx context.Context, c vpp.Client, store BootStore, key scheduler.Key, value string) (bool, string, error) {
	id, err := IdentitySource(ctx, c)
	if err != nil {
		return false, "", err
	}
	r, ok := store.Get(string(key))
	return ok && bootid.Matches(r.Identity, id) && r.Value == value, id.String(), nil
}

// StartedThisBoot reports whether store has any record for key on the running VPP instance.
func StartedThisBoot(ctx context.Context, c vpp.Client, store BootStore, key scheduler.Key) (bool, error) {
	id, err := IdentitySource(ctx, c)
	if err != nil {
		return false, err
	}
	r, ok := store.Get(string(key))
	return ok && bootid.Matches(r.Identity, id), nil
}

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
