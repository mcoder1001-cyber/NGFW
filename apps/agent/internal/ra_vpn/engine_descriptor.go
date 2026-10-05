package ravpn

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/scheduler"
)

// EngineDescriptor reconciles independently owned private remote-access engine generations.
type EngineDescriptor struct{ Runtime *Runtime }

// Name identifies the engine descriptor family.
func (*EngineDescriptor) Name() string { return EngineName }

// Stage orders activation after the verified transport dependencies.
func (*EngineDescriptor) Stage() scheduler.Stage { return scheduler.StageDaemon }

// KeyOf returns the full private instance identity.
func (*EngineDescriptor) KeyOf(v proto.Message) scheduler.Key {
	x, _ := v.(*structpb.Struct)
	s, _ := DecodeEngine(x)
	return scheduler.Join(EngineName, s.Instance)
}

// Dependencies requires every transport and policy object before engine activation.
func (*EngineDescriptor) Dependencies(v proto.Message) []scheduler.Dependency {
	x, _ := v.(*structpb.Struct)
	s, e := DecodeEngine(x)
	if e != nil {
		return nil
	}
	kvs, e := TransportObjects(s)
	if e != nil {
		return nil
	}
	out := make([]scheduler.Dependency, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, scheduler.Dependency{Key: kv.Key})
	}
	return out
}

// Create starts a verified isolated engine generation.
func (d *EngineDescriptor) Create(ctx context.Context, v proto.Message) (any, error) {
	x, _ := v.(*structpb.Struct)
	s, e := DecodeEngine(x)
	if e != nil {
		return nil, ErrEngine
	}
	record, e := d.Runtime.Create(ctx, s)
	if e != nil {
		if record.Spec.Instance == "" {
			return nil, ErrEngine
		}
		return record, scheduler.PartialCreate(ErrEngine)
	}
	return record, nil
}

// Update requests replacement of an immutable engine generation.
func (*EngineDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete stops the verified owned generation before cleanup.
func (d *EngineDescriptor) Delete(ctx context.Context, v proto.Message, _ any) error {
	x, _ := v.(*structpb.Struct)
	s, e := DecodeEngine(x)
	if e != nil {
		return ErrEngine
	}
	return d.Runtime.Delete(ctx, s)
}

// Retrieve recovers and revalidates observed engine generations.
func (d *EngineDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if d.Runtime == nil {
		return nil, ErrEngine
	}
	d.Runtime.mu.Lock()
	configured := d.Runtime.configured()
	if !configured {
		records, e := d.Runtime.store.List()
		d.Runtime.mu.Unlock()
		if e != nil || len(records) != 0 {
			return nil, ErrEngine
		}
		return nil, nil
	}
	d.Runtime.mu.Unlock()
	if e := d.Runtime.Recover(ctx); e != nil {
		return nil, e
	}
	records, e := d.Runtime.Records(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]scheduler.KV, 0, len(records))
	for _, r := range records {
		v, e := r.Spec.Proto()
		if e != nil {
			return nil, e
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(EngineName, r.Spec.Instance), Value: v, Meta: r})
	}
	return out, nil
}

// FileEngineStore holds references and process identities only. Parent/file
// descriptors are pinned with NOFOLLOW; linked or writable records fail closed.
type FileEngineStore struct {
	fd    int
	owner string
}

