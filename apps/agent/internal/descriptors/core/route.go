package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/scheduler"
)

// RouteDescriptor manages static routes (ip_route_add_del, API FIB source). Routes have no tag
// field in VPP; ownership is the owner table (Env.Owned): the key is recorded before the route is
// added and removed after it is deleted, and Retrieve reports recorded routes that exist in VPP.
type RouteDescriptor struct{ Env }

var _ scheduler.Descriptor = (*RouteDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*RouteDescriptor) Name() string { return RouteName }

func asRoute(obj proto.Message) *Route {
	v, _ := obj.(*Route)
	if v == nil {
		return &Route{}
	}
	return v
}

// KeyOf implements scheduler.Descriptor.
func (*RouteDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	v := asRoute(obj)
	return RouteKey(v.GetTableId(), v.GetPrefix())
}

// Dependencies implements scheduler.Descriptor: the VRF table (unless table 0) and, optionally,
// every egress interface (ordering only — physical interfaces are not objects of this agent).
func (d *RouteDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asRoute(obj)
	var deps []scheduler.Dependency
	if v.GetTableId() != 0 {
		deps = append(deps, scheduler.Dependency{Key: VRFKey(v.GetTableId())})
	}
	seen := map[string]bool{}
	for _, p := range v.GetPaths() {
		if p.GetInterface() != "" && !seen[p.GetInterface()] {
			seen[p.GetInterface()] = true
			deps = append(deps, scheduler.Dependency{Key: d.ifRef(p.GetInterface()), Optional: true})
		}
	}
	return deps
}

func (d *RouteDescriptor) encode(ctx context.Context, v *Route) (ip.IPRoute, error) {
	dst, err := netip.ParsePrefix(v.GetPrefix())
	if err != nil {
		return ip.IPRoute{}, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	pfx, err := ip_types.ParsePrefix(dst.String())
	if err != nil {
		return ip.IPRoute{}, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	proto := fib_types.FIB_API_PATH_NH_PROTO_IP4
	if dst.Addr().Is6() {
		proto = fib_types.FIB_API_PATH_NH_PROTO_IP6
	}
	pref := v.GetPreference()
	if pref > 255 {
		return ip.IPRoute{}, fmt.Errorf("%w: preference %d > 255", ErrBadValue, pref)
	}
	r := ip.IPRoute{TableID: v.GetTableId(), Prefix: pfx}
	if len(v.GetPaths()) == 0 {
		r.Paths = []fib_types.FibPath{{SwIfIndex: ^uint32(0), TableID: v.GetTableId(), Type: fib_types.FIB_API_PATH_TYPE_DROP, Proto: proto, Weight: 1, Preference: uint8(pref)}}
		r.NPaths = 1
		return r, nil
	}
	var ifs *ifTable
	for _, p := range v.GetPaths() {
		fp := fib_types.FibPath{SwIfIndex: ^uint32(0), TableID: v.GetTableId(), Type: fib_types.FIB_API_PATH_TYPE_NORMAL, Proto: proto, Preference: uint8(pref)}
		w := p.GetWeight()
		if w == 0 {
			w = 1
		}
		if w > 255 {
			return ip.IPRoute{}, fmt.Errorf("%w: weight %d > 255", ErrBadValue, w)
		}
		fp.Weight = uint8(w)
		if p.GetAddress() != "" {
			a, err := netip.ParseAddr(p.GetAddress())
			if err != nil {
				return ip.IPRoute{}, fmt.Errorf("core: invalid next hop %q: %v", p.GetAddress(), err)
			}
			if a.Is6() != dst.Addr().Is6() {
				return ip.IPRoute{}, fmt.Errorf("%w: next hop %s and prefix %s are of different families", ErrBadValue, a, dst)
			}
			if a.Is4() {
				fp.Nh.Address = ip_types.AddressUnionIP4(ip_types.IP4Address(a.As4()))
			} else {
				fp.Nh.Address = ip_types.AddressUnionIP6(ip_types.IP6Address(a.As16()))
			}
		}
		if p.GetInterface() != "" {
			if ifs == nil {
				if ifs, err = dumpInterfaces(ctx, d.Client, d.Owner); err != nil {
					return ip.IPRoute{}, err
				}
			}
			in, ok := ifs.any(p.GetInterface())
			if !ok {
				return ip.IPRoute{}, fmt.Errorf("core: egress interface %q not found", p.GetInterface())
			}
			fp.SwIfIndex = in.Index
		}
		r.Paths = append(r.Paths, fp)
	}
	if len(r.Paths) > 255 {
		return ip.IPRoute{}, fmt.Errorf("%w: %d paths > 255", ErrBadValue, len(r.Paths))
	}
	r.NPaths = uint8(len(r.Paths)) //nolint:gosec // bounded above
	return r, nil
}

func (d *RouteDescriptor) addDel(ctx context.Context, v *Route, add bool) error {
	r, err := d.encode(ctx, v)
	if err != nil {
		return err
	}
	if !add {
		r.Paths, r.NPaths = nil, 0
	}
	// is_multipath=false: add replaces the whole path set of the API source; delete removes it.
	if _, err := ip.NewServiceClient(d.Client).IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: add, Route: r}); err != nil {
		return fmt.Errorf("ip_route_add_del %d %s add=%v: %w", v.GetTableId(), v.GetPrefix(), add, err)
	}
	return nil
}

