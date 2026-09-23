package iface

// Logical interface names (D-065, D-069)
//
// Every interface has ONE logical name, the name config, UI and every consumer use and the id of
// its alias key "interface/<name>":
//
//   - interfaces this agent creates: the id of the creator's key, which is also the owner-tag id
//     ("w2-tap40" for tag "w2:w2-tap40", "loop201", "w2-tap11.100");
//   - physical / pre-existing interfaces (DPDK NICs, anything no descriptor created, i.e.
//     untagged): VPP's interface name. F-startup-gen names DPDK NICs by their logical name
//     (dpdk { dev <pci> { name lan } }), so VPP's name and the logical name coincide;
//   - interfaces tagged by ANOTHER owner have no logical name for this owner: resolution refuses
//     them (ErrForeignInterface), Retrieve never reports them.
//
// ResolveName / Table.IndexByName is the one resolver (logical name → sw_if_index) exported for
// every descriptor package (DF-1…DF-8). local0 is never resolvable.
//
// Canonical references. Every DF-1 model field that names an interface accepts either the
// creator key ("tapv2.tap/w2-tap40", DF-1-internal) or the alias key ("interface/w2-tap40",
// what consumers and P08 write). Retrieve always reports the alias form and each descriptor's
// Normalize maps creator keys to it (CanonicalRef), so desired and actual compare equal.
//
// Untagged interfaces carry no owner, so per-interface objects on them (an MTU, a bond
// membership, …) would be invisible to Retrieve. As in DF-4 (acl ClaimStore), a descriptor that
// configures an untagged interface records a claim (interface name, descriptor name) in the
// owner's ClaimStore on Create and releases it on Delete; Retrieve reports the object iff the
// claim exists. No DF-1 descriptor ever deletes an interface it did not create (creators delete
// only their own tagged interfaces), so a physical NIC is never removed.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ErrForeignInterface is returned when a name resolves to an interface tagged by another owner.
var ErrForeignInterface = errors.New("iface: interface belongs to another owner")

// VPPName is the interface name as VPP reports it (NUL padding removed).
func (t *Table) VPPName(idx uint32) string {
	d, ok := t.byIndex[idx]
	if !ok {
		return ""
	}
	return strings.TrimRight(d.InterfaceName, "\x00")
}

// Untagged reports whether idx exists, carries no tag and is not local0.
func (t *Table) Untagged(idx uint32) bool {
	d, ok := t.byIndex[idx]
	return ok && strings.TrimRight(d.Tag, "\x00") == "" && t.VPPName(idx) != "local0" && idx != 0
}

// Logical returns the logical name of idx for this owner: the owner-tag id of our interfaces,
// VPP's name for untagged ones; false for other owners' interfaces, local0 and unknown indexes.
func (t *Table) Logical(idx uint32) (string, bool) {
	if id, ok := t.OwnedID(idx); ok {
		return id, true
	}
	if t.Untagged(idx) {
		return t.VPPName(idx), true
	}
	return "", false
}

// Ref is the canonical reference of idx: "interface/<logical name>".
func (t *Table) Ref(idx uint32) (scheduler.Key, bool) {
	n, ok := t.Logical(idx)
	if !ok {
		return "", false
	}
	return AliasKey(n), true
}

// IndexByName resolves a logical name: this owner's tag id first, then an untagged interface
// with that VPP name. A name that only matches an interface tagged by another owner fails with
// ErrForeignInterface, anything else (incl. local0) with ErrNotFound.
func (t *Table) IndexByName(name string) (uint32, error) {
	if name == "" {
		return 0, fmt.Errorf("%w: empty interface name", ErrNotFound)
	}
	for _, idx := range t.order {
		if id, ok := t.OwnedID(idx); ok && id == name {
			return idx, nil
		}
	}
	for _, idx := range t.order {
		if t.VPPName(idx) != name || name == "local0" {
			continue
		}
		if t.Untagged(idx) {
			return idx, nil
		}
		if id, ours := t.OwnedID(idx); ours {
			// our own interface named by VPP's (index-based) name: its logical name is the tag id
			return 0, fmt.Errorf("%w: %q is VPP's name of our interface %q; use the logical name", ErrNotFound, name, id)
		}
		return 0, fmt.Errorf("%w: %q is tagged %q", ErrForeignInterface, name, strings.TrimRight(t.byIndex[idx].Tag, "\x00"))
	}
	return 0, fmt.Errorf("%w: %q (owner %q)", ErrNotFound, name, t.owner)
}

// ResolveName is the exported resolver (D-069): one sw_interface_dump, then IndexByName.
func ResolveName(ctx context.Context, client vpp.Client, owner, name string) (uint32, error) {
	t, err := Dump(ctx, client, owner)
	if err != nil {
		return 0, err
	}
	return t.IndexByName(name)
}

