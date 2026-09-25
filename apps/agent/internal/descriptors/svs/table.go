package svs

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/scheduler"
)

// TableDescriptor manages svs tables: an IPv4 and an IPv6 FIB table named "<owner>:svs:<id>" (ip_table_add_del,
// idempotent — VPP keeps one API lock however often it is added).
type TableDescriptor struct{ Env }

var (
	_ scheduler.Descriptor = (*TableDescriptor)(nil)
	_ scheduler.Reapplier  = (*TableDescriptor)(nil)
)

// Name implements scheduler.Descriptor.
func (*TableDescriptor) Name() string { return TableName }

func asTable(obj proto.Message) *Table {
	if v, ok := obj.(*Table); ok && v != nil {
		return v
	}
	return &Table{}
}

// KeyOf implements scheduler.Descriptor.
func (*TableDescriptor) KeyOf(obj proto.Message) scheduler.Key { return TableKey(asTable(obj).GetId()) }

// Dependencies implements scheduler.Descriptor.
func (*TableDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *TableDescriptor) send(ctx context.Context, id uint32, name string, v6, add bool) error {
	if _, err := ip.NewServiceClient(d.Client).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: add, Table: ip.IPTable{TableID: id, IsIP6: v6, Name: name}}); err != nil {
		return fmt.Errorf("ip_table_add_del %d ipv6=%v add=%v: %w", id, v6, add, err)
	}
	return nil
}

// ensure creates or re-asserts both families under the claim rule: a family under any other name fails before
// anything is sent. It returns the families this call created.
func (d *TableDescriptor) ensure(ctx context.Context, v *Table) ([]bool, error) {
	if v.GetId() == 0 {
		return nil, fmt.Errorf("%w: table 0 is VPP's default table", ErrBadValue)
	}
	name, err := VPPTableName(d.Owner, v.GetId())
	if err != nil {
		return nil, err
	}
	all, err := tableNames(ctx, d.Client)
	if err != nil {
		return nil, err
	}
	for _, v6 := range []bool{false, true} {
		if n, ok := all[tableFam{v.GetId(), v6}]; ok && n != name {
			return nil, fmt.Errorf("%w: table %d ipv6=%v is named %q, we are %q", ErrTableConflict, v.GetId(), v6, n, name)
		}
	}
	var created []bool
	for _, v6 := range []bool{false, true} {
		if err := d.send(ctx, v.GetId(), name, v6, true); err != nil {
			return created, err
		}
		if _, existed := all[tableFam{v.GetId(), v6}]; !existed {
			created = append(created, v6)
		}
	}
	return created, nil
}

// remove deletes the families of id that still carry our name (re-verified right before the delete, D-071).
func (d *TableDescriptor) remove(ctx context.Context, id uint32, only ...bool) error {
	name, err := VPPTableName(d.Owner, id)
	if err != nil {
		return err
	}
	all, err := tableNames(ctx, d.Client)
	if err != nil {
		return err
	}
	fams := only
	if len(fams) == 0 {
		fams = []bool{false, true}
	}
	for _, v6 := range fams {
		if n, ok := all[tableFam{id, v6}]; !ok || n != name {
			continue
		}
		if err := d.send(ctx, id, name, v6, false); err != nil {
			return err
		}
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *TableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*Table)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	created, err := d.ensure(ctx, v)
	if err != nil && len(created) > 0 {
		_ = d.remove(context.WithoutCancel(ctx), v.GetId(), created...) // never half a table pair
	}
	return nil, err
}

// Update implements scheduler.Descriptor: a missing family is re-added in place.
func (d *TableDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if asTable(oldObj).GetId() != asTable(newObj).GetId() {
		return nil, scheduler.ErrRecreate
	}
	_, err := d.ensure(ctx, asTable(newObj))
	return meta, err
}

// Reapply implements scheduler.Reapplier: VPP keeps a table alive while entries reference it even after its API lock
// is gone, so Retrieve cannot tell a lost lock from a healthy table; a resync re-asserts it (idempotent).
func (d *TableDescriptor) Reapply(ctx context.Context, obj proto.Message, _ any) error {
	_, err := d.ensure(ctx, asTable(obj))
	return err
}

// Delete implements scheduler.Descriptor (the reconciler has removed the table's routes and interface
// enablements before, V15: entries before their table).
func (d *TableDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	return d.remove(ctx, asTable(obj).GetId())
}

// Retrieve implements scheduler.Descriptor: tables named "<owner>:svs:<id>", both families merged.
func (d *TableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := ownedTables(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	ids := make([]uint32, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]scheduler.KV, 0, len(ids))
	for _, id := range ids {
		out = append(out, scheduler.KV{Key: TableKey(id), Value: &Table{Id: id, MissingIp4: !owned[id][false], MissingIp6: !owned[id][true]}})
	}
	return out, nil
}
