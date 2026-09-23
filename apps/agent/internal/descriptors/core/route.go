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

// ErrRouteConflict: the prefix already has a FIB entry in that table that this owner did not
// create (D-071: never overwrite, never claim).
var ErrRouteConflict = errors.New("core: route prefix already present in the FIB and not owned by this agent")

// lookup returns the dumped FIB entry of exactly v's prefix in v's table, if any.
func (d *RouteDescriptor) lookup(ctx context.Context, v *Route) (*ip.IPRoute, error) {
	dst, err := netip.ParsePrefix(v.GetPrefix())
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", ErrBadPrefix, v.GetPrefix(), err)
	}
	stream, err := ip.NewServiceClient(d.Client).IPRouteDump(ctx, &ip.IPRouteDump{Table: ip.IPTable{TableID: v.GetTableId(), IsIP6: dst.Addr().Is6()}})
	if err != nil {
		return nil, fmt.Errorf("ip_route_dump %d: %w", v.GetTableId(), err)
	}
	var found *ip.IPRoute
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return found, nil
		}
		if err != nil {
			if isNoSuchTable(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("ip_route_dump %d: %w", v.GetTableId(), err)
		}
		if p, err := CanonNetPrefix(det.Route.Prefix.String()); err == nil && p == dst.Masked().String() {
			r := det.Route
			found = &r
		}
	}
}

// defaultDrop reports VPP's own per-table default entry (0.0.0.0/0 or ::/0 with only drop paths),
// which a configured default route legitimately overrides.
func defaultDrop(r *ip.IPRoute) bool {
	if r.Prefix.Len != 0 {
		return false
	}
	for _, p := range r.Paths {
		if p.Type != fib_types.FIB_API_PATH_TYPE_DROP {
			return false
		}
	}
	return true
}

// Create implements scheduler.Descriptor. Claim rule (D-071): a route not yet in the owner table is
// claimed only when the FIB has no entry for the prefix in that table (VPP's default-drop /0 aside);
// otherwise Create fails without sending anything and without recording ownership.
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
		if cur != nil && !defaultDrop(cur) {
			return nil, fmt.Errorf("%w: table %d %s", ErrRouteConflict, v.GetTableId(), v.GetPrefix())
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
	switch {
	case cur == nil:
		// already gone
	case defaultDrop(cur):
		// our own blackhole default route looks like VPP's default entry: remove our (API) source;
		// VPP keeps its default-route source either way
		_ = d.addDel(ctx, v, false)
	default:
		if err := d.addDel(ctx, v, false); err != nil {
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
	svc := ip.NewServiceClient(d.Client)
	var out []scheduler.KV
	for _, f := range fams {
		stream, err := svc.IPRouteDump(ctx, &ip.IPRouteDump{Table: ip.IPTable{TableID: f.table, IsIP6: f.v6}})
		if err != nil {
			return nil, fmt.Errorf("ip_route_dump %d: %w", f.table, err)
		}
		for {
			det, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				// a table that does not exist (any more) has no routes of ours
				if isNoSuchTable(err) {
					break
				}
				return nil, fmt.Errorf("ip_route_dump %d: %w", f.table, err)
			}
			p, err := CanonNetPrefix(det.Route.Prefix.String())
			if err != nil || !want[f][p] {
				continue
			}
			v := decodeRoute(det.Route, p, ifs)
			if v == nil {
				continue
			}
			out = append(out, scheduler.KV{Key: RouteKey(f.table, p), Value: v})
		}
	}
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
