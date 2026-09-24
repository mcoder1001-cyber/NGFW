package memif

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	memifapi "ngfw/agent/binapi/memif"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// MemifDescriptor implements memif.memif (memif_create_v2 / memif_delete). All fields are
// creation parameters → Update is a recreate. Ring and buffer sizes are VPP's defaults: the API
// only reports the values negotiated with the connected peer, so they cannot be reconciled.
type MemifDescriptor struct{ base } //nolint:revive // memif.MemifDescriptor sits next to memif.SocketDescriptor; "Descriptor" alone would be ambiguous

// NewMemif returns the descriptor for owner.
func NewMemif(c vpp.Client, owner string) *MemifDescriptor {
	return &MemifDescriptor{base{c, owner, ""}}
}

// Name implements scheduler.Descriptor.
func (*MemifDescriptor) Name() string { return MemifName }

// KeyOf implements scheduler.Descriptor.
func (*MemifDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MemifName, obj.(*Memif).GetName())
}

// Dependencies implements scheduler.Descriptor.
func (*MemifDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Memif)
	if o.GetSocket() == 0 {
		return nil
	}
	return []scheduler.Dependency{{Key: SocketKey(o.GetSocket())}}
}

// Create implements scheduler.Descriptor.
func (d *MemifDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Memif)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetName() == "" {
		return nil, errors.New("memif: name is mandatory")
	}
	if o.GetSocket() == 0 {
		// socket 0 is VPP's shared /run/vpp/memif.sock, owned by nobody (review L4)
		return nil, errors.New("memif: an owned memif.socket is mandatory (socket 0 is VPP's shared default)")
	}
	rep, err := d.svc().MemifCreateV2(ctx, &memifapi.MemifCreateV2{
		Role: memifapi.MemifRole(o.GetRole()), Mode: memifapi.MemifMode(o.GetMode()), ID: o.GetId(), SocketID: o.GetSocket(), //nolint:gosec // enum values 0-2
		NoZeroCopy: !o.GetZeroCopy(), // ring_size / buffer_size 0 → VPP defaults (1024 / 2048)
	})
	if err != nil {
		return nil, fmt.Errorf("memif_create_v2: %w", err)
	}
	idx := uint32(rep.SwIfIndex)
	if err := iface.SanitizeAndTag(ctx, d.client, d.owner, o.GetName(), idx); err != nil {
		// an untagged memif is invisible to Retrieve and blocks every retry (id in use): remove it (review M3)
		if _, derr := d.svc().MemifDelete(ctx, &memifapi.MemifDelete{SwIfIndex: rep.SwIfIndex}); derr != nil {
			return nil, fmt.Errorf("%w (and memif_delete of the untagged orphan %d: %v)", err, idx, derr)
		}
		return nil, err
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor.
func (*MemifDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *MemifDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	if _, err := d.svc().MemifDelete(ctx, &memifapi.MemifDelete{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("memif_delete: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *MemifDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().MemifDump(ctx, &memifapi.MemifDump{})
	if err != nil {
		return nil, fmt.Errorf("memif_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("memif_dump: %w", err)
		}
		key, ok := t.KeyFor(uint32(m.SwIfIndex))
		if !ok || key.Descriptor() != MemifName {
			continue
		}
		out = append(out, scheduler.KV{
			Key: key,
			Value: &Memif{
				Name: key.ID(), Id: m.ID, Socket: m.SocketID, Role: Role(m.Role), Mode: Mode(m.Mode), //nolint:gosec // enum values 0-2
				ZeroCopy: m.ZeroCopy,
			},
			Meta: iface.Meta{SwIfIndex: uint32(m.SwIfIndex)},
		})
	}
	return out, nil
}
