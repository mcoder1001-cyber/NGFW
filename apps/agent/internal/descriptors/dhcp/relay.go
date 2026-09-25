package dhcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// dhcp.relay (F-kea-dhcp-relay): the record of one configured relay (`services.dhcp.relays.<name>`). VPP has no
// relay object — only one proxy server list per rx VRF (dhcp.proxy) — so the relay's name, description, client
// interfaces and enabled flag exist nowhere in VPP. The record keeps them agent-locally (a JSON file in the state
// dir, like the objects domain's store) next to the VPP tuple its dhcp.proxy objects realise, and Retrieve reports a
// record only as far as VPP confirms it: every server of an enabled relay must exist as a dhcp.proxy with the
// record's rx VRF, server VRF and source address, otherwise the record is reported with the servers VPP has (drift
// → the reconciler re-creates the missing proxies and updates the record). The document relay travels inside the
// record (Doc, deterministic protobuf, base64) so the agent's Retrieve returns exactly the configured relay.

// NameRelay is the relay record descriptor.
const NameRelay = "dhcp.relay"

// Relay is the Value of a dhcp.relay object.
type Relay struct {
	Name string `json:"name"`
	// Doc is the base64 of the deterministic protobuf encoding of the document's vrx.v1.DhcpRelay.
	Doc string `json:"doc"`
	// Enabled relays are realised by dhcp.proxy objects {RxVRF, ServerVRF, server, Src} for each of Servers.
	Enabled   bool     `json:"enabled"`
	RxVRF     uint32   `json:"rx_vrf"`
	ServerVRF uint32   `json:"server_vrf"`
	Src       string   `json:"src"`
	Servers   []string `json:"servers"`
}

// Proto returns the canonical structpb document (Servers sorted and canonical).
func (s Relay) Proto() *structpb.Struct { return dfkit.Encode(s.canon()) }

func (s Relay) canon() Relay {
	out := s
	out.Servers = make([]string, 0, len(s.Servers))
	for _, a := range s.Servers {
		out.Servers = append(out.Servers, canonAddr(a))
	}
	sort.Strings(out.Servers)
	out.Src = canonAddr(s.Src)
	return out
}

// RelayKey returns the key of a relay record.
func RelayKey(name string) scheduler.Key { return scheduler.Join(NameRelay, name) }

// RelayStore persists relay records (implementations: MemRelayStore, FileRelayStore).
type RelayStore interface {
	Load() (map[string]Relay, error)
	Put(r Relay) error
	Delete(name string) error
}

// MemRelayStore keeps records in memory (tests; the default).
type MemRelayStore struct {
	mu sync.Mutex
	m  map[string]Relay
}

// Load implements RelayStore.
func (s *MemRelayStore) Load() (map[string]Relay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Relay, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out, nil
}

// Put implements RelayStore.
func (s *MemRelayStore) Put(r Relay) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]Relay{}
	}
	s.m[r.Name] = r
	return nil
}

// Delete implements RelayStore.
func (s *MemRelayStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}

// FileRelayStore persists records as one JSON file (0600, atomic rename).
type FileRelayStore struct {
	Path string
	mu   sync.Mutex
}

// Persistent reports that the store survives an agent restart (dfkit/persist.Store, TD-11b).
func (*FileRelayStore) Persistent() bool { return true }

// Load implements RelayStore (a missing file is an empty store).
func (s *FileRelayStore) Load() (map[string]Relay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *FileRelayStore) load() (map[string]Relay, error) {
	out := map[string]Relay{}
	b, err := os.ReadFile(s.Path) //nolint:gosec // the agent's own state file
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		// The records are re-creatable metadata (review L3): an unreadable file must not fail every transaction
		// that touches `services`. Treat it as empty — the next reconcile re-creates the records and rewrites it.
		return map[string]Relay{}, nil
	}
	return out, nil
}

func (s *FileRelayStore) save(m map[string]Relay) error {
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
	if err := f.Sync(); err != nil { // the data before the rename (review L3)
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(s.Path))
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync() // the rename itself
}

// Put implements RelayStore.
func (s *FileRelayStore) Put(r Relay) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	m[r.Name] = r
	return s.save(m)
}

