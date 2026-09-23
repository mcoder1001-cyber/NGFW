package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Spd manages security policy databases (ipsec_spd_add_del; dump ipsec_spds_dump).
type Spd struct{ cfg Config }

// SpdMeta is the runtime handle of an SPD (the id is the handle).
type SpdMeta struct{ SpdID uint32 }

// NewSpd returns the descriptor.
func NewSpd(cfg Config) *Spd { return &Spd{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Spd) Name() string { return SpdName }

// KeyOf implements scheduler.Descriptor: ipsec.spd/<spd_id>.
func (*Spd) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecSpd)
	return scheduler.Join(SpdName, vpn.Uint(o.GetSpdId()))
}

// Dependencies implements scheduler.Descriptor (none).
func (*Spd) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *Spd) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSpd)
	if !ok {
		return nil, typeErr(SpdName, obj)
	}
	if err := d.cfg.IDs.Check("spd", o.GetSpdId()); err != nil {
		return nil, err
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: true, SpdID: o.GetSpdId()}); err != nil {
		return nil, fmt.Errorf("ipsec_spd_add_del: %w", err)
	}
	return SpdMeta{SpdID: o.GetSpdId()}, nil
}

// Update implements scheduler.Descriptor: an SPD has no mutable field.
func (*Spd) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *Spd) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.IpsecSpd)
	if !ok {
		return typeErr(SpdName, obj)
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: false, SpdID: o.GetSpdId()}); err != nil {
		return fmt.Errorf("ipsec_spd_add_del: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: every SPD whose id is in the owned range.
func (d *Spd) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ids, err := d.ownedSpdIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(ids))
	for _, id := range ids {
		v := &vpnpb.IpsecSpd{SpdId: id}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: SpdMeta{SpdID: id}})
	}
	return out, nil
}

// ownedSpdIDs dumps ipsec_spds_dump and keeps the owned ids, ascending.
func (d *Spd) ownedSpdIDs(ctx context.Context) ([]uint32, error) {
	return ownedSpdIDs(ctx, d.cfg)
}

func ownedSpdIDs(ctx context.Context, cfg Config) ([]uint32, error) {
	stream, err := ipsec.NewServiceClient(cfg.Client).IpsecSpdsDump(ctx, &ipsec.IpsecSpdsDump{})
	if err != nil {
		return nil, fmt.Errorf("ipsec_spds_dump: %w", err)
	}
	var ids []uint32
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_spds_dump: %w", err)
		}
		if cfg.IDs.Contains(det.SpdID) {
			ids = append(ids, det.SpdID)
		}
	}
	sortUint32(ids)
	return ids, nil
}

func sortUint32(s []uint32) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
