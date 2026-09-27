package subsystems

// F-vrrp-config-sync: domain `ha` — DF-7's VPP vrrp descriptors (engine "vpp"), the agent-local vrrp.meta
// record (names VPP cannot hold) and RF-4's keepalived renderer stage (engine "keepalived", keepalived.go).
//
//	ownership (TD-11b): a VR belongs to the owner of its interface (DF-7: re-resolved on every call, D-071);
//	                    the applied-once records of the df7 families live in the owner's BootStore
//	                    (df7.SetBootStore, installed by register); vrrp.meta is a persisted file store.
//	id range (TD-8b):   VRRP allocates no VPP id (a VR is keyed by interface + VRID + family), so no
//	                    df7.WithIDRange option is passed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// HA is the `ha` domain.
const HA = "ha"

// vrrpDescriptors are the descriptors of the `ha` domain.
var vrrpDescriptors = []string{vrrp.NameVR, vrrp.NamePeers, vrrp.NameTrack, vrrp.NameState, desired.VrrpMetaName, desired.KeepalivedConfigName}

func init() { Domains[HA] = append(Domains[HA], vrrpDescriptors...) }

// registerVrrp registers the vrrp family (tolerant Retrieve on a VPP without the vrrp plugin), vrrp.meta
// over <state dir>/vrrp-meta-<owner>.json and the keepalived stage.
func registerVrrp(r scheduler.Registry, w *Wiring) error {
	c, owner := w.env.Client, w.env.Owner
	vrrp.Register(vrrpRegistry{r, c}, c, owner, df7.WithInterfaceKey(dfkit.DefaultInterfaceKey))
	store, err := newVrrpMetaFile(filepath.Join(w.env.StateDir, "vrrp-meta-"+owner+".json"))
	if err != nil {
		return fmt.Errorf("vrrp: %w", err)
	}
	r.Register(desired.NewVrrpMeta(store))
	registerKeepalived(r, w)
	return nil
}

// vrrpRegistry decorates every vrrp descriptor: Retrieve on a VPP without the vrrp plugin reports no
// objects instead of failing the Retrieve / plan of every domain (same rule as F-lisp / F-tunnels).
type vrrpRegistry struct {
	scheduler.Registry
	c vpp.Client
}

func (g vrrpRegistry) Register(d scheduler.Descriptor) {
	g.Registry.Register(vrrpTolerant{Descriptor: d, c: g.c})
}

type vrrpTolerant struct {
	scheduler.Descriptor
	c vpp.Client
}

// Unwrap exposes the decorated descriptor (dfkit/persist walks the chain).
func (t vrrpTolerant) Unwrap() scheduler.Descriptor { return t.Descriptor }

// Retrieve implements scheduler.Descriptor.
func (t vrrpTolerant) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := t.Descriptor.Retrieve(ctx)
	if err == nil || scheduler.IsRetrieveUnsupported(err) {
		return kvs, err
	}
	if errors.Is(err, df7.ErrPluginNotLoaded) {
		return nil, nil
	}
	if h, ok := t.c.(interface{ Handles(string) bool }); ok && !h.Handles("vrrp_vr_dump") {
		return nil, nil
	}
	return kvs, err
}

// vrrpMetaFile is the persisted desired.VrrpMetaStore (one JSON file, written atomically).
type vrrpMetaFile struct {
	mu   sync.Mutex
	path string
}

type vrrpMetaDoc struct {
	Version int                    `json:"version"`
	Entries []desired.VrrpMetaSpec `json:"entries"`
}

func newVrrpMetaFile(path string) (*vrrpMetaFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f := &vrrpMetaFile{path: path}
	if _, err := f.Load(); err != nil {
		return nil, err
	}
	return f, nil
}

// Persistent marks the store as surviving an agent restart (dfkit/persist).
func (*vrrpMetaFile) Persistent() bool { return true }

// Load implements desired.VrrpMetaStore.
func (f *vrrpMetaFile) Load() (map[string]desired.VrrpMetaSpec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]desired.VrrpMetaSpec{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s store: %w", desired.VrrpMetaName, err)
	}
	var doc vrrpMetaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s store %s: %w", desired.VrrpMetaName, f.path, err)
	}
	m := make(map[string]desired.VrrpMetaSpec, len(doc.Entries))
	for _, e := range doc.Entries {
		m[e.ID] = e
	}
	return m, nil
}

// Save implements desired.VrrpMetaStore.
func (f *vrrpMetaFile) Save(m map[string]desired.VrrpMetaSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := vrrpMetaDoc{Version: 1, Entries: make([]desired.VrrpMetaSpec, 0, len(m))}
	for _, e := range m {
		doc.Entries = append(doc.Entries, e)
	}
	sort.Slice(doc.Entries, func(i, j int) bool { return doc.Entries[i].ID < doc.Entries[j].ID })
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(f.path, append(raw, '\n'))
}
