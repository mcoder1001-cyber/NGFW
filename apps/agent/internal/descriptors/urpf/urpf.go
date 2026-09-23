// Package urpf implements the descriptor of VPP's urpf plugin: unicast reverse-path
// forwarding checks per interface, address family and direction. Messages come from
// apps/agent/binapi/urpf only.
package urpf

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	urpfapi "ngfw/agent/binapi/urpf"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Name is the descriptor name; keys are "urpf.interface/<interface>/<ipv4|ipv6>/<rx|tx>".
const Name = "urpf.interface"

// Descriptor manages uRPF checks (urpf_update_v2) on interfaces owned by this agent.
type Descriptor struct {
	client vpp.Client
	owner  string
	opts   df2.Options
}

// New returns the descriptor for the given owner.
func New(c vpp.Client, owner string, opts ...df2.Option) *Descriptor {
	return &Descriptor{client: c, owner: owner, opts: df2.BuildOptions(opts...)}
}

// Meta is the runtime handle.
type Meta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return Name }

func afID(af df2.AddressFamily) string {
	if af == df2.AddressFamily_IPV6 {
		return "ipv6"
	}
	return "ipv4"
}

func dirID(dir Interface_Direction) string {
	if dir == Interface_TX {
		return "tx"
	}
	return "rx"
}

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	u := obj.(*Interface)
	return scheduler.Join(Name, u.GetInterface(), afID(u.GetAf()), dirID(u.GetDirection()))
}

// Dependencies implements scheduler.Descriptor: the interface and the table (table 0 always
// exists and is not modelled).
func (*Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	u := obj.(*Interface)
	return append([]scheduler.Dependency{df2.InterfaceDep(u.GetInterface())}, df2.VRFDeps(u.GetTableId())...)
}

func (d *Descriptor) update(ctx context.Context, u *Interface, idx interface_types.InterfaceIndex, mode urpfapi.UrpfMode) error {
	req := &urpfapi.UrpfUpdateV2{
		IsInput:   u.GetDirection() == Interface_RX,
		Mode:      mode,
		Af:        df2.ToIPTypesAF(u.GetAf()),
		SwIfIndex: idx,
		TableID:   u.GetTableId(),
	}
	if _, err := urpfapi.NewServiceClient(d.client).UrpfUpdateV2(ctx, req); err != nil {
		return fmt.Errorf("urpf_update_v2: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	u := obj.(*Interface)
	if u.GetMode() == Interface_OFF {
		return nil, fmt.Errorf("%s: mode OFF is not a desired state (omit the object instead)", Name)
	}
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx32, untagged, err := ifs.Resolve(u.GetInterface())
	if err != nil {
		return nil, err
	}
	idx := interface_types.InterfaceIndex(idx32)
	if err := d.update(ctx, u, idx, urpfapi.UrpfMode(u.GetMode())); err != nil { //nolint:gosec // enum 0..2
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(obj)); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: idx32}, nil
}

// Update changes mode and table in place (urpf_update_v2 replaces the check); a different
// interface, family or direction is a different object.
func (d *Descriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(Meta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", Name, df2.ErrBadMeta, meta)
	}
	u := newObj.(*Interface)
	if u.GetMode() == Interface_OFF {
		return nil, fmt.Errorf("%s: mode OFF is not a desired state (omit the object instead)", Name)
	}
	if err := d.update(ctx, u, interface_types.InterfaceIndex(m.SwIfIndex), urpfapi.UrpfMode(u.GetMode())); err != nil { //nolint:gosec // enum 0..2
		return nil, err
	}
	return m, nil
}

// Delete switches the check off.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return fmt.Errorf("%s: %w %T", Name, df2.ErrBadMeta, meta)
	}
	if err := d.update(ctx, obj.(*Interface), interface_types.InterfaceIndex(m.SwIfIndex), urpfapi.URPF_API_MODE_OFF); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// Retrieve dumps every configured check (urpf_interface_dump) on owned interfaces.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := urpfapi.NewServiceClient(d.client).UrpfInterfaceDump(ctx, &urpfapi.UrpfInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(df2.NoInterface)})
	if err != nil {
		return nil, fmt.Errorf("urpf_interface_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("urpf_interface_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		if det.Mode == urpfapi.URPF_API_MODE_OFF {
			continue
		}
		name, ok := ifs.Name(uint32(det.SwIfIndex))
		if !ok {
			continue
		}
		v := &Interface{Interface: name, Af: df2.FromIPTypesAF(det.Af), Mode: Interface_Mode(det.Mode), TableId: det.TableID} //nolint:gosec // enum 0..2
		if !det.IsInput {
			v.Direction = Interface_TX
		}
		if !ifs.OwnsObject(uint32(det.SwIfIndex), d.KeyOf(v), d.opts.Claims) {
			continue
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: Meta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}

// Register registers the urpf descriptor with r; opts (df2.WithClaims) attribute checks on
// untagged interfaces.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(New(c, owner, opts...))
}
