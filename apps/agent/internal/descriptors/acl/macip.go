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

// ErrNoMacipACL is returned (wrapped) when a binding names a MACIP ACL this owner does not have.
var ErrNoMacipACL = errors.New("acl: no such macip acl")

// MacipMeta is the runtime handle of an acl.macip-acl object: its acl_index in the MACIP pool
// (a separate index space from acl.acl).
type MacipMeta struct {
	ACLIndex uint32
}

// KeyMacipACL is the scheduler key of the MACIP ACL called name: "acl.macip-acl/<name>".
func KeyMacipACL(name string) scheduler.Key { return scheduler.Join(NameMacipACL, name) }

// MacipACLDescriptor manages acl.macip-acl objects (macip_acl_add_replace / macip_acl_del /
// macip_acl_dump).
type MacipACLDescriptor struct {
	client vpp.Client
	owner  string
}

var _ scheduler.Descriptor = (*MacipACLDescriptor)(nil)

// NewMacipACL returns the acl.macip-acl descriptor for one owner.
func NewMacipACL(client vpp.Client, owner string) *MacipACLDescriptor {
	return &MacipACLDescriptor{client: client, owner: owner}
}

// Name implements scheduler.Descriptor.
func (*MacipACLDescriptor) Name() string { return NameMacipACL }

// KeyOf implements scheduler.Descriptor: acl.macip-acl/<name>.
func (*MacipACLDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	a, _ := MacipACLFromProto(obj)
	return KeyMacipACL(a.Name)
}

// Dependencies implements scheduler.Descriptor: a MACIP ACL depends on nothing.
func (*MacipACLDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *MacipACLDescriptor) addReplace(ctx context.Context, index uint32, a MacipACL) (uint32, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	tag, err := vpp.OwnerTag(d.owner, a.Name)
	if err != nil {
		return 0, err
	}
	rules, err := encodeMacipRules(a.Rules)
	if err != nil {
		return 0, fmt.Errorf("macip-acl %q: %w", a.Name, err)
	}
	rep, err := acl.NewServiceClient(d.client).MacipACLAddReplace(ctx, &acl.MacipACLAddReplace{ACLIndex: index, Tag: tag, R: rules})
	if err != nil {
		return 0, fmt.Errorf("macip_acl_add_replace %q: %w", a.Name, err)
	}
	return rep.ACLIndex, nil
}

// Create implements scheduler.Descriptor: macip_acl_add_replace with acl_index = ~0.
func (d *MacipACLDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a, err := MacipACLFromProto(obj)
	if err != nil {
		return nil, err
	}
	idx, err := d.addReplace(ctx, noACL, a)
	if err != nil {
		return nil, err
	}
	return MacipMeta{ACLIndex: idx}, nil
}

// Update implements scheduler.Descriptor: macip_acl_add_replace on the same index (interface
// bindings stay valid). A different name means ErrRecreate.
func (d *MacipACLDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldACL, err := MacipACLFromProto(oldObj)
	if err != nil {
		return nil, err
	}
	newACL, err := MacipACLFromProto(newObj)
	if err != nil {
		return nil, err
	}
	if oldACL.Name != newACL.Name {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(MacipMeta)
	if !ok {
		return nil, fmt.Errorf("acl.macip-acl: unexpected meta %T", meta)
	}
	if _, err := d.addReplace(ctx, m.ACLIndex, newACL); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: macip_acl_del. VPP refuses while the ACL is bound.
func (d *MacipACLDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(MacipMeta)
	if !ok {
		return fmt.Errorf("acl.macip-acl: unexpected meta %T", meta)
	}
	if _, err := acl.NewServiceClient(d.client).MacipACLDel(ctx, &acl.MacipACLDel{ACLIndex: m.ACLIndex}); err != nil {
		return fmt.Errorf("macip_acl_del %d: %w", m.ACLIndex, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: macip_acl_dump (all), owner-tagged only. Duplicate
// tags are handled like acl.acl: the lowest index is "acl.macip-acl/<name>", every other one
// "acl.macip-acl/<name>#<index>" (never desired, so the scheduler deletes it).
func (d *MacipACLDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := dumpOwnedMacipACLs(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(owned))
	for _, e := range owned {
		rules, err := decodeMacipRules(e.Rules)
		if err != nil {
			return nil, fmt.Errorf("macip acl %d (%q): %w", e.Index, e.Name, err)
		}
		out = append(out, scheduler.KV{
			Key:   KeyMacipACL(e.keyName()),
			Value: MacipACL{Name: e.keyName(), Rules: rules}.Proto(),
			Meta:  MacipMeta{ACLIndex: e.Index},
		})
	}
	return out, nil
}

type ownedMacipACL struct {
	Index uint32
	Name  string
	Dup   bool // another MACIP ACL with the same tag has a lower index
	Rules []acl_types.MacipACLRule
}

func (e ownedMacipACL) keyName() string { return dupName(e.Name, e.Index, e.Dup) }

func dumpOwnedMacipACLs(ctx context.Context, c vpp.Client, owner string) ([]ownedMacipACL, error) {
	stream, err := acl.NewServiceClient(c).MacipACLDump(ctx, &acl.MacipACLDump{ACLIndex: noACL})
	if err != nil {
		return nil, fmt.Errorf("macip_acl_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("macip_acl_dump: %w", err)
	}
	var out []ownedMacipACL
	for _, det := range details {
		name, ok := vpp.ParseOwnerTag(det.Tag, owner)
		if !ok {
			continue
		}
		out = append(out, ownedMacipACL{Index: det.ACLIndex, Name: name, Rules: det.R})
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

func macipIndexByName(owned []ownedMacipACL) map[string]uint32 {
	m := make(map[string]uint32, len(owned))
	for _, e := range owned {
		if !e.Dup {
			m[e.Name] = e.Index
		}
	}
	return m
}

func macipNameByIndex(owned []ownedMacipACL) map[uint32]string {
	m := make(map[uint32]string, len(owned))
	for _, e := range owned {
		m[e.Index] = e.keyName()
	}
	return m
}
