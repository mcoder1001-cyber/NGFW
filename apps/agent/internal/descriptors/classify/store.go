// Package classify implements the descriptors of VPP's classify API: tables, sessions, the
// per-interface ip/l2 table bindings and the input/output ACL bindings. Messages come from
// apps/agent/binapi/classify only.
//
// VPP classify tables have no tag or name: the only handle is the index VPP allocates, and
// indices are reused after a delete or a VPP restart. The owner's name ↔ index mapping
// therefore lives in a Store next to the create-time parameters VPP does not report back
// (memory_size, current_data_*, session action/metadata) and the output-ACL tables. A record
// is trusted only when (1) the Store was written against the running VPP instance (the D-080
// boot identity, bootid.Current; a different identity drops every record), (2) classify_table_ids
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
	"ngfw/agent/internal/vpp/bootid"
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
	IP4Table  string `json:"ip4_table,omitempty"`
	IP4Index  uint32 `json:"ip4_index"`
	IP6Table  string `json:"ip6_table,omitempty"`
	IP6Index  uint32 `json:"ip6_index"`
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
	// Outputs returns every output-ACL record sorted by interface.
	Outputs() []OutputRecord
	// PutBinding / DeleteBinding / Bindings keep the write-only bindings (interface-ip-table,
	// interface-l2-tables) this owner applied, keyed by descriptor key, for TableUsers.
	PutBinding(rec BindingRecord) error
	DeleteBinding(key string) error
	Bindings() []BindingRecord
	// Instance returns the VPP instance (D-080 boot identity) the records were written against;
	// known is false for a fresh or legacy store (pre-TD-1 files carried the vpe_pid only).
	Instance() (id bootid.Identity, known bool)
	// Reset drops every record and binds the store to VPP instance id.
	Reset(id bootid.Identity) error
}

type storeData struct {
	// VPPBoot is the encoded boot identity (bootid.Identity.String). Files written before TD-1
	// carry "vpp_instance" (the vpe_pid) instead, which is ignored: such a store has an unknown
	// instance, so its records are not trusted and the first Prune resets it.
	VPPBoot string         `json:"vpp_boot,omitempty"`
	Tables  []TableRecord  `json:"tables"`
	Outputs []OutputRecord `json:"outputs,omitempty"`
	// Bindings are the write-only bindings applied (TD-3, D-095).
	Bindings []BindingRecord `json:"bindings,omitempty"`
}

// store is the Store implementation; path "" keeps it in memory.
type store struct {
	txn      sync.Mutex
	mu       sync.Mutex
	path     string
	instance *bootid.Identity
	recs     map[string]TableRecord
	outputs  map[string]OutputRecord
	bindings map[string]BindingRecord
}

// MemStore is an in-memory Store: fine for tests and for an agent that is never restarted;
// after a restart it is empty and Retrieve cannot attribute existing tables.
type MemStore struct{ store }

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{store{recs: map[string]TableRecord{}, outputs: map[string]OutputRecord{}, bindings: map[string]BindingRecord{}}}
}

// FileStore is a Store persisted as one JSON file (rewritten atomically on every change), so
// the mapping survives an agent restart. The production agent points it at its state dir;
// tests use a temporary path.
type FileStore struct{ store }

// OpenFileStore loads path (a missing file is an empty store). A legacy file (a bare array of
// records, or a vpe_pid-only "vpp_instance") loads with an unknown instance, so its records are
// not trusted.
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{store{path: path, recs: map[string]TableRecord{}, outputs: map[string]OutputRecord{}, bindings: map[string]BindingRecord{}}}
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
	if d.VPPBoot != "" {
		if id, err := bootid.Parse(d.VPPBoot); err == nil {
			s.instance = &id
		}
	}
	for _, r := range d.Tables {
		s.recs[r.Name] = r
	}
	for _, o := range d.Outputs {
		s.outputs[o.Interface] = o
	}
	for _, b := range d.Bindings {
		s.bindings[b.Key] = b
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

func (s *store) Outputs() []OutputRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outputList()
}

func (s *store) outputList() []OutputRecord {
	out := make([]OutputRecord, 0, len(s.outputs))
	for _, o := range s.outputs {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out
}

func (s *store) PutBinding(rec BindingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[rec.Key] = rec
	return s.save()
}

func (s *store) DeleteBinding(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[key]; !ok {
		return nil
	}
	delete(s.bindings, key)
	return s.save()
}

func (s *store) Bindings() []BindingRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindingList()
}

func (s *store) bindingList() []BindingRecord {
	out := make([]BindingRecord, 0, len(s.bindings))
	for _, b := range s.bindings {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (s *store) Instance() (bootid.Identity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.instance == nil {
		return bootid.Identity{}, false
	}
	return *s.instance, true
}

func (s *store) Reset(id bootid.Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = map[string]TableRecord{}
	s.outputs = map[string]OutputRecord{}
	s.bindings = map[string]BindingRecord{}
	s.instance = &id
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
	d := storeData{Tables: sorted(s.recs)}
	if s.instance != nil {
		d.VPPBoot = s.instance.String()
	}
	d.Outputs = s.outputList()
	if len(d.Outputs) == 0 {
		d.Outputs = nil
	}
	if b := s.bindingList(); len(b) > 0 {
		d.Bindings = b
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := df2.WriteFileAtomic(s.path, data); err != nil {
		return fmt.Errorf("classify store: %w", err)
	}
	return nil
}
