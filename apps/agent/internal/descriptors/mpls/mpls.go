// Package mpls holds the reconciler descriptors for core VPP MPLS (task DF-7, WBS D2.8): MPLS
// FIB tables (mpls_table_add_del / mpls_table_dump), MPLS on an interface
// (sw_interface_set_mpls_enable / mpls_interface_dump), label routes with out-label stacks
// (mpls_route_add_del / mpls_route_dump), local-label ↔ IP prefix bindings (mpls_ip_bind_unbind,
// write-only) and MPLS tunnels (mpls_tunnel_add_del / mpls_tunnel_dump).
//
// Key contract for DF-6 (SR-MPLS) and F-mpls: "mpls-table/<id>" and "mpls-interface/<name>"
// (the descriptor names are hyphenated for all five MPLS types, matching that contract).
//
// Ownership: a table carries the name vpp.OwnerTag(owner, "<id>"); routes belong to the owner
// of their table; an interface's MPLS enable belongs to the owner of the interface (D-071 claim
// rule); a tunnel carries mt_tag = vpp.OwnerTag(owner, name) and its interface is tagged with the
// same owner tag, so its logical name (D-069) is the tunnel name. Messages come only from
// apps/agent/binapi/{mpls,fib_types}. docs/agent/descriptors/mpls.md is the object ↔ message
// table.
package mpls

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names (the keys DF-6 depends on: mpls-table/<id>, mpls-interface/<name>).
const (
	NameTable     = "mpls-table"
	NameInterface = "mpls-interface"
	NameRoute     = "mpls-route"
	NameIPBind    = "mpls-ip-bind"
	NameTunnel    = "mpls-tunnel"
)

// MinLabel is the first unreserved label (RFC 3032: 0–15 are reserved); VPP's special entries
// in every table use the reserved ones and are never reported.
const MinLabel = 16

// ---- specs ------------------------------------------------------------------------------------

// Table is the desired state of one mpls-table object.
type Table struct {
	ID uint32 `json:"id,omitempty"`
}

// Validate checks t.
func (Table) Validate() error { return nil }

// Interface is the desired state of one mpls-interface object: MPLS enabled on the interface.
type Interface struct {
	Interface string `json:"interface,omitempty"`
}

// Validate checks i.
func (i Interface) Validate() error {
	if i.Interface == "" {
		return df7.Specf("mpls interface needs an interface")
	}
	return nil
}

// EOS payload protocols of an end-of-stack route (dpo_proto).
const (
	PayloadIP4      = "ip4"
	PayloadIP6      = "ip6"
	PayloadMPLS     = "mpls"
	PayloadEthernet = "ethernet"
)

var payloads = map[string]uint8{PayloadIP4: 0, PayloadIP6: 1, PayloadMPLS: 2, PayloadEthernet: 3}

// Route is the desired state of one mpls-route object: a local label in a table, end-of-stack
// or not, with its paths (next hops and pushed out-label stacks). Paths are canonical
// (df7.NormalizePaths). EOSProto is the payload after the label, EOS routes only.
type Route struct {
	Table     uint32     `json:"table,omitempty"`
	Label     uint32     `json:"label,omitempty"`
	EOS       bool       `json:"eos,omitempty"`
	EOSProto  string     `json:"eos_proto,omitempty"`
	Multicast bool       `json:"multicast,omitempty"`
	Paths     []df7.Path `json:"paths,omitempty"`
}

// Validate checks r (paths must already be canonical).
func (r Route) Validate() error {
	if r.Label < MinLabel || r.Label > df7.MaxLabel {
		return df7.Specf("mpls label %d outside %d..%d", r.Label, MinLabel, df7.MaxLabel)
	}
	if r.EOS {
		if _, ok := payloads[r.EOSProto]; !ok {
			return df7.Specf("eos route needs eos_proto ip4, ip6, mpls or ethernet")
		}
	} else if r.EOSProto != "" {
		return df7.Specf("eos_proto applies to end-of-stack routes only")
	}
	return checkPaths(r.Paths)
}

