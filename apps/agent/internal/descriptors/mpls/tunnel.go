package mpls

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TunnelMeta is the runtime handle of a tunnel: its interface and tunnel index.
type TunnelMeta struct {
	SwIfIndex   uint32
	TunnelIndex uint32
}

// TunnelDescriptor manages mpls-tunnel objects.
type TunnelDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*TunnelDescriptor)(nil)

// NewTunnel returns the mpls-tunnel descriptor.
func NewTunnel(c vpp.Client, owner string, opts ...df7.Option) *TunnelDescriptor {
	return &TunnelDescriptor{df7.NewBase(NameTunnel, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *TunnelDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	t, _ := df7.Decode[Tunnel](obj)
	return KeyTunnel(t.Name)
}

// Dependencies implements scheduler.Descriptor: the next-hop interfaces (optional).
func (d *TunnelDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, _ := df7.Decode[Tunnel](obj)
	var deps []scheduler.Dependency
	for _, n := range df7.PathInterfaces(t.Paths) {
		deps = append(deps, scheduler.Dependency{Key: d.Opts.InterfaceKey(n), Optional: true})
	}
	return deps
}

// ownedTunnel is one tunnel of this owner as dumped.
type ownedTunnel struct {
	Name   string
	Detail *mpls.MplsTunnel
}

func (d *TunnelDescriptor) dump(ctx context.Context, sw uint32) ([]ownedTunnel, error) {
	stream, err := mpls.NewServiceClient(d.Client).MplsTunnelDump(ctx, &mpls.MplsTunnelDump{SwIfIndex: interface_types.InterfaceIndex(sw)})
	if err != nil {
		return nil, d.Wrap("mpls_tunnel_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("mpls_tunnel_dump", err)
	}
	var out []ownedTunnel
	for _, det := range dets {
		name, ok := vpp.ParseOwnerTag(strings.TrimRight(det.MtTunnel.MtTag, "\x00"), d.Owner)
		if !ok {
			continue
		}
		t := det.MtTunnel
		out = append(out, ownedTunnel{Name: name, Detail: &t})
	}
	// lowest sw_if_index first: it is the canonical tunnel of a duplicated name
	sort.Slice(out, func(i, j int) bool { return out[i].Detail.MtSwIfIndex < out[j].Detail.MtSwIfIndex })
	return out, nil
}

// Create implements scheduler.Descriptor: mpls_tunnel_add_del with sw_if_index ~0 creates the
// tunnel with all its paths; the new interface is tagged with the owner tag of the tunnel name,
// so its logical name (D-069) is the tunnel name and other objects can reference
// interface/<name>. A tagging failure removes the tunnel again (no orphan).
func (d *TunnelDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := df7.DecodeValid[Tunnel](obj)
	if err != nil {
		return nil, err
	}
	tag, err := vpp.OwnerTag(d.Owner, t.Name)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	paths, err := df7.EncodePaths(t.Paths, ifs)
	if err != nil {
		return nil, err
	}
	rep, err := mpls.NewServiceClient(d.Client).MplsTunnelAddDel(ctx, &mpls.MplsTunnelAddDel{MtIsAdd: true, MtTunnel: mpls.MplsTunnel{
		MtSwIfIndex: interface_types.InterfaceIndex(df7.NoIndex), MtL2Only: t.L2Only, MtIsMulticast: t.Multicast, MtTag: tag,
		MtNPaths: uint8(len(paths)), MtPaths: paths}}) //nolint:gosec // ≤ 255
	if err != nil {
		return nil, d.Wrap("mpls_tunnel_add_del (add) "+t.Name, err)
	}
	m := TunnelMeta{SwIfIndex: uint32(rep.SwIfIndex), TunnelIndex: rep.TunnelIndex}
	if _, err := interfaces.NewServiceClient(d.Client).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		_ = d.remove(ctx, m.SwIfIndex, paths)
		return nil, d.Wrap("sw_interface_tag_add_del "+t.Name, err)
	}
	return m, nil
}

// remove sends the tunnel's own paths back as a delete: VPP removes them, and deletes the
// tunnel once no path is left (an empty path list would remove nothing).
func (d *TunnelDescriptor) remove(ctx context.Context, sw uint32, paths []fibPath) error {
	_, err := mpls.NewServiceClient(d.Client).MplsTunnelAddDel(ctx, &mpls.MplsTunnelAddDel{MtIsAdd: false, MtTunnel: mpls.MplsTunnel{
		MtSwIfIndex: interface_types.InterfaceIndex(sw), MtNPaths: uint8(len(paths)), MtPaths: paths}}) //nolint:gosec // ≤ 255
	return d.Wrap(fmt.Sprintf("mpls_tunnel_add_del (del) %d", sw), err)
}

// Update implements scheduler.Descriptor: flags and paths are recreated (a new tunnel
// interface; dependents are re-created by the scheduler).
func (*TunnelDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: the tunnel is found again by its owner tag right
// before the delete (D-071: never a stored sw_if_index), and removed with the paths VPP reports
// for it. A tunnel that is gone is success.
func (d *TunnelDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := df7.Decode[Tunnel](obj)
	if err != nil {
		return err
	}
	all, err := d.dump(ctx, df7.NoIndex)
	if err != nil {
		return err
	}
	name, dupSw := t.Name, ""
	if i := strings.LastIndex(t.Name, "#"); i > 0 { // a duplicate reported by Retrieve
		name, dupSw = t.Name[:i], t.Name[i+1:]
	}
	for _, o := range all {
		if o.Name != name || (dupSw != "" && dupSw != u32(uint32(o.Detail.MtSwIfIndex))) {
			continue
		}
		if err := d.remove(ctx, uint32(o.Detail.MtSwIfIndex), o.Detail.MtPaths); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: mpls_tunnel_dump (all), tunnels tagged with this
// owner's tag; a name seen twice (a lost reply followed by a retry) is reported once per extra
// tunnel as "<name>#<sw_if_index>" — a key never desired, so the scheduler deletes it.
func (d *TunnelDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	all, err := d.dump(ctx, df7.NoIndex)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	seen := map[string]bool{}
	for _, o := range all {
		name := o.Name
		if seen[name] {
			name = fmt.Sprintf("%s#%d", o.Name, o.Detail.MtSwIfIndex)
		}
		seen[o.Name] = true
		t := Tunnel{Name: name, L2Only: o.Detail.MtL2Only, Multicast: o.Detail.MtIsMulticast, Paths: df7.DecodePaths(o.Detail.MtPaths, ifs)}
		out = append(out, df7.KV(KeyTunnel(name), t, TunnelMeta{SwIfIndex: uint32(o.Detail.MtSwIfIndex), TunnelIndex: o.Detail.MtTunnelIndex}))
	}
	sortKVs(out)
	return out, nil
}

type fibPath = fib_types.FibPath
