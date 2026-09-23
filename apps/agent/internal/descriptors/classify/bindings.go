package classify

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names of the interface bindings.
const (
	InterfaceIPTableName  = "classify.interface-ip-table"  // keys: <interface>/<ipv4|ipv6>
	InterfaceL2TablesName = "classify.interface-l2-tables" // keys: <interface>/<input|output>
	InputACLName          = "classify.input-acl"           // keys: <interface>
	OutputACLName         = "classify.output-acl"          // keys: <interface>
)

// BindingMeta is the runtime handle of every binding descriptor.
type BindingMeta struct{ SwIfIndex uint32 }

// binder is what the four binding descriptors share: interface resolution and table name →
// index lookup through the store.
type binder struct {
	client vpp.Client
	owner  string
	store  Store
	opts   df2.Options
}

func newBinder(c vpp.Client, owner string, store Store, opts []df2.Option) binder {
	return binder{client: c, owner: owner, store: store, opts: df2.BuildOptions(opts...)}
}

// ifIndex resolves the interface a binding is created on; interfaces of another owner are
// refused (df2.ErrForeignInterface), untagged ones must be claimed after Create.
func (b binder) ifIndex(ctx context.Context, name string) (interface_types.InterfaceIndex, error) {
	idx, _, err := b.resolve(ctx, name)
	return idx, err
}

func (b binder) resolve(ctx context.Context, name string) (interface_types.InterfaceIndex, bool, error) {
	ifs, err := df2.DumpInterfaces(ctx, b.client, b.owner)
	if err != nil {
		return 0, false, err
	}
	idx, untagged, err := ifs.Resolve(name)
	return interface_types.InterfaceIndex(idx), untagged, err
}

// tableIndex resolves a table name; "" is NoIndex.
func (b binder) tableIndex(name string) (uint32, error) {
	if name == "" {
		return NoIndex, nil
	}
	rec, ok := b.store.Get(name)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoSuchTable, name)
	}
	return rec.Index, nil
}

func tableDeps(iface string, tables ...string) []scheduler.Dependency {
	deps := []scheduler.Dependency{df2.InterfaceDep(iface)}
	for _, t := range tables {
		if t != "" {
			deps = append(deps, scheduler.Dependency{Key: TableKey(t)})
		}
	}
	return deps
}

func metaIndex(name string, meta any) (interface_types.InterfaceIndex, error) {
	m, ok := meta.(BindingMeta)
	if !ok {
		return 0, fmt.Errorf("%s: %w %T", name, df2.ErrBadMeta, meta)
	}
	return interface_types.InterfaceIndex(m.SwIfIndex), nil
}

// ---- interface ip table --------------------------------------------------------------------

// InterfaceIPTableDescriptor binds a table to the ip4/ip6-classify lookup of an interface
// (classify_set_interface_ip_table). Write-only: VPP offers no readback.
type InterfaceIPTableDescriptor struct{ binder }

// NewInterfaceIPTable returns the descriptor.
func NewInterfaceIPTable(c vpp.Client, owner string, store Store, opts ...df2.Option) *InterfaceIPTableDescriptor {
	return &InterfaceIPTableDescriptor{newBinder(c, owner, store, opts)}
}

// Name implements scheduler.Descriptor.
func (*InterfaceIPTableDescriptor) Name() string { return InterfaceIPTableName }

func afOf(ipv6 bool) string {
	if ipv6 {
		return "ipv6"
	}
	return "ipv4"
}

// KeyOf implements scheduler.Descriptor.
func (*InterfaceIPTableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*InterfaceIpTable)
	return scheduler.Join(InterfaceIPTableName, o.GetInterface(), afOf(o.GetIpv6()))
}

// Dependencies implements scheduler.Descriptor.
func (*InterfaceIPTableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*InterfaceIpTable)
	return tableDeps(o.GetInterface(), o.GetTable())
}