func checkPaths(paths []df7.Path) error {
	if len(paths) == 0 {
		return df7.Specf("at least one path is required")
	}
	norm, err := df7.NormalizePaths(paths)
	if err != nil {
		return err
	}
	for i := range norm {
		if fmt.Sprint(norm[i]) != fmt.Sprint(paths[i]) {
			return df7.Specf("paths are not canonical (df7.NormalizePaths): %+v", norm)
		}
	}
	return nil
}

// IPBind is the desired state of one mpls-ip-bind object: local label Label bound to IP prefix
// Prefix of IP table VRF (VPP creates the label's entries in MPLS table 0).
type IPBind struct {
	MPLSTable uint32 `json:"mpls_table,omitempty"`
	Label     uint32 `json:"label,omitempty"`
	VRF       uint32 `json:"vrf,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

// Validate checks b.
func (b IPBind) Validate() error {
	if b.Label < MinLabel || b.Label > df7.MaxLabel {
		return df7.Specf("mpls label %d outside %d..%d", b.Label, MinLabel, df7.MaxLabel)
	}
	_, err := df7.ParsePrefix(b.Prefix)
	return err
}

// Tunnel is the desired state of one mpls-tunnel object: a named MPLS tunnel interface with its
// paths (each with an out-label stack).
type Tunnel struct {
	Name      string     `json:"name,omitempty"`
	L2Only    bool       `json:"l2_only,omitempty"`
	Multicast bool       `json:"multicast,omitempty"`
	Paths     []df7.Path `json:"paths,omitempty"`
}

// Validate checks t.
func (t Tunnel) Validate() error {
	if t.Name == "" {
		return df7.Specf("mpls tunnel needs a name")
	}
	if _, err := vpp.OwnerTag("w00000", t.Name); err != nil {
		return df7.Specf("mpls tunnel name %q: %v", t.Name, err)
	}
	return checkPaths(t.Paths)
}

// ---- keys -------------------------------------------------------------------------------------

func u32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// KeyTable is "mpls-table/<id>".
func KeyTable(id uint32) scheduler.Key { return scheduler.Join(NameTable, u32(id)) }

// KeyInterface is "mpls-interface/<interface>".
func KeyInterface(ifName string) scheduler.Key { return scheduler.Join(NameInterface, ifName) }

func eosID(eos bool) string {
	if eos {
		return "eos"
	}
	return "neos"
}

// KeyRoute is "mpls-route/<table>/<label>/<eos|neos>".
func KeyRoute(table, label uint32, eos bool) scheduler.Key {
	return scheduler.Join(NameRoute, u32(table), u32(label), eosID(eos))
}

// KeyIPBind is "mpls-ip-bind/<mpls table>/<label>/<vrf>/<prefix>".
func KeyIPBind(b IPBind) scheduler.Key {
	return scheduler.Join(NameIPBind, u32(b.MPLSTable), u32(b.Label), u32(b.VRF), b.Prefix)
}

// KeyTunnel is "mpls-tunnel/<name>".
func KeyTunnel(name string) scheduler.Key { return scheduler.Join(NameTunnel, name) }

func sortKVs(kvs []scheduler.KV) {
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
}

// ownedTables returns the ids of this owner's MPLS tables (name = owner tag).
func ownedTables(ctx context.Context, c vpp.Client, owner string) (map[uint32]bool, error) {
	owned, _, err := tablesView(ctx, c, owner)
	return owned, err
}

// tablesView is one mpls_table_dump: the ids of this owner's tables (name = owner tag) and whether
// the shared table 0 exists at all, whoever named it (F-mpls-srmpls: VPP-global, D-071).
func tablesView(ctx context.Context, c vpp.Client, owner string) (owned map[uint32]bool, zero bool, err error) {
	stream, err := mpls.NewServiceClient(c).MplsTableDump(ctx, &mpls.MplsTableDump{})
	if err != nil {
		return nil, false, fmt.Errorf("mpls_table_dump: %w", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, false, fmt.Errorf("mpls_table_dump: %w", err)
	}
	owned = map[uint32]bool{}
	for _, d := range dets {
		if d.MtTable.MtTableID == SharedTable {
			zero = true
		}
		if id, ok := vpp.ParseOwnerTag(d.MtTable.MtName, owner); ok && id == u32(d.MtTable.MtTableID) {
			owned[d.MtTable.MtTableID] = true
		}
	}
	return owned, zero, nil
}

// readableTables are the tables whose label routes this owner reads and deletes: its own and, when
// it exists, the shared table 0 whoever created it (the globals owner, D-071) — in table 0 only the
// labels this owner recorded are ever reported or deleted (review H2). F-mpls-srmpls gap: before,
// table 0 counted only when it carried this owner's name, so an agent that is not the globals owner
// never reported its own table-0 label routes and its Delete forgot them without removing them.
func readableTables(ctx context.Context, c vpp.Client, owner string) (map[uint32]bool, error) {
	owned, zero, err := tablesView(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	if zero {
		owned[SharedTable] = true
	}
	return owned, nil
}

// ---- mpls-table -------------------------------------------------------------------------------

// TableDescriptor manages mpls-table objects.
type TableDescriptor struct {
	df7.Base
	// zero is set when this agent is not the globals owner (D-071): MPLS table 0 is then only
	// required, never created or deleted (see NewTableFor).
	zero *zeroRequirement
}

var _ scheduler.Descriptor = (*TableDescriptor)(nil)

// NewTable returns the mpls-table descriptor of the globals owner (D-071): it creates and deletes
// every table it is given, MPLS table 0 included.
func NewTable(c vpp.Client, owner string, opts ...df7.Option) *TableDescriptor {
	return &TableDescriptor{Base: df7.NewBase(NameTable, c, owner, opts)}
}

// NewTableFor returns the mpls-table descriptor for an agent in the given D-071 role
// (F-mpls-srmpls). The globals owner manages table 0 like any other table (NewTable). Any other
// agent only REQUIRES table 0: Create of mpls-table/0 succeeds while VPP has table 0 (whoever
// created it) and fails with dfkit.ErrNotGlobalsOwner otherwise, sending nothing; Delete never
// touches it; Retrieve reports mpls-table/0 exactly while it is required and exists — so plan,
// verification and resync converge, and a lost table 0 shows up as a failed requirement instead of
// being re-created by an agent that does not own it. Tables other than 0 are unaffected.
func NewTableFor(c vpp.Client, owner string, globalsOwner bool, opts ...df7.Option) *TableDescriptor {
	d := NewTable(c, owner, opts...)
	if !globalsOwner {
		d.zero = &zeroRequirement{}
	}
	return d
}

// zeroRequirement remembers whether mpls-table/0 is required (per process: an agent restart
// re-establishes it with the first Create, which only checks).
type zeroRequirement struct {
	mu       sync.Mutex
	required bool
}

func (z *zeroRequirement) set(v bool) {
	z.mu.Lock()
	z.required = v
	z.mu.Unlock()
}

func (z *zeroRequirement) get() bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.required
}

// requiresZero reports whether id is table 0 of an agent that is not the globals owner.
func (d *TableDescriptor) requiresZero(id uint32) bool { return d.zero != nil && id == SharedTable }

// KeyOf implements scheduler.Descriptor.
func (d *TableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	t, _ := df7.Decode[Table](obj)
	return KeyTable(t.ID)
}

// Dependencies implements scheduler.Descriptor: none.
func (*TableDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *TableDescriptor) addDel(ctx context.Context, id uint32, add bool) error {
	name, err := vpp.OwnerTag(d.Owner, u32(id))
	if err != nil {
		return err
	}
	_, err = mpls.NewServiceClient(d.Client).MplsTableAddDel(ctx, &mpls.MplsTableAddDel{MtIsAdd: add, MtTable: mpls.MplsTable{MtTableID: id, MtName: name}})
	return d.Wrap(fmt.Sprintf("mpls_table_add_del %d add=%v", id, add), err)
}

// Create implements scheduler.Descriptor. A table that exists under another owner's name is
// refused (VPP would only add a lock and keep the other name).
func (d *TableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := df7.DecodeValid[Table](obj)
	if err != nil {
		return nil, err
	}
	if d.requiresZero(t.ID) {
		_, zero, err := tablesView(ctx, d.Client, d.Owner)
		if err != nil {
			return nil, d.Wrap("retrieve", err)
		}
		if !zero {
			return nil, fmt.Errorf("%s: %w: MPLS table 0 does not exist in VPP; it is VPP-global and only the globals owner creates it (D-071)", NameTable, dfkit.ErrNotGlobalsOwner)
		}
		d.zero.set(true)
		return nil, nil
	}
	// table 0 is VPP's default MPLS table, never an allocated id: the globals owner declares it
	// whatever its id range (D-071)
	if t.ID != SharedTable {
		if err := d.Opts.CheckID("mpls table", t.ID); err != nil {
			return nil, err
		}
	}
	if err := d.foreign(ctx, t.ID); err != nil {
		return nil, err
	}
	return nil, d.addDel(ctx, t.ID, true)
}

// foreign fails when table id exists but is not ours.
func (d *TableDescriptor) foreign(ctx context.Context, id uint32) error {
	stream, err := mpls.NewServiceClient(d.Client).MplsTableDump(ctx, &mpls.MplsTableDump{})
	if err != nil {
		return d.Wrap("mpls_table_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return d.Wrap("mpls_table_dump", err)
	}
	for _, t := range dets {
		if t.MtTable.MtTableID != id {
			continue
		}
		if tid, ok := vpp.ParseOwnerTag(t.MtTable.MtName, d.Owner); !ok || tid != u32(id) {
			return fmt.Errorf("%s: %w: table %d exists as %q", NameTable, df7.ErrForeignInterface, id, t.MtTable.MtName)
		}
	}
	return nil
}

// Update implements scheduler.Descriptor: the id is the key; nothing else to change.
func (*TableDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: re-verify the table is ours right before deleting it
// (D-071); an absent table is success. Routes depend on the table and are removed first.
func (d *TableDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := df7.Decode[Table](obj)
	if err != nil {
		return err
	}
	if d.requiresZero(t.ID) {
		d.zero.set(false) // a requirement only: never deleted by an agent that does not own it
		return nil
	}
	owned, err := ownedTables(ctx, d.Client, d.Owner)
	if err != nil {
		return d.Wrap("retrieve", err)
	}
	if !owned[t.ID] {
		return nil
	}
	return d.addDel(ctx, t.ID, false)
}

// Retrieve implements scheduler.Descriptor: mpls_table_dump, tables named with this owner's tag;
// for an agent that is not the globals owner, table 0 exactly while it is required and exists.
func (d *TableDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, zero, err := tablesView(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, d.Wrap("retrieve", err)
	}
	if d.zero != nil {
		delete(owned, SharedTable)
		if zero && d.zero.get() {
			owned[SharedTable] = true
		}
	}
	out := make([]scheduler.KV, 0, len(owned))
	for id := range owned {
		out = append(out, df7.KV(KeyTable(id), Table{ID: id}, nil))
	}
	sortKVs(out)
	return out, nil
}

// ---- mpls-interface ---------------------------------------------------------------------------

// Meta is the interface index of an mpls-interface.
type Meta struct{ SwIfIndex uint32 }

// InterfaceDescriptor manages mpls-interface objects.
type InterfaceDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the mpls-interface descriptor.
func NewInterface(c vpp.Client, owner string, opts ...df7.Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{df7.NewBase(NameInterface, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	i, _ := df7.Decode[Interface](obj)
	return KeyInterface(i.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface and MPLS table 0 (VPP enables
// MPLS on an interface only while the default table exists).
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	i, _ := df7.Decode[Interface](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(i.Interface), {Key: KeyTable(0), Optional: true}}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx uint32, enable bool) error {
	_, err := mpls.NewServiceClient(d.Client).SwInterfaceSetMplsEnable(ctx, &mpls.SwInterfaceSetMplsEnable{SwIfIndex: interface_types.InterfaceIndex(idx), Enable: enable})
	return d.Wrap(fmt.Sprintf("sw_interface_set_mpls_enable %d enable=%v", idx, enable), err)
}

// enabled reports whether MPLS is on for idx (mpls_interface_dump).
func (d *InterfaceDescriptor) enabled(ctx context.Context, idx uint32) (bool, error) {
	stream, err := mpls.NewServiceClient(d.Client).MplsInterfaceDump(ctx, &mpls.MplsInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return false, d.Wrap("mpls_interface_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return false, d.Wrap("mpls_interface_dump", err)
	}
	return len(dets) > 0, nil
}

// Create implements scheduler.Descriptor (needs MPLS table 0 — NO_SUCH_FIB otherwise).
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	i, err := df7.DecodeValid[Interface](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, i.Interface, string(KeyInterface(i.Interface)))
	if err != nil {
		return nil, err
	}
	// claim-first (TD-11b, review 3.3; F-mpls-srmpls gap): a claim recorded after the enable left
	// an enabled, unclaimed interface whenever the claim failed — never reported, never disabled.
	c, err := tg.ClaimFirst(ctx)
	if err != nil {
		return nil, err
	}
	// the enable is a u8 counter: never a second reference for the same object (D-076); an
	// already enabled interface is ours only when tagged or claimed by us before (review M1)
	on, err := d.enabled(ctx, tg.Index)
	if err != nil {
		return nil, c.Undo(err)
	}
	if on {
		return Meta{SwIfIndex: tg.Index}, c.Adopt()
	}
	if err := d.set(ctx, tg.Index, true); err != nil {
		return nil, c.Undo(err)
	}
	return Meta{SwIfIndex: tg.Index}, nil
}

// Update implements scheduler.Descriptor: the interface is the key.
func (*InterfaceDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and send ONE disable
// — our one reference (review L1: further references belong to other consumers) — and only while
// mpls_interface_dump shows the interface enabled: VPP decrements a u8 counter without checking
// (mpls.c: a disable of a disabled interface wraps it and breaks the next enable).
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	i, err := df7.Decode[Interface](obj)
	if err != nil {
		return err
	}
	tg, found, err := d.Detach(ctx, i.Interface, string(KeyInterface(i.Interface)))
	if err != nil || !found {
		return err
	}
	on, err := d.enabled(ctx, tg.Index)
	if err != nil {
		return err
	}
	if on {
		if err := d.set(ctx, tg.Index, false); err != nil {
			return err
		}
	}
	return tg.Release()
}

// Retrieve implements scheduler.Descriptor: mpls_interface_dump, owned interfaces.
func (d *InterfaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := mpls.NewServiceClient(d.Client).MplsInterfaceDump(ctx, &mpls.MplsInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex)})
	if err != nil {
		return nil, d.Wrap("mpls_interface_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("mpls_interface_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		name, ok := ifs.Owned(uint32(det.SwIfIndex), func(n string) string { return string(KeyInterface(n)) })
		if !ok {
			continue
		}
		out = append(out, df7.KV(KeyInterface(name), Interface{Interface: name}, Meta{SwIfIndex: uint32(det.SwIfIndex)}))
	}
	sortKVs(out)
	return out, nil
}

// ---- mpls-route -------------------------------------------------------------------------------

// RouteDescriptor manages mpls-route objects.
type RouteDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*RouteDescriptor)(nil)

// NewRoute returns the mpls-route descriptor.
func NewRoute(c vpp.Client, owner string, opts ...df7.Option) *RouteDescriptor {
	return &RouteDescriptor{df7.NewBase(NameRoute, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *RouteDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	r, _ := df7.Decode[Route](obj)
	return KeyRoute(r.Table, r.Label, r.EOS)
}

// Dependencies implements scheduler.Descriptor: the table, and (optional) the next-hop
// interfaces.
func (d *RouteDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	r, _ := df7.Decode[Route](obj)
	deps := []scheduler.Dependency{{Key: KeyTable(r.Table)}}
	for _, n := range df7.PathInterfaces(r.Paths) {
		deps = append(deps, scheduler.Dependency{Key: d.Opts.InterfaceKey(n), Optional: true})
	}
	return append(deps, lookupTableDeps(r.Paths)...)
}

// lookupTableDeps are the IP tables (P05 core "vrf/<id>") the paths resolve their next hop in or
// look the payload up in (a pop-and-lookup path): VPP refuses a path into a missing table, so the
// table must exist first and outlive the path (F-mpls-srmpls gap: DF-7 had no dependency on them).
// Table 0 always exists.
func lookupTableDeps(paths []df7.Path) []scheduler.Dependency {
	var deps []scheduler.Dependency
	seen := map[uint32]bool{}
	for _, p := range paths {
		if p.TableID == 0 || seen[p.TableID] || (p.Proto != df7.ProtoIP4 && p.Proto != df7.ProtoIP6) {
			continue
		}
		seen[p.TableID] = true
		deps = append(deps, scheduler.Dependency{Key: df7.VRFKey(p.TableID)})
	}
	return deps
}

func (d *RouteDescriptor) addDel(ctx context.Context, r Route, add bool) error {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return err
	}
	var paths []fib_types.FibPath
	if add {
		paths, err = df7.EncodePaths(r.Paths, ifs)
		if err != nil {
			return err
		}
	}
	route := mpls.MplsRoute{MrTableID: r.Table, MrLabel: r.Label, MrIsMulticast: r.Multicast, MrNPaths: uint8(len(paths)), MrPaths: paths} //nolint:gosec // ≤ 255
	if r.EOS {
		route.MrEos, route.MrEosProto = 1, payloads[r.EOSProto]
	}
	_, err = mpls.NewServiceClient(d.Client).MplsRouteAddDel(ctx, &mpls.MplsRouteAddDel{MrIsAdd: add, MrRoute: route})
	return d.Wrap(fmt.Sprintf("mpls_route_add_del %s add=%v", KeyRoute(r.Table, r.Label, r.EOS), add), err)
}

// SharedTable is the MPLS table other features program too (SR-MPLS BSIDs of DF-6, the local
// labels of mpls-ip-bind, FRR/linux-cp labels): mpls_route_details carries no FIB source, so in
// this table only labels this owner recorded after its own successful add are ours (review H2).
const SharedTable = 0

func (d *RouteDescriptor) shared(r Route) bool { return r.Table == SharedTable }

// exists reports whether (label, eos) has an entry in table r.Table.
func (d *RouteDescriptor) exists(ctx context.Context, r Route) (bool, error) {
	stream, err := mpls.NewServiceClient(d.Client).MplsRouteDump(ctx, &mpls.MplsRouteDump{Table: mpls.MplsTable{MtTableID: r.Table}})
	if err != nil {
		return false, d.Wrap("mpls_route_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return false, d.Wrap("mpls_route_dump", err)
	}
	for _, det := range dets {
		if det.MrRoute.MrLabel == r.Label && (det.MrRoute.MrEos != 0) == r.EOS {
			return true, nil
		}
	}
	return false, nil
}

// Create implements scheduler.Descriptor: mpls_route_add_del (not multipath: the path set is
// replaced as a whole). In the shared table 0 an existing entry for the label is never taken
// over (another feature's label, ErrNotOurs), and the route is recorded (D-080 identity) after
// the add so Retrieve/Delete recognise it.
func (d *RouteDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	r, err := df7.DecodeValid[Route](obj)
	if err != nil {
		return nil, err
	}
	key := string(KeyRoute(r.Table, r.Label, r.EOS))
	recorded := false // this Create wrote the record (and so drops it again on failure)
	if d.shared(r) {
		ours, err := d.Recorded(ctx, key)
		if err != nil {
			return nil, err
		}
		taken, err := d.exists(ctx, r)
		if err != nil {
			return nil, err
		}
		if taken && !ours {
			return nil, fmt.Errorf("%s: %w: label %d/%s in MPLS table 0 belongs to another feature", NameRoute, dfkit.ErrNotOurs, r.Label, eosID(r.EOS))
		}
		// record-first (TD-11b, review 3.3; F-mpls-srmpls gap): a record written after the add
		// left an unrecorded label in table 0 whenever the write failed — never reported, never
		// deleted
		if !ours {
			if err := d.RecordNow(ctx, key, "route"); err != nil {
				return nil, err
			}
			recorded = true
		}
	}
	if err := d.addDel(ctx, r, true); err != nil {
		if recorded {
			if ferr := d.ForgetApplied(key); ferr != nil {
				return nil, fmt.Errorf("%w (and dropping the record: %v)", err, ferr)
			}
		}
		return nil, err
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: the same add replaces the paths in place; a change of
// the multicast flag or the EOS payload is ErrRecreate. In table 0 only a recorded route.
func (d *RouteDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Route](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Route](newObj)
	if err != nil {
		return nil, err
	}
	if o.Multicast != n.Multicast || o.EOSProto != n.EOSProto {
		return nil, scheduler.ErrRecreate
	}
	if d.shared(n) {
		ours, err := d.Recorded(ctx, string(KeyRoute(n.Table, n.Label, n.EOS)))
		if err != nil {
			return nil, err
		}
		if !ours {
			return nil, fmt.Errorf("%s: %w: label %d in MPLS table 0", NameRoute, dfkit.ErrNotOurs, n.Label)
		}
	}
	return nil, d.addDel(ctx, n, true)
}

// Delete implements scheduler.Descriptor: removes the API-sourced entry — only in a table this
// owner owns (re-verified, D-071) and, in table 0, only a route this owner recorded.
func (d *RouteDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	r, err := df7.Decode[Route](obj)
	if err != nil {
		return err
	}
	key := string(KeyRoute(r.Table, r.Label, r.EOS))
	readable, err := readableTables(ctx, d.Client, d.Owner)
	if err != nil {
		return d.Wrap("retrieve", err)
	}
	if !readable[r.Table] {
		return d.ForgetApplied(key) // the table (and every route in it) is gone
	}
	if d.shared(r) {
		ours, err := d.Recorded(ctx, key)
		if err != nil || !ours {
			return err
		}
	}
	if err := d.addDel(ctx, r, false); err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return err
	}
	return d.ForgetApplied(key)
}

// Retrieve implements scheduler.Descriptor: mpls_route_dump for every table of this owner,
// unreserved labels only (VPP's special entries use 0–15); in the shared table 0 only the labels
// this owner recorded on this VPP instance.
func (d *RouteDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := readableTables(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, d.Wrap("retrieve", err)
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	seen := map[scheduler.Key]bool{}
	for id := range owned {
		stream, err := mpls.NewServiceClient(d.Client).MplsRouteDump(ctx, &mpls.MplsRouteDump{Table: mpls.MplsTable{MtTableID: id}})
		if err != nil {
			return nil, d.Wrap("mpls_route_dump", err)
		}
		dets, err := df7.Collect(stream.Recv)
		if err != nil {
			return nil, d.Wrap("mpls_route_dump", err)
		}
		for _, det := range dets {
			rt := det.MrRoute
			if rt.MrLabel < MinLabel {
				continue
			}
			r := Route{Table: id, Label: rt.MrLabel, EOS: rt.MrEos != 0, Multicast: rt.MrIsMulticast, Paths: df7.DecodePaths(rt.MrPaths, ifs)}
			if r.EOS {
				r.EOSProto = fmt.Sprintf("#%d", rt.MrEosProto)
				for n, v := range payloads {
					if v == rt.MrEosProto {
						r.EOSProto = n
					}
				}
			}
			k := KeyRoute(r.Table, r.Label, r.EOS)
			if id == SharedTable {
				ours, err := d.Recorded(ctx, string(k))
				if err != nil {
					return nil, err
				}
				if !ours {
					continue // another feature's label in the shared table (review H2)
				}
			}
			if seen[k] {
				continue // one entry per (label, eos) in a table; never report a key twice
			}
			seen[k] = true
			out = append(out, df7.KV(k, r, nil))
		}
	}
	sortKVs(out)
	return out, nil
}

// ---- mpls-ip-bind -----------------------------------------------------------------------------

// IPBindDescriptor manages mpls-ip-bind objects (mpls_ip_bind_unbind). Write-only (D-063):
// VPP 26.06 reports no bindings; their label entries appear in MPLS table 0 but cannot be told
// apart from mpls-route entries (mpls_route_details carries no FIB source). Bind and unbind are
// idempotent (VPP sources the prefix once and replaces the label).
type IPBindDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*IPBindDescriptor)(nil)

// NewIPBind returns the mpls-ip-bind descriptor.
func NewIPBind(c vpp.Client, owner string, opts ...df7.Option) *IPBindDescriptor {
	return &IPBindDescriptor{df7.NewBase(NameIPBind, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *IPBindDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	b, _ := df7.Decode[IPBind](obj)
	return KeyIPBind(b)
}

// Dependencies implements scheduler.Descriptor: the MPLS table and the VRF — a non-default one:
// the default IP table 0 always exists and no object provides "vrf/0" (F-mpls-srmpls gap: a
// binding in the default VRF could never be planned).
func (d *IPBindDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	b, _ := df7.Decode[IPBind](obj)
	deps := []scheduler.Dependency{{Key: KeyTable(b.MPLSTable)}}
	if b.VRF != 0 {
		deps = append(deps, scheduler.Dependency{Key: df7.VRFKey(b.VRF)})
	}
	return deps
}

func (d *IPBindDescriptor) bind(ctx context.Context, b IPBind, bind bool) error {
	p, err := df7.ParsePrefix(b.Prefix)
	if err != nil {
		return err
	}
	_, err = mpls.NewServiceClient(d.Client).MplsIPBindUnbind(ctx, &mpls.MplsIPBindUnbind{MbMplsTableID: b.MPLSTable, MbLabel: b.Label,
		MbIPTableID: b.VRF, MbIsBind: bind, MbPrefix: df7.ToPrefix(p)})
	return d.Wrap(fmt.Sprintf("mpls_ip_bind_unbind %s bind=%v", KeyIPBind(b), bind), err)
}

// Create implements scheduler.Descriptor (idempotent).
func (d *IPBindDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	b, err := df7.DecodeValid[IPBind](obj)
	if err != nil {
		return nil, err
	}
	return nil, d.bind(ctx, b, true)
}

// Update implements scheduler.Descriptor: everything is in the key.
func (*IPBindDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: unbind (VPP ignores an unbind whose label does not
// match the binding). A missing table is success.
func (d *IPBindDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	b, err := df7.Decode[IPBind](obj)
	if err != nil {
		return err
	}
	if err := d.bind(ctx, b, false); err != nil && !df7.IsVPPError(err, api.NO_SUCH_FIB) {
		return err
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *IPBindDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameIPBind, "VPP 26.06 has no dump of MPLS/IP label bindings")
}

// Register constructs every mpls descriptor (tables before interfaces, routes, bindings and
// tunnels) for the globals owner.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	RegisterFor(r, c, owner, true, opts...)
}

// RegisterFor is Register for an agent in the given D-071 role: only the globals owner creates
// and deletes MPLS table 0; any other agent only requires it (NewTableFor, F-mpls-srmpls).
func RegisterFor(r scheduler.Registry, c vpp.Client, owner string, globalsOwner bool, opts ...df7.Option) {
	r.Register(NewTableFor(c, owner, globalsOwner, opts...))
	r.Register(NewInterface(c, owner, opts...))
	r.Register(NewRoute(c, owner, opts...))
	r.Register(NewIPBind(c, owner, opts...))
	r.Register(NewTunnel(c, owner, opts...))
}