// Delete implements RelayStore.
func (s *FileRelayStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := m[name]; !ok {
		return nil
	}
	delete(m, name)
	return s.save(m)
}

// RelayDescriptor manages dhcp.relay records.
type RelayDescriptor struct {
	client vpp.Client
	store  RelayStore
	o      options
}

var _ scheduler.Descriptor = (*RelayDescriptor)(nil)

// NewRelay returns the dhcp.relay descriptor (store nil = in memory: records are re-created on every agent start).
func NewRelay(client vpp.Client, store RelayStore, opts ...Option) *RelayDescriptor {
	if store == nil {
		store = &MemRelayStore{}
	}
	return &RelayDescriptor{client: client, store: store, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*RelayDescriptor) Name() string { return NameRelay }

// RecordsNoOwnership declares for the ownership guard (TD-11b): the record store holds relay metadata (name,
// description, client interfaces), not ownership — the relay's VPP objects are dhcp.proxy, owned through their rx VRF;
// a lost store only re-creates the records (no VPP operation). The product store is a file all the same.
func (*RelayDescriptor) RecordsNoOwnership() {}

// KeyOf implements scheduler.Descriptor.
func (d *RelayDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[Relay](obj)
	if err != nil || s.Name == "" {
		return scheduler.Join(NameRelay, "invalid")
	}
	return RelayKey(s.Name)
}

// Dependencies implements scheduler.Descriptor: the dhcp.proxy objects of an enabled relay (the record is written
// after them and removed before them).
func (d *RelayDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[Relay](obj)
	if err != nil || !s.Enabled {
		return nil
	}
	var deps []scheduler.Dependency
	for _, srv := range s.canon().Servers {
		deps = append(deps, scheduler.Dependency{Key: ProxyKey(s.RxVRF, s.ServerVRF, srv)})
	}
	return deps
}

func (d *RelayDescriptor) put(obj proto.Message) error {
	s, err := decode[Relay](obj)
	if err != nil {
		return err
	}
	if s.Name == "" {
		return dfkit.Specf("dhcp relay: empty name")
	}
	if s.Enabled && !d.o.vrfScope(s.RxVRF) {
		return dfkit.Specf("dhcp relay %s: rx vrf %d is outside this agent's VRF scope", s.Name, s.RxVRF)
	}
	return d.store.Put(s.canon())
}

// Create implements scheduler.Descriptor (no VPP operation: the proxies are their own objects).
func (d *RelayDescriptor) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, d.put(obj)
}

// Update implements scheduler.Descriptor.
func (d *RelayDescriptor) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.put(newObj)
}

// Delete implements scheduler.Descriptor.
func (d *RelayDescriptor) Delete(_ context.Context, obj proto.Message, _ any) error {
	s, err := decode[Relay](obj)
	if err != nil {
		return err
	}
	return d.store.Delete(s.Name)
}

// Retrieve implements scheduler.Descriptor: every stored record, its servers as far as VPP has them.
func (d *RelayDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := d.store.Load()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	details, err := dumpProxies(ctx, d.client)
	if err != nil {
		return nil, err
	}
	have := map[scheduler.Key]string{} // proxy key → src
	for _, p := range details {
		src := detailAddr(p.DHCPSrcAddress, p.IsIPv6).String()
		for _, srv := range p.Servers {
			have[ProxyKey(p.RxVrfID, srv.ServerVrfID, detailAddr(srv.DHCPServer, p.IsIPv6).String())] = src
		}
	}
	names := make([]string, 0, len(recs))
	for n := range recs {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []scheduler.KV
	for _, n := range names {
		r := recs[n].canon()
		if r.Enabled {
			kept := make([]string, 0, len(r.Servers))
			for _, srv := range r.Servers {
				if src, ok := have[ProxyKey(r.RxVRF, r.ServerVRF, srv)]; ok && src == r.Src {
					kept = append(kept, srv)
				}
			}
			if !slices.Equal(kept, r.Servers) {
				r.Servers = kept
			}
		}
		out = append(out, scheduler.KV{Key: RelayKey(n), Value: r.Proto()})
	}
	return out, nil
}
