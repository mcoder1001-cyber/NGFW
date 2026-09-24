package ipsec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// SpdInterface binds an SPD to an interface (ipsec_interface_add_del_spd; dump
// ipsec_spd_interface_dump). The interface is named by its logical name (D-069): one of our
// interfaces by its tag id, or an untagged (physical) interface by VPP's name — the usual case for
// policy-based IPsec on a WAN NIC. Another owner's interface is refused.
//
// Ownership and the SPD id: a binding carries no tag, and ipsec_spd_interface_details reports
// the SPD's *pool index*, not its spd_id (no dump maps one to the other). After our add the
// descriptor writes an ownership record (vpn.Records, bound to the VPP boot identity) keyed by the
// logical interface name with value "<sw_if_index>/<spd_id>/<spd pool index>". Retrieve reports a
// binding only when such a record matches it — on our tagged interfaces too, since charon's
// kernel-vpp plugin or another owner may bind an SPD to any interface — and takes the spd_id from
// the record. A binding without our record is never adopted (D-071): Create then fails with VPP's
// "SPD already assigned". The agent must install a persisted store (WithBootStore) so its bindings
// survive an agent restart.
type SpdInterface struct{ cfg Config }

// SpdInterfaceMeta is the runtime handle of a binding.
type SpdInterfaceMeta struct {
	SwIfIndex uint32
	SpdID     uint32
	SpdIndex  uint32 // VPP pool index of the SPD as reported by ipsec_spd_interface_details
}

// NewSpdInterface returns the descriptor.
func NewSpdInterface(cfg Config) *SpdInterface { return &SpdInterface{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*SpdInterface) Name() string { return SpdInterfaceName }

// KeyOf implements scheduler.Descriptor: ipsec.spd-interface/<logical interface name>.
func (*SpdInterface) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.IpsecSpdInterface)
	return scheduler.Join(SpdInterfaceName, o.GetInterface())
}

// Dependencies implements scheduler.Descriptor: the SPD and the interface (alias interface/<name>).
func (*SpdInterface) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.IpsecSpdInterface)
	return []scheduler.Dependency{
		{Key: scheduler.Join(SpdName, vpn.Uint(o.GetSpdId()))},
		{Key: vpn.InterfaceKey(o.GetInterface())},
	}
}

type bindingRecord struct{ swIfIndex, spdID, spdIndex uint32 }

func (r bindingRecord) String() string {
	return fmt.Sprintf("%d/%d/%d", r.swIfIndex, r.spdID, r.spdIndex)
}

func parseBindingRecord(s string) (bindingRecord, bool) {
	f := strings.Split(s, "/")
	if len(f) != 3 {
		return bindingRecord{}, false
	}
	var v [3]uint32
	for i, p := range f {
		n, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return bindingRecord{}, false
		}
		v[i] = uint32(n)
	}
	return bindingRecord{v[0], v[1], v[2]}, true
}

// anyIndex marks the pool index of a pending binding record (not known before the bind).
const anyIndex = ^uint32(0)

func bindingRecordKey(ifName string) string { return string(scheduler.Join(SpdInterfaceName, ifName)) }

// Create implements scheduler.Descriptor. VPP refuses a second SPD on an interface
// (SYSCALL_ERROR_2), so an existing binding is never adopted.
func (d *SpdInterface) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.IpsecSpdInterface)
	if !ok {
		return nil, typeErr(SpdInterfaceName, obj)
	}
	if err := d.cfg.IDs.Check("spd", o.GetSpdId()); err != nil {
		return nil, err
	}
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	idx, err := tbl.Resolve(o.GetInterface())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SpdInterfaceName, err)
	}
	rec := d.cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SpdInterfaceName, err)
	}
	svc := ipsec.NewServiceClient(d.cfg.Client)
	key := bindingRecordKey(o.GetInterface())
	bindings, err := d.dumpBindings(ctx)
	if err != nil {
		return nil, err
	}
	if spdIndex, bound := bindings[uint32(idx)]; bound {
		// bound already: ours only with our record for this index and SPD (a retry after a lost
		// reply or a failure after the bind); VPP would refuse a second SPD anyway
		if r, ok := d.record(bid, o.GetInterface(), uint32(idx), spdIndex); ok && r.spdID == o.GetSpdId() {
			r.spdIndex = spdIndex
			return SpdInterfaceMeta{SwIfIndex: uint32(idx), SpdID: r.spdID, SpdIndex: spdIndex}, rec.Put(bid, key, r.String())
		}
		return nil, fmt.Errorf("%s: %s already has an SPD: %w", SpdInterfaceName, o.GetInterface(), vpn.ErrNotOurs)
	}
	// write-ahead (review M4): pool index still unknown (anyIndex matches any)
	pending := bindingRecord{swIfIndex: uint32(idx), spdID: o.GetSpdId(), spdIndex: anyIndex}
	if err := rec.PutPending(bid, key, pending.String()); err != nil {
		return nil, fmt.Errorf("%s: record: %w", SpdInterfaceName, err)
	}
	if _, err := svc.IpsecInterfaceAddDelSpd(ctx, &ipsec.IpsecInterfaceAddDelSpd{IsAdd: true, SwIfIndex: idx, SpdID: o.GetSpdId()}); err != nil {
		if after, derr := d.dumpBindings(ctx); derr == nil {
			if _, bound := after[uint32(idx)]; !bound {
				_ = rec.Drop(key)
			}
		}
		return nil, fmt.Errorf("ipsec_interface_add_del_spd (%s): %w", o.GetInterface(), err)
	}
	meta := SpdInterfaceMeta{SwIfIndex: uint32(idx), SpdID: o.GetSpdId(), SpdIndex: noInterface}
	bindings, err = d.dumpBindings(ctx)
	if err != nil {
		return meta, err // the pending record keeps the binding ours
	}
	if spdIndex, ok := bindings[uint32(idx)]; ok {
		meta.SpdIndex = spdIndex
	}
	r := bindingRecord{swIfIndex: uint32(idx), spdID: o.GetSpdId(), spdIndex: meta.SpdIndex}
	if err := rec.Put(bid, bindingRecordKey(o.GetInterface()), r.String()); err != nil {
		return meta, fmt.Errorf("%s: record: %w", SpdInterfaceName, err)
	}
	return meta, nil
}