// ErrRouteConflict means the prefix already has a client-programmed FIB entry in that table that
// this owner did not create (D-071: never overwrite, never claim).
var ErrRouteConflict = errors.New("core: route prefix already present in the FIB and not owned by this agent")

// FIB sources as reported by ip_route_v2_details.src (fib_source_t, src/vnet/fib/fib_source.h in
// VPP 26.06; verified on the host: API=8, recursive-resolution=18, default-route=20, special=1).
// The value is the entry's BEST source; API (priority 0x80) outranks every VPP-generated source
// except special/classify/proxy/interface (review N1).
const (
	fibSrcSR      = 5
	fibSrc6RD     = 7
	fibSrcAPI     = 8
	fibSrcCLI     = 9
	fibSrcLISP    = 10
	fibSrcMAP     = 11
	fibSrcDHCP    = 12
	fibSrcLastFix = 21 // FIB_SOURCE_INTERPOSE; higher ids are allocated by plugins (lcp-rt, nat-hi, lb…)
)

// clientSource reports whether a best source means "someone programmed this route": API/CLI/DHCP and
// the other client sources, and every plugin-allocated source. VPP-generated sources (interface,
// adjacency, recursive-resolution, default-route, special, …) never block a claim: VPP stacks our API
// source next to them and nobody else's object is touched.
func clientSource(src uint8) bool {
	switch src {
	case fibSrcSR, fibSrc6RD, fibSrcAPI, fibSrcCLI, fibSrcLISP, fibSrcMAP, fibSrcDHCP:
		return true
	}
	return src > fibSrcLastFix
}

// mayHideAPI reports whether a best source outranks API, i.e. an API source (ours) may sit below it.
func mayHideAPI(src uint8) bool { return src != 0 && src < fibSrcAPI }

// dumpTable returns every FIB entry of one table/family with its best source, keyed by canonical
// prefix; a missing table yields an empty map.
func (d *RouteDescriptor) dumpTable(ctx context.Context, table uint32, v6 bool) (map[string]ip.IPRouteV2, error) {
	stream, err := ip.NewServiceClient(d.Client).IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Table: ip.IPTable{TableID: table, IsIP6: v6}})
	if err != nil {
		return nil, fmt.Errorf("ip_route_v2_dump %d: %w", table, err)
	}
	out := map[string]ip.IPRouteV2{}
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			if isNoSuchTable(err) {
				return out, nil
			}
			return nil, fmt.Errorf("ip_route_v2_dump %d: %w", table, err)
		}
		if p, err := CanonNetPrefix(det.Route.Prefix.String()); err == nil {
			out[p] = det.Route
		}
	}
}

// lookup returns the FIB entry of exactly v's prefix in v's table, if any.
func (d *RouteDescriptor) lookup(ctx context.Context, v *Route) (*ip.IPRouteV2, error) {
	dst, err := netip.ParsePrefix(v.GetPrefix())
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	all, err := d.dumpTable(ctx, v.GetTableId(), dst.Addr().Is6())
	if err != nil {
		return nil, err
	}
	if r, ok := all[dst.Masked().String()]; ok {
		return &r, nil
	}
	return nil, nil
}

