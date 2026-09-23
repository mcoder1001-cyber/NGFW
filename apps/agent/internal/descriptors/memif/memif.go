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
// creation parameters → Update is a recreate.
type MemifDescriptor struct{ base }

// NewMemif returns the descriptor for owner.
func NewMemif(c vpp.Client, owner string) *MemifDescriptor { return &MemifDescriptor{base{c, owner, ""}} }

func (*MemifDescriptor) Name() string { return MemifName }

func (*MemifDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MemifName, obj.(*Memif).GetName())
}

func (*MemifDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Memif)
	if o.GetSocket() == 0 {
		return nil
	}
	return []scheduler.Dependency{{Key: SocketKey(o.GetSocket())}}
}

func (d *MemifDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Memif)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetName() == "" {
		return nil, errors.New("memif: name is mandatory")
	}
	if o.GetRingSize() == 0 || o.GetRingSize()&(o.GetRingSize()-1) != 0 || o.GetBufferSize() == 0 || o.GetBufferSize() > 0xffff {
		return nil, fmt.Errorf("memif: ring_size must be a power of two and buffer_size 1..65535 (got %d/%d)", o.GetRingSize(), o.GetBufferSize())
	}
	rep, err := d.svc().MemifCreateV2(ctx, &memifapi.MemifCreateV2{
		Role: memifapi.MemifRole(o.GetRole()), Mode: memifapi.MemifMode(o.GetMode()), ID: o.GetId(), SocketID: o.GetSocket(), //nolint:gosec // enum values 0-2
		RingSize: o.GetRingSize(), BufferSize: uint16(o.GetBufferSize()), NoZeroCopy: !o.GetZeroCopy(), //nolint:gosec // range-checked
	})
	if err != nil {
		return nil, fmt.Errorf("memif_create_v2: %w", err)
	}
	idx := uint32(rep.SwIfIndex)
	if err := iface.Tag(ctx, d.client, d.owner, o.GetName(), idx); err != nil {
		return iface.Meta{SwIfIndex: idx}, err
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

func (*MemifDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

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
				RingSize: m.RingSize, BufferSize: uint32(m.BufferSize), ZeroCopy: m.ZeroCopy,
			},
			Meta: iface.Meta{SwIfIndex: uint32(m.SwIfIndex)},
		})
	}
	return out, nil
}
