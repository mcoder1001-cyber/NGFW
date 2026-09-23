package acl

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// KeyEtypeWhitelist is the key of the ethertype whitelist on ifName: "acl.etype-whitelist/<ifName>".
func KeyEtypeWhitelist(ifName string) scheduler.Key {
	return scheduler.Join(NameEtypeWhitelist, ifName)
}

// ErrForeignInterface is returned (wrapped) when a whitelist names an interface tagged by
// another owner.
var ErrForeignInterface = errors.New("acl: interface belongs to another owner")

// ClaimStore records the untagged interfaces (physical ports, which have no owner tag) that
// this owner has applied an ethertype whitelist to, so Retrieve can report those whitelists as
// ours. Whitelists carry no tag and reference no ACL, so without it a whitelist on an untagged
// interface would be invisible: re-created on every reconcile and never deleted. Implementations
// must be safe for concurrent use. P05 may pass a store persisted in the agent state dir
// (WithEtypeClaims) so the claims survive an agent restart.
type ClaimStore interface {
	Claim(ifName string) error
	Release(ifName string) error
	Claimed(ifName string) bool
}

// NewMemoryClaimStore returns the default in-memory ClaimStore.
func NewMemoryClaimStore() ClaimStore { return &memClaims{m: map[string]struct{}{}} }

type memClaims struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func (c *memClaims) Claim(ifName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[ifName] = struct{}{}
	return nil
}

func (c *memClaims) Release(ifName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, ifName)
	return nil
}

func (c *memClaims) Claimed(ifName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[ifName]
	return ok
}

// EtypeWhitelistDescriptor manages acl.etype-whitelist objects with
// acl_interface_set_etype_whitelist / acl_interface_etype_whitelist_dump. Ownership (whitelists
// have no tag and reference no ACL): a whitelist is ours when its interface carries this
// owner's tag, or when the interface is untagged and this descriptor applied the whitelist
// (recorded in the ClaimStore). Interfaces tagged by another owner are refused.
type EtypeWhitelistDescriptor struct {
	client vpp.Client
	owner  string
	opts   options
}

var _ scheduler.Descriptor = (*EtypeWhitelistDescriptor)(nil)

// NewEtypeWhitelist returns the acl.etype-whitelist descriptor for one owner.
func NewEtypeWhitelist(client vpp.Client, owner string, opts ...Option) *EtypeWhitelistDescriptor {
	o := buildOptions(opts)
	if o.claims == nil {
		o.claims = NewMemoryClaimStore()
	}
	return &EtypeWhitelistDescriptor{client: client, owner: owner, opts: o}
}

// owns reports whether a whitelist on iface belongs to this owner (see the type doc).
func (d *EtypeWhitelistDescriptor) owns(iface ifaceInfo) bool {
	if _, ours := vpp.ParseOwnerTag(iface.Tag, d.owner); ours {
		return true
	}
	return iface.Tag == "" && d.opts.claims.Claimed(iface.Name)
}

// Name implements scheduler.Descriptor.
func (*EtypeWhitelistDescriptor) Name() string { return NameEtypeWhitelist }

