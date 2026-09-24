// Package ownertable is the agent's persistent record of objects it owns that VPP cannot tag
// (static routes and anything else without a tag/name field; docs/contracts/proto.md §6, D-030).
// Descriptors add a key BEFORE creating the object and remove it AFTER deleting it, so a crash in
// between leaves at most a stale entry — Retrieve only reports entries that also exist in VPP, and
// the next transaction deletes owned leftovers. One file per owner in the agent's state dir:
// "<dir>/owned-<owner>.json" (atomic replace on every change).
package ownertable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Set is the ownership record the descriptors use.
type Set interface {
	Has(key string) bool
	Add(key string) error
	Remove(key string) error
	// Keys returns every owned key with the given prefix, sorted.
	Keys(prefix string) []string
}

// Memory is an in-memory Set (unit tests).
type Memory struct {
	mu   sync.Mutex
	keys map[string]bool
}

// NewMemory returns an empty in-memory set.
func NewMemory() *Memory { return &Memory{keys: map[string]bool{}} }

// Has implements Set.
func (m *Memory) Has(k string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[k]
}

// Add implements Set.
func (m *Memory) Add(k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[k] = true
	return nil
}

// Remove implements Set.
func (m *Memory) Remove(k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, k)
	return nil
}

// Keys implements Set.
func (m *Memory) Keys(prefix string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return filterSorted(m.keys, prefix)
}

func filterSorted(keys map[string]bool, prefix string) []string {
	var out []string
	for k := range keys {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// File is a Set persisted as a JSON array in one file.
type File struct {
	mu   sync.Mutex
	path string
	keys map[string]bool
}

type fileFormat struct {
	Owner string   `json:"owner"`
	Keys  []string `json:"keys"`
}

// Open loads (or starts) the owner table of owner in dir.
func Open(dir, owner string) (*File, error) {
	if owner == "" || strings.ContainsAny(owner, "/\\:") {
		return nil, fmt.Errorf("ownertable: invalid owner %q", owner)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ownertable: %w", err)
	}
	f := &File{path: filepath.Join(dir, "owned-"+owner+".json"), keys: map[string]bool{}}
	b, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return f, nil
	case err != nil:
		return nil, fmt.Errorf("ownertable: %w", err)
	}
	var ff fileFormat
	if err := json.Unmarshal(b, &ff); err != nil {
		return nil, fmt.Errorf("ownertable: %s: %w", f.path, err)
	}
	if ff.Owner != owner {
		return nil, fmt.Errorf("ownertable: %s belongs to owner %q, not %q", f.path, ff.Owner, owner)
	}
	for _, k := range ff.Keys {
		f.keys[k] = true
	}
	return f, nil
}

// Path returns the file path.
func (f *File) Path() string { return f.path }

// Has implements Set.
func (f *File) Has(k string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keys[k]
}

// Add implements Set.
func (f *File) Add(k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys[k] {
		return nil
	}
	f.keys[k] = true
	if err := f.save(); err != nil {
		delete(f.keys, k)
		return err
	}
	return nil
}

// Remove implements Set.
func (f *File) Remove(k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.keys[k] {
		return nil
	}
	delete(f.keys, k)
	if err := f.save(); err != nil {
		f.keys[k] = true
		return err
	}
	return nil
}

// Keys implements Set.
func (f *File) Keys(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return filterSorted(f.keys, prefix)
}

func (f *File) save() error {
	owner := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f.path), "owned-"), ".json")
	b, err := json.MarshalIndent(fileFormat{Owner: owner, Keys: filterSorted(f.keys, "")}, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(f.path, append(b, '\n'), 0o640)
}

// SyncDir fsyncs the directory dir: a rename in it is durable only once the directory is synced
// (TD-9, review 1.5a — after a power loss the file could otherwise come back with its old content).
// The shared helper for every atomic replace of an agent state file.
func SyncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // the caller's own state directory
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("fsync %s: %w", dir, err)
	}
	return d.Close()
}

// syncDir is SyncDir (a var for the tests).
var syncDir = SyncDir

// SetSyncDir replaces the directory fsync of WriteAtomic (tests of other packages that write through
// it, e.g. dfkit's boot store) and returns a function restoring the previous one.
func SetSyncDir(f func(dir string) error) (restore func()) {
	prev := syncDir
	syncDir = f
	return func() { syncDir = prev }
}

// WriteAtomic writes data to path via a temporary file in the same directory: write, fsync, rename,
// then fsync the directory.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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
	return syncDir(filepath.Dir(path))
}
