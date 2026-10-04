package pki

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"log/slog"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"os"
	"sort"
)

// rollbackFiles keeps only the previous disk generation. Secret bytes never enter
// scheduler metadata; reverse updates work even while the candidate cache is active.
type rollbackFiles struct {
	old   *ngfwv1.PkiFileStateSet
	files map[string]diskFile
}
type diskFile struct {
	content []byte
	mode    os.FileMode
	present bool
}

func (r *rollbackFiles) GoString() string     { return r.String() }
func (r *rollbackFiles) LogValue() slog.Value { return slog.StringValue(r.String()) }
func (*rollbackFiles) String() string         { return "pki rollback files (redacted)" }
func (r *rollbackFiles) clear() {
	if r != nil {
		for _, f := range r.files {
			clear(f.content)
		}
		r.files = nil
	}
}
func (m *Materialiser) checkpoint(paths []string) (*rollbackFiles, error) {
	old, err := m.Retrieve(context.Background())
	if err != nil {
		return nil, err
	}
	r := &rollbackFiles{old: proto.Clone(old).(*ngfwv1.PkiFileStateSet), files: map[string]diskFile{}}
	total := 0
	for _, path := range append(paths, m.manifest.Path()) {
		if _, exists := r.files[path]; exists {
			continue
		}
		content, mode, err := readBounded(path, MaxCRLSize)
		if errors.Is(err, os.ErrNotExist) {
			r.files[path] = diskFile{}
			continue
		}
		if err != nil {
			r.clear()
			return nil, errors.New("pki: cannot checkpoint owned file")
		}
		total += len(content)
		if total > 8<<20 {
			clear(content)
			r.clear()
			return nil, errors.New("pki: rollback checkpoint exceeds bounded capacity")
		}
		r.files[path] = diskFile{content: content, mode: mode, present: true}
	}
	return r, nil
}
func (m *Materialiser) restoreFiles(r *rollbackFiles) error {
	var paths []string
	for path := range r.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		f := r.files[path]
		if !f.present {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("pki: rollback remove failed")
			}
			continue
		}
		if err := renderers.WriteFileAtomic(path, renderers.File{Content: f.content, Mode: f.mode, Owner: m.owner, Secret: f.mode&0o077 == 0}); err != nil {
			return errors.New("pki: rollback restore failed")
		}
	}
	return nil
}

func (m *Materialiser) restoreCheckpoint() error {
	if err := m.restoreFiles(m.rollback); err != nil {
		return err
	}
	m.rollback.clear()
	m.rollback = nil
	return nil
}

// Close clears the bounded previous generation retained for transaction rollback.
func (m *Materialiser) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollback.clear()
	m.rollback = nil
}
