package acl

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// noACL is the acl_index wildcard: "create" in acl_add_replace, "all" in acl_dump.
const noACL = ^uint32(0)

// ErrNoACL is returned (wrapped) when a binding names an ACL this owner does not have on VPP.
var ErrNoACL = errors.New("acl: no such acl")

// Meta is the runtime handle of an acl.acl object: the acl_index VPP assigned. Interface
// bindings and ABF policies (DF-2) reference this index; Update keeps it (acl_add_replace on the
// same index), Retrieve fills it from acl_details.
type Meta struct {
	ACLIndex uint32
}

// KeyACL is the scheduler key of the ACL called name: "acl.acl/<name>". Other descriptors
// (acl.interface-binding, DF-2 abf.policy) declare dependencies with it.
func KeyACL(name string) scheduler.Key { return scheduler.Join(NameACL, name) }

// Descriptor manages acl.acl objects (VPP acl_add_replace / acl_del / acl_dump).
type Descriptor struct {
	client vpp.Client
	owner  string
}

var _ scheduler.Descriptor = (*Descriptor)(nil)

// NewACL returns the acl.acl descriptor for one owner (VRX_OWNER; tests: VRX_TEST_PREFIX).
func NewACL(client vpp.Client, owner string) *Descriptor {
	return &Descriptor{client: client, owner: owner}
}

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return NameACL }

// KeyOf implements scheduler.Descriptor: acl.acl/<name>.
func (*Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	a, _ := FromProto(obj)
	return KeyACL(a.Name)
}

// Dependencies implements scheduler.Descriptor: an ACL depends on nothing.
func (*Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// addReplace sends acl_add_replace for index (noACL = create) and returns the resulting index.
func (d *Descriptor) addReplace(ctx context.Context, index uint32, a ACL) (uint32, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	tag, err := vpp.OwnerTag(d.owner, a.Name)
	if err != nil {
		return 0, err
	}
	rules, err := encodeRules(a.Rules)
	if err != nil {
		return 0, fmt.Errorf("acl %q: %w", a.Name, err)
	}
	rep, err := acl.NewServiceClient(d.client).ACLAddReplace(ctx, &acl.ACLAddReplace{ACLIndex: index, Tag: tag, R: rules})
	if err != nil {
		return 0, fmt.Errorf("acl_add_replace %q: %w", a.Name, err)
	}
	return rep.ACLIndex, nil
}

// Create implements scheduler.Descriptor: acl_add_replace with acl_index = ~0.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a, err := FromProto(obj)
	if err != nil {
		return nil, err
	}
	idx, err := d.addReplace(ctx, noACL, a)
	if err != nil {
		return nil, err
	}
	return Meta{ACLIndex: idx}, nil
}

