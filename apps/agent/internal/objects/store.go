package objects

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Store is the applied objects document of one owner, persisted in the agent state dir
// (<dir>/objects-<owner>.json, protobuf JSON of ObjectsConfig, 0600, atomic replace). It is the
// actual state of the objects.* descriptors: Create/Update/Delete write it, Retrieve reads it, the
// FQDN resolver and Snapshot read it. Safe for concurrent use.
type Store struct {
	path string

	mu  sync.RWMutex
	doc *vrxv1.ObjectsConfig
	// onChange runs after every successful write (outside the lock), e.g. the resolver's sync.
	onChange func()
}

// OpenStore loads the store at path; a missing file is an empty store. A file that does not parse
// is an error (fail closed, like the claim stores: the agent must not forget what it applied).
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, doc: &vrxv1.ObjectsConfig{}}
	raw, err := os.ReadFile(path) //nolint:gosec // agent state file in the configured state dir
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("objects store: %w", err)
	}
	if err := protojson.Unmarshal(raw, s.doc); err != nil {
		return nil, fmt.Errorf("objects store %s is corrupt (move it aside; the next resync re-applies the configuration): %w", path, err)
	}
	return s, nil
}

// Path is the store's file.
func (s *Store) Path() string { return s.path }

// Snapshot returns a deep copy of the applied objects document.
func (s *Store) Snapshot() *vrxv1.ObjectsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return proto.Clone(s.doc).(*vrxv1.ObjectsConfig)
}

// entries returns (name, value) of one kind, sorted by name, values cloned.
func (s *Store) entries(k Kind) []namedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := listKind(s.doc, k)
	for i := range out {
		out[i].value = proto.Clone(out[i].value)
	}
	return out
}

type namedValue struct {
	name  string
	value proto.Message
}

// put stores v (cloned) as kind k's entry name and persists; on a write error the previous entry
// is restored and the error returned.
func (s *Store) put(k Kind, name string, v proto.Message) error {
	return s.mutate(func(doc *vrxv1.ObjectsConfig) error { return setEntry(doc, k, name, proto.Clone(v)) })
}

// remove deletes kind k's entry name (no error if absent) and persists.
func (s *Store) remove(k Kind, name string) error {
	return s.mutate(func(doc *vrxv1.ObjectsConfig) error { return setEntry(doc, k, name, nil) })
}

func (s *Store) mutate(f func(*vrxv1.ObjectsConfig) error) error {
	s.mu.Lock()
	next := proto.Clone(s.doc).(*vrxv1.ObjectsConfig)
	if err := f(next); err != nil {
		s.mu.Unlock()
		return err
	}
	raw, err := protojson.MarshalOptions{Multiline: true}.Marshal(next)
	if err == nil {
		err = atomicWrite(s.path, raw)
	}
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("objects store: %w", err)
	}
	s.doc = next
	cb := s.onChange
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
	return nil
}

// listKind returns kind k's entries of doc sorted by name (values not cloned).
func listKind(doc *vrxv1.ObjectsConfig, k Kind) []namedValue {
	var out []namedValue
	add := func(name string, v proto.Message) { out = append(out, namedValue{name, v}) }
	switch k {
	case KindAddresses:
		for n, v := range doc.GetAddresses() {
			add(n, v)
		}
	case KindAddressGroups:
		for n, v := range doc.GetAddressGroups() {
			add(n, v)
		}
	case KindServices:
		for n, v := range doc.GetServices() {
			add(n, v)
		}
	case KindServiceGroups:
		for n, v := range doc.GetServiceGroups() {
			add(n, v)
		}
	case KindSchedules:
		for n, v := range doc.GetSchedules() {
			add(n, v)
		}
	case KindZones:
		for n, v := range doc.GetZones() {
			add(n, v)
		}
	case KindTags:
		for n, v := range doc.GetTags() {
			add(n, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// setEntry sets (v != nil) or deletes (v == nil) kind k's entry name in doc.
func setEntry(doc *vrxv1.ObjectsConfig, k Kind, name string, v proto.Message) error {
	wrong := func() error { return fmt.Errorf("%w: a %T is not a %s entry", ErrInvalid, v, k) }
	switch k {
	case KindAddresses:
		return setMap(&doc.Addresses, name, v, wrong)
	case KindAddressGroups:
		return setMap(&doc.AddressGroups, name, v, wrong)
	case KindServices:
		return setMap(&doc.Services, name, v, wrong)
	case KindServiceGroups:
		return setMap(&doc.ServiceGroups, name, v, wrong)
	case KindSchedules:
		return setMap(&doc.Schedules, name, v, wrong)
	case KindZones:
		return setMap(&doc.Zones, name, v, wrong)
	case KindTags:
		return setMap(&doc.Tags, name, v, wrong)
	}
	return fmt.Errorf("%w: unknown kind %q", ErrInvalid, k)
}

func setMap[T proto.Message](m *map[string]T, name string, v proto.Message, wrong func() error) error {
	if v == nil {
		delete(*m, name)
		return nil
	}
	t, ok := v.(T)
	if !ok {
		return wrong()
	}
	if *m == nil {
		*m = map[string]T{}
	}
	(*m)[name] = t
	return nil
}

// atomicWrite replaces path with raw (0600): temp file in the same dir, fsync, rename, fsync dir.
func atomicWrite(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // no-op after the rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil { //nolint:gosec // the state dir
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
