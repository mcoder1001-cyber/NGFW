package wireguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// MetaName is the descriptor of the agent-local WireGuard metadata (F-wireguard).
const MetaName = "wireguard.meta"

// MetaSpec is what the configuration says about a WireGuard interface or peer that VPP cannot hold
// (D-073b style): the configuration key, the description, the D-051 secret reference the key
// material was resolved from, and (interfaces only) the underlay VRF name and the routeAllowedIps
// flag. Never key material: SecretRef is a "<kind>/<name>" handle.
//
// ID is "wg<instance>" for an interface and "wg<instance>/<public key>" for a peer — the ids of the
// wireguard.interface and wireguard.peer keys the entry describes.
type MetaSpec struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	SecretRef       string `json:"secret_ref"`
	UnderlayVrf     string `json:"underlay_vrf"`
	RouteAllowedIps bool   `json:"route_allowed_ips"`
}

// MetaKey returns wireguard.meta/<id>.
func MetaKey(id string) scheduler.Key { return scheduler.Join(MetaName, id) }

// MetaValue encodes a spec as the descriptor's value.
func MetaValue(s MetaSpec) proto.Message { return dfkit.Encode(s) }

// DecodeMeta decodes a wireguard.meta value.
func DecodeMeta(v proto.Message) (MetaSpec, error) {
	var s MetaSpec
	err := dfkit.Decode(v, &s)
	return s, err
}

// MetaStore persists the metadata table. Load returns a copy.
type MetaStore interface {
	Load() (map[string]MetaSpec, error)
	Save(map[string]MetaSpec) error
}

// Meta is the wireguard.meta descriptor: an agent-local table, no VPP call. It takes part in
// transactions like any descriptor (journaled, rolled back, verified), so Retrieve can name the
// objects VPP reports with the configuration's names only after a transaction that applied them;
// DryRun never writes it. Create and Update store the entry, Delete removes it, Retrieve returns
// the whole table (the assembler joins it with the VPP objects that exist).
type Meta struct{ store MetaStore }

// NewMeta returns the descriptor over store (NewMemoryMetaStore when nil).
func NewMeta(store MetaStore) *Meta {
	if store == nil {
		store = NewMemoryMetaStore()
	}
	return &Meta{store: store}
}

// Name implements scheduler.Descriptor.
func (*Meta) Name() string { return MetaName }

// CheckPersistent declares the TD-11b protocol (dfkit/persist): the table must survive an agent
// restart in the product agent (FileMetaStore), or Retrieve would lose the configuration's names.
func (d *Meta) CheckPersistent() error {
	if p, ok := d.store.(interface{ Persistent() bool }); ok && p.Persistent() {
		return nil
	}
	return fmt.Errorf("%s: metadata store %T does not survive an agent restart (in memory)", MetaName, d.store)
}

// KeyOf implements scheduler.Descriptor.
func (*Meta) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := DecodeMeta(obj)
	return MetaKey(s.ID)
}

// Dependencies implements scheduler.Descriptor (none: the table is agent-local).
func (*Meta) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *Meta) put(obj proto.Message) error {
	s, err := DecodeMeta(obj)
	if err != nil {
		return err
	}
	if s.ID == "" || s.Name == "" {
		return dfkit.Specf("%s: id and name are required", MetaName)
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	m[s.ID] = s
	return d.store.Save(m)
}

// Create implements scheduler.Descriptor.
func (d *Meta) Create(_ context.Context, obj proto.Message) (any, error) { return nil, d.put(obj) }

// Update implements scheduler.Descriptor.
func (d *Meta) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.put(newObj)
}

// Delete implements scheduler.Descriptor.
func (d *Meta) Delete(_ context.Context, obj proto.Message, _ any) error {
	s, err := DecodeMeta(obj)
	if err != nil {
		return err
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	if _, ok := m[s.ID]; !ok {
		return nil
	}
	delete(m, s.ID)
	return d.store.Save(m)
}

// Retrieve implements scheduler.Descriptor.
func (d *Meta) Retrieve(context.Context) ([]scheduler.KV, error) {
	m, err := d.store.Load()
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(m))
	for _, s := range m {
		out = append(out, scheduler.KV{Key: MetaKey(s.ID), Value: MetaValue(s)})
	}
	return sortKVs(out), nil
}

// ---- stores ------------------------------------------------------------------------------------

type memMeta struct {
	mu sync.Mutex
	m  map[string]MetaSpec
}

// NewMemoryMetaStore returns an in-memory store (tests; an agent restart forgets the names).
func NewMemoryMetaStore() MetaStore { return &memMeta{m: map[string]MetaSpec{}} }

func (s *memMeta) Load() (map[string]MetaSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]MetaSpec, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out, nil
}

func (s *memMeta) Save(m map[string]MetaSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[string]MetaSpec, len(m))
	for k, v := range m {
		s.m[k] = v
	}
	return nil
}

// FileMetaStore keeps the table in one JSON file (0600, in the agent's state dir), written
// crash-safe: temporary file in the same directory, fsync, rename, directory fsync.
type FileMetaStore struct {
	mu   sync.Mutex
	path string
}

// NewFileMetaStore returns the store at path; a missing file is an empty table.
func NewFileMetaStore(path string) (*FileMetaStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("%s store: %w", MetaName, err)
	}
	s := &FileMetaStore{path: path}
	if _, err := s.Load(); err != nil {
		return nil, err
	}
	return s, nil
}

type metaFile struct {
	Version int        `json:"version"`
	Entries []MetaSpec `json:"entries"`
}

// Persistent reports that the store survives an agent restart (TD-11b dfkit/persist.Store).
func (*FileMetaStore) Persistent() bool { return true }

// Load implements MetaStore.
func (s *FileMetaStore) Load() (map[string]MetaSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]MetaSpec{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s store: %w", MetaName, err)
	}
	var f metaFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s store %s: %w", MetaName, s.path, err)
	}
	m := make(map[string]MetaSpec, len(f.Entries))
	for _, e := range f.Entries {
		m[e.ID] = e
	}
	return m, nil
}

// Save implements MetaStore.
func (s *FileMetaStore) Save(m map[string]MetaSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := metaFile{Version: 1, Entries: make([]MetaSpec, 0, len(m))}
	for _, e := range m {
		f.Entries = append(f.Entries, e)
	}
	sort.Slice(f.Entries, func(i, j int) bool { return f.Entries[i].ID < f.Entries[j].ID })
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "."+strings.TrimSuffix(filepath.Base(s.path), ".json")+"-*")
	if err != nil {
		return fmt.Errorf("%s store: %w", MetaName, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err == nil {
		if _, err = tmp.Write(append(raw, '\n')); err == nil {
			err = tmp.Sync()
		}
		if err != nil {
			_ = tmp.Close()
			return fmt.Errorf("%s store: %w", MetaName, err)
		}
	} else {
		_ = tmp.Close()
		return fmt.Errorf("%s store: %w", MetaName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s store: %w", MetaName, err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("%s store: %w", MetaName, err)
	}
	d, err := os.Open(dir) //nolint:gosec // the agent's state dir
	if err != nil {
		return fmt.Errorf("%s store: %w", MetaName, err)
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
