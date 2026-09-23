// Package classify implements the descriptors of VPP's classify API: tables, sessions, the
// per-interface ip/l2 table bindings and the input/output ACL bindings. Messages come from
// apps/agent/binapi/classify only.
//
// VPP classify tables have no tag or name: the only handle is the index VPP allocates. The
// owner's name ↔ index mapping therefore lives in a Store next to the create-time parameters
// VPP does not report back (memory_size, current_data_*, session action/metadata). Retrieve
// trusts the Store only for indices that classify_table_ids still lists; stale records are
// dropped. See docs/agent/descriptors/classify.md.
package classify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// TableRecord is what the Store keeps per owned classify table.
type TableRecord struct {
	Name              string                   `json:"name"`
	Index             uint32                   `json:"index"`
	SkipNVectors      uint32                   `json:"skip_n_vectors"`
	MatchNVectors     uint32                   `json:"match_n_vectors"`
	MemorySize        uint32                   `json:"memory_size"`
	CurrentDataFlag   bool                     `json:"current_data_flag,omitempty"`
	CurrentDataOffset int32                    `json:"current_data_offset,omitempty"`
	Sessions          map[string]SessionRecord `json:"sessions,omitempty"` // by hex(match)
}

// SessionRecord holds the session parameters VPP does not report.
type SessionRecord struct {
	Action   int32  `json:"action"`
	Metadata uint32 `json:"metadata"`
}

// ErrNoSuchTable is returned when a classify table name is not in the Store.
var ErrNoSuchTable = errors.New("classify: no such table")

// Store maps this owner's classify table names to VPP table indices. Implementations are
// safe for concurrent use.
type Store interface {
	Get(name string) (TableRecord, bool)
	Put(rec TableRecord) error
	Delete(name string) error
	// All returns every record sorted by name.
	All() []TableRecord
}

// MemStore is an in-memory Store: fine for tests and for an agent that is never restarted;
// after a restart it is empty and Retrieve cannot attribute existing tables.
type MemStore struct {
	mu   sync.Mutex
	recs map[string]TableRecord
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore { return &MemStore{recs: map[string]TableRecord{}} }

// Get implements Store.
func (s *MemStore) Get(name string) (TableRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[name]
	return r, ok
}

// Put implements Store.
func (s *MemStore) Put(rec TableRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[rec.Name] = rec
	return nil
}

// Delete implements Store.
func (s *MemStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, name)
	return nil
}

// All implements Store.
func (s *MemStore) All() []TableRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sorted(s.recs)
}

func sorted(m map[string]TableRecord) []TableRecord {
	out := make([]TableRecord, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FileStore is a Store persisted as one JSON file (rewritten atomically on every change), so
// the mapping survives an agent restart. The production agent points it at its state dir;
// tests use a slot-prefixed path.
type FileStore struct {
	mu   sync.Mutex
	path string
	recs map[string]TableRecord
}

// OpenFileStore loads path (a missing file is an empty store).
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path, recs: map[string]TableRecord{}}
	data, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("classify store %s: %w", path, err)
	}
	var recs []TableRecord
	if len(data) > 0 {
		if err := json.Unmarshal(data, &recs); err != nil {
			return nil, fmt.Errorf("classify store %s: %w", path, err)
		}
	}
	for _, r := range recs {
		s.recs[r.Name] = r
	}
	return s, nil
}

// Get implements Store.
func (s *FileStore) Get(name string) (TableRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[name]
	return r, ok
}

// Put implements Store.
func (s *FileStore) Put(rec TableRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[rec.Name] = rec
	return s.save()
}

// Delete implements Store.
func (s *FileStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, name)
	return s.save()
}

// All implements Store.
func (s *FileStore) All() []TableRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sorted(s.recs)
}

func (s *FileStore) save() error {
	data, err := json.MarshalIndent(sorted(s.recs), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("classify store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".classify-*.json")
	if err != nil {
		return fmt.Errorf("classify store: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("classify store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("classify store: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("classify store: %w", err)
	}
	return nil
}
