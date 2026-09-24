package classify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/bits"

	"go.fd.io/govpp/api"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// TableName is the descriptor name; keys are "classify.table/<name>".
const TableName = "classify.table"

// VPP defaults of classify_add_del_table.
const (
	DefaultNbuckets      = 2
	DefaultMemorySize    = 2097152
	DefaultMatchNVectors = 1
	// NoIndex is VPP's "none" table / node index (~0).
	NoIndex = ^uint32(0)
	// VectorSize is the size of one classifier vector (u32x4).
	VectorSize = 16
)

// NormalizeTable returns t in the form Retrieve produces: defaults filled, nbuckets rounded
// up to a power of two (as VPP does), mask padded/truncated to match_n_vectors × 16 bytes.
func NormalizeTable(t *Table) *Table {
	n := proto.Clone(t).(*Table)
	if n.Nbuckets == 0 {
		n.Nbuckets = DefaultNbuckets
	}
	if bits.OnesCount32(n.Nbuckets) != 1 {
		n.Nbuckets = 1 << bits.Len32(n.Nbuckets)
	}
	if n.MemorySize == 0 {
		n.MemorySize = DefaultMemorySize
	}
	if n.MatchNVectors == 0 {
		n.MatchNVectors = DefaultMatchNVectors
	}
	n.Mask = fit(n.Mask, int(n.MatchNVectors)*VectorSize)
	return n
}

// fit pads b with zeros or truncates it to n bytes.
func fit(b []byte, n int) []byte {
	out := make([]byte, n)
	copy(out, b)
	return out
}

// TableDescriptor manages classifier tables (classify_add_del_table).
type TableDescriptor struct {
	client vpp.Client
	store  Store
}

// NewTable returns the descriptor backed by store.
func NewTable(c vpp.Client, store Store) *TableDescriptor {
	return &TableDescriptor{client: c, store: store}
}

// TableMeta is the runtime handle: the VPP table index.
type TableMeta struct{ Index uint32 }

// Name implements scheduler.Descriptor.
func (*TableDescriptor) Name() string { return TableName }

// KeyOf implements scheduler.Descriptor.
func (*TableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(TableName, obj.(*Table).GetName())
}

// TableKey is the key of the classify table called name (for dependents in other packages).
func TableKey(name string) scheduler.Key { return scheduler.Join(TableName, name) }

// Dependencies implements scheduler.Descriptor: the next table in the chain, if any.
func (*TableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	if next := obj.(*Table).GetNextTable(); next != "" {
		return []scheduler.Dependency{{Key: TableKey(next)}}
	}
	return nil
}

func (d *TableDescriptor) indexOf(name string) (TableRecord, error) {
	rec, ok := d.store.Get(name)
	if !ok {
		return TableRecord{}, fmt.Errorf("%w: %q", ErrNoSuchTable, name)
	}
	return rec, nil
}

