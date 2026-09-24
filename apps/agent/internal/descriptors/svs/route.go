package svs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/svs"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// RouteDescriptor manages the source prefix → VRF entries of svs tables (svs_route_add_del). VPP's add is not
// idempotent (a second add only bumps the entry's source reference count and keeps the old lookup table), so Create
// adds only an entry that is not there and replaces one that is; the selected table is recorded per VPP boot identity
// (D-076/D-080) because the dump does not carry it.
type RouteDescriptor struct{ Env }

var _ scheduler.Descriptor = (*RouteDescriptor)(nil)

// Name implements scheduler.Descriptor.
func (*RouteDescriptor) Name() string { return RouteName }

func asRoute(obj proto.Message) *Route {
	if v, ok := obj.(*Route); ok && v != nil {
		return v
	}
	return &Route{}
}

// KeyOf implements scheduler.Descriptor.
func (*RouteDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	v := asRoute(obj)
	return RouteKey(v.GetTableId(), v.GetPrefix())
}

// Dependencies implements scheduler.Descriptor: the svs table and the selected VRF's table (unless table 0).
func (*RouteDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v := asRoute(obj)
	deps := []scheduler.Dependency{{Key: TableKey(v.GetTableId())}}
	if t := v.GetSourceTableId(); t != 0 && t != UnknownTable {
		deps = append(deps, scheduler.Dependency{Key: dfkit.DefaultVRFKey(strconv.FormatUint(uint64(t), 10))})
	}
	return deps
}

func (d *RouteDescriptor) send(ctx context.Context, v *Route, add bool) error {
	p, err := canonPrefix(v.GetPrefix())
	if err != nil {
		return err
	}
	pfx, err := ip_types.ParsePrefix(p.String())
	if err != nil {
		return fmt.Errorf("%w: prefix %s: %v", ErrBadValue, p, err)
	}
	req := &svs.SvsRouteAddDel{IsAdd: add, Prefix: pfx, TableID: v.GetTableId()}
	if add {
		req.SourceTableID = v.GetSourceTableId()
	}
	if _, err := svs.NewServiceClient(d.Client).SvsRouteAddDel(ctx, req); err != nil {
		return fmt.Errorf("svs_route_add_del table %d %s add=%v: %w", v.GetTableId(), p, add, err)
	}
	return nil
}

// present reports whether v's prefix has an svs entry in v's table.
func (d *RouteDescriptor) present(ctx context.Context, v *Route) (bool, error) {
	p, err := canonPrefix(v.GetPrefix())
	if err != nil {
		return false, err
	}
	src, ok, err := SourceID(ctx, d.Client, fibSourceName)
	if err != nil || !ok {
		return false, err
	}
	entries, err := dumpSvs(ctx, d, v.GetTableId(), p.Addr().Is6(), src)
	if err != nil {
		return false, err
	}
	_, found := entries[p.String()]
	return found, nil
}

// record is the applied-once record of v (value = the selected table).
func (d *RouteDescriptor) record(ctx context.Context, v *Route) error {
	id, err := d.identity(ctx)
	if err != nil {
		return err
	}
	return d.Boot.Put(dfkit.BootRecord{Key: string(d.KeyOf(v)), Identity: id.String(), Value: strconv.FormatUint(uint64(v.GetSourceTableId()), 10)})
}

// Create implements scheduler.Descriptor: an entry already present (ours: it is in our table) is removed first,
// so the add always programs v's table (VPP keeps the first DPO of a repeated add).
func (d *RouteDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*Route)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	if v.GetSourceTableId() == UnknownTable {
		return nil, fmt.Errorf("%w: source table unknown", ErrBadValue)
	}
	have, err := d.present(ctx, v)
	if err != nil {
		return nil, err
	}
	if have {
		if err := d.send(ctx, v, false); err != nil {
			return nil, err
		}
	}
	if err := d.send(ctx, v, true); err != nil {
		return nil, err
	}
	if err := d.record(ctx, v); err != nil {
		_ = d.send(context.WithoutCancel(ctx), v, false)
		return nil, fmt.Errorf("svs route record: %w", err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: another selected table needs delete + add.
func (*RouteDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: removes the entry when it is still there, then the record.
func (d *RouteDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v := asRoute(obj)
	have, err := d.present(ctx, v)
	if err != nil {
		return err
	}
	if have {
		if err := d.send(ctx, v, false); err != nil {
			return err
		}
	}
	return d.Boot.Delete(string(d.KeyOf(v)))
}

// Retrieve implements scheduler.Descriptor: the svs entries (except 0/0, the interface's own table added by
// svs_enable_disable) of every table of ours; the selected table from the record of the running VPP instance.
func (d *RouteDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	owned, err := ownedTables(ctx, d.Client, d.Owner)
	if err != nil || len(owned) == 0 {
		return nil, err
	}
	src, ok, err := SourceID(ctx, d.Client, fibSourceName)
	if err != nil || !ok {
		return nil, err // no svs plugin: nothing of ours can exist
	}
	var cur bootid.Identity
	haveID := false
	if id, err := d.identity(ctx); err == nil {
		cur, haveID = id, true
	}
	var out []scheduler.KV
	for id, fams := range owned {
		for _, v6 := range []bool{false, true} {
			if !fams[v6] {
				continue
			}
			entries, err := dumpSvs(ctx, d, id, v6, src)
			if err != nil {
				return nil, err
			}
			for p := range entries {
				v := &Route{TableId: id, Prefix: p, SourceTableId: UnknownTable}
				if r, ok := d.Boot.Get(string(RouteKey(id, p))); ok && haveID && bootid.Matches(r.Identity, cur) {
					if t, err := strconv.ParseUint(r.Value, 10, 32); err == nil {
						v.SourceTableId = uint32(t)
					}
				}
				out = append(out, scheduler.KV{Key: RouteKey(id, p), Value: v})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// dumpSvs returns the prefixes (canonical, not /0) of the entries whose best source is src in one table/family.
func dumpSvs(ctx context.Context, d *RouteDescriptor, table uint32, v6 bool, src uint8) (map[string]bool, error) {
	stream, err := ip.NewServiceClient(d.Client).IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Src: src, Table: ip.IPTable{TableID: table, IsIP6: v6}})
	if err != nil {
		return nil, fmt.Errorf("ip_route_v2_dump %d: %w", table, err)
	}
	out := map[string]bool{}
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ip_route_v2_dump %d: %w", table, err)
		}
		if det.Route.Src != src {
			continue // defensive: the dump filter already keeps only entries whose best source is src
		}
		p, err := netip.ParsePrefix(det.Route.Prefix.String())
		if err != nil || p.Bits() == 0 {
			continue
		}
		out[p.Masked().String()] = true
	}
}