// Create implements scheduler.Descriptor. Claim rule (D-071): a route not yet in the owner table is
// claimed only when the FIB has no client-programmed entry for the prefix in that table (entries VPP
// generates itself — connected, neighbour, recursive-resolution next hops, default-drop — never
// block); otherwise Create fails without sending anything and without recording ownership.
func (d *RouteDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*Route)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	key := string(d.KeyOf(v))
	existed := d.Owned.Has(key)
	if !existed {
		cur, err := d.lookup(ctx, v)
		if err != nil {
			return nil, err
		}
		if cur != nil && clientSource(cur.Src) {
			return nil, fmt.Errorf("%w: table %d %s (fib source %d)", ErrRouteConflict, v.GetTableId(), v.GetPrefix(), cur.Src)
		}
	}
	if err := d.Owned.Add(key); err != nil {
		return nil, fmt.Errorf("owner table: %w", err)
	}
	if err := d.addDel(ctx, v, true); err != nil {
		if !existed {
			_ = d.Owned.Remove(key)
		}
		return nil, err
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: replace the path set in place (only reached for routes
// Retrieve reported, i.e. claimed ones).
func (d *RouteDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	return meta, d.addDel(ctx, asRoute(newObj), true)
}

// Delete implements scheduler.Descriptor: only a route we still claim, and only when the FIB still
// has an entry for it (checked right before the delete, D-071/D-074); the claim is dropped after.
func (d *RouteDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v := asRoute(obj)
	key := string(d.KeyOf(v))
	if !d.Owned.Has(key) {
		return nil // not ours: never touched
	}
	cur, err := d.lookup(ctx, v)
	if err != nil {
		return err
	}
	// Our API source exists only if it is the best source or hidden below a higher-priority one;
	// entries whose best source is VPP-generated and below API carry no API source: nothing to delete.
	if cur != nil && (cur.Src == fibSrcAPI || mayHideAPI(cur.Src)) {
		if err := d.addDel(ctx, v, false); err != nil && cur.Src == fibSrcAPI {
			return err
		}
	}
	if err := d.Owned.Remove(key); err != nil {
		return fmt.Errorf("owner table: %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *RouteDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned := d.Owned.Keys(RouteName + scheduler.KeySeparator)
	if len(owned) == 0 {
		return nil, nil
	}
	type tf struct {
		table uint32
		v6    bool
	}
	want := map[tf]map[string]bool{}
	for _, k := range owned {
		key := scheduler.Key(k)
		tableStr, pfx, ok := strings.Cut(key.ID(), scheduler.KeySeparator)
		if !ok {
			continue
		}
		table, err := strconv.ParseUint(tableStr, 10, 32)
		if err != nil {
			continue
		}
		p, err := netip.ParsePrefix(pfx)
		if err != nil {
			continue
		}
		f := tf{uint32(table), p.Addr().Is6()}
		if want[f] == nil {
			want[f] = map[string]bool{}
		}
		want[f][p.String()] = true
	}
	ifs, err := dumpInterfaces(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, err
	}
	fams := make([]tf, 0, len(want))
	for f := range want {
		fams = append(fams, f)
	}
	sort.Slice(fams, func(i, j int) bool {
		if fams[i].table != fams[j].table {
			return fams[i].table < fams[j].table
		}
		return !fams[i].v6 && fams[j].v6
	})
	var out []scheduler.KV
	for _, f := range fams {
		all, err := d.dumpTable(ctx, f.table, f.v6)
		if err != nil {
			return nil, err
		}
		for p := range want[f] {
			e, ok := all[p]
			if !ok || e.Src != fibSrcAPI {
				continue // gone, or only VPP-generated/foreign-hidden state left: not our route
			}
			v := decodeRoute(ip.IPRoute{TableID: e.TableID, Prefix: e.Prefix, NPaths: e.NPaths, Paths: e.Paths}, p, ifs)
			if v == nil {
				continue
			}
			out = append(out, scheduler.KV{Key: RouteKey(f.table, p), Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// decodeRoute converts a dumped route into the canonical value; nil when the entry carries no
// path we could have installed (e.g. only attached/connected paths).
func decodeRoute(r ip.IPRoute, prefix string, ifs *ifTable) *Route {
	v := &Route{TableId: r.TableID, Prefix: prefix}
	var paths []*RoutePath
	drop := false
	for _, fp := range r.Paths {
		if fp.Preference != 0 {
			v.Preference = uint32(fp.Preference)
		}
		switch fp.Type {
		case fib_types.FIB_API_PATH_TYPE_DROP:
			drop = true
			continue
		case fib_types.FIB_API_PATH_TYPE_NORMAL:
		default:
			continue
		}
		p := &RoutePath{Weight: uint32(fp.Weight)}
		if p.Weight == 0 {
			p.Weight = 1
		}
		var a netip.Addr
		switch fp.Proto {
		case fib_types.FIB_API_PATH_NH_PROTO_IP4:
			a = netip.AddrFrom4(fp.Nh.Address.GetIP4())
		case fib_types.FIB_API_PATH_NH_PROTO_IP6:
			a = netip.AddrFrom16(fp.Nh.Address.GetIP6())
		}
		if a.IsValid() && !a.IsUnspecified() {
			p.Address = a.String()
		}
		if fp.SwIfIndex != ^uint32(0) {
			if n, ok := ifs.nameOf(fp.SwIfIndex); ok {
				p.Interface = n
			}
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 && !drop {
		return nil
	}
	SortPaths(paths)
	v.Paths = paths
	return v
}

// SortPaths sorts paths canonically by (address, interface, weight).
func SortPaths(paths []*RoutePath) {
	sort.Slice(paths, func(i, j int) bool {
		a, b := paths[i], paths[j]
		if a.GetAddress() != b.GetAddress() {
			return addrLess(a.GetAddress(), b.GetAddress())
		}
		if a.GetInterface() != b.GetInterface() {
			return a.GetInterface() < b.GetInterface()
		}
		return a.GetWeight() < b.GetWeight()
	})
}

func addrLess(a, b string) bool {
	x, e1 := netip.ParseAddr(a)
	y, e2 := netip.ParseAddr(b)
	if e1 != nil || e2 != nil {
		return a < b
	}
	return x.Less(y)
}

func isNoSuchTable(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no such fib") || strings.Contains(err.Error(), "NO_SUCH_FIB")
}
