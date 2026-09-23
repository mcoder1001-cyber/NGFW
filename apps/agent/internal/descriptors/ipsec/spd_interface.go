package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// SpdInterface binds an SPD to an interface (ipsec_interface_add_del_spd; dump
// ipsec_spd_interface_dump).
//
// VPP limitation: ipsec_spd_interface_details carries the SPD's *pool index*, not its spd_id, and
// no dump maps one to the other. The descriptor learns index→id when it binds (it dumps the
// bindings right after ipsec_interface_add_del_spd) and keeps the map for the life of the
// process. A binding whose index it has never seen (after an agent restart) is retrieved with
// spd_id 0; the scheduler then plans an Update, which returns ErrRecreate, and the re-bind
// teaches the map. See docs/agent/descriptors/ipsec.md.
type SpdInterface struct {
	cfg        Config
	mu         sync.Mutex
	idByIndex  map[uint32]uint32 // SPD pool index → spd_id
	knownIndex map[uint32]uint32 // spd_id → pool index
}

// SpdInterfaceMeta is the runtime handle of a binding.
type SpdInterfaceMeta struct {
	SwIfIndex uint32
	SpdID     uint32
	SpdIndex  uint32 // VPP pool index of the SPD as reported by ipsec_spd_interface_details
}

// NewSpdInterface returns the descriptor.
func NewSpdInterface(cfg Config) *SpdInterface {
	return &SpdInterface{cfg: cfg, idByIndex: map[uint32]uint32{}, knownIndex: map[uint32]uint32{}}
}

// Name implements scheduler.Descriptor.
func (*SpdInterface) Name() string { return SpdInterfaceName }

// KeyOf implements scheduler.Descriptor: ipsec.spd-interface/<interface>.
func (*SpdInterface) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecSpdInterface)
	return scheduler.Join(SpdInterfaceName, o.GetInterface())
}

// Dependencies implements scheduler.Descriptor: the SPD and the interface.
func (*SpdInterface) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.IpsecSpdInterface)
	return []scheduler.Dependency{
		{Key: scheduler.Join(SpdName, vpn.Uint(o.GetSpdId()))},
		{Key: vpn.InterfaceDependency(o.GetInterface())},
	}
}

// Create implements scheduler.Descriptor.
func (d *SpdInterface) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSpdInterface)
	if !ok {
		return nil, typeErr(SpdInterfaceName, obj)
	}
	if err := d.cfg.IDs.Check("spd", o.GetSpdId()); err != nil {
		return nil, err
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.Index(o.GetInterface())
	if err != nil {
		return nil, err
	}
	svc := ipsec.NewServiceClient(d.cfg.Client)
	if _, err := svc.IpsecInterfaceAddDelSpd(ctx, &ipsec.IpsecInterfaceAddDelSpd{IsAdd: true, SwIfIndex: idx, SpdID: o.GetSpdId()}); err != nil {
		return nil, fmt.Errorf("ipsec_interface_add_del_spd: %w", err)
	}
	meta := SpdInterfaceMeta{SwIfIndex: uint32(idx), SpdID: o.GetSpdId(), SpdIndex: noInterface}
	bindings, err := d.dumpBindings(ctx)
	if err != nil {
		return meta, err
	}
	if spdIndex, ok := bindings[uint32(idx)]; ok {
		meta.SpdIndex = spdIndex
		d.learn(spdIndex, o.GetSpdId())
	}
	return meta, nil
}

// Update implements scheduler.Descriptor: VPP allows one SPD per interface and no re-bind in
// place, so a different spd_id (or an unknown actual one) is a recreate.
func (*SpdInterface) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *SpdInterface) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.IpsecSpdInterface)
	if !ok {
		return typeErr(SpdInterfaceName, obj)
	}
	m, ok := meta.(SpdInterfaceMeta)
	if !ok {
		return metaErr(SpdInterfaceName, meta)
	}
	spdID := o.GetSpdId()
	if spdID == 0 {
		spdID = m.SpdID
	}
	if spdID == 0 {
		// Retrieved with an unknown id (see the type doc): ipsec_set_interface_spd(is_add=0) only
		// needs *an existing* spd_id to look up, it unbinds whatever the interface has.
		ids, err := ownedSpdIDs(ctx, d.cfg)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("%s: cannot unbind %s: no SPD id known", SpdInterfaceName, o.GetInterface())
		}
		spdID = ids[0]
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecInterfaceAddDelSpd(ctx, &ipsec.IpsecInterfaceAddDelSpd{
		IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), SpdID: spdID,
	}); err != nil {
		return fmt.Errorf("ipsec_interface_add_del_spd: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: every binding on an interface tagged by this owner.
func (d *SpdInterface) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
	if err != nil {
		return nil, err
	}
	bindings, err := d.dumpBindings(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for swIfIndex, spdIndex := range bindings {
		if _, owned := tbl.Owned(swIfIndex, d.cfg.Owner); !owned {
			continue
		}
		spdID := d.idOf(spdIndex)
		if spdID != 0 && !d.cfg.IDs.Contains(spdID) {
			continue
		}
		v := &vpnpb.IpsecSpdInterface{Interface: tbl.Name(swIfIndex), SpdId: spdID}
		out = append(out, scheduler.KV{
			Key:   d.KeyOf(v),
			Value: v,
			Meta:  SpdInterfaceMeta{SwIfIndex: swIfIndex, SpdID: spdID, SpdIndex: spdIndex},
		})
	}
	sortKVs(out)
	return out, nil
}

// dumpBindings returns sw_if_index → SPD pool index.
func (d *SpdInterface) dumpBindings(ctx context.Context) (map[uint32]uint32, error) {
	stream, err := ipsec.NewServiceClient(d.cfg.Client).IpsecSpdInterfaceDump(ctx, &ipsec.IpsecSpdInterfaceDump{})
	if err != nil {
		return nil, fmt.Errorf("ipsec_spd_interface_dump: %w", err)
	}
	out := map[uint32]uint32{}
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ipsec_spd_interface_dump: %w", err)
		}
		out[uint32(det.SwIfIndex)] = det.SpdIndex
	}
}

func (d *SpdInterface) learn(spdIndex, spdID uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if old, ok := d.knownIndex[spdID]; ok && old != spdIndex {
		delete(d.idByIndex, old)
	}
	d.idByIndex[spdIndex] = spdID
	d.knownIndex[spdID] = spdIndex
}

func (d *SpdInterface) idOf(spdIndex uint32) uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.idByIndex[spdIndex]
}

func sortKVs(kvs []scheduler.KV) {
	for i := 1; i < len(kvs); i++ {
		for j := i; j > 0 && kvs[j-1].Key > kvs[j].Key; j-- {
			kvs[j-1], kvs[j] = kvs[j], kvs[j-1]
		}
	}
}
