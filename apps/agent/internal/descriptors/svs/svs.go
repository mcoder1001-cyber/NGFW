// Package svs holds the source-VRF-select descriptors of F-vrf-static-ecmp (VPP svs plugin; WBS D2.1):
//
//	descriptor      key                                 VPP messages (binapi)
//	svs.table       svs.table/<id>                      ip_table_add_del (IPv4 + IPv6, named "<owner>:svs:<id>"), ip_table_dump
//	svs.route       svs.route/<table id>/<prefix>       svs_route_add_del, ip_route_v2_dump (the "svs" FIB source), fib_source_dump
//	svs.interface   svs.interface/<interface>           svs_enable_disable (IPv4 + IPv6), svs_dump
//
// Configuration: `vrfs.<vrf>.sourceSelect[]{prefix, interface}` — a packet arriving on `interface` with a source
// address in `prefix` is routed in `<vrf>`. VPP models that as one svs table per ingress interface (svs_enable_disable
// binds the table to the interface and adds 0/0 → "the interface's own table"); each entry is a route of that table
// whose DPO is a destination lookup in the selected VRF's table. internal/desired/vrf_static_ecmp.go projects the
// configuration onto these objects; the svs table id of an interface comes from Allocate (docs/agent/descriptors/svs.md).
//
// Ownership: svs tables are named "<owner>:svs:<id>" (the VRF descriptor skips names with ":"); routes and enablements
// are ours when their table is. Readback (D-063/D-076/D-080): tables from ip_table_dump, enablements from svs_dump,
// routes from ip_route_v2_dump of our tables (entries of the "svs" source). VPP does not expose which table an svs
// route selects (an exclusive lookup DPO, dumped as a bare path): that one attribute comes from an applied-once record
// keyed by the VPP boot identity in the persisted BootStore (never from desired state); without a record for the running
// VPP it is reported as UnknownTable and the diff re-programs the entry.
//
// svs_table_add_del is deliberately not used: its lock is counted per add, so a repeated add leaks and an unbalanced
// delete underflows VPP's per-source lock count; the table exists for svs_route_add_del/svs_enable_disable as soon as
// ip_table_add_del created it (both only fib_table_find it), and ip_table_add_del is idempotent (one API lock).
package svs

//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative svs_model.proto

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"ngfw/agent/binapi/fib"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Descriptor names.
const (
	TableName     = "svs.table"
	RouteName     = "svs.route"
	InterfaceName = "svs.interface"
)

// UnknownTable is the source_table_id Retrieve reports for an svs route whose selected table is not
// known from an applied-once record of the running VPP (it is not in VPP's dump).
const UnknownTable = ^uint32(0)

// fibSourceName is the name the svs plugin allocates its FIB source under (fib_source_allocate("svs")).
const fibSourceName = "svs"

// Errors.
var (
	ErrBadValue = errors.New("svs: unexpected value")
	// ErrTableConflict means the table id exists in VPP under a name that is not this owner's svs table
	// (D-071: foreign → never touched).
	ErrTableConflict = errors.New("svs: table id is in use by another owner or VRF")
	// ErrInterfaceConflict means source VRF select is already enabled on the interface with another table.
	ErrInterfaceConflict = errors.New("svs: source VRF select already enabled on the interface with another table")
)

// Env is what the svs descriptors need.
type Env struct {
	Client vpp.Client
	Owner  string
	// Boot is the persisted D-076 applied-once store (subsystems.Wiring.BootStore); nil = in-memory (tests).
	Boot dfkit.BootStore
	// IfRef maps an interface name to the key svs.interface depends on (nil = the alias "interface/<name>").
	IfRef func(name string) scheduler.Key
	// IfTableRef is the optional ordering dependency on the interface's VRF binding (nil = none):
	// core.InterfaceTableKey, so the binding exists before svs is enabled and a rebind re-enables it
	// (svs_enable_disable reads the interface's table once, when it is enabled).
	IfTableRef func(name string) scheduler.Key
	// Identity is the D-080 VPP boot identity (nil = dfkit.IdentitySource).
	Identity func(ctx context.Context, c vpp.Client) (bootid.Identity, error)
}

func (e Env) ifRef(name string) scheduler.Key {
	if e.IfRef != nil {
		return e.IfRef(name)
	}
	return dfkit.DefaultInterfaceKey(name)
}

func (e Env) identity(ctx context.Context) (bootid.Identity, error) {
	if e.Identity != nil {
		return e.Identity(ctx, e.Client)
	}
	return dfkit.IdentitySource(ctx, e.Client)
}

// Register registers the svs descriptors in dependency-friendly order: table, interface, route.
func Register(r scheduler.Registry, env Env) {
	if env.Boot == nil {
		env.Boot = dfkit.NewMemoryBootStore()
	}
	r.Register(&TableDescriptor{env})
	r.Register(&InterfaceDescriptor{env})
	r.Register(&RouteDescriptor{env})
}

// Names returns the svs descriptor names in registration order.
func Names() []string { return []string{TableName, InterfaceName, RouteName} }

// TableKey is "svs.table/<id>".
func TableKey(id uint32) scheduler.Key {
	return scheduler.Join(TableName, strconv.FormatUint(uint64(id), 10))
}

// RouteKey is "svs.route/<table>/<prefix>".
func RouteKey(table uint32, prefix string) scheduler.Key {
	return scheduler.Join(RouteName, strconv.FormatUint(uint64(table), 10), prefix)
}

// InterfaceKey is "svs.interface/<interface>".
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceName, name) }

// VPPTableName is the VPP name of an owned svs table: "<owner>:svs:<id>".
func VPPTableName(owner string, id uint32) (string, error) {
	return vpp.OwnerTag(owner, "svs:"+strconv.FormatUint(uint64(id), 10))
}