// Create implements scheduler.Descriptor. It holds the Store's transaction lock from the
// stale-record prune to the Put, so a concurrent Retrieve's prune cannot drop the new record.
func (d *TableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	t := NormalizeTable(obj.(*Table))
	if t.GetName() == "" {
		return nil, fmt.Errorf("%s: name is required", TableName)
	}
	if t.GetCurrentDataOffset() < -32768 || t.GetCurrentDataOffset() > 32767 {
		return nil, fmt.Errorf("%s: current_data_offset %d out of int16 range", TableName, t.GetCurrentDataOffset())
	}
	d.store.Lock()
	defer d.store.Unlock()
	if err := pruneLocked(ctx, d.client, d.store); err != nil {
		return nil, err
	}
	if _, exists := d.store.Get(t.GetName()); exists {
		return nil, fmt.Errorf("%s: table %q already exists in the store", TableName, t.GetName())
	}
	next := NoIndex
	if t.GetNextTable() != "" {
		rec, err := d.indexOf(t.GetNextTable())
		if err != nil {
			return nil, err
		}
		next = rec.Index
	}
	req := &classifyapi.ClassifyAddDelTable{
		IsAdd:             true,
		TableIndex:        NoIndex,
		Nbuckets:          t.GetNbuckets(),
		MemorySize:        t.GetMemorySize(),
		SkipNVectors:      t.GetSkipNVectors(),
		MatchNVectors:     t.GetMatchNVectors(),
		NextTableIndex:    next,
		MissNextIndex:     t.GetMissNextIndex(),
		CurrentDataFlag:   b2u(t.GetCurrentDataFlag()),
		CurrentDataOffset: int16(t.GetCurrentDataOffset()), //nolint:gosec // checked above
		MaskLen:           uint32(len(t.GetMask())),        //nolint:gosec // match_n_vectors*16
		Mask:              t.GetMask(),
	}
	rep, err := classifyapi.NewServiceClient(d.client).ClassifyAddDelTable(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("classify_add_del_table: %w", err)
	}
	rec := TableRecord{
		Name: t.GetName(), Index: rep.NewTableIndex, SkipNVectors: rep.SkipNVectors, MatchNVectors: rep.MatchNVectors,
		Mask: t.GetMask(), MemorySize: t.GetMemorySize(), CurrentDataFlag: t.GetCurrentDataFlag(), CurrentDataOffset: t.GetCurrentDataOffset(),
	}
	if err := d.store.Put(rec); err != nil {
		// Never leave a table in VPP that no record attributes to us.
		if derr := deleteTable(ctx, d.client, rep.NewTableIndex); derr != nil {
			return nil, fmt.Errorf("%s: store: %w (and removing table %d failed: %v)", TableName, err, rep.NewTableIndex, derr)
		}
		return nil, fmt.Errorf("%s: store: %w", TableName, err)
	}
	return TableMeta{Index: rep.NewTableIndex}, nil
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// Update implements scheduler.Descriptor: VPP cannot change a table in place.
func (*TableDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *TableDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	name := obj.(*Table).GetName()
	m, hasMeta := meta.(TableMeta)
	if meta != nil && !hasMeta {
		return fmt.Errorf("%s: %w %T", TableName, df2.ErrBadMeta, meta)
	}
	d.store.Lock()
	defer d.store.Unlock()
	// D-071: re-verify identity in the same sequence, right before deleting by index. The
	// index is deleted only when the record is live now (same VPP instance, index listed,
	// geometry equal — see snapshot) and it is the index the caller holds. Otherwise our
	// table is already gone and the index may belong to someone else: drop the record only.
	live, _, _, _, err := snapshot(ctx, d.client, d.store)
	if err != nil {
		return err
	}
	var rec *TableRecord
	for i := range live {
		if live[i].rec.Name == name {
			rec = &live[i].rec
		}
	}
	if rec == nil || (hasMeta && m.Index != rec.Index) {
		return d.store.Delete(name)
	}
	// D-095 (b): never free a table something is still bound to — the binding would point at a
	// freed index and the first packet through it crashes VPP (V19). Re-verified from VPP right
	// before the delete; the scheduler deletes known bindings first (their Dependencies).
	users, err := TableUsers(ctx, d.client, d.store, rec.Index)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return fmt.Errorf("%s: %w: %q (index %d) is still referenced by %v", TableName, ErrTableInUse, name, rec.Index, users)
	}
	if err := deleteTable(ctx, d.client, rec.Index); err != nil {
		return err
	}
	return d.store.Delete(name)
}

// deleteTable removes table index. The handler validates mask_len == match_n_vectors × 16 on
// delete too (-7 otherwise, verified on vrx-a), so a consistent dummy geometry is sent along.
func deleteTable(ctx context.Context, c vpp.Client, index uint32) error {
	req := &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: index, Nbuckets: DefaultNbuckets, MemorySize: DefaultMemorySize,
		MatchNVectors: DefaultMatchNVectors, MaskLen: VectorSize, Mask: make([]byte, VectorSize), NextTableIndex: NoIndex, MissNextIndex: NoIndex}
	if _, err := classifyapi.NewServiceClient(c).ClassifyAddDelTable(ctx, req); err != nil {
		return fmt.Errorf("classify_add_del_table: %w", err)
	}
	return nil
}

type liveTable struct {
	rec  TableRecord
	info *classifyapi.ClassifyTableInfoReply
}

