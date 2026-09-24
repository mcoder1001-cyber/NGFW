package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Itf manages ipsec interfaces (ipsec_itf_create / _delete; dump ipsec_itf_dump). VPP names the
// interface ipsec<instance>; the owner tag is "<owner>:ipsec<instance>", so the logical name
// (D-069: the tag id) and VPP's name coincide. VPP refuses an existing instance, so an interface
// that is not ours is never adopted.
type Itf struct{ cfg Config }

// ItfMeta is the runtime handle of an ipsec interface.
type ItfMeta struct{ SwIfIndex uint32 }

// NewItf returns the descriptor.
func NewItf(cfg Config) *Itf { return &Itf{cfg: cfg} }

// ItfInterfaceName is the VPP name of the ipsec interface with the given instance.
func ItfInterfaceName(instance uint32) string { return "ipsec" + vpn.Uint(instance) }

// Name implements scheduler.Descriptor.
func (*Itf) Name() string { return ItfName }

// KeyOf implements scheduler.Descriptor: ipsec.itf/ipsec<instance>.
func (*Itf) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecItf)
	return scheduler.Join(ItfName, ItfInterfaceName(o.GetInstance()))
}

// Dependencies implements scheduler.Descriptor (none).
func (*Itf) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// ProvidedKeys lets the ipsec interface satisfy the cross-plugin alias interface/ipsec<instance>
// (D-065; P05's optional KeyProvider extension), so tunnel-protect, SPD bindings and IKEv2 profiles
// that reference it by name order after it.
func (*Itf) ProvidedKeys(obj proto.Message) []scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecItf)
	return []scheduler.Key{vpn.InterfaceKey(ItfInterfaceName(o.GetInstance()))}
}

// Create implements scheduler.Descriptor.
func (d *Itf) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecItf)
	if !ok {
		return nil, typeErr(ItfName, obj)
	}
	mode, ok := tunnelModes[o.GetMode()]
	if !ok {
		return nil, fmt.Errorf("ipsec: itf mode %q (want p2p|p2mp)", o.GetMode())
	}
	if o.GetInstance() == noInterface {
		return nil, errors.New("ipsec: itf needs an explicit instance")
	}
	svc := ipsec.NewServiceClient(d.cfg.Client)
	rep, err := svc.IpsecItfCreate(ctx, &ipsec.IpsecItfCreate{Itf: ipsec.IpsecItf{UserInstance: o.GetInstance(), Mode: mode}})
	if err != nil {
		return nil, fmt.Errorf("ipsec_itf_create: %w", err)
	}
	if err := vpn.TagInterface(ctx, d.cfg.Client, rep.SwIfIndex, d.cfg.Owner, ItfInterfaceName(o.GetInstance())); err != nil {
		_, _ = svc.IpsecItfDelete(ctx, &ipsec.IpsecItfDelete{SwIfIndex: rep.SwIfIndex})
		return nil, err
	}
	return ItfMeta{SwIfIndex: uint32(rep.SwIfIndex)}, nil
}

// Update implements scheduler.Descriptor: instance and mode are immutable.
func (*Itf) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before the delete it re-verifies that the index
// still carries our tag for this instance (D-071: indexes are reused after a VPP restart); an
// interface that is gone needs nothing (D-074).
func (d *Itf) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.IpsecItf)
	if !ok {
		return typeErr(ItfName, obj)
	}
	m, ok := meta.(ItfMeta)
	if !ok {
		return metaErr(ItfName, meta)
	}
	present, err := vpn.OwnedAt(ctx, d.cfg.Client, d.cfg.Owner, m.SwIfIndex, ItfInterfaceName(o.GetInstance()))
	if err != nil || !present {
		return err
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecItfDelete(ctx, &ipsec.IpsecItfDelete{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("ipsec_itf_delete (%s): %w", ItfInterfaceName(o.GetInstance()), err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: ipsec interfaces tagged by this owner with the tag id
// of their instance.
func (d *Itf) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecItfDump(ctx, &ipsec.IpsecItfDump{SwIfIndex: interface_types.InterfaceIndex(noInterface)})
	if err != nil {
		return nil, fmt.Errorf("ipsec_itf_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_itf_dump: %w", err)
		}
		idx := uint32(det.Itf.SwIfIndex)
		if id, owned := tbl.Owned(idx); !owned || id != ItfInterfaceName(det.Itf.UserInstance) {
			continue
		}
		v := &vpnpb.IpsecItf{Instance: det.Itf.UserInstance, Mode: tunnelModeName(det.Itf.Mode)}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: ItfMeta{SwIfIndex: idx}})
	}
	return sortKVs(out), nil
}
