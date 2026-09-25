package qos

// qos.meta (F-qos-flat): the agent-local record of what the configuration document says about `services.qos` that
// VPP cannot hold — descriptions (D-073b), the names of the egress maps (VPP numbers them, the document names them)
// and whether the document fixed a map id or a shaper burst, and the order of each map row's entries. It is not a
// VPP object: Retrieve reads the record file, and the agent's assembler uses it only to name and decorate the
// objects VPP reports (a map, a policer or a mark VPP does not have is never reported because of this record). The
// record is re-creatable metadata: an unreadable file counts as absent and the next reconcile rewrites it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
)

// NameMeta is the document record descriptor.
const NameMeta = "qos.meta"

// MetaID is the id of the one record: the document node it describes.
const MetaID = "services.qos"

// KeyMeta is "qos.meta/services.qos".
func KeyMeta() scheduler.Key { return scheduler.Join(NameMeta, MetaID) }

// DocMeta is the Value of the qos.meta record.
type DocMeta struct {
	// Policers maps a policer name to its description (only policers with one).
	Policers map[string]string `json:"policers,omitempty"`
	// Shapers holds the shapers with a description or an explicit burst.
	Shapers map[string]ShaperMeta `json:"shapers,omitempty"`
	// Maps holds every egress map of the document by name.
	Maps map[string]MapMeta `json:"maps,omitempty"`
	// Interfaces maps an attachment's interface to its description (only attachments with one).
	Interfaces map[string]string `json:"interfaces,omitempty"`
}

// ShaperMeta is what the record keeps of one shaper.
type ShaperMeta struct {
	Description string `json:"description,omitempty"`
	// Burst is true when the document set burstBytes (otherwise the agent derived it from the rate).
	Burst bool `json:"burst,omitempty"`
}

// MapMeta is what the record keeps of one egress map.
type MapMeta struct {
	// ID is the VPP egress map id the map was projected onto (fixed by the document or allocated by the agent).
	ID uint32 `json:"id"`
	// ExplicitID is true when the document fixed the id.
	ExplicitID  bool   `json:"explicit_id,omitempty"`
	Description string `json:"description,omitempty"`
	// Rows lists, per source (ext, vlan, mpls, ip), the recorded values the document lists, in document order.
	Rows map[string][]int `json:"rows,omitempty"`
}

// Empty reports whether m carries nothing (then no record is desired).
func (m DocMeta) Empty() bool {
	return len(m.Policers) == 0 && len(m.Shapers) == 0 && len(m.Maps) == 0 && len(m.Interfaces) == 0
}

// MapByID returns the name and record of the map with VPP id id.
func (m DocMeta) MapByID(id uint32) (string, MapMeta, bool) {
	names := make([]string, 0, len(m.Maps))
	for n := range m.Maps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if m.Maps[n].ID == id {
			return n, m.Maps[n], true
		}
	}
	return "", MapMeta{}, false
}

// Validate checks m: unique map ids, known row sources, byte-sized recorded values.
func (m DocMeta) Validate() error {
	ids := map[uint32]string{}
	for name, mm := range m.Maps {
		if name == "" {
			return df7.Specf("qos meta: empty map name")
		}
		if other, dup := ids[mm.ID]; dup {
			return df7.Specf("qos meta: maps %q and %q share id %d", other, name, mm.ID)
		}
		ids[mm.ID] = name
		for src, froms := range mm.Rows {
			if err := checkSource(src); err != nil {
				return err
			}
			for _, f := range froms {
				if f < 0 || f > 255 {
					return df7.Specf("qos meta: map %q row %s value %d is not a byte", name, src, f)
				}
			}
		}
	}
	return nil
}

// MetaStore persists the record.
type MetaStore interface {
	// Load returns the record, or nil when there is none.
	Load() (*DocMeta, error)
	Save(m DocMeta) error
	Delete() error
}

