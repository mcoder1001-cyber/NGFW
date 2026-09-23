// Package adl implements the descriptors of VPP's adl plugin (allow/deny lists, D2.4):
// the per-interface ADL switch and the per-interface allow-list table binding. The plugin
// has no dump message, so both descriptors are write-only: Retrieve returns
// df2.ErrRetrieveUnsupported (docs/agent/descriptors/adl.md, DF-2-questions.md). Messages
// come from apps/agent/binapi/adl only.
package adl

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	adlapi "ngfw/agent/binapi/adl"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names; keys are "adl.interface/<interface>" and "adl.allowlist/<interface>".
const (
	InterfaceName = "adl.interface"
	AllowlistName = "adl.allowlist"
)

// Meta is the runtime handle of both descriptors.
type Meta struct{ SwIfIndex uint32 }

func resolve(ctx context.Context, c vpp.Client, owner, name string) (interface_types.InterfaceIndex, error) {
	ifs, err := df2.DumpInterfaces(ctx, c, owner)
	if err != nil {
		return 0, err
	}
	return ifs.Index(name)
}

// InterfaceDescriptor enables the ADL input feature (adl_interface_enable_disable).
type InterfaceDescriptor struct {
	client vpp.Client
	owner  string
}

// NewInterface returns the descriptor for the given owner.
func NewInterface(c vpp.Client, owner string) *InterfaceDescriptor {
	return &InterfaceDescriptor{client: c, owner: owner}
}

// Name implements scheduler.Descriptor.
func (*InterfaceDescriptor) Name() string { return InterfaceName }

// KeyOf implements scheduler.Descriptor.
func (*InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(InterfaceName, obj.(*Interface).GetInterface())
}

// Dependencies implements scheduler.Descriptor.
func (*InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{df2.InterfaceDep(obj.(*Interface).GetInterface())}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx interface_types.InterfaceIndex, enable bool) error {
	if _, err := adlapi.NewServiceClient(d.client).AdlInterfaceEnableDisable(ctx, &adlapi.AdlInterfaceEnableDisable{SwIfIndex: idx, EnableDisable: enable}); err != nil {
		return fmt.Errorf("adl_interface_enable_disable: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	idx, err := resolve(ctx, d.client, d.owner, obj.(*Interface).GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, true); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: the interface is the key.
func (*InterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *InterfaceDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", InterfaceName, df2.ErrBadMeta, meta)
	}
	return d.set(ctx, interface_types.InterfaceIndex(m.SwIfIndex), false)
}

// Retrieve is unsupported: the adl plugin has no dump.
func (*InterfaceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", InterfaceName, df2.ErrRetrieveUnsupported)
}

// AllowlistDescriptor binds an interface's ADL check to a FIB table of allowed prefixes
// (adl_allowlist_enable_disable).
type AllowlistDescriptor struct {
	client vpp.Client
	owner  string
}

// NewAllowlist returns the descriptor for the given owner.
func NewAllowlist(c vpp.Client, owner string) *AllowlistDescriptor {
	return &AllowlistDescriptor{client: c, owner: owner}
}

// Name implements scheduler.Descriptor.
func (*AllowlistDescriptor) Name() string { return AllowlistName }

// KeyOf implements scheduler.Descriptor.
func (*AllowlistDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(AllowlistName, obj.(*Allowlist).GetInterface())
}

// Dependencies implements scheduler.Descriptor: the interface (with ADL enabled on it) and
// the allow-list table.
func (*AllowlistDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	a := obj.(*Allowlist)
	deps := []scheduler.Dependency{
		df2.InterfaceDep(a.GetInterface()),
		{Key: scheduler.Join(InterfaceName, a.GetInterface()), Optional: true},
	}
	return append(deps, df2.VRFDeps(a.GetFibId())...)
}

func (d *AllowlistDescriptor) set(ctx context.Context, a *Allowlist, idx interface_types.InterfaceIndex, enable bool) error {
	req := &adlapi.AdlAllowlistEnableDisable{SwIfIndex: idx, FibID: a.GetFibId(), DefaultAdl: a.GetDefaultAdl()}
	if enable {
		req.IP4, req.IP6 = a.GetIp4(), a.GetIp6()
	}
	if _, err := adlapi.NewServiceClient(d.client).AdlAllowlistEnableDisable(ctx, req); err != nil {
		return fmt.Errorf("adl_allowlist_enable_disable: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *AllowlistDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a := obj.(*Allowlist)
	if !a.GetIp4() && !a.GetIp6() {
		return nil, fmt.Errorf("%s: at least one of ip4/ip6 must be set", AllowlistName)
	}
	idx, err := resolve(ctx, d.client, d.owner, a.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, a, idx, true); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: uint32(idx)}, nil
}

// Update re-applies in place; a different interface is a different object.
func (d *AllowlistDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(Meta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", AllowlistName, df2.ErrBadMeta, meta)
	}
	a := newObj.(*Allowlist)
	if !a.GetIp4() && !a.GetIp6() {
		return nil, fmt.Errorf("%s: at least one of ip4/ip6 must be set", AllowlistName)
	}
	if err := d.set(ctx, a, interface_types.InterfaceIndex(m.SwIfIndex), true); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete clears both families (ip4 = ip6 = false disables the allow-list).
func (d *AllowlistDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", AllowlistName, df2.ErrBadMeta, meta)
	}
	return d.set(ctx, obj.(*Allowlist), interface_types.InterfaceIndex(m.SwIfIndex), false)
}

// Retrieve is unsupported: the adl plugin has no dump.
func (*AllowlistDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", AllowlistName, df2.ErrRetrieveUnsupported)
}

// Register registers the adl descriptors (interface, allowlist) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewInterface(c, owner))
	r.Register(NewAllowlist(c, owner))
}
