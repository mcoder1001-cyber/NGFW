package subsystems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// tunnelsDescriptors are the descriptors of tunnels.gre / .ipip / .vxlan (F-tunnels), appended to
// the `tunnels` domain next to F-lisp's LISP family.
var tunnelsDescriptors = []string{gre.TunnelName, ipip.TunnelName, vxlan.TunnelName, desired.TunnelMetaName}

func init() { Domains[Tunnels] = append(Domains[Tunnels], tunnelsDescriptors...) }

// registerTunnels registers DF-6's GRE, IPIP and VXLAN tunnel descriptors (ownership = the owner tag
// on the interface, TD-11b RecordsNoOwnership) and tunnels.meta over the file store
// <state dir>/tunnels-meta-<owner>.json (names and descriptions VPP cannot hold).
func registerTunnels(r scheduler.Registry, w *Wiring) error {
	c, owner := w.env.Client, w.env.Owner
	r.Register(tunnelTolerant{Descriptor: gre.NewTunnel(c, owner), c: c, dump: "gre_tunnel_v2_dump"})
	r.Register(tunnelTolerant{Descriptor: ipip.NewTunnel(c, owner), c: c, dump: "ipip_tunnel_dump"})
	r.Register(tunnelTolerant{Descriptor: vxlan.NewTunnel(c, owner), c: c, dump: "vxlan_tunnel_v2_dump"})
	store, err := newTunnelMetaFile(filepath.Join(w.env.StateDir, "tunnels-meta-"+owner+".json"))
	if err != nil {
		return fmt.Errorf("tunnels: %w", err)
	}
	r.Register(desired.NewTunnelMeta(store))
	return nil
}

// TunnelsIDSpan is the id range tunnel instances must lie in (TD-8b, fail closed): the slot's range
// with NGFW_VPP_TABLE_BASE, every id with NGFW_VPP_ID_RANGE=all, and the empty range when neither is set
// or the setting is malformed — the projection then refuses every tunnel with a pointer at its
// instance. Resolved from the same environment as Env.IDs (the projection holds no Wiring; see SvsRange).
func TunnelsIDSpan() desired.TunnelIDSpan {
	s, err := ResolveIDScope()
	switch {
	case err != nil:
		return desired.TunnelIDSpan{Lo: 1, Hi: 0}
	case s.All:
		return desired.TunnelIDSpan{All: true}
	case s.Range != nil:
		return desired.TunnelIDSpan{Lo: s.Range.Lo, Hi: s.Range.Hi}
	}
	return desired.TunnelIDSpan{Lo: 1, Hi: 0}
}

// tunnelMetaFile is the persisted desired.TunnelMetaStore (one JSON file, written atomically).
type tunnelMetaFile struct {
	mu   sync.Mutex
	path string
}

type tunnelMetaDoc struct {
	Version int                      `json:"version"`
	Entries []desired.TunnelMetaSpec `json:"entries"`
}

func newTunnelMetaFile(path string) (*tunnelMetaFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f := &tunnelMetaFile{path: path}
	if _, err := f.Load(); err != nil {
		return nil, err
	}
	return f, nil
}

// Persistent marks the store as surviving an agent restart (dfkit/persist).
func (*tunnelMetaFile) Persistent() bool { return true }

// Load implements desired.TunnelMetaStore.
func (f *tunnelMetaFile) Load() (map[string]desired.TunnelMetaSpec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]desired.TunnelMetaSpec{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s store: %w", desired.TunnelMetaName, err)
	}
	var doc tunnelMetaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s store %s: %w", desired.TunnelMetaName, f.path, err)
	}
	m := make(map[string]desired.TunnelMetaSpec, len(doc.Entries))
	for _, e := range doc.Entries {
		m[e.ID] = e
	}
	return m, nil
}

// Save implements desired.TunnelMetaStore.
func (f *tunnelMetaFile) Save(m map[string]desired.TunnelMetaSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := tunnelMetaDoc{Version: 1, Entries: make([]desired.TunnelMetaSpec, 0, len(m))}
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

// tunnelTolerant: Retrieve on a VPP without the tunnel's plugin reports no objects instead of failing
// the Retrieve / plan of every domain (a VPP that does not know GRE holds no GRE tunnel); creating one
// still fails with df6.ErrPluginNotLoaded. Same rule as F-lisp's lispTolerant.
type tunnelTolerant struct {
	scheduler.Descriptor
	c    vpp.Client
	dump string
}

// Unwrap exposes the decorated descriptor (dfkit/persist walks the chain for RecordsNoOwnership).
func (t tunnelTolerant) Unwrap() scheduler.Descriptor { return t.Descriptor }

// Retrieve implements scheduler.Descriptor.
func (t tunnelTolerant) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := t.Descriptor.Retrieve(ctx)
	if err == nil || scheduler.IsRetrieveUnsupported(err) {
		return kvs, err
	}
	if errors.Is(err, df6.ErrPluginNotLoaded) {
		return nil, nil
	}
	if h, ok := t.c.(interface{ Handles(string) bool }); ok && !h.Handles(t.dump) {
		return nil, nil
	}
	return kvs, err
}
