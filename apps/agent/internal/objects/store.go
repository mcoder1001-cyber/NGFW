package objects

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// FlushDelay is how long the store waits after the last change before it writes (review F1): a
// transaction's objects are written once, normally by the scheduler's verification Retrieve, else this
// long after the transaction's last change.
const FlushDelay = 200 * time.Millisecond

// Store is the applied objects document of one owner, persisted in the agent state dir
// (<dir>/objects-<owner>.json, protobuf JSON of ObjectsConfig, 0600, atomic replace). It is the
// actual state of the objects.* descriptors: Create/Update/Delete change it, Retrieve reads it, the
// FQDN resolver follows its FQDN objects. Safe for concurrent use.
//
// It is a derived record of applied state, not a source of truth (review §A): a change updates memory
// only (in place, O(1)) and is persisted coalesced — once per transaction (Retrieve flushes, and the
// scheduler retrieves to verify), else FlushDelay after the last change, and at Close. Losing the last
// unflushed changes in a crash is harmless: Retrieve then shows the older set and the next resync
// re-applies. For the same reason a corrupt file never stops the agent (OpenStore moves it aside).
type Store struct {
	path string
	log  *slog.Logger

	mu    sync.RWMutex
	doc   *vrxv1.ObjectsConfig
	gen   uint64 // bumped by every change
	saved uint64 // gen of the last successful write
	timer *time.Timer
	// onFQDN runs (outside the lock) when an FQDN address object appears, changes its FQDN or goes:
	// host is its FQDN now, "" when it is no FQDN object any more.
	onFQDN func(name, host string)

	wmu sync.Mutex // one file write at a time
}

// OpenStore loads the store at path; a missing file is an empty store. A file that does not parse is
// moved aside to <path>.corrupt-<unix time>, logged as an ERROR and counted
// (vrx_agent_objects_store_corrupt_total); the store starts empty and the next resync re-applies the
// configuration (review F2). Only an unreadable file (an I/O error) is an error.
func OpenStore(path string, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.Default()
	}
	s := &Store{path: path, log: log, doc: &vrxv1.ObjectsConfig{}}
	raw, err := os.ReadFile(path) //nolint:gosec // agent state file in the configured state dir
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("objects store: %w", err)
	}
	if perr := protojson.Unmarshal(raw, s.doc); perr != nil {
		s.doc = &vrxv1.ObjectsConfig{}
		aside := path + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
		if rerr := os.Rename(path, aside); rerr != nil {
			aside = "(not moved: " + rerr.Error() + "; the next write replaces it)"
		}
		storeCorrupt.Add(1)
		log.Error("objects store is corrupt: moved aside, starting empty; the next resync re-applies the configuration",
			"file", path, "moved_to", aside, "err", perr)
	}
	return s, nil
}

// Path is the store's file.
func (s *Store) Path() string { return s.path }

// Snapshot returns a deep copy of the applied objects document (diagnostics and tests).
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

// put stores v (cloned) as kind k's entry name. Memory only; the next flush persists it.
func (s *Store) put(k Kind, name string, v proto.Message) error {
	return s.change(k, name, proto.Clone(v))
}

// remove deletes kind k's entry name (no error if absent). Memory only; the next flush persists it.
func (s *Store) remove(k Kind, name string) error {
	return s.change(k, name, nil)
}

// change sets (v != nil) or deletes one entry in place — no copy of the document — schedules the flush
// and tells the resolver when an FQDN object changed (review F1).
func (s *Store) change(k Kind, name string, v proto.Message) error {
	s.mu.Lock()
	before := fqdnOf(s.doc, k, name)
	if err := setEntry(s.doc, k, name, v); err != nil {
		s.mu.Unlock()
		return err
	}
	after := fqdnOf(s.doc, k, name)
	s.gen++
	if s.timer == nil { // debounced: FlushDelay after the last change of a burst
		s.timer = time.AfterFunc(FlushDelay, s.flushLater)
	} else {
		s.timer.Reset(FlushDelay)
	}
	cb := s.onFQDN
	s.mu.Unlock()
	if cb != nil && before != after {
		cb(name, after)
	}
	return nil
}

// fqdnOf is the FQDN of address object name in doc, "" when it is none (or k is not addresses).
func fqdnOf(doc *vrxv1.ObjectsConfig, k Kind, name string) string {
	if k != KindAddresses {
		return ""
	}
	if a, ok := doc.GetAddresses()[name]; ok && a.GetType() == "fqdn" {
		return a.GetFqdn()
	}
	return ""
}

func (s *Store) flushLater() {
	s.mu.Lock()
	s.timer = nil
	s.mu.Unlock()
	if err := s.Flush(); err != nil {
		s.mu.Lock()
		if s.timer == nil { // retry later; Retrieve serves the changes from memory meanwhile
			s.timer = time.AfterFunc(5*time.Second, s.flushLater)
		}
		s.mu.Unlock()
	}
}

// Dirty reports whether changes are not written yet.
func (s *Store) Dirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gen != s.saved
}

// Flush writes the document if it changed since the last write: one marshal, one atomic write. A write
// error is logged, counted (vrx_agent_objects_store_persist_errors_total) and returned; the changes stay
// in memory and the next flush writes them.
func (s *Store) Flush() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.mu.RLock()
	if s.gen == s.saved {
		s.mu.RUnlock()
		return nil
	}
	gen := s.gen
	raw, err := protojson.MarshalOptions{Multiline: true}.Marshal(s.doc)
	s.mu.RUnlock()
	if err == nil {
		err = atomicWrite(s.path, raw)
	}
	if err != nil {
		storePersistErrors.Add(1)
		s.log.Error("objects store: write failed; the applied objects stay in memory and the next flush writes them", "file", s.path, "err", err)
		return fmt.Errorf("objects store: %w", err)
	}
	s.mu.Lock()
	if gen > s.saved {
		s.saved = gen
	}
	s.mu.Unlock()
	return nil
}

// Close writes pending changes and stops the flush timer.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	return s.Flush()
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
