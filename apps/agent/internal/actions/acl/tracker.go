package acl

import (
	"context"
	"sort"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/scheduler"
)

// Applied is what the acl.acl (or acl.macip-acl) descriptor last saw in VPP for one list of this
// owner: its index and the fingerprint of the rules VPP holds.
type Applied struct {
	Name        string
	Index       uint32
	Fingerprint string
	VPPRules    int
}

// Tracker follows the acl.acl and acl.macip-acl descriptors: every successful Create, Update,
// Delete and Retrieve updates it, so AclState knows the index and content of each list without
// dumping whole ACLs (D-132: a 100 000-rule list is never dumped for a counter refresh).
type Tracker struct {
	mu    sync.Mutex
	acls  map[string]Applied
	macip map[string]Applied
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{acls: map[string]Applied{}, macip: map[string]Applied{}}
}

func sortedApplied(m map[string]Applied) []Applied {
	out := make([]Applied, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ACLs returns the tracked L3/L4 ACLs sorted by name.
func (t *Tracker) ACLs() []Applied {
	t.mu.Lock()
	defer t.mu.Unlock()
	return sortedApplied(t.acls)
}

// MacipACLs returns the tracked MACIP ACLs sorted by name.
func (t *Tracker) MacipACLs() []Applied {
	t.mu.Lock()
	defer t.mu.Unlock()
	return sortedApplied(t.macip)
}

// ACL returns the tracked ACL called name.
func (t *Tracker) ACL(name string) (Applied, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.acls[name]
	return a, ok
}

func (t *Tracker) set(m map[string]Applied, a Applied) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m[a.Name] = a
}

func (t *Tracker) remove(m map[string]Applied, name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(m, name)
}

func (t *Tracker) replace(macip bool, all []Applied) {
	m := make(map[string]Applied, len(all))
	for _, a := range all {
		m[a.Name] = a
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if macip {
		t.macip = m
	} else {
		t.acls = m
	}
}

// isDuplicate reports a "<name>#<index>" object (a duplicate-tag ACL the scheduler deletes).
func isDuplicate(name string) bool { return strings.Contains(name, "#") }

func appliedACL(obj proto.Message, meta any) (Applied, bool) {
	a, err := descacl.FromProto(obj)
	m, ok := meta.(descacl.Meta)
	if err != nil || !ok || isDuplicate(a.Name) {
		return Applied{}, false
	}
	return Applied{Name: a.Name, Index: m.ACLIndex, Fingerprint: Fingerprint(a.Rules), VPPRules: len(a.Rules)}, true
}

func appliedMacip(obj proto.Message, meta any) (Applied, bool) {
	a, err := descacl.MacipACLFromProto(obj)
	m, ok := meta.(descacl.MacipMeta)
	if err != nil || !ok || isDuplicate(a.Name) {
		return Applied{}, false
	}
	return Applied{Name: a.Name, Index: m.ACLIndex, Fingerprint: MacipFingerprint(a.Rules), VPPRules: len(a.Rules)}, true
}

// ACLDescriptor is DF-4's acl.acl descriptor with the tracker attached (same name, keys, values
// and behaviour; every method not overridden here is DF-4's).
type ACLDescriptor struct {
	*descacl.Descriptor
	t *Tracker
}

var _ scheduler.Descriptor = (*ACLDescriptor)(nil)

// WrapACL attaches the tracker to d.
func (t *Tracker) WrapACL(d *descacl.Descriptor) *ACLDescriptor {
	return &ACLDescriptor{Descriptor: d, t: t}
}

// Create implements scheduler.Descriptor.
func (d *ACLDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	meta, err := d.Descriptor.Create(ctx, obj)
	if a, ok := appliedACL(obj, meta); err == nil && ok {
		d.t.set(d.t.acls, a)
	}
	return meta, err
}

// Update implements scheduler.Descriptor.
func (d *ACLDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := d.Descriptor.Update(ctx, oldObj, newObj, meta)
	if a, ok := appliedACL(newObj, m); err == nil && ok {
		d.t.set(d.t.acls, a)
	}
	return m, err
}

// Delete implements scheduler.Descriptor.
func (d *ACLDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	err := d.Descriptor.Delete(ctx, obj, meta)
	if a, derr := descacl.FromProto(obj); err == nil && derr == nil {
		d.t.remove(d.t.acls, a.Name)
	}
	return err
}

// Retrieve implements scheduler.Descriptor.
func (d *ACLDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := d.Descriptor.Retrieve(ctx)
	if err == nil {
		all := make([]Applied, 0, len(kvs))
		for _, kv := range kvs {
			if a, ok := appliedACL(kv.Value, kv.Meta); ok {
				all = append(all, a)
			}
		}
		d.t.replace(false, all)
	}
	return kvs, err
}

// MacipDescriptor is DF-4's acl.macip-acl descriptor with the tracker attached.
type MacipDescriptor struct {
	*descacl.MacipACLDescriptor
	t *Tracker
}

var _ scheduler.Descriptor = (*MacipDescriptor)(nil)

// WrapMacip attaches the tracker to d.
func (t *Tracker) WrapMacip(d *descacl.MacipACLDescriptor) *MacipDescriptor {
	return &MacipDescriptor{MacipACLDescriptor: d, t: t}
}

// Create implements scheduler.Descriptor.
func (d *MacipDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	meta, err := d.MacipACLDescriptor.Create(ctx, obj)
	if a, ok := appliedMacip(obj, meta); err == nil && ok {
		d.t.set(d.t.macip, a)
	}
	return meta, err
}

// Update implements scheduler.Descriptor.
func (d *MacipDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := d.MacipACLDescriptor.Update(ctx, oldObj, newObj, meta)
	if a, ok := appliedMacip(newObj, m); err == nil && ok {
		d.t.set(d.t.macip, a)
	}
	return m, err
}

// Delete implements scheduler.Descriptor.
func (d *MacipDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	err := d.MacipACLDescriptor.Delete(ctx, obj, meta)
	if a, derr := descacl.MacipACLFromProto(obj); err == nil && derr == nil {
		d.t.remove(d.t.macip, a.Name)
	}
	return err
}

// Retrieve implements scheduler.Descriptor.
func (d *MacipDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := d.MacipACLDescriptor.Retrieve(ctx)
	if err == nil {
		all := make([]Applied, 0, len(kvs))
		for _, kv := range kvs {
			if a, ok := appliedMacip(kv.Value, kv.Meta); ok {
				all = append(all, a)
			}
		}
		d.t.replace(true, all)
	}
	return kvs, err
}
