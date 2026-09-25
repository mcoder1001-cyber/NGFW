// Package qos holds the reconciler descriptors for VPP's QoS marking infrastructure (task
// DF-7, WBS D7.8): recording the QoS bits of received packets (qos_record_enable_disable),
// storing a fixed value (qos_store_enable_disable), egress maps that translate a recorded value
// into an output value per source (qos_egress_map_update / _delete) and marking on egress with
// a map (qos_mark_enable_disable). Every object type is retrieved from its dump
// (qos_record_dump, qos_store_dump, qos_egress_map_dump, qos_mark_dump).
//
// Messages come only from apps/agent/binapi/qos. Values are *structpb.Struct documents of the
// typed specs below (D-055). Ownership: record/store/mark belong to the owner of their
// interface; egress maps are attributed by id (df7.WithIDRange; production owns every id).
// docs/agent/descriptors/qos.md is the object ↔ message table.
package qos

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/qos"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameRecord    = "qos.record"
	NameStore     = "qos.store"
	NameEgressMap = "qos.egress-map"
	NameMark      = "qos.mark"
)

// QoS sources (qos.QosSource): where the bits are read from / written to.
const (
	SourceExt  = "ext"
	SourceVLAN = "vlan"
	SourceMPLS = "mpls"
	SourceIP   = "ip"
)

var sources = map[string]qos.QosSource{
	SourceExt:  qos.QOS_API_SOURCE_EXT,
	SourceVLAN: qos.QOS_API_SOURCE_VLAN,
	SourceMPLS: qos.QOS_API_SOURCE_MPLS,
	SourceIP:   qos.QOS_API_SOURCE_IP,
}

// sourceOrder is the row order of an egress map (qos_egress_map.rows[4], indexed by source).
var sourceOrder = []string{SourceExt, SourceVLAN, SourceMPLS, SourceIP}

func sourceName(s qos.QosSource) string {
	for n, v := range sources {
		if v == s {
			return n
		}
	}
	return fmt.Sprintf("#%d", s)
}

func checkSource(s string) error {
	if _, ok := sources[s]; !ok {
		return df7.Specf("qos source %q: want ext, vlan, mpls or ip", s)
	}
	return nil
}

// Meta of record/store/mark: the interface index.
type Meta struct{ SwIfIndex uint32 }

// ---- specs ------------------------------------------------------------------------------------

