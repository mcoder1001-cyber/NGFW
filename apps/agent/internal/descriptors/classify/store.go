// Package classify implements the descriptors of VPP's classify API: tables, sessions, the
// per-interface ip/l2 table bindings and the input/output ACL bindings. Messages come from
// apps/agent/binapi/classify only.
//
// VPP classify tables have no tag or name: the only handle is the index VPP allocates, and
// indices are reused after a delete or a VPP restart. The owner's name ↔ index mapping
// therefore lives in a Store next to the create-time parameters VPP does not report back
// (memory_size, current_data_*, session action/metadata) and the output-ACL tables. A record
// is trusted only when (1) the Store was written against the running VPP instance (the
// vpe_pid of control_ping_reply; a different pid drops every record), (2) classify_table_ids
// still lists its index and (3) classify_table_info shows the recorded geometry (skip/match
// vectors and mask). See docs/agent/descriptors/classify.md.
package classify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"ngfw/agent/internal/descriptors/df2"
)

// TableRecord is what the Store keeps per owned classify table.
type TableRecord struct {
	Name              string                   `json:"name"`
	Index             uint32                   `json:"index"`
	SkipNVectors      uint32                   `json:"skip_n_vectors"`
	MatchNVectors     uint32                   `json:"match_n_vectors"`
	Mask              []byte                   `json:"mask"`
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

// OutputRecord is the output-ACL binding this owner applied to an interface: VPP reports
// only whether ip4-outacl / ip6-outacl is enabled, not which table, and refuses an unbind
// with a different table index, so the indices bound are kept here.
type OutputRecord struct {
	Interface string `json:"interface"`
	Ip4Table  string `json:"ip4_table,omitempty"`
	Ip4Index  uint32 `json:"ip4_index"`
	Ip6Table  string `json:"ip6_table,omitempty"`
	Ip6Index  uint32 `json:"ip6_index"`
}

// ErrNoSuchTable is returned when a classify table name is not in the Store.
var ErrNoSuchTable = errors.New("classify: no such table")

// Store maps this owner's classify table names to VPP table indices. Implementations are
// safe for concurrent use. The embedded Locker is the transaction lock: every Create/Delete
// that changes VPP tables together with the Store, and every prune of stale records, holds
// it, so a prune never drops a record whose table was created after the prune's snapshot.
type Store interface {
	sync.Locker
	Get(name string) (TableRecord, bool)
	Put(rec TableRecord) error
	Delete(name string) error
	// All returns every record sorted by name.
	All() []TableRecord
	GetOutput(iface string) (OutputRecord, bool)
	PutOutput(rec OutputRecord) error
	DeleteOutput(iface string) error
	// Instance returns the VPP instance (vpe_pid) the records were written against; known is
	// false for a fresh or legacy store.
	Instance() (pid uint32, known bool)
	// Reset drops every record and binds the store to VPP instance pid.
	Reset(pid uint32) error
}

type storeData struct {
	VPPInstance *uint32        `json:"vpp_instance,omitempty"`
	Tables      []TableRecord  `json:"tables"`
	Outputs     []OutputRecord `json:"outputs,omitempty"`
}

// store is the Store implementation; path "" keeps it in memory.
type store struct {
	txn      sync.Mutex
	mu       sync.Mutex
	path     string
	instance *uint32
	recs     map[string]TableRecord
	outputs  map[string]OutputRecord
}

// MemStore is an in-memory Store: fine for tests and for an agent that is never restarted;
// after a restart it is empty and Retrieve cannot attribute existing tables.
type MemStore struct{ store }

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{store{recs: map[string]TableRecord{}, outputs: map[string]OutputRecord{}}}
}

// FileStore is a Store persisted as one JSON file (rewritten atomically on every change), so
// the mapping survives an agent restart. The production agent points it at its state dir;
// tests use a temporary path.
type FileStore struct{ store }

// OpenFileStore loads path (a missing file is an empty store). A legacy file (a bare array of
// records, no VPP instance) loads with an unknown instance, so its records are not trusted.
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{store{path: path, recs: map[string]TableRecord{}, outputs: map[string]OutputRecord{}}}
	data, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("classify store %s: %w", path, err)
	}
	var d storeData
	if len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			var legacy []TableRecord
			if err2 := json.Unmarshal(data, &legacy); err2 != nil {
				return nil, fmt.Errorf("classify store %s: %w", path, err)
			}
			d = storeData{Tables: legacy}
		}
	}
	s.instance = d.VPPInstance
	for _, r := range d.Tables {
		s.recs[r.Name] = r
	}
	for _, o := range d.Outputs {
		s.outputs[o.Interface] = o
	}
	return s, nil
}

func (s *store) Lock()   { s.txn.Lock() }
func (s *store) Unlock() { s.txn.Unlock() }

func (s *store) Get(name string) (TableRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[name]
	return r, ok
}

func (s *store) Put(rec TableRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[rec.Name] = rec
	return s.save()
}

func (s *store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, name)
	return s.save()
}

func (s *store) All() []TableRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sorted(s.recs)
}

func (s *store) GetOutput(iface string) (OutputRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.outputs[iface]
	return o, ok
}

func (s *store) PutOutput(rec OutputRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputs[rec.Interface] = rec
	return s.save()
}

func (s *store) DeleteOutput(iface string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.outputs, iface)
	return s.save()
}

func (s *store) Instance() (uint32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.instance == nil {
		return 0, false
	}
	return *s.instance, true
}

func (s *store) Reset(pid uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = map[string]TableRecord{}
	s.outputs = map[string]OutputRecord{}
	s.instance = &pid
	return s.save()
}

func sorted(m map[string]TableRecord) []TableRecord {
	out := make([]TableRecord, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// save persists the store (caller holds mu); a memory store has nothing to do.
func (s *store) save() error {
	if s.path == "" {
		return nil
	}
	d := storeData{VPPInstance: s.instance, Tables: sorted(s.recs)}
	for _, o := range s.outputs {
		d.Outputs = append(d.Outputs, o)
	}
	sort.Slice(d.Outputs, func(i, j int) bool { return d.Outputs[i].Interface < d.Outputs[j].Interface })
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := df2.WriteFileAtomic(s.path, data); err != nil {
		return fmt.Errorf("classify store: %w", err)
	}
	return nil
}
