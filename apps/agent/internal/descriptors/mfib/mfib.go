// Package mfib reconciles IPv4 static multicast routes, never adopting foreign API entries.
package mfib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/mfib_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Name identifies the static multicast route descriptor.
const Name = "mfib.route"

// Path selects an accept or forwarding interface.
type Path struct {
	Interface string `json:"interface"`
	Flags     string `json:"flags"`
}

// Route is one IPv4 static source/group entry.
type Route struct {
	Table  uint32 `json:"table"`
	Group  string `json:"group"`
	Source string `json:"source,omitempty"`
	Paths  []Path `json:"paths"`
}

// Key identifies a route by table, group and source.
func Key(r Route) scheduler.Key {
	source := r.Source
	if source == "" {
		source = "*"
	}
	return scheduler.Join(Name, strconv.FormatUint(uint64(r.Table), 10), r.Group, source)
}

// Validate rejects unsupported groups, sources and path flags.
func (r Route) Validate() error {
	g, e := netip.ParseAddr(r.Group)
	if e != nil || !g.Is4() || !g.IsMulticast() || g.String() != r.Group || netip.MustParsePrefix("224.0.0.0/24").Contains(g) {
		return fmt.Errorf("invalid multicast group %q", r.Group)
	}
	if r.Source != "" {
		s, e := netip.ParseAddr(r.Source)
		if e != nil || !s.Is4() || s.IsMulticast() || s.IsUnspecified() || s.String() != r.Source {
			return fmt.Errorf("invalid multicast source %q", r.Source)
		}
	}
	if len(r.Paths) == 0 || len(r.Paths) > 255 {
		return fmt.Errorf("multicast route needs 1–255 paths")
	}
	accept := 0
	seen := map[string]bool{}
	for _, p := range r.Paths {
		if p.Interface == "" || seen[p.Interface] {
			return fmt.Errorf("duplicate or empty multicast interface")
		}
		seen[p.Interface] = true
		switch p.Flags {
		case "accept":
			accept++
		case "forward":
		default:
			return fmt.Errorf("invalid multicast path flag %q", p.Flags)
		}
	}
	if accept > 1 {
		return fmt.Errorf("multicast route accepts at most one interface")
	}
	return nil
}

// Descriptor reconciles routes guarded by persistent boot ownership.
type Descriptor struct {
	df7.Base
	Store dfkit.BootStore
}

// CheckPersistent requires a durable ownership record store.
func (d *Descriptor) CheckPersistent() error { return dfkit.CheckBoot(Name, d.Store) }

// New constructs a static mFIB descriptor with explicit ownership storage.
func New(c vpp.Client, owner string, store dfkit.BootStore, opts ...df7.Option) *Descriptor {
	return &Descriptor{df7.NewBase(Name, c, owner, opts), store}
}

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(m proto.Message) scheduler.Key { r, _ := df7.Decode[Route](m); return Key(r) }

// Dependencies orders routes after their VRF and interfaces.
func (d *Descriptor) Dependencies(m proto.Message) []scheduler.Dependency {
	r, _ := df7.Decode[Route](m)
	var out []scheduler.Dependency
	if r.Table != 0 {
		out = append(out, scheduler.Dependency{Key: df7.VRFKey(r.Table)})
	}
	for _, p := range r.Paths {
		out = append(out, d.Opts.IfaceDep(p.Interface))
	}
	return out
}