// KeyOf implements scheduler.Descriptor: acl.etype-whitelist/<interface>.
func (*EtypeWhitelistDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	w, _ := EtypeWhitelistFromProto(obj)
	return KeyEtypeWhitelist(w.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface (optional; see WithInterfaceKey).
func (d *EtypeWhitelistDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	w, _ := EtypeWhitelistFromProto(obj)
	return []scheduler.Dependency{d.opts.interfaceDependency(w.Interface)}
}

func (d *EtypeWhitelistDescriptor) set(ctx context.Context, swIfIndex uint32, w EtypeWhitelist) error {
	if err := w.validateLists(); err != nil {
		return err
	}
	list := make([]uint16, 0, len(w.Input)+len(w.Output))
	list = append(append(list, w.Input...), w.Output...)
	req := &acl.ACLInterfaceSetEtypeWhitelist{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		NInput:    uint8(len(w.Input)), //nolint:gosec // Validate bounds the lists at 255
		Whitelist: list,
	}
	if _, err := acl.NewServiceClient(d.client).ACLInterfaceSetEtypeWhitelist(ctx, req); err != nil {
		return fmt.Errorf("acl_interface_set_etype_whitelist %q: %w", w.Interface, err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *EtypeWhitelistDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	w, err := EtypeWhitelistFromProto(obj)
	if err != nil {
		return nil, err
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	ifaces, err := dumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	swIfIndex, err := ifaces.index(w.Interface)
	if err != nil {
		return nil, err
	}
	iface := ifaces.byIndex[swIfIndex]
	if iface.Tag != "" {
		if _, ours := vpp.ParseOwnerTag(iface.Tag, d.owner); !ours {
			return nil, fmt.Errorf("%w: %q is tagged %q", ErrForeignInterface, w.Interface, iface.Tag)
		}
	} else if err := d.opts.claims.Claim(w.Interface); err != nil {
		return nil, fmt.Errorf("acl.etype-whitelist: claim %q: %w", w.Interface, err)
	}
	if err := d.set(ctx, swIfIndex, w); err != nil {
		if iface.Tag == "" {
			_ = d.opts.claims.Release(w.Interface)
		}
		return nil, err
	}
	return BindingMeta{SwIfIndex: swIfIndex}, nil
}

// Update implements scheduler.Descriptor: the new lists replace the old ones in place.
func (d *EtypeWhitelistDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldW, err := EtypeWhitelistFromProto(oldObj)
	if err != nil {
		return nil, err
	}
	newW, err := EtypeWhitelistFromProto(newObj)
	if err != nil {
		return nil, err
	}
	if oldW.Interface != newW.Interface {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(BindingMeta)
	if !ok {
		return nil, fmt.Errorf("acl.etype-whitelist: unexpected meta %T", meta)
	}
	if err := d.set(ctx, m.SwIfIndex, newW); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: empty lists clear the whitelist; the claim on an
// untagged interface is released.
func (d *EtypeWhitelistDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	w, err := EtypeWhitelistFromProto(obj)
	if err != nil {
		return err
	}
	m, ok := meta.(BindingMeta)
	if !ok {
		return fmt.Errorf("acl.etype-whitelist: unexpected meta %T", meta)
	}
	if err := d.set(ctx, m.SwIfIndex, EtypeWhitelist{Interface: w.Interface}); err != nil {
		return err
	}
	if err := d.opts.claims.Release(w.Interface); err != nil {
		return fmt.Errorf("acl.etype-whitelist: release %q: %w", w.Interface, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: acl_interface_etype_whitelist_dump for all
// interfaces, keeping non-empty whitelists on interfaces this owner owns (see the type doc).
func (d *EtypeWhitelistDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifaces, err := dumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := acl.NewServiceClient(d.client).ACLInterfaceEtypeWhitelistDump(ctx, &acl.ACLInterfaceEtypeWhitelistDump{SwIfIndex: interface_types.InterfaceIndex(allInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("acl_interface_etype_whitelist_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_interface_etype_whitelist_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		if len(det.Whitelist) == 0 || int(det.NInput) > len(det.Whitelist) {
			continue
		}
		iface, known := ifaces.byIndex[uint32(det.SwIfIndex)]
		if !known {
			continue
		}
		if !d.owns(iface) {
			continue
		}
		w := EtypeWhitelist{
			Interface: iface.Name,
			Input:     append([]uint16{}, det.Whitelist[:det.NInput]...),
			Output:    append([]uint16{}, det.Whitelist[det.NInput:]...),
		}
		out = append(out, scheduler.KV{Key: KeyEtypeWhitelist(w.Interface), Value: w.Proto(), Meta: BindingMeta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}