// Update implements scheduler.Descriptor: VPP allows one SPD per interface and no re-bind in
// place, so a different spd_id (or an unknown actual one) is a recreate.
func (*SpdInterface) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before unbinding it re-resolves the logical name
// and re-reads the binding (D-071/D-074): an interface that went away or now has another index,
// or a binding that is gone, needs nothing; a binding without our matching record is refused.
func (d *SpdInterface) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.IpsecSpdInterface)
	if !ok {
		return typeErr(SpdInterfaceName, obj)
	}
	m, ok := meta.(SpdInterfaceMeta)
	if !ok {
		return metaErr(SpdInterfaceName, meta)
	}
	rec := d.cfg.records()
	key := bindingRecordKey(o.GetInterface())
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return err
	}
	idx, err := tbl.Resolve(o.GetInterface())
	if errors.Is(err, vpn.ErrNoInterface) || (err == nil && uint32(idx) != m.SwIfIndex) {
		return rec.Drop(key) // the interface (and with it the binding) is gone
	}
	if err != nil {
		return fmt.Errorf("%s: %w", SpdInterfaceName, err)
	}
	bindings, err := d.dumpBindings(ctx)
	if err != nil {
		return err
	}
	spdIndex, bound := bindings[uint32(idx)]
	if !bound {
		return rec.Drop(key)
	}
	bid, err := rec.Identity(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", SpdInterfaceName, err)
	}
	r, hasRec := d.record(bid, o.GetInterface(), uint32(idx), spdIndex)
	if !hasRec {
		return fmt.Errorf("%s: %s: %w", SpdInterfaceName, o.GetInterface(), vpn.ErrNotOurs)
	}
	if _, err := ipsec.NewServiceClient(d.cfg.Client).IpsecInterfaceAddDelSpd(ctx, &ipsec.IpsecInterfaceAddDelSpd{
		IsAdd: false, SwIfIndex: idx, SpdID: r.spdID,
	}); err != nil {
		return fmt.Errorf("ipsec_interface_add_del_spd (%s, del): %w", o.GetInterface(), err)
	}
	return rec.Drop(key)
}

// record returns our binding record of ifName when it matches the running VPP instance, the
// interface index and the SPD pool index.
func (d *SpdInterface) record(bid bootid.Identity, ifName string, swIfIndex, spdIndex uint32) (bindingRecord, bool) {
	v, ok := d.cfg.records().Valid(bid, bindingRecordKey(ifName))
	if !ok {
		return bindingRecord{}, false
	}
	r, ok := parseBindingRecord(v)
	if !ok || r.swIfIndex != swIfIndex || (r.spdIndex != spdIndex && r.spdIndex != anyIndex) {
		return bindingRecord{}, false
	}
	return r, true
}

// Retrieve implements scheduler.Descriptor: bindings with our matching record (see the type doc).
func (d *SpdInterface) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	bindings, err := d.dumpBindings(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	var bid bootid.Identity
	for swIfIndex, spdIndex := range bindings {
		name := tbl.Logical(swIfIndex)
		if name == "" {
			continue // another owner's interface, local0, unknown
		}
		if bid.IsZero() {
			if bid, err = d.cfg.records().Identity(ctx); err != nil {
				return nil, fmt.Errorf("%s: %w", SpdInterfaceName, err)
			}
		}
		r, hasRec := d.record(bid, name, swIfIndex, spdIndex)
		if !hasRec || !d.cfg.IDs.Contains(r.spdID) {
			continue // not our binding
		}
		v := &vpnpb.IpsecSpdInterface{Interface: name, SpdId: r.spdID}
		out = append(out, scheduler.KV{
			Key:   d.KeyOf(v),
			Value: v,
			Meta:  SpdInterfaceMeta{SwIfIndex: swIfIndex, SpdID: r.spdID, SpdIndex: spdIndex},
		})
	}
	return sortKVs(out), nil
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