// tables only includes owned named mFIB tables. Table zero is allowed only when the configured ID range owns it.
func (d *Descriptor) tables(ctx context.Context) (map[uint32]bool, error) {
	s, e := ip.NewServiceClient(d.Client).IPMtableDump(ctx, &ip.IPMtableDump{})
	if e != nil {
		return nil, e
	}
	ts, e := df7.Collect(s.Recv)
	if e != nil {
		return nil, e
	}
	out := map[uint32]bool{}
	for _, t := range ts {
		if !t.Table.IsIP6 && d.Opts.IDs.Owns(t.Table.TableID) && (strings.HasPrefix(strings.TrimRight(t.Table.Name, "\x00"), d.Owner+":") || t.Table.TableID == 0) {
			out[t.Table.TableID] = true
		}
	}
	return out, nil
}
func (d *Descriptor) records(ctx context.Context) (string, error) {
	if d.Store == nil {
		return "", fmt.Errorf("mfib requires persistent ownership store")
	}
	id, e := dfkit.IdentitySource(ctx, d.Client)
	if e != nil {
		return "", e
	}
	return id.String(), nil
}
func (d *Descriptor) dump(ctx context.Context, table uint32) ([]ip.IPMroute, error) {
	s, e := ip.NewServiceClient(d.Client).IPMrouteDump(ctx, &ip.IPMrouteDump{Table: ip.IPTable{TableID: table}})
	if e != nil {
		return nil, e
	}
	rs, e := df7.Collect(s.Recv)
	if e != nil {
		return nil, e
	}
	var out []ip.IPMroute
	for _, r := range rs {
		out = append(out, r.Route)
	}
	return out, nil
}
func rawKey(r ip.IPMroute) (scheduler.Key, bool) {
	if r.Prefix.Af != ip_types.ADDRESS_IP4 || r.Prefix.GrpAddressLength != 32 {
		return "", false
	}
	group := netip.AddrFrom4(r.Prefix.GrpAddress.GetIP4()).String()
	src := r.Prefix.SrcAddress.GetIP4()
	source := ""
	if src != [4]byte{} {
		source = netip.AddrFrom4(src).String()
	}
	return Key(Route{Table: r.TableID, Group: group, Source: source}), true
}
func routeOf(r ip.IPMroute, ifs *df7.Interfaces) (Route, bool) {
	if r.Prefix.Af != ip_types.ADDRESS_IP4 || r.Prefix.GrpAddressLength != 32 {
		return Route{}, false
	}
	g := r.Prefix.GrpAddress.GetIP4()
	src := r.Prefix.SrcAddress.GetIP4()
	v := Route{Table: r.TableID, Group: netip.AddrFrom4(g).String()}
	if src != [4]byte{} {
		v.Source = netip.AddrFrom4(src).String()
	}
	for _, p := range r.Paths {
		name, ok := ifs.Logical(p.Path.SwIfIndex)
		if !ok {
			return Route{}, false
		}
		flag := ""
		switch p.ItfFlags {
		case mfib_types.MFIB_API_ITF_FLAG_ACCEPT:
			flag = "accept"
		case mfib_types.MFIB_API_ITF_FLAG_FORWARD:
			flag = "forward"
		default:
			return Route{}, false
		}
		v.Paths = append(v.Paths, Path{name, flag})
	}
	sort.Slice(v.Paths, func(i, j int) bool { return v.Paths[i].Interface < v.Paths[j].Interface })
	return v, v.Validate() == nil
}
func (d *Descriptor) encode(ctx context.Context, r Route) (ip.IPMroute, error) {
	ifs, e := d.Ifaces(ctx)
	if e != nil {
		return ip.IPMroute{}, e
	}
	g := netip.MustParseAddr(r.Group)
	out := ip.IPMroute{TableID: r.Table, RpfID: ^uint32(0), Prefix: ip_types.Mprefix{Af: ip_types.ADDRESS_IP4, GrpAddressLength: 32, GrpAddress: ip_types.AddressUnionIP4(g.As4())}}
	if r.Source != "" {
		out.Prefix.SrcAddress = ip_types.AddressUnionIP4(netip.MustParseAddr(r.Source).As4())
	}
	for _, p := range r.Paths {
		idx, e := ifs.Resolve(p.Interface)
		if e != nil {
			return out, e
		}
		f := mfib_types.MFIB_API_ITF_FLAG_FORWARD
		if p.Flags == "accept" {
			f = mfib_types.MFIB_API_ITF_FLAG_ACCEPT
		}
		out.Paths = append(out.Paths, mfib_types.MfibPath{ItfFlags: f, Path: fib_types.FibPath{SwIfIndex: idx, Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4, Weight: 1}})
	}
	if len(out.Paths) > 255 {
		return out, fmt.Errorf("too many multicast paths")
	}
	for range out.Paths {
		out.NPaths++
	}
	return out, nil
}