// CanonicalRef maps an interface reference to its canonical alias form: a creator key
// "<descriptor>/<id>" becomes "interface/<id>"; alias keys and invalid references are returned
// unchanged (the latter fail in Create).
func CanonicalRef(ref string) string {
	k, err := ParseRef(ref)
	if err != nil || k.Descriptor() == AliasName {
		return ref
	}
	return string(AliasKey(k.ID()))
}

// ---------------------------------------------------------------------------- claims

// ClaimStore records which descriptor configured which untagged interface (see the package
// comment above). Implementations must be safe for concurrent use; P05 may install a store
// persisted in the agent state dir with SetClaimStore so claims survive an agent restart.
type ClaimStore interface {
	Claim(ifName, holder string) error
	Release(ifName, holder string) error
	Claimed(ifName, holder string) bool
}

// NewMemoryClaimStore returns an in-memory ClaimStore.
func NewMemoryClaimStore() ClaimStore { return &memClaims{m: map[[2]string]struct{}{}} }

type memClaims struct {
	mu sync.Mutex
	m  map[[2]string]struct{}
}

func (c *memClaims) Claim(ifName, holder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[[2]string{ifName, holder}] = struct{}{}
	return nil
}

func (c *memClaims) Release(ifName, holder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{ifName, holder})
	return nil
}

func (c *memClaims) Claimed(ifName, holder string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[[2]string{ifName, holder}]
	return ok
}

var (
	claimsMu sync.Mutex
	claimsBy = map[string]ClaimStore{}
)

// SetClaimStore installs the ClaimStore of owner (P05: a store persisted in the state dir). Call
// it before the first transaction; nil restores a fresh in-memory store.
func SetClaimStore(owner string, s ClaimStore) {
	claimsMu.Lock()
	defer claimsMu.Unlock()
	if s == nil {
		s = NewMemoryClaimStore()
	}
	claimsBy[owner] = s
}

// Claims returns the ClaimStore of owner (in-memory by default, shared by every DF-1 descriptor
// of that owner).
func Claims(owner string) ClaimStore {
	claimsMu.Lock()
	defer claimsMu.Unlock()
	s, ok := claimsBy[owner]
	if !ok {
		s = NewMemoryClaimStore()
		claimsBy[owner] = s
	}
	return s
}

// Owns reports whether a per-interface object of descriptor holder on idx is ours: the interface
// carries our tag, or it is untagged and holder claimed it.
func (t *Table) Owns(idx uint32, holder string) bool {
	if _, ok := t.OwnedID(idx); ok {
		return true
	}
	return t.Untagged(idx) && Claims(t.owner).Claimed(t.VPPName(idx), holder)
}

// ClaimIfUntagged records holder's claim when idx is untagged (no-op for our tagged interfaces).
func (t *Table) ClaimIfUntagged(idx uint32, holder string) error {
	if !t.Untagged(idx) {
		return nil
	}
	return Claims(t.owner).Claim(t.VPPName(idx), holder)
}

// ReleaseRef releases holder's claim on the interface ref names (no-op when there is none).
func ReleaseRef(owner, ref, holder string) error {
	return Claims(owner).Release(RefID(ref), holder)
}

// OwnedRef is Ref for interfaces on which holder's per-interface objects are ours (Owns).
func (t *Table) OwnedRef(idx uint32, holder string) (scheduler.Key, bool) {
	if !t.Owns(idx, holder) {
		return "", false
	}
	return t.Ref(idx)
}

// Normalizer is P05's optional scheduler extension (task/P05 scheduler.Normalizer), declared here
// so DF-1 descriptors can assert it at compile time before P05 is merged.
type Normalizer interface {
	Normalize(obj proto.Message) proto.Message
}

// NormalizeRefs returns a copy of m whose string fields named in fields (interface references)
// are in canonical form (CanonicalRef). Unknown field names are ignored; m is never modified.
func NormalizeRefs(m proto.Message, fields ...string) proto.Message {
	if m == nil {
		return m
	}
	out := proto.Clone(m)
	r := out.ProtoReflect()
	for _, f := range fields {
		fd := r.Descriptor().Fields().ByName(protoreflect.Name(f))
		if fd == nil || fd.Kind() != protoreflect.StringKind || fd.IsList() {
			continue
		}
		if v := r.Get(fd).String(); v != "" {
			r.Set(fd, protoreflect.ValueOfString(CanonicalRef(v)))
		}
	}
	return out
}

// Normalize runs d's Normalize when it has one (test helper for proto.Equal against Retrieve).
func Normalize(d any, m proto.Message) proto.Message {
	if n, ok := d.(Normalizer); ok {
		return n.Normalize(m)
	}
	return m
}