// MemMetaStore keeps the record in memory (unit tests; the product agent refuses it, CheckPersistent).
type MemMetaStore struct {
	mu sync.Mutex
	m  *DocMeta
}

// Load implements MetaStore.
func (s *MemMetaStore) Load() (*DocMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		return nil, nil
	}
	c := *s.m
	return &c, nil
}

// Save implements MetaStore.
func (s *MemMetaStore) Save(m DocMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = &m
	return nil
}

// Delete implements MetaStore.
func (s *MemMetaStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = nil
	return nil
}

// FileMetaStore persists the record as one JSON file (0600, written to a temporary file, fsynced and renamed).
type FileMetaStore struct {
	Path string
	mu   sync.Mutex
}

// Persistent marks the store as surviving an agent restart (dfkit/persist, TD-11b).
func (*FileMetaStore) Persistent() bool { return true }

// Load implements MetaStore. A missing or unreadable file is no record: the record is re-creatable metadata, and a
// corrupt file must not fail every transaction that touches `services` (the next reconcile rewrites it).
func (s *FileMetaStore) Load() (*DocMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.Path) //nolint:gosec // the agent's own state file
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m DocMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, nil
	}
	return &m, nil
}

// Save implements MetaStore.
func (s *FileMetaStore) Save(m DocMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o750); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the agent's own state file
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(s.Path))
}

// Delete implements MetaStore.
func (s *FileMetaStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDir(filepath.Dir(s.Path))
}

func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // the agent's state dir
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// MetaDescriptor manages the qos.meta record.
type MetaDescriptor struct {
	store MetaStore
}

var _ scheduler.Descriptor = (*MetaDescriptor)(nil)

// NewMeta returns the qos.meta descriptor over store (nil: an in-memory store, unit tests only).
func NewMeta(store MetaStore) *MetaDescriptor {
	if store == nil {
		store = &MemMetaStore{}
	}
	return &MetaDescriptor{store: store}
}

// Name implements scheduler.Descriptor.
func (*MetaDescriptor) Name() string { return NameMeta }

// KeyOf implements scheduler.Descriptor: there is one record.
func (*MetaDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyMeta() }

// Dependencies implements scheduler.Descriptor: none.
func (*MetaDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *MetaDescriptor) save(obj proto.Message) error {
	m, err := df7.DecodeValid[DocMeta](obj)
	if err != nil {
		return err
	}
	if err := d.store.Save(m); err != nil {
		return fmt.Errorf("%s: save: %w", NameMeta, err)
	}
	return nil
}

// Create implements scheduler.Descriptor: write the record.
func (d *MetaDescriptor) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, d.save(obj)
}

// Update implements scheduler.Descriptor: rewrite the record.
func (d *MetaDescriptor) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.save(newObj)
}

// Delete implements scheduler.Descriptor: remove the record.
func (d *MetaDescriptor) Delete(context.Context, proto.Message, any) error {
	if err := d.store.Delete(); err != nil {
		return fmt.Errorf("%s: delete: %w", NameMeta, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: the record file (none → nothing).
func (d *MetaDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	m, err := d.store.Load()
	if err != nil {
		return nil, fmt.Errorf("%s: load: %w", NameMeta, err)
	}
	if m == nil {
		return nil, nil
	}
	return []scheduler.KV{df7.KV(KeyMeta(), *m, nil)}, nil
}

// CheckPersistent: the record must survive an agent restart (it names the egress maps VPP reports by id).
func (d *MetaDescriptor) CheckPersistent() error {
	return persist.Require(NameMeta+": the services.qos document record (pass a FileMetaStore)", d.store)
}

// DocMetaOf returns the record a Retrieve result carries (false when the KVs have none).
func DocMetaOf(kvs []scheduler.KV) (DocMeta, bool) {
	for _, kv := range kvs {
		if kv.Key == KeyMeta() {
			m, err := df7.Decode[DocMeta](kv.Value)
			return m, err == nil
		}
	}
	return DocMeta{}, false
}