func (d *InterfaceIPTableDescriptor) set(ctx context.Context, o *InterfaceIpTable, idx interface_types.InterfaceIndex, table uint32) error {
	req := &classifyapi.ClassifySetInterfaceIPTable{IsIPv6: o.GetIpv6(), SwIfIndex: idx, TableIndex: table}
	if _, err := classifyapi.NewServiceClient(d.client).ClassifySetInterfaceIPTable(ctx, req); err != nil {
		return fmt.Errorf("classify_set_interface_ip_table: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceIPTableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o := obj.(*InterfaceIpTable)
	if o.GetTable() == "" {
		return nil, fmt.Errorf("%s: table is required", InterfaceIPTableName)
	}
	table, err := d.tableIndex(o.GetTable())
	if err != nil {
		return nil, err
	}
	idx, err := d.ifIndex(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, o, idx, table); err != nil {
		return nil, err
	}
	return BindingMeta{SwIfIndex: uint32(idx)}, nil
}

// Update changes the table in place; interface or family are the key.
func (d *InterfaceIPTableDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	idx, err := metaIndex(InterfaceIPTableName, meta)
	if err != nil {
		return nil, err
	}
	o := newObj.(*InterfaceIpTable)
	table, err := d.tableIndex(o.GetTable())
	if err != nil {
		return nil, err
	}
	if table == NoIndex {
		return nil, fmt.Errorf("%s: table is required", InterfaceIPTableName)
	}
	if err := d.set(ctx, o, idx, table); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete unbinds (table index ~0).
func (d *InterfaceIPTableDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	idx, err := metaIndex(InterfaceIPTableName, meta)
	if err != nil {
		return err
	}
	return d.set(ctx, obj.(*InterfaceIpTable), idx, NoIndex)
}

// Retrieve is unsupported: VPP has no readback for this binding.
func (*InterfaceIPTableDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", InterfaceIPTableName, df2.ErrRetrieveUnsupported)
}

// ---- interface l2 tables -------------------------------------------------------------------

// InterfaceL2TablesDescriptor binds the L2 input or output classifier tables of an interface
// (classify_set_interface_l2_tables). Write-only: VPP offers no readback.
type InterfaceL2TablesDescriptor struct{ binder }

// NewInterfaceL2Tables returns the descriptor.
func NewInterfaceL2Tables(c vpp.Client, owner string, store Store, opts ...df2.Option) *InterfaceL2TablesDescriptor {
	return &InterfaceL2TablesDescriptor{newBinder(c, owner, store, opts)}
}

// Name implements scheduler.Descriptor.
func (*InterfaceL2TablesDescriptor) Name() string { return InterfaceL2TablesName }

func dirOf(input bool) string {
	if input {
		return "input"
	}
	return "output"
}

// KeyOf implements scheduler.Descriptor.
func (*InterfaceL2TablesDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*InterfaceL2Tables)
	return scheduler.Join(InterfaceL2TablesName, o.GetInterface(), dirOf(o.GetInput()))
}

// Dependencies implements scheduler.Descriptor.
func (*InterfaceL2TablesDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*InterfaceL2Tables)
	return tableDeps(o.GetInterface(), o.GetIp4Table(), o.GetIp6Table(), o.GetOtherTable())
}

func (d *InterfaceL2TablesDescriptor) set(ctx context.Context, o *InterfaceL2Tables, idx interface_types.InterfaceIndex, unbind bool) error {
	ip4, ip6, other := NoIndex, NoIndex, NoIndex
	if !unbind {
		var err error
		if ip4, err = d.tableIndex(o.GetIp4Table()); err != nil {
			return err
		}
		if ip6, err = d.tableIndex(o.GetIp6Table()); err != nil {
			return err
		}
		if other, err = d.tableIndex(o.GetOtherTable()); err != nil {
			return err
		}
		if ip4 == NoIndex && ip6 == NoIndex && other == NoIndex {
			return fmt.Errorf("%s: at least one table is required", InterfaceL2TablesName)
		}
	}
	req := &classifyapi.ClassifySetInterfaceL2Tables{SwIfIndex: idx, IP4TableIndex: ip4, IP6TableIndex: ip6, OtherTableIndex: other, IsInput: o.GetInput()}
	if _, err := classifyapi.NewServiceClient(d.client).ClassifySetInterfaceL2Tables(ctx, req); err != nil {
		return fmt.Errorf("classify_set_interface_l2_tables: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InterfaceL2TablesDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o := obj.(*InterfaceL2Tables)
	idx, err := d.ifIndex(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, o, idx, false); err != nil {
		return nil, err
	}
	return BindingMeta{SwIfIndex: uint32(idx)}, nil
}

// Update replaces the table set in place.
func (d *InterfaceL2TablesDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	idx, err := metaIndex(InterfaceL2TablesName, meta)
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, newObj.(*InterfaceL2Tables), idx, false); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete clears all three tables.
func (d *InterfaceL2TablesDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	idx, err := metaIndex(InterfaceL2TablesName, meta)
	if err != nil {
		return err
	}
	return d.set(ctx, obj.(*InterfaceL2Tables), idx, true)
}

// Retrieve is unsupported: VPP has no readback for this binding.
func (*InterfaceL2TablesDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", InterfaceL2TablesName, df2.ErrRetrieveUnsupported)
}

// ---- input / output acl --------------------------------------------------------------------

// aclTables is the common shape of InputAcl and OutputAcl.
type aclTables struct{ iface, ip4, ip6, l2 string }

func (b binder) aclIndices(t aclTables) (ip4, ip6, l2 uint32, err error) {
	if ip4, err = b.tableIndex(t.ip4); err != nil {
		return
	}
	if ip6, err = b.tableIndex(t.ip6); err != nil {
		return
	}
	if l2, err = b.tableIndex(t.l2); err != nil {
		return
	}
	if ip4 == NoIndex && ip6 == NoIndex && l2 == NoIndex {
		err = fmt.Errorf("at least one table is required")
	}
	return
}

// InputACLDescriptor binds the input ACL classifier tables of an interface
// (input_acl_set_interface); read back with classify_table_by_interface.
type InputACLDescriptor struct{ binder }

// NewInputACL returns the descriptor.
func NewInputACL(c vpp.Client, owner string, store Store, opts ...df2.Option) *InputACLDescriptor {
	return &InputACLDescriptor{newBinder(c, owner, store, opts)}
}

// Name implements scheduler.Descriptor.
func (*InputACLDescriptor) Name() string { return InputACLName }

// KeyOf implements scheduler.Descriptor.
func (*InputACLDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(InputACLName, obj.(*InputAcl).GetInterface())
}

// Dependencies implements scheduler.Descriptor.
func (*InputACLDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*InputAcl)
	return tableDeps(o.GetInterface(), o.GetIp4Table(), o.GetIp6Table(), o.GetL2Table())
}

func (d *InputACLDescriptor) set(ctx context.Context, o *InputAcl, idx interface_types.InterfaceIndex, isAdd bool) error {
	ip4, ip6, l2, err := d.aclIndices(aclTables{o.GetInterface(), o.GetIp4Table(), o.GetIp6Table(), o.GetL2Table()})
	if err != nil {
		return fmt.Errorf("%s: %w", InputACLName, err)
	}
	req := &classifyapi.InputACLSetInterface{SwIfIndex: idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: isAdd}
	if _, err := classifyapi.NewServiceClient(d.client).InputACLSetInterface(ctx, req); err != nil {
		return fmt.Errorf("input_acl_set_interface: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *InputACLDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o := obj.(*InputAcl)
	idx, untagged, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, o, idx, true); err != nil {
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(o)); err != nil {
		return nil, err
	}
	return BindingMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: the table set is replaced by unbind + bind.
func (*InputACLDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete unbinds the same tables.
func (d *InputACLDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	idx, err := metaIndex(InputACLName, meta)
	if err != nil {
		return err
	}
	if err := d.set(ctx, obj.(*InputAcl), idx, false); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// Retrieve queries classify_table_by_interface on every interface of this owner (tagged, or
// untagged with a claimed binding).
func (d *InputACLDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	recs, err := LiveTables(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	names := map[uint32]string{}
	for _, r := range recs {
		names[r.Index] = r.Name
	}
	svc := classifyapi.NewServiceClient(d.client)
	var out []scheduler.KV
	for _, idx := range ifs.Candidates() {
		rep, err := svc.ClassifyTableByInterface(ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if df2.InterfaceVanished(err) {
			continue // deleted after the interface snapshot
		}
		if err != nil {
			return nil, fmt.Errorf("classify_table_by_interface %d: %w", idx, err)
		}
		if rep.IP4TableID == NoIndex && rep.IP6TableID == NoIndex && rep.L2TableID == NoIndex {
			continue
		}
		name, _ := ifs.Name(idx)
		v := &InputAcl{Interface: name, Ip4Table: tableName(names, rep.IP4TableID), Ip6Table: tableName(names, rep.IP6TableID), L2Table: tableName(names, rep.L2TableID)}
		if !ifs.OwnsObject(idx, d.KeyOf(v), d.opts.Claims) {
			continue
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: BindingMeta{SwIfIndex: idx}})
	}
	return out, nil
}

// tableName renders a dumped table index: "" for none, the owner's name, or "#<idx>" for a
// table this owner does not know.
func tableName(names map[uint32]string, idx uint32) string {
	if idx == NoIndex {
		return ""
	}
	if n, ok := names[idx]; ok {
		return n
	}
	return fmt.Sprintf("#%d", idx)
}

// OutputACLDescriptor binds the output ACL classifier tables of an interface
// (output_acl_set_interface). Read back partially: VPP reports whether the ip4-outacl /
// ip6-outacl features are enabled (feature_is_enabled) but not which table, so the table
// names and indices bound come from the Store's OutputRecord (like memory_size). The L2 output
// ACL has no readback at all and is refused (use a separate write-only path if ever needed).
type OutputACLDescriptor struct{ binder }

// NewOutputACL returns the descriptor.
func NewOutputACL(c vpp.Client, owner string, store Store, opts ...df2.Option) *OutputACLDescriptor {
	return &OutputACLDescriptor{newBinder(c, owner, store, opts)}
}

// Name implements scheduler.Descriptor.
func (*OutputACLDescriptor) Name() string { return OutputACLName }

// KeyOf implements scheduler.Descriptor.
func (*OutputACLDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(OutputACLName, obj.(*OutputAcl).GetInterface())
}

// Dependencies implements scheduler.Descriptor.
func (*OutputACLDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*OutputAcl)
	return tableDeps(o.GetInterface(), o.GetIp4Table(), o.GetIp6Table(), o.GetL2Table())
}

// Output-ACL feature arcs (vnet/classify/in_out_acl.c).
const (
	outArc4, outFeature4 = "ip4-output", "ip4-outacl"
	outArc6, outFeature6 = "ip6-output", "ip6-outacl"
)

func (d *OutputACLDescriptor) call(ctx context.Context, idx interface_types.InterfaceIndex, ip4, ip6 uint32, isAdd bool) error {
	req := &classifyapi.OutputACLSetInterface{SwIfIndex: idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: NoIndex, IsAdd: isAdd}
	if _, err := classifyapi.NewServiceClient(d.client).OutputACLSetInterface(ctx, req); err != nil {
		return fmt.Errorf("output_acl_set_interface: %w", err)
	}
	return nil
}

// unbind removes the binding recorded for the interface. VPP returns 0 on an add while the
// feature is already enabled, without switching the table, so a new binding is only applied
// after the old one is gone (A→B otherwise silently keeps A).
func (d *OutputACLDescriptor) unbind(ctx context.Context, idx interface_types.InterfaceIndex, rec OutputRecord) error {
	if rec.Ip4Table == "" && rec.Ip6Table == "" {
		return nil
	}
	ip4, ip6 := NoIndex, NoIndex
	if rec.Ip4Table != "" {
		ip4 = rec.Ip4Index
	}
	if rec.Ip6Table != "" {
		ip6 = rec.Ip6Index
	}
	if err := d.call(ctx, idx, ip4, ip6, false); err != nil {
		return err
	}
	return d.store.DeleteOutput(rec.Interface)
}

// Create implements scheduler.Descriptor: an existing recorded binding on the interface is
// removed first, then the new tables are bound and recorded.
func (d *OutputACLDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o := obj.(*OutputAcl)
	if o.GetL2Table() != "" {
		return nil, fmt.Errorf("%s: l2_table is not supported (VPP has no readback for the L2 output ACL)", OutputACLName)
	}
	idx, untagged, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	d.store.Lock()
	defer d.store.Unlock()
	ip4, ip6, _, err := d.aclIndices(aclTables{o.GetInterface(), o.GetIp4Table(), o.GetIp6Table(), ""})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", OutputACLName, err)
	}
	if old, ok := d.store.GetOutput(o.GetInterface()); ok {
		if err := d.unbind(ctx, idx, old); err != nil {
			return nil, fmt.Errorf("%s: unbind previous tables: %w", OutputACLName, err)
		}
	} else {
		// No record, but the feature is on: an unknown table is bound and an add would be a
		// silent no-op. Refuse instead of reporting success.
		for _, f := range []struct {
			arc, feature string
			want         bool
		}{{outArc4, outFeature4, ip4 != NoIndex}, {outArc6, outFeature6, ip6 != NoIndex}} {
			if !f.want {
				continue
			}
			on, err := featureEnabled(ctx, d.client, f.arc, f.feature, uint32(idx))
			if err != nil {
				return nil, err
			}
			if on {
				return nil, fmt.Errorf("%s: %s already has an output ACL (%s) bound outside this agent's records", OutputACLName, o.GetInterface(), f.feature)
			}
		}
	}
	if err := d.call(ctx, idx, ip4, ip6, true); err != nil {
		return nil, err
	}
	rec := OutputRecord{Interface: o.GetInterface(), Ip4Table: o.GetIp4Table(), Ip4Index: ip4, Ip6Table: o.GetIp6Table(), Ip6Index: ip6}
	if err := d.store.PutOutput(rec); err != nil {
		_ = d.call(ctx, idx, ip4, ip6, false)
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(o)); err != nil {
		return nil, err
	}
	return BindingMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: the table set is replaced by unbind + bind (the
// scheduler's Delete + Create).
func (*OutputACLDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete unbinds the recorded tables (their indices as bound, not re-resolved names).
func (d *OutputACLDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	idx, err := metaIndex(OutputACLName, meta)
	if err != nil {
		return err
	}
	o := obj.(*OutputAcl)
	d.store.Lock()
	defer d.store.Unlock()
	rec, ok := d.store.GetOutput(o.GetInterface())
	if !ok {
		return fmt.Errorf("%s: no recorded binding on %q (tables bound outside this agent cannot be unbound: VPP requires the bound table index)", OutputACLName, o.GetInterface())
	}
	if err := d.unbind(ctx, idx, rec); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(o))
}

func featureEnabled(ctx context.Context, c vpp.Client, arc, feature string, idx uint32) (bool, error) {
	rep, err := featureapi.NewServiceClient(c).FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: arc, FeatureName: feature, SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return false, fmt.Errorf("feature_is_enabled %s/%s %d: %w", arc, feature, idx, err)
	}
	return rep.IsEnabled, nil
}

// Retrieve reports an output ACL on every interface of this owner where VPP has ip4-outacl
// or ip6-outacl enabled (feature_is_enabled). The table names come from the Store's record
// (only for the families VPP shows enabled, and only while the recorded tables are live);
// an enabled family without a usable record is reported as "#unknown" so it diffs.
func (d *OutputACLDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	recs, err := LiveTables(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	live := map[string]uint32{}
	for _, r := range recs {
		live[r.Name] = r.Index
	}
	pick := func(enabled bool, name string, index uint32) string {
		if !enabled {
			return ""
		}
		if i, ok := live[name]; name != "" && ok && i == index {
			return name
		}
		return "#unknown"
	}
	var out []scheduler.KV
	for _, idx := range ifs.Candidates() {
		name, _ := ifs.Name(idx)
		key := scheduler.Join(OutputACLName, name)
		if !ifs.OwnsObject(idx, key, d.opts.Claims) {
			continue
		}
		on4, err := featureEnabled(ctx, d.client, outArc4, outFeature4, idx)
		if df2.InterfaceVanished(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		on6, err := featureEnabled(ctx, d.client, outArc6, outFeature6, idx)
		if df2.InterfaceVanished(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !on4 && !on6 {
			continue
		}
		rec, _ := d.store.GetOutput(name)
		v := &OutputAcl{Interface: name, Ip4Table: pick(on4, rec.Ip4Table, rec.Ip4Index), Ip6Table: pick(on6, rec.Ip6Table, rec.Ip6Index)}
		out = append(out, scheduler.KV{Key: key, Value: v, Meta: BindingMeta{SwIfIndex: idx}})
	}
	return out, nil
}

// Register registers the classify descriptors that VPP can read back (table, session,
// input-acl, output-acl) with r, all sharing store (nil = MemStore). The write-only bindings
// (interface-ip-table, interface-l2-tables: no readback in VPP 26.06) are left out; see
// RegisterWriteOnly and D-063.
func Register(r scheduler.Registry, c vpp.Client, owner string, store Store, opts ...df2.Option) {
	if store == nil {
		store = NewMemStore()
	}
	r.Register(NewTable(c, store))
	r.Register(NewSession(c, store))
	r.Register(NewInputACL(c, owner, store, opts...))
	r.Register(NewOutputACL(c, owner, store, opts...))
}

// RegisterWriteOnly registers the write-only bindings (interface-ip-table,
// interface-l2-tables). Their Retrieve returns df2.ErrRetrieveUnsupported: only a reconciler
// implementing D-063 (re-apply on every resync with idempotent Create, never delete on
// absence, skip verification) may register them. Removing one from the desired state does
// not unbind it in VPP after an agent restart.
func RegisterWriteOnly(r scheduler.Registry, c vpp.Client, owner string, store Store, opts ...df2.Option) {
	r.Register(NewInterfaceIPTable(c, owner, store, opts...))
	r.Register(NewInterfaceL2Tables(c, owner, store, opts...))
}