// Create refuses foreign prefixes and records ownership before the VPP write.
func (d *Descriptor) Create(ctx context.Context, m proto.Message) (any, error) {
	r, e := df7.DecodeValid[Route](m)
	if e != nil {
		return nil, e
	}
	if e = d.Opts.CheckID("multicast table", r.Table); e != nil {
		return nil, e
	}
	tables, e := d.tables(ctx)
	if e != nil {
		return nil, e
	}
	if !tables[r.Table] {
		return nil, fmt.Errorf("multicast table %d is absent or foreign", r.Table)
	}
	boot, e := d.records(ctx)
	if e != nil {
		return nil, e
	}
	have, e := d.dump(ctx, r.Table)
	if e != nil {
		return nil, e
	}
	rec, claimed := d.Store.Get(string(Key(r)))
	for _, raw := range have {
		key, ok := rawKey(raw)
		if ok && key == Key(r) && (!claimed || rec.Identity != boot) {
			return nil, fmt.Errorf("foreign multicast route %s", Key(r))
		}
	}
	encoded, e := d.encode(ctx, r)
	if e != nil {
		return nil, e
	}
	value, e := json.Marshal(r)
	if e != nil {
		return nil, e
	}
	if e = d.Store.Put(dfkit.BootRecord{Key: string(Key(r)), Identity: boot, Value: string(value)}); e != nil {
		return nil, e
	}
	_, e = ip.NewServiceClient(d.Client).IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{IsAdd: true, Route: encoded})
	if e != nil {
		var release error
		if claimed {
			release = d.Store.Put(rec)
		} else {
			release = d.Store.Delete(string(Key(r)))
		}
		if release != nil {
			return nil, fmt.Errorf("%w; ownership restore: %v", e, release)
		}
	}
	return nil, e
}

// Update requests delete/create to replace the entire path set.
func (*Descriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete rechecks ownership, table and prefix existence before removing a route.
func (d *Descriptor) Delete(ctx context.Context, m proto.Message, _ any) error {
	r, e := df7.DecodeValid[Route](m)
	if e != nil {
		return e
	}
	boot, e := d.records(ctx)
	if e != nil {
		return e
	}
	rec, ok := d.Store.Get(string(Key(r)))
	if !ok || rec.Identity != boot {
		return nil
	}
	tables, e := d.tables(ctx)
	if e != nil {
		return e
	}
	if !tables[r.Table] {
		return d.Store.Delete(string(Key(r)))
	}
	raws, e := d.dump(ctx, r.Table)
	if e != nil {
		return e
	}
	for _, raw := range raws {
		key, ok := rawKey(raw)
		if ok && key == Key(r) {
			_, e = ip.NewServiceClient(d.Client).IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{Route: raw})
			if e != nil {
				return e
			}
			break
		}
	}
	return d.Store.Delete(string(Key(r)))
}

// Retrieve returns only owned routes on the current VPP boot.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ts, e := d.tables(ctx)
	if e != nil {
		return nil, e
	}
	ifs, e := d.Ifaces(ctx)
	if e != nil {
		return nil, e
	}
	var out []scheduler.KV
	for table := range ts {
		rs, e := d.dump(ctx, table)
		if e != nil {
			return nil, e
		}
		for _, raw := range rs {
			r, ok := routeOf(raw, ifs)
			if !ok {
				continue
			}
			rec, ok := d.Store.Get(string(Key(r)))
			if ok {
				boot, e := d.records(ctx)
				if e != nil {
					return nil, e
				}
				if rec.Identity != boot {
					continue
				}
				out = append(out, df7.KV(Key(r), r, nil))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
