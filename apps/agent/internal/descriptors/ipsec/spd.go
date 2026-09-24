package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
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

// spdRecordKey is the ownership record of an SPD (vpn.Records).
func spdRecordKey(id uint32) string { return string(scheduler.Join(SpdName, vpn.Uint(id))) }

// Create implements scheduler.Descriptor. VPP refuses an existing spd_id
// (ENTRY_ALREADY_EXISTS), so an SPD that is not ours is never adopted; the ownership record is
// written only after VPP accepted the add.
func (d *Spd) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSpd)
	if !ok {
		return nil, typeErr(SpdName, obj)
	}
	if err := d.cfg.IDs.Check("spd", o.GetSpdId()); err != nil {
		return nil, err
	}
	rec := d.cfg.records()
	id, err := rec.Identity(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SpdName, err)
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: true, SpdID: o.GetSpdId()}); err != nil {
		return nil, fmt.Errorf("ipsec_spd_add_del (spd %d): %w", o.GetSpdId(), err)
	}
	if err := rec.Put(id, spdRecordKey(o.GetSpdId()), "spd"); err != nil {
		return nil, fmt.Errorf("%s: record: %w", SpdName, err)
	}
	return SpdMeta{SpdID: o.GetSpdId()}, nil
}

// Update implements scheduler.Descriptor: an SPD has no mutable field.
func (*Spd) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before the delete it re-verifies that the SPD
// still exists and is ours on the running VPP instance (D-071/D-074): a vanished SPD is done, an
// SPD without our record is refused (ErrNotOurs).
func (d *Spd) Delete(ctx context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.IpsecSpd)
	if !ok {
		return typeErr(SpdName, obj)
	}
	owned, exists, err := ownedSpd(ctx, d.cfg, o.GetSpdId())
	if err != nil {
		return err
	}
	if !exists {
		return d.cfg.records().Drop(spdRecordKey(o.GetSpdId()))
	}
	if !owned {
		return fmt.Errorf("%s: spd %d: %w", SpdName, o.GetSpdId(), vpn.ErrNotOurs)
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: false, SpdID: o.GetSpdId()}); err != nil {
		return fmt.Errorf("ipsec_spd_add_del (spd %d, del): %w", o.GetSpdId(), err)
	}
	return d.cfg.records().Drop(spdRecordKey(o.GetSpdId()))
}

// Retrieve implements scheduler.Descriptor: every SPD in the owned range with our record.
func (d *Spd) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ids, err := ownedSpdIDs(ctx, d.cfg)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(ids))
	for _, id := range ids {
		v := &vpnpb.IpsecSpd{SpdId: id}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: SpdMeta{SpdID: id}})
	}
	return sortKVs(out), nil
}

// dumpSpdIDs returns every SPD id VPP has (ipsec_spds_dump), ascending, without duplicates.
func dumpSpdIDs(ctx context.Context, cfg Config) ([]uint32, error) {
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
		ids = append(ids, det.SpdID)
	}
	sortUint32(ids)
	return slices.Compact(ids), nil
}

// ownedSpdIDs returns the SPD ids that are ours: in the owned range and recorded on the running
// VPP instance.
func ownedSpdIDs(ctx context.Context, cfg Config) ([]uint32, error) {
	all, err := dumpSpdIDs(ctx, cfg)
	if err != nil {
		return nil, err
	}
	rec := cfg.records()
	var ids []uint32
	var bid bootid.Identity
	for _, id := range all {
		if !cfg.IDs.Contains(id) {
			continue
		}
		if bid.IsZero() {
			if bid, err = rec.Identity(ctx); err != nil {
				return nil, fmt.Errorf("%s: %w", SpdName, err)
			}
		}
		if _, ok := rec.Valid(bid, spdRecordKey(id)); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ownedSpd reports whether SPD id exists and whether it is ours.
func ownedSpd(ctx context.Context, cfg Config, id uint32) (owned, exists bool, err error) {
	all, err := dumpSpdIDs(ctx, cfg)
	if err != nil {
		return false, false, err
	}
	if !slices.Contains(all, id) {
		return false, false, nil
	}
	if !cfg.IDs.Contains(id) {
		return false, true, nil
	}
	bid, err := cfg.records().Identity(ctx)
	if err != nil {
		return false, true, fmt.Errorf("%s: %w", SpdName, err)
	}
	_, owned = cfg.records().Valid(bid, spdRecordKey(id))
	return owned, true, nil
}

func sortUint32(s []uint32) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// sortKVs sorts by key and drops repeated keys (a Retrieve never reports one key twice).
func sortKVs(kvs []scheduler.KV) []scheduler.KV {
	sort.SliceStable(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
	return dfkit.Dedupe(kvs)
}
