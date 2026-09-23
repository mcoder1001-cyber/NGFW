package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Backend selects the crypto backend per protocol (ipsec_select_backend; dump
// ipsec_backend_dump). It is a global singleton: Retrieve always reports the active backend of
// each protocol, Create/Update select by name, Delete leaves VPP as it is.
type Backend struct{ cfg Config }

// BackendMeta is the VPP index of the selected backend.
type BackendMeta struct{ Index uint8 }

// NewBackend returns the descriptor.
func NewBackend(cfg Config) *Backend { return &Backend{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Backend) Name() string { return BackendName }

// KeyOf implements scheduler.Descriptor: ipsec.backend/<protocol>.
func (*Backend) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecBackend)
	return scheduler.Join(BackendName, o.GetProtocol())
}

// Dependencies implements scheduler.Descriptor (none).
func (*Backend) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Backend) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecBackend)
	if !ok {
		return nil, typeErr(BackendName, obj)
	}
	return d.selectBackend(ctx, o)
}

// Update implements scheduler.Descriptor.
func (d *Backend) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a singleton cannot be absent; VPP keeps its selection.
func (*Backend) Delete(context.Context, proto.Message, any) error { return nil }

func (d *Backend) selectBackend(ctx context.Context, o *vpnpb.IpsecBackend) (any, error) {
	protoV, err := protocols.value("protocol", o.GetProtocol())
	if err != nil {
		return nil, err
	}
	all, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, b := range all {
		if b.Protocol != protoV {
			continue
		}
		if b.Name == o.GetName() {
			if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSelectBackend(ctx, &ipsec.IpsecSelectBackend{Protocol: protoV, Index: b.Index}); err != nil {
				return nil, fmt.Errorf("ipsec_select_backend: %w", err)
			}
			return BackendMeta{Index: b.Index}, nil
		}
		names = append(names, b.Name)
	}
	return nil, fmt.Errorf("ipsec: no %s backend named %q (have %s)", o.GetProtocol(), o.GetName(), strings.Join(names, ", "))
}

// Retrieve implements scheduler.Descriptor: the active backend of every protocol.
func (d *Backend) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	all, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, b := range all {
		if !b.Active {
			continue
		}
		v := &vpnpb.IpsecBackend{Protocol: protocols.name(b.Protocol), Name: b.Name}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: BackendMeta{Index: b.Index}})
	}
	sortKVs(out)
	return out, nil
}

type backend struct {
	Name     string
	Protocol ipsec_types.IpsecProto
	Index    uint8
	Active   bool
}

func (d *Backend) dump(ctx context.Context) ([]backend, error) {
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecBackendDump(ctx, &ipsec.IpsecBackendDump{})
	if err != nil {
		return nil, fmt.Errorf("ipsec_backend_dump: %w", err)
	}
	var out []backend
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_backend_dump: %w", err)
		}
		out = append(out, backend{Name: strings.TrimRight(det.Name, "\x00"), Protocol: det.Protocol, Index: det.Index, Active: det.Active})
	}
}