// Record is the desired state of one qos.record object: record the QoS bits of Source (vlan
// PCP, MPLS EXP, IP DSCP/TC) of packets received on Interface into the buffer metadata.
type Record struct {
	Interface string `json:"interface,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Validate checks r.
func (r Record) Validate() error {
	if r.Interface == "" {
		return df7.Specf("qos record needs an interface")
	}
	return checkSource(r.Source)
}

// Store is the desired state of one qos.store object: write Value into the QoS metadata of
// packets received on Interface. VPP 26.06 implements only the ip source.
type Store struct {
	Interface string `json:"interface,omitempty"`
	Source    string `json:"source,omitempty"`
	Value     uint8  `json:"value,omitempty"`
}

// Validate checks s.
func (s Store) Validate() error {
	if s.Interface == "" {
		return df7.Specf("qos store needs an interface")
	}
	if s.Source != SourceIP {
		return df7.Specf("qos store source %q: VPP 26.06 implements only ip", s.Source)
	}
	return nil
}

// EgressMap is the desired state of one qos.egress-map object: per input source a 256-entry
// row mapping a recorded value to the value written on egress. A row that is omitted (or
// empty) is all zeros; an explicit all-zero row is not canonical and rejected.
type EgressMap struct {
	ID   uint32 `json:"id,omitempty"`
	Ext  []int  `json:"ext,omitempty"`
	VLAN []int  `json:"vlan,omitempty"`
	MPLS []int  `json:"mpls,omitempty"`
	IP   []int  `json:"ip,omitempty"`
}

func (m EgressMap) rows() [][]int { return [][]int{m.Ext, m.VLAN, m.MPLS, m.IP} }

// Validate checks m.
func (m EgressMap) Validate() error {
	for i, row := range m.rows() {
		if len(row) == 0 {
			continue
		}
		if len(row) != 256 {
			return df7.Specf("egress map %d row %s has %d entries, want 256", m.ID, sourceOrder[i], len(row))
		}
		zero := true
		for j, v := range row {
			if v < 0 || v > 255 {
				return df7.Specf("egress map %d row %s[%d] = %d is not a byte", m.ID, sourceOrder[i], j, v)
			}
			if v != 0 {
				zero = false
			}
		}
		if zero {
			return df7.Specf("egress map %d row %s is all zeros; omit it (canonical form)", m.ID, sourceOrder[i])
		}
	}
	return nil
}

func (m EgressMap) toAPI() qos.QosEgressMap {
	out := qos.QosEgressMap{ID: m.ID}
	for i, row := range m.rows() {
		out.Rows[i].Outputs = make([]byte, 256)
		for j, v := range row {
			out.Rows[i].Outputs[j] = uint8(v) //nolint:gosec // Validate bounds 0..255
		}
	}
	return out
}

func fromAPI(m qos.QosEgressMap) EgressMap {
	row := func(r qos.QosEgressMapRow) []int {
		zero := true
		out := make([]int, 256)
		for j, v := range r.Outputs {
			out[j] = int(v)
			if v != 0 {
				zero = false
			}
		}
		if zero {
			return nil
		}
		return out
	}
	return EgressMap{ID: m.ID, Ext: row(m.Rows[0]), VLAN: row(m.Rows[1]), MPLS: row(m.Rows[2]), IP: row(m.Rows[3])}
}

// Mark is the desired state of one qos.mark object: on egress of Interface, write the Source
// bits (vlan PCP, MPLS EXP, IP DSCP) from egress map Map.
type Mark struct {
	Interface string `json:"interface,omitempty"`
	Source    string `json:"source,omitempty"`
	Map       uint32 `json:"map,omitempty"`
}

// Validate checks m.
func (m Mark) Validate() error {
	if m.Interface == "" {
		return df7.Specf("qos mark needs an interface")
	}
	return checkSource(m.Source)
}

// ---- keys -------------------------------------------------------------------------------------

// KeyRecord is "qos.record/<interface>/<source>".
func KeyRecord(ifName, source string) scheduler.Key {
	return scheduler.Join(NameRecord, ifName, source)
}

// KeyStore is "qos.store/<interface>/<source>".
func KeyStore(ifName, source string) scheduler.Key { return scheduler.Join(NameStore, ifName, source) }

// KeyEgressMap is "qos.egress-map/<id>".
func KeyEgressMap(id uint32) scheduler.Key {
	return scheduler.Join(NameEgressMap, strconv.FormatUint(uint64(id), 10))
}

// KeyMark is "qos.mark/<interface>/<source>".
func KeyMark(ifName, source string) scheduler.Key { return scheduler.Join(NameMark, ifName, source) }

// ---- helpers ----------------------------------------------------------------------------------

// disableOnce sends one disable of a reference-counted record/store enable (review L1: one
// object = one reference; further references belong to other consumers of the same counter and
// are left alone). VALUE_EXIST / NO_MATCHING_INTERFACE mean nothing is enabled: success.
func disableOnce(ctx context.Context, disable func(context.Context) error) error {
	if err := disable(ctx); err != nil && !df7.IsVPPError(err, api.VALUE_EXIST, api.NO_MATCHING_INTERFACE) {
		return err
	}
	return nil
}

// detach re-resolves the interface of an object about to be deleted, runs del on it when it
// still exists, and releases the claim.
func detach(ctx context.Context, b df7.Base, ifName, holder string, del func(ctx context.Context, idx uint32) error) error {
	tg, found, err := b.Detach(ctx, ifName, holder)
	if err != nil || !found {
		return err
	}
	if err := del(ctx, tg.Index); err != nil {
		return err
	}
	return tg.Release()
}

func sortKVs(kvs []scheduler.KV) {
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
}

// ---- qos.record -------------------------------------------------------------------------------

// RecordDescriptor manages qos.record objects.
type RecordDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*RecordDescriptor)(nil)

// NewRecord returns the qos.record descriptor.
func NewRecord(c vpp.Client, owner string, opts ...df7.Option) *RecordDescriptor {
	return &RecordDescriptor{df7.NewBase(NameRecord, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *RecordDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	r, _ := df7.Decode[Record](obj)
	return KeyRecord(r.Interface, r.Source)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *RecordDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	r, _ := df7.Decode[Record](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(r.Interface)}
}

func (d *RecordDescriptor) set(ctx context.Context, idx uint32, source string, enable bool) error {
	_, err := qos.NewServiceClient(d.Client).QosRecordEnableDisable(ctx, &qos.QosRecordEnableDisable{
		Enable: enable, Record: qos.QosRecord{SwIfIndex: interface_types.InterfaceIndex(idx), InputSource: sources[source]}})
	return d.Wrap(fmt.Sprintf("qos_record_enable_disable %d %s enable=%v", idx, source, enable), err)
}

// Create implements scheduler.Descriptor.
func (d *RecordDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	r, err := df7.DecodeValid[Record](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, r.Interface, string(KeyRecord(r.Interface, r.Source)))
	if err != nil {
		return nil, err
	}
	idx := tg.Index
	// the enable is reference-counted in VPP: an existing record is never enabled a second time
	// (D-076) — it is ours only on our tagged interface or with our live claim (review M1)
	on, err := d.enabled(ctx, idx, r.Source)
	if err != nil {
		return nil, err
	}
	if on {
		return Meta{SwIfIndex: idx}, tg.Adopt()
	}
	c, err := tg.ClaimFirst(ctx) // TD-11b: the claim before the VPP write, released when VPP refuses
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, r.Source, true); err != nil {
		return nil, c.Undo(err)
	}
	return Meta{SwIfIndex: idx}, nil
}

func (d *RecordDescriptor) enabled(ctx context.Context, idx uint32, source string) (bool, error) {
	stream, err := qos.NewServiceClient(d.Client).QosRecordDump(ctx, &qos.QosRecordDump{})
	if err != nil {
		return false, d.Wrap("qos_record_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return false, d.Wrap("qos_record_dump", err)
	}
	for _, det := range dets {
		if uint32(det.Record.SwIfIndex) == idx && det.Record.InputSource == sources[source] {
			return true, nil
		}
	}
	return false, nil
}

// Update implements scheduler.Descriptor: every field is part of the key.
func (*RecordDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and send one disable.
func (d *RecordDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	r, err := df7.Decode[Record](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, r.Interface, string(KeyRecord(r.Interface, r.Source)), func(ctx context.Context, idx uint32) error {
		return disableOnce(ctx, func(ctx context.Context) error { return d.set(ctx, idx, r.Source, false) })
	})
}

// Retrieve implements scheduler.Descriptor: qos_record_dump, owned interfaces only.
func (d *RecordDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qos.NewServiceClient(d.Client).QosRecordDump(ctx, &qos.QosRecordDump{})
	if err != nil {
		return nil, d.Wrap("qos_record_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("qos_record_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		src := sourceName(det.Record.InputSource)
		name, ok := ifs.Owned(uint32(det.Record.SwIfIndex), func(n string) string { return string(KeyRecord(n, src)) })
		if !ok {
			continue
		}
		r := Record{Interface: name, Source: src}
		out = append(out, df7.KV(KeyRecord(r.Interface, r.Source), r, Meta{SwIfIndex: uint32(det.Record.SwIfIndex)}))
	}
	sortKVs(out)
	return out, nil
}

// ---- qos.store --------------------------------------------------------------------------------

// StoreDescriptor manages qos.store objects.
type StoreDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*StoreDescriptor)(nil)

// NewStore returns the qos.store descriptor.
func NewStore(c vpp.Client, owner string, opts ...df7.Option) *StoreDescriptor {
	return &StoreDescriptor{df7.NewBase(NameStore, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *StoreDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := df7.Decode[Store](obj)
	return KeyStore(s.Interface, s.Source)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *StoreDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, _ := df7.Decode[Store](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(s.Interface)}
}

func (d *StoreDescriptor) set(ctx context.Context, idx uint32, s Store, enable bool) error {
	_, err := qos.NewServiceClient(d.Client).QosStoreEnableDisable(ctx, &qos.QosStoreEnableDisable{
		Enable: enable, Store: qos.QosStore{SwIfIndex: interface_types.InterfaceIndex(idx), InputSource: sources[s.Source], Value: s.Value}})
	return d.Wrap(fmt.Sprintf("qos_store_enable_disable %d %s enable=%v", idx, s.Source, enable), err)
}

// Create implements scheduler.Descriptor.
func (d *StoreDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := df7.DecodeValid[Store](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, s.Interface, string(KeyStore(s.Interface, s.Source)))
	if err != nil {
		return nil, err
	}
	idx := tg.Index
	on, err := d.enabled(ctx, idx, s.Source)
	if err != nil {
		return nil, err
	}
	if on { // reference-counted: never a second enable (D-076); adopt only our own (M1)
		return Meta{SwIfIndex: idx}, tg.Adopt()
	}
	c, err := tg.ClaimFirst(ctx) // TD-11b: the claim before the VPP write, released when VPP refuses
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, s, true); err != nil {
		return nil, c.Undo(err)
	}
	return Meta{SwIfIndex: idx}, nil
}

func (d *StoreDescriptor) enabled(ctx context.Context, idx uint32, source string) (bool, error) {
	stream, err := qos.NewServiceClient(d.Client).QosStoreDump(ctx, &qos.QosStoreDump{})
	if err != nil {
		return false, d.Wrap("qos_store_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return false, d.Wrap("qos_store_dump", err)
	}
	for _, det := range dets {
		if uint32(det.Store.SwIfIndex) == idx && det.Store.InputSource == sources[source] {
			return true, nil
		}
	}
	return false, nil
}

// Update implements scheduler.Descriptor: VPP keeps the first value while the store is
// enabled, so a new value is ErrRecreate.
func (*StoreDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and send one disable.
func (d *StoreDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := df7.Decode[Store](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, s.Interface, string(KeyStore(s.Interface, s.Source)), func(ctx context.Context, idx uint32) error {
		return disableOnce(ctx, func(ctx context.Context) error { return d.set(ctx, idx, s, false) })
	})
}

// Retrieve implements scheduler.Descriptor: qos_store_dump, owned interfaces only.
func (d *StoreDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qos.NewServiceClient(d.Client).QosStoreDump(ctx, &qos.QosStoreDump{})
	if err != nil {
		return nil, d.Wrap("qos_store_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("qos_store_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		src := sourceName(det.Store.InputSource)
		name, ok := ifs.Owned(uint32(det.Store.SwIfIndex), func(n string) string { return string(KeyStore(n, src)) })
		if !ok {
			continue
		}
		s := Store{Interface: name, Source: src, Value: det.Store.Value}
		out = append(out, df7.KV(KeyStore(s.Interface, s.Source), s, Meta{SwIfIndex: uint32(det.Store.SwIfIndex)}))
	}
	sortKVs(out)
	return out, nil
}

// ---- qos.egress-map ---------------------------------------------------------------------------

// EgressMapDescriptor manages qos.egress-map objects.
type EgressMapDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*EgressMapDescriptor)(nil)

// NewEgressMap returns the qos.egress-map descriptor.
func NewEgressMap(c vpp.Client, owner string, opts ...df7.Option) *EgressMapDescriptor {
	return &EgressMapDescriptor{df7.NewBase(NameEgressMap, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *EgressMapDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	m, _ := df7.Decode[EgressMap](obj)
	return KeyEgressMap(m.ID)
}

// Dependencies implements scheduler.Descriptor: none.
func (*EgressMapDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *EgressMapDescriptor) update(ctx context.Context, obj proto.Message) error {
	m, err := df7.DecodeValid[EgressMap](obj)
	if err != nil {
		return err
	}
	if err := d.Opts.CheckID("egress map", m.ID); err != nil {
		return err
	}
	_, err = qos.NewServiceClient(d.Client).QosEgressMapUpdate(ctx, &qos.QosEgressMapUpdate{Map: m.toAPI()})
	return d.Wrap(fmt.Sprintf("qos_egress_map_update %d", m.ID), err)
}

// Create implements scheduler.Descriptor: qos_egress_map_update (creates the id).
func (d *EgressMapDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.update(ctx, obj)
}

// Update implements scheduler.Descriptor: qos_egress_map_update rewrites all rows in place,
// so marks referencing the map keep working.
func (d *EgressMapDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.update(ctx, newObj)
}

// Delete implements scheduler.Descriptor: qos_egress_map_delete (marks depend on the map and
// are removed first).
func (d *EgressMapDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	m, err := df7.Decode[EgressMap](obj)
	if err != nil {
		return err
	}
	_, err = qos.NewServiceClient(d.Client).QosEgressMapDelete(ctx, &qos.QosEgressMapDelete{ID: m.ID})
	return d.Wrap(fmt.Sprintf("qos_egress_map_delete %d", m.ID), err)
}

// Retrieve implements scheduler.Descriptor: qos_egress_map_dump, ids in the owned range.
func (d *EgressMapDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := qos.NewServiceClient(d.Client).QosEgressMapDump(ctx, &qos.QosEgressMapDump{})
	if err != nil {
		return nil, d.Wrap("qos_egress_map_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("qos_egress_map_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		if !d.Opts.IDs.Owns(det.Map.ID) {
			continue
		}
		m := fromAPI(det.Map)
		out = append(out, df7.KV(KeyEgressMap(m.ID), m, nil))
	}
	sortKVs(out)
	return out, nil
}

// ---- qos.mark ---------------------------------------------------------------------------------

// MarkDescriptor manages qos.mark objects.
type MarkDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*MarkDescriptor)(nil)

// NewMark returns the qos.mark descriptor.
func NewMark(c vpp.Client, owner string, opts ...df7.Option) *MarkDescriptor {
	return &MarkDescriptor{df7.NewBase(NameMark, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *MarkDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	m, _ := df7.Decode[Mark](obj)
	return KeyMark(m.Interface, m.Source)
}

// Dependencies implements scheduler.Descriptor: the interface and the egress map.
func (d *MarkDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	m, _ := df7.Decode[Mark](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(m.Interface), {Key: KeyEgressMap(m.Map)}}
}

func (d *MarkDescriptor) set(ctx context.Context, idx uint32, m Mark, enable bool) error {
	_, err := qos.NewServiceClient(d.Client).QosMarkEnableDisable(ctx, &qos.QosMarkEnableDisable{
		Enable: enable, Mark: qos.QosMark{SwIfIndex: idx, MapID: m.Map, OutputSource: sources[m.Source]}})
	return d.Wrap(fmt.Sprintf("qos_mark_enable_disable %d %s map %d enable=%v", idx, m.Source, m.Map, enable), err)
}

// Create implements scheduler.Descriptor.
func (d *MarkDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	m, err := df7.DecodeValid[Mark](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, m.Interface, string(KeyMark(m.Interface, m.Source)))
	if err != nil {
		return nil, err
	}
	idx := tg.Index
	c, err := tg.ClaimFirst(ctx) // TD-11b: the claim before the VPP write, released when VPP refuses
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, m, true); err != nil {
		return nil, c.Undo(err)
	}
	return Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: another map is applied in place (VPP replaces the
// interface's map for the source; the interface is re-resolved, D-071).
func (d *MarkDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	oldM, err := df7.Decode[Mark](oldObj)
	if err != nil {
		return nil, err
	}
	newM, err := df7.DecodeValid[Mark](newObj)
	if err != nil {
		return nil, err
	}
	if oldM.Interface != newM.Interface || oldM.Source != newM.Source {
		return nil, scheduler.ErrRecreate
	}
	tg, found, err := d.Detach(ctx, newM.Interface, string(KeyMark(newM.Interface, newM.Source)))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %q", NameMark, df7.ErrNoSuchInterface, newM.Interface)
	}
	return Meta{SwIfIndex: tg.Index}, d.set(ctx, tg.Index, newM, true)
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and disable.
func (d *MarkDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	mk, err := df7.Decode[Mark](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, mk.Interface, string(KeyMark(mk.Interface, mk.Source)), func(ctx context.Context, idx uint32) error {
		// VALUE_EXIST: not marking (nothing left to remove)
		if err := d.set(ctx, idx, mk, false); err != nil && !df7.IsVPPError(err, api.VALUE_EXIST, api.NO_MATCHING_INTERFACE) {
			return err
		}
		return nil
	})
}

// Retrieve implements scheduler.Descriptor: qos_mark_dump, owned interfaces only.
func (d *MarkDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qos.NewServiceClient(d.Client).QosMarkDump(ctx, &qos.QosMarkDump{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex)})
	if err != nil {
		return nil, d.Wrap("qos_mark_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("qos_mark_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		src := sourceName(det.Mark.OutputSource)
		name, ok := ifs.Owned(det.Mark.SwIfIndex, func(n string) string { return string(KeyMark(n, src)) })
		if !ok {
			continue
		}
		m := Mark{Interface: name, Source: src, Map: det.Mark.MapID}
		out = append(out, df7.KV(KeyMark(m.Interface, m.Source), m, Meta{SwIfIndex: det.Mark.SwIfIndex}))
	}
	sortKVs(out)
	return out, nil
}

// Register constructs every descriptor of the qos plugin (maps before marks).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewEgressMap(c, owner, opts...))
	r.Register(NewRecord(c, owner, opts...))
	r.Register(NewStore(c, owner, opts...))
	r.Register(NewMark(c, owner, opts...))
}
