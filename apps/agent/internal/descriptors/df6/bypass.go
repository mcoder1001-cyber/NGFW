package df6

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// BypassSpec describes a per-interface "bypass" feature toggle (sw_interface_set_vxlan_bypass,
// sw_interface_set_vxlan_gpe_bypass, sw_interface_set_gtpu_bypass): enable/disable for IPv4
// and IPv6 on one interface. VPP has no dump for these features, so the descriptors built from
// it are write-only: Retrieve returns ErrRetrieveUnsupported and the doc table says so.
type BypassSpec[T proto.Message] struct {
	// Name is the descriptor name ("vxlan.bypass").
	Name string
	// Plugin is the VPP plugin for ErrPluginNotLoaded.
	Plugin string
	// Fields returns the interface name and the desired IPv4 / IPv6 state of obj.
	Fields func(obj T) (iface string, ipv4, ipv6 bool)
	// Set sends the enable/disable message for one family.
	Set func(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, ipv6, enable bool) error
}

// BypassDescriptor is the scheduler.Descriptor built from a BypassSpec.
type BypassDescriptor[T proto.Message] struct {
	spec   BypassSpec[T]
	client vpp.Client
	owner  string
}

// NewBypassDescriptor returns the descriptor for spec.
func NewBypassDescriptor[T proto.Message](spec BypassSpec[T], c vpp.Client, owner string) *BypassDescriptor[T] {
	return &BypassDescriptor[T]{spec: spec, client: c, owner: owner}
}

func (d *BypassDescriptor[T]) cast(obj proto.Message) (T, error) {
	t, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s: %w: %T", d.spec.Name, ErrBadValue, obj)
	}
	return t, nil
}

// Name implements scheduler.Descriptor.
func (d *BypassDescriptor[T]) Name() string { return d.spec.Name }

// KeyOf implements scheduler.Descriptor: "<name>/<interface>".
func (d *BypassDescriptor[T]) KeyOf(obj proto.Message) scheduler.Key {
	t, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	iface, _, _ := d.spec.Fields(t)
	if iface == "" {
		return scheduler.Join(d.spec.Name, "invalid")
	}
	return scheduler.Join(d.spec.Name, iface)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *BypassDescriptor[T]) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, err := d.cast(obj)
	if err != nil {
		return nil
	}
	iface, _, _ := d.spec.Fields(t)
	return InterfaceDeps(iface)
}

func (d *BypassDescriptor[T]) apply(ctx context.Context, idx interface_types.InterfaceIndex, v4, v6 bool, want4, want6 bool) error {
	if v4 != want4 {
		if err := d.spec.Set(ctx, d.client, idx, false, want4); err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
	}
	if v6 != want6 {
		if err := d.spec.Set(ctx, d.client, idx, true, want6); err != nil {
			return PluginError(d.spec.Plugin, fmt.Errorf("%s: %w", d.spec.Name, err))
		}
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *BypassDescriptor[T]) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	iface, v4, v6 := d.spec.Fields(t)
	if iface == "" {
		return nil, fmt.Errorf("%s: %w: interface is mandatory", d.spec.Name, ErrBadValue)
	}
	if !v4 && !v6 {
		return nil, fmt.Errorf("%s: %w: at least one of ipv4/ipv6 must be set", d.spec.Name, ErrBadValue)
	}
	ifs, err := DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Index(iface)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.spec.Name, err)
	}
	if err := d.apply(ctx, idx, false, false, v4, v6); err != nil {
		return nil, err
	}
	return IfMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: toggles the families that changed in place.
func (d *BypassDescriptor[T]) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	m, err := IfMetaOf(d.spec.Name, meta)
	if err != nil {
		return nil, err
	}
	oi, o4, o6 := d.spec.Fields(o)
	ni, n4, n6 := d.spec.Fields(n)
	if oi != ni {
		return nil, scheduler.ErrRecreate
	}
	if !n4 && !n6 {
		return nil, fmt.Errorf("%s: %w: at least one of ipv4/ipv6 must be set", d.spec.Name, ErrBadValue)
	}
	if err := d.apply(ctx, interface_types.InterfaceIndex(m.SwIfIndex), o4, o6, n4, n6); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: disables the enabled families.
func (d *BypassDescriptor[T]) Delete(ctx context.Context, obj proto.Message, meta any) error {
	t, err := d.cast(obj)
	if err != nil {
		return err
	}
	m, err := IfMetaOf(d.spec.Name, meta)
	if err != nil {
		return err
	}
	_, v4, v6 := d.spec.Fields(t)
	return d.apply(ctx, interface_types.InterfaceIndex(m.SwIfIndex), v4, v6, false, false)
}

// Retrieve implements scheduler.Descriptor: VPP has no dump for bypass features.
func (d *BypassDescriptor[T]) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", d.spec.Name, ErrRetrieveUnsupported)
}