// Update implements scheduler.Descriptor: acl_add_replace on the existing index, so bindings
// and ABF policies that reference the index stay valid. A different name is a different object
// (ErrRecreate; it cannot happen through the scheduler because the key changes too).
func (d *Descriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldACL, err := FromProto(oldObj)
	if err != nil {
		return nil, err
	}
	newACL, err := FromProto(newObj)
	if err != nil {
		return nil, err
	}
	if oldACL.Name != newACL.Name {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(Meta)
	if !ok {
		return nil, fmt.Errorf("acl.acl: unexpected meta %T", meta)
	}
	if _, err := d.addReplace(ctx, m.ACLIndex, newACL); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: acl_del by the index in meta. VPP refuses while the
// ACL is bound to an interface or a lookup context (ACL_IN_USE_*); the binding dependencies make
// the scheduler unbind first.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("acl.acl: unexpected meta %T", meta)
	}
	if _, err := acl.NewServiceClient(d.client).ACLDel(ctx, &acl.ACLDel{ACLIndex: m.ACLIndex}); err != nil {
		return fmt.Errorf("acl_del %d: %w", m.ACLIndex, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: acl_dump (all), keep the ACLs tagged by this owner,
// decode their rules in VPP order. When several VPP ACLs carry the same owner tag (a lost
// acl_add_replace reply followed by a retry, two agents with one owner id), the lowest index is
// the object ("acl.acl/<name>") and every other one is reported as "acl.acl/<name>#<index>": such
// a key is never desired (names cannot contain "#"), so the scheduler deletes the extras.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := dumpOwnedACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(owned))
	for _, e := range owned {
		rules, err := decodeRules(e.Rules)
		if err != nil {
			return nil, fmt.Errorf("acl %d (%q): %w", e.Index, e.Name, err)
		}
		out = append(out, scheduler.KV{
			Key:   KeyACL(e.keyName()),
			Value: ACL{Name: e.keyName(), Rules: rules}.Proto(),
			Meta:  Meta{ACLIndex: e.Index},
		})
	}
	return out, nil
}

// ownedACL is one acl_details of this owner.
type ownedACL struct {
	Index uint32
	Name  string
	Dup   bool // another ACL with the same tag has a lower index (see Descriptor.Retrieve)
	Rules []acl_types.ACLRule
}

// keyName is the object name the ACL is reported under: Name, or "<Name>#<index>" for a duplicate.
func (e ownedACL) keyName() string { return dupName(e.Name, e.Index, e.Dup) }

// dupName builds the reported name of an owned (MACIP) ACL; see Descriptor.Retrieve.
func dupName(name string, index uint32, dup bool) string {
	if !dup {
		return name
	}
	return fmt.Sprintf("%s%s%d", name, dupSeparator, index)
}

// dumpOwnedACLs runs acl_dump for all ACLs and keeps those whose tag parses as this owner's,
// sorted by index; for a tag seen more than once every entry but the lowest index is marked Dup.
func dumpOwnedACLs(ctx context.Context, c vpp.Client, owner string) ([]ownedACL, error) {
	stream, err := acl.NewServiceClient(c).ACLDump(ctx, &acl.ACLDump{ACLIndex: noACL})
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	var out []ownedACL
	for _, det := range details {
		name, ok := vpp.ParseOwnerTag(det.Tag, owner)
		if !ok {
			continue // another owner's ACL or untagged: never ours
		}
		out = append(out, ownedACL{Index: det.ACLIndex, Name: name, Rules: det.R})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	seen := make(map[string]struct{}, len(out))
	for i := range out {
		if _, dup := seen[out[i].Name]; dup {
			out[i].Dup = true
			continue
		}
		seen[out[i].Name] = struct{}{}
	}
	return out, nil
}

// aclIndexByName indexes a dumpOwnedACLs result by object name (duplicates excluded, so a
// name always resolves to the lowest index — the one Retrieve reports under the plain key).
func aclIndexByName(owned []ownedACL) map[string]uint32 {
	m := make(map[string]uint32, len(owned))
	for _, e := range owned {
		if !e.Dup {
			m[e.Name] = e.Index
		}
	}
	return m
}

// aclNameByIndex indexes a dumpOwnedACLs result by acl_index, with the reported (keyName) names.
func aclNameByIndex(owned []ownedACL) map[uint32]string {
	m := make(map[uint32]string, len(owned))
	for _, e := range owned {
		m[e.Index] = e.keyName()
	}
	return m
}

// LookupIndex returns the acl_index of this owner's ACL called name, for descriptors of other
// plugins that reference ACLs by index (DF-2 abf.policy). It wraps ErrNoACL when absent.
func LookupIndex(ctx context.Context, c vpp.Client, owner, name string) (uint32, error) {
	owned, err := dumpOwnedACLs(ctx, c, owner)
	if err != nil {
		return 0, err
	}
	if idx, ok := aclIndexByName(owned)[name]; ok {
		return idx, nil
	}
	return 0, fmt.Errorf("%w: %q (owner %q)", ErrNoACL, name, owner)
}