// snapshot classifies the Store's records against VPP without changing anything: a record is
// live when the store belongs to the running VPP instance, classify_table_ids lists its index
// and classify_table_info shows its geometry (skip/match vectors, mask). Everything else is
// stale — in particular an index VPP has reused for another owner's table is never claimed.
// sameInstance is false when the whole store predates the running VPP.
//
// The instance is the D-080 boot identity (bootid.Current): table indices are only meaningful
// within one VPP process, and the PID alone repeats across host reboots.
func snapshot(ctx context.Context, c vpp.Client, st Store) (live []liveTable, stale []string, inst bootid.Identity, sameInstance bool, err error) {
	if inst, err = bootid.Current(ctx, c); err != nil {
		return nil, nil, bootid.Identity{}, false, err
	}
	recs := st.All() // before the VPP reads: a record added later is not judged on an old snapshot
	if have, known := st.Instance(); !known || !have.Equal(inst) {
		for _, r := range recs {
			stale = append(stale, r.Name)
		}
		return nil, stale, inst, false, nil
	}
	svc := classifyapi.NewServiceClient(c)
	rep, err := svc.ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, nil, bootid.Identity{}, false, fmt.Errorf("classify_table_ids: %w", err)
	}
	ids := map[uint32]bool{}
	for _, id := range rep.Ids {
		ids[id] = true
	}
	for _, rec := range recs {
		if !ids[rec.Index] {
			stale = append(stale, rec.Name)
			continue
		}
		info, err := svc.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: rec.Index})
		var apiErr api.VPPApiError
		if errors.As(err, &apiErr) {
			stale = append(stale, rec.Name) // deleted since classify_table_ids
			continue
		}
		if err != nil {
			return nil, nil, bootid.Identity{}, false, fmt.Errorf("classify_table_info %d: %w", rec.Index, err)
		}
		if !sameGeometry(rec, info) {
			stale = append(stale, rec.Name)
			continue
		}
		live = append(live, liveTable{rec: rec, info: info})
	}
	return live, stale, inst, true, nil
}

// sameGeometry reports whether VPP's table has the geometry the record was created with.
func sameGeometry(rec TableRecord, info *classifyapi.ClassifyTableInfoReply) bool {
	if len(rec.Mask) == 0 || info.SkipNVectors != rec.SkipNVectors || info.MatchNVectors != rec.MatchNVectors {
		return false
	}
	n := int(rec.MatchNVectors) * VectorSize
	return bytes.Equal(fit(info.Mask, n), fit(rec.Mask, n))
}

// LiveTables returns the Store records that are live in VPP (see snapshot). It never changes
// the Store.
func LiveTables(ctx context.Context, c vpp.Client, st Store) ([]TableRecord, error) {
	live, _, _, _, err := snapshot(ctx, c, st)
	if err != nil {
		return nil, err
	}
	out := make([]TableRecord, 0, len(live))
	for _, l := range live {
		out = append(out, l.rec)
	}
	return out, nil
}

// Prune drops stale records (see snapshot) under the Store's transaction lock, on a fresh
// snapshot taken inside the lock; a store of an earlier VPP instance is reset.
func Prune(ctx context.Context, c vpp.Client, st Store) error {
	st.Lock()
	defer st.Unlock()
	return pruneLocked(ctx, c, st)
}

func pruneLocked(ctx context.Context, c vpp.Client, st Store) error {
	_, stale, inst, same, err := snapshot(ctx, c, st)
	if err != nil {
		return err
	}
	if !same {
		return st.Reset(inst)
	}
	for _, name := range stale {
		if err := st.Delete(name); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve reports every live owned table (see snapshot) from classify_table_info, then
// prunes stale records under the transaction lock (collect first, prune after).
func (d *TableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	live, _, _, _, err := snapshot(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	byIndex := map[uint32]string{}
	for _, l := range live {
		byIndex[l.rec.Index] = l.rec.Name
	}
	out := make([]scheduler.KV, 0, len(live))
	for _, l := range live {
		rec, info := l.rec, l.info
		v := &Table{
			Name:              rec.Name,
			Nbuckets:          info.Nbuckets,
			MemorySize:        rec.MemorySize,
			SkipNVectors:      info.SkipNVectors,
			MatchNVectors:     info.MatchNVectors,
			Mask:              fit(info.Mask, int(info.MatchNVectors)*VectorSize),
			MissNextIndex:     info.MissNextIndex,
			CurrentDataFlag:   rec.CurrentDataFlag,
			CurrentDataOffset: rec.CurrentDataOffset,
		}
		if info.NextTableIndex != NoIndex {
			if name, ok := byIndex[info.NextTableIndex]; ok {
				v.NextTable = name
			} else {
				v.NextTable = fmt.Sprintf("#%d", info.NextTableIndex)
			}
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: TableMeta{Index: rec.Index}})
	}
	if err := Prune(ctx, d.client, d.store); err != nil {
		return nil, err
	}
	return out, nil
}
