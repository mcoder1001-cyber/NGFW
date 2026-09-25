package nftables

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
)

// DescriptorName is the scheduler descriptor of the host firewall (subsystems.Domains["acl"]).
const DescriptorName = "host-acl.nftables"

// Key is the key of the one host firewall object of an agent.
var Key = scheduler.Join(DescriptorName, "vrx")

// Descriptor wraps the renderer as one singleton scheduler object (decision (a) of F-host-acl-nftables:
// the agent core runs descriptors only, so the renderer rides on one):
//
//	Create/Update  Render → Validate (nft -c on a staged copy) → Apply (one nft -f transaction) → store
//	Delete         remove the table (add + delete in one transaction) → forget the store
//	Retrieve       the kernel table (nft -j list table) + the configuration and rule annotations of the
//	               store entry the kernel still matches (rules pair by their comment, which ends in a
//	               hash of the rule text)
//
// The store (<state dir>/host-acl-<owner>.json, 0600) is the value last applied: it survives an agent
// restart, so Retrieve after a restart equals the desired value when the kernel table is intact, and
// differs (→ Update, re-render) when the table was lost or edited.
type Descriptor struct {
	r   *Renderer
	st  *Store
	log *slog.Logger
	mu  sync.Mutex
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// NewDescriptor returns the descriptor of renderer r with its store.
func NewDescriptor(r *Renderer, st *Store, log *slog.Logger) *Descriptor {
	if log == nil {
		log = slog.Default()
	}
	return &Descriptor{r: r, st: st, log: log}
}

// Name implements scheduler.Descriptor.
func (d *Descriptor) Name() string { return DescriptorName }

// RecordsNoOwnership declares, for the TD-11b ownership guard (dfkit/persist.NoOwnership), that this
// descriptor records no ownership claim: what is ours is the owner-specific table name itself (`inet vrx`,
// `inet vrx_<owner>`), the kernel's equivalent of an owner-prefixed name. Its store (the value last
// applied, for Retrieve's configuration and annotations) is always a file in the state dir.
func (*Descriptor) RecordsNoOwnership() {}

// KeyOf implements scheduler.Descriptor: there is one object.
func (d *Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies implements scheduler.Descriptor: objects are expanded into the value, nothing else.
func (d *Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *Descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	files, err := d.r.Render(ctx, nil)
	if err != nil {
		return err
	}
	if err := d.r.Validate(ctx, files); err != nil {
		return err
	}
	if err := d.r.Apply(ctx, files); err != nil {
		return err
	}
	d.log.Info("host firewall removed", "table", d.r.paths.Table, "mode", d.r.paths.Mode)
	return d.st.Remove()
}

func (d *Descriptor) apply(ctx context.Context, obj proto.Message) error {
	v, ok := obj.(*HostTable)
	if !ok {
		return fmt.Errorf("nftables: value %T, want *HostTable", obj)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	files, err := d.r.Render(ctx, v)
	if err != nil {
		return err
	}
	if err := d.r.Validate(ctx, files); err != nil {
		return err
	}
	if err := d.r.Apply(ctx, files); err != nil {
		return err
	}
	if err := d.st.Save(d.withKernelHashes(ctx, v)); err != nil {
		return err
	}
	d.log.Info("host firewall rendered", "table", d.r.paths.Table, "mode", d.r.paths.Mode, "chains", len(v.GetChains()), "sets", len(v.GetSets()))
	return nil
}

// withKernelHashes is v plus the kernel's own rule-body hashes read back right after the load (M1): a
// later hand edit of a rule body that keeps the comment is then visible to Retrieve. Without a kernel
// table (mode check, nothing rendered) or when the read-back fails, v is stored as it is.
func (d *Descriptor) withKernelHashes(ctx context.Context, v *HostTable) *HostTable {
	if d.r.paths.Mode == ModeCheck || len(v.GetChains()) == 0 {
		return v
	}
	k, err := d.r.Kernel(ctx)
	if err != nil || k == nil {
		d.log.Warn("host firewall: read-back after load failed; rule bodies are paired by comment only until the next apply", "err", err)
		return v
	}
	out := proto.Clone(v).(*HostTable)
	out.KernelHashes = k.Hashes()
	return out
}

// Retrieve implements scheduler.Descriptor.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	v, _, err := d.actual(ctx)
	if err != nil || v == nil {
		return nil, err
	}
	return []scheduler.KV{{Key: Key, Value: v}}, nil
}

// actual is the value Retrieve reports and the kernel table it came from (nil in mode check).
func (d *Descriptor) actual(ctx context.Context) (*HostTable, *KernelTable, error) {
	stored, err := d.st.Load()
	if err != nil {
		return nil, nil, err
	}
	if d.r.paths.Mode == ModeCheck {
		return stored, nil, nil // nothing is loaded: the stored rendering is all there is
	}
	k, err := d.r.Kernel(ctx)
	if err != nil {
		return nil, nil, err
	}
	if k == nil && stored == nil {
		return nil, nil, nil
	}
	v := &HostTable{}
	if stored != nil {
		v.Config = stored.GetConfig()
	}
	if k != nil {
		kt := k.Table()
		v.Sets, v.Chains, v.Dormant = kt.GetSets(), kt.GetChains(), kt.GetDormant()
		annotate(v, stored, k)
	}
	return v, k, nil
}

// annotate copies text, kind, list, sequence, pointer and verdict from the stored rule with the same
// comment (the comment ends in the hash of the rendered text) — but only while the kernel rule is still
// that rule (fix round 1, M1): its verdict equals the stored one, and its body hashes as it did right after
// the last `nft -f` (stored kernel_hashes; a store without them pairs by comment and verdict). A rule edited
// by hand keeps the kernel's verdict and no text, so the value differs from the desired one → Update.
// v's chains and rules are k's, in k's order.
func annotate(v, stored *HostTable, k *KernelTable) {
	byComment := map[string]*Rule{}
	for _, c := range stored.GetChains() {
		for _, r := range c.GetRules() {
			byComment[r.GetComment()] = r
		}
	}
	hashes := stored.GetKernelHashes()
	for ci, c := range v.GetChains() {
		for ri, r := range c.GetRules() {
			s, ok := byComment[r.GetComment()]
			if !ok || r.GetComment() == "" || r.GetVerdict() != s.GetVerdict() {
				continue
			}
			if want, recorded := hashes[c.GetName()+"/"+r.GetComment()]; recorded && k != nil && ci < len(k.Chains) && ri < len(k.Chains[ci].Rules) && k.Chains[ci].Rules[ri].ExprHash != want {
				continue
			}
			r.Text, r.Kind, r.List, r.Sequence, r.Pointer, r.Verdict = s.GetText(), s.GetKind(), s.GetList(), s.GetSequence(), s.GetPointer(), s.GetVerdict()
		}
	}
}

// ---- store -------------------------------------------------------------------------------------

// Store is the value last applied, persisted as protobuf JSON (0600, atomic replace).
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore returns the store at path (the file need not exist).
func NewStore(path string) *Store { return &Store{path: path} }

// Path is the store file.
func (s *Store) Path() string { return s.path }

// Load returns the stored value, nil when there is none.
func (s *Store) Load() (*HostTable, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("nftables: read store: %w", err)
	}
	v := &HostTable{}
	if err := protojson.Unmarshal(raw, v); err != nil {
		return nil, fmt.Errorf("nftables: store %s: %w", s.path, err)
	}
	return v, nil
}

// Save replaces the stored value.
func (s *Store) Save(v *HostTable) error {
	raw, err := protojson.MarshalOptions{Multiline: true}.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return renderers.WriteFileAtomic(s.path, renderers.File{Mode: 0o600, Content: append(raw, '\n')})
}

// Remove forgets the stored value.
func (s *Store) Remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