// NewFileEngineStore opens a root-protected no-follow store for one logical owner.
func NewFileEngineStore(stateDir, owner string) (*FileEngineStore, error) {
	if !safeOwnerName(owner) || !filepath.IsAbs(stateDir) || filepath.Clean(stateDir) != stateDir {
		return nil, ErrEngine
	}
	parent, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e == nil {
		for _, part := range splitPath(stateDir) {
			next, err := unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			_ = unix.Close(parent)
			if err != nil {
				return nil, ErrEngine
			}
			parent = next
		}
	}
	if e != nil {
		return nil, ErrEngine
	}
	defer func() { _ = unix.Close(parent) }() // Read-only descriptor; no buffered writes to commit.
	var st unix.Stat_t
	if unix.Fstat(parent, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 {
		return nil, ErrEngine
	}
	if e := unix.Mkdirat(parent, "ra-engine", 0700); e != nil && e != unix.EEXIST {
		return nil, ErrEngine
	}
	fd, e := unix.Openat(parent, "ra-engine", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrEngine
	}
	if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0077 != 0 {
		_ = unix.Close(fd)
		return nil, ErrEngine
	}
	return &FileEngineStore{fd: fd, owner: owner}, nil
}

// Close releases the protected store directory descriptor.
func (s *FileEngineStore) Close() error { return unix.Close(s.fd) }
func (s *FileEngineStore) read(name string) (EngineRecord, error) {
	var r EngineRecord
	if !strings.HasSuffix(name, ".json") || !ValidInstance(strings.TrimSuffix(name, ".json")) {
		return r, ErrEngine
	}
	fd, e := unix.Openat(s.fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return r, ErrEngine
	}
	f := os.NewFile(uintptr(fd), name)
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Nlink != 1 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Size > (MaxEngineSpecBytes+65536) {
		return r, ErrEngine
	}
	b, e := io.ReadAll(io.LimitReader(f, (MaxEngineSpecBytes + 65537)))
	if e != nil || len(b) > (MaxEngineSpecBytes+65536) || json.Unmarshal(b, &r) != nil || r.Spec.Validate() != nil || r.Spec.Owner != s.owner || r.Spec.Instance+".json" != name || !r.Unit.Valid() {
		return EngineRecord{}, ErrEngine
	}
	return r, nil
}

// List reads bounded authenticated public generation records.
func (s *FileEngineStore) List() ([]EngineRecord, error) {
	fd, e := unix.Openat(s.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrEngine
	}
	f := os.NewFile(uintptr(fd), "ra-engine")
	defer func() { _ = f.Close() }()
	names, e := f.Readdirnames(65)
	if e != nil && e != io.EOF {
		return nil, ErrEngine
	}
	if len(names) > 64 {
		return nil, ErrEngine
	}
	sort.Strings(names)
	out := make([]EngineRecord, 0, len(names))
	for _, name := range names {
		r, e := s.read(name)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}

// Save atomically persists one verified public generation record.
func (s *FileEngineStore) Save(r EngineRecord) error {
	if r.Spec.Validate() != nil || r.Spec.Owner != s.owner || !r.Unit.Valid() {
		return ErrEngine
	}
	name := r.Spec.Instance + ".json"
	b, e := json.Marshal(r)
	if e != nil || len(b) > (MaxEngineSpecBytes+65536) {
		return ErrEngine
	}
	var before unix.Stat_t
	existed := unix.Fstatat(s.fd, name, &before, unix.AT_SYMLINK_NOFOLLOW) == nil
	if existed {
		old, e := s.read(name)
		if e != nil || old.Unit != r.Unit || old.Spec.Instance != r.Spec.Instance {
			return ErrEngine
		}
	}
	temp := r.Spec.Instance + ".pending"
	fd, e := unix.Openat(s.fd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrEngine
	}
	f := os.NewFile(uintptr(fd), temp)
	n, e := f.Write(b)
	if e == nil && n == len(b) {
		e = f.Sync()
	} else {
		e = ErrEngine
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return ErrEngine
	}
	var after unix.Stat_t
	lookup := unix.Fstatat(s.fd, name, &after, unix.AT_SYMLINK_NOFOLLOW)
	if existed && (lookup != nil || before.Ino != after.Ino || before.Dev != after.Dev) || !existed && lookup != unix.ENOENT {
		return ErrEngine
	}
	if unix.Renameat(s.fd, temp, s.fd, name) != nil || unix.Fsync(s.fd) != nil {
		return ErrEngine
	}
	return nil
}

// Remove unlinks only the authenticated owned record.
func (s *FileEngineStore) Remove(instance string) error {
	if !ValidInstance(instance) {
		return ErrEngine
	}
	if _, e := s.read(instance + ".json"); e != nil {
		if e2 := unix.Faccessat(s.fd, instance+".json", unix.F_OK, unix.AT_SYMLINK_NOFOLLOW); e2 == unix.ENOENT {
			if e3 := unix.Faccessat(s.fd, instance+".pending", unix.F_OK, unix.AT_SYMLINK_NOFOLLOW); e3 != unix.ENOENT {
				return ErrEngine
			}
			return nil
		}
		return ErrEngine
	}
	if unix.Unlinkat(s.fd, instance+".json", 0) != nil || unix.Fsync(s.fd) != nil {
		return ErrEngine
	}
	return nil
}

// LazyEngineStore opens the protected record store only when it is needed.
type LazyEngineStore struct {
	StateDir, Owner string
	store           *FileEngineStore
}

func (s *LazyEngineStore) open() error {
	if s.store != nil {
		return nil
	}
	store, e := NewFileEngineStore(s.StateDir, s.Owner)
	if e != nil {
		return e
	}
	s.store = store
	return nil
}

// List reads bounded authenticated public generation records.
func (s *LazyEngineStore) List() ([]EngineRecord, error) {
	if s.StateDir == "" {
		return nil, nil
	}
	if _, e := os.Lstat(filepath.Join(s.StateDir, "ra-engine")); os.IsNotExist(e) {
		return nil, nil
	}
	if e := s.open(); e != nil {
		return nil, e
	}
	return s.store.List()
}

// Save atomically persists one verified public generation record.
func (s *LazyEngineStore) Save(r EngineRecord) error {
	if e := s.open(); e != nil {
		return e
	}
	return s.store.Save(r)
}

// Remove unlinks only the authenticated owned record.
func (s *LazyEngineStore) Remove(id string) error {
	if s.store == nil {
		if _, e := os.Lstat(filepath.Join(s.StateDir, "ra-engine")); os.IsNotExist(e) {
			return nil
		}
	}
	if e := s.open(); e != nil {
		return e
	}
	return s.store.Remove(id)
}

// Close releases the protected store directory descriptor.
func (s *LazyEngineStore) Close() {
	if s.store != nil {
		_ = s.store.Close()
		s.store = nil
	}
}

// Persistent reports whether the store has a protected durable location.
func (s *LazyEngineStore) Persistent() bool {
	return filepath.IsAbs(s.StateDir) && safeOwnerName(s.Owner)
}

// CheckPersistent requires durable protected generation records.
func (d *EngineDescriptor) CheckPersistent() error {
	if d.Runtime == nil {
		return ErrEngine
	}
	if p, ok := d.Runtime.store.(interface{ Persistent() bool }); !ok || !p.Persistent() {
		return ErrEngine
	}
	return nil
}

// Validate checks transport dependencies and sealed credentials before mutation.
func (d *EngineDescriptor) Validate(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error {
	x, _ := value.(*structpb.Struct)
	spec, e := DecodeEngine(x)
	if e != nil || d.Runtime == nil || spec.Owner != d.Runtime.owner || key != scheduler.Join(EngineName, spec.Instance) || view == nil {
		return ErrEngine
	}
	objects, e := TransportObjects(spec)
	if e != nil {
		return ErrEngine
	}
	for _, kv := range objects {
		actual, exists := view.Get(kv.Key)
		if !exists || !proto.Equal(kv.Value, actual) {
			return ErrEngine
		}
	}
	d.Runtime.mu.Lock()
	defer d.Runtime.mu.Unlock()
	validator, ok := d.Runtime.preparation.(SnapshotValidation)
	if !ok {
		return ErrEngine
	}
	if validator.Validate(ctx, spec) != nil {
		return ErrEngine
	}
	return d.Runtime.validateActivation(ctx, spec)
}

// Preflight verifies the durable store location without creating any directory.
func (s *LazyEngineStore) Preflight(ctx context.Context) error {
	if ctx.Err() != nil || !s.Persistent() || filepath.Clean(s.StateDir) != s.StateDir {
		return ErrEngine
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrEngine
	}
	for _, part := range splitPath(s.StateDir) {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if e != nil {
			return ErrEngine
		}
		fd = next
	}
	defer func() { _ = unix.Close(fd) }() // Read-only descriptor; no buffered writes to commit.
	var metadata unix.Stat_t
	var filesystem unix.Statfs_t
	if unix.Fstat(fd, &metadata) != nil || metadata.Uid != 0 || metadata.Mode&0022 != 0 || unix.Fstatfs(fd, &filesystem) != nil || filesystem.Flags&unix.ST_RDONLY != 0 {
		return ErrEngine
	}
	child, e := unix.Openat(fd, "ra-engine", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e == unix.ENOENT {
		return nil
	}
	if e != nil {
		return ErrEngine
	}
	defer func() { _ = unix.Close(child) }() // Read-only descriptor; no buffered writes to commit.
	if unix.Fstat(child, &metadata) != nil || metadata.Uid != 0 || metadata.Mode&0077 != 0 {
		return ErrEngine
	}
	return nil
}
