package pki

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/renderers"
)

// Source returns the material behind a D-051 reference ("cert/<name>", "key/<name>"): the agent's secret channel
// (PENDING-secret-channel). The same shape as vpn.Resolver and strongswan.SecretResolver. An absent reference is an
// error wrapping ErrNotFound (or vpn.ErrSecretNotFound). Implementations must never log material and must be opaque to
// fmt/slog (see MapSource).
type Source interface {
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

// Errors of the materialiser. None carries material.
var (
	// ErrNoSource: the agent has no secret material (the API→agent secret channel is PENDING-secret-channel).
	ErrNoSource = errors.New("PKI secret resolver is unavailable")
	// ErrNotFound: the source has no material for the reference.
	ErrNotFound = errors.New("pki: secret not found")
	// ErrChanged: the material changed between validation (Plan) and Apply.
	ErrChanged = errors.New("pki: material changed since it was validated")
)

// File is one file of the plan: its kind and name decide the path (<root>/<kind dir>/<name>.pem), Ref its content.
type File struct {
	Kind Kind
	Name string
	Ref  string
	// Optional files (CRLs) whose material the source does not have are left out of the plan, not refused.
	Optional bool
}

func (f File) String() string { return fmt.Sprintf("%s/%s.pem (%s)", f.Kind.Dir(), f.Name, f.Ref) }

// FileError is the Plan finding for one file (Index into the planned files).
type FileError struct {
	Index int
	File  File
	Err   error
}

func (e *FileError) Error() string { return fmt.Sprintf("%s: %v", e.File, e.Err) }
func (e *FileError) Unwrap() error { return e.Err }

// Config configures a Materialiser.
type Config struct {
	// Root is the swanctl directory: files go to <Root>/{x509,x509ca,private,x509crl}/<name>.pem.
	Root string
	// FileOwner is "user[:group]" of the written files ("root:root" in the product, "" in tests).
	FileOwner string
	// Manifest is the path of the manifest of written files (in the agent's state dir).
	Manifest string
	// Keyer fingerprints private keys (HMAC-SHA256, D-096). Required.
	Keyer *vpn.Keyer
	// Source resolves references; nil = no secret channel (Plan refuses with ErrNoSource).
	Source Source
	// Log receives one line per apply (names and counts only).
	Log *slog.Logger
}

// Materialiser writes, removes and reports the PKI files of one agent (see the package comment). Plan and Retrieve
// may run concurrently with each other; Apply is serialised.
type Materialiser struct {
	root, owner string
	keyer       *vpn.Keyer
	src         Source
	manifest    *Manifest
	log         *slog.Logger
	mu          sync.Mutex
	rollback    *rollbackFiles
}

// New returns a materialiser for cfg.
func New(cfg Config) (*Materialiser, error) {
	if !filepath.IsAbs(cfg.Root) || filepath.Clean(cfg.Root) != cfg.Root {
		return nil, fmt.Errorf("pki: root %q must be an absolute, clean path", cfg.Root)
	}
	if cfg.Keyer == nil {
		return nil, vpn.ErrNoKeyer
	}
	man, err := OpenManifest(cfg.Manifest)
	if err != nil {
		return nil, err
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Materialiser{root: cfg.Root, owner: cfg.FileOwner, keyer: cfg.Keyer, src: cfg.Source, manifest: man,
		log: cfg.Log.With("component", "pki")}, nil
}

// Root returns the swanctl directory.
func (m *Materialiser) Root() string { return m.root }

// HasSource reports whether the materialiser can resolve references.
func (m *Materialiser) HasSource() bool { return m.src != nil }

// Manifest returns the manifest store (the descriptor's CheckPersistent).
func (m *Materialiser) Manifest() *Manifest { return m.manifest }

// Path returns where a file of kind and name lives.
func (m *Materialiser) Path(kind Kind, name string) string {
	return filepath.Join(m.root, kind.Dir(), name+".pem")
}

// fingerprint is the reported identity of content: sha256 of public material, HMAC-SHA256 of a private key.
func (m *Materialiser) fingerprint(kind Kind, content []byte) string {
	if kind == KindKey {
		return m.keyer.Ref(content)
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// resolved is one file with its validated material.
type resolved struct {
	file File
	c    *checked
}

func (r *resolved) wipe() {
	if r.c != nil {
		zero(r.c.content)
	}
}

// load resolves and validates files: per-file findings, the resolved files (absent optional ones are left out),
// cross-checks (a key matches its certificate, a CRL is signed by its CA). The caller wipes the result.
func (m *Materialiser) load(ctx context.Context, files []File) ([]*resolved, []*FileError) {
	var errs []*FileError
	fail := func(i int, f File, err error) { errs = append(errs, &FileError{Index: i, File: f, Err: err}) }
	seen := map[string]bool{}
	var out []*resolved
	for i, f := range files {
		switch {
		case !f.Kind.Valid():
			fail(i, f, fmt.Errorf("unknown kind %q", f.Kind))
			continue
		case !nameRe.MatchString(f.Name):
			fail(i, f, fmt.Errorf("%q is not a vpn.pki object name", clip(f.Name)))
			continue
		case !refRe.MatchString(f.Ref):
			fail(i, File{Kind: f.Kind, Name: f.Name, Ref: "<redacted>"}, errors.New("not a cert/<name> or key/<name> reference (the value is not shown)"))
			continue
		case seen[string(f.Kind)+"/"+f.Name]:
			fail(i, f, errors.New("listed twice"))
			continue
		}
		seen[string(f.Kind)+"/"+f.Name] = true
		if m.src == nil {
			if !f.Optional {
				fail(i, f, ErrNoSource)
			}
			continue
		}
		data, err := m.src.Resolve(ctx, f.Ref)
		if err != nil {
			zero(data)
			if f.Optional && (errors.Is(err, ErrNotFound) || errors.Is(err, vpn.ErrSecretNotFound)) {
				continue
			}
			fail(i, f, ErrNotFound)
			continue
		}
		c, err := check(f.Kind, data)
		zero(data)
		if err != nil {
			fail(i, f, err)
			continue
		}
		out = append(out, &resolved{file: f, c: c})
	}
	certs, cas := map[string]*x509.Certificate{}, map[string]*x509.Certificate{}
	for _, r := range out {
		switch r.file.Kind {
		case KindCert:
			certs[r.file.Name] = r.c.leaf
		case KindCA:
			cas[r.file.Name] = r.c.leaf
		}
	}
	for _, r := range out {
		idx := slices.IndexFunc(files, func(f File) bool { return f.Kind == r.file.Kind && f.Name == r.file.Name })
		switch r.file.Kind {
		case KindKey:
			leaf, ok := certs[r.file.Name]
			switch {
			case !ok && !hasFile(files, KindCert, r.file.Name):
				fail(idx, r.file, fmt.Errorf("private key without its certificate x509/%s.pem", r.file.Name))
			case ok && !keyMatches(r.c.pub, leaf):
				fail(idx, r.file, fmt.Errorf("the private key does not match certificate %s", r.file.Name))
			}
		case KindCRL:
			if ca, ok := cas[r.file.Name]; ok {
				if err := r.c.crl.CheckSignatureFrom(ca); err != nil {
					fail(idx, r.file, fmt.Errorf("%w: the CRL is not signed by CA %s", ErrMaterial, r.file.Name))
				}
			}
		}
	}
	slices.SortFunc(errs, func(a, b *FileError) int { return a.Index - b.Index })
	return out, errs
}

func hasFile(files []File, kind Kind, name string) bool {
	return slices.ContainsFunc(files, func(f File) bool { return f.Kind == kind && f.Name == name })
}

// Plan resolves and validates files and returns the value of the pki.files object: one entry per file to have on
// disk (kind, name, ref, fingerprint, mode), sorted. Read only. Any finding fails the plan (the set is then nil).
func (m *Materialiser) Plan(ctx context.Context, files []File) (*ngfwv1.PkiFileStateSet, []*FileError) {
	res, errs := m.load(ctx, files)
	defer func() {
		for _, r := range res {
			r.wipe()
		}
	}()
	if len(errs) > 0 {
		return nil, errs
	}
	set := &ngfwv1.PkiFileStateSet{}
	for _, r := range res {
		set.Files = append(set.Files, &ngfwv1.PkiFileStateFile{Kind: string(r.file.Kind), Name: r.file.Name, Ref: r.file.Ref,
			Fingerprint: m.fingerprint(r.file.Kind, r.c.content), Mode: uint32(r.file.Kind.Mode())})
	}
	sortFiles(set.Files)
	return set, nil
}

// sortFiles orders files by kind (Kinds order), then name.
func sortFiles(fs []*ngfwv1.PkiFileStateFile) {
	slices.SortFunc(fs, func(a, b *ngfwv1.PkiFileStateFile) int {
		if d := Kind(a.GetKind()).rank() - Kind(b.GetKind()).rank(); d != 0 {
			return d
		}
		return strings.Compare(a.GetName(), b.GetName())
	})
}

// Apply makes the disk hold exactly set: every file written atomically with its mode and owner (material resolved
// again and checked against the planned fingerprint), every file of the manifest that set no longer lists removed, the
// manifest saved. On any failure the previous files and manifest are restored. An empty set removes every file.
func (m *Materialiser) Apply(ctx context.Context, set *ngfwv1.PkiFileStateSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rollback != nil && proto.Equal(set, m.rollback.old) {
		return m.restoreCheckpoint()
	}
	files := make([]File, 0, len(set.GetFiles()))
	for _, f := range set.GetFiles() {
		k := Kind(f.GetKind())
		if k.Valid() && f.GetMode() != uint32(k.Mode()) {
			return fmt.Errorf("pki: %s/%s.pem: mode %#o, want %#o", k.Dir(), f.GetName(), f.GetMode(), uint32(k.Mode()))
		}
		files = append(files, File{Kind: k, Name: f.GetName(), Ref: f.GetRef()})
	}
	res, errs := m.load(ctx, files)
	defer func() {
		for _, r := range res {
			r.wipe()
		}
	}()
	if len(errs) > 0 {
		return fmt.Errorf("pki: %w", errs[0])
	}
	if len(res) != len(files) {
		return fmt.Errorf("pki: %d of %d files have no material", len(files)-len(res), len(files))
	}
	for i, r := range res {
		if got, want := m.fingerprint(r.file.Kind, r.c.content), set.GetFiles()[i].GetFingerprint(); got != want {
			return fmt.Errorf("%w: %s", ErrChanged, r.file)
		}
	}
	old, err := m.manifest.Load()
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	var paths []string
	for _, r := range res {
		p := m.Path(r.file.Kind, r.file.Name)
		keep[p] = true
		paths = append(paths, p)
	}
	var stale []string
	for _, e := range old {
		if p := m.Path(e.Kind, e.Name); !keep[p] {
			stale = append(stale, p)
		}
	}
	for _, k := range Kinds {
		if err := m.ensureDir(k); err != nil {
			return err
		}
	}
	for _, path := range append(append([]string{}, paths...), stale...) {
		st, err := os.Lstat(path)
		if err == nil && !st.Mode().IsRegular() {
			return errors.New("pki: target must be a regular file")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("pki: target cannot be inspected")
		}
	}
	checkpoint, err := m.checkpoint(append(append([]string{}, paths...), stale...))
	if err != nil {
		return err
	}
	restore := func(cause error) error {
		if err := m.restoreFiles(checkpoint); err != nil {
			return errors.Join(cause, err)
		}
		return cause
	}
	success := false
	defer func() {
		if !success {
			checkpoint.clear()
		}
	}()
	for _, r := range res {
		f := renderers.File{Mode: r.file.Kind.Mode(), Owner: m.owner, Content: r.c.content, Secret: r.file.Kind == KindKey}
		if err := renderers.WriteFileAtomic(m.Path(r.file.Kind, r.file.Name), f); err != nil {
			return restore(fmt.Errorf("pki: write %s: %w", r.file, err))
		}
	}
	for _, p := range stale {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return restore(fmt.Errorf("pki: remove %s: %w", p, err))
		}
	}
	entries := make([]ManifestEntry, 0, len(res))
	for _, r := range res {
		entries = append(entries, ManifestEntry{Kind: r.file.Kind, Name: r.file.Name, Ref: r.file.Ref, Fingerprint: m.fingerprint(r.file.Kind, r.c.content)})
	}
	if err := m.manifest.Save(entries); err != nil {
		return restore(err)
	}
	m.rollback.clear()
	m.rollback = checkpoint
	success = true
	m.log.Info("PKI files materialised", "root", m.root, "files", len(res), "removed", len(stale))
	return nil
}

// ensureDir creates <root>/<kind dir> (private/ 0700, the others 0755) when missing; existing directories are left as
// the strongSwan package made them.
func (m *Materialiser) ensureDir(k Kind) error {
	if err := ensureOwnedDir(m.root, 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o755)
	if k == KindKey {
		mode = 0o700
	}
	return ensureOwnedDir(filepath.Join(m.root, k.Dir()), mode)
}
func ensureOwnedDir(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("pki: create directory: %w", err)
	}
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("pki: owned directory is unavailable or unsafe")
	}
	return os.Chmod(path, mode)
}

// Retrieve reports the manifest's files that are on disk (kind, name, ref, fingerprint of the content, mode), sorted.
// A missing or unreadable file is left out (a difference to the desired value).
func (m *Materialiser) Retrieve(_ context.Context) (*ngfwv1.PkiFileStateSet, error) {
	st, err := m.State()
	if err != nil {
		return nil, err
	}
	set := &ngfwv1.PkiFileStateSet{}
	for _, f := range st {
		if f.GetPresent() {
			set.Files = append(set.Files, &ngfwv1.PkiFileStateFile{Kind: f.GetKind(), Name: f.GetName(), Ref: f.GetRef(),
				Fingerprint: f.GetFingerprint(), Mode: f.GetMode()})
		}
	}
	return set, nil
}

// State reports every manifest file with size and presence (the PkiFileState RPC).
func (m *Materialiser) State() ([]*ngfwv1.PkiFileStateFile, error) {
	entries, err := m.manifest.Load()
	if err != nil {
		return nil, err
	}
	out := make([]*ngfwv1.PkiFileStateFile, 0, len(entries))
	for _, e := range entries {
		f := &ngfwv1.PkiFileStateFile{Kind: string(e.Kind), Name: e.Name, Ref: e.Ref}
		if content, mode, err := readBounded(m.Path(e.Kind, e.Name), e.Kind.maxSize()); err == nil {
			f.Fingerprint, f.Mode, f.Size, f.Present = m.fingerprint(e.Kind, content), uint32(mode), uint32(len(content)), true //nolint:gosec // bounded by maxSize
			zero(content)
		}
		out = append(out, f)
	}
	sortFiles(out)
	return out, nil
}

// readBounded reads a regular file (no symlink) of at most max bytes and returns its content and permission bits.
func readBounded(path string, maxSize int) ([]byte, os.FileMode, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // a path under the materialiser's root
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("pki: %s is not a regular file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(maxSize)+1))
	if err != nil {
		return nil, 0, err
	}
	if len(b) > maxSize {
		zero(b)
		return nil, 0, fmt.Errorf("pki: %s is larger than %d bytes", path, maxSize)
	}
	return b, info.Mode().Perm(), nil
}

// Watch checks every interval whether the files on disk still match the last apply (missing, changed, wrong mode)
// and calls onDrift when they do not — the agent's resync then writes them again. It returns when ctx ends.
func (m *Materialiser) Watch(ctx context.Context, interval time.Duration, onDrift func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	var last string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		st, err := m.State()
		if err != nil {
			continue
		}
		entries, err := m.manifest.Load()
		if err != nil {
			continue
		}
		expected := map[string]string{}
		for _, entry := range entries {
			expected[string(entry.Kind)+"/"+entry.Name] = entry.Fingerprint
		}
		var bad []string
		for _, f := range st {
			if !f.GetPresent() || f.GetMode() != uint32(Kind(f.GetKind()).Mode()) || expected[f.GetKind()+"/"+f.GetName()] != "" && f.GetFingerprint() != expected[f.GetKind()+"/"+f.GetName()] {
				bad = append(bad, f.GetKind()+"/"+f.GetName())
			}
		}
		if sig := strings.Join(bad, ","); len(bad) > 0 && sig != last {
			m.log.Warn("PKI files missing or changed on disk; asking for a resync", "files", bad)
			onDrift()
			last = sig
		} else if len(bad) == 0 {
			last = ""
		}
	}
}

// ---- manifest ----------------------------------------------------------------------------------

// ManifestEntry is one file this agent wrote.
type ManifestEntry struct {
	Kind        Kind   `json:"kind"`
	Name        string `json:"name"`
	Ref         string `json:"ref"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Manifest is the persisted list of the files this agent wrote (<state dir>/pki-files-<owner>.json, 0600): the
// materialiser removes only these and reports them after an agent restart.
type Manifest struct{ path string }

type manifestDoc struct {
	Version int             `json:"version"`
	Files   []ManifestEntry `json:"files"`
}

// OpenManifest returns the manifest at path (an absolute file path; the file is created by the first Save).
func OpenManifest(path string) (*Manifest, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("pki: manifest path %q must be absolute and clean", path)
	}
	return &Manifest{path: path}, nil
}

// Persistent implements persist.Store: the manifest is a file in the agent's state dir.
func (m *Manifest) Persistent() bool { return m != nil && m.path != "" }

// Path returns the manifest file.
func (m *Manifest) Path() string { return m.path }

// Load returns the recorded files (none when the file does not exist). Malformed entries are dropped.
func (m *Manifest) Load() ([]ManifestEntry, error) {
	b, _, err := readBounded(m.path, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pki: manifest: %w", err)
	}
	var doc manifestDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("pki: manifest %s does not parse: %w", m.path, err)
	}
	if doc.Version != 1 {
		return nil, errors.New("pki: unsupported manifest version")
	}
	out := doc.Files[:0]
	for _, e := range doc.Files {
		if e.Kind.Valid() && nameRe.MatchString(e.Name) && refRe.MatchString(e.Ref) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Save replaces the recorded files atomically.
func (m *Manifest) Save(entries []ManifestEntry) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return fmt.Errorf("pki: manifest dir: %w", err)
	}
	b, err := json.MarshalIndent(manifestDoc{Version: 1, Files: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("pki: manifest: %w", err)
	}
	if err := renderers.WriteFileAtomic(m.path, renderers.File{Mode: 0o600, Content: append(b, '\n')}); err != nil {
		return fmt.Errorf("pki: manifest: %w", err)
	}
	return nil
}

// ---- MapSource -------------------------------------------------------------------------------

// MapSource is an in-memory Source (tests, slot fixtures). Material sits behind a pointer and the type formats as
// "pki.MapSource(n secrets)", so %v/%+v/slog never print it. Safe for concurrent use.
type MapSource struct{ s *mapStore }

type mapStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

// NewMapSource returns an empty source.
func NewMapSource() *MapSource { return &MapSource{s: &mapStore{m: map[string][]byte{}}} }

// Put stores a copy of material under ref.
func (s *MapSource) Put(ref string, material []byte) {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	if old, ok := s.s.m[ref]; ok {
		zero(old)
	}
	s.s.m[ref] = append([]byte(nil), material...)
}

// Delete removes ref.
func (s *MapSource) Delete(ref string) {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	zero(s.s.m[ref])
	delete(s.s.m, ref)
}

// Resolve implements Source.
func (s *MapSource) Resolve(_ context.Context, ref string) ([]byte, error) {
	s.s.mu.RLock()
	defer s.s.mu.RUnlock()
	b, ok := s.s.m[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, vpn.Redact(ref))
	}
	return append([]byte(nil), b...), nil
}

// String implements fmt.Stringer without references or material.
func (s *MapSource) String() string {
	s.s.mu.RLock()
	defer s.s.mu.RUnlock()
	return fmt.Sprintf("pki.MapSource(%d secrets)", len(s.s.m))
}

// GoString implements fmt.GoStringer.
func (s *MapSource) GoString() string { return s.String() }

// LogValue implements slog.LogValuer.
func (s *MapSource) LogValue() slog.Value { return slog.StringValue(s.String()) }
