package l3xc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	l3xcapi "ngfw/agent/binapi/l3xc"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor and dependency names.
const (
	L3xcName = "l3xc.l3xc"
	// TableDescriptor is P05 core's FIB-table descriptor name: "vrf", keys "vrf/<id>"
	// (task/P05 core.VRFName / core.VRFKey; DF-1-questions.md Q1, resolved).
	TableDescriptor = "vrf"
)

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("l3xc: nil or wrong desired value type")

// TableKey is the dependency key of FIB table id.
func TableKey(id uint32) scheduler.Key {
	return scheduler.Join(TableDescriptor, strconv.FormatUint(uint64(id), 10))
}

// Descriptor implements l3xc.l3xc (l3xc_update / l3xc_del / l3xc_dump): every packet received on
// the interface is forwarded via the paths, bypassing the FIB lookup. One object per
// (interface, address family); paths are replaced in place by Update.
type Descriptor struct {
	client vpp.Client
	owner  string
}

// New returns the descriptor for owner.
func New(c vpp.Client, owner string) *Descriptor { return &Descriptor{c, owner} }

func (d *Descriptor) svc() l3xcapi.RPCService { return l3xcapi.NewServiceClient(d.client) }

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return L3xcName }

func af(o *L3Xc) string {
	if o.GetIpv6() {
		return "ip6"
	}
	return "ip4"
}

// KeyOf implements scheduler.Descriptor.
func (*Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*L3Xc)
	return scheduler.Join(L3xcName, iface.RefID(o.GetInterface()), af(o))
}

// Dependencies implements scheduler.Descriptor.
func (*Descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*L3Xc)
	deps := []scheduler.Dependency{{Key: scheduler.Key(o.GetInterface())}}
	seen := map[scheduler.Key]bool{}
	for _, p := range o.GetPaths() {
		if p.GetInterface() != "" {
			if k := scheduler.Key(p.GetInterface()); !seen[k] {
				seen[k] = true
				deps = append(deps, scheduler.Dependency{Key: k})
			}
		}
		if p.GetTable() != 0 {
			if k := TableKey(p.GetTable()); !seen[k] {
				seen[k] = true
				deps = append(deps, scheduler.Dependency{Key: k})
			}
		}
	}
	return deps
}

// SortPaths orders paths canonically (next_hop, interface, table); Retrieve returns them in this
// order and the desired state must too.
func SortPaths(paths []*Path) {
	sort.SliceStable(paths, func(i, j int) bool {
		a, b := paths[i], paths[j]
		if a.GetNextHop() != b.GetNextHop() {
			return a.GetNextHop() < b.GetNextHop()
		}
		if a.GetInterface() != b.GetInterface() {
			return a.GetInterface() < b.GetInterface()
		}
		return a.GetTable() < b.GetTable()
	})
}

func encodePaths(o *L3Xc, t *iface.Table) ([]fib_types.FibPath, error) {
	out := make([]fib_types.FibPath, 0, len(o.GetPaths()))
	for _, p := range o.GetPaths() {
		if p.GetWeight() > 255 || p.GetPreference() > 255 {
			return nil, fmt.Errorf("l3xc: weight/preference out of range in path %v", p)
		}
		fp := fib_types.FibPath{
			SwIfIndex: iface.AllInterfaces, TableID: p.GetTable(), Weight: uint8(p.GetWeight()), Preference: uint8(p.GetPreference()), //nolint:gosec // range-checked
			Type: fib_types.FIB_API_PATH_TYPE_NORMAL, Flags: fib_types.FIB_API_PATH_FLAG_NONE, Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4,
		}
		if o.GetIpv6() {
			fp.Proto = fib_types.FIB_API_PATH_NH_PROTO_IP6
		}
		if p.GetInterface() != "" {
			idx, err := t.Index(p.GetInterface())
			if err != nil {
				return nil, err
			}
			fp.SwIfIndex = idx
		}
		if p.GetNextHop() != "" {
			a, err := netip.ParseAddr(p.GetNextHop())
			if err != nil {
				return nil, fmt.Errorf("l3xc: invalid next hop %q", p.GetNextHop())
			}
			switch {
			case a.Is4() && !o.GetIpv6():
				fp.Nh.Address = ip_types.AddressUnionIP4(ip_types.IP4Address(a.As4()))
			case a.Is6() && !a.Is4In6() && o.GetIpv6():
				fp.Nh.Address = ip_types.AddressUnionIP6(ip_types.IP6Address(a.As16()))
			default:
				return nil, fmt.Errorf("l3xc: next hop %q does not match the object's address family", p.GetNextHop())
			}
		}
		out = append(out, fp)
	}
	return out, nil
}

