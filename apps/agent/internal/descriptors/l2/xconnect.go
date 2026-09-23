package l2

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// XconnectDescriptor implements l2.xconnect (sw_interface_set_l2_xconnect). One object is one
// direction rx → tx, keyed on rx; a bidirectional cross-connect is two objects. This mirrors
// VPP (each rx interface has exactly one xconnect target) and lets Retrieve be a plain decoding
// of l2_xconnect_dump.
type XconnectDescriptor struct{ base }

// NewXconnect returns the descriptor for owner.
func NewXconnect(c vpp.Client, owner string) *XconnectDescriptor { return &XconnectDescriptor{base{c, owner}} }

// XcMeta holds both indexes: Delete needs rx, Update replaces tx in place.
type XcMeta struct{ Rx, Tx uint32 }

func (*XconnectDescriptor) Name() string { return XconnectName }

func (*XconnectDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(XconnectName, iface.RefID(obj.(*Xconnect).GetRx()))
}

func (*XconnectDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*Xconnect)
	return []scheduler.Dependency{{Key: scheduler.Key(o.GetRx())}, {Key: scheduler.Key(o.GetTx())}}
}

func (d *XconnectDescriptor) set(ctx context.Context, rx, tx uint32, enable bool) error {
	_, err := d.svc().SwInterfaceSetL2Xconnect(ctx, &l2api.SwInterfaceSetL2Xconnect{
		RxSwIfIndex: interface_types.InterfaceIndex(rx), TxSwIfIndex: interface_types.InterfaceIndex(tx), Enable: enable,
	})
	if err != nil {
		return fmt.Errorf("sw_interface_set_l2_xconnect: %w", err)
	}
	return nil
}

func (d *XconnectDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Xconnect)
	if !ok {
		return nil, ErrEmptyValue
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	rx, err := t.Index(o.GetRx())
	if err != nil {
		return nil, err
	}
	tx, err := t.Index(o.GetTx())
	if err != nil {
		return nil, err
	}
	return XcMeta{rx, tx}, d.set(ctx, rx, tx, true)
}

// Update replaces the tx side in place when rx is unchanged.
func (d *XconnectDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, ok := meta.(XcMeta)
	if !ok {
		return nil, fmt.Errorf("l2: unexpected meta %T", meta)
	}
	o, n := oldObj.(*Xconnect), newObj.(*Xconnect)
	if o.GetRx() != n.GetRx() {
		return nil, scheduler.ErrRecreate
	}
	tx, err := iface.Resolve(ctx, d.client, d.owner, n.GetTx())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, m.Rx, tx, true); err != nil {
		return nil, err
	}
	return XcMeta{m.Rx, tx}, nil
}

func (d *XconnectDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(XcMeta)
	if !ok {
		return fmt.Errorf("l2: unexpected meta %T", meta)
	}
	return d.set(ctx, m.Rx, m.Tx, false)
}

func (d *XconnectDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().L2XconnectDump(ctx, &l2api.L2XconnectDump{})
	if err != nil {
		return nil, fmt.Errorf("l2_xconnect_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		x, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("l2_xconnect_dump: %w", err)
		}
		rxKey, ok := t.KeyFor(uint32(x.RxSwIfIndex))
		if !ok {
			continue
		}
		txKey, ok := t.KeyFor(uint32(x.TxSwIfIndex))
		if !ok {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(XconnectName, rxKey.ID()),
			Value: &Xconnect{Rx: string(rxKey), Tx: string(txKey)},
			Meta:  XcMeta{uint32(x.RxSwIfIndex), uint32(x.TxSwIfIndex)},
		})
	}
	return out, nil
}