// parseTableName returns the id of an owned svs table name.
func parseTableName(name, owner string) (uint32, bool) {
	id, ok := vpp.ParseOwnerTag(strings.TrimRight(name, "\x00"), owner)
	if !ok || !strings.HasPrefix(id, "svs:") {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, "svs:"), 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint32(n), true
}

// ---------------------------------------------------------------------------------------------- allocation

// Range is the closed range svs table ids are allocated from.
type Range struct{ Lo, Hi uint32 }

// DefaultRange is the product agent's range: the 255 table ids just below 2^32-1 (VRFs are numbered by the operator
// from the bottom; a declared VRF id inside the range is skipped, never reused).
var DefaultRange = Range{Lo: 0xFFFFFF00, Hi: 0xFFFFFFFE}

// RangeIn is the svs range inside a slot's table range lo..hi (shared-host rules §1): its top 100 ids (all of them for
// a smaller range).
func RangeIn(lo, hi uint32) Range {
	if hi-lo < 100 {
		return Range{Lo: lo, Hi: hi}
	}
	return Range{Lo: hi - 99, Hi: hi}
}

// ErrRangeFull means more interfaces need an svs table than the range has free ids.
var ErrRangeFull = errors.New("svs: no free table id left in the source-VRF-select range")

// Allocate maps each interface to its own svs table id in r, skipping ids for which taken is true (declared VRFs).
// Deterministic for a given set: interfaces in name order each take the first free id probing downward (with
// wrap-around) from Hi - fnv32a(name) mod size, so an interface keeps its id when others come and go (unless two
// hash to the same id).
func Allocate(ifaces []string, r Range, taken func(uint32) bool) (map[string]uint32, error) {
	if r.Hi < r.Lo || r.Lo == 0 {
		return nil, fmt.Errorf("%w: bad range %d-%d", ErrBadValue, r.Lo, r.Hi)
	}
	names := append([]string(nil), ifaces...)
	sort.Strings(names)
	size := uint64(r.Hi) - uint64(r.Lo) + 1
	used := map[uint32]bool{}
	out := map[string]uint32{}
	for _, n := range names {
		if _, dup := out[n]; dup {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(n))
		off := uint64(h.Sum32()) % size
		found := false
		for i := uint64(0); i < size; i++ {
			id := uint32(uint64(r.Hi) - (off+i)%size) //nolint:gosec // within [Lo, Hi]
			if used[id] || (taken != nil && taken(id)) {
				continue
			}
			used[id], out[n], found = true, id, true
			break
		}
		if !found {
			return nil, fmt.Errorf("%w (%d-%d)", ErrRangeFull, r.Lo, r.Hi)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------------------------- shared VPP reads

// ownedTables dumps ip_table_dump and returns this owner's svs tables: id → families present.
func ownedTables(ctx context.Context, c vpp.Client, owner string) (map[uint32]map[bool]bool, error) {
	all, err := tableNames(ctx, c)
	if err != nil {
		return nil, err
	}
	out := map[uint32]map[bool]bool{}
	for k, name := range all {
		if id, ok := parseTableName(name, owner); ok && id == k.id {
			if out[id] == nil {
				out[id] = map[bool]bool{}
			}
			out[id][k.v6] = true
		}
	}
	return out, nil
}

type tableFam struct {
	id uint32
	v6 bool
}

// tableNames returns the VPP name of every FIB table (by id and family).
func tableNames(ctx context.Context, c vpp.Client) (map[tableFam]string, error) {
	stream, err := ip.NewServiceClient(c).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		return nil, fmt.Errorf("ip_table_dump: %w", err)
	}
	out := map[tableFam]string{}
	for {
		t, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ip_table_dump: %w", err)
		}
		out[tableFam{t.Table.TableID, t.Table.IsIP6}] = strings.TrimRight(t.Table.Name, "\x00")
	}
}

// SourceID returns the FIB source id VPP allocated to name (fib_source_dump); ok=false when VPP has no such source.
func SourceID(ctx context.Context, c vpp.Client, name string) (uint8, bool, error) {
	all, err := Sources(ctx, c)
	if err != nil {
		return 0, false, err
	}
	for id, n := range all {
		if n == name {
			return id, true, nil
		}
	}
	return 0, false, nil
}

// Sources returns every FIB source VPP knows (fib_source_dump): id → name.
func Sources(ctx context.Context, c vpp.Client) (map[uint8]string, error) {
	stream, err := fib.NewServiceClient(c).FibSourceDump(ctx, &fib.FibSourceDump{})
	if err != nil {
		return nil, fmt.Errorf("fib_source_dump: %w", err)
	}
	out := map[uint8]string{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("fib_source_dump: %w", err)
		}
		out[d.Src.ID] = strings.TrimRight(d.Src.Name, "\x00")
	}
}

func af(v6 bool) ip_types.AddressFamily {
	if v6 {
		return ip_types.ADDRESS_IP6
	}
	return ip_types.ADDRESS_IP4
}

// canonPrefix canonicalises a source prefix (host bits cleared) and refuses /0.
func canonPrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: prefix %q: %v", ErrBadValue, s, err)
	}
	p = netip.PrefixFrom(p.Addr().Unmap().WithZone(""), p.Bits()).Masked()
	if p.Bits() == 0 {
		return netip.Prefix{}, fmt.Errorf("%w: prefix %s: /0 is the interface's own table (svs.interface)", ErrBadValue, p)
	}
	return p, nil
}

// CanonPrefix is canonPrefix as a string (the projection and Retrieve use the same form).
func CanonPrefix(s string) (string, error) {
	p, err := canonPrefix(s)
	if err != nil {
		return "", err
	}
	return p.String(), nil
}
