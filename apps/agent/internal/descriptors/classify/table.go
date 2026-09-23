package classify

import (
	"context"
	"fmt"
	"math/bits"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
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

// Create implements scheduler.Descriptor.
func (d *TableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	t := NormalizeTable(obj.(*Table))
	if t.GetName() == "" {
		return nil, fmt.Errorf("%s: name is required", TableName)
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
	if t.GetCurrentDataOffset() < -32768 || t.GetCurrentDataOffset() > 32767 {
		return nil, fmt.Errorf("%s: current_data_offset %d out of int16 range", TableName, t.GetCurrentDataOffset())
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
		MemorySize: t.GetMemorySize(), CurrentDataFlag: t.GetCurrentDataFlag(), CurrentDataOffset: t.GetCurrentDataOffset(),
	}
	if err := d.store.Put(rec); err != nil {
		return nil, err
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
	m, ok := meta.(TableMeta)
	if !ok {
		rec, err := d.indexOf(name)
		if err != nil {
			return fmt.Errorf("%s: %w %T and %v", TableName, df2.ErrBadMeta, meta, err)
		}
		m = TableMeta{Index: rec.Index}
	}
	req := &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: m.Index, Nbuckets: DefaultNbuckets, MemorySize: DefaultMemorySize, MatchNVectors: DefaultMatchNVectors, NextTableIndex: NoIndex, MissNextIndex: NoIndex}
	if _, err := classifyapi.NewServiceClient(d.client).ClassifyAddDelTable(ctx, req); err != nil {
		return fmt.Errorf("classify_add_del_table: %w", err)
	}
	return d.store.Delete(name)
}

// LiveTables returns the store records whose index VPP still lists (classify_table_ids);
// records of vanished tables are dropped from the store.
func LiveTables(ctx context.Context, c vpp.Client, store Store) ([]TableRecord, error) {
	rep, err := classifyapi.NewServiceClient(c).ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, fmt.Errorf("classify_table_ids: %w", err)
	}
	live := map[uint32]bool{}
	for _, id := range rep.Ids {
		live[id] = true
	}
	var out []TableRecord
	for _, rec := range store.All() {
		if !live[rec.Index] {
			_ = store.Delete(rec.Name)
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// Retrieve reads every owned, still existing table with classify_table_info.
func (d *TableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := LiveTables(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	byIndex := map[uint32]string{}
	for _, r := range recs {
		byIndex[r.Index] = r.Name
	}
	svc := classifyapi.NewServiceClient(d.client)
	var out []scheduler.KV
	for _, rec := range recs {
		info, err := svc.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: rec.Index})
		if err != nil {
			return nil, fmt.Errorf("classify_table_info %d: %w", rec.Index, err)
		}
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
	return out, nil
}