func decodePaths(x *l3xcapi.L3xc, t *iface.Table) ([]*Path, bool) {
	out := make([]*Path, 0, len(x.Paths))
	for _, fp := range x.Paths {
		p := &Path{Table: fp.TableID, Weight: uint32(fp.Weight), Preference: uint32(fp.Preference)}
		if fp.SwIfIndex != iface.AllInterfaces {
			key, ok := t.Ref(fp.SwIfIndex) // ours or untagged; never another owner's
			if !ok {
				return nil, false
			}
			p.Interface = string(key)
		}
		var a netip.Addr
		if x.IsIP6 {
			a = netip.AddrFrom16(fp.Nh.Address.GetIP6())
		} else {
			a = netip.AddrFrom4(fp.Nh.Address.GetIP4())
		}
		if !a.IsUnspecified() {
			p.NextHop = a.String()
		}
		out = append(out, p)
	}
	SortPaths(out)
	return out, true
}

func (d *Descriptor) update(ctx context.Context, o *L3Xc) (uint32, error) {
	if len(o.GetPaths()) == 0 {
		return 0, errors.New("l3xc: at least one path is required")
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return 0, err
	}
	idx, err := t.Index(o.GetInterface())
	if err != nil {
		return 0, err
	}
	paths, err := encodePaths(o, t)
	if err != nil {
		return 0, err
	}
	_, err = d.svc().L3xcUpdate(ctx, &l3xcapi.L3xcUpdate{L3xc: l3xcapi.L3xc{
		SwIfIndex: interface_types.InterfaceIndex(idx), IsIP6: o.GetIpv6(), NPaths: uint8(len(paths)), Paths: paths, //nolint:gosec // bounded by API
	}})
	if err != nil {
		return 0, fmt.Errorf("l3xc_update: %w", err)
	}
	return idx, nil
}

// Create implements scheduler.Descriptor.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*L3Xc)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.update(ctx, o)
	if err != nil {
		return nil, err
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	return iface.Meta{SwIfIndex: idx}, t.ClaimIfUntagged(idx, claimHolder(o.GetIpv6()))
}

// claimHolder is the ClaimStore holder of an l3xc on an untagged interface (one per family).
func claimHolder(ipv6 bool) string {
	if ipv6 {
		return L3xcName + "/ip6"
	}
	return L3xcName + "/ip4"
}

// Normalize implements scheduler.Normalizer (review M4): references in canonical alias form,
// next hops via netip, weight 0 → 1 (VPP stores 0 as 1, fib_api.c) and paths sorted (SortPaths),
// exactly as Retrieve reports them.
func (*Descriptor) Normalize(obj proto.Message) proto.Message {
	o, ok := obj.(*L3Xc)
	if !ok || o == nil {
		return obj
	}
	out := iface.NormalizeRefs(o, "interface").(*L3Xc)
	for i, p := range out.GetPaths() {
		np := iface.NormalizeRefs(p, "interface").(*Path)
		if np.GetWeight() == 0 {
			np.Weight = 1
		}
		if a, err := netip.ParseAddr(np.GetNextHop()); err == nil {
			np.NextHop = a.String()
		}
		out.Paths[i] = np
	}
	SortPaths(out.Paths)
	return out
}

// Update implements scheduler.Descriptor.
func (d *Descriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return nil, err
	}
	o, n := oldObj.(*L3Xc), newObj.(*L3Xc)
	if o.GetInterface() != n.GetInterface() || o.GetIpv6() != n.GetIpv6() {
		return nil, scheduler.ErrRecreate
	}
	if _, err := d.update(ctx, n); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	o, ok := obj.(*L3Xc)
	if !ok {
		return ErrEmptyValue
	}
	if _, err := d.svc().L3xcDel(ctx, &l3xcapi.L3xcDel{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex), IsIP6: o.GetIpv6()}); err != nil {
		return fmt.Errorf("l3xc_del: %w", err)
	}
	_ = iface.ReleaseRef(d.owner, o.GetInterface(), claimHolder(o.GetIpv6()))
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().L3xcDump(ctx, &l3xcapi.L3xcDump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("l3xc_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("l3xc_dump: %w", err)
		}
		idx := uint32(row.L3xc.SwIfIndex)
		key, ok := t.OwnedRef(idx, claimHolder(row.L3xc.IsIP6))
		if !ok {
			continue
		}
		paths, ok := decodePaths(&row.L3xc, t)
		if !ok {
			continue // a path points at an interface we cannot name
		}
		v := &L3Xc{Interface: string(key), Ipv6: row.L3xc.IsIP6, Paths: paths}
		out = append(out, scheduler.KV{Key: scheduler.Join(L3xcName, key.ID(), af(v)), Value: v, Meta: iface.Meta{SwIfIndex: idx}})
	}
	return out, nil
}

// Register registers the l3xc descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) { r.Register(New(c, owner)) }
